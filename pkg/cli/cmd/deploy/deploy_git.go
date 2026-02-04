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

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/radius-project/radius/pkg/cli/git/commit"
	"github.com/radius-project/radius/pkg/cli/git/deploy"
	"github.com/radius-project/radius/pkg/cli/git/deploy/executor"
	"github.com/radius-project/radius/pkg/cli/git/plan"
	"github.com/radius-project/radius/pkg/cli/git/repo"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

// GitRunner implements deployment for Git workspace mode.
type GitRunner struct {
	Output   output.Interface
	Prompter prompt.Interface

	// Yes indicates to auto-confirm prompts.
	Yes bool

	// NoAutoCommit indicates to disable auto-commit.
	NoAutoCommit bool

	// Environment is the environment to deploy.
	Environment string

	// WorkDir is the working directory.
	WorkDir string

	// Options contains parsed options.
	Options *GitOptions
}

// GitOptions contains options for Git workspace deployment.
type GitOptions struct {
	// PlanDir is the directory containing the plan.
	PlanDir string

	// Plan is the loaded plan.
	Plan *plan.Plan

	// Application is the application name.
	Application string

	// Environment is the environment name.
	Environment string

	// EnvironmentFile is the path to the environment file.
	EnvironmentFile string

	// KubernetesNamespace is the Kubernetes namespace.
	KubernetesNamespace string

	// KubernetesContext is the Kubernetes context.
	KubernetesContext string

	// GitInfo contains Git information.
	GitInfo *repo.GitInfo
}

// NewGitRunner creates a new GitRunner.
func NewGitRunner(output output.Interface, prompter prompt.Interface, workDir string) *GitRunner {
	return &GitRunner{
		Output:   output,
		Prompter: prompter,
		WorkDir:  workDir,
	}
}

// ValidateGitWorkspace validates that we're in a Git workspace.
func (r *GitRunner) ValidateGitWorkspace() error {
	// Check for .git directory
	if _, err := os.Stat(filepath.Join(r.WorkDir, ".git")); os.IsNotExist(err) {
		return fmt.Errorf("not a Git repository")
	}

	return nil
}

// LoadPlan loads the plan for the specified environment.
func (r *GitRunner) LoadPlan(environment string) error {
	if environment == "" {
		environment = r.detectEnvironment()
	}
	r.Environment = environment

	planDir := filepath.Join(r.WorkDir, ".radius", "plan", environment)
	planPath := filepath.Join(planDir, "plan.yaml")

	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		return fmt.Errorf("no plan found for environment '%s'. Run 'rad plan' first", environment)
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
	gitInfo, _ := repo.GetGitInfo(r.WorkDir)

	r.Options = &GitOptions{
		PlanDir:         planDir,
		Plan:            &p,
		Application:     p.Application,
		Environment:     environment,
		EnvironmentFile: p.EnvironmentFile,
		GitInfo:         gitInfo,
	}

	return nil
}

// RunGit executes the deployment in Git workspace mode.
func (r *GitRunner) RunGit(ctx context.Context) error {
	// Check for uncommitted changes
	if err := r.checkUncommittedChanges(ctx); err != nil {
		return err
	}

	// Confirm deployment
	if !r.Yes {
		if err := r.confirmDeployment(); err != nil {
			return err
		}
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("🚀 Deploying resources...")
	r.Output.LogInfo("")

	// Create deployment record
	record := r.createDeploymentRecord()
	startTime := time.Now()

	// Execute each step
	for _, step := range r.Options.Plan.Steps {
		stepDir := fmt.Sprintf("%03d-%s-terraform", step.Sequence, step.Resource.Name)
		stepPath := filepath.Join(r.Options.PlanDir, stepDir)

		r.Output.LogInfo("   🎯 %s (%s)", step.Resource.Name, step.Resource.Type)

		exec := executor.NewTerraformExecutor(stepPath).
			WithResource(step.Resource.Name, step.Resource.Type).
			WithSequence(step.Sequence)

		if step.Recipe.Name != "" {
			exec.WithRecipe(step.Recipe.Name, step.Recipe.Location)
		}

		result, err := exec.Execute(ctx)
		if err != nil {
			r.Output.LogInfo("      ✗ Failed: %v", err)
			result.Status = deploy.StatusFailed
			result.Error = err.Error()
			record.AddStepResult(*result)
			record.Fail(err)

			// Save partial record
			recordPath, _ := r.saveDeploymentRecord(record)
			r.Output.LogInfo("")
			r.Output.LogInfo("❌ Deployment failed")
			if recordPath != "" {
				relPath, _ := filepath.Rel(r.WorkDir, recordPath)
				r.Output.LogInfo("   Partial record saved: %s", relPath)
			}
			return &gitExitError{message: fmt.Sprintf("Deployment failed: %v", err)}
		}

		r.Output.LogInfo("      ✓ Deployed")
		if result.Changes != nil && (result.Changes.Add > 0 || result.Changes.Change > 0) {
			r.Output.LogInfo("      Resources: +%d ~%d -%d",
				result.Changes.Add, result.Changes.Change, result.Changes.Destroy)
		}

		record.AddStepResult(*result)
	}

	// Complete the record
	record.Complete()

	// Save deployment record
	recordPath, err := r.saveDeploymentRecord(record)
	if err != nil {
		r.Output.LogInfo("")
		r.Output.LogInfo("⚠️ Warning: Failed to save deployment record: %v", err)
	}

	// Display results
	duration := time.Since(startTime)
	r.displayResults(record, recordPath, duration)

	// Auto-commit if enabled
	if !r.NoAutoCommit {
		if err := r.autoCommit(ctx, recordPath); err != nil {
			r.Output.LogInfo("")
			r.Output.LogInfo("⚠️ Auto-commit failed: %v", err)
			r.Output.LogInfo("   Run manually: git add %s && git commit", recordPath)
		}
	}

	return nil
}

// detectEnvironment auto-detects the environment from existing plans.
func (r *GitRunner) detectEnvironment() string {
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

// checkUncommittedChanges checks for uncommitted changes in the plan directory.
func (r *GitRunner) checkUncommittedChanges(ctx context.Context) error {
	if r.Yes {
		return nil // Skip check when auto-confirming
	}

	hasUncommitted, err := repo.HasUncommittedChanges(r.WorkDir, r.Options.PlanDir)
	if err != nil {
		return nil // Ignore errors checking uncommitted changes
	}

	if hasUncommitted {
		r.Output.LogInfo("")
		r.Output.LogInfo("⚠️ Warning: You have uncommitted changes in the plan directory.")
		r.Output.LogInfo("   Consider committing your changes first:")
		r.Output.LogInfo("   git add .radius/plan && git commit -m \"Update plan\"")
		r.Output.LogInfo("")

		confirmed, err := prompt.YesOrNoPrompt("Continue with deployment anyway?", prompt.ConfirmNo, r.Prompter)
		if err != nil {
			return err
		}

		if !confirmed {
			return &gitExitError{message: "Deployment cancelled"}
		}
	}

	return nil
}

// confirmDeployment prompts for deployment confirmation.
func (r *GitRunner) confirmDeployment() error {
	r.Output.LogInfo("")
	r.Output.LogInfo("📍 Deployment Target")
	r.Output.LogInfo("")
	r.Output.LogInfo("   Application: %s", r.Options.Application)
	r.Output.LogInfo("   Environment: %s", r.Options.Environment)

	if r.Options.KubernetesContext != "" {
		r.Output.LogInfo("   Context: %s", r.Options.KubernetesContext)
	}
	if r.Options.KubernetesNamespace != "" {
		r.Output.LogInfo("   Namespace: %s", r.Options.KubernetesNamespace)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("📦 Resources to deploy:")
	for _, step := range r.Options.Plan.Steps {
		r.Output.LogInfo("   %d. %s (%s)", step.Sequence, step.Resource.Name, step.Resource.Type)
		if step.Recipe.Name != "" {
			r.Output.LogInfo("      Recipe: %s", step.Recipe.Name)
		}
	}
	r.Output.LogInfo("")

	confirmed, err := prompt.YesOrNoPrompt("Deploy these resources?", prompt.ConfirmNo, r.Prompter)
	if err != nil {
		return err
	}

	if !confirmed {
		return &gitExitError{message: "Deployment cancelled"}
	}

	return nil
}

// createDeploymentRecord creates a new deployment record.
func (r *GitRunner) createDeploymentRecord() *deploy.DeploymentRecord {
	envInfo := r.buildEnvironmentInfo()
	planRef := &deploy.PlanReference{
		Path: filepath.Join(r.Options.PlanDir, "plan.yaml"),
	}

	if r.Options.GitInfo != nil {
		planRef.Commit = r.Options.GitInfo.CommitSHA
	}

	gitInfo := r.buildGitInfo()

	return deploy.NewDeploymentRecord(r.Options.Application, envInfo, planRef, gitInfo)
}

// buildEnvironmentInfo builds EnvironmentInfo.
func (r *GitRunner) buildEnvironmentInfo() *deploy.EnvironmentInfo {
	return &deploy.EnvironmentInfo{
		Name:                r.Options.Environment,
		EnvironmentFile:     r.Options.EnvironmentFile,
		KubernetesContext:   r.Options.KubernetesContext,
		KubernetesNamespace: r.Options.KubernetesNamespace,
	}
}

// buildGitInfo builds GitInfo from repo.GitInfo.
func (r *GitRunner) buildGitInfo() *deploy.GitInfo {
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

// saveDeploymentRecord saves the deployment record to disk.
func (r *GitRunner) saveDeploymentRecord(record *deploy.DeploymentRecord) (string, error) {
	recordDir := filepath.Join(r.WorkDir, ".radius", "deployments", r.Options.Environment)
	if err := os.MkdirAll(recordDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create record directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("deploy-%s.json", timestamp)
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

// displayResults displays the deployment results.
func (r *GitRunner) displayResults(record *deploy.DeploymentRecord, recordPath string, duration time.Duration) {
	r.Output.LogInfo("")
	r.Output.LogInfo("✅ Deployment completed successfully!")
	r.Output.LogInfo("")

	r.Output.LogInfo("📊 Summary:")
	r.Output.LogInfo("   Application: %s", r.Options.Application)
	r.Output.LogInfo("   Environment: %s", r.Options.Environment)
	r.Output.LogInfo("   Resources deployed: %d", len(record.Steps))
	r.Output.LogInfo("   Duration: %s", duration.Round(time.Second))

	// Count total cloud resources
	totalResources := 0
	for _, step := range record.Steps {
		totalResources += len(step.CloudResources)
	}
	if totalResources > 0 {
		r.Output.LogInfo("   Cloud resources created: %d", totalResources)
	}

	if recordPath != "" {
		relRecordPath, _ := filepath.Rel(r.WorkDir, recordPath)
		r.Output.LogInfo("")
		r.Output.LogInfo("📁 Review the deployment record:")
		r.Output.LogInfo("   cat %s", relRecordPath)
	}

	// Show next steps
	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	if recordPath != "" {
		relRecordPath, _ := filepath.Rel(r.WorkDir, recordPath)
		commitMsg := fmt.Sprintf("Deploy %s to %s", r.Options.Application, r.Options.Environment)
		if r.Options.GitInfo != nil && r.Options.GitInfo.ShortSHA != "" {
			commitMsg = fmt.Sprintf("Deploy %s@%s", r.Options.Application, r.Options.GitInfo.ShortSHA)
		}
		r.Output.LogInfo("   git add %s && git commit -m \"%s\"", relRecordPath, commitMsg)
	}
	r.Output.LogInfo("   rad diff ...live   # Check for drift")
	r.Output.LogInfo("   rad delete         # Remove resources")
}

// autoCommit commits the deployment record.
func (r *GitRunner) autoCommit(ctx context.Context, recordPath string) error {
	relPath, _ := filepath.Rel(r.WorkDir, recordPath)

	return commit.AutoCommit(&commit.CommitOptions{
		RepoRoot:    r.WorkDir,
		FilesToAdd:  []string{relPath},
		Action:      commit.ActionDeploy,
		Application: r.Options.Application,
		Environment: r.Options.Environment,
	})
}

// DetectGitWorkspace checks if we're in a Git workspace with Radius config.
func DetectGitWorkspace(workDir string) bool {
	// Check for .git directory
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		return false
	}

	// Check for .radius directory with config or plan
	radiusDir := filepath.Join(workDir, ".radius")
	if _, err := os.Stat(radiusDir); os.IsNotExist(err) {
		return false
	}

	// Check for plan directory or config directory
	planDir := filepath.Join(radiusDir, "plan")
	configDir := filepath.Join(radiusDir, "config")

	if _, err := os.Stat(planDir); err == nil {
		return true
	}
	if _, err := os.Stat(configDir); err == nil {
		return true
	}

	return false
}

// GetEnvironmentNameFromPath extracts environment name from env file path.
func GetEnvironmentNameFromPath(envFile string) string {
	if envFile == ".env" {
		return "default"
	}
	if strings.HasPrefix(envFile, ".env.") {
		return strings.TrimPrefix(envFile, ".env.")
	}
	if strings.HasSuffix(envFile, ".env") {
		return strings.TrimSuffix(filepath.Base(envFile), ".env")
	}
	return "default"
}

// gitExitError is a friendly error that doesn't print TraceId.
type gitExitError struct {
	message string
}

func (e *gitExitError) Error() string {
	return e.message
}

func (e *gitExitError) IsFriendlyError() bool {
	return true
}
