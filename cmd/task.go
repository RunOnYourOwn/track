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
	taskCmd.AddCommand(taskDeleteCmd)
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
	taskCreateCmd.Flags().String("due", "", "Due date (YYYY-MM-DD)")
	_ = taskCreateCmd.MarkFlagRequired("project")
	_ = taskCreateCmd.MarkFlagRequired("title")

	taskMoveCmd.Flags().String("status", "", "Target status")
	_ = taskMoveCmd.MarkFlagRequired("status")

	taskDoneCmd.Flags().Float64("actual-hours", 0, "Actual hours spent")

	taskLinkCmd.Flags().String("blocks", "", "Task ID that this task blocks")
	taskLinkCmd.Flags().String("type", "blocks", "Dependency type: blocks, soft, informational")
	taskLinkCmd.Flags().String("reason", "", "Reason for dependency")

	taskUnlinkCmd.Flags().String("blocks", "", "Task ID to unlink")

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
		conn, _ := db.Open()

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
		conn, _ := db.Open()

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
		conn, _ := db.Open()
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
		conn, _ := db.Open()
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
		conn, _ := db.Open()
		hours, _ := cmd.Flags().GetFloat64("actual-hours")

		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		if err := db.CompleteTask(conn, taskID, hours); err != nil {
			return err
		}
		fmt.Println("Done ✓")
		return nil
	},
}

var taskDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
		_ = conn
		taskID, err := resolveID(args[0])
		if err != nil {
			return err
		}
		return db.DeleteTask(conn, taskID)
	},
}

var taskLinkCmd = &cobra.Command{
	Use:   "link [id]",
	Short: "Create dependency between tasks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
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
		conn, _ := db.Open()
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
		conn, _ := db.Open()
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
