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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

// NewCommand creates the `rad init` command for Git workspace mode.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a Git repository for Radius Git workspace mode",
		Long: `Initialize the current Git repository as a Radius Git workspace.

Git workspace mode enables decentralized deployments without a control plane, 
using the Git repository as the system of record. This is ideal for CI/CD 
workflows and environments where a centralized control plane is not desired.

The command will:
  • Create the .radius/ directory structure
  • Populate Resource Types from radius-project/resource-types-contrib  
  • Configure environment settings (.env files)
  • Create default recipe configurations
  • Set the active workspace to 'git'

To install the Radius control plane on Kubernetes, use 'rad install kubernetes'.`,
		Example: `
# Initialize Git workspace in current directory
rad init

# Initialize with verbose output
rad init --verbose
`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	return cmd, runner
}

// Runner implements the rad init command.
type Runner struct {
	factory  framework.Factory
	Output   output.Interface
	Prompter prompt.Interface

	// WorkDir is the working directory.
	WorkDir string

	// Options contains configured options.
	Options *Options
}

// Options contains initialization options.
type Options struct {
	// Platform is the deployment platform (kubernetes, aws, azure).
	Platform string

	// DeploymentTool is the preferred deployment tool (terraform, bicep).
	DeploymentTool string

	// KubernetesContext is the Kubernetes context.
	KubernetesContext string

	// KubernetesNamespace is the Kubernetes namespace.
	KubernetesNamespace string

	// RecipeFile is the path to the recipes file.
	RecipeFile string
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

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	r.WorkDir = workDir

	// Validate this is a Git repository
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		return clierrors.Message(`❌ Current directory is not a Git repository.

Git workspace mode requires a Git repository.
Please run 'git init' first, then retry 'rad init'.

To install the Radius control plane on Kubernetes instead,
run 'rad install kubernetes'.`)
	}

	return nil
}

// Run executes the initialization.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("🌟 Welcome to Radius!")
	r.Output.LogInfo("")
	r.Output.LogInfo("rad init sets up your Git repository to use Radius.")
	r.Output.LogInfo("")
	r.Output.LogInfo("")
	r.Output.LogInfo("")

	// Step 1: Create directory structure
	if err := r.createDirectoryStructure(); err != nil {
		return err
	}
	r.Output.LogInfo("  ✓ Created .radius/ directory structure")

	// Step 2: Update .gitignore
	if err := r.updateGitignore(); err != nil {
		return fmt.Errorf("failed to update .gitignore: %w", err)
	}
	r.Output.LogInfo("  ✓ Updated .gitignore")

	// Step 3: Fetch and create resource types
	if err := r.fetchResourceTypes(); err != nil {
		return err
	}
	r.Output.LogInfo("  ✓ Fetched Resource Types")

	// Step 4: Prompt for platform selection
	if err := r.selectPlatform(); err != nil {
		return err
	}

	// Step 5: Prompt for deployment tool if multiple available
	if err := r.selectDeploymentTool(); err != nil {
		return err
	}

	// Step 6: Configure Kubernetes if selected
	if r.Options.Platform == "kubernetes" {
		if err := r.configureKubernetes(); err != nil {
			return err
		}
	}

	r.Output.LogInfo("  ✓ Configured deployment platform")

	// Step 7: Create default recipes
	if err := r.createRecipes(); err != nil {
		return err
	}
	r.Output.LogInfo("  ✓ Created default recipes")

	// Step 8: Create environment configuration
	if err := r.createEnvironmentConfig(); err != nil {
		return err
	}
	r.Output.LogInfo("  ✓ Set up environment configuration")

	r.Output.LogInfo("")
	r.Output.LogInfo("")
	r.Output.LogInfo("✅ Git workspace initialized successfully")
	r.Output.LogInfo("")

	// Display results
	r.displayResults()

	return nil
}

// createDirectoryStructure creates the .radius directory structure.
func (r *Runner) createDirectoryStructure() error {
	dirs := []string{
		".radius/config/types",
		".radius/config/recipes",
		".radius/deploy",
		".radius/model",
		".radius/plan",
	}

	for _, dir := range dirs {
		path := filepath.Join(r.WorkDir, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// updateGitignore updates .gitignore with Radius/Terraform patterns.
func (r *Runner) updateGitignore() error {
	gitignorePath := filepath.Join(r.WorkDir, ".gitignore")

	patterns := `# Radius / Terraform
*.tfstate
*.tfstate.*
*.tfvars
!*.tfvars.example
.terraform/
.terraform.lock.hcl
`

	// Check if file exists and contains our patterns
	if content, err := os.ReadFile(gitignorePath); err == nil {
		if strings.Contains(string(content), "# Radius / Terraform") {
			return nil // Already has patterns
		}
		// Append to existing file
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()

		if !strings.HasSuffix(string(content), "\n") {
			f.WriteString("\n")
		}
		_, err = f.WriteString(patterns)
		return err
	}

	// Create new file
	return os.WriteFile(gitignorePath, []byte(patterns), 0644)
}

// fetchResourceTypes fetches resource types from embedded data.
func (r *Runner) fetchResourceTypes() error {
	// Define the resource types to create
	resourceTypes := getEmbeddedResourceTypes()

	for filename, content := range resourceTypes {
		path := filepath.Join(r.WorkDir, ".radius", "config", "types", filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write resource type %s: %w", filename, err)
		}
	}

	return nil
}

// selectPlatform prompts for deployment platform selection.
func (r *Runner) selectPlatform() error {
	options := []string{"Kubernetes", "AWS", "Azure"}
	promptText := "Which platform should Radius deploy resources (databases, caches, etc.) to?"

	selected, err := r.Prompter.GetListInput(options, promptText)
	if err != nil {
		return err
	}

	switch selected {
	case "Kubernetes":
		r.Options.Platform = "kubernetes"
	case "AWS":
		r.Options.Platform = "aws"
	case "Azure":
		r.Options.Platform = "azure"
	}

	return nil
}

// selectDeploymentTool prompts for deployment tool selection.
func (r *Runner) selectDeploymentTool() error {
	// Check which tools are available
	hasTerraform := commandExists("terraform")
	hasBicep := commandExists("az") // bicep usually comes with az cli

	// Default to terraform
	r.Options.DeploymentTool = "terraform"

	if hasTerraform && hasBicep {
		options := []string{"Terraform", "Bicep"}
		selected, err := r.Prompter.GetListInput(options, "Multiple deployment tools found. Select preferred tool:")
		if err != nil {
			return err
		}

		if selected == "Bicep" {
			r.Options.DeploymentTool = "bicep"
		}
	} else if hasBicep && !hasTerraform {
		r.Options.DeploymentTool = "bicep"
	}

	return nil
}

// configureKubernetes prompts for Kubernetes configuration.
func (r *Runner) configureKubernetes() error {
	// Get current context
	currentContext := getCurrentKubeContext()

	// Get list of contexts
	contexts := getKubeContexts()
	if len(contexts) == 0 {
		// No contexts found, will configure later
		r.Options.KubernetesContext = ""
		return nil
	}

	// Build options list - just the context names
	options := make([]string, len(contexts))
	copy(options, contexts)
	options = append(options, "(Configure later)")

	promptText := fmt.Sprintf("Select Kubernetes context (current: %s):", currentContext)
	selected, err := r.Prompter.GetListInput(options, promptText)
	if err != nil {
		return err
	}

	// Parse selection
	if selected == "(Configure later)" {
		return nil
	}

	r.Options.KubernetesContext = selected

	// Get namespaces
	namespaces := getKubeNamespaces(r.Options.KubernetesContext)
	if len(namespaces) == 0 {
		r.Options.KubernetesNamespace = "default"
		return nil
	}

	// Build namespace options - just the namespace names
	selected, err = r.Prompter.GetListInput(namespaces, "Select Kubernetes namespace:")
	if err != nil {
		return err
	}

	r.Options.KubernetesNamespace = selected
	return nil
}

// createRecipes creates the default recipes file.
func (r *Runner) createRecipes() error {
	recipeFile := fmt.Sprintf("recipes-%s-%s.yaml", r.Options.Platform, r.Options.DeploymentTool)
	r.Options.RecipeFile = filepath.Join(".radius", "config", "recipes", recipeFile)

	content := getRecipesContent(r.Options.Platform, r.Options.DeploymentTool)
	path := filepath.Join(r.WorkDir, r.Options.RecipeFile)

	return os.WriteFile(path, []byte(content), 0644)
}

// createEnvironmentConfig creates the .env file.
func (r *Runner) createEnvironmentConfig() error {
	envContent := "# Radius Environment Configuration\n"
	envContent += "# Generated by 'rad init'\n\n"

	if r.Options.KubernetesContext != "" {
		envContent += "# Kubernetes Configuration\n"
		envContent += fmt.Sprintf("KUBERNETES_CONTEXT=%s\n", r.Options.KubernetesContext)
		envContent += fmt.Sprintf("KUBERNETES_NAMESPACE=%s\n", r.Options.KubernetesNamespace)
		envContent += "\n"
	}

	envContent += "# Recipe Configuration\n"
	envContent += fmt.Sprintf("RECIPES=%s\n", r.Options.RecipeFile)

	return os.WriteFile(filepath.Join(r.WorkDir, ".env"), []byte(envContent), 0644)
}

// displayResults displays the initialization results.
func (r *Runner) displayResults() {
	// Get resource types
	types := getResourceTypeNames()

	r.Output.LogInfo("📦 Resource Types:")
	for _, t := range types {
		r.Output.LogInfo("   • %s", t)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("🌍 Environments:")
	r.Output.LogInfo("   • default (.env):")
	if r.Options.KubernetesContext != "" {
		r.Output.LogInfo("       Kubernetes: context=%s, namespace=%s", r.Options.KubernetesContext, r.Options.KubernetesNamespace)
	}
	r.Output.LogInfo("       Recipes:    %s", r.Options.RecipeFile)

	r.Output.LogInfo("")
	r.Output.LogInfo("🚀 Next steps:")
	r.Output.LogInfo("   1. Commit the initialized configuration:")
	r.Output.LogInfo("      git add .radius && git commit -m \"Initialize Radius\"")
	r.Output.LogInfo("   2. Model your application:")
	r.Output.LogInfo("      rad model")
	r.Output.LogInfo("")
	r.Output.LogInfo("💡 Run 'rad --help' for more commands and options")
}

// Helper functions

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func getCurrentKubeContext() string {
	cmd := exec.Command("kubectl", "config", "current-context")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getKubeContexts() []string {
	cmd := exec.Command("kubectl", "config", "get-contexts", "-o", "name")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var contexts []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			contexts = append(contexts, line)
		}
	}
	return contexts
}

func getKubeNamespaces(context string) []string {
	cmd := exec.Command("kubectl", "--context", context, "get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var namespaces []string
	for _, ns := range strings.Split(string(out), " ") {
		if ns != "" {
			namespaces = append(namespaces, ns)
		}
	}
	sort.Strings(namespaces)
	return namespaces
}

func getResourceTypeNames() []string {
	return []string{
		"Radius.Compute/containers",
		"Radius.Compute/persistentVolumes",
		"Radius.Compute/routes",
		"Radius.Data/mySqlDatabases",
		"Radius.Data/postgreSqlDatabases",
		"Radius.Security/secrets",
	}
}
