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

package initialize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/git/commit"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

// Gitignore patterns for Radius
var radiusGitignorePatterns = []string{
	"# Radius generated files",
	"*.tfstate",
	"*.tfstate.*",
	"*.tfvars",
	"!*.tfvars.example",
	".terraform/",
	".terraform.lock.hcl",
}

// NewCommand creates the `rad init --git` command for Git workspace mode.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a Git workspace for Radius",
		Long: `Initialize a Git repository as a Radius workspace.

This command sets up the directory structure and configuration files needed
for Git workspace mode, which enables deployments without a Radius control plane.

The following structure is created:
  .radius/
    config/           # Configuration files
      types/          # Resource type definitions
    model/            # Bicep model files
    plan/             # Generated deployment plans
    deployments/      # Deployment records

Examples:
  # Initialize with prompts
  rad init --git

  # Initialize with auto-confirmation
  rad init --git -y

  # Initialize with specific application name
  rad init --git --application myapp`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	// Add flags
	cmd.Flags().Bool("git", false, "Initialize Git workspace mode (required)")
	cmd.Flags().BoolP("yes", "y", false, "Auto-confirm prompts")
	cmd.Flags().Bool("no-auto-commit", false, "Disable auto-commit of initialization")
	cmd.Flags().StringP("application", "a", "", "Application name")

	return cmd, runner
}

// Runner implements the rad init --git command.
type Runner struct {
	factory  framework.Factory
	Output   output.Interface
	Prompter prompt.Interface

	// GitMode indicates Git workspace mode.
	GitMode bool

	// Yes indicates to auto-confirm prompts.
	Yes bool

	// NoAutoCommit indicates to disable auto-commit.
	NoAutoCommit bool

	// ApplicationName is the application name.
	ApplicationName string

	// WorkDir is the working directory.
	WorkDir string

	// Options contains configured options.
	Options *Options
}

// Options contains initialization options.
type Options struct {
	// ApplicationName is the application name.
	ApplicationName string

	// EnvironmentName is the environment name.
	EnvironmentName string

	// KubernetesContext is the Kubernetes context.
	KubernetesContext string

	// KubernetesNamespace is the Kubernetes namespace.
	KubernetesNamespace string
}

// NewRunner creates a new Runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		factory: factory,
		Options: &Options{},
	}
}

// Validate validates the command arguments and flags.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	r.Output = r.factory.GetOutput()
	r.Prompter = r.factory.GetPrompter()
	r.GitMode, _ = cmd.Flags().GetBool("git")
	r.Yes, _ = cmd.Flags().GetBool("yes")
	r.NoAutoCommit, _ = cmd.Flags().GetBool("no-auto-commit")
	r.ApplicationName, _ = cmd.Flags().GetString("application")

	if !r.GitMode {
		return clierrors.Message("Use 'rad init --git' for Git workspace mode or 'rad initialize' for control plane mode.")
	}

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	r.WorkDir = workDir

	// Validate this is a Git repository
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		return clierrors.Message("Not a Git repository. Run 'git init' first or use 'rad initialize' for control plane mode.")
	}

	// Check if already initialized
	radiusDir := filepath.Join(workDir, ".radius")
	if _, err := os.Stat(radiusDir); err == nil {
		if !r.Yes {
			confirmed, err := prompt.YesOrNoPrompt("Radius workspace already exists. Reinitialize?", prompt.ConfirmNo, r.Prompter)
			if err != nil {
				return err
			}
			if !confirmed {
				return &exitError{message: "Initialization cancelled"}
			}
		}
	}

	return nil
}

// Run executes the initialization.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("")
	r.Output.LogInfo("🚀 Initializing Radius Git workspace...")
	r.Output.LogInfo("")

	// Step 1: Gather configuration
	if err := r.gatherConfiguration(); err != nil {
		return err
	}

	// Step 2: Create directory structure
	if err := r.createDirectoryStructure(); err != nil {
		return err
	}

	// Step 3: Create configuration files
	if err := r.createConfigurationFiles(); err != nil {
		return err
	}

	// Step 4: Update .gitignore
	if err := r.updateGitignore(); err != nil {
		r.Output.LogInfo("Warning: Failed to update .gitignore: %v", err)
	}

	// Step 5: Create sample model
	if err := r.createSampleModel(); err != nil {
		return err
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("✅ Radius workspace initialized!")
	r.Output.LogInfo("")

	// Display results
	r.displayResults()

	// Auto-commit if enabled
	if !r.NoAutoCommit {
		if err := r.autoCommit(ctx); err != nil {
			r.Output.LogInfo("")
			r.Output.LogInfo("Warning: Auto-commit failed: %v", err)
		}
	}

	return nil
}

// gatherConfiguration prompts for configuration if not provided.
func (r *Runner) gatherConfiguration() error {
	// Application name
	if r.ApplicationName == "" {
		r.ApplicationName = filepath.Base(r.WorkDir)

		if !r.Yes {
			name, err := r.Prompter.GetTextInput(fmt.Sprintf("Application name [%s]:", r.ApplicationName), prompt.TextInputOptions{})
			if err != nil {
				return err
			}
			if name != "" {
				r.ApplicationName = name
			}
		}
	}
	r.Options.ApplicationName = r.ApplicationName

	// Environment name
	r.Options.EnvironmentName = "default"
	if !r.Yes {
		options := []string{"default", "development", "staging", "production", "other"}
		selected, err := r.Prompter.GetListInput(options, "Select environment:")
		if err != nil {
			return err
		}
		if selected == "other" {
			name, err := r.Prompter.GetTextInput("Environment name:", prompt.TextInputOptions{})
			if err != nil {
				return err
			}
			r.Options.EnvironmentName = name
		} else {
			r.Options.EnvironmentName = selected
		}
	}

	// Kubernetes context (optional)
	if !r.Yes {
		configure, err := prompt.YesOrNoPrompt("Configure Kubernetes context?", prompt.ConfirmNo, r.Prompter)
		if err != nil {
			return err
		}
		if configure {
			context, err := r.Prompter.GetTextInput("Kubernetes context:", prompt.TextInputOptions{})
			if err != nil {
				return err
			}
			r.Options.KubernetesContext = context

			namespace, err := r.Prompter.GetTextInput("Kubernetes namespace [default]:", prompt.TextInputOptions{})
			if err != nil {
				return err
			}
			if namespace == "" {
				namespace = "default"
			}
			r.Options.KubernetesNamespace = namespace
		}
	}

	return nil
}

// createDirectoryStructure creates the .radius directory structure.
func (r *Runner) createDirectoryStructure() error {
	dirs := []string{
		".radius/config",
		".radius/config/types",
		".radius/model",
		".radius/plan",
		".radius/deployments",
	}

	for _, dir := range dirs {
		path := filepath.Join(r.WorkDir, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	r.Output.LogInfo("   Created .radius/ directory structure")
	return nil
}

// createConfigurationFiles creates configuration files.
func (r *Runner) createConfigurationFiles() error {
	// Create radius.yaml config file
	config := map[string]any{
		"application": r.Options.ApplicationName,
		"environment": map[string]any{
			"name": r.Options.EnvironmentName,
			"file": r.getEnvironmentFileName(),
		},
	}

	if r.Options.KubernetesContext != "" {
		config["kubernetes"] = map[string]any{
			"context":   r.Options.KubernetesContext,
			"namespace": r.Options.KubernetesNamespace,
		}
	}

	configPath := filepath.Join(r.WorkDir, ".radius", "config", "radius.yaml")
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, configYAML, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Create environment file
	envFileName := r.getEnvironmentFileName()
	envPath := filepath.Join(r.WorkDir, envFileName)
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envContent := fmt.Sprintf("# Environment: %s\n# Add environment-specific variables here\n", r.Options.EnvironmentName)
		if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
			return fmt.Errorf("failed to write environment file: %w", err)
		}
	}

	r.Output.LogInfo("   Created configuration files")
	return nil
}

// getEnvironmentFileName returns the environment file name.
func (r *Runner) getEnvironmentFileName() string {
	if r.Options.EnvironmentName == "default" {
		return ".env"
	}
	return fmt.Sprintf(".env.%s", r.Options.EnvironmentName)
}

// updateGitignore updates .gitignore with Radius patterns.
func (r *Runner) updateGitignore() error {
	gitignorePath := filepath.Join(r.WorkDir, ".gitignore")

	var existingContent string
	if content, err := os.ReadFile(gitignorePath); err == nil {
		existingContent = string(content)
	}

	// Check which patterns are missing
	var missingPatterns []string
	for _, pattern := range radiusGitignorePatterns {
		if !strings.Contains(existingContent, pattern) {
			missingPatterns = append(missingPatterns, pattern)
		}
	}

	if len(missingPatterns) == 0 {
		return nil // All patterns already present
	}

	// Append missing patterns
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newlines if file doesn't end with one
	if existingContent != "" && !strings.HasSuffix(existingContent, "\n") {
		f.WriteString("\n")
	}

	// Add blank line before our patterns
	if existingContent != "" {
		f.WriteString("\n")
	}

	for _, pattern := range missingPatterns {
		f.WriteString(pattern + "\n")
	}

	r.Output.LogInfo("   Updated .gitignore")
	return nil
}

// createSampleModel creates a sample Bicep model file.
func (r *Runner) createSampleModel() error {
	modelPath := filepath.Join(r.WorkDir, ".radius", "model", "app.bicep")
	if _, err := os.Stat(modelPath); err == nil {
		return nil // Model already exists
	}

	sampleModel := fmt.Sprintf(`// Sample Radius application model
// Edit this file to define your application resources

extension radius

@description('The application resource')
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: '%s'
  properties: {
    environment: environment().id
  }
}

// Add your resources here
// Example:
// resource container 'Applications.Core/containers@2023-10-01-preview' = {
//   name: 'frontend'
//   properties: {
//     application: app.id
//     container: {
//       image: 'nginx:latest'
//     }
//   }
// }
`, r.Options.ApplicationName)

	if err := os.WriteFile(modelPath, []byte(sampleModel), 0644); err != nil {
		return fmt.Errorf("failed to write sample model: %w", err)
	}

	r.Output.LogInfo("   Created sample model: .radius/model/app.bicep")
	return nil
}

// displayResults displays the initialization results.
func (r *Runner) displayResults() {
	r.Output.LogInfo("📊 Configuration:")
	r.Output.LogInfo("   Application: %s", r.Options.ApplicationName)
	r.Output.LogInfo("   Environment: %s", r.Options.EnvironmentName)
	if r.Options.KubernetesContext != "" {
		r.Output.LogInfo("   Kubernetes Context: %s", r.Options.KubernetesContext)
		r.Output.LogInfo("   Kubernetes Namespace: %s", r.Options.KubernetesNamespace)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("📁 Directory structure:")
	r.Output.LogInfo("   .radius/")
	r.Output.LogInfo("   ├── config/       # Configuration")
	r.Output.LogInfo("   ├── model/        # Bicep models")
	r.Output.LogInfo("   ├── plan/         # Generated plans")
	r.Output.LogInfo("   └── deployments/  # Deployment records")

	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("   git add .radius && git commit -m \"Initialize Radius\"")
	r.Output.LogInfo("   # Edit .radius/model/app.bicep to define your resources")
	r.Output.LogInfo("   rad plan")
	r.Output.LogInfo("   rad deploy")
}

// autoCommit commits the initialization.
func (r *Runner) autoCommit(ctx context.Context) error {
	files := []string{".radius", ".gitignore"}
	
	// Add environment file if created
	envFile := r.getEnvironmentFileName()
	if _, err := os.Stat(filepath.Join(r.WorkDir, envFile)); err == nil {
		files = append(files, envFile)
	}

	return commit.AutoCommit(&commit.CommitOptions{
		RepoRoot:    r.WorkDir,
		FilesToAdd:  files,
		Action:      commit.ActionInit,
		Application: r.Options.ApplicationName,
		Environment: r.Options.EnvironmentName,
	})
}

// exitError is a friendly error that doesn't print TraceId.
type exitError struct {
	message string
}

func (e *exitError) Error() string {
	return e.message
}

func (e *exitError) IsFriendlyError() bool {
	return true
}
