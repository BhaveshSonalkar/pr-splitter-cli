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
	maxFiles, err := promptForInt("Max files per partition?", 25, 1, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get max files per partition: %w", err)
	}

	// Get max partitions
	maxPartitions, err := promptForInt("Max total partitions?", 8, 1, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to get max partitions: %w", err)
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

	fmt.Println()
	fmt.Println("✅ Configuration complete!")
	fmt.Println()

	return &types.Config{
		MaxFilesPerPartition: maxFiles,
		MaxPartitions:        maxPartitions,
		BranchPrefix:         branchPrefix,
		Strategy:             strategy,
		TargetBranch:         targetBranch,
	}, nil
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
