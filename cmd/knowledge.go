package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/spf13/cobra"
)

func init() {
	// decision sub-commands
	rootCmd.AddCommand(decisionCmd)
	decisionCmd.AddCommand(decisionCreateCmd)
	decisionCmd.AddCommand(decisionListCmd)
	decisionCmd.AddCommand(decisionResolveCmd)
	decisionCmd.AddCommand(decisionEditCmd)

	// decision edit flags (only passed flags change)
	decisionEditCmd.Flags().String("title", "", "New title")
	decisionEditCmd.Flags().String("context", "", "New context")
	decisionEditCmd.Flags().String("options", "", "New options (JSON array or text)")
	decisionEditCmd.Flags().String("revisit-by", "", "New revisit date (YYYY-MM-DD)")
	decisionEditCmd.Flags().String("decided-by", "", "Who decides")

	// decision create flags
	decisionCreateCmd.Flags().String("project", "", "Project prefix (required)")
	decisionCreateCmd.Flags().String("title", "", "Decision title (required)")
	decisionCreateCmd.Flags().String("task", "", "Associated task display ID, e.g. PROJ-5")
	decisionCreateCmd.Flags().String("context", "", "Background / context")
	decisionCreateCmd.Flags().String("options", "", "Options considered (JSON array or free text)")
	decisionCreateCmd.Flags().String("revisit-by", "", "Date to revisit decision (YYYY-MM-DD)")
	decisionCreateCmd.Flags().String("decided-by", "", "Who decides: collaborative, individual, etc.")
	decisionCreateCmd.Flags().String("supersedes", "", "Decision ID this supersedes")
	_ = decisionCreateCmd.MarkFlagRequired("project")
	_ = decisionCreateCmd.MarkFlagRequired("title")

	// decision list flags
	decisionListCmd.Flags().String("project", "", "Filter by project prefix")
	decisionListCmd.Flags().String("status", "", "Filter by status (comma-separated, e.g. open,decided)")
	decisionListCmd.Flags().Bool("expiring", false, "Show decided decisions with revisit_by within 7 days")

	// decision resolve flags
	decisionResolveCmd.Flags().String("decision", "", "The decision made (required)")
	decisionResolveCmd.Flags().String("rationale", "", "Why this decision was made (required)")
	_ = decisionResolveCmd.MarkFlagRequired("decision")
	_ = decisionResolveCmd.MarkFlagRequired("rationale")

	// learn sub-commands
	rootCmd.AddCommand(learnCmd)
	learnCmd.AddCommand(learnCreateCmd)
	learnCmd.AddCommand(learnSearchCmd)
	learnCmd.AddCommand(learnListCmd)
	learnCmd.AddCommand(learnEditCmd)

	// learn edit flags (only passed flags change)
	learnEditCmd.Flags().String("title", "", "New title")
	learnEditCmd.Flags().String("body", "", "New body")
	learnEditCmd.Flags().String("category", "", "New category: pattern, pitfall, tool, process, other")
	learnEditCmd.Flags().String("applies-to", "", "Comma-separated project prefixes")

	// learn create flags (the command itself acts as "learn create" via learnCreateCmd,
	// but we also keep a top-level `track learn` that just prints help)
	learnCreateCmd.Flags().String("project", "", "Project prefix (required)")
	learnCreateCmd.Flags().String("title", "", "Learning title (required)")
	learnCreateCmd.Flags().String("body", "", "Learning body / description (required)")
	learnCreateCmd.Flags().String("category", "pattern", "Category: pattern, pitfall, tool, process, other")
	learnCreateCmd.Flags().String("applies-to", "", "Comma-separated project prefixes, e.g. PROJ,ACME")
	learnCreateCmd.Flags().String("task", "", "Associated task display ID, e.g. PROJ-5")
	_ = learnCreateCmd.MarkFlagRequired("project")
	_ = learnCreateCmd.MarkFlagRequired("title")
	_ = learnCreateCmd.MarkFlagRequired("body")

	// learn list flags
	learnListCmd.Flags().String("project", "", "Filter by project prefix")
	learnListCmd.Flags().String("category", "", "Filter by category")
}

// ---- decision ----------------------------------------------------------------

var decisionCmd = &cobra.Command{
	Use:   "decision",
	Short: "Manage architectural / process decisions (ADRs)",
}

var decisionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Record a new decision",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		projectPrefix, _ := cmd.Flags().GetString("project")
		p, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		title, _ := cmd.Flags().GetString("title")
		taskDisplay, _ := cmd.Flags().GetString("task")
		context, _ := cmd.Flags().GetString("context")
		options, _ := cmd.Flags().GetString("options")
		revisitBy, _ := cmd.Flags().GetString("revisit-by")
		decidedBy, _ := cmd.Flags().GetString("decided-by")
		supersedes, _ := cmd.Flags().GetString("supersedes")

		// Resolve optional task ID
		var taskID string
		if taskDisplay != "" {
			taskID, err = resolveID(taskDisplay)
			if err != nil {
				return fmt.Errorf("task %q: %w", taskDisplay, err)
			}
		}

		// Normalise options: if not already JSON wrap as plain string list element
		if options != "" && !strings.HasPrefix(strings.TrimSpace(options), "[") {
			b, _ := json.Marshal([]string{options})
			options = string(b)
		}

		d, err := db.CreateDecision(conn, db.CreateDecisionOpts{
			ProjectID:    p.ID,
			TaskID:       taskID,
			Title:        title,
			Context:      context,
			Options:      options,
			RevisitBy:    revisitBy,
			DecidedBy:    decidedBy,
			SupersedesID: supersedes,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(d)
		}

		fmt.Printf("Decision recorded: %s\n  Status: %s\n  ID: %s\n", d.Title, d.Status, d.ID)
		return nil
	},
}

var decisionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List decisions",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		projectPrefix, _ := cmd.Flags().GetString("project")
		statusStr, _ := cmd.Flags().GetString("status")
		expiring, _ := cmd.Flags().GetBool("expiring")

		var projectID string
		if projectPrefix != "" {
			p, err := db.GetProjectByPrefix(conn, projectPrefix)
			if err != nil {
				return fmt.Errorf("project %q not found", projectPrefix)
			}
			projectID = p.ID
		}

		var statuses []string
		if statusStr != "" {
			statuses = strings.Split(statusStr, ",")
		}

		decisions, err := db.ListDecisions(conn, projectID, statuses, expiring)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(decisions)
		}

		if len(decisions) == 0 {
			fmt.Println("No decisions found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tREVISIT\tTITLE")
		for _, d := range decisions {
			revisit := ""
			if d.RevisitBy != nil {
				revisit = *d.RevisitBy
			}
			title := d.Title
			if len(title) > 60 {
				title = title[:57] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", d.ID[:8]+"…", d.Status, revisit, title)
		}
		return w.Flush()
	},
}

var decisionResolveCmd = &cobra.Command{
	Use:   "resolve <id>",
	Short: "Mark a decision as resolved with the decision made and rationale",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		decisionText, _ := cmd.Flags().GetString("decision")
		rationale, _ := cmd.Flags().GetString("rationale")

		// args[0] may be a short prefix of the ULID; do a prefix lookup
		id, err := resolveDecisionID(args[0])
		if err != nil {
			return err
		}

		if err := db.ResolveDecision(conn, id, decisionText, rationale); err != nil {
			return err
		}

		if jsonOutput {
			d, _ := db.GetDecision(conn, id)
			return json.NewEncoder(os.Stdout).Encode(d)
		}

		fmt.Printf("Decision resolved: %s\n", id)
		return nil
	},
}

// ---- learn ------------------------------------------------------------------

var learnCmd = &cobra.Command{
	Use:   "learn",
	Short: "Manage learnings / retrospective notes",
}

var learnCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Record a new learning",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		projectPrefix, _ := cmd.Flags().GetString("project")
		p, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		title, _ := cmd.Flags().GetString("title")
		body, _ := cmd.Flags().GetString("body")
		category, _ := cmd.Flags().GetString("category")
		appliesToStr, _ := cmd.Flags().GetString("applies-to")
		taskDisplay, _ := cmd.Flags().GetString("task")

		// Build JSON array for applies_to
		appliesTo := "[]"
		if appliesToStr != "" {
			parts := strings.Split(appliesToStr, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(strings.ToUpper(parts[i]))
			}
			b, _ := json.Marshal(parts)
			appliesTo = string(b)
		}

		// Resolve optional task ID
		var taskID string
		if taskDisplay != "" {
			taskID, err = resolveID(taskDisplay)
			if err != nil {
				return fmt.Errorf("task %q: %w", taskDisplay, err)
			}
		}

		l, err := db.CreateLearning(conn, db.CreateLearningOpts{
			ProjectID: p.ID,
			TaskID:    taskID,
			Title:     title,
			Body:      body,
			Category:  category,
			AppliesTo: appliesTo,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(l)
		}

		fmt.Printf("Learning recorded: %s\n  Category: %s\n  ID: %s\n", l.Title, l.Category, l.ID)
		return nil
	},
}

var learnSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search learnings by title or body",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		learnings, err := db.SearchLearnings(conn, "", args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(learnings)
		}

		if len(learnings) == 0 {
			fmt.Printf("No learnings matching %q.\n", args[0])
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCAT\tTITLE")
		for _, l := range learnings {
			title := l.Title
			if len(title) > 65 {
				title = title[:62] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", l.ID[:8]+"…", l.Category, title)
		}
		return w.Flush()
	},
}

var learnListCmd = &cobra.Command{
	Use:   "list",
	Short: "List learnings",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		projectPrefix, _ := cmd.Flags().GetString("project")
		category, _ := cmd.Flags().GetString("category")

		var projectID string
		if projectPrefix != "" {
			p, err := db.GetProjectByPrefix(conn, projectPrefix)
			if err != nil {
				return fmt.Errorf("project %q not found", projectPrefix)
			}
			projectID = p.ID
		}

		learnings, err := db.ListLearnings(conn, projectID, category)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(learnings)
		}

		if len(learnings) == 0 {
			fmt.Println("No learnings found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCAT\tAPPLIES-TO\tTITLE")
		for _, l := range learnings {
			title := l.Title
			if len(title) > 55 {
				title = title[:52] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", l.ID[:8]+"…", l.Category, l.AppliesTo, title)
		}
		return w.Flush()
	},
}

// resolveDecisionID accepts either a full ULID or a prefix match against stored decisions.
func resolveDecisionID(raw string) (string, error) {
	if len(raw) == 26 {
		return raw, nil
	}
	conn, err := db.Open()
	if err != nil {
		return "", err
	}
	var id string
	err = conn.QueryRow(`SELECT id FROM decisions WHERE id LIKE ? LIMIT 1`, raw+"%").Scan(&id)
	if err != nil {
		return "", fmt.Errorf("decision %q not found", raw)
	}
	return id, nil
}

// resolveLearningID accepts either a full ULID or a prefix match against stored learnings.
func resolveLearningID(raw string) (string, error) {
	if len(raw) == 26 {
		return raw, nil
	}
	conn, err := db.Open()
	if err != nil {
		return "", err
	}
	var id string
	err = conn.QueryRow(`SELECT id FROM learnings WHERE id LIKE ? LIMIT 1`, raw+"%").Scan(&id)
	if err != nil {
		return "", fmt.Errorf("learning %q not found", raw)
	}
	return id, nil
}

var decisionEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a decision (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		id, err := resolveDecisionID(args[0])
		if err != nil {
			return err
		}
		for _, fc := range [][2]string{{"title", "title"}, {"context", "context"}, {"options", "options"}, {"revisit-by", "revisit_by"}, {"decided-by", "decided_by"}} {
			if !cmd.Flags().Changed(fc[0]) {
				continue
			}
			v, _ := cmd.Flags().GetString(fc[0])
			if err := db.UpdateDecisionField(conn, id, fc[1], v); err != nil {
				return err
			}
		}
		fmt.Printf("Updated decision %s\n", id[:8])
		return nil
	},
}

var learnEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a learning (only the flags you pass change)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		id, err := resolveLearningID(args[0])
		if err != nil {
			return err
		}
		for _, fc := range [][2]string{{"title", "title"}, {"body", "body"}, {"category", "category"}, {"applies-to", "applies_to"}} {
			if !cmd.Flags().Changed(fc[0]) {
				continue
			}
			v, _ := cmd.Flags().GetString(fc[0])
			if err := db.UpdateLearningField(conn, id, fc[1], v); err != nil {
				return err
			}
		}
		fmt.Printf("Updated learning %s\n", id[:8])
		return nil
	},
}
