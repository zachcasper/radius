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

// shortCommit returns the short commit SHA from GitInfo, or empty string if not available.
func (r *GitRunner) shortCommit() string {
	if r.Options == nil || r.Options.GitInfo == nil {
		return ""
	}
	if r.Options.GitInfo.ShortSHA != "" {
		return r.Options.GitInfo.ShortSHA
	}
	// Fallback to truncating CommitSHA
	if len(r.Options.GitInfo.CommitSHA) > 8 {
		return r.Options.GitInfo.CommitSHA[:8]
	}
	return r.Options.GitInfo.CommitSHA
}

// LoadPlan loads the plan for the specified environment.
func (r *GitRunner) LoadPlan(environment string) error {
	// First, find the application directory
	planBaseDir := filepath.Join(r.WorkDir, ".radius", "plan")
	appDirs, err := os.ReadDir(planBaseDir)
	if err != nil {
		return fmt.Errorf("no plan found. Run 'rad plan' first")
	}

	// Find the application and environment
	var planDir, planPath, appName string
	for _, appEntry := range appDirs {
		if !appEntry.IsDir() {
			continue
		}
		appPath := filepath.Join(planBaseDir, appEntry.Name())
		envDirs, err := os.ReadDir(appPath)
		if err != nil {
			continue
		}
		for _, envEntry := range envDirs {
			if !envEntry.IsDir() {
				continue
			}
			// Check if this matches the requested environment (or any if not specified)
			if environment != "" && envEntry.Name() != environment {
				continue
			}
			testPlanPath := filepath.Join(appPath, envEntry.Name(), "plan.yaml")
			if _, err := os.Stat(testPlanPath); err == nil {
				planDir = filepath.Join(appPath, envEntry.Name())
				planPath = testPlanPath
				appName = appEntry.Name()
				if environment == "" {
					environment = envEntry.Name()
				}
				break
			}
		}
		if planPath != "" {
			break
		}
	}

	if planPath == "" {
		if environment != "" {
			return fmt.Errorf("no plan found for environment '%s'. Run 'rad plan' first", environment)
		}
		return fmt.Errorf("no plan found. Run 'rad plan' first")
	}

	r.Environment = environment

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

	// Load environment config from .env file
	kubeContext, kubeNamespace := loadKubernetesConfig(r.WorkDir, p.EnvironmentFile)

	r.Options = &GitOptions{
		PlanDir:             planDir,
		Plan:                &p,
		Application:         appName,
		Environment:         environment,
		EnvironmentFile:     p.EnvironmentFile,
		KubernetesContext:   kubeContext,
		KubernetesNamespace: kubeNamespace,
		GitInfo:             gitInfo,
	}

	return nil
}

// loadKubernetesConfig loads Kubernetes context and namespace from the environment file.
func loadKubernetesConfig(workDir, envFile string) (context string, namespace string) {
	if envFile == "" {
		envFile = ".env"
	}
	envPath := filepath.Join(workDir, envFile)
	data, err := os.ReadFile(envPath)
	if err != nil {
		return "", ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.HasPrefix(line, "KUBERNETES_CONTEXT=") {
			context = strings.TrimPrefix(line, "KUBERNETES_CONTEXT=")
		} else if strings.HasPrefix(line, "KUBERNETES_NAMESPACE=") {
			namespace = strings.TrimPrefix(line, "KUBERNETES_NAMESPACE=")
		}
	}
	return context, namespace
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

	// Show deployment header
	r.Output.LogInfo("🚀 Deploying %s to %s", r.Options.Application, r.Options.Environment)
	if commit := r.shortCommit(); commit != "" {
		r.Output.LogInfo("   Commit: %s", commit)
	}
	r.Output.LogInfo("")

	// Create deployment record and deploy directory
	record := r.createDeploymentRecord()
	startTime := time.Now()
	totalSteps := len(r.Options.Plan.Steps)

	// Create deploy directory for captured resource manifests
	// Structure: .radius/deploy/<app>/<env>/<commit>/
	commitID := r.shortCommit()
	if commitID == "" {
		commitID = time.Now().Format("20060102-150405")
	}
	deployDir := filepath.Join(r.WorkDir, ".radius", "deploy", r.Options.Application, r.Options.Environment, commitID)
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		return &gitExitError{message: fmt.Sprintf("failed to create deploy directory: %v", err)}
	}

	// Execute each step
	for i, step := range r.Options.Plan.Steps {
		stepDir := fmt.Sprintf("%03d-%s-terraform", step.Sequence, step.Resource.Name)
		stepPath := filepath.Join(r.Options.PlanDir, stepDir)
		stepStart := time.Now()

		r.Output.LogInfo("   [%d/%d] %s", i+1, totalSteps, step.Resource.Name)
		r.Output.LogInfo("   ... terraform init")

		exec := executor.NewTerraformExecutor(stepPath).
			WithResource(step.Resource.Name, step.Resource.Type).
			WithSequence(step.Sequence).
			WithKubernetes(r.Options.KubernetesNamespace, r.Options.KubernetesContext).
			WithDeployDir(deployDir)

		if step.Recipe.Name != "" {
			exec.WithRecipe(step.Recipe.Name, step.Recipe.Location)
		}

		r.Output.LogInfo("   ... terraform apply")
		result, err := exec.Execute(ctx)
		if err != nil {
			r.Output.LogInfo("       ❌ Failed: %v", err)
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

		stepDuration := time.Since(stepStart)
		if len(result.CapturedResources) > 0 {
			r.Output.LogInfo("   ... captured %d Kubernetes manifests", len(result.CapturedResources))
		}
		r.Output.LogInfo("       ✅ Done (%ds)", int(stepDuration.Seconds()))

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
		relPlanDir, _ := filepath.Rel(r.WorkDir, r.Options.PlanDir)
		commitMsg := fmt.Sprintf("Generate plan for %s (%s)", r.Options.Application, r.Options.Environment)
		if r.Options.GitInfo != nil && r.Options.GitInfo.ShortSHA != "" {
			commitMsg = fmt.Sprintf("Plan %s@%s", r.Options.Application, r.Options.GitInfo.ShortSHA)
		}

		r.Output.LogInfo("")
		r.Output.LogInfo("⚠️ Warning: You have uncommitted changes in the plan directory.")
		r.Output.LogInfo("   Consider committing your changes first:")
		r.Output.LogInfo("   git add %s && git commit -m \"%s\"", relPlanDir, commitMsg)
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
	// Show deploy source (Git info)
	r.Output.LogInfo("")
	r.Output.LogInfo("📍 Deploy from:")
	if r.Options.GitInfo != nil {
		r.Output.LogInfo("   Commit: %s", r.shortCommit())
		r.Output.LogInfo("   Branch: %s", r.Options.GitInfo.Branch)
	}

	// Show target info
	r.Output.LogInfo("")
	r.Output.LogInfo("🎯 Target: %s", r.Options.Environment)
	if r.Options.KubernetesContext != "" {
		r.Output.LogInfo("   Context:   %s", r.Options.KubernetesContext)
	}
	if r.Options.KubernetesNamespace != "" {
		r.Output.LogInfo("   Namespace: %s", r.Options.KubernetesNamespace)
	}

	// Show resources
	r.Output.LogInfo("")
	r.Output.LogInfo("📦 Resources: %d", len(r.Options.Plan.Steps))
	r.Output.LogInfo("")
	for _, step := range r.Options.Plan.Steps {
		r.Output.LogInfo("   %d. %s", step.Sequence, step.Resource.Name)
		r.Output.LogInfo("      Type:   %s", step.Resource.Type)
		if step.Recipe.Name != "" {
			r.Output.LogInfo("      Recipe: %s", step.Recipe.Kind)
		}
		if step.ExpectedChanges != nil && (step.ExpectedChanges.Add > 0 || step.ExpectedChanges.Change > 0 || step.ExpectedChanges.Destroy > 0) {
			r.Output.LogInfo("      Changes: +%d ~%d -%d", step.ExpectedChanges.Add, step.ExpectedChanges.Change, step.ExpectedChanges.Destroy)
		}
		r.Output.LogInfo("")
	}

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
		PlanFile: filepath.Join(r.Options.PlanDir, "plan.yaml"),
	}

	if r.Options.GitInfo != nil {
		planRef.PlanCommit = r.Options.GitInfo.CommitSHA
	}
	if r.Options.Plan != nil {
		planRef.GeneratedAt = string(r.Options.Plan.GeneratedAt)
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
// Path: .radius/deploy/{app}/{env}/{commit}/deployment.json
func (r *GitRunner) saveDeploymentRecord(record *deploy.DeploymentRecord) (string, error) {
	// Use short commit for directory name
	commitID := r.shortCommit()
	if commitID == "" {
		commitID = time.Now().Format("20060102-150405")
	}

	// Directory already created before deployment execution
	recordDir := filepath.Join(r.WorkDir, ".radius", "deploy", r.Options.Application, r.Options.Environment, commitID)
	if err := os.MkdirAll(recordDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create record directory: %w", err)
	}

	recordPath := filepath.Join(recordDir, "deployment.json")

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
	r.Output.LogInfo("✅ Deployment succeeded")
	r.Output.LogInfo("")

	// Calculate total resource changes
	totalAdd, totalChange, totalDestroy := 0, 0, 0
	for _, step := range record.Steps {
		if step.Changes != nil {
			totalAdd += step.Changes.Add
			totalChange += step.Changes.Change
			totalDestroy += step.Changes.Destroy
		}
	}

	shortCommit := r.shortCommit()

	r.Output.LogInfo("📊 Summary")
	if shortCommit != "" {
		r.Output.LogInfo("   Commit:      %s", shortCommit)
	}
	r.Output.LogInfo("   Environment: %s", r.Options.Environment)
	r.Output.LogInfo("   Duration:    %ds", int(duration.Seconds()))
	r.Output.LogInfo("   Steps:       %d/%d succeeded", len(record.Steps), len(record.Steps))
	r.Output.LogInfo("   Resources:   +%d ~%d -%d", totalAdd, totalChange, totalDestroy)

	// Show next steps if deployment record was saved
	if recordPath != "" {
		relPath, _ := filepath.Rel(r.WorkDir, recordPath)
		r.Output.LogInfo("")
		r.Output.LogInfo("Next steps:")
		r.Output.LogInfo("  Review the deployment record:")
		r.Output.LogInfo("    cat %s", relPath)
		r.Output.LogInfo("")
		r.Output.LogInfo("  Commit the deployment record:")
		r.Output.LogInfo("    git add .radius/deploy && git commit -m \"Deployment record for git commit %s\"", shortCommit)
	}
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
