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

package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

const (
	// DefaultModelName is the default application model name
	DefaultModelName = "todolist"

	// ModelFileExtension is the file extension for model files
	ModelFileExtension = ".bicep"
)

// todolistBicepTemplate is the sample application model template
const todolistBicepTemplate = `extension radius

param environment string 

resource todolist 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'todolist'
  properties: {
    environment: environment
  }
}

resource frontend 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'frontend'
  properties: {
    application: todolist.id
    environment: environment
    containers: {
      frontend: {
        image: 'ghcr.io/radius-project/samples/demo:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
    }
    connections: {
      postgresql:{
        source: db.id
      }
    }
  }
}

resource db 'Radius.Data/postgreSqlDatabases@2025-08-01-preview' = {
  name: 'db'
  properties: {
    environment: environment
    application: todolist.id
    size: 'S'
  }
}
`

// NewCommand creates an instance of the command and runner for the ` + "`rad model`" + ` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	cmd := &cobra.Command{
		Use:   "model",
		Short: "Create an application model",
		Long: `Create a sample application model file for Radius.
This command generates a starter application model at .radius/model/todolist.bicep
containing a sample application with a frontend container and PostgreSQL database.
This is a placeholder for future AI-assisted modeling functionality.`,
		Example: `# Create a sample application model
rad model
# The generated model will be at .radius/model/todolist.bicep`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	cmd.Flags().BoolP("yes", "y", false, "Overwrite existing model file without prompting")

	return cmd, runner
}

// NewRunner creates a new instance of the ` + "`rad model`" + ` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		Prompter:     factory.GetPrompter(),
	}
}

// Runner is the runner implementation for the ` + "`rad model`" + ` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Prompter     prompt.Interface
	Workspace    *workspaces.Workspace
	Yes          bool
}

// Validate runs validation for the ` + "`rad model`" + ` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Load workspace
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	// Validate that this is a GitHub workspace
	if workspace.Connection == nil {
		return clierrors.Message("Current workspace is not configured. Run 'rad init --github' to configure a GitHub workspace.")
	}

	kind, ok := workspace.Connection["kind"].(string)
	if !ok || kind != workspaces.KindGitHub {
		return clierrors.Message("This command requires a GitHub workspace. Current workspace kind is '%s'. Run 'rad init --github' to configure a GitHub workspace.", kind)
	}

	r.Workspace = workspace

	// Get --yes flag
	r.Yes, _ = cmd.Flags().GetBool("yes")

	// Verify we're in a git repository with .radius/ directory
	cwd, err := os.Getwd()
	if err != nil {
		return clierrors.Message("Failed to get current directory: %v", err)
	}

	radiusDir := filepath.Join(cwd, ".radius")
	if _, err := os.Stat(radiusDir); os.IsNotExist(err) {
		return clierrors.Message("Radius not initialized. Run 'rad init' first to initialize the repository.")
	}

	return nil
}

// Run executes the ` + "`rad model`" + ` command.
func (r *Runner) Run(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return clierrors.Message("Failed to get current directory: %v", err)
	}

	// Create .radius/model/ directory
	modelDir := filepath.Join(cwd, ".radius", "model")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return clierrors.Message("Failed to create model directory: %v", err)
	}

	// Check if model file already exists
	modelFile := filepath.Join(modelDir, DefaultModelName+ModelFileExtension)
	if _, err := os.Stat(modelFile); err == nil {
		// File exists, prompt for overwrite
		if !r.Yes {
			overwrite, err := r.Prompter.GetListInput([]string{"Yes", "No"}, fmt.Sprintf("Model file %s already exists. Overwrite?", modelFile))
			if err != nil {
				return err
			}
			if overwrite != "Yes" {
				r.Output.LogInfo("Model creation cancelled.")
				return nil
			}
		}
	}

	// Write the model file
	r.Output.LogInfo("Creating application model...")
	if err := os.WriteFile(modelFile, []byte(todolistBicepTemplate), 0644); err != nil {
		return clierrors.Message("Failed to write model file: %v", err)
	}
	r.Output.LogInfo("Created: %s", modelFile)

	// Commit the changes
	gitHelper, err := github.NewGitHelper(cwd)
	if err != nil {
		return clierrors.Message("Failed to initialize git helper: %v", err)
	}

	// go-git requires relative path from repository root
	relativeModelPath := filepath.Join(".radius", "model", DefaultModelName+ModelFileExtension)
	if err := gitHelper.Add(relativeModelPath); err != nil {
		return clierrors.Message("Failed to stage model file: %v", err)
	}

	_, err = gitHelper.CommitWithTrailer("Add application model", "model")
	if err != nil {
		return clierrors.Message("Failed to commit changes: %v", err)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("✓ Application model created successfully!")
	r.Output.LogInfo("")
	r.Output.LogInfo("The model contains a sample 'todolist' application with:")
	r.Output.LogInfo("  - A frontend container (Radius.Compute/containers)")
	r.Output.LogInfo("  - A PostgreSQL database (Radius.Data/postgreSqlDatabases)")
	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("  1. Edit the model to define your application resources")
	r.Output.LogInfo("  2. Run 'rad pr create' to create a deployment plan")

	return nil
}
