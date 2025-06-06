package partition

import (
	"fmt"
	"path/filepath"
	"strings"

	"pr-splitter-cli/internal/types"
)

// PartitionNamer generates meaningful names and descriptions for partitions
type PartitionNamer struct{}

// NewPartitionNamer creates a new partition namer
func NewPartitionNamer() *PartitionNamer {
	return &PartitionNamer{}
}

// GenerateName generates a concise name for a partition
func (n *PartitionNamer) GenerateName(files []types.FileChange) string {
	if len(files) == 0 {
		return "empty"
	}

	// Try common directory
	if commonDir := n.findCommonDirectory(files); commonDir != "" {
		return n.sanitizeName(commonDir)
	}

	// Try file type patterns
	if name := n.generateByFileType(files); name != "" {
		return name
	}

	// Try functionality patterns
	if name := n.generateByFunctionality(files); name != "" {
		return name
	}

	// Fallback to generic name
	return fmt.Sprintf("partition-%d-files", len(files))
}

// GenerateDescription generates a descriptive text for a partition
func (n *PartitionNamer) GenerateDescription(files []types.FileChange) string {
	name := n.GenerateName(files)

	// Make description more readable
	readableName := strings.ReplaceAll(name, "-", " ")
	readableName = strings.Title(readableName)

	return fmt.Sprintf("%s (%d files)", readableName, len(files))
}

// findCommonDirectory finds the most common directory among files
func (n *PartitionNamer) findCommonDirectory(files []types.FileChange) string {
	if len(files) == 0 {
		return ""
	}

	dirCount := make(map[string]int)

	for _, file := range files {
		dir := filepath.Dir(file.Path)
		if dir == "." {
			continue
		}

		// Count both full directory and top-level directory
		dirCount[dir]++

		parts := strings.Split(dir, "/")
		if len(parts) > 0 {
			dirCount[parts[0]]++
		}
	}

	// Find directory that appears in more than half the files
	threshold := len(files) / 2
	bestDir := ""
	maxCount := 0

	for dir, count := range dirCount {
		if count > maxCount && count > threshold {
			maxCount = count
			bestDir = dir
		}
	}

	return bestDir
}

// generateByFileType generates name based on file extensions
func (n *PartitionNamer) generateByFileType(files []types.FileChange) string {
	extensions := make(map[string]int)

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Path))
		if ext != "" {
			extensions[ext]++
		}
	}

	// Check for dominant file types
	totalFiles := len(files)

	typeNames := map[string]string{
		".tsx":  "components",
		".jsx":  "components",
		".ts":   "typescript",
		".js":   "javascript",
		".py":   "python",
		".go":   "golang",
		".css":  "styles",
		".scss": "styles",
		".sass": "styles",
		".json": "config",
		".yaml": "config",
		".yml":  "config",
		".md":   "docs",
		".html": "markup",
	}

	for ext, count := range extensions {
		if count > totalFiles/2 {
			if name, exists := typeNames[ext]; exists {
				return name
			}
		}
	}

	return ""
}

// generateByFunctionality generates name based on code patterns and keywords
func (n *PartitionNamer) generateByFunctionality(files []types.FileChange) string {
	pathText := strings.Join(n.getAllPaths(files), " ")
	lowerPathText := strings.ToLower(pathText)

	functionalityPatterns := []struct {
		keywords []string
		name     string
	}{
		{[]string{"auth", "authentication", "login", "signin"}, "authentication"},
		{[]string{"user", "profile", "account"}, "user-management"},
		{[]string{"api", "endpoint", "route", "handler"}, "api"},
		{[]string{"database", "db", "model", "schema"}, "database"},
		{[]string{"component", "ui", "interface"}, "components"},
		{[]string{"util", "helper", "common"}, "utilities"},
		{[]string{"test", "spec", "__test__"}, "tests"},
		{[]string{"config", "setting", "constant"}, "configuration"},
		{[]string{"style", "css", "theme"}, "styling"},
		{[]string{"service", "client", "provider"}, "services"},
		{[]string{"hook", "context", "state"}, "state-management"},
		{[]string{"layout", "template", "page"}, "layout"},
		{[]string{"form", "input", "validation"}, "forms"},
		{[]string{"chart", "graph", "visualization"}, "visualization"},
		{[]string{"admin", "dashboard", "panel"}, "admin"},
	}

	for _, pattern := range functionalityPatterns {
		matches := 0
		for _, keyword := range pattern.keywords {
			if strings.Contains(lowerPathText, keyword) {
				matches++
			}
		}

		// If we find multiple keyword matches, use this functionality
		if matches >= 2 || (matches >= 1 && len(files) <= 5) {
			return pattern.name
		}
	}

	return ""
}

// getAllPaths returns all file paths as a slice
func (n *PartitionNamer) getAllPaths(files []types.FileChange) []string {
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	return paths
}

// sanitizeName cleans up a name to be suitable for branch names
func (n *PartitionNamer) sanitizeName(name string) string {
	// Replace path separators and other problematic characters
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")

	// Remove multiple consecutive dashes
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	// Trim dashes from start and end
	name = strings.Trim(name, "-")

	// Ensure it's not empty
	if name == "" {
		name = "files"
	}

	// Limit length
	if len(name) > 30 {
		name = name[:30]
		name = strings.Trim(name, "-")
	}

	return strings.ToLower(name)
}
