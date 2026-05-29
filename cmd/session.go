package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionStartCmd)
	sessionCmd.AddCommand(sessionEndCmd)
	sessionCmd.AddCommand(sessionCurrentCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionLogCmd)

	sessionStartCmd.Flags().String("project", "", "Project prefix (required)")
	sessionStartCmd.Flags().String("branch", "", "Git branch")
	_ = sessionStartCmd.MarkFlagRequired("project")

	sessionEndCmd.Flags().String("summary", "", "Session summary")

	sessionListCmd.Flags().String("project", "", "Filter by project")
	sessionListCmd.Flags().Int("limit", 10, "Max results")

	sessionLogCmd.Flags().Float64("hours", 0, "Hours to log")
	sessionLogCmd.Flags().String("note", "", "Note")
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Track work sessions",
}

var sessionStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new session",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		projectPrefix, _ := cmd.Flags().GetString("project")
		branch, _ := cmd.Flags().GetString("branch")

		p, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		// Check for existing open session
		existing, _ := db.GetCurrentSession(conn, p.ID)
		if existing != nil {
			return fmt.Errorf("session already active (started %s). End it first with 'track session end'", existing.StartedAt.Format("2006-01-02 15:04"))
		}

		session, err := db.StartSession(conn, p.ID, branch)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(session)
		}

		fmt.Printf("Session started for %s", p.Prefix)
		if branch != "" {
			fmt.Printf(" on %s", branch)
		}
		fmt.Printf(" (ID: %s)\n", session.ID[:8])
		return nil
	},
}

var sessionEndCmd = &cobra.Command{
	Use:   "end",
	Short: "End the current session",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		summary, _ := cmd.Flags().GetString("summary")

		session, err := db.GetCurrentSession(conn, "")
		if err != nil || session == nil {
			return fmt.Errorf("no active session")
		}

		if err := db.EndSession(conn, session.ID, summary); err != nil {
			return err
		}

		fmt.Printf("Session ended")
		if summary != "" {
			fmt.Printf(": %s", summary)
		}
		fmt.Println()
		return nil
	},
}

var sessionCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show active session",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		session, _ := db.GetCurrentSession(conn, "")
		if session == nil {
			fmt.Println("No active session.")
			return nil
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(session)
		}

		prefix := getPrefix(conn, session.ProjectID, "")
		fmt.Printf("Active session: %s", prefix)
		if session.Branch != "" {
			fmt.Printf(" (%s)", session.Branch)
		}
		fmt.Printf("\nStarted: %s\n", session.StartedAt.Format("2006-01-02 15:04"))
		return nil
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		projectPrefix, _ := cmd.Flags().GetString("project")
		limit, _ := cmd.Flags().GetInt("limit")

		var projectID string
		if projectPrefix != "" {
			p, err := db.GetProjectByPrefix(conn, projectPrefix)
			if err != nil {
				return fmt.Errorf("project %q not found", projectPrefix)
			}
			projectID = p.ID
		}

		sessions, err := db.ListSessions(conn, projectID, limit)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(sessions)
		}

		if len(sessions) == 0 {
			fmt.Println("No sessions.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DATE\tPROJECT\tBRANCH\tSUMMARY")
		for _, s := range sessions {
			prefix := getPrefix(conn, s.ProjectID, "")
			date := s.StartedAt.Format("2006-01-02")
			summary := s.Summary
			if len(summary) > 50 {
				summary = summary[:47] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", date, prefix, s.Branch, summary)
		}
		return w.Flush()
	},
}

var sessionLogCmd = &cobra.Command{
	Use:   "log [task-id]",
	Short: "Log time to a task in the current session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		hours, _ := cmd.Flags().GetFloat64("hours")
		note, _ := cmd.Flags().GetString("note")

		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}

		// Get current session ID if active
		var sessionID string
		session, _ := db.GetCurrentSession(conn, "")
		if session != nil {
			sessionID = session.ID
		}

		if err := db.LogTime(conn, taskID, sessionID, hours, note); err != nil {
			return err
		}

		fmt.Printf("Logged %.1fh to %s\n", hours, args[0])
		return nil
	},
}
