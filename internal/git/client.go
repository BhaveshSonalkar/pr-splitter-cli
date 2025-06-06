package git

import (
	"os"
	"os/exec"
	"strings"

	"pr-splitter-cli/internal/types"
)

// Client handles git operations with focused responsibilities
type Client struct {
	workingDir string
	validator  *Validator
	differ     *Differ
	brancher   *Brancher
}

// NewClient creates a new git client with all sub-components
func NewClient() *Client {
	wd, _ := os.Getwd()
	validator := NewValidator(wd)
	differ := NewDiffer(wd)
	brancher := NewBrancher(wd)

	return &Client{
		workingDir: wd,
		validator:  validator,
		differ:     differ,
		brancher:   brancher,
	}
}

// ValidateGitRepository checks if we're in a valid git repository
func (c *Client) ValidateGitRepository() error {
	return c.validator.ValidateRepository()
}

// ValidateBranches validates that source and target branches exist
func (c *Client) ValidateBranches(sourceBranch, targetBranch string) error {
	return c.validator.ValidateBranches(sourceBranch, targetBranch)
}

// GetChanges analyzes git changes between source and target branches
func (c *Client) GetChanges(sourceBranch, targetBranch string) ([]types.FileChange, error) {
	if err := c.ValidateGitRepository(); err != nil {
		return nil, err
	}

	if err := c.ValidateBranches(sourceBranch, targetBranch); err != nil {
		return nil, err
	}

	return c.differ.GetChanges(sourceBranch, targetBranch)
}

// CreateBranches creates branches for each partition
func (c *Client) CreateBranches(plan *types.PartitionPlan, cfg *types.Config, sourceBranch string) ([]string, error) {
	return c.brancher.CreateBranches(plan, cfg, sourceBranch)
}

// Utility methods for external access
func (c *Client) GetCurrentBranch() (string, error) {
	return c.brancher.GetCurrentBranch()
}

func (c *Client) CheckoutBranch(branchName string) error {
	return c.brancher.CheckoutBranch(branchName)
}

func (c *Client) DeleteLocalBranch(branchName string) error {
	return c.brancher.DeleteLocalBranch(branchName)
}

func (c *Client) DeleteRemoteBranch(branchName string) error {
	return c.brancher.DeleteRemoteBranch(branchName)
}

func (c *Client) GetLocalBranches() ([]string, error) {
	return c.brancher.GetLocalBranches()
}

func (c *Client) GetRemoteBranches() ([]string, error) {
	return c.brancher.GetRemoteBranches()
}

// runGitCommand executes a git command and returns output
func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// runGitCommandQuiet executes a git command without capturing output
func runGitCommandQuiet(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}
