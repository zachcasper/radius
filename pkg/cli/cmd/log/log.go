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

package log

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/git/commit"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// NewCommand creates the rad log command for Git workspace mode.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show Radius action history",
		Long: `Show the history of Radius actions (init, plan, deploy, delete) in the Git repository.

This command filters git log to show only commits with Radius trailers, providing
a summarized view of Radius actions and the artifacts they created or modified.`,
		Example: `# Show all Radius actions
rad log

# Show last 5 Radius actions
rad log -n 5

# Filter by application
rad log --application myapp

# Filter by action type
rad log --action deploy

# Show detailed artifact information
rad log --verbose

# Output as JSON
rad log --output json`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().IntP("number", "n", 10, "Number of entries to show")
	cmd.Flags().String("action", "", "Filter by action type (init, plan, deploy, delete)")
	cmd.Flags().BoolP("verbose", "v", false, "Show detailed artifact information")
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad log` command.
type Runner struct {
	Output output.Interface

	// Options
	RepoRoot    string
	Number      int
	Action      string
	Application string
	Environment string
	Verbose     bool
	Format      string
}

// NewRunner creates a new instance of the `rad log` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Output: factory.GetOutput(),
	}
}

// Validate validates the command arguments.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	r.Number, _ = cmd.Flags().GetInt("number")
	r.Action, _ = cmd.Flags().GetString("action")
	r.Verbose, _ = cmd.Flags().GetBool("verbose")
	r.Application, _ = cmd.Flags().GetString("application")
	r.Environment, _ = cmd.Flags().GetString("environment")
	r.Format, _ = cmd.Flags().GetString("output")

	// Validate action if provided
	if r.Action != "" {
		validActions := []string{"init", "plan", "deploy", "delete"}
		valid := false
		for _, a := range validActions {
			if r.Action == a {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid action '%s': must be one of init, plan, deploy, delete", r.Action)
		}
	}

	// Get repository root
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	repoRoot, err := getRepositoryRoot(wd)
	if err != nil {
		return err
	}
	r.RepoRoot = repoRoot

	return nil
}

// getRepositoryRoot finds the git repository root.
func getRepositoryRoot(wd string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = wd
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a Git repository")
	}
	return strings.TrimSpace(string(output)), nil
}

// LogEntry represents a single Radius action log entry.
type LogEntry struct {
	CommitHash  string    `json:"commitHash"`
	CommitShort string    `json:"commitShort"`
	Date        time.Time `json:"date"`
	Author      string    `json:"author"`
	Message     string    `json:"message"`
	Action      string    `json:"action"`
	Application string    `json:"application,omitempty"`
	Environment string    `json:"environment,omitempty"`
	Artifacts   []string  `json:"artifacts,omitempty"`
	Deleted     []string  `json:"deleted,omitempty"`
}

// Run executes the `rad log` command.
func (r *Runner) Run(ctx context.Context) error {
	entries, err := r.getRadiusCommits()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		r.Output.LogInfo("No Radius actions found in the repository.")
		return nil
	}

	// Output based on format
	if r.Format == "json" {
		return r.outputJSON(entries)
	}

	return r.outputTable(entries)
}

// getRadiusCommits retrieves commits with Radius trailers.
func (r *Runner) getRadiusCommits() ([]LogEntry, error) {
	// Use git log with grep to find commits with Radius-Action trailer
	// Format: hash|date|author|subject
	cmd := exec.Command("git", "log",
		"--grep=Radius-Action:",
		fmt.Sprintf("-n%d", r.Number*3), // Get more to account for filtering
		"--format=%H|%aI|%an|%s",
	)
	cmd.Dir = r.RepoRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil, nil
	}

	var entries []LogEntry
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}

		hash := parts[0]
		dateStr := parts[1]
		author := parts[2]
		subject := parts[3]

		// Parse the date
		date, _ := time.Parse(time.RFC3339, dateStr)

		// Get trailers for this commit
		trailers, err := commit.ParseTrailers(hash, r.RepoRoot)
		if err != nil {
			continue
		}

		action := trailers[commit.TrailerAction]
		app := trailers[commit.TrailerApplication]
		env := trailers[commit.TrailerEnvironment]

		// Apply filters
		if r.Action != "" && action != r.Action {
			continue
		}
		if r.Application != "" && app != r.Application {
			continue
		}
		if r.Environment != "" && env != r.Environment {
			continue
		}

		entry := LogEntry{
			CommitHash:  hash,
			CommitShort: hash[:8],
			Date:        date,
			Author:      author,
			Message:     subject,
			Action:      action,
			Application: app,
			Environment: env,
		}

		// Get artifacts created/deleted in this commit
		if r.Verbose {
			entry.Artifacts, entry.Deleted = r.getArtifacts(hash)
		}

		entries = append(entries, entry)

		// Stop when we have enough entries
		if len(entries) >= r.Number {
			break
		}
	}

	return entries, nil
}

// getArtifacts returns the artifacts created and deleted in a commit.
func (r *Runner) getArtifacts(commitHash string) ([]string, []string) {
	// Get the diff for this commit showing only .radius/ files
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "--name-status", "-r", commitHash, "--", ".radius/")
	cmd.Dir = r.RepoRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	var added, deleted []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		path := parts[1]

		// Summarize the artifact type
		artifactType := r.summarizeArtifact(path)
		if artifactType == "" {
			continue
		}

		switch status[0] {
		case 'A': // Added
			added = append(added, artifactType)
		case 'D': // Deleted
			deleted = append(deleted, artifactType)
		case 'M': // Modified
			added = append(added, artifactType+" (updated)")
		}
	}

	// Deduplicate
	added = uniqueStrings(added)
	deleted = uniqueStrings(deleted)

	return added, deleted
}

// summarizeArtifact returns a human-readable description of the artifact.
func (r *Runner) summarizeArtifact(path string) string {
	// Parse the path to understand the artifact type
	// Examples:
	//   .radius/config/recipes/recipes.yaml -> Recipes configuration
	//   .radius/plan/todolist/default/plan.yaml -> Plan manifest
	//   .radius/plan/todolist/default/01-db/main.tf -> Terraform module
	//   .radius/deploy/todolist/default/deployment-abc123.json -> Deployment record

	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) < 2 {
		return ""
	}

	switch parts[1] {
	case "config":
		if len(parts) >= 4 && parts[2] == "recipes" {
			return "recipes.yaml"
		}
		return filepath.Base(path)

	case "model":
		return "Resource Types"

	case "plan":
		if len(parts) >= 4 {
			app := parts[2]
			env := parts[3]
			if len(parts) == 5 && parts[4] == "plan.yaml" {
				return fmt.Sprintf("plan.yaml (%s/%s)", app, env)
			}
			if len(parts) >= 5 {
				// Step directory
				stepDir := parts[4]
				if len(parts) >= 6 {
					filename := parts[len(parts)-1]
					switch {
					case strings.HasSuffix(filename, ".tf"):
						return fmt.Sprintf("Terraform: %s/%s/%s", app, env, stepDir)
					case strings.HasSuffix(filename, ".bicep"):
						return fmt.Sprintf("Bicep: %s/%s/%s", app, env, stepDir)
					case strings.HasSuffix(filename, ".tfvars"):
						return fmt.Sprintf("Terraform vars: %s/%s/%s", app, env, stepDir)
					case filename == "terraform-context.txt":
						return fmt.Sprintf("Terraform context: %s/%s/%s", app, env, stepDir)
					case filename == "bicep-context.txt":
						return fmt.Sprintf("Bicep context: %s/%s/%s", app, env, stepDir)
					}
				}
			}
		}
		return filepath.Base(path)

	case "deploy":
		if len(parts) >= 5 {
			app := parts[2]
			env := parts[3]
			filename := parts[len(parts)-1]
			if strings.HasPrefix(filename, "deployment-") {
				return fmt.Sprintf("Deployment record: %s/%s", app, env)
			}
		}
		return filepath.Base(path)

	default:
		return filepath.Base(path)
	}
}

// outputJSON outputs the entries as JSON.
func (r *Runner) outputJSON(entries []LogEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// outputTable outputs the entries in a human-readable table format.
func (r *Runner) outputTable(entries []LogEntry) error {
	for i, entry := range entries {
		// Color and format based on action
		actionIcon := r.getActionIcon(entry.Action)
		actionColor := r.getActionColor(entry.Action)

		// Header line
		fmt.Printf("%s%s %s%s %s\n",
			actionColor,
			actionIcon,
			entry.CommitShort,
			"\033[0m", // Reset color
			entry.Message,
		)

		// Details line
		details := []string{}
		if entry.Application != "" {
			details = append(details, fmt.Sprintf("app: %s", entry.Application))
		}
		if entry.Environment != "" {
			details = append(details, fmt.Sprintf("env: %s", entry.Environment))
		}
		details = append(details, fmt.Sprintf("date: %s", entry.Date.Format("2006-01-02 15:04")))
		details = append(details, fmt.Sprintf("author: %s", entry.Author))

		fmt.Printf("   %s\n", strings.Join(details, " | "))

		// Artifacts (if verbose)
		if r.Verbose && len(entry.Artifacts) > 0 {
			fmt.Printf("   \033[32m+ %s\033[0m\n", strings.Join(entry.Artifacts, ", "))
		}
		if r.Verbose && len(entry.Deleted) > 0 {
			fmt.Printf("   \033[31m- %s\033[0m\n", strings.Join(entry.Deleted, ", "))
		}

		// Add blank line between entries (except last)
		if i < len(entries)-1 {
			fmt.Println()
		}
	}

	return nil
}

// getActionIcon returns an icon for the action type.
func (r *Runner) getActionIcon(action string) string {
	switch action {
	case "init":
		return "🚀"
	case "plan":
		return "📋"
	case "deploy":
		return "🎯"
	case "delete":
		return "🗑️ "
	default:
		return "📌"
	}
}

// getActionColor returns ANSI color code for the action type.
func (r *Runner) getActionColor(action string) string {
	switch action {
	case "init":
		return "\033[36m" // Cyan
	case "plan":
		return "\033[33m" // Yellow
	case "deploy":
		return "\033[32m" // Green
	case "delete":
		return "\033[31m" // Red
	default:
		return "\033[0m" // Reset
	}
}

// uniqueStrings returns unique strings from a slice.
func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
