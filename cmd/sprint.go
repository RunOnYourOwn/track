package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/spf13/cobra"
)

var sprintCmd = &cobra.Command{
	Use:   "sprint",
	Short: "Manage sprints",
}

var sprintCreateCmd = &cobra.Command{
	Use:   "create <project-prefix> <name>",
	Short: "Create a new sprint",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := db.Open()
		proj, err := db.GetProjectByPrefix(d, args[0])
		if err != nil {
			return fmt.Errorf("project not found: %w", err)
		}

		goal, _ := cmd.Flags().GetString("goal")
		startDate, _ := cmd.Flags().GetString("start")
		endDate, _ := cmd.Flags().GetString("end")

		sprint, err := db.CreateSprint(d, db.CreateSprintOpts{
			ProjectID: proj.ID,
			Name:      args[1],
			Goal:      goal,
			StartDate: startDate,
			EndDate:   endDate,
		})
		if err != nil {
			return fmt.Errorf("create sprint: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(sprint)
		}

		fmt.Printf("Created sprint: %s (%s)\n", sprint.Name, sprint.ID[:8])
		return nil
	},
}

var sprintListCmd = &cobra.Command{
	Use:   "list <project-prefix>",
	Short: "List sprints for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := db.Open()
		proj, err := db.GetProjectByPrefix(d, args[0])
		if err != nil {
			return fmt.Errorf("project not found: %w", err)
		}

		sprints, err := db.ListSprints(d, proj.ID)
		if err != nil {
			return fmt.Errorf("list sprints: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(sprints)
		}

		if len(sprints) == 0 {
			fmt.Println("No sprints.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tSTART\tEND")
		for _, s := range sprints {
			start := "—"
			if s.StartDate != nil {
				start = *s.StartDate
			}
			end := "—"
			if s.EndDate != nil {
				end = *s.EndDate
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID[:8], s.Name, s.Status, start, end)
		}
		w.Flush()
		return nil
	},
}

var sprintStartCmd = &cobra.Command{
	Use:   "start <sprint-id>",
	Short: "Start a sprint (set status to active)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := db.Open()
		sid, err := db.ResolveSprintID(d, args[0])
		if err != nil {
			return err
		}
		if err := db.UpdateSprintStatus(d, sid, "active"); err != nil {
			return fmt.Errorf("start sprint: %w", err)
		}
		fmt.Println("Sprint started.")
		return nil
	},
}

var sprintCompleteCmd = &cobra.Command{
	Use:   "complete <sprint-id>",
	Short: "Complete a sprint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := db.Open()
		sid, err := db.ResolveSprintID(d, args[0])
		if err != nil {
			return err
		}
		if err := db.UpdateSprintStatus(d, sid, "completed"); err != nil {
			return fmt.Errorf("complete sprint: %w", err)
		}
		fmt.Println("Sprint completed.")
		return nil
	},
}

var sprintAddCmd = &cobra.Command{
	Use:   "add <sprint-id> <task-id>",
	Short: "Add a task to a sprint",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := db.Open()
		sid, err := db.ResolveSprintID(d, args[0])
		if err != nil {
			return err
		}
		taskID, err := resolveID(args[1])
		if err != nil {
			return err
		}
		if err := db.AddTaskToSprint(d, sid, taskID); err != nil {
			return fmt.Errorf("add task to sprint: %w", err)
		}
		fmt.Println("Task added to sprint.")
		return nil
	},
}

var sprintRemoveCmd = &cobra.Command{
	Use:   "remove <sprint-id> <task-id>",
	Short: "Remove a task from a sprint",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := db.Open()
		sid, err := db.ResolveSprintID(d, args[0])
		if err != nil {
			return err
		}
		taskID, err := resolveID(args[1])
		if err != nil {
			return err
		}
		if err := db.RemoveTaskFromSprint(d, sid, taskID); err != nil {
			return fmt.Errorf("remove task from sprint: %w", err)
		}
		fmt.Println("Task removed from sprint.")
		return nil
	},
}

var sprintTasksCmd = &cobra.Command{
	Use:   "tasks <sprint-id>",
	Short: "List tasks in a sprint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, _ := db.Open()
		sid, err := db.ResolveSprintID(d, args[0])
		if err != nil {
			return err
		}
		tasks, err := db.ListSprintTasks(d, sid)
		if err != nil {
			return fmt.Errorf("list sprint tasks: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(tasks)
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks in sprint.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "SEQ\tTYPE\tSTATUS\tPRIORITY\tTITLE")
		for _, t := range tasks {
			title := t.Title
			if len(title) > 50 {
				title = title[:49] + "…"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", t.Seq, t.Type, t.Status, t.Priority, title)
		}
		w.Flush()
		return nil
	},
}

func init() {
	sprintCreateCmd.Flags().String("goal", "", "Sprint goal")
	sprintCreateCmd.Flags().String("start", "", "Start date (YYYY-MM-DD)")
	sprintCreateCmd.Flags().String("end", "", "End date (YYYY-MM-DD)")

	sprintCmd.AddCommand(sprintCreateCmd)
	sprintCmd.AddCommand(sprintListCmd)
	sprintCmd.AddCommand(sprintStartCmd)
	sprintCmd.AddCommand(sprintCompleteCmd)
	sprintCmd.AddCommand(sprintAddCmd)
	sprintCmd.AddCommand(sprintRemoveCmd)
	sprintCmd.AddCommand(sprintTasksCmd)
	rootCmd.AddCommand(sprintCmd)
}
