package splitter

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"pr-splitter-cli/internal/config"
	"pr-splitter-cli/internal/git"
	"pr-splitter-cli/internal/partition"
	"pr-splitter-cli/internal/plugin"
	"pr-splitter-cli/internal/types"
	"pr-splitter-cli/internal/validation"
)

// Splitter orchestrates the entire PR splitting process
type Splitter struct {
	gitClient     *git.Client
	pluginManager *plugin.Manager
	partitioner   *partition.Partitioner
	validator     *validation.Validator
}

// New creates a new Splitter instance
func New() *Splitter {
	return &Splitter{
		gitClient:     git.NewClient(),
		pluginManager: plugin.NewManager(),
		partitioner:   partition.NewPartitioner(),
		validator:     validation.NewValidator(),
	}
}

// Split performs the complete PR splitting process
func (s *Splitter) Split(sourceBranch string) (*types.SplitResult, error) {
	// Step 1: Get configuration from user
	fmt.Println("🔧 Getting configuration...")
	cfg, err := config.GetFromUser()
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Step 2: Analyze git changes
	fmt.Printf("🔍 Analyzing git changes from %s to %s...\n", sourceBranch, cfg.TargetBranch)
	changes, err := s.gitClient.GetChanges(sourceBranch, cfg.TargetBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze git changes: %w", err)
	}

	if len(changes) == 0 {
		return nil, fmt.Errorf("no changes found between %s and %s", sourceBranch, cfg.TargetBranch)
	}

	fmt.Printf("📊 Found %d changed files\n", len(changes))

	// Step 3: Analyze dependencies with plugins
	fmt.Println("🧠 Analyzing dependencies with plugins...")
	dependencies, err := s.pluginManager.AnalyzeDependencies(changes)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze dependencies: %w", err)
	}

	fmt.Printf("🔗 Found %d dependencies\n", len(dependencies))

	// Step 4: Create partition plan
	fmt.Println("📦 Creating partition plan...")
	plan, err := s.partitioner.CreatePlan(changes, dependencies, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create partition plan: %w", err)
	}

	fmt.Printf("📋 Created %d partitions\n", len(plan.Partitions))

	// Display partition summary for user review
	err = s.displayPartitionSummary(plan)
	if err != nil {
		return nil, fmt.Errorf("failed to display partition summary: %w", err)
	}

	// Ask user for approval
	approved, err := s.promptForApproval()
	if err != nil {
		return nil, fmt.Errorf("failed to get user approval: %w", err)
	}

	if !approved {
		return nil, fmt.Errorf("user cancelled the operation")
	}

	// Step 5: Pre-execution validation
	fmt.Println("✅ Validating partition plan...")
	preValidation, err := s.validator.ValidatePlan(plan, changes)
	if err != nil {
		return nil, fmt.Errorf("pre-validation failed: %w", err)
	}

	if !s.validator.AllPassed(preValidation) {
		s.displayValidationResults(preValidation)
		return nil, fmt.Errorf("partition plan validation failed")
	}

	fmt.Println("✅ Plan validation passed")

	// Step 6: Create branches
	fmt.Println("🌿 Creating branches...")
	branches, err := s.gitClient.CreateBranches(plan, cfg, sourceBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to create branches: %w", err)
	}

	fmt.Printf("✅ Created %d branches\n", len(branches))

	// Step 7: Post-creation validation
	fmt.Println("🔍 Post-creation validation...")
	postValidation, err := s.validator.ValidateBranches(branches, changes, sourceBranch, cfg.TargetBranch)
	if err != nil {
		return nil, fmt.Errorf("post-validation failed: %w", err)
	}

	if !s.validator.AllPassed(postValidation) {
		s.displayValidationResults(postValidation)
		return nil, fmt.Errorf("branch validation failed")
	}

	fmt.Println("✅ Post-creation validation passed")

	// Step 8: Build and return result
	result := &types.SplitResult{
		SourceBranch:      sourceBranch,
		TargetBranch:      cfg.TargetBranch,
		Partitions:        plan.Partitions,
		CreatedBranches:   branches,
		ValidationResults: append(preValidation, postValidation...),
		Config:            *cfg,
	}

	return result, nil
}

// displayPartitionSummary shows the partition plan to the user
func (s *Splitter) displayPartitionSummary(plan *types.PartitionPlan) error {
	fmt.Println()
	fmt.Println("📦 Partition Plan:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for i, partition := range plan.Partitions {
		fmt.Printf("Partition %d: %s (%d files)\n", i+1, partition.Description, len(partition.Files))

		// Show first few files as preview
		maxShow := 3
		for j, file := range partition.Files {
			if j >= maxShow {
				fmt.Printf("  ... and %d more files\n", len(partition.Files)-maxShow)
				break
			}
			fmt.Printf("  - %s (%s)\n", file.Path, file.ChangeType)
		}

		// Show dependencies
		if len(partition.Dependencies) > 0 {
			fmt.Printf("  Dependencies: Partition %v\n", partition.Dependencies)
		} else {
			fmt.Printf("  Dependencies: None (base partition)\n")
		}

		fmt.Println()
	}

	fmt.Printf("Total: %d files across %d partitions\n", plan.Metadata.TotalFiles, plan.Metadata.TotalPartitions)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	return nil
}

// promptForApproval asks user to approve the partition plan
func (s *Splitter) promptForApproval() (bool, error) {
	fmt.Print("Proceed with this partition plan? [Y/n]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))

	switch input {
	case "n", "no":
		return false, nil
	case "y", "yes", "":
		return true, nil
	default:
		return true, nil // Default to yes for any other input
	}
}

// displayValidationResults shows validation results to the user
func (s *Splitter) displayValidationResults(results []types.ValidationResult) {
	fmt.Println("\n❌ Validation Results:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for _, result := range results {
		var status string
		switch result.Status {
		case types.ValidationStatusPass:
			status = "✅ PASS"
		case types.ValidationStatusWarn:
			status = "⚠️  WARN"
		case types.ValidationStatusFail:
			status = "❌ FAIL"
		}

		fmt.Printf("%s %s: %s\n", status, result.Type, result.Message)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
