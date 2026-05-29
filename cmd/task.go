package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskGetCmd)
	taskCmd.AddCommand(taskMoveCmd)
	taskCmd.AddCommand(taskDoneCmd)
	taskCmd.AddCommand(taskCancelCmd)
	taskCmd.AddCommand(taskDeleteCmd)
	taskCmd.AddCommand(taskEditCmd)
	taskCmd.AddCommand(taskLinkCmd)
	taskCmd.AddCommand(taskUnlinkCmd)
	taskCmd.AddCommand(taskNextCmd)

	taskListCmd.Flags().String("project", "", "Filter by project prefix")
	taskListCmd.Flags().String("status", "", "Filter by status (comma-separated)")
	taskListCmd.Flags().String("priority", "", "Filter by priority (comma-separated)")
	taskListCmd.Flags().String("type", "", "Filter by type: epic, feature, task")

	taskCreateCmd.Flags().String("project", "", "Project prefix (required)")
	taskCreateCmd.Flags().String("title", "", "Task title (required)")
	taskCreateCmd.Flags().String("description", "", "Description")
	taskCreateCmd.Flags().String("priority", "medium", "Priority: urgent, high, medium, low")
	taskCreateCmd.Flags().String("estimate", "", "T-shirt size: XS, S, M, L, XL")
	taskCreateCmd.Flags().Float64("hours", 0, "Estimated hours")
	taskCreateCmd.Flags().Int("agent-minutes", 0, "Estimated agent minutes (XS:5, S:15, M:45, L:75, XL:90+)")
	taskCreateCmd.Flags().String("type", "task", "Type: epic, feature, task")
	taskCreateCmd.Flags().String("parent", "", "Parent task ID")
	taskCreateCmd.Flags().String("source", "planned", "Source: planned, discovered, stakeholder, bug, debt")
	taskCreateCmd.Flags().String("context", "", "Agent context JSON")
	taskCreateCmd.Flags().String("start-date", "", "Start date (YYYY-MM-DD)")
	taskCreateCmd.Flags().String("due", "", "Due date (YYYY-MM-DD)")
	_ = taskCreateCmd.MarkFlagRequired("project")
	_ = taskCreateCmd.MarkFlagRequired("title")

	taskMoveCmd.Flags().String("status", "", "Target status")
	_ = taskMoveCmd.MarkFlagRequired("status")

	taskDoneCmd.Flags().Float64("actual-hours", 0, "Actual hours spent")
	taskDoneCmd.Flags().String("note", "", "Completion note (what shipped / outcome)")
	taskCancelCmd.Flags().String("reason", "", "Why this task is being cancelled (stored in completion_note)")

	taskLinkCmd.Flags().String("blocks", "", "Task ID that this task blocks")
	taskLinkCmd.Flags().String("type", "blocks", "Dependency type: blocks, soft, informational")
	taskLinkCmd.Flags().String("reason", "", "Reason for dependency")
	_ = taskLinkCmd.MarkFlagRequired("blocks")

	taskUnlinkCmd.Flags().String("blocks", "", "Task ID to unlink")
	_ = taskUnlinkCmd.MarkFlagRequired("blocks")

	taskEditCmd.Flags().String("title", "", "New title")
	taskEditCmd.Flags().String("description", "", "New description")
	taskEditCmd.Flags().String("type", "", "Type: epic, feature, task")
	taskEditCmd.Flags().String("priority", "", "Priority: urgent, high, medium, low")
	taskEditCmd.Flags().String("estimate", "", "T-shirt size: XS, S, M, L, XL")
	taskEditCmd.Flags().Float64("hours", 0, "Estimated hours")
	taskEditCmd.Flags().Int("agent-minutes", 0, "Estimated agent minutes")
	taskEditCmd.Flags().String("start-date", "", "Start date (YYYY-MM-DD)")
	taskEditCmd.Flags().String("due", "", "Due date (YYYY-MM-DD)")
	taskEditCmd.Flags().String("tags", "", "Comma-separated tags")
	taskEditCmd.Flags().String("parent", "", "Parent task ID (or 'none' to unparent)")
	taskEditCmd.Flags().Int("sort-order", 0, "Sort order within parent")

	taskNextCmd.Flags().String("project", "", "Project prefix")
}

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		projectPrefix, _ := cmd.Flags().GetString("project")
		statusStr, _ := cmd.Flags().GetString("status")
		priorityStr, _ := cmd.Flags().GetString("priority")
		typeFilter, _ := cmd.Flags().GetString("type")

		opts := db.ListTaskOpts{Type: typeFilter}
		prefix := projectPrefix
		if projectPrefix != "" {
			p, err := db.GetProjectByPrefix(conn, projectPrefix)
			if err != nil {
				return fmt.Errorf("project %q not found", projectPrefix)
			}
			opts.ProjectID = p.ID
			prefix = p.Prefix
		}
		if statusStr != "" {
			opts.Status = strings.Split(statusStr, ",")
		}
		if priorityStr != "" {
			opts.Priority = strings.Split(priorityStr, ",")
		}

		tasks, err := db.ListTasks(conn, opts)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(tasks)
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTYPE\tSTATUS\tPRI\tEST\tTITLE")
		for _, t := range tasks {
			displayPrefix := prefix
			if displayPrefix == "" {
				displayPrefix = getPrefix(conn, t.ProjectID, "")
			}
			displayID := displayPrefix + "-" + strconv.Itoa(t.Seq)
			est := t.EstimateSize
			if est == "" && t.EstimateHours > 0 {
				est = fmt.Sprintf("%.1fh", t.EstimateHours)
			}
			title := t.Title
			if len(title) > 60 {
				title = title[:57] + "..."
			}
			typeLabel := t.Type
			if typeLabel == "" {
				typeLabel = "task"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", displayID, typeLabel, t.Status, t.Priority, est, title)
		}
		return w.Flush()
	},
}

var taskCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new task",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()

		projectPrefix, _ := cmd.Flags().GetString("project")
		p, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		title, _ := cmd.Flags().GetString("title")
		desc, _ := cmd.Flags().GetString("description")
		priority, _ := cmd.Flags().GetString("priority")
		taskType, _ := cmd.Flags().GetString("type")
		estimate, _ := cmd.Flags().GetString("estimate")
		hours, _ := cmd.Flags().GetFloat64("hours")
		agentMinutes, _ := cmd.Flags().GetInt("agent-minutes")
		parent, _ := cmd.Flags().GetString("parent")
		source, _ := cmd.Flags().GetString("source")
		agentCtx, _ := cmd.Flags().GetString("context")
		due, _ := cmd.Flags().GetString("due")
		startDate, _ := cmd.Flags().GetString("start-date")

		parentID := ""
		if parent != "" {
			parentID, err = resolveID(parent)
			if err != nil {
				return fmt.Errorf("parent %q: %w", parent, err)
			}
		}

		task, err := db.CreateTask(conn, db.CreateTaskOpts{
			ProjectID:            p.ID,
			Title:                title,
			Description:          desc,
			Priority:             priority,
			Type:                 taskType,
			EstimateSize:         estimate,
			EstimateHours:        hours,
			EstimateAgentMinutes: agentMinutes,
			ParentID:             parentID,
			SourceType:           source,
			AgentContext:         agentCtx,
			StartDate:            startDate,
			DueDate:              due,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(task)
		}

		fmt.Printf("Created %s-%d: %s\n", p.Prefix, task.Seq, task.Title)
		return nil
	},
}

var taskGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		task, err := db.GetTask(conn, taskID)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(task)
		}

		prefix := getPrefix(conn, task.ProjectID, "")
		fmt.Printf("%s-%d: %s\n", prefix, task.Seq, task.Title)
		fmt.Printf("Status: %s | Priority: %s | Estimate: %s (%.1fh)\n", task.Status, task.Priority, task.EstimateSize, task.EstimateHours)
		if task.Description != "" {
			fmt.Printf("\n%s\n", task.Description)
		}
		if task.AgentContext != "{}" && task.AgentContext != "" {
			fmt.Printf("\nAgent context: %s\n", task.AgentContext)
		}
		return nil
	},
}

var taskMoveCmd = &cobra.Command{
	Use:   "move [id]",
	Short: "Move task to a status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		status, _ := cmd.Flags().GetString("status")

		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		if err := db.MoveTask(conn, taskID, status); err != nil {
			return err
		}
		fmt.Printf("Moved to %s\n", status)
		return nil
	},
}

var taskDoneCmd = &cobra.Command{
	Use:   "done [id]",
	Short: "Mark task as done",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		hours, _ := cmd.Flags().GetFloat64("actual-hours")
		note, _ := cmd.Flags().GetString("note")

		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		if err := db.CompleteTask(conn, taskID, hours, note); err != nil {
			return err
		}
		fmt.Println("Done ✓")
		return nil
	},
}

var taskCancelCmd = &cobra.Command{
	Use:   "cancel [id]",
	Short: "Cancel a task (terminal, not completed) with an optional reason",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		reason, _ := cmd.Flags().GetString("reason")

		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		if err := db.CancelTask(conn, taskID, reason); err != nil {
			return err
		}
		fmt.Println("Cancelled ✗")
		return nil
	},
}

var taskDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		_ = conn
		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		return db.DeleteTask(conn, taskID)
	},
}

var taskEditCmd = &cobra.Command{
	Use:   "edit [id]",
	Short: "Edit task fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}

		fields := map[string]string{}
		if v, _ := cmd.Flags().GetString("title"); v != "" {
			fields["title"] = v
		}
		if cmd.Flags().Changed("description") {
			v, _ := cmd.Flags().GetString("description")
			fields["description"] = v // Changed → allow clearing to ""
		}
		if v, _ := cmd.Flags().GetString("type"); v != "" {
			fields["type"] = v
		}
		if v, _ := cmd.Flags().GetString("priority"); v != "" {
			fields["priority"] = v
		}
		if cmd.Flags().Changed("estimate") {
			v, _ := cmd.Flags().GetString("estimate")
			fields["estimate_size"] = v // Changed → allow un-sizing
		}
		if cmd.Flags().Changed("hours") {
			v, _ := cmd.Flags().GetFloat64("hours")
			fields["estimate_hours"] = strconv.FormatFloat(v, 'f', -1, 64) // Changed → allow reset to 0
		}
		if cmd.Flags().Changed("agent-minutes") {
			v, _ := cmd.Flags().GetInt("agent-minutes")
			fields["estimate_agent_minutes"] = strconv.Itoa(v) // Changed → allow reset to 0
		}
		if v, _ := cmd.Flags().GetString("start-date"); v != "" {
			fields["start_date"] = v
		}
		if v, _ := cmd.Flags().GetString("due"); v != "" {
			fields["due_date"] = v
		}
		if v, _ := cmd.Flags().GetString("tags"); v != "" {
			fields["tags"] = v
		}
		if cmd.Flags().Changed("sort-order") {
			v, _ := cmd.Flags().GetInt("sort-order")
			fields["sort_order"] = strconv.Itoa(v) // Changed → allow reset to 0
		}

		for field, value := range fields {
			if err := db.UpdateTaskField(conn, taskID, field, value); err != nil {
				return fmt.Errorf("updating %s: %w", field, err)
			}
		}

		if parent, _ := cmd.Flags().GetString("parent"); parent != "" {
			parentID := ""
			if parent != "none" {
				parentID, err = resolveID(parent)
				if err != nil {
					return fmt.Errorf("parent %q: %w", parent, err)
				}
			}
			if err := db.SetParentID(conn, taskID, parentID); err != nil {
				return err
			}
		}

		task, err := db.GetTask(conn, taskID)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(task)
		}

		prefix := getPrefix(conn, task.ProjectID, "")
		fmt.Printf("Updated %s-%d: %s\n", prefix, task.Seq, task.Title)
		return nil
	},
}

var taskLinkCmd = &cobra.Command{
	Use:   "link [id]",
	Short: "Create dependency between tasks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		_ = conn
		blocksStr, _ := cmd.Flags().GetString("blocks")
		depType, _ := cmd.Flags().GetString("type")
		reason, _ := cmd.Flags().GetString("reason")

		fromID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		toID, err := resolveID(blocksStr)
		if err != nil {
			return fmt.Errorf("target %q: %w", blocksStr, err)
		}

		if err := db.CreateDependency(conn, fromID, toID, depType, reason); err != nil {
			return err
		}
		fmt.Printf("Linked: %s blocks %s\n", args[0], blocksStr)
		return nil
	},
}

var taskUnlinkCmd = &cobra.Command{
	Use:   "unlink [id]",
	Short: "Remove dependency",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		_ = conn
		blocksStr, _ := cmd.Flags().GetString("blocks")

		fromID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		toID, err := resolveID(blocksStr)
		if err != nil {
			return err
		}
		return db.DeleteDependency(conn, fromID, toID)
	},
}

var taskNextCmd = &cobra.Command{
	Use:   "next",
	Short: "Suggest next task to work on",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn := mustOpen()
		projectPrefix, _ := cmd.Flags().GetString("project")

		var projectID string
		var prefix string
		if projectPrefix != "" {
			p, err := db.GetProjectByPrefix(conn, projectPrefix)
			if err != nil {
				return fmt.Errorf("project %q not found", projectPrefix)
			}
			projectID = p.ID
			prefix = p.Prefix
		} else {
			projects, _ := db.ListProjects(conn)
			if len(projects) == 1 {
				projectID = projects[0].ID
				prefix = projects[0].Prefix
			} else if len(projects) == 0 {
				return fmt.Errorf("no projects — create one first")
			} else {
				return fmt.Errorf("multiple projects — specify --project")
			}
		}

		task, err := db.SuggestNext(conn, projectID)
		if err != nil {
			return err
		}
		if task == nil {
			fmt.Println("No available tasks (all done or blocked).")
			return nil
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(task)
		}

		fmt.Printf("Next: %s-%d [%s] %s\n", prefix, task.Seq, task.Priority, task.Title)
		if task.Description != "" && len(task.Description) <= 200 {
			fmt.Printf("  %s\n", task.Description)
		}
		return nil
	},
}
