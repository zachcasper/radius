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
	"os/exec"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/radius-project/radius/pkg/cli/git/deploy"
	"github.com/radius-project/radius/pkg/cli/git/kubernetes"
	"github.com/radius-project/radius/pkg/cli/git/plan"
)

// Differ performs diff operations for Git workspace mode.
type Differ struct {
	// WorkDir is the Git workspace directory.
	WorkDir string

	// Application is the application to diff.
	Application string

	// Environment is the environment to diff.
	Environment string

	// YAMLDiffer is the YAML differ instance.
	YAMLDiffer *YAMLDiffer

	// KubeContext is the Kubernetes context for live queries.
	KubeContext string

	// KubeNamespace is the Kubernetes namespace for live queries.
	KubeNamespace string
}

// NewDiffer creates a new Differ.
func NewDiffer(workDir string) *Differ {
	return &Differ{
		WorkDir:    workDir,
		YAMLDiffer: NewYAMLDiffer(),
	}
}

// WithApplication sets the application name.
func (d *Differ) WithApplication(app string) *Differ {
	d.Application = app
	return d
}

// WithEnvironment sets the environment name.
func (d *Differ) WithEnvironment(env string) *Differ {
	d.Environment = env
	return d
}

// WithKubernetes sets the Kubernetes configuration.
func (d *Differ) WithKubernetes(kubeContext, namespace string) *Differ {
	d.KubeContext = kubeContext
	d.KubeNamespace = namespace
	return d
}

// DiffUncommitted compares the current working directory against HEAD.
func (d *Differ) DiffUncommitted(ctx context.Context) (*DiffResult, error) {
	result := &DiffResult{
		Source:      "HEAD",
		Target:      "working directory",
		Application: d.Application,
		Environment: d.Environment,
	}

	// Get list of changed files
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "HEAD")
	cmd.Dir = d.WorkDir
	output, err := cmd.Output()
	if err != nil {
		// If git diff fails (e.g., no commits yet), return empty result
		return result, nil
	}

	changedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(changedFiles) == 0 || (len(changedFiles) == 1 && changedFiles[0] == "") {
		return result, nil
	}

	// Check for plan.yaml changes
	for _, file := range changedFiles {
		if strings.HasSuffix(file, "plan.yaml") {
			diffs, dyffOutput, err := d.diffPlanFile(ctx, "HEAD", "", file)
			if err != nil {
				continue
			}
			result.PlanDiffs = append(result.PlanDiffs, diffs...)
			result.DyffOutput += dyffOutput
			result.HasDiff = true
		}
	}

	return result, nil
}

// DiffTwoCommits compares two Git commits.
func (d *Differ) DiffTwoCommits(ctx context.Context, sourceCommit, targetCommit string) (*DiffResult, error) {
	result := &DiffResult{
		Source:      sourceCommit,
		Target:      targetCommit,
		Application: d.Application,
		Environment: d.Environment,
	}

	// Get list of changed files between commits
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", sourceCommit, targetCommit)
	cmd.Dir = d.WorkDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to diff commits: %w", err)
	}

	changedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(changedFiles) == 0 || (len(changedFiles) == 1 && changedFiles[0] == "") {
		return result, nil
	}

	// Check for plan.yaml changes
	for _, file := range changedFiles {
		if strings.HasSuffix(file, "plan.yaml") {
			diffs, dyffOutput, err := d.diffPlanFile(ctx, sourceCommit, targetCommit, file)
			if err != nil {
				continue
			}
			result.PlanDiffs = append(result.PlanDiffs, diffs...)
			result.DyffOutput += dyffOutput
			result.HasDiff = true
		}

		// Check for deployment record changes
		if strings.Contains(file, ".radius/deployments/") && strings.HasSuffix(file, ".json") {
			diffs, err := d.diffDeploymentRecord(ctx, sourceCommit, targetCommit, file)
			if err != nil {
				continue
			}
			result.DeploymentDiffs = append(result.DeploymentDiffs, diffs...)
			result.HasDiff = true
		}
	}

	return result, nil
}

// DiffCommitToLive compares a commit's deployment against live cloud state.
func (d *Differ) DiffCommitToLive(ctx context.Context, commit string) (*DiffResult, error) {
	result := &DiffResult{
		Source:      commit,
		Target:      "live",
		Application: d.Application,
		Environment: d.Environment,
	}

	// Find the latest deployment record at the commit
	deploymentPath, err := d.findLatestDeploymentRecord(ctx, commit)
	if err != nil {
		return nil, fmt.Errorf("failed to find deployment record at %s: %w", commit, err)
	}

	// Read deployment record
	record, err := d.readDeploymentRecordAtCommit(ctx, commit, deploymentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read deployment record: %w", err)
	}

	// Create Kubernetes manifest capture
	capture, err := kubernetes.NewManifestCapture(d.KubeContext, d.KubeNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest capture: %w", err)
	}

	// Get deployment directory for reading YAML files
	deployDir := filepath.Dir(deploymentPath)

	// Compare each captured resource against live state
	for _, step := range record.Steps {
		for _, captured := range step.CapturedResources {
			// Parse resource ID (format: namespace/kind/name)
			namespace, kind, name, err := kubernetes.ParseResourceID(captured.ResourceID)
			if err != nil {
				continue
			}

			// Get live manifest
			liveManifest, err := capture.GetRawManifest(ctx, kind, name, namespace)
			if err != nil {
				// Resource may have been deleted
				result.ManifestDiffs = append(result.ManifestDiffs, ManifestDiff{
					Kind:      kind,
					Name:      name,
					Namespace: namespace,
					Change:    ChangeRemoved,
				})
				result.HasDiff = true
				continue
			}

			// Read captured YAML from the resource definition file
			capturedYAML, err := d.getFileAtCommit(ctx, commit, filepath.Join(deployDir, captured.ResourceDefinitionFile))
			if err != nil {
				continue
			}

			diffs, dyffOutput, err := DiffStrings(capturedYAML, liveManifest)
			if err != nil {
				continue
			}

			if len(diffs) > 0 {
				result.ManifestDiffs = append(result.ManifestDiffs, ManifestDiff{
					Kind:          kind,
					Name:          name,
					Namespace:     namespace,
					Change:        ChangeModified,
					PropertyDiffs: diffs,
					DyffOutput:    dyffOutput,
				})
				result.HasDiff = true
			}
		}
	}

	return result, nil
}

// DiscoverAppAndEnv discovers application and environment from the repository.
func (d *Differ) DiscoverAppAndEnv(ctx context.Context, commit string) (string, string, error) {
	// Look for plan.yaml files at the commit
	cmd := exec.CommandContext(ctx, "git", "ls-tree", "-r", "--name-only", commit)
	cmd.Dir = d.WorkDir
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to list files at %s: %w", commit, err)
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, file := range files {
		if strings.HasSuffix(file, "plan.yaml") {
			// Parse the plan.yaml to get app/env
			content, err := d.getFileAtCommit(ctx, commit, file)
			if err != nil {
				continue
			}

			var p plan.Plan
			if err := yaml.Unmarshal([]byte(content), &p); err != nil {
				continue
			}

			app := p.Application
			env := d.extractEnvFromPath(file)
			if env == "" {
				env = d.extractEnvFromEnvFile(p.EnvironmentFile)
			}

			return app, env, nil
		}
	}

	return "", "", fmt.Errorf("could not discover application and environment")
}

// detectArtifactType determines what type of artifact exists at a commit.
func (d *Differ) detectArtifactType(ctx context.Context, commit string) (ArtifactType, error) {
	// Use git ls-tree to check files at the commit
	cmd := exec.CommandContext(ctx, "git", "ls-tree", "-r", "--name-only", commit)
	cmd.Dir = d.WorkDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list files at %s: %w", commit, err)
	}

	files := strings.Split(string(output), "\n")

	hasDeployment := false
	hasPlan := false
	hasModel := false

	for _, file := range files {
		file = strings.TrimSpace(file)
		if strings.Contains(file, ".radius/deployments/") && strings.HasSuffix(file, ".json") {
			hasDeployment = true
		}
		if strings.HasSuffix(file, "plan.yaml") {
			hasPlan = true
		}
		if strings.HasSuffix(file, ".bicep") {
			hasModel = true
		}
	}

	// Return in order of specificity
	if hasDeployment {
		return ArtifactTypeDeployment, nil
	}
	if hasPlan {
		return ArtifactTypePlan, nil
	}
	if hasModel {
		return ArtifactTypeModel, nil
	}

	return "", fmt.Errorf("no recognizable artifacts at commit %s", commit)
}

// diffPlanFile diffs a plan.yaml file between two commits.
func (d *Differ) diffPlanFile(ctx context.Context, sourceCommit, targetCommit, filePath string) ([]PropertyDiff, string, error) {
	sourceContent, err := d.getFileAtCommit(ctx, sourceCommit, filePath)
	if err != nil {
		return nil, "", err
	}

	var targetContent string
	if targetCommit == "" {
		// Compare against working directory
		fullPath := filepath.Join(d.WorkDir, filePath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, "", err
		}
		targetContent = string(content)
	} else {
		targetContent, err = d.getFileAtCommit(ctx, targetCommit, filePath)
		if err != nil {
			return nil, "", err
		}
	}

	return DiffStrings(sourceContent, targetContent)
}

// diffDeploymentRecord diffs a deployment record between two commits.
func (d *Differ) diffDeploymentRecord(ctx context.Context, sourceCommit, targetCommit, filePath string) ([]PropertyDiff, error) {
	sourceContent, err := d.getFileAtCommit(ctx, sourceCommit, filePath)
	if err != nil {
		return nil, err
	}

	targetContent, err := d.getFileAtCommit(ctx, targetCommit, filePath)
	if err != nil {
		return nil, err
	}

	var sourceRecord, targetRecord map[string]any
	if err := json.Unmarshal([]byte(sourceContent), &sourceRecord); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(targetContent), &targetRecord); err != nil {
		return nil, err
	}

	return DiffMaps(sourceRecord, targetRecord)
}

// getFileAtCommit retrieves a file's content at a specific commit.
func (d *Differ) getFileAtCommit(ctx context.Context, commit, filePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "show", fmt.Sprintf("%s:%s", commit, filePath))
	cmd.Dir = d.WorkDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get file %s at %s: %w", filePath, commit, err)
	}
	return string(output), nil
}

// findLatestDeploymentRecord finds the latest deployment record at a commit.
func (d *Differ) findLatestDeploymentRecord(ctx context.Context, commit string) (string, error) {
	// List deployment records at commit
	cmd := exec.CommandContext(ctx, "git", "ls-tree", "-r", "--name-only", commit)
	cmd.Dir = d.WorkDir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var latestPath string
	files := strings.Split(string(output), "\n")
	for _, file := range files {
		file = strings.TrimSpace(file)
		if !strings.Contains(file, ".radius/deployments/") || !strings.HasSuffix(file, ".json") {
			continue
		}

		// Filter by environment if set
		if d.Environment != "" && !strings.Contains(file, "/"+d.Environment+"/") {
			continue
		}

		// Take the last one (they're sorted by name which includes timestamp)
		latestPath = file
	}

	if latestPath == "" {
		return "", fmt.Errorf("no deployment records found")
	}

	return latestPath, nil
}

// readDeploymentRecordAtCommit reads a deployment record from a commit.
func (d *Differ) readDeploymentRecordAtCommit(ctx context.Context, commit, filePath string) (*deploy.DeploymentRecord, error) {
	content, err := d.getFileAtCommit(ctx, commit, filePath)
	if err != nil {
		return nil, err
	}

	var record deploy.DeploymentRecord
	if err := json.Unmarshal([]byte(content), &record); err != nil {
		return nil, fmt.Errorf("failed to parse deployment record: %w", err)
	}

	return &record, nil
}

// extractEnvFromPath extracts environment name from a file path.
func (d *Differ) extractEnvFromPath(path string) string {
	// Path like .radius/plan/production/plan.yaml
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "plan" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// extractEnvFromEnvFile extracts environment name from an env file path.
func (d *Differ) extractEnvFromEnvFile(envFile string) string {
	if envFile == ".env" {
		return "default"
	}
	if strings.HasPrefix(envFile, ".env.") {
		return strings.TrimPrefix(envFile, ".env.")
	}
	if strings.HasSuffix(envFile, ".env") {
		return strings.TrimSuffix(filepath.Base(envFile), ".env")
	}
	return ""
}

// FormatDiffOutput formats a DiffResult for human-readable output.
func FormatDiffOutput(result *DiffResult) string {
	if result == nil {
		return "No result"
	}

	if !result.HasDiff {
		return "No differences found"
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Comparing %s → %s\n", result.Source, result.Target))
	sb.WriteString(fmt.Sprintf("Application: %s\n", result.Application))
	sb.WriteString(fmt.Sprintf("Environment: %s\n\n", result.Environment))

	if len(result.PlanDiffs) > 0 {
		sb.WriteString("Plan Changes:\n")
		for _, diff := range result.PlanDiffs {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", diff.Path, diff.Change))
		}
		sb.WriteString("\n")
	}

	if len(result.DeploymentDiffs) > 0 {
		sb.WriteString("Deployment Record Changes:\n")
		for _, diff := range result.DeploymentDiffs {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", diff.Path, diff.Change))
		}
		sb.WriteString("\n")
	}

	if len(result.ManifestDiffs) > 0 {
		sb.WriteString("Kubernetes Manifest Changes:\n")
		for _, diff := range result.ManifestDiffs {
			sb.WriteString(fmt.Sprintf("  %s/%s (%s): %s\n", diff.Kind, diff.Name, diff.Namespace, diff.Change))
			if diff.DyffOutput != "" {
				sb.WriteString(diff.DyffOutput)
				sb.WriteString("\n")
			}
		}
	}

	if result.DyffOutput != "" {
		sb.WriteString("\nDetailed Changes:\n")
		sb.WriteString(result.DyffOutput)
	}

	return sb.String()
}
