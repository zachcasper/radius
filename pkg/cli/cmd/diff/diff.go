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

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	gitdiff "github.com/radius-project/radius/pkg/cli/git/diff"
	"github.com/radius-project/radius/pkg/cli/output"
)

// NewCommand creates the `rad diff` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "diff [source...target]",
		Short: "Show differences between commits, deployments, or live state",
		Long: `Show differences between commits, deployments, or live cloud state.

This command compares Radius artifacts (plans, deployments, manifests) between
different points in time or against live cloud state for drift detection.

Comparison modes:
  rad diff                    Compare uncommitted changes against HEAD
  rad diff <commit>           Compare <commit> against HEAD
  rad diff <source>...<target>  Compare two commits or "live" for drift detection

Exit codes:
  0  No differences found
  1  Differences found
  2  Validation error
  3  Authentication error

Examples:
  # Show uncommitted changes
  rad diff

  # Compare two commits
  rad diff abc123...def456

  # Drift detection - compare deployment to live state
  rad diff abc123...live -a myapp -e production

  # JSON output
  rad diff --output json`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	// Add flags
	commonflags.AddApplicationNameFlag(cmd)
	cmd.Flags().StringP("environment", "e", "", "Environment name")
	cmd.Flags().BoolP("all-environments", "A", false, "Diff all environments")
	cmd.Flags().Bool("plan-only", false, "Only diff plan.yaml files")
	commonflags.AddOutputFlag(cmd)

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

	// Application is the application name.
	Application string

	// Environment is the environment name.
	Environment string

	// AllEnvironments indicates to diff all environments.
	AllEnvironments bool

	// PlanOnly indicates to only diff plan files.
	PlanOnly bool

	// OutputFormat is the output format.
	OutputFormat string

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
	r.Application, _ = cmd.Flags().GetString("application")
	r.Environment, _ = cmd.Flags().GetString("environment")
	r.AllEnvironments, _ = cmd.Flags().GetBool("all-environments")
	r.PlanOnly, _ = cmd.Flags().GetBool("plan-only")
	r.OutputFormat, _ = cmd.Flags().GetString("output")

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	r.WorkDir = workDir

	// Validate this is a Git repository
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		return clierrors.Message("This command must be run from a Git repository root.")
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
				return clierrors.Message("Invalid comparison format. Use: <source>...<target>")
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
		WithApplication(r.Application).
		WithEnvironment(r.Environment)

	// Auto-discover application and environment if not provided
	if r.Application == "" || r.Environment == "" {
		sourceCommit := r.Source
		if sourceCommit == "" || sourceCommit == "HEAD" {
			sourceCommit = "HEAD"
		}

		app, env, err := differ.DiscoverAppAndEnv(ctx, sourceCommit)
		if err == nil {
			if r.Application == "" {
				r.Application = app
				differ.WithApplication(app)
			}
			if r.Environment == "" {
				r.Environment = env
				differ.WithEnvironment(env)
			}
		}
	}

	var result *gitdiff.DiffResult
	var err error

	// Determine diff mode
	if r.Target == "" {
		// Uncommitted changes
		result, err = differ.DiffUncommitted(ctx)
	} else if r.Target == "live" {
		// Drift detection
		result, err = differ.DiffCommitToLive(ctx, r.Source)
	} else {
		// Two commits
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
	if !result.HasDiff {
		r.Output.LogInfo("")
		r.Output.LogInfo("✅ No differences found")
		r.Output.LogInfo("   Application: %s", result.Application)
		r.Output.LogInfo("   Environment: %s", result.Environment)
		return nil
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("⚠️ Differences detected")
	r.Output.LogInfo("")
	r.Output.LogInfo("Comparing: %s → %s", result.Source, result.Target)
	r.Output.LogInfo("Application: %s", result.Application)
	r.Output.LogInfo("Environment: %s", result.Environment)
	r.Output.LogInfo("")

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
