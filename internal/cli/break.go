package cli

import (
	"fmt"

	"pr-splitter-cli/internal/config"
	"pr-splitter-cli/internal/splitter"
	"pr-splitter-cli/internal/types"

	"github.com/spf13/cobra"
)

// Command flags
var (
	targetBranch   string
	branchPrefix   string
	maxSize        int
	maxDepth       int
	configFile     string
	nonInteractive bool
)

// breakCmd represents the break command
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
	RunE: runBreakCommand,
}

// runBreakCommand executes the break command
func runBreakCommand(cmd *cobra.Command, args []string) error {
	sourceBranch := args[0]

	fmt.Printf("🚀 Breaking PR from branch: %s\n", sourceBranch)
	fmt.Println()

	// Create configuration from flags or interactive prompts
	cfg, err := createConfiguration(sourceBranch)
	if err != nil {
		return fmt.Errorf("failed to create configuration: %w", err)
	}

	// Create splitter and run the process with configuration
	s := splitter.New()
	result, err := s.SplitWithConfig(sourceBranch, cfg)
	if err != nil {
		return fmt.Errorf("failed to split PR: %w", err)
	}

	// Display final results
	displayBreakResults(result)

	return nil
}

// createConfiguration creates config from flags or interactive prompts
func createConfiguration(sourceBranch string) (*types.Config, error) {
	// If config file is specified, try to load it first
	if configFile != "" {
		cfg, err := config.LoadFromFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
		// Override with any explicit flags
		overrideConfigFromFlags(cfg)
		return cfg, nil
	}

	// Check if multiple flags were provided (non-interactive mode)
	if hasMultipleFlags() {
		return createConfigFromFlags(), nil
	}

	// Interactive mode, but use smart analysis with preferred target if specified
	s := splitter.New()
	return s.GetSmartConfiguration(sourceBranch, targetBranch)
}

// hasMultipleFlags checks if enough flags were set to warrant non-interactive mode
func hasMultipleFlags() bool {
	flagCount := 0
	if targetBranch != "" {
		flagCount++
	}
	if branchPrefix != "" {
		flagCount++
	}
	if maxSize > 0 {
		flagCount++
	}
	if maxDepth > 0 {
		flagCount++
	}

	// Non-interactive flag always enables non-interactive mode
	return nonInteractive || flagCount >= 2
}

// createConfigFromFlags creates configuration from command-line flags
func createConfigFromFlags() *types.Config {
	cfg := &types.Config{
		MaxFilesPerPartition: config.ConfigDefaults.MaxFilesPerPartition,
		MaxPartitions:        config.ConfigDefaults.MaxPartitions,
		BranchPrefix:         config.ConfigDefaults.BranchPrefix,
		Strategy:             config.ConfigDefaults.Strategy,
		TargetBranch:         config.ConfigDefaults.TargetBranch,
	}

	// Override with provided flags
	overrideConfigFromFlags(cfg)

	return cfg
}

// overrideConfigFromFlags applies command-line flags to configuration
func overrideConfigFromFlags(cfg *types.Config) {
	if targetBranch != "" {
		cfg.TargetBranch = targetBranch
	}
	if branchPrefix != "" {
		cfg.BranchPrefix = branchPrefix
	}
	if maxSize > 0 {
		cfg.MaxFilesPerPartition = maxSize
	}
	// Calculate max partitions based on max depth if provided
	if maxDepth > 0 {
		cfg.MaxPartitions = maxDepth * 2 // Simple heuristic
	}
}

// displayBreakResults shows the final results to the user
func displayBreakResults(result *types.SplitResult) {
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
		fmt.Printf("1. Create GitHub PR: %s → %s\n", result.CreatedBranches[0], result.TargetBranch)
		if len(result.CreatedBranches) > 1 {
			fmt.Println("2. After merge, create subsequent PRs in dependency order")
		}
		fmt.Printf("3. Use 'pr-split rollback %s' to cleanup when done\n", result.Config.BranchPrefix)
	}
}

func init() {
	// Add flags to the break command
	breakCmd.Flags().StringVarP(&targetBranch, "target", "t", "", "Target branch (default \"main\")")
	breakCmd.Flags().StringVarP(&branchPrefix, "prefix", "p", "", "Branch prefix (default \"pr-split\")")
	breakCmd.Flags().IntVarP(&maxSize, "max-size", "s", 0, "Maximum files per partition (default 15)")
	breakCmd.Flags().IntVarP(&maxDepth, "max-depth", "d", 0, "Maximum dependency depth (default 10)")
	breakCmd.Flags().StringVarP(&configFile, "config", "c", "", "Config file path")
	breakCmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Run without prompts using defaults")
}
