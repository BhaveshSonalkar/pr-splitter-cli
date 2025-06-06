package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"pr-splitter-cli/internal/types"
)

// Client handles all git operations
type Client struct {
	workingDir string
}

// NewClient creates a new git client
func NewClient() *Client {
	wd, _ := os.Getwd()
	return &Client{
		workingDir: wd,
	}
}

// ValidateGitRepository checks if we're in a valid git repository
func (c *Client) ValidateGitRepository() error {
	// Check if we're in a git repository
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = c.workingDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Check if working directory is clean
	cmd = exec.Command("git", "diff", "--quiet")
	cmd.Dir = c.workingDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("working directory has uncommitted changes - please commit or stash changes first")
	}

	// Check if there are staged changes
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = c.workingDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("working directory has staged changes - please commit or reset staged changes first")
	}

	return nil
}

// ValidateBranches validates that source and target branches exist and are accessible
func (c *Client) ValidateBranches(sourceBranch, targetBranch string) error {
	// Validate branch name format
	if err := c.validateBranchName(sourceBranch); err != nil {
		return fmt.Errorf("invalid source branch name '%s': %w", sourceBranch, err)
	}

	if err := c.validateBranchName(targetBranch); err != nil {
		return fmt.Errorf("invalid target branch name '%s': %w", targetBranch, err)
	}

	// Check if branches exist
	if err := c.verifyBranch(sourceBranch); err != nil {
		return fmt.Errorf("source branch '%s' not found: %w", sourceBranch, err)
	}

	if err := c.verifyBranch(targetBranch); err != nil {
		return fmt.Errorf("target branch '%s' not found: %w", targetBranch, err)
	}

	// Check if source branch is ahead of target
	ahead, behind, err := c.getBranchDistance(sourceBranch, targetBranch)
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

// validateBranchName checks if a branch name is valid according to Git rules
func (c *Client) validateBranchName(branchName string) error {
	if branchName == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Git branch name rules
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

// getBranchDistance returns how many commits ahead and behind source is compared to target
func (c *Client) getBranchDistance(sourceBranch, targetBranch string) (ahead, behind int, err error) {
	cmd := exec.Command("git", "rev-list", "--left-right", "--count", fmt.Sprintf("%s...%s", targetBranch, sourceBranch))
	cmd.Dir = c.workingDir

	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get branch distance: %w", err)
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
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

// GetChanges analyzes git changes between source and target branches
func (c *Client) GetChanges(sourceBranch, targetBranch string) ([]types.FileChange, error) {
	// Validate git repository state first
	if err := c.ValidateGitRepository(); err != nil {
		return nil, err
	}

	// Validate branches
	if err := c.ValidateBranches(sourceBranch, targetBranch); err != nil {
		return nil, err
	}

	// Get file changes with rename detection and line count stats
	cmd := exec.Command("git", "diff", "--numstat", "-M90", fmt.Sprintf("%s...%s", targetBranch, sourceBranch))
	cmd.Dir = c.workingDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git diff: %w", err)
	}

	changes, err := c.parseGitDiff(string(output), sourceBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to parse git diff: %w", err)
	}

	// Filter out relevant files and get project context
	relevantChanges, err := c.filterAndEnrichChanges(changes)
	if err != nil {
		return nil, fmt.Errorf("failed to process changes: %w", err)
	}

	if len(relevantChanges) == 0 {
		return nil, fmt.Errorf("no relevant file changes found between %s and %s", sourceBranch, targetBranch)
	}

	return relevantChanges, nil
}

// parseGitDiff parses the output of git diff --numstat -M
func (c *Client) parseGitDiff(output, sourceBranch string) ([]types.FileChange, error) {
	var changes []types.FileChange

	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		added := parts[0]
		deleted := parts[1]
		filePath := parts[2]

		// Handle renames (format: "added deleted oldfile newfile")
		var changeType types.ChangeType
		var oldPath string

		if len(parts) == 4 {
			// Rename detected: added deleted oldfile newfile
			changeType = types.ChangeTypeRename
			oldPath = filePath
			filePath = parts[3]
		} else {
			// Determine change type based on stats
			if added == "0" && deleted != "0" {
				changeType = types.ChangeTypeDelete
			} else if added != "0" && deleted == "0" {
				changeType = types.ChangeTypeAdd
			} else {
				changeType = types.ChangeTypeModify
			}
		}

		// Parse line counts
		linesAdded := 0
		linesDeleted := 0

		if added != "-" {
			if val, err := strconv.Atoi(added); err == nil {
				linesAdded = val
			}
		}

		if deleted != "-" {
			if val, err := strconv.Atoi(deleted); err == nil {
				linesDeleted = val
			}
		}

		// Get file content
		content, err := c.getFileContent(filePath, sourceBranch, changeType)
		if err != nil {
			// For deleted files, content might not be available
			if changeType != types.ChangeTypeDelete {
				fmt.Printf("⚠️  Warning: Could not read content for %s: %v\n", filePath, err)
			}
			content = ""
		}

		change := types.FileChange{
			Path:         filePath,
			ChangeType:   changeType,
			Content:      content,
			LinesAdded:   linesAdded,
			LinesDeleted: linesDeleted,
			IsChanged:    true,
			OldPath:      oldPath,
		}

		changes = append(changes, change)
	}

	return changes, nil
}

// getFileContent retrieves the content of a file from a specific branch
func (c *Client) getFileContent(filePath, branch string, changeType types.ChangeType) (string, error) {
	// For deleted files, don't try to get content
	if changeType == types.ChangeTypeDelete {
		return "", nil
	}

	cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", branch, filePath))
	cmd.Dir = c.workingDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get file content: %w", err)
	}

	return string(output), nil
}

// filterAndEnrichChanges filters relevant files and adds project context
func (c *Client) filterAndEnrichChanges(changes []types.FileChange) ([]types.FileChange, error) {
	var relevantChanges []types.FileChange

	// Get all project files for plugin context
	projectFiles, err := c.getAllProjectFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get project files: %w", err)
	}

	// Add project files as context (not changed)
	for _, projectFile := range projectFiles {
		// Skip if already in changes
		exists := false
		for _, change := range changes {
			if change.Path == projectFile.Path {
				exists = true
				break
			}
		}

		if !exists {
			relevantChanges = append(relevantChanges, projectFile)
		}
	}

	// Add changed files
	relevantChanges = append(relevantChanges, changes...)

	return relevantChanges, nil
}

// getAllProjectFiles gets all relevant project files for plugin context
func (c *Client) getAllProjectFiles() ([]types.FileChange, error) {
	// Find TypeScript/JavaScript files in the project
	var projectFiles []types.FileChange

	err := filepath.Walk(c.workingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Skip common ignore patterns
		if c.shouldIgnoreFile(path) {
			return nil
		}

		// Only include relevant file types
		if c.isRelevantFile(path) {
			// Get relative path from working directory
			relPath, err := filepath.Rel(c.workingDir, path)
			if err != nil {
				return err
			}

			// Convert to forward slashes for consistency
			relPath = filepath.ToSlash(relPath)

			// Read file content
			content, err := c.readFileFromDisk(path)
			if err != nil {
				fmt.Printf("⚠️  Warning: Could not read %s: %v\n", relPath, err)
				content = ""
			}

			projectFile := types.FileChange{
				Path:      relPath,
				Content:   content,
				IsChanged: false, // This is project context, not a changed file
			}

			projectFiles = append(projectFiles, projectFile)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk project directory: %w", err)
	}

	return projectFiles, nil
}

// isRelevantFile checks if a file should be included in analysis
func (c *Client) isRelevantFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	relevantExts := []string{".ts", ".tsx", ".js", ".jsx", ".json"}

	for _, relevantExt := range relevantExts {
		if ext == relevantExt {
			return true
		}
	}

	return false
}

// shouldIgnoreFile checks if a file should be ignored
func (c *Client) shouldIgnoreFile(path string) bool {
	ignorePaths := []string{
		"node_modules/",
		"dist/",
		"build/",
		".next/",
		"coverage/",
		".git/",
		"__pycache__/",
		".pytest_cache/",
		".vscode/",
		".idea/",
	}

	// Check if path contains any ignore patterns
	for _, ignore := range ignorePaths {
		if strings.Contains(path, ignore) {
			return true
		}
	}

	// Skip test files by default (can be configurable later)
	if strings.Contains(path, ".test.") || strings.Contains(path, ".spec.") {
		return true
	}

	return false
}

// readFileFromDisk reads file content from disk
func (c *Client) readFileFromDisk(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var content strings.Builder
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		content.WriteString(scanner.Text())
		content.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return content.String(), nil
}

// verifyBranch checks if a branch exists
func (c *Client) verifyBranch(branch string) error {
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = c.workingDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("branch does not exist or is not accessible")
	}

	return nil
}

// CreateBranches creates branches for each partition with rollback support
func (c *Client) CreateBranches(plan *types.PartitionPlan, cfg *types.Config, sourceBranch string) ([]string, error) {
	// Store original branch for rollback
	originalBranch, err := c.getCurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch for rollback: %w", err)
	}

	var createdBranches []string
	var pushedBranches []string

	// Defer rollback function that will execute if we return with an error
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("🔴 Panic occurred during branch creation, rolling back...\n")
			c.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			panic(r) // Re-panic after cleanup
		}
	}()

	// Create branches in dependency order
	for _, partition := range plan.Partitions {
		branchName := fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, partition.ID, partition.Name)

		// Check if branch already exists
		if c.branchExists(branchName) {
			err := fmt.Errorf("branch '%s' already exists - please delete existing branches or use a different prefix", branchName)
			c.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, err
		}

		// Determine base branch
		baseBranch, err := c.determineBaseBranch(partition, plan, cfg)
		if err != nil {
			c.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to determine base branch for partition %d: %w", partition.ID, err)
		}

		// Create and checkout new branch
		fmt.Printf("🌿 Creating branch: %s (from %s)\n", branchName, baseBranch)
		if err := c.createBranch(branchName, baseBranch); err != nil {
			c.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to create branch %s: %w", branchName, err)
		}
		createdBranches = append(createdBranches, branchName)

		// Apply file changes for this partition
		fmt.Printf("📝 Applying changes to %s (%d files)\n", branchName, len(partition.Files))
		if err := c.applyPartitionChanges(&partition, sourceBranch); err != nil {
			c.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to apply changes to branch %s: %w", branchName, err)
		}

		// Check if there are actually changes to commit
		hasChanges, err := c.hasUncommittedChanges()
		if err != nil {
			c.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to check for changes in branch %s: %w", branchName, err)
		}

		if hasChanges {
			// Commit changes
			commitMsg := fmt.Sprintf("Partition %d: %s\n\nUpdates %d files for %s",
				partition.ID, partition.Description, len(partition.Files), partition.Description)

			if err := c.commitChanges(commitMsg); err != nil {
				c.rollbackBranches(createdBranches, pushedBranches, originalBranch)
				return nil, fmt.Errorf("failed to commit changes to branch %s: %w", branchName, err)
			}
		} else {
			fmt.Printf("⚠️  No changes to commit in branch %s\n", branchName)
		}

		// Push branch
		fmt.Printf("⬆️  Pushing branch: %s\n", branchName)
		if err := c.pushBranch(branchName); err != nil {
			c.rollbackBranches(createdBranches, pushedBranches, originalBranch)
			return nil, fmt.Errorf("failed to push branch %s: %w", branchName, err)
		}
		pushedBranches = append(pushedBranches, branchName)

		fmt.Printf("✅ Successfully created and pushed branch: %s\n", branchName)
	}

	// Return to original branch
	if err := c.checkoutBranch(originalBranch); err != nil {
		fmt.Printf("⚠️  Warning: Could not return to original branch %s: %v\n", originalBranch, err)
		// Try target branch as fallback
		if err := c.checkoutBranch(cfg.TargetBranch); err != nil {
			fmt.Printf("⚠️  Warning: Could not return to target branch %s: %v\n", cfg.TargetBranch, err)
		}
	}

	fmt.Printf("🎉 Successfully created %d branches\n", len(createdBranches))
	return createdBranches, nil
}

// createBranch creates a new branch from base
func (c *Client) createBranch(branchName, baseBranch string) error {
	cmd := exec.Command("git", "checkout", "-b", branchName, baseBranch)
	cmd.Dir = c.workingDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	return nil
}

// applyPartitionChanges applies file changes for a partition
func (c *Client) applyPartitionChanges(partition *types.Partition, sourceBranch string) error {
	for _, file := range partition.Files {
		if !file.IsChanged {
			continue // Skip project context files
		}

		switch file.ChangeType {
		case types.ChangeTypeAdd, types.ChangeTypeModify:
			if err := c.checkoutFileFromBranch(file.Path, sourceBranch); err != nil {
				return fmt.Errorf("failed to checkout file %s: %w", file.Path, err)
			}

		case types.ChangeTypeDelete:
			if err := c.deleteFile(file.Path); err != nil {
				return fmt.Errorf("failed to delete file %s: %w", file.Path, err)
			}

		case types.ChangeTypeRename:
			// Handle rename: delete old file, add new file
			if file.OldPath != "" {
				if err := c.deleteFile(file.OldPath); err != nil {
					fmt.Printf("⚠️  Warning: Could not delete old file %s: %v\n", file.OldPath, err)
				}
			}
			if err := c.checkoutFileFromBranch(file.Path, sourceBranch); err != nil {
				return fmt.Errorf("failed to checkout renamed file %s: %w", file.Path, err)
			}
		}
	}

	return nil
}

// checkoutFileFromBranch checks out a specific file from a branch
func (c *Client) checkoutFileFromBranch(filePath, branch string) error {
	cmd := exec.Command("git", "checkout", branch, "--", filePath)
	cmd.Dir = c.workingDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	return nil
}

// deleteFile removes a file and stages the deletion
func (c *Client) deleteFile(filePath string) error {
	cmd := exec.Command("git", "rm", filePath)
	cmd.Dir = c.workingDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git rm failed: %w", err)
	}

	return nil
}

// commitChanges commits all staged changes
func (c *Client) commitChanges(message string) error {
	// Stage all changes
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = c.workingDir

	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Commit changes
	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = c.workingDir

	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}

// pushBranch pushes a branch to origin
func (c *Client) pushBranch(branchName string) error {
	cmd := exec.Command("git", "push", "origin", branchName)
	cmd.Dir = c.workingDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	return nil
}

// checkoutBranch checks out an existing branch
func (c *Client) checkoutBranch(branchName string) error {
	cmd := exec.Command("git", "checkout", branchName)
	cmd.Dir = c.workingDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	return nil
}

// getCurrentBranch returns the currently checked out branch
func (c *Client) getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = c.workingDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// branchExists checks if a branch exists locally
func (c *Client) branchExists(branchName string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", branchName)
	cmd.Dir = c.workingDir
	return cmd.Run() == nil
}

// hasUncommittedChanges checks if there are uncommitted changes in the working directory
func (c *Client) hasUncommittedChanges() (bool, error) {
	// Check for staged changes
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = c.workingDir
	if err := cmd.Run(); err != nil {
		return true, nil // There are staged changes
	}

	// Check for unstaged changes
	cmd = exec.Command("git", "diff", "--quiet")
	cmd.Dir = c.workingDir
	if err := cmd.Run(); err != nil {
		return true, nil // There are unstaged changes
	}

	// Check for untracked files (that would be added by git add .)
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = c.workingDir
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %w", err)
	}

	// If there's any output, there are changes
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// determineBaseBranch determines the base branch for a partition based on its dependencies
func (c *Client) determineBaseBranch(partition types.Partition, plan *types.PartitionPlan, cfg *types.Config) (string, error) {
	if len(partition.Dependencies) == 0 {
		return cfg.TargetBranch, nil
	}

	// Use the last dependency as base (assuming linear chain)
	lastDep := partition.Dependencies[len(partition.Dependencies)-1]

	// Find the actual branch name for the dependency
	for _, p := range plan.Partitions {
		if p.ID == lastDep {
			baseBranch := fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, p.ID, p.Name)
			// Verify the base branch exists
			if !c.branchExists(baseBranch) {
				return "", fmt.Errorf("dependency branch '%s' does not exist", baseBranch)
			}
			return baseBranch, nil
		}
	}

	return "", fmt.Errorf("could not find partition with ID %d", lastDep)
}

// rollbackBranches cleans up created branches when an error occurs
func (c *Client) rollbackBranches(createdBranches, pushedBranches []string, originalBranch string) {
	if len(createdBranches) == 0 && len(pushedBranches) == 0 {
		return
	}

	fmt.Printf("🔄 Rolling back branch creation...\n")

	// First, checkout to original branch to safely delete other branches
	if err := c.checkoutBranch(originalBranch); err != nil {
		fmt.Printf("⚠️  Warning: Could not checkout original branch %s during rollback: %v\n", originalBranch, err)
	}

	// Delete remote branches (pushed branches)
	for _, branchName := range pushedBranches {
		fmt.Printf("🗑️  Deleting remote branch: %s\n", branchName)
		if err := c.deleteRemoteBranch(branchName); err != nil {
			fmt.Printf("⚠️  Warning: Could not delete remote branch %s: %v\n", branchName, err)
		} else {
			fmt.Printf("✅ Deleted remote branch: %s\n", branchName)
		}
	}

	// Delete local branches
	for _, branchName := range createdBranches {
		fmt.Printf("🗑️  Deleting local branch: %s\n", branchName)
		if err := c.deleteLocalBranch(branchName); err != nil {
			fmt.Printf("⚠️  Warning: Could not delete local branch %s: %v\n", branchName, err)
		} else {
			fmt.Printf("✅ Deleted local branch: %s\n", branchName)
		}
	}

	fmt.Printf("🔄 Rollback completed. Repository returned to clean state.\n")
}

// deleteLocalBranch deletes a local branch
func (c *Client) deleteLocalBranch(branchName string) error {
	cmd := exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = c.workingDir
	return cmd.Run()
}

// deleteRemoteBranch deletes a remote branch
func (c *Client) deleteRemoteBranch(branchName string) error {
	cmd := exec.Command("git", "push", "origin", "--delete", branchName)
	cmd.Dir = c.workingDir
	return cmd.Run()
}

// Public methods for external access

// GetCurrentBranch returns the currently checked out branch (public wrapper)
func (c *Client) GetCurrentBranch() (string, error) {
	return c.getCurrentBranch()
}

// CheckoutBranch checks out an existing branch (public wrapper)
func (c *Client) CheckoutBranch(branchName string) error {
	return c.checkoutBranch(branchName)
}

// DeleteLocalBranch deletes a local branch (public wrapper)
func (c *Client) DeleteLocalBranch(branchName string) error {
	return c.deleteLocalBranch(branchName)
}

// DeleteRemoteBranch deletes a remote branch (public wrapper)
func (c *Client) DeleteRemoteBranch(branchName string) error {
	return c.deleteRemoteBranch(branchName)
}

// GetLocalBranches returns a list of all local branches
func (c *Client) GetLocalBranches() ([]string, error) {
	cmd := exec.Command("git", "branch", "--format=%(refname:short)")
	cmd.Dir = c.workingDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get local branches: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var branches []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}

	return branches, nil
}

// GetRemoteBranches returns a list of all remote branches
func (c *Client) GetRemoteBranches() ([]string, error) {
	cmd := exec.Command("git", "branch", "-r", "--format=%(refname:short)")
	cmd.Dir = c.workingDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote branches: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var branches []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "HEAD") {
			branches = append(branches, line)
		}
	}

	return branches, nil
}
