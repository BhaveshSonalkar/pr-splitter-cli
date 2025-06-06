package partition

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"pr-splitter-cli/internal/config"
	"pr-splitter-cli/internal/types"
)

// Partitioner creates logical partitions based on dependencies
type Partitioner struct {
	// Internal state for partitioning
	depthCache map[string]int // Cache for dependency depth calculations
}

// NewPartitioner creates a new partitioner instance
func NewPartitioner() *Partitioner {
	return &Partitioner{}
}

// CreatePlan creates a partition plan based on file changes and dependencies
func (p *Partitioner) CreatePlan(changes []types.FileChange, dependencies []types.Dependency, cfg *types.Config) (*types.PartitionPlan, error) {
	// Filter to only changed files for partitioning
	changedFiles := p.filterChangedFiles(changes)

	if len(changedFiles) == 0 {
		return nil, fmt.Errorf("no changed files to partition")
	}

	fmt.Printf("📊 Partitioning %d changed files with %d dependencies\n", len(changedFiles), len(dependencies))

	// Build dependency graph
	graph, err := p.buildDependencyGraph(changedFiles, dependencies)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Find strongly connected components (circular dependencies)
	sccs, err := p.findStronglyConnectedComponents(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to find SCCs: %w", err)
	}

	// Handle oversized SCCs with user prompts
	approvedSCCs, err := p.handleOversizedSCCs(sccs, cfg.MaxFilesPerPartition)
	if err != nil {
		return nil, fmt.Errorf("failed to handle oversized SCCs: %w", err)
	}

	// Create partitions using dependency-first strategy
	partitions, err := p.createPartitions(changedFiles, graph, approvedSCCs, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create partitions: %w", err)
	}

	// Build partition plan
	plan := &types.PartitionPlan{
		Partitions: partitions,
		Metadata: types.PlanMetadata{
			TotalFiles:           len(changedFiles),
			TotalPartitions:      len(partitions),
			MaxFilesPerPartition: cfg.MaxFilesPerPartition,
			Strategy:             cfg.Strategy,
			CreatedAt:            time.Now(),
		},
	}

	return plan, nil
}

// filterChangedFiles returns only files that were actually changed
func (p *Partitioner) filterChangedFiles(changes []types.FileChange) []types.FileChange {
	var changedFiles []types.FileChange

	for _, change := range changes {
		if change.IsChanged {
			changedFiles = append(changedFiles, change)
		}
	}

	return changedFiles
}

// buildDependencyGraph creates a dependency graph from files and dependencies
func (p *Partitioner) buildDependencyGraph(files []types.FileChange, dependencies []types.Dependency) (*types.DependencyGraph, error) {
	graph := &types.DependencyGraph{
		Nodes:     make([]string, 0),
		Edges:     dependencies,
		Adjacency: make(map[string][]string),
		InDegree:  make(map[string]int),
		OutDegree: make(map[string]int),
	}

	// Add all files as nodes
	nodeSet := make(map[string]bool)
	for _, file := range files {
		if !nodeSet[file.Path] {
			graph.Nodes = append(graph.Nodes, file.Path)
			nodeSet[file.Path] = true
			graph.InDegree[file.Path] = 0
			graph.OutDegree[file.Path] = 0
		}
	}

	// Build adjacency list and degree counts
	for _, dep := range dependencies {
		// Only include dependencies between changed files
		if nodeSet[dep.From] && nodeSet[dep.To] {
			graph.Adjacency[dep.From] = append(graph.Adjacency[dep.From], dep.To)
			graph.OutDegree[dep.From]++
			graph.InDegree[dep.To]++
		}
	}

	return graph, nil
}

// findStronglyConnectedComponents finds circular dependency groups using Tarjan's algorithm
func (p *Partitioner) findStronglyConnectedComponents(graph *types.DependencyGraph) ([]types.StronglyConnectedComponent, error) {
	var sccs []types.StronglyConnectedComponent

	// Tarjan's SCC algorithm state
	index := 0
	stack := make([]string, 0)
	indices := make(map[string]int)
	lowlinks := make(map[string]int)
	onStack := make(map[string]bool)

	// Helper function for Tarjan's algorithm
	var strongConnect func(string)
	strongConnect = func(v string) {
		indices[v] = index
		lowlinks[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		// Consider successors of v
		for _, w := range graph.Adjacency[v] {
			if _, exists := indices[w]; !exists {
				// Successor w has not been visited; recurse
				strongConnect(w)
				if lowlinks[w] < lowlinks[v] {
					lowlinks[v] = lowlinks[w]
				}
			} else if onStack[w] {
				// Successor w is in stack and hence in current SCC
				if indices[w] < lowlinks[v] {
					lowlinks[v] = indices[w]
				}
			}
		}

		// If v is a root node, pop the stack and generate an SCC
		if lowlinks[v] == indices[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}

			// Only add SCCs with more than one node (circular dependencies)
			if len(scc) > 1 {
				sccs = append(sccs, types.StronglyConnectedComponent{
					Files: scc,
					Size:  len(scc),
				})
			}
		}
	}

	// Run Tarjan's algorithm on all unvisited nodes
	for _, node := range graph.Nodes {
		if _, visited := indices[node]; !visited {
			strongConnect(node)
		}
	}

	// Sort SCCs by size (largest first) for better user experience
	sort.Slice(sccs, func(i, j int) bool {
		return sccs[i].Size > sccs[j].Size
	})

	if len(sccs) > 0 {
		fmt.Printf("🔄 Found %d circular dependency groups\n", len(sccs))
		for i, scc := range sccs {
			fmt.Printf("   Group %d: %d files\n", i+1, scc.Size)
		}
	}

	return sccs, nil
}

// handleOversizedSCCs prompts user for approval of SCCs that exceed size limits
func (p *Partitioner) handleOversizedSCCs(sccs []types.StronglyConnectedComponent, maxSize int) ([]types.StronglyConnectedComponent, error) {
	var approvedSCCs []types.StronglyConnectedComponent

	for _, scc := range sccs {
		if scc.Size > maxSize {
			// Prompt user for decision
			approved, err := config.PromptForSCCDecision(scc.Files, scc.Size, maxSize)
			if err != nil {
				return nil, fmt.Errorf("SCC approval failed: %w", err)
			}

			if approved {
				approvedSCCs = append(approvedSCCs, scc)
			} else {
				return nil, fmt.Errorf("user rejected oversized SCC with %d files", scc.Size)
			}
		} else {
			approvedSCCs = append(approvedSCCs, scc)
		}
	}

	return approvedSCCs, nil
}

// createPartitions creates partitions using dependency-first strategy
func (p *Partitioner) createPartitions(files []types.FileChange, graph *types.DependencyGraph, sccs []types.StronglyConnectedComponent, cfg *types.Config) ([]types.Partition, error) {
	var partitions []types.Partition

	// Track which files are already allocated
	allocated := make(map[string]bool)

	// First, allocate SCCs as complete groups
	for _, scc := range sccs {
		sccFiles := p.getFilesByPaths(files, scc.Files)

		partition := types.Partition{
			ID:           len(partitions) + 1,
			Name:         p.generatePartitionName(sccFiles),
			Description:  fmt.Sprintf("Circular dependency group (%d files)", len(sccFiles)),
			Files:        sccFiles,
			Dependencies: p.calculatePartitionDependencies(scc.Files, partitions, graph),
		}

		partition.BranchName = fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, partition.ID, partition.Name)
		partitions = append(partitions, partition)

		// Mark files as allocated
		for _, filePath := range scc.Files {
			allocated[filePath] = true
		}
	}

	// Then, allocate remaining files using dependency-first strategy
	remainingFiles := p.getRemainingFiles(files, allocated)

	if len(remainingFiles) > 0 {
		dependencyPartitions, err := p.createDependencyBasedPartitions(remainingFiles, graph, partitions, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create dependency-based partitions: %w", err)
		}

		partitions = append(partitions, dependencyPartitions...)
	}

	// Final validation and adjustment
	partitions = p.balancePartitions(partitions, cfg.MaxFilesPerPartition)

	return partitions, nil
}

// createDependencyBasedPartitions creates partitions for non-SCC files based on dependencies
func (p *Partitioner) createDependencyBasedPartitions(files []types.FileChange, graph *types.DependencyGraph, existingPartitions []types.Partition, cfg *types.Config) ([]types.Partition, error) {
	var partitions []types.Partition
	allocated := make(map[string]bool)

	// Create a working copy of the graph with only unallocated files
	workingNodes := make([]string, 0)
	for _, file := range files {
		workingNodes = append(workingNodes, file.Path)
	}

	// Group files by their dependency depth (files with no dependencies first)
	depthGroups := p.groupFilesByDependencyDepth(workingNodes, graph)

	// Track processed depths to prevent infinite loops
	maxDepth := len(workingNodes) // Upper bound on possible depth

	// Create partitions starting from lowest dependency depth
	for depth := 0; len(workingNodes) > 0 && len(partitions) < cfg.MaxPartitions-len(existingPartitions) && depth <= maxDepth; depth++ {
		depthFiles := depthGroups[depth]

		// If no files at this depth and we've processed all possible depths,
		// handle remaining files in a final catch-all partition
		if len(depthFiles) == 0 && depth >= maxDepth {
			// Collect all remaining unallocated files
			for _, node := range workingNodes {
				if !allocated[node] {
					depthFiles = append(depthFiles, node)
				}
			}
		}

		// Skip empty depth levels
		if len(depthFiles) == 0 {
			continue
		}

		// Create partition for this depth level
		partitionFiles := make([]types.FileChange, 0)
		fileCount := 0

		for _, filePath := range depthFiles {
			if allocated[filePath] {
				continue
			}

			// Check partition size limit
			if fileCount >= cfg.MaxFilesPerPartition {
				break
			}

			file := p.getFileByPath(files, filePath)
			if file != nil {
				partitionFiles = append(partitionFiles, *file)
				allocated[filePath] = true
				fileCount++
			}
		}

		// Remove allocated files from working nodes
		var remainingNodes []string
		for _, node := range workingNodes {
			if !allocated[node] {
				remainingNodes = append(remainingNodes, node)
			}
		}
		workingNodes = remainingNodes

		// Create partition if we have files
		if len(partitionFiles) > 0 {
			partition := types.Partition{
				ID:           len(existingPartitions) + len(partitions) + 1,
				Name:         p.generatePartitionName(partitionFiles),
				Description:  p.generatePartitionDescription(partitionFiles),
				Files:        partitionFiles,
				Dependencies: p.calculatePartitionDependencies(p.getFilePaths(partitionFiles), append(existingPartitions, partitions...), graph),
			}

			partition.BranchName = fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, partition.ID, partition.Name)
			partitions = append(partitions, partition)
		}
	}

	// Ensure all files are allocated - if not, it's an error condition
	if len(workingNodes) > 0 {
		var unallocatedFiles []string
		for _, node := range workingNodes {
			if !allocated[node] {
				unallocatedFiles = append(unallocatedFiles, node)
			}
		}
		if len(unallocatedFiles) > 0 {
			return partitions, fmt.Errorf("failed to allocate %d files to partitions: %v", len(unallocatedFiles), unallocatedFiles)
		}
	}

	return partitions, nil
}

// groupFilesByDependencyDepth groups files by their dependency depth (0 = no dependencies)
func (p *Partitioner) groupFilesByDependencyDepth(nodes []string, graph *types.DependencyGraph) map[int][]string {
	groups := make(map[int][]string)

	for _, node := range nodes {
		depth := p.calculateDependencyDepth(node, graph, make(map[string]bool))
		groups[depth] = append(groups[depth], node)
	}

	return groups
}

// calculateDependencyDepth calculates the maximum dependency depth for a file
func (p *Partitioner) calculateDependencyDepth(node string, graph *types.DependencyGraph, visiting map[string]bool) int {
	// Check for cycles - if we're currently visiting this node, it's a cycle
	if visiting[node] {
		return 0 // Return 0 for circular dependencies
	}

	// Use memoization to avoid recalculating depths
	if depth, exists := p.depthCache[node]; exists {
		return depth
	}

	visiting[node] = true
	maxDepth := 0

	// Check all dependencies
	for _, dep := range graph.Adjacency[node] {
		depth := 1 + p.calculateDependencyDepth(dep, graph, visiting)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	visiting[node] = false // Mark as no longer visiting

	// Cache the result
	if p.depthCache == nil {
		p.depthCache = make(map[string]int)
	}
	p.depthCache[node] = maxDepth

	return maxDepth
}

// Helper functions

func (p *Partitioner) getFilesByPaths(files []types.FileChange, paths []string) []types.FileChange {
	var result []types.FileChange

	for _, path := range paths {
		for _, file := range files {
			if file.Path == path {
				result = append(result, file)
				break
			}
		}
	}

	return result
}

func (p *Partitioner) getFileByPath(files []types.FileChange, path string) *types.FileChange {
	for _, file := range files {
		if file.Path == path {
			return &file
		}
	}
	return nil
}

func (p *Partitioner) getRemainingFiles(files []types.FileChange, allocated map[string]bool) []types.FileChange {
	var remaining []types.FileChange

	for _, file := range files {
		if !allocated[file.Path] {
			remaining = append(remaining, file)
		}
	}

	return remaining
}

func (p *Partitioner) getFilePaths(files []types.FileChange) []string {
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	return paths
}

func (p *Partitioner) removeFromSlice(slice []string, item string) []string {
	for i, v := range slice {
		if v == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func (p *Partitioner) generatePartitionName(files []types.FileChange) string {
	if len(files) == 0 {
		return "empty"
	}

	// Try to find a common directory or pattern
	commonDir := p.findCommonDirectory(files)
	if commonDir != "" {
		return strings.ReplaceAll(commonDir, "/", "-")
	}

	// Fallback to file type or generic name
	if p.hasFileType(files, ".tsx") || p.hasFileType(files, ".jsx") {
		return "components"
	}

	if p.hasFileType(files, ".ts") && p.containsKeyword(files, "utils") {
		return "utils"
	}

	return fmt.Sprintf("partition-%d-files", len(files))
}

func (p *Partitioner) generatePartitionDescription(files []types.FileChange) string {
	name := p.generatePartitionName(files)
	return fmt.Sprintf("%s (%d files)", strings.Title(strings.ReplaceAll(name, "-", " ")), len(files))
}

func (p *Partitioner) findCommonDirectory(files []types.FileChange) string {
	if len(files) == 0 {
		return ""
	}

	// Get directory paths
	var dirs []string
	for _, file := range files {
		dir := strings.Split(file.Path, "/")
		if len(dir) > 1 {
			dirs = append(dirs, dir[0])
		}
	}

	// Find most common directory
	dirCount := make(map[string]int)
	for _, dir := range dirs {
		dirCount[dir]++
	}

	maxCount := 0
	commonDir := ""
	for dir, count := range dirCount {
		if count > maxCount && count > len(files)/2 {
			maxCount = count
			commonDir = dir
		}
	}

	return commonDir
}

func (p *Partitioner) hasFileType(files []types.FileChange, ext string) bool {
	for _, file := range files {
		if strings.HasSuffix(file.Path, ext) {
			return true
		}
	}
	return false
}

func (p *Partitioner) containsKeyword(files []types.FileChange, keyword string) bool {
	for _, file := range files {
		if strings.Contains(strings.ToLower(file.Path), keyword) {
			return true
		}
	}
	return false
}

func (p *Partitioner) calculatePartitionDependencies(filePaths []string, existingPartitions []types.Partition, graph *types.DependencyGraph) []int {
	var dependencies []int
	depSet := make(map[int]bool)

	// Check if any files in this partition depend on files in existing partitions
	for _, filePath := range filePaths {
		for _, dep := range graph.Adjacency[filePath] {
			// Find which partition contains the dependency
			for _, partition := range existingPartitions {
				for _, partitionFile := range partition.Files {
					if partitionFile.Path == dep {
						depSet[partition.ID] = true
						break
					}
				}
			}
		}
	}

	// Convert set to slice
	for id := range depSet {
		dependencies = append(dependencies, id)
	}

	// Sort for consistency
	sort.Ints(dependencies)

	return dependencies
}

func (p *Partitioner) balancePartitions(partitions []types.Partition, maxSize int) []types.Partition {
	// For now, just return as-is
	// Future enhancement: redistribute files for better balance
	return partitions
}
