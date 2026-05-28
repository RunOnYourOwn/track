package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/RunOnYourOwn/track/web"
	"github.com/RunOnYourOwn/track/web/api"
	"github.com/spf13/cobra"
)

func pidFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".track", "track.pid")
}

func logFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".track", "track.log")
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(serverStatusCmd)
	serveCmd.Flags().Int("port", 3011, "Port to serve on")
	serveCmd.Flags().Bool("foreground", false, "Run in foreground (don't daemonize)")
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Launch web UI",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		fg, _ := cmd.Flags().GetBool("foreground")

		// If not foreground and not already the child process, fork and exit
		if !fg && os.Getenv("_TRACK_CHILD") == "" {
			// Stop any existing server first, and wait for it to exit so the new
			// child can bind the port (otherwise the restart races and the child
			// fails with "address already in use").
			if data, err := os.ReadFile(pidFile()); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
					if proc, err := os.FindProcess(pid); err == nil {
						_ = proc.Signal(syscall.SIGTERM)
						for i := 0; i < 50; i++ {
							if sigErr := proc.Signal(syscall.Signal(0)); sigErr != nil {
								break // old process has exited
							}
							time.Sleep(100 * time.Millisecond)
						}
					}
				}
				_ = os.Remove(pidFile())
			}

			exe, _ := os.Executable()
			child := exec.Command(exe, "serve", "--port", strconv.Itoa(port), "--foreground")
			child.Env = append(os.Environ(), "_TRACK_CHILD=1")

			lf, lfErr := os.OpenFile(logFile(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if lfErr != nil {
				return fmt.Errorf("open log file: %w", lfErr)
			}
			child.Stdout = lf
			child.Stderr = lf
			defer lf.Close()
			setDetach(child)

			if err := child.Start(); err != nil {
				return fmt.Errorf("failed to start background server: %w", err)
			}
			fmt.Printf("Track: http://localhost:%d (PID %d)\n", port, child.Process.Pid)
			return nil
		}

		conn, err := db.Open()
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}

		// Write PID file
		_ = os.WriteFile(pidFile(), []byte(strconv.Itoa(os.Getpid())), 0644)

		mux := http.NewServeMux()
		api.RegisterRoutes(mux, conn)

		// Compute build fingerprint from embedded files for cache-busting verification
		staticFS, _ := fs.Sub(web.StaticFiles, "static")
		buildHash := computeStaticHash(staticFS)

		// Serve embedded static files — wrapper guarantees no-cache headers
		staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))
		mux.Handle("GET /static/", noCacheMiddleware(buildHash, staticHandler))

		// SPA fallback: serve index.html for all non-API, non-static routes
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			w.Header().Set("X-Track-Build", buildHash)
			http.ServeFileFS(w, r, staticFS, "index.html")
		})

		srv := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: securityMiddleware(mux)}

		// Graceful shutdown: drain in-flight requests on SIGTERM/SIGINT, then let
		// PersistentPostRun close the DB (which checkpoints WAL).
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			<-sigCh
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
		}()

		err = srv.ListenAndServe()
		_ = os.Remove(pidFile())
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	},
}

// securityMiddleware recovers panics (so one bad request can't crash the daemon)
// and sets security headers — including the Content-Security-Policy — on EVERY
// response, not just /api/* . The CSP forbids inline scripts, which is what
// neutralizes injected markup like <img onerror=...> in the rendered DOM.
func securityMiddleware(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}
		}()
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

var serverStatusCmd = &cobra.Command{
	Use:   "server-status",
	Short: "Check if the web server is running",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(pidFile())
		if err != nil {
			fmt.Println("Server: not running (no PID file)")
			return nil
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			fmt.Println("Server: not running (invalid PID file)")
			return nil
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			fmt.Printf("Server: not running (PID %d not found)\n", pid)
			return nil
		}
		// Signal 0 checks if process exists without killing it
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			fmt.Printf("Server: not running (PID %d dead)\n", pid)
			_ = os.Remove(pidFile())
			return nil
		}
		fmt.Printf("Server: running (PID %d)\n", pid)
		return nil
	},
}

// noCacheWriter wraps http.ResponseWriter to inject no-cache headers
// right before the status code is written — guarantees they appear on the wire.
type noCacheWriter struct {
	http.ResponseWriter
	buildHash   string
	wroteHeader bool
}

func (w *noCacheWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.ResponseWriter.Header().Set("Pragma", "no-cache")
		w.ResponseWriter.Header().Set("Expires", "0")
		w.ResponseWriter.Header().Set("X-Track-Build", w.buildHash)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *noCacheWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func noCacheMiddleware(buildHash string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&noCacheWriter{ResponseWriter: w, buildHash: buildHash}, r)
	})
}

func computeStaticHash(staticFS fs.FS) string {
	h := sha256.New()
	fs.WalkDir(staticFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(staticFS, path)
		if err == nil {
			h.Write(data)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:8]
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running web server",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(pidFile())
		if err != nil {
			return fmt.Errorf("no running server found (no PID file)")
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return fmt.Errorf("invalid PID file")
		}

		process, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("process %d not found", pid)
		}

		if err := process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to stop process %d: %w", pid, err)
		}

		_ = os.Remove(pidFile())
		fmt.Printf("Stopped track server (PID %d)\n", pid)
		return nil
	},
}
