package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectDeleteCmd)

	projectDeleteCmd.Flags().Bool("yes", false, "Skip the interactive confirmation prompt (for automation)")

	projectCreateCmd.Flags().String("prefix", "", "Project prefix (3-4 uppercase letters)")
	projectCreateCmd.Flags().String("name", "", "Project name")
	projectCreateCmd.Flags().String("phase", "", "Current phase")
	projectCreateCmd.Flags().String("phase-type", "build", "Phase type: discovery, design, build, stabilize, maintain")
	projectCreateCmd.Flags().String("external-id", "", "External project or resource ID")
	projectCreateCmd.Flags().String("metadata", "{}", "JSON metadata")
	projectCreateCmd.Flags().Int("wip-limit", 3, "WIP limit")
	_ = projectCreateCmd.MarkFlagRequired("prefix")
	_ = projectCreateCmd.MarkFlagRequired("name")
}

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
		projects, err := db.ListProjects(conn)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(projects)
		}

		if len(projects) == 0 {
			fmt.Println("No projects. Use 'track project create' to add one.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PREFIX\tNAME\tPHASE\tWIP")
		for _, p := range projects {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", p.Prefix, p.Name, p.Phase, p.WIPLimit)
		}
		return w.Flush()
	},
}

var projectCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new project",
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, _ := cmd.Flags().GetString("prefix")
		name, _ := cmd.Flags().GetString("name")
		phase, _ := cmd.Flags().GetString("phase")
		phaseType, _ := cmd.Flags().GetString("phase-type")
		externalID, _ := cmd.Flags().GetString("external-id")
		metadata, _ := cmd.Flags().GetString("metadata")
		wipLimit, _ := cmd.Flags().GetInt("wip-limit")

		conn, _ := db.Open()
		p, err := db.CreateProject(conn, prefix, name, phase, phaseType, externalID, metadata, wipLimit)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(p)
		}

		fmt.Printf("Created project %s (%s) — ID: %s\n", p.Prefix, p.Name, p.ID)
		return nil
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete [prefix]",
	Short: "Permanently delete a project and ALL its data",
	Long: "Permanently delete a project and ALL its data — every task, sprint, session, " +
		"decision, learning, and blocker. This cannot be undone. You will be asked to " +
		"retype the prefix to confirm unless --yes is given.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
		p, err := db.GetProjectByPrefix(conn, args[0])
		if err != nil {
			return fmt.Errorf("project %q not found", args[0])
		}

		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			tasks, _ := db.ListTasks(conn, db.ListTaskOpts{ProjectID: p.ID})
			fmt.Printf("This will PERMANENTLY delete project %s (%s) and all its data", p.Prefix, p.Name)
			if n := len(tasks); n > 0 {
				fmt.Printf(", including %d task(s)", n)
			}
			fmt.Println(". This cannot be undone.")
			fmt.Printf("Type %q to confirm: ", p.Prefix)
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if strings.TrimSpace(line) != p.Prefix {
				return fmt.Errorf("aborted: confirmation did not match %q", p.Prefix)
			}
		}

		if err := db.DeleteProject(conn, p.ID); err != nil {
			return err
		}
		fmt.Printf("Deleted project %s\n", p.Prefix)
		return nil
	},
}
