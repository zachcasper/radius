# Feature Specification: Radius on GitHub

**Feature Branch**: `001-github-mode`  
**Created**: 2026-02-12  
**Status**: Draft  
**Input**: User description: "Radius on GitHub - a new mode of operation where all data and processing runs in a GitHub repository, complementing the existing Radius on Kubernetes mode"

## Clarifications

### Session 2026-02-12

- Q: Where should Terraform state be stored for deployments executed in GitHub Actions? → A: Cloud backend (S3 for AWS, Azure Storage for Azure) with OIDC authentication; future enhancement to allow custom existing backends.
- Q: How should concurrent deployments to the same environment be handled? → A: GitHub Actions concurrency groups queue subsequent runs; see Session 2026-02-15 clarification for details.
- Q: What happens when a deployment partially fails? → A: Leave successfully deployed resources in place; report which resources failed; user decides whether to fix and re-deploy or destroy.
- Q: What scope should resource destruction target when multiple applications exist in an environment? → A: Single application only; require `--application` flag to specify which application to destroy.
- Q: How should sensitive values (API keys, connection strings) be handled in configuration? → A: Reference GitHub Secrets by name in configuration; workflow injects values at runtime.

### Session 2026-02-15

- Q: How should the CLI behave while a dispatched GitHub Action workflow runs? → A: CLI shows contextual message (e.g., "Creating deployment..."), then displays an animated progress indicator with automatic step-level status updates showing each workflow step's progress.
- Q: How should concurrent deployments to the same app/environment be locked? → A: GitHub Actions concurrency groups using the `concurrency:` key in workflow YAML scoped to app-env.
- Q: How does `rad app delete` execute in GitHub mode? → A: Dispatches the `rad-app-delete.yaml` GitHub Action workflow that runs destroy operations using cloud OIDC credentials.
- Q: Should `rad environment create` verify that OIDC authentication works after setup? → A: Yes, auto-verify by dispatching a lightweight GitHub Action workflow that authenticates to the cloud provider via OIDC and reports success/failure.

## Overview

Radius on GitHub is a new operational mode that enables users to deploy cloud applications using Radius without requiring a centralized Kubernetes-based control plane. Environment configuration is stored via the GitHub Environments API, application definitions are stored in the repository, and deployments execute in ephemeral k3d clusters within GitHub Action runners. Deployment functionality is the same as today's Radius — `rad deploy` deploys applications and `rad app delete` destroys them. The difference is that these commands dispatch GitHub Action workflows instead of operating against a persistent Kubernetes control plane.

This mode complements the existing "Radius on Kubernetes" mode, giving users the choice of how they want to operate Radius based on their infrastructure preferences.

### Radius on Kubernetes vs Radius on GitHub

| Concept | Radius on Kubernetes | Radius on GitHub |
|---------|---------------------|-----------------|
| **Environments** | Created via `rad environment create`; stored in the Radius control plane | Created via `rad environment create`; stored using GitHub Environments API |
| **Credentials** | Created via `rad credential register` | OIDC configured automatically as part of `rad environment create` |
| **Resource Types** | Created via `rad resource-type create` | A config repository is specified in a GitHub repository variable (`RADIUS_CONFIG_REPO`), set during `rad init`. The repo contains resource type definitions (`types.yaml`), pre-built Bicep extensions (`.tgz` files), and a `bicepconfig.json`. |
| **Recipes** | Recipe Packs stored in the control plane and referenced in the Environment resource | An environment-scoped variable (`RADIUS_RECIPES_MANIFEST`) points to a manifest of recipes for each resource type |
| **Resource Groups** | Managed via `rad group` commands | No Radius resource groups |
| **Workspaces** | Points to a kube context describing the Kubernetes cluster running Radius | A URL to the GitHub repository |

### Key Characteristics

- **Standard deployment commands**: `rad deploy` deploys applications; `rad app delete` destroys them. Same commands as Kubernetes mode, but dispatch GitHub Action workflows.
- **GitHub API storage**: Environment configuration stored as GitHub Environment-scoped variables; config repository referenced via GitHub repository variable
- **GitHub Actions execution**: `rad deploy` and `rad app delete` dispatch GitHub Action workflows that run in ephemeral k3d clusters
- **OIDC authentication**: Secure, credential-free authentication to AWS and Azure via OIDC federation
- **CLI-driven**: Familiar `rad` CLI commands: `rad init --github`, `rad environment create`, `rad deploy`, `rad app delete`

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize Repository for Radius (Priority: P1)

A developer wants to use Radius with their existing GitHub repository. They run `rad init` to set up the repository structure and GitHub Actions workflows. The command no longer requires a `--provider` flag or creates environment-specific configuration — that is handled separately by `rad environment create`.

**Why this priority**: This is the entry point for all Radius on GitHub functionality. Without initialization, no other features can be used.

**Independent Test**: Can be fully tested by running `rad init` on a fresh GitHub repository clone and verifying the workspace is registered, GitHub Actions workflows are created, the RADIUS_CONFIG_REPO variable is set, and changes are committed.

**Acceptance Scenarios**:

1. **Given** a cloned GitHub repository without Radius configuration, **When** the user runs `rad init --github`, **Then** the system generates GitHub Actions workflow files, uses the GitHub API to create a repository variable `RADIUS_CONFIG_REPO` with the default value `https://github.com/zachcasper/resource-types-contrib/tree/github-radius`, updates `~/.rad/config.yaml` with a `github` kind workspace, commits changes with trailer `Radius-Action: init`, and pushes to the remote.

2. **Given** a cloned GitHub repository, **When** the user runs `rad init --github --config-repo https://github.com/myorg/resource-types-contrib/tree/main`, **Then** the system sets `RADIUS_CONFIG_REPO` to `https://github.com/myorg/resource-types-contrib/tree/main`.

3. **Given** a directory that is not a Git repository, **When** the user runs `rad init --github`, **Then** the system displays an error message instructing the user to initialize a Git repository first or clone from GitHub.

4. **Given** a Git repository without a GitHub remote (origin), **When** the user runs `rad init --github`, **Then** the system displays an error message instructing the user to add a GitHub remote.

5. **Given** the user is not authenticated with the GitHub CLI, **When** the user runs `rad init --github`, **Then** the system verifies `gh auth status` and displays an error instructing the user to authenticate using `gh auth login`.

6. **Given** a repository already initialized with Radius, **When** the user runs `rad init --github`, **Then** the system warns that Radius is already initialized and offers to reinitialize.

---

### User Story 2 - Create Environment with Cloud Provider (Priority: P1)

A platform engineer needs to create a deployment target for an application. They run `rad environment create <name> --provider <aws|azure>` which creates a GitHub Environment via the GitHub API, sets up OIDC authentication with the cloud provider, and stores all configuration as environment-scoped variables in GitHub.

**Why this priority**: Without an environment, there is nowhere to deploy. This is a prerequisite for all deployments.

**Independent Test**: Can be fully tested by running `rad environment create dev --provider azure` and verifying a GitHub Environment named "dev" is created with the correct environment variables set via the GitHub API.

**Acceptance Scenarios**:

1. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider azure`, **Then** the system creates a GitHub Environment named "dev" via the GitHub API, follows the OIDC setup flow (prompting for subscription, resource group, and federated credential), creates a Terraform state backend (Azure Storage account), and creates environment variables: `AZURE_CLIENT_ID`, `AZURE_RESOURCE_GROUP`, `AZURE_SUBSCRIPTION_ID`, `AZURE_TENANT_ID`, `TF_STATE_CONTAINER`, `TF_STATE_STORAGE_ACCOUNT`, and `RADIUS_RECIPES_MANIFEST` (defaulting to `config/recipes-azure-bicep.yaml` from the config repository).

2. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider azure --deployment-tool terraform`, **Then** the system sets `RADIUS_RECIPES_MANIFEST` to the `config/recipes-azure-terraform.yaml` from the config repository.

3. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider azure --deployment-tool bicep`, **Then** the system sets `RADIUS_RECIPES_MANIFEST` to the `config/recipes-azure-bicep.yaml` from the config repository.

4. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider aws`, **Then** the system creates a GitHub Environment named "dev" via the GitHub API, follows the OIDC setup flow (prompting for account ID, region, and IAM role), creates a Terraform state backend (S3 bucket and DynamoDB table), and creates environment variables: `AWS_ACCOUNT_ID`, `AWS_ROLE_ARN`, `AWS_REGION`, `TF_STATE_BUCKET`, `TF_STATE_KEY`, `TF_STATE_DYNAMODB_TABLE`, and `RADIUS_RECIPES_MANIFEST` (defaulting to `config/recipes-aws-terraform.yaml` from the config repository).

5. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider aws --deployment-tool terraform`, **Then** the system sets `RADIUS_RECIPES_MANIFEST` to the `config/recipes-aws-terraform.yaml` from the config repository.

6. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider aws --deployment-tool bicep`, **Then** the system returns an error because Bicep is not supported as a deployment tool for AWS.

7. **Given** a GitHub Environment named "dev" already exists, **When** the user runs `rad environment create dev --provider azure`, **Then** the system warns that the environment already exists and offers to update it.

8. **Given** the workspace kind is Kubernetes, **When** the user runs `rad environment create dev`, **Then** the system retains the existing Kubernetes-mode `rad environment create` functionality (creating an environment resource in the Radius control plane).

9. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create prod --provider azure --recipes https://github.com/myorg/custom-recipes/recipes.yaml`, **Then** the system sets `RADIUS_RECIPES_MANIFEST` to the custom URL instead of the default.

10. **Given** AWS CLI is not installed or authenticated, **When** the user chooses to create a new IAM role, **Then** the system displays an error with instructions to install/authenticate AWS CLI.

11. **Given** OIDC setup completes successfully, **When** environment variables are stored, **Then** the system dispatches a lightweight authentication test workflow that verifies the OIDC credentials can authenticate to the cloud provider and reports success or failure to the CLI.

12. **Given** the authentication test workflow fails, **When** the CLI displays the result, **Then** the error message identifies the likely OIDC misconfiguration (e.g., wrong audience, wrong subject claim) and suggests remediation steps.

13. **Given** the current directory is not a GitHub workspace, **When** the user runs `rad environment create`, **Then** the system displays an error message indicating the command requires a GitHub workspace.

---

### User Story 3 - Create Application Definition (Priority: P1)

After connecting their cloud provider, a developer needs to create an application definition that defines the resources their application requires. They run `rad app model` to generate a starter application definition file.

**Why this priority**: An application definition is required before any deployment can occur. This creates the foundational Bicep file that describes the application's resource requirements. The current implementation is a placeholder for future AI-assisted modeling functionality.

**Independent Test**: Can be fully tested by running `rad app model` in an initialized repository and verifying the application definition file is created with the correct structure.

**Acceptance Scenarios**:

1. **Given** an initialized GitHub repository with Radius configuration, **When** the user runs `rad app model`, **Then** the system creates `.radius/applications/todolist.bicep` with a sample application definition containing an application resource, a container resource, and a database resource.

2. **Given** an initialized repository, **When** the user runs `rad app model`, **Then** the generated definition includes proper Bicep syntax with `extension radius`, parameter declarations, and resource definitions using the `2025-08-01-preview` API version.

3. **Given** the application definition is created, **When** the command completes, **Then** the system commits the changes with trailer `Radius-Action: model` and pushes to the remote repository.

4. **Given** a directory without Radius initialization, **When** the user runs `rad app model`, **Then** the system displays an error message instructing the user to run `rad init` first.

5. **Given** an application definition already exists at `.radius/applications/todolist.bicep`, **When** the user runs `rad app model`, **Then** the system prompts the user to confirm overwriting the existing file or choose a different name.

---

### User Story 4 - Deploy an Application (Priority: P1)

A developer or platform engineer wants to deploy an application to an environment. They run `rad deploy <bicep-file> --environment <environment-name>` which dispatches a GitHub Action workflow that spins up an ephemeral k3d cluster, runs the Radius control plane to deploy the application, and provisions all resources.

**Why this priority**: This is the core deployment action. Without it, no application can be deployed.

**Independent Test**: Can be fully tested by running `rad deploy .radius/applications/todolist.bicep --environment dev` and verifying a GitHub Action workflow is dispatched and the application is deployed to the specified environment.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with an environment and application definition, **When** the user runs `rad deploy .radius/applications/todolist.bicep --environment dev`, **Then** the system dispatches the `rad-deploy.yaml` GitHub Action workflow that deploys the application to the specified environment.

2. **Given** a GitHub workspace with multiple environments, **When** the user runs `rad deploy .radius/applications/todolist.bicep` without specifying `--environment`, **Then** the system returns an error listing available environments.

3. **Given** a Kubernetes workspace, **When** the user runs `rad deploy .radius/applications/todolist.bicep`, **Then** the system executes the deployment directly against the Kubernetes control plane (existing Radius behavior).

---

### User Story 5 - Delete Deployed Application (Priority: P1)

A developer or platform engineer wants to tear down a previously deployed application from an environment. They run `rad app delete --application <name> --environment <name>` which dispatches the `rad-app-delete.yaml` GitHub Action workflow that destroys all resources belonging to that application in the specified environment using cloud OIDC credentials.

**Why this priority**: The ability to clean up deployed resources is essential for environment management and cost control.

**Independent Test**: Can be fully tested by deploying an application to an environment, running `rad app delete --application todolist --environment dev`, and verifying all application resources are removed.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with "todolist" deployed to the "dev" environment, **When** the user runs `rad app delete --application todolist --environment dev`, **Then** the system dispatches the `rad-app-delete.yaml` GitHub Action workflow that destroys all resources belonging to the "todolist" application in the "dev" environment.

2. **Given** a GitHub workspace with multiple environments, **When** the user runs `rad app delete --application todolist` without specifying `--environment`, **Then** the system returns an error listing available environments. The `--application` flag is required.

3. **Given** a GitHub workspace with multiple applications deployed, **When** the user runs `rad app delete --environment dev` without specifying `--application`, **Then** the system returns an error listing deployed applications in that environment. The `--application` flag is required.

4. **Given** no application named "todolist" is deployed to the "dev" environment, **When** the user runs `rad app delete --application todolist --environment dev`, **Then** the system returns an error indicating the application is not deployed to that environment.

5. **Given** a Kubernetes workspace, **When** the user runs `rad app delete --application todolist`, **Then** the system retains the existing Kubernetes-mode destroy functionality.

---

### User Story 6 - Delete Environment (Priority: P2)

A platform engineer wants to remove a deployment target. They run `rad environment delete <name>` to remove the GitHub Environment and its associated configuration.

**Why this priority**: Environment lifecycle management is important but secondary to creation and deployment.

**Independent Test**: Can be fully tested by running `rad environment delete dev` and verifying the GitHub Environment is removed via the GitHub API.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with a GitHub Environment named "dev", **When** the user runs `rad environment delete dev`, **Then** the system deletes the GitHub Environment via the GitHub API.

2. **Given** a GitHub workspace without a GitHub Environment named "dev", **When** the user runs `rad environment delete dev`, **Then** the system returns an error indicating the environment does not exist.

3. **Given** a Kubernetes workspace, **When** the user runs `rad environment delete dev`, **Then** the system retains the existing Kubernetes-mode delete functionality.

4. **Given** applications are currently deployed to the "dev" environment, **When** the user runs `rad environment delete dev`, **Then** the system prompts the user to (1) delete the environment and the applications deployed (the default); (2) delete the environment and leave the deployed applications but manage them outside of Radius. Asks for confirmation before proceeding with either options.

5. **Given** the environment was configured with `--provider azure`, **When** the user runs `rad environment delete dev`, **Then** the system prompts whether to delete the Azure AD app registration and federated credential used for OIDC authentication. If the user confirms, the system deletes the app registration via `az ad app delete`. If declined, the system displays the app registration details for manual cleanup.

6. **Given** the environment was configured with `--provider aws`, **When** the user runs `rad environment delete dev`, **Then** the system prompts whether to delete the AWS IAM OIDC provider and IAM role used for OIDC authentication. If the user confirms, the system deletes the resources via AWS CLI. If declined, the system displays the IAM role ARN and OIDC provider ARN for manual cleanup.

---

### User Story 7 - Manage Workspaces Across Repositories (Priority: P3)

A developer works with multiple repositories, some using Radius on GitHub and others using Radius on Kubernetes. The workspace configuration allows seamless switching between different contexts.

**Why this priority**: Multi-repository support enhances user experience but is not essential for single-repo usage.

**Independent Test**: Can be fully tested by configuring multiple workspaces in `~/.rad/config.yaml` and switching between them while verifying command behavior changes appropriately.

**Acceptance Scenarios**:

1. **Given** multiple workspaces are configured (some `github` kind, some `kubernetes` kind), **When** the user switches workspaces, **Then** subsequent commands use the selected workspace's configuration and connection type.

2. **Given** a GitHub workspace is current, **When** the user runs `rad deploy .radius/applications/todolist.bicep --environment dev`, **Then** the system dispatches the `rad-deploy.yaml` workflow to deploy the application.

3. **Given** a Kubernetes workspace is current, **When** the user runs commands, **Then** they operate against the Kubernetes control plane as before.

4. **Given** the user wants to use Kubernetes-based Radius, **When** they run `rad install kubernetes`, **Then** the traditional Kubernetes control plane is installed (the new `rad init` does not replace this path).

---

### User Story 8 - List and Inspect Environments (Priority: P2)

A developer or platform engineer wants to see what environments are available in their repository and inspect the configuration of a specific environment. They run `rad env list` to see all environments and `rad env show <name>` to see the details of a specific environment.

**Why this priority**: Understanding available environments and their configuration is essential for deployment planning and troubleshooting.

**Independent Test**: Can be fully tested by running `rad env list` and `rad env show` in a repository with configured environments and verifying the output matches the GitHub Environment configuration.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with multiple environments configured, **When** the user runs `rad env list`, **Then** the system displays a table of all environments in the repository showing the environment name and cloud provider.

2. **Given** a GitHub workspace with an environment named "dev", **When** the user runs `rad env show dev`, **Then** the system displays the repository name, environment name, cloud provider, and all environment variables (with sensitive values masked).

3. **Given** a GitHub workspace with no environments configured, **When** the user runs `rad env list`, **Then** the system displays an empty table with a message suggesting to run `rad env create`.

4. **Given** the user runs `rad env show` for a non-existent environment, **Then** the system displays an error message indicating the environment was not found.

5. **Given** the user runs `rad env list` with `--output json`, **Then** the system outputs the environment list in JSON format suitable for scripting.

6. **Given** the user runs `rad env show dev --output json`, **Then** the system outputs the environment details in JSON format including all variable names and values (with sensitive values masked).

---

### Edge Cases

- What happens when the GitHub repository has no Actions enabled? System displays an error with instructions to enable GitHub Actions in repository settings.
- What happens when OIDC role permissions are insufficient? Deployment fails with clear error message indicating missing permissions and which permissions are needed.
- What happens when a deployment is in progress and another is requested? GitHub Actions concurrency groups queue the new run until the in-progress run completes; parallel deployments to different environments are unaffected.
- What happens when network connectivity to GitHub is lost during deployment? The GitHub Action retries or fails gracefully with state preserved in Terraform state.
- What happens when the resource types repository is unavailable during workflow execution? Workflow fails with clear error message indicating the repository could not be cloned.
- What happens when Terraform state conflicts occur? Deployment fails with instructions to resolve state conflicts manually.
- What happens when the k3d cluster fails to start in GitHub Actions? Workflow fails with diagnostic information about resource constraints or configuration issues.
- What happens when the GitHub API rate limit is exceeded during `rad environment create`? System displays a clear error with the rate limit reset time and suggests retrying later.
- What happens when the user's GitHub token lacks permissions to create environments or variables? System displays an error listing the required permissions.
- What happens when `--config-repo` points to a non-existent repository? System validates the URL format but does not fetch the repository at init time; errors surface when the workflow clones the repo.
- What happens when the bicep file passed to `rad deploy` is not found? System returns an error indicating the file does not exist.
- What happens when GitHub Environment variables are missing or incomplete? `rad deploy` validates required variables before deployment and lists missing values.
- What happens when `rad environment create` is run with `--provider aws --deployment-tool bicep`? System returns an error because Bicep is only supported for Azure.
- What happens when `rad deploy` is run with committed but unpushed changes? System returns an error instructing the user to push changes to the remote first.

## Requirements *(mandatory)*

### Functional Requirements

#### CLI Command: rad init

- **FR-001**: `rad init` MUST NOT take a `--provider` flag; provider configuration is handled by `rad environment create`.
- **FR-002**: `rad init` MUST NOT create a `.radius/types.yaml` manifest file.
- **FR-003**: `rad init` MUST NOT create a `.radius/recipes.yaml` manifest file.
- **FR-004**: `rad init` MUST NOT create a `.radius/env.*.yaml` environment file.
- **FR-005**: `rad init --github` MUST use the GitHub API to create a repository variable named `RADIUS_CONFIG_REPO`.
- **FR-006**: `RADIUS_CONFIG_REPO` MUST default to `https://github.com/zachcasper/resource-types-contrib/tree/github-radius`.
- **FR-007**: `rad init` MUST accept a `--config-repo` flag that overrides the default config repository URL. When specified, `RADIUS_CONFIG_REPO` is set to the provided URL.
- **FR-008**: System MUST validate the current directory is a Git repository by checking for `.git` directory.
- **FR-009**: System MUST validate the Git repository has a GitHub remote (origin) by parsing the remote URL.
- **FR-010**: System MUST verify GitHub CLI (`gh`) is authenticated by running `gh auth status`.
- **FR-011**: System MUST update or create `~/.rad/config.yaml` with a new workspace of kind `github` using the repository URL as connection; workspace name MUST match the repository name.
- **FR-012**: System MUST rename the workspace config property `default` to `current` for clarity.
- **FR-013**: System MUST commit changes with `git add` and `git commit` including the trailer `Radius-Action: init`, then push to the remote with `git push`.
- **FR-014**: System MUST NOT be interactive; all configuration comes from command-line flags.
- **FR-014-A**: `rad init` MUST NOT create any `.radius/` directory or subdirectories, and MUST NOT use `.gitkeep` files. Directories are created on demand by the commands that need them: `rad app model` creates `.radius/applications/`.

#### CLI Command: rad environment create (replaces rad environment connect)

- **FR-015**: `rad environment connect` MUST be renamed to `rad environment create`.
- **FR-016**: When the workspace kind is GitHub, `rad environment create` MUST follow the behavior defined in this section. When the workspace kind is Kubernetes, retain the existing `rad environment create` functionality.
- **FR-017**: `rad environment create <name>` MUST require a `--provider` flag with values `aws` or `azure`.
- **FR-018**: `rad environment create` MUST accept an optional `--deployment-tool` flag with values `terraform` or `bicep`.
- **FR-019**: When `--provider` is `aws`, the default deployment tool MUST be `terraform`.
- **FR-020**: When `--provider` is `azure`, the default deployment tool MUST be `bicep`.
- **FR-021**: When `--provider` is `aws` and `--deployment-tool` is `bicep`, the system MUST return an error because Bicep is not supported for AWS.
- **FR-022**: `rad environment create` MUST use the GitHub API to create a GitHub Environment with the specified name on the repository.
- **FR-023**: `rad environment create` MUST accept an optional `--recipes` flag to override the default recipes manifest URL.
- **FR-024**: For Azure environments, system MUST follow the same OIDC setup flow as the current `rad environment connect` (prompting for subscription, resource group, AKS cluster, namespace, and federated credential).
- **FR-025**: For Azure environments, system MUST create the following environment variables within the GitHub Environment:
  - `AZURE_CLIENT_ID`
  - `AZURE_RESOURCE_GROUP`
  - `AZURE_SUBSCRIPTION_ID`
  - `AZURE_TENANT_ID`
  - `TF_STATE_CONTAINER`
  - `TF_STATE_STORAGE_ACCOUNT`
  - `RADIUS_RECIPES_MANIFEST`
- **FR-026**: When `--provider` is `azure` and `--deployment-tool` is `bicep` (or default), `RADIUS_RECIPES_MANIFEST` MUST default to the `config/recipes-azure-bicep.yaml` file from the config repository specified by `RADIUS_CONFIG_REPO`.
- **FR-027**: When `--provider` is `azure` and `--deployment-tool` is `terraform`, `RADIUS_RECIPES_MANIFEST` MUST default to the `config/recipes-azure-terraform.yaml` file from the config repository specified by `RADIUS_CONFIG_REPO`.
- **FR-028**: For AWS environments, system MUST follow the same OIDC setup flow as the current `rad environment connect` (prompting for account ID, region, EKS cluster, namespace, and IAM role).
- **FR-029**: For AWS environments, system MUST create the following environment variables within the GitHub Environment:
  - `AWS_ACCOUNT_ID`
  - `AWS_ROLE_ARN`
  - `AWS_REGION`
  - `TF_STATE_BUCKET`
  - `TF_STATE_KEY`
  - `TF_STATE_DYNAMODB_TABLE`
  - `RADIUS_RECIPES_MANIFEST`
- **FR-030**: When `--provider` is `aws`, `RADIUS_RECIPES_MANIFEST` MUST default to the `config/recipes-aws-terraform.yaml` file from the config repository specified by `RADIUS_CONFIG_REPO`.
- **FR-030-A**: System MUST NOT store cloud secret keys locally; only OIDC-related identifiers are stored as GitHub Environment variables.
- **FR-030-E**: After storing environment variables, `rad environment create` MUST dispatch a lightweight GitHub Action authentication test workflow that verifies OIDC federation works by authenticating to the cloud provider.
- **FR-030-F**: The authentication test workflow MUST use the same GitHub Environment and OIDC credentials that deployment workflows will use.
- **FR-030-G**: The CLI MUST display "Creating authentication test workflow..." followed by an animated progress indicator showing "Testing authentication to <provider>..." with automatic workflow step status display (consistent with FR-089-C through FR-089-G).
- **FR-030-H**: If the authentication test fails, the CLI MUST display a clear error identifying the likely OIDC misconfiguration and suggest remediation steps. The GitHub Environment and variables MUST remain in place so the user can fix and re-run verification.
- **FR-030-I**: If the authentication test succeeds, the CLI MUST display a success message confirming the environment is ready for deployments.

#### CLI Command: rad environment delete

- **FR-030-B**: `rad environment delete <name>` MUST delete the GitHub Environment from the repository using the GitHub API when the workspace kind is GitHub.
- **FR-030-C**: When the workspace kind is Kubernetes, `rad environment delete` MUST retain the existing Kubernetes-mode delete functionality.
- **FR-030-D**: `rad environment delete` MUST warn the user if resources may still be deployed to the environment and ask for confirmation.
- **FR-030-J**: After confirming environment deletion, `rad environment delete` MUST prompt the user whether to also delete the OIDC configuration from the cloud provider.
- **FR-030-K**: For Azure environments, if the user confirms OIDC cleanup, the system MUST delete the Azure AD app registration and its federated credential using the Azure CLI (`az ad app delete --id <CLIENT_ID>`). The `AZURE_CLIENT_ID` stored in the GitHub Environment variables MUST be used to identify the app registration.
- **FR-030-L**: For AWS environments, if the user confirms OIDC cleanup, the system MUST delete the IAM role and OIDC identity provider using the AWS CLI (`aws iam delete-role` and `aws iam delete-open-id-connect-provider`). The `AWS_ROLE_ARN` stored in the GitHub Environment variables MUST be used to identify the resources.
- **FR-030-M**: If the user declines OIDC cleanup, the system MUST display the cloud provider resource identifiers (Azure app registration client ID or AWS IAM role ARN and OIDC provider ARN) so the user can clean them up manually.
- **FR-030-N**: OIDC cleanup MUST occur before the GitHub Environment is deleted, since the environment variables contain the identifiers needed to locate the cloud resources.

#### CLI Commands: rad pr create, rad pr merge, rad pr destroy (Removed)

- **FR-032**: The `rad pr create` command MUST be removed entirely. Radius does not own the PR workflow.
- **FR-033**: The `rad pr merge` command MUST be removed entirely.
- **FR-034**: The `rad pr destroy` command MUST be removed entirely.
- **FR-035**: Platform engineers compose deployment pipelines using standard GitHub Actions with `rad deploy` as the deployment command.

#### CLI Command: rad deploy (Enhanced)

- **FR-036**: `rad deploy <bicep-file>` MUST deploy the specified Bicep application definition file to the target environment.
- **FR-037**: In GitHub mode, `rad deploy` MUST dispatch the `rad-deploy.yaml` GitHub Action workflow. In Kubernetes mode, `rad deploy` MUST execute the deployment directly against the Kubernetes control plane (existing Radius behavior).
- **FR-038**: `rad deploy` MUST accept a `--environment` (or `-e` or `--env`) flag specifying the target environment.
- **FR-039**: When exactly one GitHub Environment exists for the repository, `--environment` MAY be omitted and the system MUST auto-select that environment.
- **FR-040**: When multiple GitHub Environments exist, `--environment` MUST be required. The system MUST error with a message listing available environments if omitted.
- **FR-040a**: `rad deploy` MUST verify that the specified Bicep file exists before dispatching the workflow. If the file does not exist, the system MUST error with a message indicating the file was not found.
- **FR-041**: `rad deploy` MUST verify that the local working tree has no uncommitted changes before dispatching the workflow. If uncommitted changes exist, the system MUST error with a message instructing the user to commit all changes first.
- **FR-042**: `rad deploy` MUST verify that all local commits have been pushed to the remote before dispatching the workflow. If unpushed commits exist, the system MUST error with a message instructing the user to push changes first.
- **FR-043**: The deployment MUST be scoped to the current HEAD commit hash by default. The commit hash provides traceability between the application definition that was deployed and the deployment execution.
- **FR-044**: The dispatched workflow MUST read environment configuration from GitHub Environment variables (e.g., `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `AZURE_CLIENT_ID`, `RADIUS_RECIPES_MANIFEST`) and the config repository URL from the `RADIUS_CONFIG_REPO` repository variable.
- **FR-045**: The dispatched deployment workflow MUST clone the config repository (`RADIUS_CONFIG_REPO`), copy `bicepconfig.json` and Bicep extension `.tgz` files to the application repo root, create a k3d cluster, install the Radius control plane, register resource types from `config/types.yaml`, register recipes from the environment's `RADIUS_RECIPES_MANIFEST`, and execute `rad deploy <bicep-file>` to deploy the application.

#### Configuration Data Storage

- **FR-062**: Config repository MUST be stored as a GitHub repository variable `RADIUS_CONFIG_REPO` set during `rad init`.
- **FR-063**: The `RADIUS_CONFIG_REPO` variable MUST contain the URL to the config repository (e.g., `https://github.com/zachcasper/resource-types-contrib/tree/github-radius`). The URL format is `https://github.com/<owner>/<repo>/tree/<branch>`. The `config/` directory within the repository contains the types manifest, Bicep configuration, and extension files.
- **FR-064**: Recipes manifest MUST be stored as a GitHub Environment-scoped variable `RADIUS_RECIPES_MANIFEST` set during `rad environment create`.
- **FR-065**: The `RADIUS_RECIPES_MANIFEST` variable MUST contain the URL to the appropriate recipes manifest file in the resource types repository based on cloud provider and deployment tool.
- **FR-066**: Cloud provider configuration MUST be stored as GitHub Environment-scoped variables set during `rad environment create`.
- **FR-067**: Azure environment variables MUST include the variables listed in FR-025: `AZURE_CLIENT_ID`, `AZURE_RESOURCE_GROUP`, `AZURE_SUBSCRIPTION_ID`, `AZURE_TENANT_ID`, `TF_STATE_CONTAINER`, `TF_STATE_STORAGE_ACCOUNT`, `RADIUS_RECIPES_MANIFEST`.
- **FR-067-A**: AWS environment variables MUST include the variables listed in FR-029: `AWS_ACCOUNT_ID`, `AWS_ROLE_ARN`, `AWS_REGION`, `TF_STATE_BUCKET`, `TF_STATE_KEY`, `TF_STATE_DYNAMODB_TABLE`, `RADIUS_RECIPES_MANIFEST`.
- **FR-067-B**: Environment files (`.radius/env.*.yaml`) MUST NOT be used; all environment configuration is stored in GitHub Environment variables.
- **FR-067-C**: Resource types files (`.radius/types.yaml`) MUST NOT be used; the resource types repository reference is stored as a GitHub repository variable.
- **FR-068**: Application Definitions MUST be stored in `.radius/applications/<APP_NAME>.bicep`.
- **FR-068-A**: System MUST provide `rad app model` command that creates a sample application definition file at `.radius/applications/todolist.bicep` with a `Radius.Core/applications` resource, a `Radius.Compute/containers` resource, and a `Radius.Data/postgreSqlDatabases` resource. This is a placeholder for future AI-assisted modeling functionality.
- **FR-068-B**: `rad app model` MUST create the `.radius/applications/` directory if it does not already exist.
- **FR-068-C**: `rad app model` MUST commit the generated file with trailer `Radius-Action: model` and push to the remote repository.
- **FR-071**: Workspaces MUST be stored in `~/.rad/config.yaml` with `current` property (renamed from `default`).
- **FR-072**: GitHub workspaces MUST have `connection.kind: github` and `connection.url` pointing to the repository.

#### Command Behavior Changes

- **FR-073**: In GitHub mode, `rad resource-type` commands MUST operate against the resource type definitions in the repository referenced by `RADIUS_CONFIG_REPO`.
- **FR-074**: In GitHub mode, `rad environment` commands (`create`, `delete`, `list`, `show`) MUST operate against GitHub Environments via the GitHub API.
- **FR-074-A**: In GitHub mode, `rad env list` MUST query the GitHub API to retrieve all environments for the current repository and display them in a table with columns: Name, Provider (derived from environment variables).
- **FR-074-B**: In GitHub mode, `rad env show <name>` MUST query the GitHub API to retrieve the specified environment's details and display: repository name, environment name, cloud provider, and all environment variables with their values.
- **FR-074-C**: `rad env show` MUST mask sensitive values (secrets) in the output while showing regular environment variables.
- **FR-074-D**: Both `rad env list` and `rad env show` MUST support `--output json` for programmatic access.
- **FR-075**: In GitHub mode, `rad recipe` commands MUST operate against the recipes manifest referenced by the environment's `RADIUS_RECIPES_MANIFEST` variable.
- **FR-076**: Users wanting Kubernetes-based Radius MUST use `rad install kubernetes` (not affected by new `rad init`).
- **FR-077**: Radius on GitHub MUST NOT use Resource Groups; that concept does not apply.

#### GitHub Actions Execution

- **FR-086**: GitHub Action runner MUST be capable of running k3d cluster (approximately 45 seconds startup, ~875 MiB download).
- **FR-087**: k3d cluster MUST map `/github_workspace` in containers to `${GITHUB_WORKSPACE}` for file access.
- **FR-088**: The deployment workflow MUST clone the config repository specified by `RADIUS_CONFIG_REPO`. The workflow MUST copy `config/bicepconfig.json` and all `config/*.tgz` Bicep extension archives into the application repository's root directory so that Bicep's parent-directory resolution finds them when compiling `.radius/applications/*.bicep` files (Bicep searches from the `.bicep` file's directory upward through parent directories for `bicepconfig.json`). The workflow MUST then register resource types by reading `config/types.yaml` and invoking `rad resource-type create --from-file` for each referenced definition. The workflow MUST download the recipes manifest from the target environment's `RADIUS_RECIPES_MANIFEST` variable and register each recipe on the Radius environment by invoking `rad recipe register` for each entry in the manifest.
- **FR-088-A**: Radius installation on k3d MUST use `--skip-contour-install` flag (ingress not needed in ephemeral cluster) and `--set dashboard.enabled=false` (dashboard not needed in CI).
- **FR-089**: GitHub Actions workflow MUST leverage GitHub PR Checks for deployment status reporting.
#### CLI Workflow Dispatch UX

- **FR-089-C**: When a CLI command dispatches a GitHub Action workflow, the CLI MUST display a contextual status message describing the action (e.g., "Creating authentication test workflow...", "Creating deployment workflow...", "Creating deployment...", "Testing authentication to azure...").
- **FR-089-D**: After dispatching, the CLI MUST display an animated progress indicator (e.g., spinner) with a contextual status label (e.g., "Testing authentication to azure...", "Creating deployment...").
- **FR-089-E**: While the animated progress indicator is active, the CLI MUST automatically display the workflow step status below the spinner, showing each step with a status icon (`✓` success, `✗` failure, `⊘` skipped, `●` in progress, `○` pending). The step display MUST update on each poll cycle. The CLI MUST NOT provide a quit option (e.g., `q` key); the user MUST wait until the workflow completes with success or failure. For detailed logs, the user can click the workflow run URL displayed above the steps.
- **FR-089-F**: The CLI MUST poll step status via `gh run view <id> --json jobs` every 2 seconds to provide real-time step-level progress for both in-progress and completed runs.
- **FR-089-G**: The CLI MUST display the final workflow result (success or failure) with a summary when the workflow completes, regardless of whether log streaming was active.

- **FR-089-A**: All GitHub Actions workflow runs MUST be named using the following convention:
  - Deploy workflows: `Radius deploy for <bicep_file> in <env> environment`
  - Delete workflows: `Radius app delete for <app_name> in <env> environment`
  - Authentication test workflows: `Radius authentication test for <env> environment`
- **FR-089-B**: Generated workflows MUST only use external GitHub Actions from trusted sources:
  - GitHub official actions (e.g., `actions/checkout`)
  - Radius Project actions (e.g., `radius-project/*`)
  - Cloud provider official actions: AWS (`aws-actions/*`), Azure (`azure/*`), Google (`google-github-actions/*`)
  - HashiCorp official actions (e.g., `hashicorp/setup-terraform`)
  - Direct shell commands for tools without official actions (e.g., k3d installation via official installer script)

#### Resource Type Extensions

- **FR-090**: A new `Radius.Core/applications` Resource Type MUST be created in the `radius-project/resource-types-contrib` repository.
- **FR-091**: The `Radius.Core/applications` Resource Type MUST have an `environment` property of type string.
- **FR-092**: Application definitions MUST declare an application resource using `Radius.Core/applications@2025-08-01-preview`.

#### Resource Types Repository

- **FR-092-A**: The config repository (`RADIUS_CONFIG_REPO`) MUST contain a `config/` directory with:
  - `types.yaml` — a manifest of resource type definition YAML files with relative paths to each definition
  - `bicepconfig.json` — a Bicep configuration file that maps extension names to pre-built `.tgz` extension files
  - Pre-built Bicep extension `.tgz` files for each resource type (e.g., `radius-compute-containers.tgz`, `radius-data-postgresqldatabases.tgz`)
  - `generate-bicep-extensions.sh` — a script to regenerate the `.tgz` files and `bicepconfig.json` from the type definitions
  - Recipe manifests (`recipes-azure-terraform.yaml`, `recipes-azure-bicep.yaml`, `recipes-aws-terraform.yaml`)
- **FR-092-B**: The deployment workflow MUST clone the config repository and copy `config/bicepconfig.json` and all `config/*.tgz` Bicep extension archives into the application repository's root directory. Bicep resolves `bicepconfig.json` by searching from the `.bicep` file's directory upward through parent directories; placing the config at the app repo root ensures `.radius/applications/*.bicep` files find it. The `.tgz` extension paths in `bicepconfig.json` (e.g., `"./radius-compute-containers.tgz"`) resolve relative to the `bicepconfig.json` location.
- **FR-092-C**: When the Radius control plane starts in the k3d cluster, the workflow MUST read `config/types.yaml` and register each resource type by reading the referenced definition YAML file and creating the resource type within the Radius control plane.
- **FR-092-D**: The `types.yaml` manifest MUST use relative paths for `definitionLocation` values (e.g., `../Compute/containers/containers.yaml`) that resolve within the cloned repository.
- **FR-092-E**: The `bicepconfig.json` MUST map each resource type to a relative path `.tgz` extension file (e.g., `"radius-compute-containers": "./radius-compute-containers.tgz"`) and include a `"radius"` entry pointing to the Radius extension registry (`br:biceptypes.azurecr.io/radius:latest`).
- **FR-092-F**: Application definition files (`.radius/applications/*.bicep`) MUST use the extension names defined in the resource types repository's `bicepconfig.json` (e.g., `extension radius` resolves to the `"radius"` entry in `bicepconfig.json`).

#### Terraform State Management

- **FR-093**: Terraform state MUST be stored in a cloud backend (S3 for AWS, Azure Storage for Azure) rather than locally in the repository.
- **FR-094**: The `rad environment create` command MUST configure or create the state backend storage as part of OIDC setup.
- **FR-095**: State backend credentials MUST use the same OIDC authentication configured for deployments.
- **FR-096**: State backend MUST support state locking to prevent concurrent modification conflicts.
- **FR-097**: State backend location MUST be stored as GitHub Environment-scoped variables: `TF_STATE_BACKEND_TYPE` (value: `s3` or `azurerm`), `TF_STATE_BACKEND_CONFIG` (JSON-encoded backend configuration including bucket/container name, region, and key prefix).
- **FR-097-A**: Azure Storage account for Terraform state MUST be named `tfstateradius<org><repo>` (lowercase alphanumeric, max 24 characters).
- **FR-097-B**: AWS S3 bucket for Terraform state MUST be named `tfstate-<org>-<repo>` (lowercase with hyphens).

#### Concurrent Deployment Handling

- **FR-098**: The generated deployment workflows MUST use the GitHub Actions `concurrency:` key to serialize runs. The concurrency group MUST be scoped to the application and environment (e.g., `radius-deploy-<app>-<env>`).
- **FR-099**: The `concurrency:` configuration MUST use `cancel-in-progress: false` so that a queued deployment waits rather than cancelling the in-progress run.
- **FR-100**: When a deployment is queued behind an in-progress run, the CLI MUST display a message indicating another deployment is in progress and the current run is queued.
- **FR-101**: GitHub Actions natively releases the concurrency lock when the workflow completes (success, failure, or cancellation); no additional lock release logic is required.

#### Partial Deployment Failure Handling

- **FR-102**: When a deployment partially fails, successfully deployed resources MUST remain in place (no auto-rollback).
- **FR-103**: The deployment record MUST clearly identify which resources succeeded and which failed.
- **FR-104**: The deployment workflow MUST exit with a failure status when any resource fails to deploy.
- **FR-105**: Users MAY re-run `rad deploy` after fixing issues to resume/retry the deployment.

#### CLI Command: rad app delete (GitHub mode)

- **FR-106**: Resource destruction MUST only destroy resources belonging to the specified application and environment.
- **FR-106-A**: In GitHub mode, `rad app delete --application <name> --environment <name>` MUST dispatch the `rad-app-delete.yaml` GitHub Action workflow that destroys all resources belonging to the specified application in the specified environment using OIDC credentials from the GitHub Environment.
- **FR-106-B**: The `--application` flag is required for `rad app delete` in GitHub mode.
- **FR-106-C**: The `--environment` flag is required when multiple GitHub Environments exist; if exactly one exists, it MAY be auto-selected.
- **FR-106-D**: The CLI MUST display an animated progress indicator with automatic workflow step status display, consistent with other workflow-dispatching commands (FR-089-C through FR-089-G).
- **FR-107**: If `--application` is omitted, the command MUST error with a message listing deployed applications.

#### Secret Management

- **FR-108**: Deployment workflows MAY reference GitHub Secrets using the syntax `${{ secrets.SECRET_NAME }}`.
- **FR-109**: The deployment workflow MUST inject referenced GitHub Secrets as environment variables at runtime.
- **FR-110**: Secret references MUST NOT be expanded or logged during plan generation.
- **FR-111**: The `rad init` command SHOULD document the GitHub Secrets reference syntax in generated workflow file comments.

#### Workflow Installation

- **FR-112**: The `rad init --github` command MUST generate `.github/workflows/rad-deploy.yaml`, `.github/workflows/rad-app-delete.yaml`, and `.github/workflows/rad-auth-test.yaml` workflow templates that use `rad deploy`, `rad app delete`, and OIDC authentication verification respectively.
- **FR-113**: The generated workflows MUST reference GitHub Environment variables for cloud provider configuration and recipe manifests, and the `RADIUS_CONFIG_REPO` repository variable for the config repository.
- **FR-113-A**: The generated workflow MUST use the `environment:` key in the job definition to scope GitHub Environment variables to the target deployment environment.
- **FR-114**: Generated workflow files MUST be included in the initial commit created by `rad init --github`.

#### Resource Group Defaults

- **FR-115**: The `rad` CLI MUST fall back to the `default` resource group when no `--group` flag is provided and no workspace scope is configured. This supports GitHub-mode workspaces (which do not set a `Scope` property) and ephemeral CI environments (where no `~/.rad/config.yaml` exists). The Radius control plane auto-creates the resource group on first use.

### Key Entities

- **Workspace**: User's working context stored in `~/.rad/config.yaml`; can be of kind `github` (URL-based connection) or `kubernetes` (context-based connection). GitHub workspaces connect to repository URLs; Kubernetes workspaces connect to cluster contexts with scope and environment references.

- **GitHub Environment**: A GitHub Environments API deployment target created via `rad environment create`. Stores cloud provider configuration (subscription IDs, cluster names, namespaces, OIDC credentials) and recipes manifest URL as environment-scoped variables. Corresponds to a deployment target such as "dev", "staging", or "production".

- **Config Repository**: External repository (e.g., `zachcasper/resource-types-contrib`) hosting resource type definitions, pre-built Bicep extensions, `bicepconfig.json`, and recipes manifests (`recipes-aws-terraform.yaml`, `recipes-azure-bicep.yaml`, `recipes-azure-terraform.yaml`). Referenced via the `RADIUS_CONFIG_REPO` repository variable. The `config/` directory contains the types manifest, Bicep configuration, extension files, and recipe manifests.

- **Resource Type**: Definition of infrastructure resource schemas stored in the config repository (`RADIUS_CONFIG_REPO`), e.g., `radius-project/resource-types-contrib`. The repository contains type definition YAML files, pre-built Bicep extensions (`.tgz` files), and a `bicepconfig.json`. Includes types like `Radius.Core/applications`, `Radius.Compute/containers`, `Radius.Data/postgreSqlDatabases`, etc.

- **Recipe**: Implementation template for provisioning resources. Referenced via the `RADIUS_RECIPES_MANIFEST` environment variable. Default deployment tool: Terraform for AWS, Bicep for Azure. Organized by provider and deployment tool in the config repository.

- **Application Definition**: Bicep-based declaration of application resources and their relationships; stored in `.radius/applications/<APP_NAME>.bicep`. Uses Radius resource types with environment parameter. Unchanged from current Radius application definition format.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a GitHub repository for Radius in under 2 minutes using `rad init --github`.
- **SC-002**: Users can create a deployment environment with cloud provider OIDC setup (AWS or Azure) in under 10 minutes with guided prompts using `rad environment create`.
- **SC-003**: Users can deploy an application using `rad deploy <bicep-file> --environment <name>` with a single command.
- **SC-004**: 95% of deployment attempts complete within 15 minutes for applications with 5 or fewer resources.
- **SC-005**: GitHub Action setup (k3d + Radius) completes in under 60 seconds with approximately 875 MiB download.
- **SC-006**: Failed deployments provide actionable error messages that identify the failing resource and root cause.
- **SC-007**: GitHub Action workflow runs are traceable to the commit hash that triggered the deployment.
- **SC-008**: Users can delete an environment and its associated resources using `rad environment delete`.
- **SC-009**: First deployment should be completable using only CLI help text and generated workflow comments, without requiring external documentation.
- **SC-010**: Workspace switching between GitHub and Kubernetes modes works seamlessly with appropriate command behavior.

## Assumptions

- Users have the GitHub CLI (`gh`) installed and authenticated on their workstation.
- Users have cloud provider CLIs (`aws` or `az`) installed and authenticated for OIDC setup.
- The GitHub repository has GitHub Actions enabled with sufficient minutes quota for workflow execution.
- A resource types repository (e.g., `zachcasper/resource-types-contrib`) hosts resource type definitions, Bicep extensions, and recipes manifests with stable URLs.
- GitHub Action runners have sufficient resources to run k3d clusters (standard GitHub-hosted runners are adequate).
- Users are familiar with Git workflows and GitHub.
- Application definition creation is handled by a separate project/workflow that outputs `.radius/applications/<APP>.bicep` files. `rad app model` provides a starting point.
- Default deployment tool is Terraform for AWS and Bicep for Azure.

## Constraints

- Default deployment tool: Terraform for AWS, Bicep for Azure. Bicep is not supported for AWS.
- Only AKS (Azure Kubernetes Service) and EKS (Amazon Elastic Kubernetes Service) are supported as deployment targets initially.
- Local development and on-premises deployment targets are not supported in initial release.
- Resource Groups concept from Radius on Kubernetes does not apply to Radius on GitHub.
- The existing `rad init` command functionality is replaced; existing Kubernetes-based users must use `rad install kubernetes`.
- Single active deployment per application/environment combination enforced via GitHub Actions concurrency groups; additional runs are queued.

## Dependencies

- GitHub CLI (`gh`) must be available on user workstations for initialization, environment management, and deployment.
- GitHub Actions must be available and enabled for the repository.
- GitHub Environments API must be available for storing environment-scoped configuration.
- Resource types repository (e.g., `zachcasper/resource-types-contrib`) must host resource type definitions, Bicep extensions, and recipes manifests.
- Cloud providers (AWS/Azure) must support OIDC federation with GitHub Actions.
- k3d must be installable and runnable on GitHub Action runners.
- Terraform must be available for recipe execution (installed in GitHub Actions).
- Bicep must be available for Azure recipe execution (installed in GitHub Actions).

## Out of Scope / Limitations

- Bicep recipe support (future enhancement for Azure only)
- Local development deployment targets
- On-premises deployment targets  
- Multi-cloud deployments within single application
- Automatic rollback on deployment failure
- Cost estimation for deployment plans
- Integration with external secret managers

## Future Enhancements

- MCP server and tools for Copilot integration (e.g., "initialize this repository to use Radius", "create a model for this application", "deploy this app")
- Application visualization in PR comments (part of app graph workstream)
- Local development and on-premises deployment targets
- Parallel deployment support for independent resources
- Deployment approval workflows with required reviewers
- Custom Terraform backend configuration - allow users to specify their own existing S3/Azure Storage backend instead of auto-provisioned storage
- Add version number tag to Resource Type definition locations - support `?ref=<version>` suffix on definition URLs to pin resource type definitions to specific versions
- `rad environment list` command to show all GitHub Environments and their configuration

---

## Appendix A: Radius Execution Model

Radius on GitHub operates with these principles:

- **Standard deployment**: `rad deploy <bicep-file> --environment <name>` dispatches a GitHub Action workflow that deploys the application. This is the same command used in Kubernetes mode, but instead of deploying to a persistent control plane, it executes in an ephemeral k3d cluster.
- **Ephemeral control plane**: Runs on a k3d cluster in a GitHub Action when triggered by `rad deploy` or `rad app delete`.
- **GitHub API storage**: Environment configuration stored as GitHub Environment-scoped variables; resource types repository referenced via GitHub repository variable.
- **CLI consistency**: Uses the existing `rad` CLI with mode-specific behavior (`rad init --github` for initialization, `rad environment create` for setup, `rad deploy` for deployment).
- **Git-native definitions**: Application definitions stored in `.radius/applications/` in the repository.
- **No persistent state**: No data is persistent in the Radius control plane; state is in GitHub (variables, environments) and the repository (application definitions).

---

## Appendix B: Configuration Data Model Examples

### B.1 GitHub Repository Variable

Set by `rad init --github`:

| Variable | Value | Scope |
|----------|-------|-------|
| `RADIUS_CONFIG_REPO` | `https://github.com/zachcasper/resource-types-contrib/tree/github-radius` | Repository |

### B.2 GitHub Environment Variables (Azure)

Set by `rad environment create dev --provider azure` for an Azure environment:

| Variable | Value | Scope |
|----------|-------|-------|
| `AZURE_CLIENT_ID` | `abcdefgh-abcd-abcd-abcd-abcdefghijkl` | Environment: dev |
| `AZURE_RESOURCE_GROUP` | `rg-radius-dev` | Environment: dev |
| `AZURE_SUBSCRIPTION_ID` | `12345678-1234-1234-1234-123456789012` | Environment: dev |
| `AZURE_TENANT_ID` | `87654321-4321-4321-4321-210987654321` | Environment: dev |
| `TF_STATE_CONTAINER` | `tfstate` | Environment: dev |
| `TF_STATE_STORAGE_ACCOUNT` | `tfstateradiusmyorgrepo` | Environment: dev |
| `RADIUS_RECIPES_MANIFEST` | `https://raw.githubusercontent.com/zachcasper/resource-types-contrib/refs/heads/github-radius/config/recipes-azure-bicep.yaml` | Environment: dev |

### B.3 GitHub Environment Variables (AWS)

Set by `rad environment create staging --provider aws` for an AWS environment:

| Variable | Value | Scope |
|----------|-------|-------|
| `AWS_ACCOUNT_ID` | `123456789012` | Environment: staging |
| `AWS_ROLE_ARN` | `arn:aws:iam::123456789012:role/radius-github-oidc` | Environment: staging |
| `AWS_REGION` | `us-east-1` | Environment: staging |
| `TF_STATE_BUCKET` | `tfstate-myorg-myrepo` | Environment: staging |
| `TF_STATE_KEY` | `radius/staging/terraform.tfstate` | Environment: staging |
| `TF_STATE_DYNAMODB_TABLE` | `tfstate-lock` | Environment: staging |
| `RADIUS_RECIPES_MANIFEST` | `https://raw.githubusercontent.com/zachcasper/resource-types-contrib/refs/heads/github-radius/config/recipes-aws-terraform.yaml` | Environment: staging |
### B.4 Workspace

```yaml
# Radius CLI configuration
# Generated by rad init
# Generated: 2026-02-05T21:35:58Z

workspaces:
  current: my-app-repo
  items:
    my-app-repo:
      connection:
        url: https://github.com/myorg/my-app-repo
        kind: github
    production-cluster:
      connection:
        context: prod-k8s
        kind: kubernetes
      environment: production
      scope: /planes/radius/local/resourceGroups/prod
```

### B.5 Application Definition (.radius/applications/todolist.bicep)

```bicep
extension radius

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
```
