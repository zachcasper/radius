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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// RadiusConfigDir is the directory name for Radius configuration in repositories.
	RadiusConfigDir = ".radius"

	// TypesFileName is the filename for resource types manifest.
	TypesFileName = "types.yaml"

	// RecipesFileName is the filename for recipes manifest.
	RecipesFileName = "recipes.yaml"

	// EnvironmentFilePrefix is the prefix for environment files.
	EnvironmentFilePrefix = "env."

	// EnvironmentFileSuffix is the suffix for environment files.
	EnvironmentFileSuffix = ".yaml"

	// ModelDir is the subdirectory for application models.
	ModelDir = "model"

	// PlanDir is the subdirectory for deployment plans.
	PlanDir = "plan"

	// DeployDir is the subdirectory for deployment records.
	DeployDir = "deploy"
)

// RadiusRepoConfig provides access to .radius/ configuration files in a repository.
type RadiusRepoConfig struct {
	// RepoRoot is the root directory of the repository.
	RepoRoot string
}

// NewRadiusRepoConfig creates a new RadiusRepoConfig for the given repository root.
func NewRadiusRepoConfig(repoRoot string) *RadiusRepoConfig {
	return &RadiusRepoConfig{RepoRoot: repoRoot}
}

// ConfigDir returns the path to the .radius/ configuration directory.
func (c *RadiusRepoConfig) ConfigDir() string {
	return filepath.Join(c.RepoRoot, RadiusConfigDir)
}

// TypesPath returns the path to .radius/types.yaml.
func (c *RadiusRepoConfig) TypesPath() string {
	return filepath.Join(c.ConfigDir(), TypesFileName)
}

// RecipesPath returns the path to .radius/recipes.yaml.
func (c *RadiusRepoConfig) RecipesPath() string {
	return filepath.Join(c.ConfigDir(), RecipesFileName)
}

// EnvironmentPath returns the path to .radius/env.<name>.yaml for the given environment name.
func (c *RadiusRepoConfig) EnvironmentPath(name string) string {
	return filepath.Join(c.ConfigDir(), EnvironmentFilePrefix+name+EnvironmentFileSuffix)
}

// ModelPath returns the path to .radius/model/<app>.bicep for the given application name.
func (c *RadiusRepoConfig) ModelPath(appName string) string {
	return filepath.Join(c.ConfigDir(), ModelDir, appName+".bicep")
}

// PlanPath returns the path to .radius/plan/<app>/<env>/ for the given application and environment.
func (c *RadiusRepoConfig) PlanPath(appName, envName string) string {
	return filepath.Join(c.ConfigDir(), PlanDir, appName, envName)
}

// PlanFilePath returns the path to .radius/plan/<app>/<env>/plan.yaml.
func (c *RadiusRepoConfig) PlanFilePath(appName, envName string) string {
	return filepath.Join(c.PlanPath(appName, envName), "plan.yaml")
}

// DeployPath returns the path to .radius/deploy/<app>/<env>/<commit>/ for deployment records.
func (c *RadiusRepoConfig) DeployPath(appName, envName, commit string) string {
	return filepath.Join(c.ConfigDir(), DeployDir, appName, envName, commit)
}

// DeploymentRecordPath returns the path to a deployment record JSON file.
func (c *RadiusRepoConfig) DeploymentRecordPath(appName, envName, commit string) string {
	return filepath.Join(c.DeployPath(appName, envName, commit), "deploy-"+commit+".json")
}

// DestructionRecordPath returns the path to a destruction record JSON file.
func (c *RadiusRepoConfig) DestructionRecordPath(appName, envName, commit string) string {
	return filepath.Join(c.DeployPath(appName, envName, commit), "destroy-"+commit+".json")
}

// Exists returns true if the .radius/ configuration directory exists.
func (c *RadiusRepoConfig) Exists() bool {
	info, err := os.Stat(c.ConfigDir())
	return err == nil && info.IsDir()
}

// LoadTypes loads the resource types manifest from .radius/types.yaml.
func (c *RadiusRepoConfig) LoadTypes() (*ResourceTypesManifest, error) {
	return LoadResourceTypesManifest(c.TypesPath())
}

// LoadRecipes loads the recipes manifest from .radius/recipes.yaml.
func (c *RadiusRepoConfig) LoadRecipes() (*RecipesManifest, error) {
	return LoadRecipesManifest(c.RecipesPath())
}

// LoadEnvironment loads an environment configuration from .radius/env.<name>.yaml.
func (c *RadiusRepoConfig) LoadEnvironment(name string) (*Environment, error) {
	return LoadEnvironment(c.EnvironmentPath(name))
}

// LoadPlan loads a deployment plan from .radius/plan/<app>/<env>/plan.yaml.
func (c *RadiusRepoConfig) LoadPlan(appName, envName string) (*DeploymentPlan, error) {
	return LoadDeploymentPlan(c.PlanFilePath(appName, envName))
}

// LoadDeploymentRecord loads a deployment record from .radius/deploy/<app>/<env>/<commit>/deploy-<commit>.json.
func (c *RadiusRepoConfig) LoadDeploymentRecord(appName, envName, commit string) (*DeploymentRecord, error) {
	return LoadDeploymentRecord(c.DeploymentRecordPath(appName, envName, commit))
}

// ListEnvironments returns a list of environment names found in .radius/.
func (c *RadiusRepoConfig) ListEnvironments() ([]string, error) {
	entries, err := os.ReadDir(c.ConfigDir())
	if err != nil {
		return nil, fmt.Errorf("failed to read config directory: %w", err)
	}

	var envs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, EnvironmentFilePrefix) && strings.HasSuffix(name, EnvironmentFileSuffix) {
			// Extract environment name: env.default.yaml -> default
			envName := strings.TrimPrefix(name, EnvironmentFilePrefix)
			envName = strings.TrimSuffix(envName, EnvironmentFileSuffix)
			envs = append(envs, envName)
		}
	}

	return envs, nil
}

// ListApplications returns a list of application names found in .radius/model/.
func (c *RadiusRepoConfig) ListApplications() ([]string, error) {
	modelDir := filepath.Join(c.ConfigDir(), ModelDir)
	entries, err := os.ReadDir(modelDir)
	if os.IsNotExist(err) {
		return nil, nil // No model directory is okay
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read model directory: %w", err)
	}

	var apps []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".bicep") {
			// Extract app name: todolist.bicep -> todolist
			appName := strings.TrimSuffix(name, ".bicep")
			apps = append(apps, appName)
		}
	}

	return apps, nil
}

// EnsureConfigDir creates the .radius/ directory if it doesn't exist.
func (c *RadiusRepoConfig) EnsureConfigDir() error {
	if err := os.MkdirAll(c.ConfigDir(), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	return nil
}

// EnsureModelDir creates the .radius/model/ directory if it doesn't exist.
func (c *RadiusRepoConfig) EnsureModelDir() error {
	modelDir := filepath.Join(c.ConfigDir(), ModelDir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("failed to create model directory: %w", err)
	}
	return nil
}

// EnsurePlanDir creates the .radius/plan/<app>/<env>/ directory if it doesn't exist.
func (c *RadiusRepoConfig) EnsurePlanDir(appName, envName string) error {
	planDir := c.PlanPath(appName, envName)
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return fmt.Errorf("failed to create plan directory: %w", err)
	}
	return nil
}

// EnsureDeployDir creates the .radius/deploy/<app>/<env>/<commit>/ directory if it doesn't exist.
func (c *RadiusRepoConfig) EnsureDeployDir(appName, envName, commit string) error {
	deployDir := c.DeployPath(appName, envName, commit)
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		return fmt.Errorf("failed to create deploy directory: %w", err)
	}
	return nil
}
