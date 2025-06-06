package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"pr-splitter-cli/internal/types"
)

// ConfigDefaults holds default configuration values
var ConfigDefaults = struct {
	MaxFilesPerPartition int
	MaxPartitions        int
	BranchPrefix         string
	Strategy             string
	TargetBranch         string
}{
	MaxFilesPerPartition: 15,
	MaxPartitions:        8,
	BranchPrefix:         "pr-split",
	Strategy:             "dependency-first",
	TargetBranch:         "main",
}

// GetFromUser prompts the user for configuration via CLI
func GetFromUser() (*types.Config, error) {
	fmt.Println("🔧 Configuration Setup:")
	fmt.Println()

	prompter := NewPrompter()

	maxFiles, err := prompter.PromptInt("Max files per partition?", ConfigDefaults.MaxFilesPerPartition, 1, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get max files per partition: %w", err)
	}

	maxPartitions, err := prompter.PromptInt("Max total partitions?", ConfigDefaults.MaxPartitions, 1, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to get max partitions: %w", err)
	}

	prompter.ShowCapacity(maxFiles, maxPartitions)

	branchPrefix, err := prompter.PromptString("Branch prefix?", ConfigDefaults.BranchPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to get branch prefix: %w", err)
	}

	targetBranch, err := prompter.PromptString("Target branch?", ConfigDefaults.TargetBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get target branch: %w", err)
	}

	config := &types.Config{
		MaxFilesPerPartition: maxFiles,
		MaxPartitions:        maxPartitions,
		BranchPrefix:         branchPrefix,
		Strategy:             ConfigDefaults.Strategy,
		TargetBranch:         targetBranch,
	}

	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Println("✅ Configuration complete!")
	fmt.Println()

	return config, nil
}

// GetFromUserWithCapacityCheck prompts user with file count awareness
func GetFromUserWithCapacityCheck(estimatedFileCount int) (*types.Config, error) {
	fmt.Println("🔧 Configuration Setup:")
	fmt.Printf("📊 Estimated files to partition: %d\n", estimatedFileCount)
	fmt.Println()

	prompter := NewPrompter()
	recommendations := CalculateRecommendations(estimatedFileCount)

	prompter.ShowRecommendations(estimatedFileCount, recommendations)

	maxFiles, err := prompter.PromptIntWithRecommendation("Max files per partition?", recommendations.MaxFilesPerPartition, 1, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get max files per partition: %w", err)
	}

	maxPartitions, err := prompter.PromptIntWithRecommendation("Max total partitions?", recommendations.MaxPartitions, 1, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to get max partitions: %w", err)
	}

	prompter.ShowCapacityAnalysis(maxFiles, maxPartitions, estimatedFileCount)

	branchPrefix, err := prompter.PromptString("Branch prefix?", ConfigDefaults.BranchPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to get branch prefix: %w", err)
	}

	targetBranch, err := prompter.PromptString("Target branch?", ConfigDefaults.TargetBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get target branch: %w", err)
	}

	config := &types.Config{
		MaxFilesPerPartition: maxFiles,
		MaxPartitions:        maxPartitions,
		BranchPrefix:         branchPrefix,
		Strategy:             ConfigDefaults.Strategy,
		TargetBranch:         targetBranch,
	}

	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Println("✅ Configuration complete!")
	fmt.Println()

	return config, nil
}

// Recommendations holds recommended configuration values
type Recommendations struct {
	MaxFilesPerPartition int
	MaxPartitions        int
	TotalCapacity        int
}

// CalculateRecommendations calculates recommended values based on file count
func CalculateRecommendations(estimatedFileCount int) Recommendations {
	var maxPartitions int
	var maxFilesPerPartition int

	// Calculate recommended partitions
	maxPartitions = (estimatedFileCount / ConfigDefaults.MaxFilesPerPartition) + 1
	if maxPartitions < ConfigDefaults.MaxPartitions {
		maxPartitions = ConfigDefaults.MaxPartitions
	}
	if maxPartitions > 50 {
		maxPartitions = 50
	}

	// Calculate recommended files per partition
	maxFilesPerPartition = ConfigDefaults.MaxFilesPerPartition
	if estimatedFileCount > 500 {
		maxFilesPerPartition = 25
	}

	return Recommendations{
		MaxFilesPerPartition: maxFilesPerPartition,
		MaxPartitions:        maxPartitions,
		TotalCapacity:        maxPartitions * maxFilesPerPartition,
	}
}

// ValidateConfig validates configuration consistency and constraints
func ValidateConfig(cfg *types.Config) error {
	if cfg.MaxFilesPerPartition <= 0 {
		return fmt.Errorf("max files per partition must be positive, got %d", cfg.MaxFilesPerPartition)
	}

	if cfg.MaxPartitions <= 0 {
		return fmt.Errorf("max partitions must be positive, got %d", cfg.MaxPartitions)
	}

	if cfg.MaxFilesPerPartition > 1000 {
		return fmt.Errorf("max files per partition seems excessive: %d (consider values under 100)", cfg.MaxFilesPerPartition)
	}

	if cfg.MaxPartitions > 100 {
		return fmt.Errorf("max partitions seems excessive: %d (consider values under 20)", cfg.MaxPartitions)
	}

	if cfg.BranchPrefix == "" {
		return fmt.Errorf("branch prefix cannot be empty")
	}

	if len(cfg.BranchPrefix) > 50 {
		return fmt.Errorf("branch prefix too long: %d characters (max 50)", len(cfg.BranchPrefix))
	}

	if cfg.TargetBranch == "" {
		return fmt.Errorf("target branch cannot be empty")
	}

	totalCapacity := cfg.MaxFilesPerPartition * cfg.MaxPartitions
	if totalCapacity < 10 {
		fmt.Printf("⚠️  Warning: Configuration allows max %d total files across all partitions\n", totalCapacity)
		fmt.Printf("   Consider increasing MaxFilesPerPartition or MaxPartitions for larger changes\n")
	}

	return nil
}

// Prompter handles user input prompts
type Prompter struct {
	reader *bufio.Reader
}

// NewPrompter creates a new prompter
func NewPrompter() *Prompter {
	return &Prompter{
		reader: bufio.NewReader(os.Stdin),
	}
}

// PromptInt prompts user for an integer with validation
func (p *Prompter) PromptInt(prompt string, defaultValue, min, max int) (int, error) {
	for {
		fmt.Printf("%s (default: %d): ", prompt, defaultValue)
		input, err := p.reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			return defaultValue, nil
		}

		value, err := strconv.Atoi(input)
		if err != nil {
			fmt.Printf("❌ Please enter a valid number\n")
			continue
		}

		if value < min || value > max {
			fmt.Printf("❌ Please enter a number between %d and %d\n", min, max)
			continue
		}

		return value, nil
	}
}

// PromptIntWithRecommendation prompts for int with a recommendation
func (p *Prompter) PromptIntWithRecommendation(prompt string, recommended, min, max int) (int, error) {
	enhancedPrompt := fmt.Sprintf("%s (recommended: %d)", prompt, recommended)
	return p.PromptInt(enhancedPrompt, recommended, min, max)
}

// PromptString prompts user for a string with validation
func (p *Prompter) PromptString(prompt, defaultValue string) (string, error) {
	for {
		fmt.Printf("%s (default: %s): ", prompt, defaultValue)
		input, err := p.reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			return defaultValue, nil
		}

		if err := p.validateStringInput(input, prompt); err != nil {
			fmt.Printf("❌ %v\n", err)
			continue
		}

		return input, nil
	}
}

// validateStringInput validates string input based on context
func (p *Prompter) validateStringInput(input, prompt string) error {
	if strings.Contains(prompt, "prefix") && strings.ContainsAny(input, " \t") {
		return fmt.Errorf("branch prefix cannot contain spaces")
	}

	if strings.Contains(prompt, "branch") && strings.ContainsAny(input, " ~^:?*[\\") {
		return fmt.Errorf("branch name contains invalid characters")
	}

	return nil
}

// ShowRecommendations displays recommendations to the user
func (p *Prompter) ShowRecommendations(fileCount int, rec Recommendations) {
	fmt.Printf("💡 Recommendations for %d files:\n", fileCount)
	fmt.Printf("   • Max partitions: %d\n", rec.MaxPartitions)
	fmt.Printf("   • Max files per partition: %d\n", rec.MaxFilesPerPartition)
	fmt.Printf("   • Total capacity: %d files\n", rec.TotalCapacity)
	fmt.Println()
}

// ShowCapacity displays capacity information
func (p *Prompter) ShowCapacity(maxFiles, maxPartitions int) {
	totalCapacity := maxFiles * maxPartitions
	fmt.Printf("💡 Total capacity: %d files (%d partitions × %d files)\n",
		totalCapacity, maxPartitions, maxFiles)
}

// ShowCapacityAnalysis shows capacity analysis with warnings
func (p *Prompter) ShowCapacityAnalysis(maxFiles, maxPartitions, estimatedFiles int) {
	totalCapacity := maxFiles * maxPartitions
	fmt.Printf("💡 Selected capacity: %d files (%d partitions × %d files)\n",
		totalCapacity, maxPartitions, maxFiles)

	if totalCapacity < estimatedFiles {
		fmt.Printf("⚠️  Warning: Selected capacity (%d) is less than estimated files (%d)\n",
			totalCapacity, estimatedFiles)
		fmt.Println("   The tool will create catch-all partitions or larger partitions as needed.")
	} else if totalCapacity > estimatedFiles*2 {
		fmt.Printf("💡 Info: Selected capacity (%d) is much larger than needed (%d)\n",
			totalCapacity, estimatedFiles)
		fmt.Println("   You may end up with many small partitions.")
	}
}

// PromptForSCCDecision prompts user when SCC exceeds size limit
func PromptForSCCDecision(sccFiles []string, currentSize, limit int) (bool, error) {
	fmt.Printf("\n⚠️  Found circular dependency group with %d files (limit: %d)\n", currentSize, limit)
	fmt.Println("Files in circular group:")

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

	prompter := NewPrompter()

	for {
		fmt.Print("Choose option (1-3): ")
		input, err := prompter.reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("failed to read input: %w", err)
		}

		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			fmt.Printf("✅ Proceeding with partition of %d files\n\n", currentSize)
			return true, nil
		case "2":
			fmt.Println("\nDetailed circular dependency files:")
			for _, file := range sccFiles {
				fmt.Printf("  - %s\n", file)
			}
			fmt.Println()
		case "3":
			fmt.Println("❌ Aborting. Please break circular dependencies and try again.")
			return false, fmt.Errorf("user chose to abort due to circular dependencies")
		default:
			fmt.Println("❌ Please choose 1, 2, or 3")
		}
	}
}
