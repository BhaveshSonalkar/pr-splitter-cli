package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "pr-split",
	Short: "Intelligently break large PRs into smaller, reviewable partitions",
	Long: `PR Splitter analyzes your code dependencies and creates logical, 
dependency-aware partitions of your large pull request.

Examples:
  pr-split break feature/large-branch    Break a branch into partitions
  pr-split --help                        Show help information`,
	Version: "1.0.0",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Add child commands here
	rootCmd.AddCommand(breakCmd)

	// Global flags can be added here if needed
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pr-splitter.yaml)")
}
