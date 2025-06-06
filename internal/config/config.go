package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"pr-splitter-cli/internal/types"
)

// GetFromUser prompts the user for configuration via CLI
func GetFromUser() (*types.Config, error) {
	fmt.Println("🔧 Configuration Setup:")
	fmt.Println()

	// Get max files per partition
	maxFiles, err := promptForInt("Max files per partition?", 15, 1, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get max files per partition: %w", err)
	}

	// Get max partitions
	maxPartitions, err := promptForInt("Max total partitions?", 8, 1, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to get max partitions: %w", err)
	}

	// Calculate and show capacity
	totalCapacity := maxFiles * maxPartitions
	fmt.Printf("💡 Total capacity: %d files (%d partitions × %d files)\n",
		totalCapacity, maxPartitions, maxFiles)

	// Get branch prefix
	branchPrefix, err := promptForString("Branch prefix?", "pr-split")
	if err != nil {
		return nil, fmt.Errorf("failed to get branch prefix: %w", err)
	}

	// Get strategy (for now, just default to dependency-first)
	strategy := "dependency-first"
	fmt.Printf("Strategy: %s (default)\n", strategy)

	// Get target branch
	targetBranch, err := promptForString("Target branch?", "main")
	if err != nil {
		return nil, fmt.Errorf("failed to get target branch: %w", err)
	}

	// Create config and validate
	config := &types.Config{
		MaxFilesPerPartition: maxFiles,
		MaxPartitions:        maxPartitions,
		BranchPrefix:         branchPrefix,
		Strategy:             strategy,
		TargetBranch:         targetBranch,
	}

	// Validate configuration consistency
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Configuration complete!")
	fmt.Println()

	return config, nil
}

// GetFromUserWithCapacityCheck prompts user with file count awareness
func GetFromUserWithCapacityCheck(estimatedFileCount int) (*types.Config, error) {
	fmt.Println("🔧 Configuration Setup:")
	fmt.Printf("📊 Estimated files to partition: %d\n", estimatedFileCount)
	fmt.Println()

	// Calculate recommended values
	recommendedPartitions := (estimatedFileCount / 15) + 1
	if recommendedPartitions < 8 {
		recommendedPartitions = 8
	}
	if recommendedPartitions > 50 {
		recommendedPartitions = 50
	}

	recommendedFilesPerPartition := 15
	if estimatedFileCount > 500 {
		recommendedFilesPerPartition = 25
	}

	fmt.Printf("💡 Recommendations for %d files:\n", estimatedFileCount)
	fmt.Printf("   • Max partitions: %d\n", recommendedPartitions)
	fmt.Printf("   • Max files per partition: %d\n", recommendedFilesPerPartition)
	fmt.Printf("   • Total capacity: %d files\n", recommendedPartitions*recommendedFilesPerPartition)
	fmt.Println()

	// Get max files per partition with recommendation
	maxFiles, err := promptForInt(
		fmt.Sprintf("Max files per partition? (recommended: %d)", recommendedFilesPerPartition),
		recommendedFilesPerPartition, 1, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get max files per partition: %w", err)
	}

	// Get max partitions with recommendation
	maxPartitions, err := promptForInt(
		fmt.Sprintf("Max total partitions? (recommended: %d)", recommendedPartitions),
		recommendedPartitions, 1, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to get max partitions: %w", err)
	}

	// Calculate and show capacity with warnings
	totalCapacity := maxFiles * maxPartitions
	fmt.Printf("💡 Selected capacity: %d files (%d partitions × %d files)\n",
		totalCapacity, maxPartitions, maxFiles)

	if totalCapacity < estimatedFileCount {
		fmt.Printf("⚠️  Warning: Selected capacity (%d) is less than estimated files (%d)\n",
			totalCapacity, estimatedFileCount)
		fmt.Println("   The tool will create catch-all partitions or larger partitions as needed.")
	} else if totalCapacity > estimatedFileCount*2 {
		fmt.Printf("💡 Info: Selected capacity (%d) is much larger than needed (%d)\n",
			totalCapacity, estimatedFileCount)
		fmt.Println("   You may end up with many small partitions.")
	}

	// Get branch prefix
	branchPrefix, err := promptForString("Branch prefix?", "pr-split")
	if err != nil {
		return nil, fmt.Errorf("failed to get branch prefix: %w", err)
	}

	// Get strategy (for now, just default to dependency-first)
	strategy := "dependency-first"
	fmt.Printf("Strategy: %s (default)\n", strategy)

	// Get target branch
	targetBranch, err := promptForString("Target branch?", "main")
	if err != nil {
		return nil, fmt.Errorf("failed to get target branch: %w", err)
	}

	// Create config and validate
	config := &types.Config{
		MaxFilesPerPartition: maxFiles,
		MaxPartitions:        maxPartitions,
		BranchPrefix:         branchPrefix,
		Strategy:             strategy,
		TargetBranch:         targetBranch,
	}

	// Validate configuration consistency
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Configuration complete!")
	fmt.Println()

	return config, nil
}

// promptForInt prompts user for an integer with validation
func promptForInt(prompt string, defaultValue, min, max int) (int, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s (default: %d): ", prompt, defaultValue)
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		// Use default if empty
		if input == "" {
			return defaultValue, nil
		}

		// Parse integer
		value, err := strconv.Atoi(input)
		if err != nil {
			fmt.Printf("❌ Please enter a valid number\n")
			continue
		}

		// Validate range
		if value < min || value > max {
			fmt.Printf("❌ Please enter a number between %d and %d\n", min, max)
			continue
		}

		return value, nil
	}
}

// promptForString prompts user for a string with validation
func promptForString(prompt, defaultValue string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s (default: %s): ", prompt, defaultValue)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		// Use default if empty
		if input == "" {
			return defaultValue, nil
		}

		// Basic validation - no spaces in branch prefix
		if strings.Contains(prompt, "prefix") && strings.ContainsAny(input, " \t") {
			fmt.Printf("❌ Branch prefix cannot contain spaces\n")
			continue
		}

		// Basic validation - no special characters in branch names
		if strings.Contains(prompt, "branch") && strings.ContainsAny(input, " ~^:?*[\\") {
			fmt.Printf("❌ Branch name contains invalid characters\n")
			continue
		}

		return input, nil
	}
}

// PromptForSCCDecision prompts user when SCC exceeds size limit
func PromptForSCCDecision(sccFiles []string, currentSize, limit int) (bool, error) {
	fmt.Printf("\n⚠️  Found circular dependency group with %d files (limit: %d)\n", currentSize, limit)
	fmt.Println("Files in circular group:")

	// Show first few files
	maxShow := 5
	for i, file := range sccFiles {
		if i >= maxShow {
			fmt.Printf("... and %d more files\n", len(sccFiles)-maxShow)
			break
		}
		fmt.Printf("  - %s\n", file)
	}

	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("[1] Proceed with extended partition")
	fmt.Println("[2] Show detailed circular dependency chain")
	fmt.Println("[3] Abort - let me break circular dependencies first")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Choose option (1-3): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("failed to read input: %w", err)
		}

		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			fmt.Printf("✅ Proceeding with partition of %d files\n\n", currentSize)
			return true, nil
		case "2":
			// Show detailed dependency chain (for now just show all files)
			fmt.Println("\nDetailed circular dependency files:")
			for _, file := range sccFiles {
				fmt.Printf("  - %s\n", file)
			}
			fmt.Println()
			// Continue prompting
		case "3":
			fmt.Println("❌ Aborting. Please break circular dependencies and try again.")
			return false, fmt.Errorf("user chose to abort due to circular dependencies")
		default:
			fmt.Println("❌ Please choose 1, 2, or 3")
		}
	}
}

// validateConfig validates configuration consistency and constraints
func validateConfig(cfg *types.Config) error {
	// Check for reasonable bounds
	if cfg.MaxFilesPerPartition <= 0 {
		return fmt.Errorf("max files per partition must be positive, got %d", cfg.MaxFilesPerPartition)
	}

	if cfg.MaxPartitions <= 0 {
		return fmt.Errorf("max partitions must be positive, got %d", cfg.MaxPartitions)
	}

	// Check for excessive values that might indicate user error
	if cfg.MaxFilesPerPartition > 1000 {
		return fmt.Errorf("max files per partition seems excessive: %d (consider values under 100)", cfg.MaxFilesPerPartition)
	}

	if cfg.MaxPartitions > 100 {
		return fmt.Errorf("max partitions seems excessive: %d (consider values under 20)", cfg.MaxPartitions)
	}

	// Validate branch prefix format
	if cfg.BranchPrefix == "" {
		return fmt.Errorf("branch prefix cannot be empty")
	}

	if len(cfg.BranchPrefix) > 50 {
		return fmt.Errorf("branch prefix too long: %d characters (max 50)", len(cfg.BranchPrefix))
	}

	// Validate target branch
	if cfg.TargetBranch == "" {
		return fmt.Errorf("target branch cannot be empty")
	}

	// Warn about potentially problematic combinations
	totalCapacity := cfg.MaxFilesPerPartition * cfg.MaxPartitions
	if totalCapacity < 10 {
		fmt.Printf("⚠️  Warning: Configuration allows max %d total files across all partitions\n", totalCapacity)
		fmt.Printf("   Consider increasing MaxFilesPerPartition or MaxPartitions for larger changes\n")
	}

	return nil
}
