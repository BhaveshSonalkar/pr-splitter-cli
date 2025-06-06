package partition

import (
	"fmt"
	"sort"
	"time"

	"pr-splitter-cli/internal/config"
	"pr-splitter-cli/internal/types"
)

// Partitioner creates logical partitions based on dependencies
type Partitioner struct {
	depthCache map[string]int
}

// NewPartitioner creates a new partitioner instance
func NewPartitioner() *Partitioner {
	return &Partitioner{}
}

// CreatePlan creates a partition plan based on file changes and dependencies
func (p *Partitioner) CreatePlan(changes []types.FileChange, dependencies []types.Dependency, cfg *types.Config) (*types.PartitionPlan, error) {
	p.depthCache = make(map[string]int)

	changedFiles := p.filterChangedFiles(changes)
	if len(changedFiles) == 0 {
		return nil, fmt.Errorf("no changed files to partition")
	}

	fmt.Printf("📊 Partitioning %d changed files with %d dependencies\n", len(changedFiles), len(dependencies))

	graph, err := p.buildDependencyGraph(changedFiles, dependencies)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	sccs, err := p.findCircularDependencies(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to find circular dependencies: %w", err)
	}

	approvedSCCs, err := p.handleOversizedCircularGroups(sccs, cfg.MaxFilesPerPartition)
	if err != nil {
		return nil, fmt.Errorf("failed to handle oversized circular groups: %w", err)
	}

	partitions, err := p.createAllPartitions(changedFiles, graph, approvedSCCs, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create partitions: %w", err)
	}

	if err := p.validateExhaustiveness(changedFiles, partitions); err != nil {
		return nil, fmt.Errorf("exhaustiveness validation failed: %w", err)
	}

	return &types.PartitionPlan{
		Partitions: partitions,
		Metadata: types.PlanMetadata{
			TotalFiles:           len(changedFiles),
			TotalPartitions:      len(partitions),
			MaxFilesPerPartition: cfg.MaxFilesPerPartition,
			Strategy:             cfg.Strategy,
			CreatedAt:            time.Now(),
		},
	}, nil
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
	nodeSet := make(map[string]bool)
	for _, file := range files {
		nodeSet[file.Path] = true
	}

	graph := &types.DependencyGraph{
		Nodes:     make([]string, 0, len(files)),
		Edges:     make([]types.Dependency, 0),
		Adjacency: make(map[string][]string),
		InDegree:  make(map[string]int),
		OutDegree: make(map[string]int),
	}

	// Initialize nodes
	for path := range nodeSet {
		graph.Nodes = append(graph.Nodes, path)
		graph.InDegree[path] = 0
		graph.OutDegree[path] = 0
	}

	// Add edges between changed files only
	for _, dep := range dependencies {
		if nodeSet[dep.From] && nodeSet[dep.To] {
			graph.Edges = append(graph.Edges, dep)
			graph.Adjacency[dep.From] = append(graph.Adjacency[dep.From], dep.To)
			graph.OutDegree[dep.From]++
			graph.InDegree[dep.To]++
		}
	}

	return graph, nil
}

// findCircularDependencies finds circular dependency groups using Tarjan's algorithm
func (p *Partitioner) findCircularDependencies(graph *types.DependencyGraph) ([]types.StronglyConnectedComponent, error) {
	tarjan := NewTarjanSCC(graph)
	sccs := tarjan.FindSCCs()

	// Filter to only circular dependencies (size > 1)
	var circularSCCs []types.StronglyConnectedComponent
	for _, scc := range sccs {
		if scc.Size > 1 {
			circularSCCs = append(circularSCCs, scc)
		}
	}

	// Sort by size (largest first)
	sort.Slice(circularSCCs, func(i, j int) bool {
		return circularSCCs[i].Size > circularSCCs[j].Size
	})

	if len(circularSCCs) > 0 {
		fmt.Printf("🔄 Found %d circular dependency groups\n", len(circularSCCs))
		for i, scc := range circularSCCs {
			fmt.Printf("   Group %d: %d files\n", i+1, scc.Size)
		}
	}

	return circularSCCs, nil
}

// handleOversizedCircularGroups prompts user for approval of large circular groups
func (p *Partitioner) handleOversizedCircularGroups(sccs []types.StronglyConnectedComponent, maxSize int) ([]types.StronglyConnectedComponent, error) {
	var approvedSCCs []types.StronglyConnectedComponent

	for _, scc := range sccs {
		if scc.Size > maxSize {
			approved, err := config.PromptForSCCDecision(scc.Files, scc.Size, maxSize)
			if err != nil {
				return nil, fmt.Errorf("SCC approval failed: %w", err)
			}
			if !approved {
				return nil, fmt.Errorf("user rejected oversized SCC with %d files", scc.Size)
			}
		}
		approvedSCCs = append(approvedSCCs, scc)
	}

	return approvedSCCs, nil
}

// createAllPartitions creates all partitions using the configured strategy
func (p *Partitioner) createAllPartitions(files []types.FileChange, graph *types.DependencyGraph, sccs []types.StronglyConnectedComponent, cfg *types.Config) ([]types.Partition, error) {
	var partitions []types.Partition
	allocated := make(map[string]bool)

	// First: Create partitions for circular dependency groups
	partitions = p.createCircularDependencyPartitions(sccs, files, partitions, cfg, allocated)

	// Second: Create dependency-based partitions for remaining files
	remainingFiles := p.getRemainingFiles(files, allocated)
	if len(remainingFiles) > 0 {
		depPartitions, err := p.createDependencyPartitions(remainingFiles, graph, partitions, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create dependency partitions: %w", err)
		}
		partitions = append(partitions, depPartitions...)

		// Update allocated files
		for _, partition := range depPartitions {
			for _, file := range partition.Files {
				allocated[file.Path] = true
			}
		}
	}

	// Third: Handle any remaining unallocated files
	unallocatedFiles := p.getRemainingFiles(files, allocated)
	if len(unallocatedFiles) > 0 {
		fmt.Printf("📋 Creating partitions for %d unallocated files...\n", len(unallocatedFiles))
		remainingPartitions := p.createRemainingFilePartitions(unallocatedFiles, partitions, cfg)
		partitions = append(partitions, remainingPartitions...)
	}

	return partitions, nil
}

// createCircularDependencyPartitions creates partitions for circular dependency groups
func (p *Partitioner) createCircularDependencyPartitions(sccs []types.StronglyConnectedComponent, files []types.FileChange, existingPartitions []types.Partition, cfg *types.Config, allocated map[string]bool) []types.Partition {
	var partitions []types.Partition

	for _, scc := range sccs {
		sccFiles := p.getFilesByPaths(files, scc.Files)

		partition := types.Partition{
			ID:           len(existingPartitions) + len(partitions) + 1,
			Name:         p.generateName(sccFiles),
			Description:  fmt.Sprintf("Circular dependency group (%d files)", len(sccFiles)),
			Files:        sccFiles,
			Dependencies: p.calculateDependencies(scc.Files, append(existingPartitions, partitions...)),
		}

		partition.BranchName = fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, partition.ID, partition.Name)
		partitions = append(partitions, partition)

		// Mark files as allocated
		for _, filePath := range scc.Files {
			allocated[filePath] = true
		}
	}

	return partitions
}

// createDependencyPartitions creates partitions based on dependency depth
func (p *Partitioner) createDependencyPartitions(files []types.FileChange, graph *types.DependencyGraph, existingPartitions []types.Partition, cfg *types.Config) ([]types.Partition, error) {
	var partitions []types.Partition
	allocated := make(map[string]bool)

	workingNodes := p.getFilePaths(files)
	depthGroups := p.groupByDependencyDepth(workingNodes, graph)

	totalFiles := len(workingNodes)
	maxCapacity := cfg.MaxPartitions * cfg.MaxFilesPerPartition
	willExceedCapacity := totalFiles > maxCapacity

	if willExceedCapacity {
		fmt.Printf("⚠️  Warning: %d files may exceed capacity (%d max)\n", totalFiles, maxCapacity)
	}

	// Process files by dependency depth
	for depth := 0; len(workingNodes) > 0 && depth <= len(workingNodes); depth++ {
		depthFiles := depthGroups[depth]
		if len(depthFiles) == 0 {
			continue
		}

		partitionGroup := p.createPartitionForDepth(depthFiles, files, allocated, existingPartitions, partitions, cfg)
		partitions = append(partitions, partitionGroup...)

		// Update working nodes
		workingNodes = p.removeAllocatedNodes(workingNodes, allocated)
	}

	return partitions, nil
}

// createPartitionForDepth creates a partition for files at a specific dependency depth
func (p *Partitioner) createPartitionForDepth(depthFiles []string, allFiles []types.FileChange, allocated map[string]bool, existingPartitions, currentPartitions []types.Partition, cfg *types.Config) []types.Partition {
	var partitionFiles []types.FileChange

	for _, filePath := range depthFiles {
		if allocated[filePath] {
			continue
		}

		if len(partitionFiles) >= cfg.MaxFilesPerPartition {
			break
		}

		if file := p.getFileByPath(allFiles, filePath); file != nil {
			partitionFiles = append(partitionFiles, *file)
			allocated[filePath] = true
		}
	}

	if len(partitionFiles) == 0 {
		return nil
	}

	partition := types.Partition{
		ID:           len(existingPartitions) + len(currentPartitions) + 1,
		Name:         p.generateName(partitionFiles),
		Description:  p.generateDescription(partitionFiles),
		Files:        partitionFiles,
		Dependencies: p.calculateDependencies(p.getFilePaths(partitionFiles), append(existingPartitions, currentPartitions...)),
	}

	partition.BranchName = fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, partition.ID, partition.Name)
	return []types.Partition{partition}
}

// createRemainingFilePartitions creates simple partitions for unallocated files
func (p *Partitioner) createRemainingFilePartitions(files []types.FileChange, existingPartitions []types.Partition, cfg *types.Config) []types.Partition {
	fileGrouper := NewFileGrouper()
	groups := fileGrouper.GroupFiles(files)

	var partitions []types.Partition
	for groupName, groupFiles := range groups {
		groupPartitions := p.createSimplePartitions(groupFiles, len(existingPartitions)+len(partitions), cfg, groupName)
		partitions = append(partitions, groupPartitions...)
	}

	// Fallback to simple size-based partitioning if no groups
	if len(partitions) == 0 {
		partitions = p.createSimplePartitions(files, len(existingPartitions), cfg, "remaining")
	}

	return partitions
}

// createSimplePartitions creates basic size-based partitions
func (p *Partitioner) createSimplePartitions(files []types.FileChange, startID int, cfg *types.Config, baseName string) []types.Partition {
	var partitions []types.Partition

	for i := 0; i < len(files); i += cfg.MaxFilesPerPartition {
		end := i + cfg.MaxFilesPerPartition
		if end > len(files) {
			end = len(files)
		}

		partitionFiles := files[i:end]
		partitionNum := (i / cfg.MaxFilesPerPartition) + 1
		name := fmt.Sprintf("%s-%d", baseName, partitionNum)

		partition := types.Partition{
			ID:           startID + len(partitions) + 1,
			Name:         name,
			Description:  fmt.Sprintf("%s files (%d files)", baseName, len(partitionFiles)),
			Files:        partitionFiles,
			Dependencies: []int{},
			BranchName:   fmt.Sprintf("%s-%d-%s", cfg.BranchPrefix, startID+len(partitions)+1, name),
		}

		partitions = append(partitions, partition)
	}

	return partitions
}

// validateExhaustiveness ensures all changed files are included in partitions
func (p *Partitioner) validateExhaustiveness(changedFiles []types.FileChange, partitions []types.Partition) error {
	partitionFiles := make(map[string]bool)
	for _, partition := range partitions {
		for _, file := range partition.Files {
			partitionFiles[file.Path] = true
		}
	}

	for _, file := range changedFiles {
		if !partitionFiles[file.Path] {
			return fmt.Errorf("file %s not included in any partition", file.Path)
		}
	}

	return nil
}

// Utility methods

func (p *Partitioner) getFilesByPaths(files []types.FileChange, paths []string) []types.FileChange {
	pathSet := make(map[string]bool)
	for _, path := range paths {
		pathSet[path] = true
	}

	var result []types.FileChange
	for _, file := range files {
		if pathSet[file.Path] {
			result = append(result, file)
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

func (p *Partitioner) removeAllocatedNodes(nodes []string, allocated map[string]bool) []string {
	var remaining []string
	for _, node := range nodes {
		if !allocated[node] {
			remaining = append(remaining, node)
		}
	}
	return remaining
}

func (p *Partitioner) groupByDependencyDepth(nodes []string, graph *types.DependencyGraph) map[int][]string {
	groups := make(map[int][]string)
	for _, node := range nodes {
		depth := p.calculateDependencyDepth(node, graph, make(map[string]bool))
		groups[depth] = append(groups[depth], node)
	}
	return groups
}

func (p *Partitioner) calculateDependencyDepth(node string, graph *types.DependencyGraph, visiting map[string]bool) int {
	if visiting[node] {
		return 0 // Circular dependency
	}

	if depth, exists := p.depthCache[node]; exists {
		return depth
	}

	visiting[node] = true
	maxDepth := 0

	for _, dep := range graph.Adjacency[node] {
		depth := 1 + p.calculateDependencyDepth(dep, graph, visiting)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	visiting[node] = false
	if p.depthCache == nil {
		p.depthCache = make(map[string]int)
	}
	p.depthCache[node] = maxDepth

	return maxDepth
}

func (p *Partitioner) calculateDependencies(filePaths []string, existingPartitions []types.Partition) []int {
	// Simplified dependency calculation - can be enhanced
	return []int{}
}

func (p *Partitioner) generateName(files []types.FileChange) string {
	namer := NewPartitionNamer()
	return namer.GenerateName(files)
}

func (p *Partitioner) generateDescription(files []types.FileChange) string {
	namer := NewPartitionNamer()
	return namer.GenerateDescription(files)
}
