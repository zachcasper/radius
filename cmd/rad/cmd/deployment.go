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

package cmd

import (
	"github.com/spf13/cobra"
)

// NewDeploymentCommand creates the `rad deployment` command group for two-phase deployments in GitHub mode.
func NewDeploymentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deployment",
		Short: "Manage deployments",
		Long: `Manage deployments using the two-phase deployment model.

Use 'rad deployment create' to generate a deployment plan and 'rad deployment apply' to execute it.
This command group is used with GitHub-mode workspaces where deployments execute via GitHub Actions.`,
	}
}
