package cmd

import (
	"fmt"
	"os"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/RunOnYourOwn/track/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "track",
	Short:   "Local project hub for AI-assisted development",
	Version: version.String(), // enables `track --version`
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		name := cmd.Name()
		if name == "help" || name == "completion" || name == "track" {
			return nil
		}
		_, err := db.Open()
		return err
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		_ = db.Close()
	},
}

var jsonOutput bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
}

func Execute() error {
	return rootCmd.Execute()
}

func exitErr(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+msg+"\n", args...)
	os.Exit(1)
}
