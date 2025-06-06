package types

import "time"

// FileChange represents a single file change from git diff
type FileChange struct {
	Path         string     `json:"path"`
	ChangeType   ChangeType `json:"changeType"`
	Content      string     `json:"content"`
	LinesAdded   int        `json:"linesAdded"`
	LinesDeleted int        `json:"linesDeleted"`
	IsChanged    bool       `json:"isChanged"`
	OldPath      string     `json:"oldPath,omitempty"` // For renames
}

// ChangeType represents the type of change made to a file
type ChangeType string

const (
	ChangeTypeAdd    ChangeType = "ADD"
	ChangeTypeModify ChangeType = "MODIFY"
	ChangeTypeDelete ChangeType = "DELETE"
	ChangeTypeRename ChangeType = "RENAME"
)

// Dependency represents a relationship between two files
type Dependency struct {
	From     string             `json:"from"`
	To       string             `json:"to"`
	Type     string             `json:"type"`
	Strength DependencyStrength `json:"strength"`
	Line     int                `json:"line,omitempty"`    // Line number where dependency occurs
	Context  string             `json:"context,omitempty"` // Code context around dependency
}

// DependencyStrength represents how strong a dependency is
type DependencyStrength string

const (
	StrengthCritical DependencyStrength = "CRITICAL" // import/export - breaks compilation
	StrengthStrong   DependencyStrength = "STRONG"   // function calls - breaks runtime
	StrengthModerate DependencyStrength = "MODERATE" // type references - breaks features
	StrengthWeak     DependencyStrength = "WEAK"     // similar patterns - reduces quality
	StrengthCircular DependencyStrength = "CIRCULAR" // mutual dependencies
)

// PluginInput represents the input sent to plugins
type PluginInput struct {
	ChangedFiles []FileChange `json:"changedFiles"`
	ProjectFiles []FileChange `json:"projectFiles"`
	ProjectRoot  string       `json:"projectRoot"`
}

// PluginOutput represents the output from plugins
type PluginOutput struct {
	Dependencies []Dependency   `json:"dependencies"`
	Metadata     PluginMetadata `json:"metadata"`
	Errors       []string       `json:"errors"`
}

// PluginMetadata contains information about the plugin analysis
type PluginMetadata struct {
	FilesAnalyzed int    `json:"filesAnalyzed"`
	AnalysisTime  string `json:"analysisTime"`
	PluginName    string `json:"pluginName"`
	PluginVersion string `json:"pluginVersion"`
}

// Partition represents a group of files that should go together
type Partition struct {
	ID           int          `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Files        []FileChange `json:"files"`
	Dependencies []int        `json:"dependencies"` // IDs of partitions this depends on
	BranchName   string       `json:"branchName"`
}

// PartitionPlan represents the complete partitioning strategy
type PartitionPlan struct {
	Partitions []Partition  `json:"partitions"`
	Metadata   PlanMetadata `json:"metadata"`
}

// PlanMetadata contains information about the partitioning plan
type PlanMetadata struct {
	TotalFiles           int       `json:"totalFiles"`
	TotalPartitions      int       `json:"totalPartitions"`
	MaxFilesPerPartition int       `json:"maxFilesPerPartition"`
	Strategy             string    `json:"strategy"`
	CreatedAt            time.Time `json:"createdAt"`
}

// SplitResult represents the final result of the splitting operation
type SplitResult struct {
	SourceBranch      string             `json:"sourceBranch"`
	TargetBranch      string             `json:"targetBranch"`
	Partitions        []Partition        `json:"partitions"`
	CreatedBranches   []string           `json:"createdBranches"`
	ValidationResults []ValidationResult `json:"validationResults"`
	Config            Config             `json:"config"`
}

// ValidationResult represents the result of a validation check
type ValidationResult struct {
	Type    ValidationType   `json:"type"`
	Status  ValidationStatus `json:"status"`
	Message string           `json:"message"`
	Details interface{}      `json:"details,omitempty"`
}

// ValidationType represents different types of validation
type ValidationType string

const (
	ValidationStructural     ValidationType = "STRUCTURAL"
	ValidationDependency     ValidationType = "DEPENDENCY"
	ValidationGitIntegrity   ValidationType = "GIT_INTEGRITY"
	ValidationDiffComparison ValidationType = "DIFF_COMPARISON"
)

// ValidationStatus represents the status of a validation check
type ValidationStatus string

const (
	ValidationStatusPass ValidationStatus = "PASS"
	ValidationStatusWarn ValidationStatus = "WARN"
	ValidationStatusFail ValidationStatus = "FAIL"
)

// Config represents the configuration for the splitting operation
type Config struct {
	MaxFilesPerPartition int    `json:"maxFilesPerPartition"`
	MaxPartitions        int    `json:"maxPartitions"`
	BranchPrefix         string `json:"branchPrefix"`
	Strategy             string `json:"strategy"`
	TargetBranch         string `json:"targetBranch"`
}

// StronglyConnectedComponent represents a group of files with circular dependencies
type StronglyConnectedComponent struct {
	Files []string `json:"files"`
	Size  int      `json:"size"`
}

// DependencyGraph represents the complete dependency relationship between files
type DependencyGraph struct {
	Nodes     []string                     `json:"nodes"`     // All file paths
	Edges     []Dependency                 `json:"edges"`     // All dependencies
	SCCs      []StronglyConnectedComponent `json:"sccs"`      // Circular dependency groups
	Adjacency map[string][]string          `json:"adjacency"` // Adjacency list representation
	InDegree  map[string]int               `json:"inDegree"`  // Number of incoming dependencies
	OutDegree map[string]int               `json:"outDegree"` // Number of outgoing dependencies
}
