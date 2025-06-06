package cli

import (
	"fmt"

	"pr-splitter-cli/internal/splitter"

	"github.com/spf13/cobra"
)

var breakCmd = &cobra.Command{
	Use:   "break [source-branch]",
	Short: "Break a large branch into smaller partitions",
	Long: `Analyze a large branch and break it into smaller, dependency-aware partitions.

The break command will:
1. Analyze git changes between your branch and main
2. Detect code dependencies using language plugins
3. Create logical partitions respecting dependencies
4. Generate branches for each partition
5. Validate the results

Examples:
  pr-split break feature/large-branch          Break the specified branch
  pr-split break feature/refactor-auth         Break authentication refactor
  pr-split break WIS-4721-to-break            Break ticket branch`,
	Args: cobra.ExactArgs(1),
	RunE: runBreak,
}

func runBreak(cmd *cobra.Command, args []string) error {
	sourceBranch := args[0]

	fmt.Printf("🚀 Breaking PR from branch: %s\n", sourceBranch)
	fmt.Println()

	// Initialize splitter
	s := splitter.New()

	// Run the splitting process
	result, err := s.Split(sourceBranch)
	if err != nil {
		return fmt.Errorf("failed to split PR: %w", err)
	}

	// Display success results
	fmt.Println()
	fmt.Printf("🎉 Successfully created %d partitions!\n", len(result.Partitions))
	fmt.Println()

	// Show partition summary
	for i, partition := range result.Partitions {
		fmt.Printf("📦 Partition %d: %s (%d files)\n",
			i+1, partition.Description, len(partition.Files))
	}

	fmt.Println()
	fmt.Println("📝 Next Steps:")
	if len(result.CreatedBranches) > 0 {
		fmt.Printf("1. Create GitHub PR: %s → main\n", result.CreatedBranches[0])
		if len(result.CreatedBranches) > 1 {
			fmt.Println("2. After merge, create subsequent PRs in order")
		}
	}

	return nil
}
