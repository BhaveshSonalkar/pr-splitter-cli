package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"pr-splitter-cli/internal/types"
)

// Manager handles plugin discovery, execution, and communication
type Manager struct {
	pluginDir string
	plugins   map[string]*Plugin
}

// Plugin represents a language-specific analysis plugin
type Plugin struct {
	Name        string   `json:"name"`
	Executable  string   `json:"executable"`
	Extensions  []string `json:"extensions"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
}

// NewManager creates a new plugin manager
func NewManager() *Manager {
	// Try to find plugins directory relative to executable
	execPath, err := os.Executable()
	if err != nil {
		// Fallback to current directory + plugins
		wd, _ := os.Getwd()
		execPath = wd
	}

	pluginDir := filepath.Join(filepath.Dir(execPath), "plugins")

	// If that doesn't exist, try relative to working directory
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		wd, _ := os.Getwd()
		pluginDir = filepath.Join(wd, "plugins")
	}

	manager := &Manager{
		pluginDir: pluginDir,
		plugins:   make(map[string]*Plugin),
	}

	// Discover available plugins
	manager.discoverPlugins()

	return manager
}

// discoverPlugins finds and registers available plugins
func (m *Manager) discoverPlugins() {
	// TypeScript plugin
	tsPlugin := &Plugin{
		Name:        "typescript",
		Executable:  filepath.Join(m.pluginDir, "typescript", "analyzer.js"),
		Extensions:  []string{".ts", ".tsx", ".js", ".jsx"},
		Description: "TypeScript/JavaScript dependency analyzer",
		Version:     "1.0.0",
	}

	// Check if TypeScript plugin exists
	if _, err := os.Stat(tsPlugin.Executable); err == nil {
		m.plugins["typescript"] = tsPlugin
		fmt.Printf("📦 Discovered plugin: %s (%s)\n", tsPlugin.Name, tsPlugin.Description)
	} else {
		fmt.Printf("⚠️  TypeScript plugin not found at: %s\n", tsPlugin.Executable)
	}

	// Future plugins can be added here
	// pythonPlugin := &Plugin{...}
}

// AnalyzeDependencies runs appropriate plugins to analyze file dependencies
func (m *Manager) AnalyzeDependencies(changes []types.FileChange) ([]types.Dependency, error) {
	var allDependencies []types.Dependency

	// Group files by plugin type
	fileGroups := m.groupFilesByPlugin(changes)

	// Run each plugin for its file group
	for pluginName, files := range fileGroups {
		if len(files) == 0 {
			continue
		}

		plugin, exists := m.plugins[pluginName]
		if !exists {
			fmt.Printf("⚠️  Plugin '%s' not available, using fallback analysis\n", pluginName)
			// Use generic fallback analysis
			fallbackDeps := m.fallbackAnalysis(files)
			allDependencies = append(allDependencies, fallbackDeps...)
			continue
		}

		fmt.Printf("🔍 Running %s plugin on %d files...\n", plugin.Name, len(files))

		dependencies, err := m.executePlugin(plugin, files)
		if err != nil {
			fmt.Printf("⚠️  Plugin '%s' failed: %v\n", plugin.Name, err)
			fmt.Printf("🔄 Falling back to generic analysis for %s files\n", plugin.Name)

			// Use fallback analysis
			fallbackDeps := m.fallbackAnalysis(files)
			allDependencies = append(allDependencies, fallbackDeps...)
			continue
		}

		fmt.Printf("✅ %s plugin found %d dependencies\n", plugin.Name, len(dependencies))
		allDependencies = append(allDependencies, dependencies...)
	}

	return allDependencies, nil
}

// groupFilesByPlugin groups files by their appropriate plugin
func (m *Manager) groupFilesByPlugin(files []types.FileChange) map[string][]types.FileChange {
	groups := make(map[string][]types.FileChange)

	for _, file := range files {
		pluginName := m.getPluginForFile(file.Path)
		if pluginName != "" {
			groups[pluginName] = append(groups[pluginName], file)
		}
	}

	return groups
}

// getPluginForFile determines which plugin should handle a file
func (m *Manager) getPluginForFile(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	// Check each plugin's supported extensions
	for pluginName, plugin := range m.plugins {
		for _, supportedExt := range plugin.Extensions {
			if ext == supportedExt {
				return pluginName
			}
		}
	}

	return "" // No plugin found
}

// executePlugin runs a plugin and returns its analysis results
func (m *Manager) executePlugin(plugin *Plugin, files []types.FileChange) ([]types.Dependency, error) {
	startTime := time.Now()

	// Separate changed files from project context files
	var changedFiles []types.FileChange
	var projectFiles []types.FileChange

	for _, file := range files {
		if file.IsChanged {
			changedFiles = append(changedFiles, file)
		} else {
			projectFiles = append(projectFiles, file)
		}
	}

	// Prepare plugin input
	input := types.PluginInput{
		ChangedFiles: changedFiles,
		ProjectFiles: projectFiles,
		ProjectRoot:  m.getProjectRoot(),
	}

	// Convert to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal plugin input: %w", err)
	}

	// Execute plugin
	cmd := exec.Command("node", plugin.Executable)
	cmd.Stdin = strings.NewReader(string(inputJSON))

	// Capture output
	output, err := cmd.Output()
	if err != nil {
		// Get stderr for better error reporting
		if exitError, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("plugin execution failed: %s\nStderr: %s", err, string(exitError.Stderr))
		}
		return nil, fmt.Errorf("plugin execution failed: %w", err)
	}

	// Parse plugin output
	var pluginOutput types.PluginOutput
	if err := json.Unmarshal(output, &pluginOutput); err != nil {
		return nil, fmt.Errorf("failed to parse plugin output: %w\nOutput: %s", err, string(output))
	}

	// Check for plugin errors
	if len(pluginOutput.Errors) > 0 {
		fmt.Printf("⚠️  Plugin reported errors:\n")
		for _, errMsg := range pluginOutput.Errors {
			fmt.Printf("   - %s\n", errMsg)
		}
	}

	// Update metadata with timing
	duration := time.Since(startTime)
	pluginOutput.Metadata.AnalysisTime = duration.String()

	fmt.Printf("📊 Plugin analysis completed in %s\n", duration)

	return pluginOutput.Dependencies, nil
}

// getProjectRoot returns the project root directory
func (m *Manager) getProjectRoot() string {
	// Try to find git root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	// Fallback to current working directory
	wd, _ := os.Getwd()
	return wd
}

// fallbackAnalysis provides basic dependency analysis when plugins fail
func (m *Manager) fallbackAnalysis(files []types.FileChange) []types.Dependency {
	var dependencies []types.Dependency

	fmt.Printf("🔍 Running fallback analysis on %d files...\n", len(files))

	// Create a map of all available files for quick lookup
	availableFiles := make(map[string]bool)
	for _, file := range files {
		availableFiles[file.Path] = true

		// Also add common variations
		if strings.HasSuffix(file.Path, ".ts") {
			// Add .js version
			jsPath := strings.TrimSuffix(file.Path, ".ts") + ".js"
			availableFiles[jsPath] = true
		}
	}

	// Analyze each changed file
	for _, file := range files {
		if !file.IsChanged {
			continue
		}

		// Simple regex-based import detection
		fileDeps := m.extractImportsFromContent(file.Content, file.Path, availableFiles)
		dependencies = append(dependencies, fileDeps...)
	}

	fmt.Printf("📊 Fallback analysis found %d dependencies\n", len(dependencies))

	return dependencies
}

// extractImportsFromContent uses regex to find import statements
func (m *Manager) extractImportsFromContent(content, filePath string, availableFiles map[string]bool) []types.Dependency {
	var dependencies []types.Dependency

	lines := strings.Split(content, "\n")
	baseDir := filepath.Dir(filePath)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// TypeScript/JavaScript import patterns
		var importPath string

		// import ... from "path"
		if strings.HasPrefix(line, "import ") && strings.Contains(line, " from ") {
			parts := strings.Split(line, " from ")
			if len(parts) == 2 {
				importPath = strings.Trim(parts[1], `"';`)
			}
		}

		// const ... = require("path")
		if strings.Contains(line, "require(") {
			start := strings.Index(line, "require(") + 8
			end := strings.Index(line[start:], ")")
			if end > 0 {
				importPath = strings.Trim(line[start:start+end], `"'`)
			}
		}

		if importPath != "" {
			// Resolve relative imports
			resolvedPath := m.resolveImportPath(importPath, baseDir, availableFiles)

			if resolvedPath != "" {
				dependency := types.Dependency{
					From:     filePath,
					To:       resolvedPath,
					Type:     "import",
					Strength: types.StrengthStrong, // Default to strong for imports
					Line:     lineNum + 1,
					Context:  line,
				}
				dependencies = append(dependencies, dependency)
			}
		}
	}

	return dependencies
}

// resolveImportPath resolves import paths to actual file paths
func (m *Manager) resolveImportPath(importPath, baseDir string, availableFiles map[string]bool) string {
	// Skip external modules (no relative path)
	if !strings.HasPrefix(importPath, ".") {
		return ""
	}

	// Resolve relative path
	resolved := filepath.Join(baseDir, importPath)
	resolved = filepath.Clean(resolved)
	resolved = filepath.ToSlash(resolved) // Convert to forward slashes

	// Try different extensions
	extensions := []string{"", ".ts", ".tsx", ".js", ".jsx", "/index.ts", "/index.js"}

	for _, ext := range extensions {
		candidate := resolved + ext
		if availableFiles[candidate] {
			return candidate
		}
	}

	return ""
}

// GetAvailablePlugins returns information about available plugins
func (m *Manager) GetAvailablePlugins() map[string]*Plugin {
	return m.plugins
}
