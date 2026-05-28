package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/RunOnYourOwn/track/internal/ado"
	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(adoCmd)
	adoCmd.AddCommand(adoPullCmd)
	adoCmd.AddCommand(adoPushCmd)
	adoCmd.AddCommand(adoStatusCmd)
	adoCmd.AddCommand(adoConfigCmd)

	adoPullCmd.Flags().String("team", "", "Only pull for this track project prefix")
	adoPullCmd.Flags().Bool("dry-run", false, "Show what would change without writing")

	adoPushCmd.Flags().String("team", "", "Only push for this track project prefix")
	adoPushCmd.Flags().Bool("dry-run", false, "Show what would push without writing")
}

var adoCmd = &cobra.Command{
	Use:   "ado",
	Short: "Azure DevOps sync",
}

var adoPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull work items from ADO into track",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ado.LoadConfig()
		if err != nil {
			return err
		}

		team, _ := cmd.Flags().GetString("team")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		conn, err := db.Open()
		if err != nil {
			return err
		}

		if dryRun {
			fmt.Println("=== DRY RUN ===")
		}

		stats, err := ado.Pull(conn, cfg, team, dryRun)
		if err != nil {
			return err
		}

		fmt.Printf("\nPull complete: %d created, %d updated, %d unchanged, %d skipped, %d failed\n",
			stats.Created, stats.Updated, stats.Unchanged, stats.Skipped, stats.Failed)
		if stats.Failed > 0 {
			return fmt.Errorf("%d item(s) failed to sync — see warnings above", stats.Failed)
		}
		return nil
	},
}

var adoPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push local status changes to ADO",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ado.LoadConfig()
		if err != nil {
			return err
		}

		team, _ := cmd.Flags().GetString("team")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		conn, err := db.Open()
		if err != nil {
			return err
		}

		if dryRun {
			fmt.Println("=== DRY RUN ===")
		}

		stats, err := ado.Push(conn, cfg, team, dryRun)
		if err != nil {
			return err
		}

		fmt.Printf("\nPush complete: %d pushed, %d skipped, %d failed\n",
			stats.Pushed, stats.Skipped, stats.Failed)
		return nil
	},
}

var adoStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show ADO sync status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ado.LoadConfig()
		if err != nil {
			return err
		}

		conn, err := db.Open()
		if err != nil {
			return err
		}

		fmt.Printf("Org: %s\n", cfg.Org)
		fmt.Printf("PAT env: %s\n", cfg.PatEnv)
		fmt.Println()

		for _, sync := range cfg.Syncs {
			fmt.Printf("Sync: %s/%s → project %s\n", sync.Project, sync.Team, sync.TrackProject)

			p, err := db.GetProjectByPrefix(conn, sync.TrackProject)
			if err != nil {
				fmt.Printf("  (project not yet created)\n")
				continue
			}

			tasks, err := db.ListTasks(conn, db.ListTaskOpts{ProjectID: p.ID})
			if err != nil {
				fmt.Printf("  (error listing tasks: %v)\n", err)
				continue
			}

			adoCount := 0
			for _, t := range tasks {
				if t.SourceType == "ado" {
					adoCount++
				}
			}
			fmt.Printf("  ADO items: %d\n", adoCount)
		}

		return nil
	},
}

var adoConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Set up ADO sync configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if config exists
		existing, _ := ado.LoadConfig()
		if existing != nil {
			fmt.Println("Current config:")
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(existing)
			fmt.Printf("\nConfig file: %s\n", ado.ConfigPath())
			return nil
		}

		// Create a default config
		cfg := &ado.Config{
			Org:    "",
			PatEnv: "TRACK_ADO_PAT",
			Email:  "",
			Syncs: []ado.SyncConfig{
				{
					Project:      "",
					Team:         "",
					TrackProject: "",
				},
			},
		}

		if err := ado.SaveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("Created config at %s\n", ado.ConfigPath())
		fmt.Println("Edit it to match your setup, then run 'track ado pull'")
		return nil
	},
}
