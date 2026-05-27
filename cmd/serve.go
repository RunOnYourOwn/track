package cmd

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

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
			// Stop any existing server first
			if data, err := os.ReadFile(pidFile()); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
					if proc, err := os.FindProcess(pid); err == nil {
						_ = proc.Signal(syscall.SIGTERM)
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

		// Serve embedded static files
		staticFS, _ := fs.Sub(web.StaticFiles, "static")
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

		// SPA fallback: serve index.html for all non-API, non-static routes
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, staticFS, "index.html")
		})

		// Handle graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			<-sigCh
			_ = os.Remove(pidFile())
			_ = db.Close()
			os.Exit(0)
		}()

		addr := fmt.Sprintf(":%d", port)
		return http.ListenAndServe(addr, mux)
	},
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
