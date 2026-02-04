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

package delete

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/git/commit"
	"github.com/radius-project/radius/pkg/cli/git/deploy"
	"github.com/radius-project/radius/pkg/cli/git/deploy/executor"
	"github.com/radius-project/radius/pkg/cli/git/plan"
	"github.com/radius-project/radius/pkg/cli/git/repo"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

// NewCommand creates the `rad delete` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete deployed resources",
		Long: `Delete resources that were deployed using rad deploy.

This command destroys the infrastructure that was created by 'rad deploy',
executing terraform destroy (or az deployment delete) for each resource
in reverse order of deployment.

The delete operation uses the deployment plan to determine what resources
to destroy and in what order.`,
		Example: `
# Delete all resources for the default environment
rad delete

# Delete resources for a specific environment
rad delete -e production

# Delete without confirmation prompt
rad delete -y

# Delete a specific application
rad delete -a myapp
`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	// Add flags
	cmd.Flags().StringP("application", "a", "", "Application to delete")
	cmd.Flags().StringP("environment", "e", "", "Target environment")
	cmd.Flags().Bool("quiet", false, "Minimal output")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	return cmd, runner
}

// Runner implements the rad delete command.
type Runner struct {
	factory  framework.Factory
	Output   output.Interface
	Prompter prompt.Interface

	// Application is the application to delete.
	Application string

	// Yes indicates to auto-confirm prompts.
	Yes bool

	// Quiet indicates minimal output.
	Quiet bool

	// Environment is the environment to delete.
	Environment string

	// WorkDir is the working directory.
	WorkDir string

	// Options contains parsed options.
	Options *Options
}

// Options contains parsed options for the delete command.
type Options struct {
	// PlanDir is the directory containing the plan.
	PlanDir string

	// Plan is the loaded plan.
	Plan *plan.Plan

	// Application is the application name.
	Application string

	// Environment is the environment name.
	Environment string

	// GitInfo contains Git information.
	GitInfo *repo.GitInfo
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
	r.Prompter = r.factory.GetPrompter()
	r.Application, _ = cmd.Flags().GetString("application")
	r.Environment, _ = cmd.Flags().GetString("environment")
	r.Quiet, _ = cmd.Flags().GetBool("quiet")
	r.Yes, _ = cmd.Flags().GetBool("yes")

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	r.WorkDir = workDir

	// Check if this is a Radius Git workspace
	radiusDir := filepath.Join(workDir, ".radius")
	if _, err := os.Stat(radiusDir); os.IsNotExist(err) {
		return &deleteExitError{message: "Not in a Radius Git workspace. Run 'rad init' first."}
	}

	// Auto-detect environment if not specified
	if r.Environment == "" {
		r.Environment = r.detectEnvironment()
	}

	// Load plan
	planDir := filepath.Join(workDir, ".radius", "plan", r.Environment)
	planPath := filepath.Join(planDir, "plan.yaml")

	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		return &deleteExitError{message: "❌ No deployment plan found\n\n   Run 'rad plan <model.bicep>' first to generate a deployment plan."}
	}

	planContent, err := os.ReadFile(planPath)
	if err != nil {
		return fmt.Errorf("failed to read plan: %w", err)
	}

	var p plan.Plan
	if err := yaml.Unmarshal(planContent, &p); err != nil {
		return fmt.Errorf("failed to parse plan: %w", err)
	}

	// Get git info
	gitInfo, _ := repo.GetGitInfo(workDir)

	r.Options = &Options{
		PlanDir:     planDir,
		Plan:        &p,
		Application: p.Application,
		Environment: r.Environment,
		GitInfo:     gitInfo,
	}

	return nil
}

// Run executes the delete command.
func (r *Runner) Run(ctx context.Context) error {
	// Confirm deletion
	if !r.Yes {
		r.Output.LogInfo("")
		r.Output.LogInfo("🗑️ Preparing to delete resources")
		r.Output.LogInfo("")
		r.Output.LogInfo("Application: %s", r.Options.Application)
		r.Output.LogInfo("Environment: %s", r.Options.Environment)
		r.Output.LogInfo("")
		r.Output.LogInfo("Resources to be deleted:")
		for i := len(r.Options.Plan.Steps) - 1; i >= 0; i-- {
			step := r.Options.Plan.Steps[i]
			r.Output.LogInfo("   - %s (%s)", step.Resource.Name, step.Resource.Type)
		}
		r.Output.LogInfo("")

		confirmed, err := prompt.YesOrNoPrompt("Are you sure you want to delete these resources?", prompt.ConfirmNo, r.Prompter)
		if err != nil {
			return err
		}

		if !confirmed {
			r.Output.LogInfo("")
			r.Output.LogInfo("⚠️ Deletion cancelled")
			return nil
		}
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("🗑️ Deleting resources...")
	r.Output.LogInfo("")

	// Create deletion record
	record := r.createDeletionRecord()

	// Execute deletion in reverse order
	steps := r.Options.Plan.Steps
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].Sequence > steps[j].Sequence // Reverse order
	})

	for _, step := range steps {
		stepDir := fmt.Sprintf("%03d-%s-terraform", step.Sequence, step.Resource.Name)
		stepPath := filepath.Join(r.Options.PlanDir, stepDir)

		r.Output.LogInfo("   - %s (%s)", step.Resource.Name, step.Resource.Type)

		exec := executor.NewTerraformExecutor(stepPath).
			WithResource(step.Resource.Name, step.Resource.Type).
			WithSequence(step.Sequence)

		if step.Recipe.Name != "" {
			exec.WithRecipe(step.Recipe.Name, step.Recipe.Location)
		}

		result, err := exec.Destroy(ctx)
		if err != nil {
			r.Output.LogInfo("      ✗ Failed: %v", err)
			result.Status = deploy.StatusFailed
			result.Error = err.Error()
		} else {
			r.Output.LogInfo("      ✓ Deleted")
		}

		record.AddStepResult(*result)
	}

	// Complete the record
	record.Complete()

	// Save deletion record
	recordPath, err := r.saveDeletionRecord(record)
	if err != nil {
		return fmt.Errorf("failed to save deletion record: %w", err)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("✅ Resources deleted successfully!")
	r.Output.LogInfo("")

	// Display results
	r.displayResults(record, recordPath)

	return nil
}

// detectEnvironment auto-detects the environment from existing plans.
func (r *Runner) detectEnvironment() string {
	planBaseDir := filepath.Join(r.WorkDir, ".radius", "plan")
	entries, err := os.ReadDir(planBaseDir)
	if err != nil {
		return "default"
	}

	for _, entry := range entries {
		if entry.IsDir() {
			planPath := filepath.Join(planBaseDir, entry.Name(), "plan.yaml")
			if _, err := os.Stat(planPath); err == nil {
				return entry.Name()
			}
		}
	}

	return "default"
}

// createDeletionRecord creates a new deletion record.
func (r *Runner) createDeletionRecord() *deploy.DeploymentRecord {
	envInfo := &deploy.EnvironmentInfo{
		Name:            r.Options.Environment,
		EnvironmentFile: r.Options.Plan.EnvironmentFile,
	}

	planRef := &deploy.PlanReference{
		Commit: "",
		Path:   filepath.Join(r.Options.PlanDir, "plan.yaml"),
	}

	if r.Options.GitInfo != nil {
		planRef.Commit = r.Options.GitInfo.CommitSHA
	}

	gitInfo := r.buildGitInfo()

	record := deploy.NewDeploymentRecord(r.Options.Application, envInfo, planRef, gitInfo)
	record.Status = deploy.StatusInProgress

	return record
}

// buildGitInfo builds GitInfo from repo.GitInfo.
func (r *Runner) buildGitInfo() *deploy.GitInfo {
	if r.Options.GitInfo == nil {
		return &deploy.GitInfo{}
	}

	return &deploy.GitInfo{
		Commit:      r.Options.GitInfo.CommitSHA,
		CommitShort: r.Options.GitInfo.ShortSHA,
		Branch:      r.Options.GitInfo.Branch,
		IsDirty:     r.Options.GitInfo.HasUncommitted,
	}
}

// saveDeletionRecord saves the deletion record to disk.
func (r *Runner) saveDeletionRecord(record *deploy.DeploymentRecord) (string, error) {
	recordDir := filepath.Join(r.WorkDir, ".radius", "deployments", r.Options.Environment)
	if err := os.MkdirAll(recordDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create record directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("delete-%s.json", timestamp)
	recordPath := filepath.Join(recordDir, filename)

	jsonBytes, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal record: %w", err)
	}

	if err := os.WriteFile(recordPath, jsonBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to write record: %w", err)
	}

	return recordPath, nil
}

// displayResults displays the deletion results.
func (r *Runner) displayResults(record *deploy.DeploymentRecord, recordPath string) {
	r.Output.LogInfo("📊 Summary:")
	r.Output.LogInfo("   Application: %s", r.Options.Application)
	r.Output.LogInfo("   Environment: %s", r.Options.Environment)
	r.Output.LogInfo("   Resources deleted: %d", len(record.Steps))
	r.Output.LogInfo("   Duration: %s", record.Duration().Round(time.Second))

	relRecordPath, _ := filepath.Rel(r.WorkDir, recordPath)
	r.Output.LogInfo("")
	r.Output.LogInfo("📁 Deletion record: %s", relRecordPath)
}

// autoCommit commits the deletion record.
func (r *Runner) autoCommit(ctx context.Context, recordPath string) error {
	relPath, _ := filepath.Rel(r.WorkDir, recordPath)

	return commit.AutoCommit(&commit.CommitOptions{
		RepoRoot:    r.WorkDir,
		FilesToAdd:  []string{relPath},
		Action:      commit.ActionDelete,
		Application: r.Options.Application,
		Environment: r.Options.Environment,
	})
}

// deleteExitError is a friendly error that doesn't print TraceId.
type deleteExitError struct {
	message string
}

func (e *deleteExitError) Error() string {
	return e.message
}

func (e *deleteExitError) IsFriendlyError() bool {
	return true
}
