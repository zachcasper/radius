# Feature Specification: GitHub Environment and Deploy Redesign

**Feature Branch**: `003-github-env-deploy`  
**Created**: 2026-02-15  
**Status**: Draft  
**Input**: User description: "Restructure GitHub mode CLI: rad init simplification, rad environment create/delete with GitHub Environments API, remove rad pr create, enhance rad deploy with --application and --environment flags"

## Overview

This feature redesigns the Radius on GitHub CLI commands to align with Azure's transactional deployment model rather than a GitOps model. The key changes are:

1. **`rad init` simplification** — no longer takes `--provider`, no longer creates types.yaml, recipes.yaml, or environment files. Instead, creates a repository environment variable for the resource types manifest and takes a `--config-repo` flag.
2. **`rad environment create` replaces `rad environment connect`** — creates a GitHub Environment via the GitHub API, prompts for cloud provider credentials (OIDC), and stores configuration as environment-scoped variables.
3. **`rad environment delete`** — removes a GitHub Environment via the GitHub API.
4. **`rad pr create` is removed** — Radius does not own the PR workflow. Platform engineers wire GitHub Actions triggers themselves.
5. **`rad deploy` enhanced** — supports `--application <name>` (resolves from `.radius/model/`), `--environment <name>` (resolves from GitHub Environment), and works in both GitHub and Kubernetes modes.

Radius provides the deployment verbs. GitHub provides the workflow orchestration. Platform engineers compose them using standard GitHub Actions.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize Repository for Radius (Priority: P1)

A developer wants to use Radius with their existing GitHub repository. They run `rad init` to set up the repository structure and GitHub Actions workflows. The command no longer requires a `--provider` flag or creates environment-specific configuration — that is handled separately by `rad environment create`.

**Why this priority**: This is the entry point for all Radius on GitHub functionality. Without initialization, no other features can be used.

**Independent Test**: Can be fully tested by running `rad init` on a fresh GitHub repository clone and verifying the workspace is registered, GitHub Actions workflows are created, the RADIUS_RESOURCE_TYPES_MANIFEST variable is set, and changes are committed.

**Acceptance Scenarios**:

1. **Given** a cloned GitHub repository without Radius configuration, **When** the user runs `rad init`, **Then** the system creates `.radius/model/`, `.radius/plan/`, and `.radius/deploy/` directories (with `.gitkeep` files), generates GitHub Actions workflow files, creates a repository environment variable `RADIUS_RESOURCE_TYPES_MANIFEST` with the default value `https://github.com/zachcasper/radius-config/types.yaml`, updates `~/.rad/config.yaml` with a `github` kind workspace, and commits changes with trailer `Radius-Action: init`.

2. **Given** a cloned GitHub repository, **When** the user runs `rad init --config-repo https://github.com/myorg/radius-config`, **Then** the system sets `RADIUS_RESOURCE_TYPES_MANIFEST` to `https://github.com/myorg/radius-config/types.yaml`.

3. **Given** a directory that is not a Git repository, **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to initialize a Git repository first or clone from GitHub.

4. **Given** a Git repository without a GitHub remote (origin), **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to add a GitHub remote.

5. **Given** the user is not authenticated with the GitHub CLI, **When** the user runs `rad init`, **Then** the system verifies `gh auth status` and displays an error instructing the user to authenticate using `gh auth login`.

6. **Given** a repository already initialized with Radius, **When** the user runs `rad init`, **Then** the system warns that Radius is already initialized and offers to reinitialize.

7. **Given** `rad init` no longer takes `--provider` or `--deployment-tool`, **When** the user passes those flags, **Then** the system returns an error indicating they are no longer supported.

---

### User Story 2 - Create Environment with Cloud Provider (Priority: P1)

A platform engineer needs to create a deployment target for an application. They run `rad environment create <name> --provider <aws|azure>` which creates a GitHub Environment via the GitHub API, sets up OIDC authentication with the cloud provider, and stores all configuration as environment-scoped variables.

**Why this priority**: Without an environment, there is nowhere to deploy. This replaces the previous `rad environment connect` command and is a prerequisite for all deployments.

**Independent Test**: Can be fully tested by running `rad environment create dev --provider azure` and verifying a GitHub Environment named "dev" is created with the correct environment variables set via the GitHub API.

**Acceptance Scenarios**:

1. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider azure`, **Then** the system creates a GitHub Environment named "dev" via the GitHub API, follows the OIDC setup flow (prompting for subscription, resource group, AKS cluster, namespace, and federated credential), and creates environment variables: `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP_NAME`, `AKS_CLUSTER_NAME`, `KUBERNETES_NAMESPACE`, `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, and `RADIUS_RECIPES_MANIFEST` (defaulting to `https://github.com/zachcasper/radius-config/recipes-azure-terraform.yaml`).

2. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider azure --deployment-tool bicep`, **Then** the system sets `RADIUS_RECIPES_MANIFEST` to `https://github.com/zachcasper/radius-config/recipes-azure-bicep.yaml`.

3. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider aws`, **Then** the system creates a GitHub Environment named "dev" via the GitHub API, follows the OIDC setup flow (prompting for account ID, region, EKS cluster, namespace, and IAM role), and creates environment variables: `AWS_ACCOUNT_ID`, `AWS_REGION`, `AWS_IAM_ROLE_NAME`, `EKS_CLUSTER_NAME`, `KUBERNETES_NAMESPACE`, and `RADIUS_RECIPES_MANIFEST` (defaulting to `https://github.com/zachcasper/radius-config/recipes-aws-terraform.yaml`).

4. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider aws --deployment-tool bicep`, **Then** the system returns an error because Bicep is not supported as a deployment tool for AWS.

5. **Given** a GitHub Environment named "dev" already exists, **When** the user runs `rad environment create dev --provider azure`, **Then** the system warns that the environment already exists and offers to update it.

6. **Given** the workspace kind is Kubernetes, **When** the user runs `rad environment create dev --provider azure`, **Then** the system retains the existing Kubernetes-mode `rad environment create` functionality (creating an environment resource in the Radius control plane).

7. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create prod --provider azure --recipes https://github.com/myorg/custom-recipes/recipes.yaml`, **Then** the system sets `RADIUS_RECIPES_MANIFEST` to the custom URL instead of the default.

---

### User Story 3 - Deploy Application (Priority: P1)

A developer or platform engineer wants to deploy an application to an environment. They run `rad deploy` which works in both GitHub and Kubernetes modes with flexible input options.

**Why this priority**: This is the core deployment action. Without it, no application can be deployed.

**Independent Test**: Can be fully tested by running `rad deploy --application todolist --environment dev` and verifying the application is deployed to the specified environment.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with one environment named "dev" and one application model at `.radius/model/todolist.bicep`, **When** the user runs `rad deploy`, **Then** the system automatically selects the "todolist" application and the "dev" environment and deploys.

2. **Given** a GitHub workspace with one environment and one application, **When** the user runs `rad deploy --application todolist --environment dev`, **Then** the system deploys the "todolist" application to the "dev" environment using the environment configuration from GitHub Environment variables.

3. **Given** a GitHub workspace with multiple environments (dev, staging, prod), **When** the user runs `rad deploy --application todolist` without specifying `--environment`, **Then** the system returns an error listing available environments and prompting the user to specify one.

4. **Given** a GitHub workspace with multiple applications in `.radius/model/`, **When** the user runs `rad deploy --environment dev` without specifying `--application` or a Bicep file, **Then** the system returns an error listing available applications and prompting the user to specify one.

5. **Given** a GitHub workspace, **When** the user runs `rad deploy ./custom/path/app.bicep --environment dev`, **Then** the system deploys using the specified Bicep file instead of resolving from `.radius/model/`.

6. **Given** a Kubernetes workspace, **When** the user runs `rad deploy app.bicep`, **Then** the system retains the existing Kubernetes-mode deploy functionality (deploying via the Radius control plane).

7. **Given** a GitHub workspace, **When** the user runs `rad deploy --application todolist --environment dev`, **Then** the system retrieves the environment configuration (cloud provider, cluster, namespace, recipes manifest) from the GitHub Environment variables and uses them during deployment.

8. **Given** a GitHub workspace with one application in `.radius/model/` and no Bicep file argument, **When** the user runs `rad deploy --environment dev`, **Then** the system automatically selects the single application.

---

### User Story 4 - Delete Environment (Priority: P2)

A platform engineer wants to remove a deployment target. They run `rad environment delete <name>` to remove the GitHub Environment and its associated configuration.

**Why this priority**: Environment lifecycle management is important but secondary to creation and deployment.

**Independent Test**: Can be fully tested by running `rad environment delete dev` and verifying the GitHub Environment is removed.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with a GitHub Environment named "dev", **When** the user runs `rad environment delete dev`, **Then** the system deletes the GitHub Environment via the GitHub API.

2. **Given** a GitHub workspace without a GitHub Environment named "dev", **When** the user runs `rad environment delete dev`, **Then** the system returns an error indicating the environment does not exist.

3. **Given** a Kubernetes workspace, **When** the user runs `rad environment delete dev`, **Then** the system retains the existing Kubernetes-mode delete functionality.

4. **Given** applications are currently deployed to the "dev" environment, **When** the user runs `rad environment delete dev`, **Then** the system warns that resources may still be deployed and asks for confirmation before proceeding.

---

### User Story 5 - Platform Engineer Configures Deployment Pipeline (Priority: P2)

A platform engineer wants to set up an automated deployment pipeline using GitHub Actions. Radius provides `rad deploy` as the building block; the platform engineer composes it into workflows triggered by GitHub events (PR, merge, manual dispatch).

**Why this priority**: Automated pipelines are critical for production use, but the building blocks (init, env create, deploy) must exist first.

**Independent Test**: Can be fully tested by configuring a GitHub Actions workflow that runs `rad deploy --application todolist --environment dev` on push to main, and verifying deployments occur automatically on merge.

**Acceptance Scenarios**:

1. **Given** `rad init` has generated baseline workflow files, **When** a platform engineer customizes the workflow to trigger on push to main, **Then** `rad deploy --application todolist --environment dev` runs automatically when PRs are merged.

2. **Given** a platform engineer wants preview on PRs, **When** they add a workflow triggered on `pull_request`, **Then** `rad deploy --application todolist --environment dev --what-if` runs and posts the plan output (future enhancement).

3. **Given** a platform engineer wants multi-environment promotion, **When** they create multiple workflow jobs with `needs:` dependencies and GitHub Environment approval gates, **Then** deployments flow from dev to staging to prod with approval at each stage.

4. **Given** a platform engineer wants manual deployments, **When** they configure the workflow with `workflow_dispatch` trigger and environment input, **Then** deployments are triggered manually from the GitHub Actions UI.

---

### User Story 6 - Create Application Model (Priority: P1)

After initializing the repository, a developer needs to create an application model. They run `rad model` to generate a starter Bicep file. This is unchanged from the existing behavior.

**Why this priority**: An application model is required before any deployment can occur.

**Independent Test**: Can be fully tested by running `rad model` in an initialized repository and verifying the application model file is created.

**Acceptance Scenarios**:

1. **Given** an initialized GitHub repository, **When** the user runs `rad model`, **Then** the system creates `.radius/model/todolist.bicep` with a sample application model.

2. **Given** a directory without Radius initialization, **When** the user runs `rad model`, **Then** the system displays an error instructing the user to run `rad init` first.

3. **Given** an application model already exists, **When** the user runs `rad model`, **Then** the system prompts the user to confirm overwriting or choose a different name.

---

### Edge Cases

- What happens when the GitHub API rate limit is exceeded during `rad environment create`? System displays a clear error with the rate limit reset time and suggests retrying later.
- What happens when the user's GitHub token lacks permissions to create environments or variables? System displays an error listing the required permissions (e.g., `admin:org`, environment write access).
- What happens when `--config-repo` points to a non-existent repository? System validates the URL and returns an error if the repository is not accessible.
- What happens when `rad deploy --application` references a name not found in `.radius/model/`? System returns an error listing available application models.
- What happens when GitHub Environment variables are missing or incomplete? `rad deploy` validates required variables before deployment and lists missing values.
- What happens when `rad environment create` is run with `--provider aws --deployment-tool bicep`? System returns an error because Bicep is only supported for Azure.
- What happens when a deployment is in progress and another is requested to the same environment? The system rejects with an error if same app/environment has an active deployment.

## Requirements *(mandatory)*

### Functional Requirements

#### CLI Command: rad init (Redesigned)

- **FR-001**: `rad init` MUST NOT take a `--provider` flag; provider configuration is handled by `rad environment create`.
- **FR-002**: `rad init` MUST NOT create a `.radius/types.yaml` manifest file.
- **FR-003**: `rad init` MUST NOT create a `.radius/recipes.yaml` manifest file.
- **FR-004**: `rad init` MUST NOT create a `.radius/env.*.yaml` environment file.
- **FR-005**: `rad init` MUST use the GitHub API to create a repository-level environment variable named `RADIUS_RESOURCE_TYPES_MANIFEST`.
- **FR-006**: `RADIUS_RESOURCE_TYPES_MANIFEST` MUST default to `https://github.com/zachcasper/radius-config/types.yaml`.
- **FR-007**: `rad init` MUST accept a `--config-repo` flag that overrides the default config repository URL. When specified, `RADIUS_RESOURCE_TYPES_MANIFEST` is set to `https://github.com/<config-repo>/types.yaml`.
- **FR-008**: `rad init` MUST continue to generate GitHub Actions workflow files in `.github/workflows/`.
- **FR-009**: `rad init` MUST continue to create a workspace of kind `github` in `~/.rad/config.yaml` with the repository URL as connection.
- **FR-010**: `rad init` MUST create empty directories `.radius/model/`, `.radius/plan/`, and `.radius/deploy/` with `.gitkeep` files.
- **FR-011**: `rad init` MUST validate the current directory is a Git repository with a GitHub remote.
- **FR-012**: `rad init` MUST verify GitHub CLI (`gh`) authentication via `gh auth status`.
- **FR-013**: `rad init` MUST commit changes with trailer `Radius-Action: init`.
- **FR-014**: `rad init` MUST NOT be interactive; all configuration comes from command-line flags.

#### CLI Command: rad environment create (New for GitHub mode)

- **FR-020**: `rad environment connect` MUST be renamed to `rad environment create`.
- **FR-021**: When the workspace kind is GitHub, `rad environment create` MUST follow the behavior defined in this section. When the workspace kind is Kubernetes, retain the existing `rad environment create` functionality.
- **FR-022**: `rad environment create <name>` MUST require a `--provider` flag with values `aws` or `azure`.
- **FR-023**: `rad environment create` MUST accept an optional `--deployment-tool` flag with values `terraform` or `bicep`.
- **FR-024**: When `--provider` is `aws`, the default deployment tool MUST be `terraform`.
- **FR-025**: When `--provider` is `azure`, the default deployment tool MUST be `bicep`.
- **FR-026**: When `--provider` is `aws` and `--deployment-tool` is `bicep`, the system MUST return an error because Bicep is not supported for AWS.
- **FR-027**: `rad environment create` MUST use the GitHub API to create a GitHub Environment with the specified name on the repository.
- **FR-028**: `rad environment create` MUST accept an optional `--recipes` flag to override the default recipes manifest URL.

##### Azure Provider Configuration

- **FR-030**: For Azure environments, `rad environment create` MUST follow the same OIDC setup flow as the current `rad environment connect` (prompting for subscription, resource group, AKS cluster, namespace, and federated credential).
- **FR-031**: For Azure environments, `rad environment create` MUST create the following environment variables within the GitHub Environment:
  - `AZURE_SUBSCRIPTION_ID`
  - `AZURE_RESOURCE_GROUP_NAME`
  - `AKS_CLUSTER_NAME`
  - `KUBERNETES_NAMESPACE`
  - `AZURE_TENANT_ID`
  - `AZURE_CLIENT_ID`
  - `RADIUS_RECIPES_MANIFEST`
- **FR-032**: When `--provider` is `azure` and `--deployment-tool` is `terraform` (or default), `RADIUS_RECIPES_MANIFEST` MUST default to `https://github.com/zachcasper/radius-config/recipes-azure-terraform.yaml`.
- **FR-033**: When `--provider` is `azure` and `--deployment-tool` is `bicep`, `RADIUS_RECIPES_MANIFEST` MUST default to `https://github.com/zachcasper/radius-config/recipes-azure-bicep.yaml`.

##### AWS Provider Configuration

- **FR-040**: For AWS environments, `rad environment create` MUST follow the same OIDC setup flow as the current `rad environment connect` (prompting for account ID, region, EKS cluster, namespace, and IAM role).
- **FR-041**: For AWS environments, `rad environment create` MUST create the following environment variables within the GitHub Environment:
  - `AWS_ACCOUNT_ID`
  - `AWS_REGION`
  - `AWS_IAM_ROLE_NAME`
  - `EKS_CLUSTER_NAME`
  - `KUBERNETES_NAMESPACE`
  - `RADIUS_RECIPES_MANIFEST`
- **FR-042**: When `--provider` is `aws`, `RADIUS_RECIPES_MANIFEST` MUST default to `https://github.com/zachcasper/radius-config/recipes-aws-terraform.yaml`.

#### CLI Command: rad environment delete (New for GitHub mode)

- **FR-050**: `rad environment delete <name>` MUST delete the GitHub Environment from the repository using the GitHub API when the workspace kind is GitHub.
- **FR-051**: When the workspace kind is Kubernetes, `rad environment delete` MUST retain the existing Kubernetes-mode delete functionality.
- **FR-052**: `rad environment delete` MUST warn the user if resources may still be deployed to the environment and ask for confirmation.

#### CLI Command: rad pr create (Removed)

- **FR-060**: The `rad pr create` command MUST be removed entirely. Radius does not own the PR workflow.
- **FR-061**: The `rad pr merge` command MUST be removed entirely.
- **FR-062**: The `rad pr destroy` command MUST be removed entirely.

#### CLI Command: rad deploy (Enhanced)

- **FR-070**: When the workspace kind is GitHub, `rad deploy` MUST use the environment specified by the GitHub Environment (retrieved via GitHub API or workflow context).
- **FR-071**: `rad deploy` MUST accept an optional `--environment <name>` flag. If more than one environment exists on the repository and `--environment` is not specified, the system MUST return an error listing available environments.
- **FR-072**: `rad deploy <app.bicep>` MUST work in both GitHub mode and Kubernetes mode, deploying the specified Bicep file.
- **FR-073**: `rad deploy --application <name>` MUST work only in GitHub mode. When `--application` is specified, Radius looks for the application named `<name>` in the `.radius/model/` directory (e.g., `.radius/model/<name>.bicep`).
- **FR-074**: When neither `--application` nor a positional Bicep file is specified, and only one application exists in `.radius/model/`, Radius MUST automatically select that application.
- **FR-075**: When neither `--application` nor a positional Bicep file is specified, and more than one application exists in `.radius/model/`, Radius MUST return an error listing available applications.
- **FR-076**: When `--application` is specified but the named application does not exist in `.radius/model/`, Radius MUST return an error indicating the application was not found.
- **FR-077**: In GitHub mode, `rad deploy` MUST retrieve environment configuration (cloud provider credentials, cluster name, namespace, recipes manifest) from the GitHub Environment variables.

#### GitHub Actions Workflows

- **FR-080**: `rad init` MUST generate a deploy workflow at `.github/workflows/radius-deploy.yml`.
- **FR-081**: `rad init` MUST generate a destroy workflow at `.github/workflows/radius-destroy.yml`.
- **FR-082**: Generated workflows MUST reference environment configuration from GitHub Environment variables (not from `.radius/env.*.yaml` files).
- **FR-083**: Generated workflows MUST use `environment:` in job definitions to leverage GitHub Environment protection rules.
- **FR-084**: Generated workflows MUST only use external GitHub Actions from trusted sources (GitHub official, Radius Project, cloud provider official, HashiCorp official).

#### Configuration Storage (Redesigned)

- **FR-090**: Resource types manifest location MUST be stored as a repository-level environment variable `RADIUS_RESOURCE_TYPES_MANIFEST` (not as a `.radius/types.yaml` file).
- **FR-091**: Recipes manifest location MUST be stored as an environment-scoped variable `RADIUS_RECIPES_MANIFEST` within each GitHub Environment.
- **FR-092**: Cloud provider configuration (subscription IDs, cluster names, OIDC credentials) MUST be stored as environment-scoped variables within each GitHub Environment.
- **FR-093**: Application models MUST continue to be stored in `.radius/model/<APP_NAME>.bicep`.
- **FR-094**: Plans MUST continue to be stored in `.radius/plan/<APP_NAME>/<ENVIRONMENT_NAME>/`.
- **FR-095**: Deployment records MUST continue to be stored in `.radius/deploy/<APP_NAME>/<ENVIRONMENT_NAME>/<COMMIT>/`.
- **FR-096**: Workspaces MUST continue to be stored in `~/.rad/config.yaml`.

#### Workspace Behavior

- **FR-100**: GitHub workspaces MUST have `connection.kind: github` and `connection.url` pointing to the repository.
- **FR-101**: Commands that are GitHub-mode-specific (e.g., `rad deploy --application`) MUST validate the workspace kind before executing.
- **FR-102**: Commands that are Kubernetes-mode-specific MUST continue to work unchanged.

### Key Entities

- **Workspace**: User's working context stored in `~/.rad/config.yaml`; can be of kind `github` (URL-based connection) or `kubernetes` (context-based connection).

- **GitHub Environment**: A deployment target created via the GitHub API. Stores cloud provider configuration, recipes manifest URL, and cluster details as environment-scoped variables. Maps 1:1 to a Radius environment.

- **Resource Types Manifest**: A YAML file hosted in a configuration repository that defines available Radius resource types. Referenced by the repository-level variable `RADIUS_RESOURCE_TYPES_MANIFEST`.

- **Recipes Manifest**: A YAML file hosted in a configuration repository that defines available recipes for a specific provider and deployment tool combination. Referenced by the environment-scoped variable `RADIUS_RECIPES_MANIFEST`.

- **Application Model**: Bicep-based declaration of application resources stored in `.radius/model/<APP_NAME>.bicep`. Unchanged from current format.

- **Config Repository**: An external repository containing resource type and recipe manifest files. Defaults to `zachcasper/radius-config`. Can be overridden with `--config-repo` on `rad init` or `--recipes` on `rad environment create`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a GitHub repository for Radius in under 2 minutes (`rad init` is faster without types/recipes fetching).
- **SC-002**: Platform engineers can create a new deployment environment in under 10 minutes including OIDC setup via `rad environment create`.
- **SC-003**: Users can deploy an application with a single command (`rad deploy`) when only one application and one environment exist.
- **SC-004**: 95% of deployment attempts complete within 15 minutes for applications with 5 or fewer resources.
- **SC-005**: Platform engineers can set up multi-environment promotion pipelines using standard GitHub Actions without any Radius-specific pipeline abstraction.
- **SC-006**: Environment deletion completes in under 30 seconds via `rad environment delete`.
- **SC-007**: Users can deploy with `rad deploy ./path/to/app.bicep` in both GitHub and Kubernetes modes.
- **SC-008**: 90% of users can complete their first deployment without external documentation beyond CLI help text.

## Assumptions

- Users have the GitHub CLI (`gh`) installed and authenticated with sufficient permissions to create GitHub Environments and environment variables.
- Users have cloud provider CLIs (`aws` or `az`) installed and authenticated for OIDC setup.
- The `zachcasper/radius-config` repository (or custom config repo) contains valid resource types and recipes manifest files.
- GitHub repository has GitHub Actions enabled with sufficient minutes quota.
- GitHub-hosted runners have sufficient resources to run k3d clusters.
- Users are familiar with Git workflows and GitHub Pull Requests.
- The GitHub API supports creating environments and environment variables (requires repository admin permissions).
