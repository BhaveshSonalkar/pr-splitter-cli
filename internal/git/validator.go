package git

import (
	"fmt"
	"strconv"
	"strings"
)

// Validator handles all git repository validation
type Validator struct {
	workingDir string
}

// NewValidator creates a new git validator
func NewValidator(workingDir string) *Validator {
	return &Validator{workingDir: workingDir}
}

// ValidateRepository checks if we're in a valid git repository
func (v *Validator) ValidateRepository() error {
	if err := v.checkGitRepository(); err != nil {
		return err
	}

	if err := v.checkWorkingDirectoryClean(); err != nil {
		return err
	}

	return v.checkNoStagedChanges()
}

// ValidateBranches validates that source and target branches exist and are accessible
func (v *Validator) ValidateBranches(sourceBranch, targetBranch string) error {
	if err := v.validateBranchName(sourceBranch); err != nil {
		return fmt.Errorf("invalid source branch name '%s': %w", sourceBranch, err)
	}

	if err := v.validateBranchName(targetBranch); err != nil {
		return fmt.Errorf("invalid target branch name '%s': %w", targetBranch, err)
	}

	if err := v.verifyBranch(sourceBranch); err != nil {
		return fmt.Errorf("source branch '%s' not found: %w", sourceBranch, err)
	}

	if err := v.verifyBranch(targetBranch); err != nil {
		return fmt.Errorf("target branch '%s' not found: %w", targetBranch, err)
	}

	return v.validateBranchDistance(sourceBranch, targetBranch)
}

// checkGitRepository verifies we're in a git repository
func (v *Validator) checkGitRepository() error {
	if err := runGitCommandQuiet(v.workingDir, "rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}
	return nil
}

// checkWorkingDirectoryClean ensures no uncommitted changes
func (v *Validator) checkWorkingDirectoryClean() error {
	if err := runGitCommandQuiet(v.workingDir, "diff", "--quiet"); err != nil {
		return fmt.Errorf("working directory has uncommitted changes - please commit or stash changes first")
	}
	return nil
}

// checkNoStagedChanges ensures no staged changes exist
func (v *Validator) checkNoStagedChanges() error {
	if err := runGitCommandQuiet(v.workingDir, "diff", "--cached", "--quiet"); err != nil {
		return fmt.Errorf("working directory has staged changes - please commit or reset staged changes first")
	}
	return nil
}

// validateBranchName checks if a branch name is valid according to Git rules
func (v *Validator) validateBranchName(branchName string) error {
	if branchName == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	invalidChars := []string{" ", "~", "^", ":", "?", "*", "[", "\\", "..", "@{", "//"}
	for _, char := range invalidChars {
		if strings.Contains(branchName, char) {
			return fmt.Errorf("branch name contains invalid character '%s'", char)
		}
	}

	if strings.HasPrefix(branchName, "-") || strings.HasSuffix(branchName, ".") {
		return fmt.Errorf("branch name cannot start with '-' or end with '.'")
	}

	return nil
}

// verifyBranch checks if a branch exists
func (v *Validator) verifyBranch(branch string) error {
	if err := runGitCommandQuiet(v.workingDir, "rev-parse", "--verify", branch); err != nil {
		return fmt.Errorf("branch does not exist or is not accessible")
	}
	return nil
}

// validateBranchDistance checks that source branch has changes compared to target
func (v *Validator) validateBranchDistance(sourceBranch, targetBranch string) error {
	ahead, behind, err := v.getBranchDistance(sourceBranch, targetBranch)
	if err != nil {
		return fmt.Errorf("failed to check branch distance: %w", err)
	}

	if ahead == 0 {
		return fmt.Errorf("source branch '%s' has no changes compared to '%s'", sourceBranch, targetBranch)
	}

	fmt.Printf("📊 Branch analysis: %s is %d commits ahead and %d commits behind %s\n",
		sourceBranch, ahead, behind, targetBranch)

	return nil
}

// getBranchDistance returns how many commits ahead and behind source is compared to target
func (v *Validator) getBranchDistance(sourceBranch, targetBranch string) (ahead, behind int, err error) {
	output, err := runGitCommand(v.workingDir, "rev-list", "--left-right", "--count",
		fmt.Sprintf("%s...%s", targetBranch, sourceBranch))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get branch distance: %w", err)
	}

	parts := strings.Fields(output)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected git rev-list output format")
	}

	behind, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse behind count: %w", err)
	}

	ahead, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse ahead count: %w", err)
	}

	return ahead, behind, nil
}
