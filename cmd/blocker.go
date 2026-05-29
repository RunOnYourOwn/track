package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/spf13/cobra"
)

// validBlockerTypes lists accepted blocker_type values.
var validBlockerTypes = []string{
	"approval",
	"third_party",
	"data_dependency",
	"stakeholder",
	"external_system",
}

func init() {
	rootCmd.AddCommand(blockerCmd)
	blockerCmd.AddCommand(blockerCreateCmd)
	blockerCmd.AddCommand(blockerResolveCmd)
	blockerCmd.AddCommand(blockerListCmd)

	// create flags
	blockerCreateCmd.Flags().String("project", "", "Project prefix (required)")
	blockerCreateCmd.Flags().String("title", "", "Short description of the blocker (required)")
	blockerCreateCmd.Flags().String("type", "", "Blocker type: approval, third_party, data_dependency, stakeholder, external_system (required)")
	blockerCreateCmd.Flags().String("task", "", "Associated task ID (optional, e.g. PROJ-5)")
	blockerCreateCmd.Flags().String("owner", "", "Team or person responsible for resolution")
	blockerCreateCmd.Flags().String("escalation-date", "", "Date when escalation is needed (YYYY-MM-DD)")
	blockerCreateCmd.Flags().String("notes", "", "Additional notes")
	_ = blockerCreateCmd.MarkFlagRequired("project")
	_ = blockerCreateCmd.MarkFlagRequired("title")
	_ = blockerCreateCmd.MarkFlagRequired("type")

	// list flags
	blockerListCmd.Flags().String("project", "", "Project prefix (required)")
	blockerListCmd.Flags().Bool("open", false, "Show only open (unresolved) blockers")
	_ = blockerListCmd.MarkFlagRequired("project")
}

var blockerCmd = &cobra.Command{
	Use:   "blocker",
	Short: "Manage blockers",
}

var blockerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new blocker",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		projectPrefix, _ := cmd.Flags().GetString("project")
		title, _ := cmd.Flags().GetString("title")
		blockerType, _ := cmd.Flags().GetString("type")
		taskFlag, _ := cmd.Flags().GetString("task")
		owner, _ := cmd.Flags().GetString("owner")
		escalationDate, _ := cmd.Flags().GetString("escalation-date")
		notes, _ := cmd.Flags().GetString("notes")

		// Validate blocker type
		if !isValidBlockerType(blockerType) {
			return fmt.Errorf("invalid blocker type %q — must be one of: %s", blockerType, strings.Join(validBlockerTypes, ", "))
		}

		p, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		// Resolve optional task ID
		taskID := ""
		if taskFlag != "" {
			taskID, err = resolveID(taskFlag)
			if err != nil {
				return fmt.Errorf("task %q: %w", taskFlag, err)
			}
		}

		blocker, err := db.CreateBlocker(conn, p.ID, title, blockerType, taskID, owner, escalationDate, notes)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(blocker)
		}

		fmt.Printf("Blocker created: %s\n", blocker.ID[:8])
		fmt.Printf("  Type:  %s\n", blocker.BlockerType)
		fmt.Printf("  Title: %s\n", blocker.Title)
		if blocker.Owner != "" {
			fmt.Printf("  Owner: %s\n", blocker.Owner)
		}
		if blocker.EscalationDate != nil {
			fmt.Printf("  Escalation: %s\n", *blocker.EscalationDate)
		}
		return nil
	},
}

var blockerResolveCmd = &cobra.Command{
	Use:   "resolve <ID>",
	Short: "Mark a blocker as resolved",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		// Support both full ULID and short prefix (8 chars)
		id := args[0]
		resolvedID, err := resolveBlockerID(conn, id)
		if err != nil {
			return err
		}

		if err := db.ResolveBlocker(conn, resolvedID); err != nil {
			return err
		}
		fmt.Printf("Blocker %s resolved.\n", id)
		return nil
	},
}


var blockerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List blockers for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		projectPrefix, _ := cmd.Flags().GetString("project")
		openOnly, _ := cmd.Flags().GetBool("open")

		p, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		blockers, err := db.ListBlockers(conn, p.ID, openOnly)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(blockers)
		}

		if len(blockers) == 0 {
			if openOnly {
				fmt.Println("No open blockers.")
			} else {
				fmt.Println("No blockers recorded.")
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTYPE\tTITLE\tOWNER\tOPENED")
		for _, b := range blockers {
			shortID := b.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}

			title := b.Title
			if len(title) > 50 {
				title = title[:47] + "..."
			}

			indicator := ""
			if b.ResolvedAt != nil {
				indicator = "✓ "
			}

			fmt.Fprintf(w, "%s\t%s\t%s%s\t%s\t%s\n",
				shortID,
				b.BlockerType,
				indicator,
				title,
				b.Owner,
				b.OpenedAt.Format("2006-01-02"),
			)
		}
		return w.Flush()
	},
}

// --- helpers ---

func isValidBlockerType(t string) bool {
	for _, v := range validBlockerTypes {
		if t == v {
			return true
		}
	}
	return false
}

// resolveBlockerID looks up a blocker by its full ULID or a leading prefix.
// The user-facing IDs shown in list output are 8-char prefixes of the ULID.
func resolveBlockerID(conn *sql.DB, id string) (string, error) {
	// If it looks like a full ULID (26 chars, no dashes), use directly.
	if len(id) == 26 && !strings.Contains(id, "-") {
		return id, nil
	}

	// Otherwise treat it as a prefix search in the blockers table.
	var fullID string
	err := conn.QueryRow(`SELECT id FROM blockers WHERE id LIKE ? LIMIT 1`, id+"%").Scan(&fullID)
	if err != nil {
		return "", fmt.Errorf("blocker %q not found", id)
	}
	return fullID, nil
}
