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
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectDeleteCmd)

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
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
		p, err := db.GetProjectByPrefix(conn, args[0])
		if err != nil {
			return fmt.Errorf("project %q not found", args[0])
		}
		if err := db.DeleteProject(conn, p.ID); err != nil {
			return err
		}
		fmt.Printf("Deleted project %s\n", p.Prefix)
		return nil
	},
}
