package partition

import (
	"fmt"
	"path/filepath"
	"strings"

	"pr-splitter-cli/internal/types"
)

// FileGrouper groups files by type, directory, and other logical patterns
type FileGrouper struct{}

// NewFileGrouper creates a new file grouper
func NewFileGrouper() *FileGrouper {
	return &FileGrouper{}
}

// GroupFiles groups files into logical categories
func (g *FileGrouper) GroupFiles(files []types.FileChange) map[string][]types.FileChange {
	groups := make(map[string][]types.FileChange)

	for _, file := range files {
		groupKey := g.determineGroup(file)
		groups[groupKey] = append(groups[groupKey], file)
	}

	return groups
}

// determineGroup determines which group a file belongs to
func (g *FileGrouper) determineGroup(file types.FileChange) string {
	path := file.Path

	// Group by file type first
	if group := g.groupByFileType(path); group != "" {
		return group
	}

	// Group by directory structure
	if group := g.groupByDirectory(path); group != "" {
		return group
	}

	// Fallback group
	return "miscellaneous"
}

// groupByFileType groups files by their extension
func (g *FileGrouper) groupByFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	typeGroups := map[string]string{
		".md":    "documentation",
		".txt":   "documentation",
		".mdx":   "documentation",
		".json":  "configuration",
		".yaml":  "configuration",
		".yml":   "configuration",
		".toml":  "configuration",
		".xml":   "configuration",
		".css":   "styles",
		".scss":  "styles",
		".sass":  "styles",
		".less":  "styles",
		".styl":  "styles",
		".png":   "assets",
		".jpg":   "assets",
		".jpeg":  "assets",
		".gif":   "assets",
		".svg":   "assets",
		".ico":   "assets",
		".woff":  "assets",
		".woff2": "assets",
		".ttf":   "assets",
		".eot":   "assets",
	}

	if group, exists := typeGroups[ext]; exists {
		return group
	}

	return ""
}

// groupByDirectory groups files by their directory structure
func (g *FileGrouper) groupByDirectory(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return ""
	}

	topDir := strings.ToLower(parts[0])

	directoryGroups := map[string]string{
		"public":        "static-assets",
		"static":        "static-assets",
		"assets":        "static-assets",
		"images":        "static-assets",
		"docs":          "documentation",
		"doc":           "documentation",
		"documentation": "documentation",
		"config":        "configuration",
		"configs":       "configuration",
		"settings":      "configuration",
		"tests":         "tests",
		"test":          "tests",
		"__tests__":     "tests",
		"spec":          "tests",
		"specs":         "tests",
		"styles":        "styles",
		"css":           "styles",
		"scss":          "styles",
		"components":    "components",
		"component":     "components",
		"pages":         "pages",
		"views":         "views",
		"routes":        "routes",
		"api":           "api",
		"services":      "services",
		"service":       "services",
		"utils":         "utilities",
		"util":          "utilities",
		"helpers":       "utilities",
		"lib":           "libraries",
		"libs":          "libraries",
		"vendor":        "vendor",
		"node_modules":  "vendor",
	}

	if group, exists := directoryGroups[topDir]; exists {
		return group
	}

	// Check for test patterns in any directory level
	if g.containsTestPattern(path) {
		return "tests"
	}

	// Use the top directory as a fallback group
	return fmt.Sprintf("dir-%s", topDir)
}

// containsTestPattern checks if the path contains test-related patterns
func (g *FileGrouper) containsTestPattern(path string) bool {
	lowerPath := strings.ToLower(path)

	testPatterns := []string{
		".test.",
		".spec.",
		"_test.",
		"_spec.",
		"/test/",
		"/tests/",
		"/spec/",
		"/specs/",
		"/__tests__/",
	}

	for _, pattern := range testPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}

	return false
}
