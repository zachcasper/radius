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

package diff

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli/framework"
	gitdiff "github.com/radius-project/radius/pkg/cli/git/diff"
	"github.com/radius-project/radius/pkg/cli/output"
)

// NewCommand creates the `rad diff` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "diff [commit1...commit2]",
		Short: "Compare infrastructure states between commits or with live state",
		Long: `The diff command compares infrastructure definitions and deployment states.

By default (no arguments), diff shows changes in the current working directory
compared to the last commit (uncommitted changes).

With a single commit reference, diff compares that commit against live infrastructure.

With two commits separated by '...', diff compares those two commits.

Use --live to compare any state against currently deployed infrastructure.

Examples:
  # Show uncommitted changes
  rad diff

  # Compare a commit against live infrastructure
  rad diff abc123 --live

  # Compare two commits
  rad diff abc123...def456

  # Show diff in JSON format
  rad diff --output json

  # Show diff for all environments
  rad diff --all-environments`,
		Example: `# Show uncommitted changes in the current directory
rad diff

# Compare HEAD against live infrastructure
rad diff HEAD --live

# Compare two specific commits
rad diff abc123...def456

# Compare with previous commit
rad diff HEAD~1...HEAD

# Show diff as JSON
rad diff --output json

# Compare all environments
rad diff --all-environments`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	// Add flags
	cmd.Flags().BoolP("all-environments", "a", false, "Show diff for all environments")
	cmd.Flags().StringP("commit", "c", "", "Commit to compare (defaults to HEAD)")
	cmd.Flags().StringP("environment", "e", "", "The environment name")
	cmd.Flags().BoolP("live", "l", false, "Compare against live deployed infrastructure")
	cmd.Flags().StringP("output", "o", "text", "Output format (text, json)")
	cmd.Flags().StringSliceP("resource-type", "r", nil, "Filter by resource type(s)")
	cmd.Flags().BoolP("show-details", "d", false, "Show detailed property-level diffs")
	cmd.Flags().Bool("show-unchanged", false, "Include unchanged resources in output")
	cmd.Flags().StringP("target", "t", "", "Target commit to compare against")
	cmd.Flags().StringP("workspace", "w", "", "The workspace name")

	return cmd, runner
}

// Runner implements the rad diff command.
type Runner struct {
	factory framework.Factory
	Output  output.Interface

	// Source is the source commit or "live".
	Source string

	// Target is the target commit or "live".
	Target string

	// Commit is the commit to compare.
	Commit string

	// Environment is the environment name.
	Environment string

	// AllEnvironments indicates to diff all environments.
	AllEnvironments bool

	// Live indicates to compare against live infrastructure.
	Live bool

	// OutputFormat is the output format.
	OutputFormat string

	// ResourceTypes is the list of resource types to filter by.
	ResourceTypes []string

	// ShowDetails indicates to show detailed diffs.
	ShowDetails bool

	// ShowUnchanged indicates to show unchanged resources.
	ShowUnchanged bool

	// TargetCommit is the target commit to compare against.
	TargetCommit string

	// WorkDir is the working directory.
	WorkDir string
}

// NewRunner creates a new Runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		factory: factory,
	}
}

// Validate validates the command arguments and flags.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	r.Output = r.factory.GetOutput()
	r.AllEnvironments, _ = cmd.Flags().GetBool("all-environments")
	r.Commit, _ = cmd.Flags().GetString("commit")
	r.Environment, _ = cmd.Flags().GetString("environment")
	r.Live, _ = cmd.Flags().GetBool("live")
	r.OutputFormat, _ = cmd.Flags().GetString("output")
	r.ResourceTypes, _ = cmd.Flags().GetStringSlice("resource-type")
	r.ShowDetails, _ = cmd.Flags().GetBool("show-details")
	r.ShowUnchanged, _ = cmd.Flags().GetBool("show-unchanged")
	r.TargetCommit, _ = cmd.Flags().GetString("target")

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	r.WorkDir = workDir

	// Check if this is a Radius Git workspace
	radiusDir := filepath.Join(workDir, ".radius")
	if _, err := os.Stat(radiusDir); os.IsNotExist(err) {
		return &diffExitError{
			message:  "Not in a Radius Git workspace. Run 'rad init' first.",
			exitCode: gitdiff.ExitCodeValidationError,
		}
	}

	// Parse arguments
	if len(args) == 0 {
		// Default: uncommitted changes
		r.Source = "HEAD"
		r.Target = ""
	} else {
		arg := args[0]
		if strings.Contains(arg, "...") {
			parts := strings.Split(arg, "...")
			if len(parts) != 2 {
				return &diffExitError{
					message:  "Invalid comparison format. Use: <source>...<target>",
					exitCode: gitdiff.ExitCodeValidationError,
				}
			}
			r.Source = parts[0]
			r.Target = parts[1]
		} else {
			// Single commit - compare against HEAD
			r.Source = arg
			r.Target = "HEAD"
		}
	}

	return nil
}

// Run executes the diff command.
func (r *Runner) Run(ctx context.Context) error {
	differ := gitdiff.NewDiffer(r.WorkDir).
		WithEnvironment(r.Environment)

	// Handle source and target comparison
	var result *gitdiff.DiffResult
	var err error

	// Determine diff mode and print header
	if r.Target == "" {
		// Uncommitted changes
		r.Output.LogInfo("Comparing uncommitted changes against HEAD...")
		result, err = differ.DiffUncommitted(ctx)
	} else if r.Target == "live" {
		// Drift detection
		r.Output.LogInfo("Comparing %s against live infrastructure...", r.Source)
		result, err = differ.DiffCommitToLive(ctx, r.Source)
	} else {
		// Two commits
		r.Output.LogInfo("Comparing %s against %s...", r.Source, r.Target)
		result, err = differ.DiffTwoCommits(ctx, r.Source, r.Target)
	}

	if err != nil {
		return &diffExitError{
			message:  fmt.Sprintf("Diff failed: %v", err),
			exitCode: gitdiff.ExitCodeValidationError,
		}
	}

	// Output results
	if r.OutputFormat == "json" {
		return r.outputJSON(result)
	}

	return r.outputHuman(result)
}

// outputJSON outputs the result as JSON.
func (r *Runner) outputJSON(result *gitdiff.DiffResult) error {
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	r.Output.LogInfo("%s", string(jsonBytes))

	if result.HasDiff {
		os.Exit(gitdiff.ExitCodeDiffFound)
	}
	return nil
}

// outputHuman outputs the result in human-readable format.
func (r *Runner) outputHuman(result *gitdiff.DiffResult) error {
	// Print comparison header
	source := result.Source
	target := result.Target
	if target == "working directory" {
		source = "commit"
		target = "uncommitted"
	}
	r.Output.LogInfo("Diff: %s -> %s", source, target)
	r.Output.LogInfo("Application: %s, Environment: %s", result.Application, result.Environment)
	r.Output.LogInfo("")

	if !result.HasDiff {
		r.Output.LogInfo("No changes detected.")
		return nil
	}

	// Show plan differences
	if len(result.PlanDiffs) > 0 {
		r.Output.LogInfo("📋 Plan Changes:")
		for _, diff := range result.PlanDiffs {
			symbol := r.getChangeSymbol(diff.Change)
			r.Output.LogInfo("   %s %s", symbol, diff.Path)
		}
		r.Output.LogInfo("")
	}

	// Show deployment differences
	if len(result.DeploymentDiffs) > 0 {
		r.Output.LogInfo("🎯 Deployment Record Changes:")
		for _, diff := range result.DeploymentDiffs {
			symbol := r.getChangeSymbol(diff.Change)
			r.Output.LogInfo("   %s %s", symbol, diff.Path)
		}
		r.Output.LogInfo("")
	}

	// Show manifest differences (drift)
	if len(result.ManifestDiffs) > 0 {
		r.Output.LogInfo("☸️  Kubernetes Manifest Drift:")
		for _, diff := range result.ManifestDiffs {
			symbol := r.getChangeSymbol(diff.Change)
			r.Output.LogInfo("   %s %s/%s (%s)", symbol, diff.Kind, diff.Name, diff.Namespace)
			if diff.DyffOutput != "" {
				lines := strings.Split(diff.DyffOutput, "\n")
				for _, line := range lines {
					if line != "" {
						r.Output.LogInfo("      %s", line)
					}
				}
			}
		}
		r.Output.LogInfo("")
	}

	// Show detailed dyff output if available
	if result.DyffOutput != "" {
		r.Output.LogInfo("Detailed Changes:")
		r.Output.LogInfo("%s", result.DyffOutput)
	}

	// Exit with code 1 to indicate differences found
	os.Exit(gitdiff.ExitCodeDiffFound)
	return nil
}

// getChangeSymbol returns a symbol for the change type.
func (r *Runner) getChangeSymbol(change string) string {
	switch change {
	case gitdiff.ChangeAdded:
		return "+"
	case gitdiff.ChangeRemoved:
		return "-"
	case gitdiff.ChangeModified:
		return "~"
	default:
		return "?"
	}
}

// diffExitError is a friendly error with an exit code.
type diffExitError struct {
	message  string
	exitCode int
}

func (e *diffExitError) Error() string {
	return e.message
}

func (e *diffExitError) IsFriendlyError() bool {
	return true
}
