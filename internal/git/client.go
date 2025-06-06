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

// GetChanges analyzes git changes between source and target branches
func (c *Client) GetChanges(sourceBranch, targetBranch string) ([]types.FileChange, error) {
	// First verify branches exist
	if err := c.verifyBranch(sourceBranch); err != nil {
		return nil, fmt.Errorf("source branch '%s' not found: %w", sourceBranch, err)
	}

	if err := c.verifyBranch(targetBranch); err != nil {
		return nil, fmt.Errorf("target branch '%s' not found: %w", targetBranch, err)
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

// CreateBranches creates branches for each partition
func (c *Client) CreateBranches(plan *types.PartitionPlan, cfg *types.Config, sourceBranch string) ([]string, error) {
	var createdBranches []string

	// Create branches in dependency order
	for _, partition := range plan.Partitions {
		branchName := fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, partition.ID, partition.Name)

		// Determine base branch
		var baseBranch string
		if len(partition.Dependencies) == 0 {
			baseBranch = cfg.TargetBranch
		} else {
			// Use the last dependency as base (assuming linear chain)
			lastDep := partition.Dependencies[len(partition.Dependencies)-1]
			baseBranch = fmt.Sprintf("%s-%d", cfg.BranchPrefix, lastDep)
			// Find the actual branch name
			for _, p := range plan.Partitions {
				if p.ID == lastDep {
					baseBranch = fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, p.ID, p.Name)
					break
				}
			}
		}

		// Create and checkout new branch
		if err := c.createBranch(branchName, baseBranch); err != nil {
			return createdBranches, fmt.Errorf("failed to create branch %s: %w", branchName, err)
		}

		// Apply file changes for this partition
		if err := c.applyPartitionChanges(&partition, sourceBranch); err != nil {
			return createdBranches, fmt.Errorf("failed to apply changes to branch %s: %w", branchName, err)
		}

		// Commit changes
		commitMsg := fmt.Sprintf("Partition %d: %s\n\nUpdates %d files for %s",
			partition.ID, partition.Description, len(partition.Files), partition.Description)

		if err := c.commitChanges(commitMsg); err != nil {
			return createdBranches, fmt.Errorf("failed to commit changes to branch %s: %w", branchName, err)
		}

		// Push branch
		if err := c.pushBranch(branchName); err != nil {
			return createdBranches, fmt.Errorf("failed to push branch %s: %w", branchName, err)
		}

		createdBranches = append(createdBranches, branchName)
		fmt.Printf("✅ Created branch: %s\n", branchName)
	}

	// Return to target branch
	if err := c.checkoutBranch(cfg.TargetBranch); err != nil {
		fmt.Printf("⚠️  Warning: Could not return to %s branch: %v\n", cfg.TargetBranch, err)
	}

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
