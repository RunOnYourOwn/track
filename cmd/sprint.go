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
	Run: func(cmd *cobra.Command, args []string) {
		d, _ := db.Open()
		proj, err := db.GetProjectByPrefix(d, args[0])
		if err != nil {
			exitErr("project not found: %v", err)
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
			exitErr("create sprint: %v", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(sprint)
			return
		}

		fmt.Printf("Created sprint: %s (%s)\n", sprint.Name, sprint.ID[:8])
	},
}

var sprintListCmd = &cobra.Command{
	Use:   "list <project-prefix>",
	Short: "List sprints for a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		d, _ := db.Open()
		proj, err := db.GetProjectByPrefix(d, args[0])
		if err != nil {
			exitErr("project not found: %v", err)
		}

		sprints, err := db.ListSprints(d, proj.ID)
		if err != nil {
			exitErr("list sprints: %v", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(sprints)
			return
		}

		if len(sprints) == 0 {
			fmt.Println("No sprints.")
			return
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
	},
}

var sprintStartCmd = &cobra.Command{
	Use:   "start <sprint-id>",
	Short: "Start a sprint (set status to active)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		d, _ := db.Open()
		if err := db.UpdateSprintStatus(d, args[0], "active"); err != nil {
			exitErr("start sprint: %v", err)
		}
		fmt.Println("Sprint started.")
	},
}

var sprintCompleteCmd = &cobra.Command{
	Use:   "complete <sprint-id>",
	Short: "Complete a sprint",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		d, _ := db.Open()
		if err := db.UpdateSprintStatus(d, args[0], "completed"); err != nil {
			exitErr("complete sprint: %v", err)
		}
		fmt.Println("Sprint completed.")
	},
}

var sprintAddCmd = &cobra.Command{
	Use:   "add <sprint-id> <task-id>",
	Short: "Add a task to a sprint",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		d, _ := db.Open()
		if err := db.AddTaskToSprint(d, args[0], args[1]); err != nil {
			exitErr("add task to sprint: %v", err)
		}
		fmt.Println("Task added to sprint.")
	},
}

var sprintRemoveCmd = &cobra.Command{
	Use:   "remove <sprint-id> <task-id>",
	Short: "Remove a task from a sprint",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		d, _ := db.Open()
		if err := db.RemoveTaskFromSprint(d, args[0], args[1]); err != nil {
			exitErr("remove task from sprint: %v", err)
		}
		fmt.Println("Task removed from sprint.")
	},
}

var sprintTasksCmd = &cobra.Command{
	Use:   "tasks <sprint-id>",
	Short: "List tasks in a sprint",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		d, _ := db.Open()
		tasks, err := db.ListSprintTasks(d, args[0])
		if err != nil {
			exitErr("list sprint tasks: %v", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(tasks)
			return
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks in sprint.")
			return
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
