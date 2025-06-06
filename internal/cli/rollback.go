package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"pr-splitter-cli/internal/git"

	"github.com/spf13/cobra"
)

// Command flags for rollback
var (
	dryRun bool
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback [branch-prefix]",
	Short: "Rollback and cleanup branches created by pr-splitter",
	Long: `Rollback and cleanup branches created by pr-splitter.

This command will:
1. List all branches matching the prefix pattern
2. Ask for confirmation (unless --dry-run)
3. Delete both local and remote branches
4. Return to the original branch

Examples:
  pr-split rollback pr-split            Cleanup all branches starting with 'pr-split'
  pr-split rollback feature-split-      Cleanup branches with custom prefix
  pr-split rollback pr-split --dry-run  Preview what would be deleted`,
	Args: cobra.ExactArgs(1),
	RunE: runRollback,
}

func runRollback(cmd *cobra.Command, args []string) error {
	branchPrefix := args[0]

	if dryRun {
		fmt.Printf("🔍 DRY RUN: Searching for branches with prefix: %s\n", branchPrefix)
	} else {
		fmt.Printf("🔍 Searching for branches with prefix: %s\n", branchPrefix)
	}
	fmt.Println()

	// Initialize git client
	gitClient := git.NewClient()

	// Validate git repository
	if err := gitClient.ValidateGitRepository(); err != nil {
		return fmt.Errorf("git repository validation failed: %w", err)
	}

	// Get current branch for safety
	originalBranch, err := gitClient.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Find matching branches
	localBranches, err := findLocalBranchesWithPrefix(gitClient, branchPrefix)
	if err != nil {
		return fmt.Errorf("failed to find local branches: %w", err)
	}

	remoteBranches, err := findRemoteBranchesWithPrefix(gitClient, branchPrefix)
	if err != nil {
		return fmt.Errorf("failed to find remote branches: %w", err)
	}

	// Display what would be deleted
	if len(localBranches) == 0 && len(remoteBranches) == 0 {
		fmt.Printf("✅ No branches found with prefix '%s'\n", branchPrefix)
		return nil
	}

	fmt.Printf("📋 Found branches to delete:\n")
	fmt.Println()

	if len(localBranches) > 0 {
		fmt.Printf("Local branches (%d):\n", len(localBranches))
		for _, branch := range localBranches {
			fmt.Printf("  🔸 %s\n", branch)
		}
		fmt.Println()
	}

	if len(remoteBranches) > 0 {
		fmt.Printf("Remote branches (%d):\n", len(remoteBranches))
		for _, branch := range remoteBranches {
			fmt.Printf("  🔸 %s\n", branch)
		}
		fmt.Println()
	}

	// For dry run, just show what would be deleted
	if dryRun {
		fmt.Printf("🔍 DRY RUN: Would delete %d local and %d remote branches\n", len(localBranches), len(remoteBranches))
		fmt.Println("Run without --dry-run to actually delete these branches")
		return nil
	}

	// Ask for confirmation
	if !promptForConfirmation(fmt.Sprintf("Delete %d local and %d remote branches?", len(localBranches), len(remoteBranches))) {
		fmt.Println("❌ Rollback cancelled by user")
		return nil
	}

	// Perform rollback
	return performRollback(gitClient, localBranches, remoteBranches, originalBranch)
}

// performRollback executes the actual branch deletion
func performRollback(gitClient *git.Client, localBranches, remoteBranches []string, originalBranch string) error {
	fmt.Printf("🔄 Starting rollback...\n")

	// Checkout to original branch to safely delete other branches
	safetyBranch := originalBranch
	if containsString(localBranches, originalBranch) {
		// Current branch will be deleted, checkout to main/master
		safetyBranch = "main"
		if err := gitClient.CheckoutBranch(safetyBranch); err != nil {
			safetyBranch = "master"
			if err := gitClient.CheckoutBranch(safetyBranch); err != nil {
				return fmt.Errorf("failed to checkout to safe branch (tried main/master): %w", err)
			}
		}
		fmt.Printf("💼 Checked out to safe branch: %s\n", safetyBranch)
	}

	// Delete remote branches first
	for _, branch := range remoteBranches {
		fmt.Printf("🗑️  Deleting remote branch: %s\n", branch)
		if err := gitClient.DeleteRemoteBranch(branch); err != nil {
			fmt.Printf("⚠️  Warning: Could not delete remote branch %s: %v\n", branch, err)
		} else {
			fmt.Printf("✅ Deleted remote branch: %s\n", branch)
		}
	}

	// Delete local branches
	for _, branch := range localBranches {
		if branch == safetyBranch {
			fmt.Printf("⚠️  Skipping current branch: %s\n", branch)
			continue
		}

		fmt.Printf("🗑️  Deleting local branch: %s\n", branch)
		if err := gitClient.DeleteLocalBranch(branch); err != nil {
			fmt.Printf("⚠️  Warning: Could not delete local branch %s: %v\n", branch, err)
		} else {
			fmt.Printf("✅ Deleted local branch: %s\n", branch)
		}
	}

	fmt.Printf("🎉 Rollback completed successfully!\n")
	fmt.Printf("📍 Currently on branch: %s\n", safetyBranch)

	return nil
}

// findLocalBranchesWithPrefix finds local branches matching the prefix
func findLocalBranchesWithPrefix(gitClient *git.Client, prefix string) ([]string, error) {
	branches, err := gitClient.GetLocalBranches()
	if err != nil {
		return nil, err
	}

	var matching []string
	for _, branch := range branches {
		if strings.HasPrefix(branch, prefix) {
			matching = append(matching, branch)
		}
	}

	return matching, nil
}

// findRemoteBranchesWithPrefix finds remote branches matching the prefix
func findRemoteBranchesWithPrefix(gitClient *git.Client, prefix string) ([]string, error) {
	branches, err := gitClient.GetRemoteBranches()
	if err != nil {
		return nil, err
	}

	var matching []string
	for _, branch := range branches {
		// Remove origin/ prefix for consistency (assumes origin remote)
		cleanBranch := strings.TrimPrefix(branch, "origin/")
		if strings.HasPrefix(cleanBranch, prefix) {
			matching = append(matching, cleanBranch)
		}
	}

	return matching, nil
}

// promptForConfirmation asks user for yes/no confirmation
func promptForConfirmation(message string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [y/N]: ", message)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "y", "yes":
			return true
		case "n", "no", "":
			return false
		default:
			fmt.Println("Please enter 'y' for yes or 'n' for no")
		}
	}
}

// containsString checks if a slice contains a string
func containsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

func init() {
	// Add dry-run flag to rollback command
	rollbackCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be deleted without actually deleting")
}
