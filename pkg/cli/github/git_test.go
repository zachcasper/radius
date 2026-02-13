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
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitHelper(t *testing.T) {
	t.Parallel()

	t.Run("valid git repo", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)

		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)
		assert.NotNil(t, helper)
	})

	t.Run("not a git repo", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()

		_, err := NewGitHelper(tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "repository does not exist")
	})
}

func TestGitHelper_GetCurrentBranch(t *testing.T) {
	t.Parallel()

	t.Run("returns branch name", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		branch, err := helper.GetCurrentBranch()
		require.NoError(t, err)
		assert.Equal(t, "main", branch)
	})
}

func TestGitHelper_CreateBranch(t *testing.T) {
	t.Parallel()

	t.Run("creates new branch", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		err = helper.CreateBranch("feature/test-branch")
		require.NoError(t, err)

		branch, err := helper.GetCurrentBranch()
		require.NoError(t, err)
		assert.Equal(t, "feature/test-branch", branch)
	})

	t.Run("errors on existing branch", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		err = helper.CreateBranch("main")
		require.Error(t, err)
	})

	t.Run("errors on invalid branch name", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		err = helper.CreateBranch("")
		require.Error(t, err)
	})
}

func TestGitHelper_CheckoutBranch(t *testing.T) {
	t.Parallel()

	t.Run("switches to existing branch", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		// Create a branch first
		err = helper.CreateBranch("feature/test")
		require.NoError(t, err)

		// Checkout main
		err = helper.CheckoutBranch("main")
		require.NoError(t, err)

		branch, err := helper.GetCurrentBranch()
		require.NoError(t, err)
		assert.Equal(t, "main", branch)

		// Checkout feature
		err = helper.CheckoutBranch("feature/test")
		require.NoError(t, err)

		branch, err = helper.GetCurrentBranch()
		require.NoError(t, err)
		assert.Equal(t, "feature/test", branch)
	})

	t.Run("errors on non-existent branch", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		err = helper.CheckoutBranch("non-existent")
		require.Error(t, err)
	})
}

func TestGitHelper_Add(t *testing.T) {
	t.Parallel()

	t.Run("stages file", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		// Create a new file
		testFile := filepath.Join(repoPath, "testfile.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		// Stage it
		err = helper.Add("testfile.txt")
		require.NoError(t, err)

		// Verify it's staged
		status, err := helper.repo.Worktree()
		require.NoError(t, err)
		st, err := status.Status()
		require.NoError(t, err)
		assert.Equal(t, git.Added, st["testfile.txt"].Staging)
	})
}

func TestGitHelper_AddAll(t *testing.T) {
	t.Parallel()

	t.Run("stages all changes", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		// Create multiple files
		for _, name := range []string{"file1.txt", "file2.txt"} {
			path := filepath.Join(repoPath, name)
			err = os.WriteFile(path, []byte("content"), 0644)
			require.NoError(t, err)
		}

		// Stage all
		err = helper.AddAll()
		require.NoError(t, err)

		// Verify
		wt, err := helper.repo.Worktree()
		require.NoError(t, err)
		st, err := wt.Status()
		require.NoError(t, err)
		assert.Equal(t, git.Added, st["file1.txt"].Staging)
		assert.Equal(t, git.Added, st["file2.txt"].Staging)
	})
}

func TestGitHelper_CommitWithTrailer(t *testing.T) {
	t.Parallel()

	t.Run("creates commit without trailer", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		// Create and stage a file
		testFile := filepath.Join(repoPath, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0644)
		require.NoError(t, err)
		err = helper.Add("test.txt")
		require.NoError(t, err)

		// Commit without trailer
		hash, err := helper.CommitWithTrailer("test commit message", "")
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		// Verify commit
		commit, err := helper.repo.CommitObject(plumbing.NewHash(hash))
		require.NoError(t, err)
		assert.Equal(t, "test commit message", commit.Message)
	})

	t.Run("creates commit with trailer", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		// Create and stage a file
		testFile := filepath.Join(repoPath, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0644)
		require.NoError(t, err)
		err = helper.Add("test.txt")
		require.NoError(t, err)

		// Commit with trailer
		hash, err := helper.CommitWithTrailer("test message", "deploy")
		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		// Verify commit message includes trailer
		commit, err := helper.repo.CommitObject(plumbing.NewHash(hash))
		require.NoError(t, err)
		assert.Contains(t, commit.Message, "Radius-Action: deploy")
	})
}

func TestGitHelper_IsDirty(t *testing.T) {
	t.Parallel()

	t.Run("clean repo", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		isDirty, err := helper.IsDirty()
		require.NoError(t, err)
		assert.False(t, isDirty)
	})

	t.Run("dirty repo", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		// Create untracked file
		testFile := filepath.Join(repoPath, "untracked.txt")
		err = os.WriteFile(testFile, []byte("test"), 0644)
		require.NoError(t, err)

		isDirty, err := helper.IsDirty()
		require.NoError(t, err)
		assert.True(t, isDirty)
	})
}

func TestGitHelper_GetOwnerRepo(t *testing.T) {
	t.Parallel()

	t.Run("returns owner and repo from https origin", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepoWithRemote(t, "https://github.com/radius-project/radius.git")
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		owner, repo, err := helper.GetOwnerRepo()
		require.NoError(t, err)
		assert.Equal(t, "radius-project", owner)
		assert.Equal(t, "radius", repo)
	})

	t.Run("returns owner and repo from ssh origin", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepoWithRemote(t, "git@github.com:radius-project/radius.git")
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		owner, repo, err := helper.GetOwnerRepo()
		require.NoError(t, err)
		assert.Equal(t, "radius-project", owner)
		assert.Equal(t, "radius", repo)
	})

	t.Run("errors when no origin", func(t *testing.T) {
		t.Parallel()
		repoPath := createTestRepo(t)
		helper, err := NewGitHelper(repoPath)
		require.NoError(t, err)

		_, _, err = helper.GetOwnerRepo()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "origin remote")
	})
}

// Test helper functions

func createTestRepo(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Configure user
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	err = repo.SetConfig(cfg)
	require.NoError(t, err)

	// Create initial commit so we have a main branch
	wt, err := repo.Worktree()
	require.NoError(t, err)

	readmePath := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(readmePath, []byte("# Test Repo"), 0644)
	require.NoError(t, err)

	_, err = wt.Add("README.md")
	require.NoError(t, err)

	_, err = wt.Commit("Initial commit", &git.CommitOptions{})
	require.NoError(t, err)

	// Create main branch reference
	head, err := repo.Head()
	require.NoError(t, err)
	err = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), head.Hash()))
	require.NoError(t, err)
	err = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main")))
	require.NoError(t, err)

	return tmpDir
}

func createTestRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()
	repoPath := createTestRepo(t)

	repo, err := git.PlainOpen(repoPath)
	require.NoError(t, err)

	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	require.NoError(t, err)

	return repoPath
}
