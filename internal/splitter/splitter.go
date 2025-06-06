package splitter

import (
	"fmt"

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

// Split performs the complete PR splitting process with smart configuration
func (s *Splitter) Split(sourceBranch string) (*types.SplitResult, error) {
	// Get configuration with smart recommendations
	fmt.Println("🔍 Analyzing repository for configuration recommendations...")
	cfg, err := s.getSmartConfiguration(sourceBranch, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	return s.SplitWithConfig(sourceBranch, cfg)
}

// SplitWithConfig performs the splitting process with provided configuration
func (s *Splitter) SplitWithConfig(sourceBranch string, cfg *types.Config) (*types.SplitResult, error) {
	return s.executeWorkflow(sourceBranch, cfg)
}

// GetSmartConfiguration exposes smart configuration for CLI usage
func (s *Splitter) GetSmartConfiguration(sourceBranch, preferredTarget string) (*types.Config, error) {
	return s.getSmartConfiguration(sourceBranch, preferredTarget)
}

// getSmartConfiguration gets configuration with file count awareness
func (s *Splitter) getSmartConfiguration(sourceBranch, preferredTarget string) (*types.Config, error) {
	// Determine target branch for analysis
	targetBranch := preferredTarget
	if targetBranch == "" {
		targetBranch = config.ConfigDefaults.TargetBranch
	}

	// Try quick analysis for recommendations using the correct target branch
	quickChanges, err := s.gitClient.GetChanges(sourceBranch, targetBranch)
	if err != nil {
		fmt.Println("⚠️  Quick analysis failed, using basic configuration...")
		return config.GetFromUser()
	}

	changedFileCount := s.countChangedFiles(quickChanges)
	return config.GetFromUserWithCapacityCheck(changedFileCount)
}

// executeWorkflow runs the main splitting workflow
func (s *Splitter) executeWorkflow(sourceBranch string, cfg *types.Config) (*types.SplitResult, error) {
	// Step 1: Analyze changes
	changes, err := s.analyzeChanges(sourceBranch, cfg.TargetBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze changes: %w", err)
	}

	// Step 2: Analyze dependencies
	dependencies, err := s.analyzeDependencies(changes)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze dependencies: %w", err)
	}

	// Step 3: Create partition plan
	plan, err := s.createPartitionPlan(changes, dependencies, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create partition plan: %w", err)
	}

	// Step 4: Get user approval
	if err := s.getApprovalForPlan(plan); err != nil {
		return nil, err
	}

	// Step 5: Validate and execute
	return s.validateAndExecute(plan, changes, cfg, sourceBranch)
}

// analyzeChanges gets git changes with validation
func (s *Splitter) analyzeChanges(sourceBranch, targetBranch string) ([]types.FileChange, error) {
	fmt.Printf("🔍 Analyzing git changes from %s to %s...\n", sourceBranch, targetBranch)

	changes, err := s.gitClient.GetChanges(sourceBranch, targetBranch)
	if err != nil {
		return nil, err
	}

	if len(changes) == 0 {
		return nil, fmt.Errorf("no changes found between %s and %s", sourceBranch, targetBranch)
	}

	fmt.Printf("📊 Found %d changed files\n", s.countChangedFiles(changes))
	return changes, nil
}

// analyzeDependencies runs plugin analysis on files
func (s *Splitter) analyzeDependencies(changes []types.FileChange) ([]types.Dependency, error) {
	fmt.Println("🧠 Analyzing dependencies with plugins...")

	dependencies, err := s.pluginManager.AnalyzeDependencies(changes)
	if err != nil {
		return nil, err
	}

	fmt.Printf("🔗 Found %d dependencies\n", len(dependencies))
	return dependencies, nil
}

// createPartitionPlan creates the partitioning plan
func (s *Splitter) createPartitionPlan(changes []types.FileChange, dependencies []types.Dependency, cfg *types.Config) (*types.PartitionPlan, error) {
	fmt.Println("📦 Creating partition plan...")

	plan, err := s.partitioner.CreatePlan(changes, dependencies, cfg)
	if err != nil {
		return nil, err
	}

	fmt.Printf("📋 Created %d partitions\n", len(plan.Partitions))
	s.displayPartitionSummary(plan)
	s.displayExhaustivenessSummary(changes, plan)

	return plan, nil
}

// getApprovalForPlan displays plan and gets user approval
func (s *Splitter) getApprovalForPlan(plan *types.PartitionPlan) error {
	s.displayDetailedPlan(plan)

	approved, err := s.promptForApproval()
	if err != nil {
		return fmt.Errorf("failed to get user approval: %w", err)
	}

	if !approved {
		return fmt.Errorf("user cancelled the operation")
	}

	return nil
}

// validateAndExecute validates the plan and creates branches
func (s *Splitter) validateAndExecute(plan *types.PartitionPlan, changes []types.FileChange, cfg *types.Config, sourceBranch string) (*types.SplitResult, error) {
	// Pre-validation
	fmt.Println("✅ Validating partition plan...")
	preValidation, err := s.validator.ValidatePlan(plan, changes)
	if err != nil {
		return nil, fmt.Errorf("pre-validation failed: %w", err)
	}

	if !s.validator.AllPassed(preValidation) {
		s.displayValidationResults(preValidation)
		return nil, fmt.Errorf("partition plan validation failed")
	}

	// Create branches
	fmt.Println("🌿 Creating branches...")
	branches, err := s.gitClient.CreateBranches(plan, cfg, sourceBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to create branches: %w", err)
	}

	// Post-validation
	fmt.Println("🔍 Post-creation validation...")
	postValidation, err := s.validator.ValidateBranches(branches, changes, sourceBranch, cfg.TargetBranch)
	if err != nil {
		return nil, fmt.Errorf("post-validation failed: %w", err)
	}

	if !s.validator.AllPassed(postValidation) {
		s.displayValidationResults(postValidation)
		return nil, fmt.Errorf("branch validation failed")
	}

	// Build result
	result := &types.SplitResult{
		SourceBranch:      sourceBranch,
		TargetBranch:      cfg.TargetBranch,
		Partitions:        plan.Partitions,
		CreatedBranches:   branches,
		ValidationResults: append(preValidation, postValidation...),
		Config:            *cfg,
	}

	s.displaySuccessSummary(result, plan)
	return result, nil
}

// Utility and display methods

func (s *Splitter) countChangedFiles(changes []types.FileChange) int {
	count := 0
	for _, change := range changes {
		if change.IsChanged {
			count++
		}
	}
	return count
}

func (s *Splitter) displayPartitionSummary(plan *types.PartitionPlan) {
	fmt.Printf("📊 Partition Summary: %d partitions covering %d files\n",
		len(plan.Partitions), plan.Metadata.TotalFiles)
}

func (s *Splitter) displayDetailedPlan(plan *types.PartitionPlan) {
	fmt.Println()
	fmt.Println("📦 Detailed Partition Plan:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for i, partition := range plan.Partitions {
		fmt.Printf("Partition %d: %s (%d files)\n", i+1, partition.Description, len(partition.Files))

		// Show preview of files
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
}

func (s *Splitter) displayExhaustivenessSummary(changes []types.FileChange, plan *types.PartitionPlan) {
	totalFiles := s.countChangedFiles(changes)
	partitionFileCount := 0

	for _, partition := range plan.Partitions {
		partitionFileCount += len(partition.Files)
	}

	fmt.Println("📊 Coverage Summary:")
	fmt.Printf("   • Total changed files: %d\n", totalFiles)
	fmt.Printf("   • Files in partitions: %d\n", partitionFileCount)

	if partitionFileCount == totalFiles {
		fmt.Println("   ✅ All files included (100% coverage)")
	} else {
		fmt.Printf("   ⚠️  Coverage gap: %d files\n", totalFiles-partitionFileCount)
	}
	fmt.Println()
}

func (s *Splitter) promptForApproval() (bool, error) {
	fmt.Print("Proceed with this partition plan? [Y/n]: ")

	var input string
	fmt.Scanln(&input)

	switch input {
	case "n", "no", "N", "No":
		return false, nil
	default:
		return true, nil
	}
}

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

func (s *Splitter) displaySuccessSummary(result *types.SplitResult, plan *types.PartitionPlan) {
	fmt.Println()
	fmt.Println("🎉 Success Summary:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Source Branch: %s\n", result.SourceBranch)
	fmt.Printf("Target Branch: %s\n", result.TargetBranch)
	fmt.Printf("Total Files: %d\n", plan.Metadata.TotalFiles)
	fmt.Printf("Total Partitions: %d\n", plan.Metadata.TotalPartitions)
	fmt.Printf("Created Branches: %d\n", len(result.CreatedBranches))
	fmt.Println()
	fmt.Println("📋 Next Steps:")
	fmt.Println("1. Review the created branches")
	fmt.Println("2. Create PRs for each branch in dependency order")
	fmt.Println("3. Merge branches sequentially")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}
