package git

import (
	"fmt"
	"strings"

	"pr-splitter-cli/internal/types"
)

// Brancher handles all git branch operations
type Brancher struct {
	workingDir string
}

// NewBrancher creates a new git brancher
func NewBrancher(workingDir string) *Brancher {
	return &Brancher{workingDir: workingDir}
}

// CreateBranches creates branches for each partition with rollback support
func (b *Brancher) CreateBranches(plan *types.PartitionPlan, cfg *types.Config, sourceBranch string) ([]string, error) {
	originalBranch, err := b.GetCurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch for rollback: %w", err)
	}

	var createdBranches []string
	var pushedBranches []string

	// Rollback on error
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("🔴 Panic occurred during branch creation, rolling back...\n")
			b.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			panic(r)
		}
	}()

	for _, partition := range plan.Partitions {
		branchName := fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, partition.ID, partition.Name)

		if b.branchExists(branchName) {
			err := fmt.Errorf("branch '%s' already exists", branchName)
			b.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, err
		}

		baseBranch, err := b.determineBaseBranch(partition, plan, cfg)
		if err != nil {
			b.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to determine base branch for partition %d: %w", partition.ID, err)
		}

		fmt.Printf("🌿 Creating branch: %s (from %s)\n", branchName, baseBranch)
		if err := b.createAndCheckoutBranch(branchName, baseBranch); err != nil {
			b.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to create branch %s: %w", branchName, err)
		}
		createdBranches = append(createdBranches, branchName)

		fmt.Printf("📝 Applying changes to %s (%d files)\n", branchName, len(partition.Files))
		if err := b.applyPartitionChanges(&partition, sourceBranch); err != nil {
			b.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to apply changes to branch %s: %w", branchName, err)
		}

		if hasChanges, err := b.hasUncommittedChanges(); err != nil {
			b.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to check for changes in branch %s: %w", branchName, err)
		} else if hasChanges {
			commitMsg := fmt.Sprintf("Partition %d: %s\n\nUpdates %d files for %s",
				partition.ID, partition.Description, len(partition.Files), partition.Description)

			if err := b.commitChanges(commitMsg); err != nil {
				b.rollbackBranches(createdBranches, pushedBranches, originalBranch)
				return nil, fmt.Errorf("failed to commit changes to branch %s: %w", branchName, err)
			}
		} else {
			fmt.Printf("⚠️  No changes to commit in branch %s\n", branchName)
		}

		fmt.Printf("⬆️  Pushing branch: %s\n", branchName)
		if err := b.pushBranch(branchName); err != nil {
			b.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to push branch %s: %w", branchName, err)
		}
		pushedBranches = append(pushedBranches, branchName)

		fmt.Printf("✅ Successfully created and pushed branch: %s\n", branchName)
	}

	if err := b.CheckoutBranch(originalBranch); err != nil {
		fmt.Printf("⚠️  Warning: Could not return to original branch %s: %v\n", originalBranch, err)
		if err := b.CheckoutBranch(cfg.TargetBranch); err != nil {
			fmt.Printf("⚠️  Warning: Could not return to target branch %s: %v\n", cfg.TargetBranch, err)
		}
	}

	fmt.Printf("🎉 Successfully created %d branches\n", len(createdBranches))
	return createdBranches, nil
}

// applyPartitionChanges applies file changes for a partition
func (b *Brancher) applyPartitionChanges(partition *types.Partition, sourceBranch string) error {
	for _, file := range partition.Files {
		if !file.IsChanged {
			continue
		}

		switch file.ChangeType {
		case types.ChangeTypeAdd, types.ChangeTypeModify:
			if err := b.checkoutFileFromBranch(file.Path, sourceBranch); err != nil {
				return fmt.Errorf("failed to checkout file %s: %w", file.Path, err)
			}

		case types.ChangeTypeDelete:
			if err := b.deleteFile(file.Path); err != nil {
				return fmt.Errorf("failed to delete file %s: %w", file.Path, err)
			}

		case types.ChangeTypeRename:
			if file.OldPath != "" {
				if err := b.deleteFile(file.OldPath); err != nil {
					fmt.Printf("⚠️  Warning: Could not delete old file %s: %v\n", file.OldPath, err)
				}
			}
			if err := b.checkoutFileFromBranch(file.Path, sourceBranch); err != nil {
				return fmt.Errorf("failed to checkout renamed file %s: %w", file.Path, err)
			}
		}
	}
	return nil
}

// Branch utility methods

func (b *Brancher) createAndCheckoutBranch(branchName, baseBranch string) error {
	return runGitCommandQuiet(b.workingDir, "checkout", "-b", branchName, baseBranch)
}

func (b *Brancher) checkoutFileFromBranch(filePath, branch string) error {
	return runGitCommandQuiet(b.workingDir, "checkout", branch, "--", filePath)
}

func (b *Brancher) deleteFile(filePath string) error {
	return runGitCommandQuiet(b.workingDir, "rm", filePath)
}

func (b *Brancher) commitChanges(message string) error {
	if err := runGitCommandQuiet(b.workingDir, "add", "."); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}
	return runGitCommandQuiet(b.workingDir, "commit", "-m", message)
}

func (b *Brancher) pushBranch(branchName string) error {
	return runGitCommandQuiet(b.workingDir, "push", "origin", branchName)
}

func (b *Brancher) CheckoutBranch(branchName string) error {
	return runGitCommandQuiet(b.workingDir, "checkout", branchName)
}

func (b *Brancher) GetCurrentBranch() (string, error) {
	return runGitCommand(b.workingDir, "branch", "--show-current")
}

func (b *Brancher) branchExists(branchName string) bool {
	return runGitCommandQuiet(b.workingDir, "rev-parse", "--verify", branchName) == nil
}

func (b *Brancher) hasUncommittedChanges() (bool, error) {
	// Check for staged changes
	if err := runGitCommandQuiet(b.workingDir, "diff", "--cached", "--quiet"); err != nil {
		return true, nil
	}

	// Check for unstaged changes
	if err := runGitCommandQuiet(b.workingDir, "diff", "--quiet"); err != nil {
		return true, nil
	}

	// Check for untracked files
	output, err := runGitCommand(b.workingDir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %w", err)
	}

	return len(strings.TrimSpace(output)) > 0, nil
}

func (b *Brancher) determineBaseBranch(partition types.Partition, plan *types.PartitionPlan, cfg *types.Config) (string, error) {
	if len(partition.Dependencies) == 0 {
		return cfg.TargetBranch, nil
	}

	lastDep := partition.Dependencies[len(partition.Dependencies)-1]

	for _, p := range plan.Partitions {
		if p.ID == lastDep {
			baseBranch := fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, p.ID, p.Name)
			if !b.branchExists(baseBranch) {
				return "", fmt.Errorf("dependency branch '%s' does not exist", baseBranch)
			}
			return baseBranch, nil
		}
	}

	return "", fmt.Errorf("could not find partition with ID %d", lastDep)
}

// Branch management methods

func (b *Brancher) DeleteLocalBranch(branchName string) error {
	return runGitCommandQuiet(b.workingDir, "branch", "-D", branchName)
}

func (b *Brancher) DeleteRemoteBranch(branchName string) error {
	return runGitCommandQuiet(b.workingDir, "push", "origin", "--delete", branchName)
}

func (b *Brancher) GetLocalBranches() ([]string, error) {
	output, err := runGitCommand(b.workingDir, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("failed to get local branches: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var branches []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}

	return branches, nil
}

func (b *Brancher) GetRemoteBranches() ([]string, error) {
	output, err := runGitCommand(b.workingDir, "branch", "-r", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("failed to get remote branches: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var branches []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "HEAD") {
			branches = append(branches, line)
		}
	}

	return branches, nil
}

// rollbackBranches cleans up created branches when an error occurs
func (b *Brancher) rollbackBranches(createdBranches, pushedBranches []string, originalBranch string) {
	if len(createdBranches) == 0 && len(pushedBranches) == 0 {
		return
	}

	fmt.Printf("🔄 Rolling back branch creation...\n")

	if err := b.CheckoutBranch(originalBranch); err != nil {
		fmt.Printf("⚠️  Warning: Could not checkout original branch %s during rollback: %v\n", originalBranch, err)
	}

	// Delete remote branches first
	for _, branchName := range pushedBranches {
		fmt.Printf("🗑️  Deleting remote branch: %s\n", branchName)
		if err := b.DeleteRemoteBranch(branchName); err != nil {
			fmt.Printf("⚠️  Warning: Could not delete remote branch %s: %v\n", branchName, err)
		} else {
			fmt.Printf("✅ Deleted remote branch: %s\n", branchName)
		}
	}

	// Delete local branches
	for _, branchName := range createdBranches {
		if branchName == originalBranch {
			fmt.Printf("⚠️  Skipping current branch: %s\n", branchName)
			continue
		}

		fmt.Printf("🗑️  Deleting local branch: %s\n", branchName)
		if err := b.DeleteLocalBranch(branchName); err != nil {
			fmt.Printf("⚠️  Warning: Could not delete local branch %s: %v\n", branchName, err)
		} else {
			fmt.Printf("✅ Deleted local branch: %s\n", branchName)
		}
	}

	fmt.Printf("🔄 Rollback completed. Repository returned to clean state.\n")
}
