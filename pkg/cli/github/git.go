/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package github

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitHelper provides git operations for Radius on GitHub mode.
type GitHelper struct {
	repoPath string
	repo     *git.Repository
}

// NewGitHelper creates a new GitHelper for the given repository path.
func NewGitHelper(repoPath string) (*GitHelper, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository at %s: %w", repoPath, err)
	}

	return &GitHelper{
		repoPath: repoPath,
		repo:     repo,
	}, nil
}

// IsGitRepository checks if the given path is a git repository.
func IsGitRepository(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	return err == nil && info.IsDir()
}

// GetRemoteURL returns the URL of the origin remote.
func (g *GitHelper) GetRemoteURL() (string, error) {
	remote, err := g.repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to get origin remote: %w", err)
	}

	if len(remote.Config().URLs) == 0 {
		return "", fmt.Errorf("origin remote has no URLs configured")
	}

	return remote.Config().URLs[0], nil
}

// IsGitHubRemote checks if the origin remote is a GitHub repository.
func (g *GitHelper) IsGitHubRemote() (bool, error) {
	url, err := g.GetRemoteURL()
	if err != nil {
		return false, err
	}

	return strings.Contains(url, "github.com"), nil
}

// GetOwnerRepo extracts the owner and repository name from the origin URL.
func (g *GitHelper) GetOwnerRepo() (owner, repo string, err error) {
	url, err := g.GetRemoteURL()
	if err != nil {
		return "", "", err
	}

	// Handle both HTTPS and SSH URLs
	// https://github.com/owner/repo.git
	// git@github.com:owner/repo.git

	url = strings.TrimSuffix(url, ".git")

	if strings.HasPrefix(url, "git@github.com:") {
		path := strings.TrimPrefix(url, "git@github.com:")
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return parts[0], parts[1], nil
		}
	}

	if strings.Contains(url, "github.com/") {
		idx := strings.Index(url, "github.com/")
		path := url[idx+len("github.com/"):]
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return parts[0], parts[1], nil
		}
	}

	return "", "", fmt.Errorf("could not parse owner/repo from URL: %s", url)
}

// Add stages a file for commit.
func (g *GitHelper) Add(path string) error {
	worktree, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	_, err = worktree.Add(path)
	if err != nil {
		return fmt.Errorf("failed to add %s: %w", path, err)
	}

	return nil
}

// AddAll stages all changes for commit.
func (g *GitHelper) AddAll() error {
	worktree, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.AddWithOptions(&git.AddOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to add all: %w", err)
	}

	return nil
}

// CommitWithTrailer creates a commit with the given message and Radius-Action trailer.
func (g *GitHelper) CommitWithTrailer(message string, action string) (string, error) {
	worktree, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Construct commit message with trailer
	fullMessage := message
	if action != "" {
		fullMessage = fmt.Sprintf("%s\n\nRadius-Action: %s", message, action)
	}

	commit, err := worktree.Commit(fullMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Radius",
			Email: "radius@radapp.io",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to commit: %w", err)
	}

	return commit.String(), nil
}

// GetCurrentBranch returns the name of the current branch.
func (g *GitHelper) GetCurrentBranch() (string, error) {
	head, err := g.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	if !head.Name().IsBranch() {
		return "", fmt.Errorf("HEAD is not a branch (detached HEAD state)")
	}

	return head.Name().Short(), nil
}

// GetCurrentCommit returns the current commit SHA.
func (g *GitHelper) GetCurrentCommit() (string, error) {
	head, err := g.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	return head.Hash().String(), nil
}

// GetCurrentCommitShort returns the short commit SHA (first 8 characters).
func (g *GitHelper) GetCurrentCommitShort() (string, error) {
	sha, err := g.GetCurrentCommit()
	if err != nil {
		return "", err
	}

	if len(sha) > 8 {
		return sha[:8], nil
	}
	return sha, nil
}

// IsDirty returns true if the working tree has uncommitted changes.
func (g *GitHelper) IsDirty() (bool, error) {
	worktree, err := g.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	return !status.IsClean(), nil
}

// CreateBranch creates a new branch from the current HEAD.
func (g *GitHelper) CreateBranch(name string) error {
	if name == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Check if branch already exists
	ref := plumbing.NewBranchReferenceName(name)
	_, err := g.repo.Reference(ref, true)
	if err == nil {
		return fmt.Errorf("branch %s already exists", name)
	}

	head, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	branchRef := plumbing.NewHashReference(ref, head.Hash())

	err = g.repo.Storer.SetReference(branchRef)
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", name, err)
	}

	// Checkout the new branch
	return g.CheckoutBranch(name)
}

// CheckoutBranch switches to the specified branch.
func (g *GitHelper) CheckoutBranch(name string) error {
	worktree, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
	})
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", name, err)
	}

	return nil
}

// Push pushes the current branch to the origin remote.
// Uses exec.Command to call git push since go-git requires auth setup.
func (g *GitHelper) Push() error {
	cmd := exec.Command("git", "push")
	cmd.Dir = g.repoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push failed: %s", stderr.String())
	}

	return nil
}

// GetGitContext returns the current git context for deployment records.
func (g *GitHelper) GetGitContext() (*GitContext, error) {
	commit, err := g.GetCurrentCommit()
	if err != nil {
		return nil, err
	}

	commitShort, err := g.GetCurrentCommitShort()
	if err != nil {
		return nil, err
	}

	branch, err := g.GetCurrentBranch()
	if err != nil {
		// Non-fatal error - might be in detached HEAD state
		branch = ""
	}

	isDirty, err := g.IsDirty()
	if err != nil {
		return nil, err
	}

	return &GitContext{
		Commit:      commit,
		CommitShort: commitShort,
		Branch:      branch,
		IsDirty:     isDirty,
	}, nil
}

// GitContext holds git state information for a deployment.
type GitContext struct {
	Commit      string `json:"commit"`
	CommitShort string `json:"commitShort"`
	Branch      string `json:"branch"`
	IsDirty     bool   `json:"isDirty"`
}
