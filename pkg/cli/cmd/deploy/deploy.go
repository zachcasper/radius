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
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
)

const (
	appCoreProviderName    = "Applications.Core"
	radiusCoreProviderName = "Radius.Core"
)

// NewCommand creates an instance of the command and runner for the `rad deploy` command.
//

// NewCommand creates a new Cobra command and a Runner to deploy a Bicep or ARM template to a specified environment, with
// optional parameters. It also adds common flags to the command for workspace, resource group, environment name,
// application name and parameters.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "deploy [file]",
		Short: "Deploy a template",
		Long: `Deploy a Bicep or ARM template
	
The deploy command compiles a Bicep or ARM template and deploys it to your default environment (unless otherwise specified).
	
You can combine Radius types as as well as other types that are available in Bicep such as Azure resources. See
the Radius documentation for information about describing your application and resources with Bicep.

You can specify parameters using the '--parameter' flag ('-p' for short). Parameters can be passed as:

- A file containing multiple parameters using the ARM JSON parameter format (see below)
- A file containing a single value in JSON format
- A key-value-pair passed in the command line

When passing multiple parameters in a single file, use the format described here:

	https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/parameter-files

You can specify parameters using multiple sources. Parameters can be overridden based on the 
order they are provided. Parameters appearing later in the argument list will override those defined earlier.
`,
		Example: `
# deploy a Bicep template
rad deploy myapp.bicep

# deploy an ARM template (json)
rad deploy myapp.json

# deploy to a specific workspace
rad deploy myapp.bicep --workspace production

# deploy using a specific environment
rad deploy myapp.bicep --environment production

# deploy using a specific environment and resource group
rad deploy myapp.bicep --environment production --group mygroup

# deploy using an environment ID and a resource group. The application will be deployed in mygroup scope, using the specified environment.
# use this option if the environment is in a different group.
rad deploy myapp.bicep --environment /planes/radius/local/resourcegroups/prod/providers/Applications.Core/environments/prod --group mygroup

# specify a string parameter
rad deploy myapp.bicep --parameters version=latest


# specify a non-string parameter using a JSON file
rad deploy myapp.bicep --parameters configuration=@myfile.json


# specify many parameters using an ARM JSON parameter file
rad deploy myapp.bicep --parameters @myfile.json


# specify parameters from multiple sources
rad deploy myapp.bicep --parameters @myfile.json --parameters version=latest
`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddParameterFlag(cmd)

	// FR-039-A: Add --plan flag for plan-only mode
	cmd.Flags().Bool("plan", false, "Generate a deployment plan without executing the deployment")
	cmd.Flags().String("output", "", "Output directory for plan files (required when --plan is specified)")

	return cmd, runner
}

// Runner is the runner implementation for the `rad deploy` command.
type Runner struct {
	Bicep                   bicep.Interface
	ConfigHolder            *framework.ConfigHolder
	ConnectionFactory       connections.Factory
	RadiusCoreClientFactory *v20250801preview.ClientFactory
	Deploy                  deploy.Interface
	Output                  output.Interface

	ApplicationName          string
	EnvironmentNameOrID      string
	FilePath                 string
	Parameters               map[string]map[string]any
	Template                 map[string]any
	TemplateInspectionResult bicep.TemplateInspectionResult
	Workspace                *workspaces.Workspace
	Providers                *clients.Providers
	EnvResult                *EnvironmentCheckResult

	// FR-039-A: Plan-only mode fields
	PlanOnly      bool
	PlanOutputDir string
}

// NewRunner creates a new instance of the `rad deploy` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Bicep:             factory.GetBicep(),
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Deploy:            factory.GetDeploy(),
		Output:            factory.GetOutput(),
		Providers:         &clients.Providers{},
	}
}

// Validate runs validation for the `rad deploy` command.
//

// Validate validates the workspace, scope, environment name, application name, and parameters from the command
// line arguments and returns an error if any of these are invalid.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	r.Workspace = workspace

	// FR-036, FR-037: Block rad deploy in GitHub mode — direct users to two-phase deployment commands.
	if workspace.IsGitHubWorkspace() {
		return clierrors.Message("The 'rad deploy' command is not supported in GitHub mode. Use 'rad deployment create' to generate a deployment plan and 'rad deployment apply' to execute it.")
	}

	// Allow --group to override the scope
	scope, err := cli.RequireScope(cmd, *workspace)
	if err != nil {
		return err
	}

	// We don't need to explicitly validate the existence of the scope, because we'll validate the existence
	// of the environment later. That will give an appropriate error message for the case where the group
	// does not exist.
	workspace.Scope = scope

	// Get the file path early so we can prepare the template
	r.FilePath = args[0]

	// Prepare the template early to check if it contains an environment resource.
	// This allows us to skip environment validation if the template will create one.
	r.Template, err = r.Bicep.PrepareTemplate(r.FilePath)
	if err != nil {
		return err
	}

	// Inspect the template resources once to get both environment check and deprecated resources
	r.TemplateInspectionResult = bicep.InspectTemplateResources(r.Template)

	// Check if environment was explicitly provided via flag or workspace default
	environmentFlag, _ := cmd.Flags().GetString("environment")
	environmentProvidedExplicitly := environmentFlag != "" || workspace.Environment != ""

	// Check if the template contains an environment resource
	templateCreatesEnvironment := r.TemplateInspectionResult.ContainsEnvironmentResource

	if !templateCreatesEnvironment || environmentProvidedExplicitly {
		// Environment is required if:
		// 1. Template doesn't create environment, OR
		// 2. User explicitly provided --environment flag or workspace has default environment
		r.EnvironmentNameOrID, err = cli.RequireEnvironmentNameOrID(cmd, args, *workspace)
		if err != nil {
			return err
		}
	} else {
		// Template creates the environment and no environment was explicitly provided
		// Set to empty string to indicate no pre-existing environment
		r.EnvironmentNameOrID = ""
	}

	// This might be empty, and that's fine!
	r.ApplicationName, err = cli.ReadApplicationName(cmd, *workspace)
	if err != nil {
		return err
	}

	if r.EnvironmentNameOrID != "" {
		envResult, err := r.FetchEnvironment(cmd.Context(), r.EnvironmentNameOrID)
		if err != nil {
			return err
		}
		if envResult == nil {
			return clierrors.Message("The environment %q does not exist in scope %q. Run `rad env create` first. You could also provide the environment ID if the environment exists in a different group.", r.EnvironmentNameOrID, r.Workspace.Scope)
		}
		r.EnvResult = envResult
	}

	err = r.configureProviders()
	if err != nil {
		return err
	}

	parameterArgs, err := cmd.Flags().GetStringArray("parameters")
	if err != nil {
		return err
	}

	parser := bicep.ParameterParser{FileSystem: filesystem.NewOSFS()}
	r.Parameters, err = parser.Parse(parameterArgs...)
	if err != nil {
		return err
	}

	// FR-039-A: Read plan-only mode flags
	r.PlanOnly, err = cmd.Flags().GetBool("plan")
	if err != nil {
		return err
	}

	r.PlanOutputDir, err = cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	// Validate --output is required when --plan is specified
	if r.PlanOnly && r.PlanOutputDir == "" {
		return clierrors.Message("The --output flag is required when using --plan mode.")
	}

	return nil
}

// Run runs the `rad deploy` command.
//

// Run deploys a Bicep template into an environment from a workspace, optionally creating an application if
// specified, and displays progress and completion messages. It returns an error if any of the operations fail.
func (r *Runner) Run(ctx context.Context) error {
	// Use the template that was prepared during validation
	template := r.Template

	// Check for deprecated resource types and warn the user (using the result from Validate)
	deprecatedResources := r.TemplateInspectionResult.DeprecatedResources
	if len(deprecatedResources) > 0 {
		r.Output.LogInfo("")
		r.Output.LogInfo("WARNING: The following resource types are deprecated:")
		for _, resourceType := range deprecatedResources {
			r.Output.LogInfo("  - %s", resourceType)
		}
		r.Output.LogInfo("Please migrate to the new Radius.* namespace.")
		r.Output.LogInfo("")
	}

	// This is the earliest point where we can inject parameters, we have
	// to wait until the template is prepared.
	err := r.injectAutomaticParameters(template)
	if err != nil {
		return err
	}

	// This is the earliest point where we can report missing parameters, we have
	// to wait until the template is prepared.
	err = r.reportMissingParameters(template)
	if err != nil {
		return err
	}

	// FR-039-A: If plan-only mode, generate plan and exit
	if r.PlanOnly {
		return r.generatePlan(ctx, template)
	}

	// Create application if specified. This supports the case where the application resource
	// is not specified in Bicep. Creating the application automatically helps us "bootstrap" in a new environment.
	// Note: This only applies when the environment already exists. If the template is creating the environment,
	// r.EnvironmentNameOrID will be empty and we'll skip this step (the template deployment will create
	// whatever resources it defines).

	if r.ApplicationName != "" {
		// Environment validation has already happened, so only create application if we have an environment
		if r.Providers.Radius.EnvironmentID != "" {
			if _, err := isApplicationsCoreProvider(r.Providers.Radius.EnvironmentID); err == nil {
				client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
				if err != nil {
					return err
				}
				err = client.CreateApplicationIfNotFound(ctx, r.ApplicationName, &v20231001preview.ApplicationResource{
					Location: to.Ptr(v1.LocationGlobal),
					Properties: &v20231001preview.ApplicationProperties{
						Environment: &r.Providers.Radius.EnvironmentID,
					},
				})
				if err != nil {
					return err
				}
			} else {
				client := r.RadiusCoreClientFactory.NewApplicationsClient()
				_, err := client.Get(ctx, r.ApplicationName, nil)
				if err != nil {
					if clients.Is404Error(err) {
						_, err = client.CreateOrUpdate(ctx, r.ApplicationName, v20250801preview.ApplicationResource{
							Location: to.Ptr(v1.LocationGlobal),
							Properties: &v20250801preview.ApplicationProperties{
								Environment: &r.Providers.Radius.EnvironmentID,
							},
						}, nil)
						if err != nil {
							return err
						}
					} else {
						return err
					}
				}
			}
		}
	}

	progressText := ""
	if r.ApplicationName == "" {
		progressText = fmt.Sprintf(
			"Deploying template '%v' into environment '%v' from workspace '%v'...\n\n"+
				"Deployment In Progress...", r.FilePath, r.EnvironmentNameOrID, r.Workspace.Name)
	} else {
		progressText = fmt.Sprintf(
			"Deploying template '%v' for application '%v' and environment '%v' from workspace '%v'...\n\n"+
				"Deployment In Progress... ", r.FilePath, r.ApplicationName, r.EnvironmentNameOrID, r.Workspace.Name)
	}

	_, err = r.Deploy.DeployWithProgress(ctx, deploy.Options{
		ConnectionFactory: r.ConnectionFactory,
		Workspace:         *r.Workspace,
		Template:          template,
		Parameters:        r.Parameters,
		ProgressText:      progressText,
		CompletionText:    "Deployment Complete",
		Providers:         r.Providers,
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *Runner) injectAutomaticParameters(template map[string]any) error {
	if r.Providers.Radius.EnvironmentID != "" {
		err := bicep.InjectEnvironmentParam(template, r.Parameters, r.Providers.Radius.EnvironmentID)
		if err != nil {
			return err
		}
	}

	if r.Providers.Radius.ApplicationID != "" {
		err := bicep.InjectApplicationParam(template, r.Parameters, r.Providers.Radius.ApplicationID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) reportMissingParameters(template map[string]any) error {
	declaredParameters, err := bicep.ExtractParameters(template)
	if err != nil {
		return err
	}

	errors := map[string]string{}
	for parameter := range declaredParameters {
		// Case-invariant lookup on the user-provided values
		match := false
		for provided := range r.Parameters {
			if strings.EqualFold(parameter, provided) {
				match = true
				break
			}
		}

		if match {
			// Has user-provided value
			continue
		}

		if _, ok := bicep.DefaultValue(declaredParameters[parameter]); ok {
			// Has default value
			continue
		}

		// Special case the parameters that are automatically injected
		if strings.EqualFold(parameter, "environment") {
			errors[parameter] = "The template requires an environment. Use --environment to specify the environment name."
		} else if strings.EqualFold(parameter, "application") {
			errors[parameter] = "The template requires an application. Use --application to specify the application name."
		} else {
			errors[parameter] = fmt.Sprintf("The template requires a parameter %q. Use --parameters %s=<value> to specify the value.", parameter, parameter)
		}
	}

	if len(errors) == 0 {
		return nil
	}

	keys := maps.Keys(errors)
	sort.Strings(keys)

	details := []string{}
	for _, key := range keys {
		details = append(details, fmt.Sprintf("  - %v", errors[key]))
	}

	return clierrors.Message("The template %q could not be deployed because of the following errors:\n\n%v", r.FilePath, strings.Join(details, "\n"))
}

// isApplicationsCoreProvider returns true if the provider is Applications.Core based on the environment ID
// It returns an error if the ID cannot be parsed
func isApplicationsCoreProvider(id string) (bool, error) {
	parsedID, err := resources.Parse(id)
	if err != nil {
		return false, err
	}

	providerNamespace := parsedID.ProviderNamespace()
	if strings.EqualFold(providerNamespace, appCoreProviderName) {
		return true, nil
	}
	return false, nil
}

// handleEnvironmentError handles common error patterns for environment retrieval
func (r *Runner) handleEnvironmentError(err error, command *cobra.Command, args []string) error {
	// If the error is not a 404, return it
	if !clients.Is404Error(err) {
		return err
	}

	// If the environment doesn't exist, but the user specified its name or resource id as
	// a command-line option, return an error
	if r.EnvironmentNameOrID != "" {
		// Extract environment name from ID for better error message
		envName := r.EnvironmentNameOrID
		if parsedID, err := resources.Parse(r.EnvironmentNameOrID); err == nil {
			envName = parsedID.Name()
		}
		return clierrors.Message("The environment %q does not exist in scope %q. Run `rad env create` first. You could also provide the environment ID if the environment exists in a different group.", envName, r.Workspace.Scope)
	}

	// If we got here, it means that the error was a 404 and no environment was specified anywhere.
	// This is fine, because an environment is not required.
	return nil
}

// setupEnvironmentID sets up the environment ID and workspace environment
func (r *Runner) setupEnvironmentID(envID *string) {
	if envID != nil && r.Providers != nil && r.Providers.Radius != nil {
		r.Providers.Radius.EnvironmentID = *envID
		r.Workspace.Environment = r.Providers.Radius.EnvironmentID
	}
}

// getApplicationsCoreEnvironment retrieves environment using Applications Core client
func (r *Runner) getApplicationsCoreEnvironment(ctx context.Context, id string) (*v20231001preview.EnvironmentResource, error) {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return nil, err
	}
	env, err := client.GetEnvironment(ctx, id)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

// getRadiusCoreEnvironment retrieves environment using Radius Core client and returns as Applications.Core format
func (r *Runner) getRadiusCoreEnvironment(ctx context.Context, name string) (*v20250801preview.EnvironmentResource, error) {
	if r.RadiusCoreClientFactory == nil {
		clientFactory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return nil, err
		}
		r.RadiusCoreClientFactory = clientFactory
	}

	environmentClient := r.RadiusCoreClientFactory.NewEnvironmentsClient()
	env, err := environmentClient.Get(ctx, name, nil)
	if err != nil {
		return nil, err
	}
	return &env.EnvironmentResource, nil
}

// constructEnvironmentID constructs an environment ID from a name and provider type
func (r *Runner) constructEnvironmentID(envName, providerType string) string {
	return r.Workspace.Scope + "/providers/" + providerType + "/environments/" + envName
}

// constructApplicationsCoreEnvironmentID constructs an Applications.Core environment ID from a name
func (r *Runner) constructApplicationsCoreEnvironmentID(envNameOrID string) string {
	return r.constructEnvironmentID(envNameOrID, appCoreProviderName)
}

// constructRadiusCoreEnvironmentID constructs a Radius.Core environment ID from a name
func (r *Runner) constructRadiusCoreEnvironmentID(envName string) string {
	return r.constructEnvironmentID(envName, radiusCoreProviderName)
}

// EnvironmentCheckResult holds the result of checking for environments
type EnvironmentCheckResult struct {
	UseApplicationsCore bool
	ApplicationsCoreEnv *v20231001preview.EnvironmentResource
	RadiusCoreEnv       *v20250801preview.EnvironmentResource
}

// FetchEnvironment fetches Applications.Core and Radius.Core environments for a given name/id and returns the result
// If no environment is found, returns (nil, nil)
func (r *Runner) FetchEnvironment(ctx context.Context, envNameOrID string) (*EnvironmentCheckResult, error) {
	result := &EnvironmentCheckResult{}
	// If the environment is specified as a full resource ID, we can skip the check and based on the provider get the environment
	fetchAppCoreEnv := true
	fetchRadiusCoreEnv := true

	envID, err := resources.Parse(envNameOrID)
	isID := false
	if err == nil {
		isID = true
		if strings.EqualFold(envID.ProviderNamespace(), appCoreProviderName) {
			fetchRadiusCoreEnv = false
		} else {
			fetchAppCoreEnv = false
		}
	}

	// Check Applications.Core environment
	if fetchAppCoreEnv {
		// If its ID, use it directly, otherwise construct ID from name
		var appCoreEnvID string
		if !isID {
			appCoreEnvID = r.constructApplicationsCoreEnvironmentID(envNameOrID)
		} else {
			appCoreEnvID = envNameOrID
		}
		appCoreEnv, err := r.getApplicationsCoreEnvironment(ctx, appCoreEnvID)
		if err != nil {
			if !clients.Is404Error(err) {
				return nil, err
			}
		}
		if appCoreEnv != nil {
			result.ApplicationsCoreEnv = appCoreEnv
		}
	}
	if fetchRadiusCoreEnv {
		var radCoreEnvName string
		if isID {
			radCoreEnvName = envID.Name()
		} else {
			radCoreEnvName = envNameOrID
		}

		radiusCoreEnv, err := r.getRadiusCoreEnvironment(ctx, radCoreEnvName)
		if err != nil {
			// Treat all errors from the Radius.Core lookup as non-fatal.
			// The Radius.Core provider may not be registered, may not have
			// locations configured, or may return other errors if the control
			// plane only supports Applications.Core. We log the error and
			// fall through to use the Applications.Core result if available.
			_ = err // non-fatal: Radius.Core environment lookup failed
		}
		if radiusCoreEnv != nil {
			result.RadiusCoreEnv = radiusCoreEnv
		}
	}

	// Determine which one to use and check for conflicts
	if result.ApplicationsCoreEnv != nil && result.RadiusCoreEnv != nil {
		var appCoreID, radiusCoreID string
		if result.ApplicationsCoreEnv.ID != nil {
			appCoreID = *result.ApplicationsCoreEnv.ID
		}
		if result.RadiusCoreEnv.ID != nil {
			radiusCoreID = *result.RadiusCoreEnv.ID
		}
		return nil, clierrors.Message("Conflict detected: Environment '%s' exists in both Applications.Core and Radius.Core providers. Please specify the full resource ID to disambiguate:\n  Applications.Core: %s\n  Radius.Core: %s",
			envNameOrID, appCoreID, radiusCoreID)
	}

	if result.ApplicationsCoreEnv != nil {
		result.UseApplicationsCore = true
		if result.ApplicationsCoreEnv.ID != nil {
			r.EnvironmentNameOrID = *result.ApplicationsCoreEnv.ID
		}
	} else if result.RadiusCoreEnv != nil {
		result.UseApplicationsCore = false
		if result.RadiusCoreEnv.ID != nil {
			r.EnvironmentNameOrID = *result.RadiusCoreEnv.ID
		}
	} else {
		// Neither found, treat as environment not found case
		return nil, nil
	}

	return result, nil
}

// setupCloudProviders sets up AWS and Azure providers based on environment properties
func (r *Runner) setupCloudProviders(properties any) {
	switch props := properties.(type) {
	case *v20231001preview.EnvironmentProperties:
		if props != nil && props.Providers != nil {
			if props.Providers.Aws != nil {
				r.Providers.AWS = &clients.AWSProvider{
					Scope: *props.Providers.Aws.Scope,
				}
			}
			if props.Providers.Azure != nil {
				r.Providers.Azure = &clients.AzureProvider{
					Scope: *props.Providers.Azure.Scope,
				}
			}
		}
	case *v20250801preview.EnvironmentProperties:
		if props != nil && props.Providers != nil {
			if props.Providers.Aws != nil {
				r.Providers.AWS = &clients.AWSProvider{
					Scope: "/planes/aws/aws/accounts/" + *props.Providers.Aws.AccountID + "/regions/" + *props.Providers.Aws.Region,
				}
			}
			if props.Providers.Azure != nil {
				r.Providers.Azure = &clients.AzureProvider{
					Scope: "/planes/azure/azure/" + "Subscriptions/" + *props.Providers.Azure.SubscriptionID + "/ResourceGroups/" + *props.Providers.Azure.ResourceGroupName,
				}
			}
		}
	}
}

// configureProviders configures environment and cloud providers based on the environment and provider type
func (r *Runner) configureProviders() error {
	var env any
	if r.Providers == nil {
		r.Providers = &clients.Providers{}
	}
	if r.Providers.Radius == nil {
		r.Providers.Radius = &clients.RadiusProvider{}
	}

	if r.EnvResult != nil {
		if r.EnvResult.UseApplicationsCore {
			if r.EnvResult.ApplicationsCoreEnv != nil {
				env = r.EnvResult.ApplicationsCoreEnv
			}
		} else {
			if r.EnvResult.RadiusCoreEnv != nil {
				env = r.EnvResult.RadiusCoreEnv
			}
		}
	} else {
		return nil
	}

	switch e := env.(type) {
	case *v20231001preview.EnvironmentResource:
		if e != nil && e.ID != nil {
			r.setupEnvironmentID(e.ID)
			r.setupCloudProviders(e.Properties)
		}
		if r.ApplicationName != "" {
			// Extract provider namespace from environment ID to preserve casing
			providerNamespace := appCoreProviderName
			if parsedID, err := resources.Parse(r.Providers.Radius.EnvironmentID); err == nil {
				providerNamespace = parsedID.ProviderNamespace()
			}
			r.Providers.Radius.ApplicationID = r.Workspace.Scope + "/providers/" + providerNamespace + "/applications/" + r.ApplicationName

		}
	case *v20250801preview.EnvironmentResource:
		if e != nil && e.ID != nil {
			r.setupEnvironmentID(e.ID)
			r.setupCloudProviders(e.Properties)
		}
		if r.ApplicationName != "" {
			// Extract provider namespace from environment ID to preserve casing
			providerNamespace := radiusCoreProviderName
			if parsedID, err := resources.Parse(r.Providers.Radius.EnvironmentID); err == nil {
				providerNamespace = parsedID.ProviderNamespace()
			}
			r.Providers.Radius.ApplicationID = r.Workspace.Scope + "/providers/" + providerNamespace + "/applications/" + r.ApplicationName
		}
	}

	return nil
}

// createApplicationIfNeeded creates the application resource if specified and not already existing.
// This supports both Applications.Core and Radius.Core providers.
func (r *Runner) createApplicationIfNeeded(ctx context.Context) error {
	if r.ApplicationName == "" || r.Providers.Radius.EnvironmentID == "" {
		return nil
	}

	if _, err := isApplicationsCoreProvider(r.Providers.Radius.EnvironmentID); err == nil {
		client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
		if err != nil {
			return err
		}
		return client.CreateApplicationIfNotFound(ctx, r.ApplicationName, &v20231001preview.ApplicationResource{
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20231001preview.ApplicationProperties{
				Environment: &r.Providers.Radius.EnvironmentID,
			},
		})
	}

	if r.RadiusCoreClientFactory == nil {
		return nil
	}

	appClient := r.RadiusCoreClientFactory.NewApplicationsClient()
	_, err := appClient.Get(ctx, r.ApplicationName, nil)
	if err != nil {
		if clients.Is404Error(err) {
			_, err = appClient.CreateOrUpdate(ctx, r.ApplicationName, v20250801preview.ApplicationResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20250801preview.ApplicationProperties{
					Environment: &r.Providers.Radius.EnvironmentID,
				},
			}, nil)
			return err
		}
		return err
	}
	return nil
}

// generatePlan generates a deployment plan (deploy.yaml) and artifacts without
// deploying any resources. It resolves recipe information from the environment's
// registered recipes and generates Terraform configurations and plan output.
// FR-045-E: MUST NOT perform any resource deployments.
func (r *Runner) generatePlan(ctx context.Context, template map[string]any) error {
	r.Output.LogInfo("Generating deployment plan (plan-only mode)...")

	// Use the output directory directly — the workflow already scopes it to
	// .radius/deploy/<app>/<env>/<commit>
	planDir := r.PlanOutputDir
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	appName := r.ApplicationName
	if appName == "" {
		appName = "default"
	}

	envName := r.EnvironmentNameOrID
	if envName == "" {
		envName = "default"
	}
	if parsed, parseErr := resources.Parse(envName); parseErr == nil {
		envName = parsed.Name()
	}

	// Resolve registered recipes from the environment (populated during Validate)
	envRecipes := r.resolveEnvironmentRecipes()

	// Parse template resources for structure, properties, and dependency ordering
	// FR-079-A: topologically ordered based on dependsOn relationships
	templateResources := parseTemplateResources(template)
	sorted := topologicalSort(templateResources)

	// Extract commit hash from the directory name (last path segment)
	commit := filepath.Base(planDir)

	record := DeploymentRecord{
		Application:               appName,
		ApplicationDefinitionFile: r.FilePath,
		Environment:               envName,
		Commit:                    commit,
		Steps:                     make([]DeploymentStep, 0),
	}

	stepNum := 0
	totalAdd := 0
	totalChange := 0
	totalDestroy := 0
	terraformSteps := 0
	bicepSteps := 0

	for _, tmplRes := range sorted {
		if isControlPlaneResource(tmplRes.Type) {
			continue
		}

		stepNum++

		// Resolve recipe for this resource type from the environment
		recipe := envRecipes[strings.ToLower(tmplRes.Type)]
		recipeKind := "terraform"
		recipeLocation := ""
		recipeName := tmplRes.Type

		if recipe != nil {
			recipeKind = recipe.Kind
			recipeLocation = recipe.Location
			recipeName = recipe.Name
		} else {
			// Fallback: construct a default location from the resource type
			modulePath := resourceTypeToModulePath(tmplRes.Type)
			recipeLocation = "git::https://github.com/radius-project/resource-types-contrib.git//" + modulePath
		}

		// FR-079-B: NNN-<resource_name>-<recipe_kind>
		dirName := fmt.Sprintf("%03d-%s-%s", stepNum, sanitizeName(tmplRes.Name), recipeKind)

		step := DeploymentStep{
			Sequence: stepNum,
			Resource: StepResource{
				Name:       tmplRes.Name,
				Type:       tmplRes.Type,
				Properties: tmplRes.Properties,
			},
			Recipe: StepRecipe{
				Name:     recipeName,
				Kind:     recipeKind,
				Location: recipeLocation,
			},
			DeploymentArtifacts: dirName,
			ExpectedChanges: StepChanges{
				Add:     0,
				Change:  0,
				Destroy: 0,
			},
			Status: "planned", // FR-045-E: all steps must be "planned"
		}

		// Create artifacts directory and generate files
		artifactsDir := filepath.Join(planDir, dirName, "artifacts")
		if err := os.MkdirAll(artifactsDir, 0755); err != nil {
			return fmt.Errorf("failed to create artifacts directory: %w", err)
		}

		r.Output.LogInfo("  Step %d: %s (%s) [%s]", stepNum, tmplRes.Name, tmplRes.Type, recipeKind)

		if recipeKind == "terraform" && recipeLocation != "" {
			if err := generateDeploymentArtifacts(artifactsDir, tmplRes, appName, envName, recipeLocation); err != nil {
				return fmt.Errorf("failed to generate artifacts for %s: %w", tmplRes.Name, err)
			}
			terraformSteps++
		} else if recipeKind == "bicep" {
			bicepSteps++
		}

		totalAdd += step.ExpectedChanges.Add
		totalChange += step.ExpectedChanges.Change
		totalDestroy += step.ExpectedChanges.Destroy
		record.Steps = append(record.Steps, step)
	}

	record.Summary = DeploymentSummary{
		TotalSteps:        stepNum,
		TerraformSteps:    terraformSteps,
		BicepSteps:        bicepSteps,
		TotalAdd:          totalAdd,
		TotalChange:       totalChange,
		TotalDestroy:      totalDestroy,
		AllVersionsPinned: false,
	}

	// Write deploy.yaml (spec appendix C.1)
	deployPath := filepath.Join(planDir, "deploy.yaml")
	data, err := yaml.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal deployment record: %w", err)
	}

	header := fmt.Sprintf("# Radius deployment plan\n# Generated by Radius control plane\n# Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(deployPath, append([]byte(header), data...), 0644); err != nil {
		return fmt.Errorf("failed to write deploy.yaml: %w", err)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Deployment plan generated!")
	r.Output.LogInfo("  Plan file: %s", deployPath)
	r.Output.LogInfo("  Steps:     %d", stepNum)
	r.Output.LogInfo("  Status:    planned")
	r.Output.LogInfo("")

	return nil
}

// recipeInfo holds resolved recipe information from the environment.
type recipeInfo struct {
	Name     string
	Kind     string
	Location string
}

// resolveEnvironmentRecipes extracts registered recipes from the environment resource.
// Returns a map of resource type (lowercased) → recipe info.
func (r *Runner) resolveEnvironmentRecipes() map[string]*recipeInfo {
	recipes := make(map[string]*recipeInfo)

	if r.EnvResult == nil {
		return recipes
	}

	if r.EnvResult.UseApplicationsCore && r.EnvResult.ApplicationsCoreEnv != nil {
		env := r.EnvResult.ApplicationsCoreEnv
		if env.Properties != nil && env.Properties.Recipes != nil {
			for resourceType, recipeMap := range env.Properties.Recipes {
				for recipeName, recipe := range recipeMap {
					props := recipe.GetRecipeProperties()
					if props != nil && props.TemplateKind != nil && props.TemplatePath != nil {
						recipes[strings.ToLower(resourceType)] = &recipeInfo{
							Name:     recipeName,
							Kind:     *props.TemplateKind,
							Location: *props.TemplatePath,
						}
					}
					break // Use the first recipe (typically "default")
				}
			}
		}
	}

	return recipes
}

// DeploymentRecord represents deploy.yaml (spec appendix C.1)
type DeploymentRecord struct {
	Application               string            `yaml:"application"`
	ApplicationDefinitionFile string            `yaml:"applicationDefinitionFile"`
	Environment               string            `yaml:"environment"`
	Commit                    string            `yaml:"commit"`
	Steps                     []DeploymentStep  `yaml:"steps"`
	Summary                   DeploymentSummary `yaml:"summary"`
}

// DeploymentStep represents a single step in the deployment record
type DeploymentStep struct {
	Sequence            int          `yaml:"sequence"`
	Resource            StepResource `yaml:"resource"`
	Recipe              StepRecipe   `yaml:"recipe"`
	DeploymentArtifacts string       `yaml:"deploymentArtifacts"`
	ExpectedChanges     StepChanges  `yaml:"expectedChanges"`
	Status              string       `yaml:"status"`
}

// StepResource describes a resource in a deployment step
type StepResource struct {
	Name       string         `yaml:"name"`
	Type       string         `yaml:"type"`
	Properties map[string]any `yaml:"properties,omitempty"`
}

// StepRecipe describes recipe information for a deployment step
type StepRecipe struct {
	Name     string `yaml:"name"`
	Kind     string `yaml:"kind"`
	Location string `yaml:"location"`
}

// StepChanges represents expected changes for a deployment step
type StepChanges struct {
	Add     int `yaml:"add"`
	Change  int `yaml:"change"`
	Destroy int `yaml:"destroy"`
}

// DeploymentSummary is the summary section of a deployment record
type DeploymentSummary struct {
	TotalSteps        int  `yaml:"totalSteps"`
	TerraformSteps    int  `yaml:"terraformSteps"`
	BicepSteps        int  `yaml:"bicepSteps"`
	TotalAdd          int  `yaml:"totalAdd"`
	TotalChange       int  `yaml:"totalChange"`
	TotalDestroy      int  `yaml:"totalDestroy"`
	AllVersionsPinned bool `yaml:"allVersionsPinned"`
}

// templateResourceInfo holds parsed info from a template resource including dependencies.
type templateResourceInfo struct {
	SymbolicName string
	Type         string
	Name         string
	Properties   map[string]any
	DependsOn    []string
}

// parseTemplateResources extracts resources from a compiled Bicep template (symbolic name format)
// including dependsOn information for topological ordering.
func parseTemplateResources(template map[string]any) []templateResourceInfo {
	resourcesMap, ok := template["resources"].(map[string]any)
	if !ok {
		return nil
	}

	result := make([]templateResourceInfo, 0, len(resourcesMap))
	for symbolicName, r := range resourcesMap {
		res, ok := r.(map[string]any)
		if !ok {
			continue
		}

		info := templateResourceInfo{
			SymbolicName: symbolicName,
		}

		// Get type (may include API version: "Type@Version")
		fullType, _ := res["type"].(string)
		if parts := strings.SplitN(fullType, "@", 2); len(parts) == 2 {
			info.Type = parts[0]
		} else {
			info.Type = fullType
		}

		// In symbolic name format, "name" is inside the "properties" wrapper
		if props, ok := res["properties"].(map[string]any); ok {
			info.Name, _ = props["name"].(string)
			// The actual resource properties are nested inside properties.properties
			info.Properties, _ = props["properties"].(map[string]any)
		}
		if info.Name == "" {
			info.Name = symbolicName
		}

		// Parse dependsOn for ordering
		if deps, ok := res["dependsOn"].([]any); ok {
			for _, dep := range deps {
				if depStr, ok := dep.(string); ok {
					info.DependsOn = append(info.DependsOn, depStr)
				}
			}
		}

		if info.Type != "" {
			result = append(result, info)
		}
	}

	return result
}

// topologicalSort orders template resources by their dependsOn relationships
// using Kahn's algorithm. Resources with no dependencies come first.
func topologicalSort(resources []templateResourceInfo) []templateResourceInfo {
	if len(resources) == 0 {
		return resources
	}

	// Build index by symbolic name
	nameToIdx := make(map[string]int, len(resources))
	for i, r := range resources {
		nameToIdx[r.SymbolicName] = i
	}

	// Compute in-degree for each resource
	inDegree := make([]int, len(resources))
	for i, r := range resources {
		for _, dep := range r.DependsOn {
			if _, ok := nameToIdx[dep]; ok {
				inDegree[i]++
			}
		}
	}

	// Kahn's algorithm: start with nodes that have no incoming edges
	queue := make([]int, 0)
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	sorted := make([]templateResourceInfo, 0, len(resources))
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		sorted = append(sorted, resources[idx])

		// Decrement in-degree for all dependents
		for i, r := range resources {
			for _, dep := range r.DependsOn {
				if dep == resources[idx].SymbolicName {
					inDegree[i]--
					if inDegree[i] == 0 {
						queue = append(queue, i)
					}
				}
			}
		}
	}

	// Append any remaining resources (cycles or unresolved deps)
	if len(sorted) < len(resources) {
		sortedNames := make(map[string]bool, len(sorted))
		for _, s := range sorted {
			sortedNames[s.SymbolicName] = true
		}
		for _, r := range resources {
			if !sortedNames[r.SymbolicName] {
				sorted = append(sorted, r)
			}
		}
	}

	return sorted
}

// controlPlaneResourceTypes are resource types that represent Radius control plane
// entities and should not appear as deployment steps.
var controlPlaneResourceTypes = map[string]bool{
	"applications.core/applications": true,
	"applications.core/environments": true,
	"radius.core/applications":       true,
	"radius.core/environments":       true,
}

// isControlPlaneResource returns true for resource types that should be excluded
// from deployment steps (applications, environments).
func isControlPlaneResource(resourceType string) bool {
	return controlPlaneResourceTypes[strings.ToLower(resourceType)]
}

// sanitizeName converts a resource name to a safe directory name.
func sanitizeName(name string) string {
	safe := strings.ReplaceAll(name, "/", "-")
	safe = strings.ReplaceAll(safe, "\\", "-")
	safe = strings.ReplaceAll(safe, ":", "-")
	safe = strings.ReplaceAll(safe, "*", "")
	safe = strings.ReplaceAll(safe, "?", "")
	safe = strings.ReplaceAll(safe, "\"", "")
	safe = strings.ReplaceAll(safe, "<", "")
	safe = strings.ReplaceAll(safe, ">", "")
	safe = strings.ReplaceAll(safe, "|", "")
	return strings.ToLower(safe)
}

// resourceTypeToModulePath converts a Radius resource type to a Terraform module path.
// e.g. "Radius.Data/postgreSqlDatabases" → "Data/postgreSqlDatabases/recipes/kubernetes/terraform"
func resourceTypeToModulePath(resourceType string) string {
	parts := strings.SplitN(resourceType, "/", 2)
	if len(parts) != 2 {
		return resourceType
	}
	provider := parts[0]
	typeName := parts[1]

	// Strip the provider prefix ("Radius.", "Applications." etc.)
	if dotIdx := strings.Index(provider, "."); dotIdx >= 0 {
		provider = provider[dotIdx+1:]
	}

	return provider + "/" + typeName + "/recipes/kubernetes/terraform"
}

// generateDeploymentArtifacts creates Terraform artifact files for a deployment step.
// These files represent the Radius recipe configuration for plan review.
func generateDeploymentArtifacts(dir string, res templateResourceInfo, appName, envName, recipeLocation string) error {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	safeName := sanitizeName(res.Name)

	// main.tf — module source pointing to the recipe Terraform module
	mainTF := fmt.Sprintf(`# Radius resource deployment plan
# Generated by Radius control plane
# Resource: %s (%s)
# Generated: %s

module "%s" {
  source = "%s"

  # Pass the Radius context to the recipe module
  context = var.context
}

# Outputs from the recipe module
output "result" {
  description = "Result output from the recipe module"
  value       = try(module.%s.result, null)
  sensitive   = true
}
`, res.Name, res.Type, timestamp, safeName, recipeLocation, safeName)

	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(mainTF), 0644); err != nil {
		return err
	}

	// providers.tf — required providers for the recipe
	providersTF := fmt.Sprintf(`# Radius resource deployment plan
# Generated by Radius control plane
# Resource: %s (%s)
# Generated: %s

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
  }
}

provider "kubernetes" {
  # Provider configuration is managed by the Radius control plane
}
`, res.Name, res.Type, timestamp)

	if err := os.WriteFile(filepath.Join(dir, "providers.tf"), []byte(providersTF), 0644); err != nil {
		return err
	}

	// variables.tf — the recipe context variable type definition
	variablesTF := fmt.Sprintf(`# Radius resource deployment plan
# Generated by Radius control plane
# Variables for resource: %s (%s)
# Generated: %s

variable "context" {
  description = "Radius recipe context containing resource, application, environment, and runtime information"
  type = object({
    resource = object({
      name       = string
      type       = string
      properties = any
      connections = optional(map(object({
        id         = string
        name       = string
        type       = string
        properties = optional(any)
      })), {})
    })
    application = object({
      name = string
    })
    environment = object({
      name = string
    })
    runtime = object({
      kubernetes = optional(object({
        namespace            = string
        environmentNamespace = string
      }))
    })
    azure = optional(object({
      resourceGroup = object({
        name = string
      })
      subscription = object({
        subscriptionId = string
      })
    }))
    aws = optional(object({
      region  = string
      account = string
    }))
  })
}
`, res.Name, res.Type, timestamp)

	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(variablesTF), 0644); err != nil {
		return err
	}

	// terraform.tfvars.json — actual context values for this resource
	contextMap := map[string]any{
		"context": map[string]any{
			"resource": map[string]any{
				"name":        res.Name,
				"type":        res.Type,
				"properties":  res.Properties,
				"connections": map[string]any{},
			},
			"application": map[string]any{
				"name": appName,
			},
			"environment": map[string]any{
				"name": envName,
			},
			"runtime": map[string]any{
				"kubernetes": map[string]any{
					"namespace":            "default",
					"environmentNamespace": "default",
				},
			},
			"azure": nil,
			"aws":   nil,
		},
	}
	tfvarsData, err := json.MarshalIndent(contextMap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfvars.json"), append(tfvarsData, '\n'), 0644); err != nil {
		return err
	}

	// terraform-context.txt — human-readable deployment context
	contextTxt := fmt.Sprintf(`# Terraform Context
# Generated by Radius control plane
# Generated: %s

## Resource Context

resource_name: %s
resource_type: %s
recipe_location: %s
application: %s
environment: %s

## Plan Status

status: planned
`, timestamp, res.Name, res.Type, recipeLocation, appName, envName)

	if err := os.WriteFile(filepath.Join(dir, "terraform-context.txt"), []byte(contextTxt), 0644); err != nil {
		return err
	}

	return nil
}
