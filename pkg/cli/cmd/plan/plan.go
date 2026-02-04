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

package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/git/config"
	"github.com/radius-project/radius/pkg/cli/git/plan"
	"github.com/radius-project/radius/pkg/cli/git/repo"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

// NewCommand creates the `rad plan` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "plan [model.bicep]",
		Short: "Generate deployment artifacts from an application model",
		Long: `Generate deployment artifacts from a Radius application model.

This command analyzes the application model (Bicep file), looks up recipes for 
each resource type, and generates ready-to-deploy artifacts (Terraform or Bicep).

If no model file is specified, rad plan will automatically use the model file
in .radius/model/ if there is only one. If multiple model files exist, you must
specify which one to use.

Use 'rad deploy' after committing the generated artifacts to execute the deployment.`,
		Example: `
# Generate deployment plan (auto-detects model if only one exists)
rad plan

# Generate deployment plan for a specific model file
rad plan .radius/model/myapp.bicep

# Generate plan for a specific environment
rad plan -e production

# Overwrite existing plan files without prompting
rad plan -y

# Allow recipes without pinned versions
rad plan --allow-unpinned-recipes
`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	// Add flags
	cmd.Flags().BoolP("yes", "y", false, "Overwrite existing plan files without prompting")
	cmd.Flags().StringP("environment", "e", "", "Target environment")
	cmd.Flags().Bool("allow-unpinned-recipes", false, "Allow recipes without pinned versions")

	return cmd, runner
}

// Runner implements the rad plan command.
type Runner struct {
	factory  framework.Factory
	Output   output.Interface
	Prompter prompt.Interface

	// ModelPath is the path to the Bicep model file.
	ModelPath string

	// Yes indicates to auto-confirm prompts.
	Yes bool

	// Environment is the target environment.
	Environment string

	// AllowUnpinnedRecipes allows recipes without pinned versions.
	AllowUnpinnedRecipes bool

	// Options contains parsed options.
	Options *Options

	// preservedStateFiles contains terraform state files preserved during cleanup.
	preservedStateFiles map[string][]byte
}

// Options contains parsed options for the plan command.
type Options struct {
	// WorkDir is the Git workspace directory.
	WorkDir string

	// BicepModel is the parsed Bicep model.
	BicepModel *plan.BicepModel

	// Application is the application name.
	Application string

	// Environment is the environment name.
	Environment string

	// EnvironmentFile is the path to the environment file.
	EnvironmentFile string

	// PlanDir is the directory for plan artifacts.
	PlanDir string

	// KubernetesNamespace is the Kubernetes namespace.
	KubernetesNamespace string

	// KubernetesContext is the Kubernetes context.
	KubernetesContext string

	// ResourceTypes is the loaded resource type registry.
	ResourceTypes *config.ResourceTypeRegistry

	// Recipes is the loaded recipe map.
	Recipes map[string]config.Recipe
}

// NewRunner creates a new Runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		factory:             factory,
		preservedStateFiles: make(map[string][]byte),
	}
}

// Validate validates the command arguments and flags.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	r.Output = r.factory.GetOutput()
	r.Prompter = r.factory.GetPrompter()
	r.Yes, _ = cmd.Flags().GetBool("yes")
	r.Environment, _ = cmd.Flags().GetString("environment")
	r.AllowUnpinnedRecipes, _ = cmd.Flags().GetBool("allow-unpinned-recipes")

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Check if this is a Radius Git workspace
	radiusDir := filepath.Join(workDir, ".radius")
	if _, err := os.Stat(radiusDir); os.IsNotExist(err) {
		return &exitError{message: "Not in a Radius Git workspace. Run 'rad init' first."}
	}

	// Resolve model path
	if len(args) > 0 {
		r.ModelPath = args[0]
	} else {
		// Auto-detect model file
		modelPath, err := r.resolveModelPath(workDir)
		if err != nil {
			return err
		}
		r.ModelPath = modelPath
	}

	// Validate model file exists
	if _, err := os.Stat(r.ModelPath); os.IsNotExist(err) {
		return &exitError{message: fmt.Sprintf("Model file not found: %s", r.ModelPath)}
	}

	r.Options = &Options{
		WorkDir: workDir,
	}

	return nil
}

// Run executes the plan command.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("")
	r.Output.LogInfo("📋 Generating deployment plan...")
	r.Output.LogInfo("")

	// Parse Bicep model
	model, err := plan.ParseBicepFile(r.ModelPath)
	if err != nil {
		return &exitError{message: fmt.Sprintf("Failed to parse Bicep model: %v", err)}
	}
	r.Options.BicepModel = model

	// Resolve all connections
	model.ResolveAllConnections()

	// Load resource types for validation
	configDir := filepath.Join(r.Options.WorkDir, ".radius", "config")
	r.Options.ResourceTypes, _ = config.LoadResourceTypes(configDir)

	// Load recipes
	recipesDir := filepath.Join(configDir, "recipes")
	if recipeFiles, err := filepath.Glob(filepath.Join(recipesDir, "*.yaml")); err == nil && len(recipeFiles) > 0 {
		r.Options.Recipes, _ = config.LoadRecipes(recipeFiles)
	}

	// Validate resources against schemas
	if r.Options.ResourceTypes != nil {
		for _, resource := range model.Resources {
			errors := r.Options.ResourceTypes.ValidateResourceProperties(resource.Type, resource.Properties)
			for _, err := range errors {
				r.Output.LogInfo("⚠️  Validation warning: %s", err.Error())
			}
		}
	}

	// Determine application and environment
	r.Options.Application = r.extractApplication(model)
	r.Options.Environment, r.Options.EnvironmentFile = r.extractEnvironment()
	// Plan directory includes application name: .radius/plan/<app>/<env>/
	r.Options.PlanDir = filepath.Join(r.Options.WorkDir, ".radius", "plan", r.Options.Application, r.Options.Environment)

	// Load environment configuration to get recipes path
	envPath := filepath.Join(r.Options.WorkDir, r.Options.EnvironmentFile)
	envConfig, _ := config.LoadEnvironment(envPath)
	if envConfig != nil && len(envConfig.Recipes) > 0 {
		// Load recipes from environment-specified paths
		var recipePaths []string
		for _, recipePath := range envConfig.Recipes {
			if !filepath.IsAbs(recipePath) {
				recipePath = filepath.Join(r.Options.WorkDir, recipePath)
			}
			recipePaths = append(recipePaths, recipePath)
		}
		r.Options.Recipes, _ = config.LoadRecipes(recipePaths)

		// Extract kubernetes config
		if envConfig.Kubernetes != nil {
			r.Options.KubernetesNamespace = envConfig.Kubernetes.Namespace
			r.Options.KubernetesContext = envConfig.Kubernetes.Context
		}
	}

	// Check for existing plan files
	if err := r.checkExistingPlanFiles(); err != nil {
		return err
	}

	// Compute relative model file path
	relModelPath := r.ModelPath
	if filepath.IsAbs(r.ModelPath) {
		if rel, err := filepath.Rel(r.Options.WorkDir, r.ModelPath); err == nil {
			relModelPath = rel
		}
	}

	// Create plan with timestamp
	p := plan.NewPlan(r.Options.Application, relModelPath, r.Options.EnvironmentFile)
	p.GeneratedAt = plan.Timestamp(time.Now().UTC().Format(time.RFC3339Nano))

	// Filter out Application resources (they're not deployable via recipes)
	resources := model.GetOrderedResources()
	var deployableResources []*plan.BicepResource
	for _, resource := range resources {
		// Skip Application resources - they define the application but aren't deployed via recipes
		if strings.Contains(resource.Type, "/applications") {
			continue
		}
		deployableResources = append(deployableResources, resource)
	}

	// Output header with app → env format
	r.Output.LogInfo("📋 Generating deployment plan for %s → %s", r.Options.Application, r.Options.Environment)
	r.Output.LogInfo("")
	r.Output.LogInfo("📦 Found %d resources:", len(deployableResources))
	r.Output.LogInfo("")

	// Generate artifacts for each deployable resource
	for i, resource := range deployableResources {
		// Preserve full resource type with API version (e.g., "Applications.Compute/containers@2023-10-01-preview")
		// Convert Applications.* to Radius.* for consistency
		resourceType := resource.Type
		resourceTypeWithoutVersion := resourceType
		apiVersion := ""
		if idx := strings.Index(resourceType, "@"); idx > 0 {
			resourceTypeWithoutVersion = resourceType[:idx]
			apiVersion = resourceType[idx+1:]
		}
		// Convert Applications.* to Radius.* for display
		if strings.HasPrefix(resourceTypeWithoutVersion, "Applications.") {
			resourceTypeWithoutVersion = "Radius." + strings.TrimPrefix(resourceTypeWithoutVersion, "Applications.")
		}
		// Reconstruct full type with API version
		fullResourceType := resourceTypeWithoutVersion
		if apiVersion != "" {
			fullResourceType = resourceTypeWithoutVersion + "@" + apiVersion
		}

		step := plan.DeploymentStep{
			Sequence: i + 1,
			Resource: plan.ResourceInfo{
				Name:       resource.SymbolicName,
				Type:       fullResourceType,
				Properties: resource.Properties,
			},
			Status: "planned",
		}

		// Look up recipe source from recipes config based on resource type (without version)
		var recipeLocation string
		var recipeKind string
		if r.Options.Recipes != nil {
			if recipe, found := config.LookupRecipe(r.Options.Recipes, resource.Type); found {
				recipeLocation = recipe.RecipeLocation
				recipeKind = recipe.RecipeKind
			}
		}

		// Set recipe info - name is the resource type without version, not "default"
		if recipeLocation != "" {
			step.Recipe = plan.RecipeReference{
				Name:     resourceTypeWithoutVersion,
				Kind:     recipeKind,
				Location: recipeLocation,
			}
		} else if resource.Recipe != nil {
			step.Recipe = plan.RecipeReference{
				Name:     resourceTypeWithoutVersion,
				Kind:     resource.Recipe.Kind,
				Location: resource.Recipe.Source,
			}
		}

		// Generate Terraform artifacts
		stepDir := fmt.Sprintf("%03d-%s-terraform", i+1, resource.SymbolicName)
		outputDir := filepath.Join(r.Options.PlanDir, stepDir)

		// Set deployment artifacts path (relative to workspace)
		relPlanDir := ".radius/plan/" + r.Options.Application + "/" + r.Options.Environment
		step.DeploymentArtifacts = relPlanDir + "/" + stepDir

		generator := plan.NewTerraformGenerator(resource, model, outputDir).
			WithApplication(r.Options.Application).
			WithEnvironment(r.Options.Environment).
			WithKubernetes(r.Options.KubernetesNamespace, r.Options.KubernetesContext)

		// Set recipe source (prefer config lookup over bicep definition)
		if recipeLocation != "" {
			generator.WithRecipeSource(recipeLocation)
		} else if resource.Recipe != nil {
			generator.WithRecipeSource(resource.Recipe.Source)
		}

		if err := generator.Generate(); err != nil {
			return &exitError{message: fmt.Sprintf("Failed to generate artifacts for %s: %v", resource.SymbolicName, err)}
		}

		// Restore preserved state file if available
		r.restoreStateFile(resource.SymbolicName, outputDir)

		// Run terraform init and plan
		r.Output.LogInfo("   📦 %s (%s)", resource.SymbolicName, resourceTypeWithoutVersion)

		planResult, err := generator.InitAndPlan(ctx)
		if err != nil {
			return &exitError{message: fmt.Sprintf("Terraform plan failed for %s: %v", resource.SymbolicName, err)}
		}

		// Set expected changes from terraform plan result
		step.ExpectedChanges = &plan.ExpectedChanges{
			Add:     planResult.Add,
			Change:  planResult.Change,
			Destroy: planResult.Destroy,
		}

		p.AddStep(step)

		if planResult.HasChanges {
			r.Output.LogInfo("      Changes: +%d ~%d -%d", planResult.Add, planResult.Change, planResult.Destroy)
		} else {
			r.Output.LogInfo("      No changes")
		}
	}

	// Update plan summary
	p.UpdateSummary()

	// Write plan.yaml with 4-space indentation to match expected format
	planPath := filepath.Join(r.Options.PlanDir, "plan.yaml")

	var planYAML []byte
	{
		var buf strings.Builder
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(4)
		if err := encoder.Encode(p); err != nil {
			return &exitError{message: fmt.Sprintf("Failed to marshal plan: %v", err)}
		}
		encoder.Close()
		planYAML = []byte(buf.String())
	}

	if err := os.MkdirAll(filepath.Dir(planPath), 0755); err != nil {
		return &exitError{message: fmt.Sprintf("Failed to create plan directory: %v", err)}
	}

	if err := os.WriteFile(planPath, planYAML, 0644); err != nil {
		return &exitError{message: fmt.Sprintf("Failed to write plan.yaml: %v", err)}
	}

	// Output results
	r.Output.LogInfo("")
	r.Output.LogInfo("✅ Plan generated successfully!")
	r.Output.LogInfo("")
	r.Output.LogInfo("📊 Summary:")
	r.Output.LogInfo("   Application: %s", r.Options.Application)
	r.Output.LogInfo("   Environment: %s", r.Options.Environment)
	r.Output.LogInfo("   Steps: %d", len(p.Steps))

	// Show relative paths
	relPlanDir, _ := filepath.Rel(r.Options.WorkDir, r.Options.PlanDir)

	r.Output.LogInfo("")
	r.Output.LogInfo("📁 Artifacts written to: %s", relPlanDir)
	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")

	// Get current git commit for commit message
	gitInfo, _ := repo.GetGitInfo(r.Options.WorkDir)
	commitMsg := fmt.Sprintf("Generate plan for %s (%s)", r.Options.Application, r.Options.Environment)
	if gitInfo != nil && gitInfo.ShortSHA != "" {
		commitMsg = fmt.Sprintf("Plan %s@%s", r.Options.Application, gitInfo.ShortSHA)
	}

	r.Output.LogInfo("   git add %s && git commit -m \"%s\"", relPlanDir, commitMsg)
	r.Output.LogInfo("   rad deploy")

	return nil
}

// resolveModelPath finds the model file automatically.
func (r *Runner) resolveModelPath(workDir string) (string, error) {
	modelDir := filepath.Join(workDir, ".radius", "model")

	files, err := plan.FindBicepFiles(modelDir)
	if err != nil {
		return "", &exitError{message: "❌ No model files found in .radius/model/\n\n   Create a Bicep model file or specify one explicitly:\n     rad plan path/to/model.bicep"}
	}

	if len(files) == 0 {
		return "", &exitError{message: "❌ No model files found in .radius/model/\n\n   Create a Bicep model file or specify one explicitly:\n     rad plan path/to/model.bicep"}
	}

	if len(files) == 1 {
		return files[0], nil
	}

	// Multiple files - prompt user to select
	var options []string
	for _, f := range files {
		rel, _ := filepath.Rel(workDir, f)
		options = append(options, rel)
	}

	selected, err := r.Prompter.GetListInput(options, "Multiple model files found. Select one:")
	if err != nil {
		return "", err
	}

	return filepath.Join(workDir, selected), nil
}

// extractApplication extracts the application name from the model.
func (r *Runner) extractApplication(model *plan.BicepModel) string {
	// Look for application resource or use directory name
	for _, resource := range model.Resources {
		// Check for Application resources (case-insensitive, handles both "Application" and "applications")
		if strings.Contains(strings.ToLower(resource.Type), "application") {
			if resource.Name != "" {
				return resource.Name
			}
		}
	}

	// Default to directory name
	return filepath.Base(r.Options.WorkDir)
}

// extractEnvironment extracts the environment from .env files.
func (r *Runner) extractEnvironment() (string, string) {
	// Check for .env files
	envFiles := []string{".env", ".env.local", ".env.development", ".env.production"}

	for _, envFile := range envFiles {
		envPath := filepath.Join(r.Options.WorkDir, envFile)
		if _, err := os.Stat(envPath); err == nil {
			env := r.getEnvironmentNameFromPath(envFile)
			return env, envFile
		}
	}

	return "default", ".env"
}

// getEnvironmentNameFromPath extracts environment name from env file path.
func (r *Runner) getEnvironmentNameFromPath(envFile string) string {
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

// checkExistingPlanFiles checks for existing plan files and prompts for overwrite.
func (r *Runner) checkExistingPlanFiles() error {
	if _, err := os.Stat(r.Options.PlanDir); os.IsNotExist(err) {
		return nil // No existing plan
	}

	if r.Yes {
		// Preserve state files before cleaning
		r.preserveStateFiles()
		return r.cleanPlanDirectory()
	}

	// Prompt for overwrite
	confirmed, err := prompt.YesOrNoPrompt("Existing plan files found. Overwrite?", prompt.ConfirmNo, r.Prompter)
	if err != nil {
		return err
	}

	if !confirmed {
		return &exitError{message: "Operation cancelled"}
	}

	// Preserve state files before cleaning
	r.preserveStateFiles()
	return r.cleanPlanDirectory()
}

// preserveStateFiles preserves terraform state files before cleanup.
func (r *Runner) preserveStateFiles() {
	entries, err := os.ReadDir(r.Options.PlanDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		stateFile := filepath.Join(r.Options.PlanDir, entry.Name(), "terraform.tfstate")
		if content, err := os.ReadFile(stateFile); err == nil {
			// Extract symbolic name from directory name (e.g., "001-frontend-terraform" -> "frontend")
			parts := strings.Split(entry.Name(), "-")
			if len(parts) >= 2 {
				symbolicName := parts[1]
				r.preservedStateFiles[symbolicName] = content
			}
		}
	}
}

// restoreStateFile restores a preserved state file for a resource.
func (r *Runner) restoreStateFile(symbolicName, outputDir string) {
	if content, ok := r.preservedStateFiles[symbolicName]; ok {
		stateFile := filepath.Join(outputDir, "terraform.tfstate")
		os.WriteFile(stateFile, content, 0644)
	}
}

// cleanPlanDirectory removes existing plan files.
func (r *Runner) cleanPlanDirectory() error {
	entries, err := os.ReadDir(r.Options.PlanDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		path := filepath.Join(r.Options.PlanDir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
	}

	return nil
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

// MarshalJSON implements json.Marshaler for output formatting.
func (r *Runner) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Options)
}
