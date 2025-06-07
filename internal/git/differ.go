package git

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pr-splitter-cli/internal/types"
)

// Differ handles git diff operations and file analysis
type Differ struct {
	workingDir string
}

// NewDiffer creates a new git differ
func NewDiffer(workingDir string) *Differ {
	return &Differ{workingDir: workingDir}
}

// GetChanges analyzes git changes between source and target branches
func (d *Differ) GetChanges(sourceBranch, targetBranch string) ([]types.FileChange, error) {
	// Get file changes with rename detection and line count stats
	output, err := runGitCommand(d.workingDir, "diff", "--numstat", "-M90",
		fmt.Sprintf("%s...%s", targetBranch, sourceBranch))
	if err != nil {
		return nil, fmt.Errorf("failed to get git diff: %w", err)
	}

	changes, err := d.parseGitDiff(output, sourceBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to parse git diff: %w", err)
	}

	relevantChanges, err := d.filterAndEnrichChanges(changes)
	if err != nil {
		return nil, fmt.Errorf("failed to process changes: %w", err)
	}

	if len(relevantChanges) == 0 {
		return nil, fmt.Errorf("no relevant file changes found between %s and %s", sourceBranch, targetBranch)
	}

	return relevantChanges, nil
}

// parseGitDiff parses the output of git diff --numstat -M
func (d *Differ) parseGitDiff(output, sourceBranch string) ([]types.FileChange, error) {
	var changes []types.FileChange
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		change, err := d.parseDiffLine(line, sourceBranch)
		if err != nil {
			fmt.Printf("⚠️  Warning: %v\n", err)
			continue
		}

		if change != nil {
			changes = append(changes, *change)
		}
	}

	return changes, nil
}

// parseDiffLine parses a single line from git diff output
func (d *Differ) parseDiffLine(line, sourceBranch string) (*types.FileChange, error) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid diff line format: %s", line)
	}

	added := parts[0]
	deleted := parts[1]
	filePath := strings.Join(parts[2:], " ")

	if !isValidFilePath(filePath) {
		return nil, fmt.Errorf("skipping malformed file path: %s", filePath)
	}

	changeType, oldPath := d.determineChangeType(filePath, added, deleted, parts)

	if changeType == "" {
		return nil, fmt.Errorf("could not determine change type for: %s", filePath)
	}

	linesAdded, linesDeleted := d.parseLineNumbers(added, deleted)

	// For renamed files, use the new path instead of the git rename format
	actualPath := filePath
	if changeType == types.ChangeTypeRename && isGitRenameFormat(filePath) {
		_, actualPath = parseGitRenameFormat(filePath)
	}

	content, err := d.getFileContent(actualPath, sourceBranch, changeType)
	if err != nil && changeType != types.ChangeTypeDelete {
		fmt.Printf("⚠️  Warning: Could not read content for %s: %v\n", filePath, err)
	}

	return &types.FileChange{
		Path:         actualPath,
		ChangeType:   changeType,
		Content:      content,
		LinesAdded:   linesAdded,
		LinesDeleted: linesDeleted,
		IsChanged:    true,
		OldPath:      oldPath,
	}, nil
}

// determineChangeType determines the type of change and handles renames
func (d *Differ) determineChangeType(filePath, added, deleted string, parts []string) (types.ChangeType, string) {
	// Handle Git's {oldname => newname} rename format
	if isGitRenameFormat(filePath) {
		oldPath, newPath := parseGitRenameFormat(filePath)
		if isValidFilePath(oldPath) && isValidFilePath(newPath) {
			return types.ChangeTypeRename, oldPath
		}
	}

	// Handle "added deleted oldfile newfile" format
	if len(parts) == 4 {
		oldPath := filePath
		newPath := parts[3]
		if isValidFilePath(oldPath) && isValidFilePath(newPath) {
			return types.ChangeTypeRename, oldPath
		}
	}

	// Regular change types
	if added == "0" && deleted != "0" {
		return types.ChangeTypeDelete, ""
	}
	if added != "0" && deleted == "0" {
		return types.ChangeTypeAdd, ""
	}
	return types.ChangeTypeModify, ""
}

// parseLineNumbers parses added and deleted line counts
func (d *Differ) parseLineNumbers(added, deleted string) (int, int) {
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

	return linesAdded, linesDeleted
}

// getFileContent retrieves the content of a file from a specific branch
func (d *Differ) getFileContent(filePath, branch string, changeType types.ChangeType) (string, error) {
	if changeType == types.ChangeTypeDelete {
		return "", nil
	}

	if !isValidFilePath(filePath) {
		return "", fmt.Errorf("invalid file path: %s", filePath)
	}

	output, err := runGitCommand(d.workingDir, "show", fmt.Sprintf("%s:%s", branch, filePath))
	if err != nil {
		return "", fmt.Errorf("git show failed for %s: %w", filePath, err)
	}

	return output, nil
}

// filterAndEnrichChanges filters relevant files and adds project context
func (d *Differ) filterAndEnrichChanges(changes []types.FileChange) ([]types.FileChange, error) {
	var relevantChanges []types.FileChange

	// Get all project files for plugin context
	projectFiles, err := d.getAllProjectFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get project files: %w", err)
	}

	// Add project files as context (not changed)
	for _, projectFile := range projectFiles {
		if !d.fileExistsInChanges(projectFile.Path, changes) {
			relevantChanges = append(relevantChanges, projectFile)
		}
	}

	// Add changed files
	relevantChanges = append(relevantChanges, changes...)

	return relevantChanges, nil
}

// fileExistsInChanges checks if a file path exists in the changes list
func (d *Differ) fileExistsInChanges(path string, changes []types.FileChange) bool {
	for _, change := range changes {
		if change.Path == path {
			return true
		}
	}
	return false
}

// getAllProjectFiles gets all relevant project files for plugin context
func (d *Differ) getAllProjectFiles() ([]types.FileChange, error) {
	var projectFiles []types.FileChange

	err := filepath.Walk(d.workingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		if shouldIgnoreFile(path) || !isRelevantFile(path) {
			return nil
		}

		relPath, err := filepath.Rel(d.workingDir, path)
		if err != nil {
			return err
		}

		relPath = filepath.ToSlash(relPath)
		content, err := d.readFileFromDisk(path)
		if err != nil {
			fmt.Printf("⚠️  Warning: Could not read %s: %v\n", relPath, err)
			content = ""
		}

		projectFiles = append(projectFiles, types.FileChange{
			Path:      relPath,
			Content:   content,
			IsChanged: false,
		})

		return nil
	})

	return projectFiles, err
}

// readFileFromDisk reads file content from disk
func (d *Differ) readFileFromDisk(path string) (string, error) {
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

	return content.String(), scanner.Err()
}

// Utility functions

// isRelevantFile checks if a file should be included in analysis
func isRelevantFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	relevantExts := []string{".ts", ".tsx", ".js", ".jsx", ".json", ".py", ".pyi"}

	for _, relevantExt := range relevantExts {
		if ext == relevantExt {
			return true
		}
	}
	return false
}

// shouldIgnoreFile checks if a file should be ignored
func shouldIgnoreFile(path string) bool {
	ignorePaths := []string{
		"node_modules/", "dist/", "build/", ".next/", "coverage/",
		".git/", "__pycache__/", ".pytest_cache/", ".vscode/", ".idea/",
	}

	for _, ignore := range ignorePaths {
		if strings.Contains(path, ignore) {
			return true
		}
	}

	return strings.Contains(path, ".test.") || strings.Contains(path, ".spec.")
}

// isValidFilePath checks if a file path is valid
func isValidFilePath(filePath string) bool {
	if filePath == "" || len(filePath) > 4096 {
		return false
	}

	openBraces := strings.Count(filePath, "{")
	closeBraces := strings.Count(filePath, "}")
	if openBraces != closeBraces {
		return false
	}

	if strings.HasPrefix(filePath, "{") && !strings.Contains(filePath, " => ") {
		return false
	}

	malformedPatterns := []string{"\x00", "\r", "\n"}
	for _, pattern := range malformedPatterns {
		if strings.Contains(filePath, pattern) {
			return false
		}
	}

	return !strings.Contains(filePath, "../") && !strings.Contains(filePath, "..\\")
}

// isGitRenameFormat checks if a file path represents a valid Git rename format
func isGitRenameFormat(filePath string) bool {
	if !strings.Contains(filePath, "{") || !strings.Contains(filePath, " => ") || !strings.Contains(filePath, "}") {
		return false
	}

	openBraces := strings.Count(filePath, "{")
	closeBraces := strings.Count(filePath, "}")
	if openBraces != closeBraces {
		return false
	}

	braceStart := strings.Index(filePath, "{")
	braceEnd := strings.LastIndex(filePath, "}")
	arrowPos := strings.Index(filePath, " => ")

	return braceStart != -1 && braceEnd != -1 && arrowPos != -1 && arrowPos > braceStart && arrowPos < braceEnd
}

// parseGitRenameFormat parses Git's {oldname => newname} rename format
func parseGitRenameFormat(filePath string) (oldPath, newPath string) {
	braceStart := strings.Index(filePath, "{")
	braceEnd := strings.Index(filePath, "}")

	if braceStart == -1 || braceEnd == -1 {
		return filePath, filePath
	}

	basePath := filePath[:braceStart]
	renameContent := filePath[braceStart+1 : braceEnd]

	parts := strings.Split(renameContent, " => ")
	if len(parts) != 2 {
		return filePath, filePath
	}

	oldName := strings.TrimSpace(parts[0])
	newName := strings.TrimSpace(parts[1])

	return basePath + oldName, basePath + newName
}
