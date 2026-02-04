// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

package commit

import (
	"fmt"
	"os/exec"
	"strings"
)

// Action represents a Radius action type for git trailers.
type Action string

const (
	// ActionInit represents the rad init command.
	ActionInit Action = "init"
	// ActionPlan represents the rad plan command.
	ActionPlan Action = "plan"
	// ActionDeploy represents the rad deploy command.
	ActionDeploy Action = "deploy"
	// ActionDelete represents the rad delete command.
	ActionDelete Action = "delete"
)

// Trailer keys for git commit trailers.
const (
	TrailerAction      = "Radius-Action"
	TrailerApplication = "Radius-Application"
	TrailerEnvironment = "Radius-Environment"
)

// CommitOptions contains options for creating a Radius commit.
type CommitOptions struct {
	// RepoRoot is the root directory of the git repository.
	RepoRoot string

	// Action is the Radius action being performed.
	Action Action

	// Application is the application name (optional for init).
	Application string

	// Environment is the environment name (optional for init).
	Environment string

	// Message is a custom commit message (optional).
	// If not provided, a default message will be generated.
	Message string

	// FilesToAdd are specific files/directories to stage.
	// If empty, stages all changes in .radius/
	FilesToAdd []string
}

// AutoCommit performs a git commit with Radius trailers after a Radius action.
func AutoCommit(opts *CommitOptions) error {
	// Stage files
	if err := stageFiles(opts); err != nil {
		return fmt.Errorf("failed to stage files: %w", err)
	}

	// Check if there are staged changes
	hasChanges, err := hasStagedChanges(opts.RepoRoot)
	if err != nil {
		return fmt.Errorf("failed to check staged changes: %w", err)
	}
	if !hasChanges {
		// Nothing to commit
		return nil
	}

	// Build commit message with trailers
	message := buildCommitMessage(opts)

	// Execute git commit
	if err := gitCommit(opts.RepoRoot, message); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// stageFiles stages the appropriate files for the commit.
func stageFiles(opts *CommitOptions) error {
	var files []string
	if len(opts.FilesToAdd) > 0 {
		files = opts.FilesToAdd
	} else {
		// Default: stage .radius/ directory
		files = []string{".radius/"}
	}

	for _, f := range files {
		cmd := exec.Command("git", "add", f)
		cmd.Dir = opts.RepoRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git add %s failed: %s", f, string(output))
		}
	}

	return nil
}

// hasStagedChanges checks if there are staged changes to commit.
func hasStagedChanges(repoRoot string) (bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = repoRoot
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means there are differences (changes staged)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, err
	}
	// Exit code 0 means no differences
	return false, nil
}

// buildCommitMessage builds the commit message with trailers.
func buildCommitMessage(opts *CommitOptions) string {
	var sb strings.Builder

	// Subject line
	if opts.Message != "" {
		sb.WriteString(opts.Message)
	} else {
		sb.WriteString(generateDefaultMessage(opts))
	}

	// Blank line before trailers
	sb.WriteString("\n\n")

	// Add trailers
	sb.WriteString(fmt.Sprintf("%s: %s\n", TrailerAction, opts.Action))

	if opts.Application != "" {
		sb.WriteString(fmt.Sprintf("%s: %s\n", TrailerApplication, opts.Application))
	}

	if opts.Environment != "" {
		sb.WriteString(fmt.Sprintf("%s: %s\n", TrailerEnvironment, opts.Environment))
	}

	return sb.String()
}

// generateDefaultMessage generates a default commit message based on the action.
func generateDefaultMessage(opts *CommitOptions) string {
	switch opts.Action {
	case ActionInit:
		return "Initialize Radius workspace"
	case ActionPlan:
		if opts.Application != "" && opts.Environment != "" {
			return fmt.Sprintf("Plan deployment of %s to %s", opts.Application, opts.Environment)
		}
		return "Generate deployment plan"
	case ActionDeploy:
		if opts.Application != "" && opts.Environment != "" {
			return fmt.Sprintf("Deploy %s to %s", opts.Application, opts.Environment)
		}
		return "Record deployment"
	case ActionDelete:
		if opts.Application != "" && opts.Environment != "" {
			return fmt.Sprintf("Delete %s from %s", opts.Application, opts.Environment)
		}
		return "Record deletion"
	default:
		return "Radius action"
	}
}

// gitCommit executes the git commit command.
func gitCommit(repoRoot, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit failed: %s", string(output))
	}
	return nil
}

// GetLastRadiusCommit returns the most recent commit with Radius trailers.
func GetLastRadiusCommit(repoRoot string) (string, error) {
	cmd := exec.Command("git", "log", "--oneline", "-1", "--grep=Radius-Action:")
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// ParseTrailers extracts Radius trailers from a commit message.
func ParseTrailers(commitHash, repoRoot string) (map[string]string, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%B", commitHash)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	trailers := make(map[string]string)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Radius-") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				trailers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	return trailers, nil
}
