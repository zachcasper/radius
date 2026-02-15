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

- Q: How should the CLI behave while a dispatched GitHub Action workflow runs? → A: CLI shows contextual message (e.g., "Creating deployment..."), then displays an animated progress indicator. User can press L to toggle real-time log streaming.
- Q: How should concurrent deployments to the same app/environment be locked? → A: GitHub Actions concurrency groups using the `concurrency:` key in workflow YAML scoped to app-env.
- Q: How does `rad app delete` execute in GitHub mode? → A: Dispatches a GitHub Action workflow that runs destroy operations using stored deployment artifacts and cloud OIDC credentials, consistent with `rad deployment create`/`apply`.
- Q: How is the commit hash resolved for `rad deployment create` and `rad deployment apply`? → A: Both default to the latest plan under `.radius/deploy/<APP>/<ENV>/`; user can specify `--git-commit <hash>` to target a specific commit.
- Q: Should `rad environment create` verify that OIDC authentication works after setup? → A: Yes, auto-verify by dispatching a lightweight GitHub Action workflow that authenticates to the cloud provider via OIDC and reports success/failure.

## Overview

Radius on GitHub is a new operational mode that enables users to deploy cloud applications using Radius without requiring a centralized Kubernetes-based control plane. Environment configuration is stored via the GitHub Environments API, application definitions and deployment artifacts are stored in the repository, and deployments execute in ephemeral k3d clusters within GitHub Action runners. The two-phase model (`rad deployment create` + `rad deployment apply`) separates plan generation from execution, enabling review, auditability, and incremental adoption.

This mode complements the existing "Radius on Kubernetes" mode, giving users the choice of how they want to operate Radius based on their infrastructure preferences.

### Radius on Kubernetes vs Radius on GitHub

| Concept | Radius on Kubernetes | Radius on GitHub |
|---------|---------------------|-----------------|
| **Environments** | Created via `rad environment create`; stored in the Radius control plane | Created via `rad environment create`; stored using GitHub Environments API |
| **Credentials** | Created via `rad credential register` | OIDC configured automatically as part of `rad environment create` |
| **Resource Types** | Created via `rad resource-type create` | A manifest of resource type definitions is specified in a GitHub repository variable (`RADIUS_RESOURCE_TYPES_MANIFEST`), set during `rad init` |
| **Recipes** | Recipe Packs stored in the control plane and referenced in the Environment resource | An environment-scoped variable (`RADIUS_RECIPES_MANIFEST`) points to a manifest of recipes for each resource type |
| **Resource Groups** | Managed via `rad group` commands | No Radius resource groups |
| **Workspaces** | Points to a kube context describing the Kubernetes cluster running Radius | A URL to the GitHub repository |

### Key Characteristics

- **Two-phase deployment**: `rad deployment create` generates a deployment plan and artifacts; `rad deployment apply` executes the plan. This separation enables review, auditability, and incremental adoption (users can generate plans with Radius but deploy with their own tools).
- **GitHub API storage**: Environment configuration stored as GitHub Environment-scoped variables; resource types manifest referenced via GitHub repository variable
- **GitHub Actions execution**: Both `rad deployment create` and `rad deployment apply` dispatch GitHub Action workflows that run in ephemeral k3d clusters
- **OIDC authentication**: Secure, credential-free authentication to AWS and Azure via OIDC federation
- **CLI-driven**: Familiar `rad` CLI commands: `rad init --github`, `rad environment create`, `rad deployment create`, `rad deployment apply`

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize Repository for Radius (Priority: P1)

A developer wants to use Radius with their existing GitHub repository. They run `rad init` to set up the repository structure and GitHub Actions workflows. The command no longer requires a `--provider` flag or creates environment-specific configuration — that is handled separately by `rad environment create`.

**Why this priority**: This is the entry point for all Radius on GitHub functionality. Without initialization, no other features can be used.

**Independent Test**: Can be fully tested by running `rad init` on a fresh GitHub repository clone and verifying the workspace is registered, GitHub Actions workflows are created, the RADIUS_RESOURCE_TYPES_MANIFEST variable is set, and changes are committed.

**Acceptance Scenarios**:

1. **Given** a cloned GitHub repository without Radius configuration, **When** the user runs `rad init`, **Then** the system creates `.radius/applications/` and `.radius/deploy/` directories (with `.gitkeep` files), generates GitHub Actions workflow files, uses the GitHub API to create a repository environment variable `RADIUS_RESOURCE_TYPES_MANIFEST` with the default value `https://github.com/zachcasper/radius-config/types.yaml`, updates `~/.rad/config.yaml` with a `github` kind workspace, commits changes with trailer `Radius-Action: init`, and pushes to the remote.

2. **Given** a cloned GitHub repository, **When** the user runs `rad init --resource-types-manifest https://github.com/myorg/radius-config/types.yaml`, **Then** the system sets `RADIUS_RESOURCE_TYPES_MANIFEST` to `https://github.com/myorg/radius-config/types.yaml`.

3. **Given** a directory that is not a Git repository, **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to initialize a Git repository first or clone from GitHub.

4. **Given** a Git repository without a GitHub remote (origin), **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to add a GitHub remote.

5. **Given** the user is not authenticated with the GitHub CLI, **When** the user runs `rad init`, **Then** the system verifies `gh auth status` and displays an error instructing the user to authenticate using `gh auth login`.

6. **Given** a repository already initialized with Radius, **When** the user runs `rad init`, **Then** the system warns that Radius is already initialized and offers to reinitialize.

7. **Given** `rad init` no longer takes `--provider` or `--deployment-tool`, **When** the user passes those flags, **Then** the system returns an error indicating they are no longer supported.

---

### User Story 2 - Create Environment with Cloud Provider (Priority: P1)

A platform engineer needs to create a deployment target for an application. They run `rad environment create <name> --provider <aws|azure>` which creates a GitHub Environment via the GitHub API, sets up OIDC authentication with the cloud provider, and stores all configuration as environment-scoped variables in GitHub.

**Why this priority**: Without an environment, there is nowhere to deploy. This replaces the previous `rad environment connect` command and is a prerequisite for all deployments.

**Independent Test**: Can be fully tested by running `rad environment create dev --provider azure` and verifying a GitHub Environment named "dev" is created with the correct environment variables set via the GitHub API.

**Acceptance Scenarios**:

1. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider azure`, **Then** the system creates a GitHub Environment named "dev" via the GitHub API, follows the OIDC setup flow (prompting for subscription, resource group, AKS cluster, namespace, and federated credential), and creates environment variables: `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP_NAME`, `AKS_CLUSTER_NAME`, `KUBERNETES_NAMESPACE`, `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, and `RADIUS_RECIPES_MANIFEST` (defaulting to `https://github.com/zachcasper/radius-config/recipes-azure-bicep.yaml`).

2. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider azure --deployment-tool terraform`, **Then** the system sets `RADIUS_RECIPES_MANIFEST` to `https://github.com/zachcasper/radius-config/recipes-azure-terraform.yaml`.

3. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider aws`, **Then** the system creates a GitHub Environment named "dev" via the GitHub API, follows the OIDC setup flow (prompting for account ID, region, EKS cluster, namespace, and IAM role), and creates environment variables: `AWS_ACCOUNT_ID`, `AWS_REGION`, `AWS_IAM_ROLE_NAME`, `EKS_CLUSTER_NAME`, `KUBERNETES_NAMESPACE`, and `RADIUS_RECIPES_MANIFEST` (defaulting to `https://github.com/zachcasper/radius-config/recipes-aws-terraform.yaml`).

4. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create dev --provider aws --deployment-tool bicep`, **Then** the system returns an error because Bicep is not supported as a deployment tool for AWS.

5. **Given** a GitHub Environment named "dev" already exists, **When** the user runs `rad environment create dev --provider azure`, **Then** the system warns that the environment already exists and offers to update it.

6. **Given** the workspace kind is Kubernetes, **When** the user runs `rad environment create dev`, **Then** the system retains the existing Kubernetes-mode `rad environment create` functionality (creating an environment resource in the Radius control plane).

7. **Given** an initialized GitHub workspace, **When** the user runs `rad environment create prod --provider azure --recipes https://github.com/myorg/custom-recipes/recipes.yaml`, **Then** the system sets `RADIUS_RECIPES_MANIFEST` to the custom URL instead of the default.

8. **Given** AWS CLI is not installed or authenticated, **When** the user chooses to create a new IAM role, **Then** the system displays an error with instructions to install/authenticate AWS CLI.

9. **Given** OIDC setup completes successfully, **When** environment variables are stored, **Then** the system dispatches a lightweight authentication test workflow that verifies the OIDC credentials can authenticate to the cloud provider and reports success or failure to the CLI.

10. **Given** the authentication test workflow fails, **When** the CLI displays the result, **Then** the error message identifies the likely OIDC misconfiguration (e.g., wrong audience, wrong subject claim) and suggests remediation steps.

9. **Given** the current directory is not a GitHub workspace, **When** the user runs `rad environment create`, **Then** the system displays an error message indicating the command requires a GitHub workspace.

---

### User Story 3 - Create Application Definition (Priority: P1)

After connecting their cloud provider, a developer needs to create an application definition that defines the resources their application requires. They run `rad app model` to generate a starter application definition file.

**Why this priority**: An application definition is required before any deployment can occur. This creates the foundational Bicep file that describes the application's resource requirements. The current implementation is a placeholder for future AI-assisted modeling functionality.

**Independent Test**: Can be fully tested by running `rad app model` in an initialized repository and verifying the application definition file is created with the correct structure.

**Acceptance Scenarios**:

1. **Given** an initialized GitHub repository with Radius configuration, **When** the user runs `rad app model`, **Then** the system creates `.radius/applications/todolist.bicep` with a sample application definition containing an application resource, a container resource, and a database resource.

2. **Given** an initialized repository, **When** the user runs `rad app model`, **Then** the generated definition includes proper Bicep syntax with `extension radius`, parameter declarations, and resource definitions using the `2025-08-01-preview` API version.

3. **Given** the application definition is created, **When** the user inspects the file, **Then** it contains a `Radius.Core/applications` resource, a `Radius.Compute/containers` resource with container configuration and connections, and a `Radius.Data/postgreSqlDatabases` resource.

4. **Given** a directory without Radius initialization, **When** the user runs `rad app model`, **Then** the system displays an error message instructing the user to run `rad init` first.

5. **Given** an application definition already exists at `.radius/applications/todolist.bicep`, **When** the user runs `rad app model`, **Then** the system prompts the user to confirm overwriting the existing file or choose a different name.

---

### User Story 4 - Create Deployment Plan (Priority: P1)

A developer or platform engineer wants to generate a deployment plan that shows what resources will be provisioned and produces the deployment artifacts (Terraform configurations, Bicep templates) needed to execute the deployment. They run `rad deployment create` which dispatches a GitHub Action workflow that spins up an ephemeral k3d cluster, runs the Radius control plane's plan API, and commits the resulting plan and artifacts to the repository.

**Why this priority**: The deployment plan is a prerequisite for deployment execution. Separating plan creation from execution enables review, auditability, and incremental adoption — users can generate plans with Radius but deploy with their own tools.

**Independent Test**: Can be fully tested by running `rad deployment create --application todolist --environment dev` and verifying a GitHub Action workflow is dispatched, a deployment plan is generated at `.radius/deploy/todolist/dev/<COMMIT_HASH>/deploy.yaml`, and deployment artifacts are committed to the repository.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with an environment and application definition, **When** the user runs `rad deployment create --application todolist --environment dev`, **Then** the system dispatches a GitHub Action workflow that generates a deployment plan and artifacts at `.radius/deploy/todolist/dev/<COMMIT_HASH>/` and commits them to the repository.

2. **Given** a GitHub workspace with one environment and one application, **When** the user runs `rad deployment create`, **Then** the system auto-selects the application and environment (same resolution rules as `rad deployment apply`).

3. **Given** a GitHub workspace with multiple environments, **When** the user runs `rad deployment create --application todolist` without specifying `--environment`, **Then** the system returns an error listing available environments.

4. **Given** a GitHub workspace with multiple applications in `.radius/applications/`, **When** the user runs `rad deployment create --environment dev` without specifying `--application`, **Then** the system returns an error listing available applications.

5. **Given** the deployment plan is generated, **When** the user reviews the committed files, **Then** they can see `deploy.yaml` with application name, environment, ordered deployment steps, and each step's deployment artifact directory containing Terraform/Bicep configurations.

6. **Given** a Kubernetes workspace, **When** the user runs `rad deployment create`, **Then** the system returns an error indicating this command is only available in GitHub mode (Kubernetes mode uses `rad deploy` directly).

7. **Given** a GitHub workspace with uncommitted changes, **When** the user runs `rad deployment create`, **Then** the system returns an error instructing the user to commit and push all changes before generating a deployment.

8. **Given** a GitHub workspace with committed but unpushed changes, **When** the user runs `rad deployment create`, **Then** the system returns an error instructing the user to push changes to the remote before generating a deployment.

---

### User Story 5 - Apply Deployment (Priority: P1)

A developer or platform engineer wants to execute a previously created deployment plan. They run `rad deployment apply` which dispatches a GitHub Action workflow that provisions the resources according to the deployment plan artifacts already committed to the repository.

**Why this priority**: This is the core deployment execution action. Without it, no application can be deployed.

**Independent Test**: Can be fully tested by running `rad deployment apply --application todolist --environment dev` (after a deployment plan exists) and verifying the application is deployed to the specified environment.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with a deployment plan at `.radius/deploy/todolist/dev/<COMMIT_HASH>/deploy.yaml`, **When** the user runs `rad deployment apply --application todolist --environment dev`, **Then** the system dispatches a GitHub Action workflow that executes the deployment plan, provisions resources, and captures deployment results.

2. **Given** a GitHub workspace with one environment and one application with a deployment plan, **When** the user runs `rad deployment apply`, **Then** the system auto-selects the application and environment.

3. **Given** a GitHub workspace with multiple environments, **When** the user runs `rad deployment apply --application todolist` without specifying `--environment`, **Then** the system returns an error listing available environments.

4. **Given** a GitHub workspace with multiple applications, **When** the user runs `rad deployment apply --environment dev` without specifying `--application`, **Then** the system returns an error listing available applications.

5. **Given** no deployment plan exists for the specified application and environment, **When** the user runs `rad deployment apply --application todolist --environment dev`, **Then** the system returns an error instructing the user to run `rad deployment create` first.

6. **Given** a GitHub workspace, **When** the deployment workflow completes, **Then** the system captures the deployed cloud resources in their native format (Kubernetes YAML, AWS JSON, Azure JSON) in `.radius/deploy/todolist/dev/<COMMIT_HASH>/<step>/resources/`.

7. **Given** a Kubernetes workspace, **When** the user runs `rad deployment apply`, **Then** the system returns an error indicating this command is only available in GitHub mode (Kubernetes mode uses `rad deploy` directly).

---

### User Story 6 - Delete Deployed Application (Priority: P1)

A developer or platform engineer wants to tear down a previously deployed application from an environment. They run `rad app delete --application <name> --environment <name>` which dispatches a GitHub Action workflow that destroys all resources belonging to that application in the specified environment using the stored deployment artifacts and cloud OIDC credentials.

**Why this priority**: The ability to clean up deployed resources is essential for environment management and cost control.

**Independent Test**: Can be fully tested by deploying an application to an environment, running `rad app delete --application todolist --environment dev`, and verifying all application resources are removed.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with "todolist" deployed to the "dev" environment, **When** the user runs `rad app delete --application todolist --environment dev`, **Then** the system dispatches a GitHub Action workflow that destroys all resources belonging to the "todolist" application in the "dev" environment using the stored deployment artifacts.

2. **Given** a GitHub workspace with one environment and one application, **When** the user runs `rad app delete`, **Then** the system auto-selects the application and environment (same resolution rules as `rad deployment create`).

3. **Given** a GitHub workspace with multiple environments, **When** the user runs `rad app delete --application todolist` without specifying `--environment`, **Then** the system returns an error listing available environments.

4. **Given** a GitHub workspace with multiple applications deployed, **When** the user runs `rad app delete --environment dev` without specifying `--application`, **Then** the system returns an error listing deployed applications in that environment.

5. **Given** no application named "todolist" is deployed to the "dev" environment, **When** the user runs `rad app delete --application todolist --environment dev`, **Then** the system returns an error indicating the application is not deployed to that environment.

6. **Given** a Kubernetes workspace, **When** the user runs `rad app delete --application todolist`, **Then** the system retains the existing Kubernetes-mode destroy functionality.

---

### User Story 7 - Delete Environment (Priority: P2)

A platform engineer wants to remove a deployment target. They run `rad environment delete <name>` to remove the GitHub Environment and its associated configuration.

**Why this priority**: Environment lifecycle management is important but secondary to creation and deployment.

**Independent Test**: Can be fully tested by running `rad environment delete dev` and verifying the GitHub Environment is removed via the GitHub API.

**Acceptance Scenarios**:

1. **Given** a GitHub workspace with a GitHub Environment named "dev", **When** the user runs `rad environment delete dev`, **Then** the system deletes the GitHub Environment via the GitHub API.

2. **Given** a GitHub workspace without a GitHub Environment named "dev", **When** the user runs `rad environment delete dev`, **Then** the system returns an error indicating the environment does not exist.

3. **Given** a Kubernetes workspace, **When** the user runs `rad environment delete dev`, **Then** the system retains the existing Kubernetes-mode delete functionality.

4. **Given** applications are currently deployed to the "dev" environment, **When** the user runs `rad environment delete dev`, **Then** the system prompts the user to (1) delete the environment and the applications deployed (the default); (2) delete the environment and leave the deployed applications but manage them outside of Radius. Asks for confirmation before proceeding with either options..

---

### User Story 8 - Platform Engineer Configures Deployment Pipeline (Priority: P2)

A platform engineer wants to set up an automated deployment pipeline using GitHub Actions. Radius provides `rad deployment create` and `rad deployment apply` as the building blocks; the platform engineer composes them into workflows triggered by GitHub events (PR, merge, manual dispatch). Radius does not own the pipeline — it provides the deployment verbs, and GitHub provides the orchestration.

**Why this priority**: Automated pipelines are critical for production use, but the building blocks (init, env create, deployment create/apply) must exist first.

**Independent Test**: Can be fully tested by configuring a GitHub Actions workflow that runs `rad deployment create` on PR and `rad deployment apply` on merge to main, and verifying the two-phase flow executes automatically.

**Acceptance Scenarios**:

1. **Given** `rad init` has generated baseline workflow files, **When** a platform engineer customizes the workflow to run `rad deployment create` on PR and `rad deployment apply` on merge to main, **Then** PRs generate deployment plans for review and merges execute the deployment.

2. **Given** a platform engineer wants multi-environment promotion, **When** they create multiple workflow jobs with `needs:` dependencies and GitHub Environment approval gates, **Then** deployments flow from dev to staging to prod with plan review and approval at each stage.

3. **Given** a platform engineer wants manual deployments, **When** they configure the workflow with `workflow_dispatch` trigger and environment input, **Then** `rad deployment create` and `rad deployment apply` are triggered manually from the GitHub Actions UI.

4. **Given** a platform engineer wants to use Radius only for planning, **When** they configure a workflow that runs only `rad deployment create`, **Then** they can extract the deployment artifacts and use their own tools (e.g., `terraform apply`) for execution.

---

### User Story 9 - Manage Workspaces Across Repositories (Priority: P3)

A developer works with multiple repositories, some using Radius on GitHub and others using Radius on Kubernetes. The workspace configuration allows seamless switching between different contexts.

**Why this priority**: Multi-repository support enhances user experience but is not essential for single-repo usage.

**Independent Test**: Can be fully tested by configuring multiple workspaces in `~/.rad/config.yaml` and switching between them while verifying command behavior changes appropriately.

**Acceptance Scenarios**:

1. **Given** multiple workspaces are configured (some `github` kind, some `kubernetes` kind), **When** the user switches workspaces, **Then** subsequent commands use the selected workspace's configuration and connection type.

2. **Given** a GitHub workspace is current, **When** the user runs `rad deployment create --application todolist --environment dev`, **Then** the system resolves the application from `.radius/applications/` and environment from GitHub Environment variables.

3. **Given** a Kubernetes workspace is current, **When** the user runs commands, **Then** they operate against the Kubernetes control plane as before.

4. **Given** the user wants to use Kubernetes-based Radius, **When** they run `rad install kubernetes`, **Then** the traditional Kubernetes control plane is installed (the new `rad init` does not replace this path).

---

### User Story 10 - Review Deployment Plans (Priority: P3)

A developer or reviewer needs to understand what changes a deployment will make before applying it. The deployment plan committed by `rad deployment create` should be clear and auditable.

**Why this priority**: Auditability is important for production deployments but builds on the core deployment functionality.

**Independent Test**: Can be fully tested by running `rad deployment create` and reviewing the committed plan files to verify they clearly document expected changes.

**Acceptance Scenarios**:

1. **Given** `rad deployment create` has been run, **When** a reviewer opens the repository, **Then** they can see `.radius/deploy/<app>/<env>/<commit>/deploy.yaml` with application name, environment, and ordered deployment steps.

2. **Given** a deployment step in the plan, **When** the reviewer examines the step, **Then** they can see the resource name, type, properties, recipe information, deployment artifacts path, and expected changes (add/change/destroy counts).

3. **Given** a deployment artifact directory, **When** the reviewer examines the files, **Then** they can see the Terraform configuration (main.tf), providers (providers.tf), variables (variables.tf, terraform.tfvars.json), and the plan output (tfplan.txt).

4. **Given** the plan summary, **When** the reviewer examines it, **Then** they can see total steps, Terraform vs Bicep steps, total resources to add/change/destroy, and whether all versions are pinned.

5. **Given** a platform engineer wants to use their own deployment tools, **When** they review the artifacts generated by `rad deployment create`, **Then** they can extract the Terraform/Bicep configurations and run `terraform apply` or `az deployment create` directly.

---

### Edge Cases

- What happens when the GitHub repository has no Actions enabled? System displays an error with instructions to enable GitHub Actions in repository settings.
- What happens when OIDC role permissions are insufficient? Deployment fails with clear error message indicating missing permissions and which permissions are needed.
- What happens when a deployment is in progress and another is requested? GitHub Actions concurrency groups queue the new run until the in-progress run completes; parallel deployments to different environments are unaffected.
- What happens when network connectivity to GitHub is lost during deployment? The GitHub Action retries or fails gracefully with state preserved in Terraform state.
- What happens when the config repository is unavailable during `rad init`? Error with clear message and retry instructions.
- What happens when Terraform state conflicts occur? Deployment fails with instructions to resolve state conflicts manually.
- What happens when the k3d cluster fails to start in GitHub Actions? Workflow fails with diagnostic information about resource constraints or configuration issues.
- What happens when the GitHub API rate limit is exceeded during `rad environment create`? System displays a clear error with the rate limit reset time and suggests retrying later.
- What happens when the user's GitHub token lacks permissions to create environments or variables? System displays an error listing the required permissions.
- What happens when `--resource-types-manifest` points to a non-existent URL? System validates the URL and returns an error if the manifest is not accessible.
- What happens when `rad deployment create --application` references a name not found in `.radius/applications/`? System returns an error listing available application definitions.
- What happens when GitHub Environment variables are missing or incomplete? `rad deployment apply` validates required variables before deployment and lists missing values.
- What happens when `rad environment create` is run with `--provider aws --deployment-tool bicep`? System returns an error because Bicep is only supported for Azure.
- What happens when `rad deployment apply` is run but no deployment plan exists? System returns an error instructing the user to run `rad deployment create` first.
- What happens when `rad deployment create` or `rad deployment apply` is run in a Kubernetes workspace? System returns an error indicating the command is only available in GitHub mode.
- What happens when `rad deployment create` is run with uncommitted changes? System returns an error instructing the user to commit and push all changes before generating a deployment.
- What happens when `rad deployment create` is run with committed but unpushed changes? System returns an error instructing the user to push changes to the remote first.

## Requirements *(mandatory)*

### Functional Requirements

#### CLI Command: rad init

- **FR-001**: `rad init` MUST NOT take a `--provider` flag; provider configuration is handled by `rad environment create`.
- **FR-002**: `rad init` MUST NOT create a `.radius/types.yaml` manifest file.
- **FR-003**: `rad init` MUST NOT create a `.radius/recipes.yaml` manifest file.
- **FR-004**: `rad init` MUST NOT create a `.radius/env.*.yaml` environment file.
- **FR-005**: `rad init` MUST use the GitHub API to create a repository-level environment variable named `RADIUS_RESOURCE_TYPES_MANIFEST`.
- **FR-006**: `RADIUS_RESOURCE_TYPES_MANIFEST` MUST default to `https://github.com/zachcasper/radius-config/types.yaml`.
- **FR-007**: `rad init` MUST accept a `--resource-types-manifest` flag that overrides the default resource types manifest URL. When specified, `RADIUS_RESOURCE_TYPES_MANIFEST` is set to the provided URL.
- **FR-008**: System MUST validate the current directory is a Git repository by checking for `.git` directory.
- **FR-009**: System MUST validate the Git repository has a GitHub remote (origin) by parsing the remote URL.
- **FR-010**: System MUST verify GitHub CLI (`gh`) is authenticated by running `gh auth status`.
- **FR-011**: System MUST update or create `~/.rad/config.yaml` with a new workspace of kind `github` using the repository URL as connection; workspace name MUST match the repository name.
- **FR-012**: System MUST rename the workspace config property `default` to `current` for clarity.
- **FR-013**: System MUST commit changes with `git add` and `git commit` including the trailer `Radius-Action: init`, then push to the remote with `git push`.
- **FR-014**: System MUST NOT be interactive; all configuration comes from command-line flags.
- **FR-014-A**: System MUST create empty directories `.radius/applications/` and `.radius/deploy/` for storing application definitions and deployment artifacts respectively. Each directory MUST contain a `.gitkeep` file to ensure the directories are tracked by Git.

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
  - `AZURE_SUBSCRIPTION_ID`
  - `AZURE_RESOURCE_GROUP_NAME`
  - `AKS_CLUSTER_NAME`
  - `KUBERNETES_NAMESPACE`
  - `AZURE_TENANT_ID`
  - `AZURE_CLIENT_ID`
  - `RADIUS_RECIPES_MANIFEST`
- **FR-026**: When `--provider` is `azure` and `--deployment-tool` is `bicep` (or default), `RADIUS_RECIPES_MANIFEST` MUST default to `https://github.com/zachcasper/radius-config/recipes-azure-bicep.yaml`.
- **FR-027**: When `--provider` is `azure` and `--deployment-tool` is `terraform`, `RADIUS_RECIPES_MANIFEST` MUST default to `https://github.com/zachcasper/radius-config/recipes-azure-terraform.yaml`.
- **FR-028**: For AWS environments, system MUST follow the same OIDC setup flow as the current `rad environment connect` (prompting for account ID, region, EKS cluster, namespace, and IAM role).
- **FR-029**: For AWS environments, system MUST create the following environment variables within the GitHub Environment:
  - `AWS_ACCOUNT_ID`
  - `AWS_REGION`
  - `AWS_IAM_ROLE_NAME`
  - `EKS_CLUSTER_NAME`
  - `KUBERNETES_NAMESPACE`
  - `RADIUS_RECIPES_MANIFEST`
- **FR-030**: When `--provider` is `aws`, `RADIUS_RECIPES_MANIFEST` MUST default to `https://github.com/zachcasper/radius-config/recipes-aws-terraform.yaml`.
- **FR-030-A**: System MUST NOT store cloud secret keys locally; only OIDC-related identifiers are stored as GitHub Environment variables.
- **FR-030-E**: After storing environment variables, `rad environment create` MUST dispatch a lightweight GitHub Action authentication test workflow that verifies OIDC federation works by authenticating to the cloud provider.
- **FR-030-F**: The authentication test workflow MUST use the same GitHub Environment and OIDC credentials that deployment workflows will use.
- **FR-030-G**: The CLI MUST display "Creating authentication test workflow..." followed by an animated progress indicator showing "Testing authentication to <provider>..." with `L` key support for real-time log streaming (consistent with FR-089-C through FR-089-G).
- **FR-030-H**: If the authentication test fails, the CLI MUST display a clear error identifying the likely OIDC misconfiguration and suggest remediation steps. The GitHub Environment and variables MUST remain in place so the user can fix and re-run verification.
- **FR-030-I**: If the authentication test succeeds, the CLI MUST display a success message confirming the environment is ready for deployments.

#### CLI Command: rad environment delete

- **FR-030-B**: `rad environment delete <name>` MUST delete the GitHub Environment from the repository using the GitHub API when the workspace kind is GitHub.
- **FR-030-C**: When the workspace kind is Kubernetes, `rad environment delete` MUST retain the existing Kubernetes-mode delete functionality.
- **FR-030-D**: `rad environment delete` MUST warn the user if resources may still be deployed to the environment and ask for confirmation.

#### CLI Commands: rad pr create, rad pr merge, rad pr destroy (Removed)

- **FR-032**: The `rad pr create` command MUST be removed entirely. Radius does not own the PR workflow.
- **FR-033**: The `rad pr merge` command MUST be removed entirely.
- **FR-034**: The `rad pr destroy` command MUST be removed entirely.
- **FR-035**: Platform engineers compose deployment pipelines using standard GitHub Actions with `rad deployment create` and `rad deployment apply` as the building blocks.

#### CLI Command: rad deploy (Enhanced)

- **FR-036**: `rad deploy` MUST only be available in Kubernetes mode. In Kubernetes mode, `rad deploy` behaves as the current Radius deployment command using the current workspace's Kubernetes context. In GitHub mode, the system MUST return an error indicating that `rad deployment create` and `rad deployment apply` should be used instead.
- **FR-037**: `rad deploy` MUST NOT be used for GitHub-mode deployments. GitHub-mode deployments use `rad deployment create` and `rad deployment apply`.

#### CLI Command: rad deployment create (GitHub mode only)

- **FR-038**: `rad deployment create` MUST accept an optional `--application` (or `-a` or `--app`) flag that specifies the application name.
- **FR-039**: When `--application <name>` is specified, the system MUST resolve the application definition from `.radius/applications/<name>.bicep`.
- **FR-040**: If `--application` is not specified and exactly one application definition exists in `.radius/applications/`, the system MUST auto-select that application.
- **FR-041**: If `--application` is not specified and multiple application definitions exist in `.radius/applications/`, the system MUST error with a message listing available applications.
- **FR-042**: `rad deployment create` MUST accept an optional `--environment` (or `-e` or `--env`) flag specifying the target environment.
- **FR-043**: When exactly one GitHub Environment exists for the repository, `--environment` MAY be omitted and the system MUST auto-select that environment.
- **FR-044**: When multiple GitHub Environments exist, `--environment` MUST be required. The system MUST error with a message listing available environments if omitted.
- **FR-045**: `rad deployment create` MUST dispatch a GitHub Action workflow that creates a k3d cluster, installs the Radius control plane, and invokes the plan API to generate deployment artifacts.
- **FR-045-A**: `rad deployment create` MUST verify that the local working tree has no uncommitted changes before dispatching the workflow. If uncommitted changes exist, the system MUST error with a message instructing the user to commit all changes first.
- **FR-045-B**: `rad deployment create` MUST verify that all local commits have been pushed to the remote before dispatching the workflow. If unpushed commits exist, the system MUST error with a message instructing the user to push changes first.
- **FR-045-C**: The deployment MUST be scoped to the current HEAD commit hash by default. The commit hash provides traceability between the application definition that was deployed and the resulting artifacts.
- **FR-045-D**: `rad deployment create` MUST accept an optional `--git-commit` flag to scope the deployment to a specific commit hash instead of the current HEAD.
- **FR-046**: The dispatched workflow MUST read environment configuration from GitHub Environment variables (e.g., `AZURE_SUBSCRIPTION_ID`, `AKS_CLUSTER_NAME`, `KUBERNETES_NAMESPACE`, `RADIUS_RECIPES_MANIFEST`).
- **FR-047**: The dispatched workflow MUST commit the generated deployment plan (`deploy.yaml`) and artifact directories to `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/` in the repository.
- **FR-048**: `rad deployment create` MUST only be available in GitHub mode. In Kubernetes mode, the system MUST return an error indicating this command is not available.

#### CLI Command: rad deployment apply (GitHub mode only)

- **FR-049**: `rad deployment apply` MUST accept the same `--application` and `--environment` flags with the same auto-selection rules as `rad deployment create`.
- **FR-050**: `rad deployment apply` MUST resolve the deployment plan as follows: if `--git-commit <hash>` is specified, use `.radius/deploy/<APP>/<ENV>/<hash>/deploy.yaml`; otherwise, use the most recent plan directory under `.radius/deploy/<APP>/<ENV>/`. If no plan exists, the system MUST error with instructions to run `rad deployment create` first.
- **FR-050-A**: `rad deployment apply` MUST accept an optional `--git-commit` flag to target a specific deployment plan by commit hash.
- **FR-051**: `rad deployment apply` MUST dispatch a GitHub Action workflow that executes the deployment plan steps in sequence using the deployment artifacts.
- **FR-052**: The deployment workflow MUST capture deployed cloud resources in their native format and store them in `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/<step>/resources/`.
- **FR-053**: The deployment workflow MUST update the deployment plan status from `planned` to `deployed` (or `failed`) for each step.
- **FR-054**: `rad deployment apply` MUST only be available in GitHub mode. In Kubernetes mode, the system MUST return an error indicating this command is not available.

#### Configuration Data Storage

- **FR-062**: Resource Types manifest MUST be stored as a GitHub repository variable `RADIUS_RESOURCE_TYPES_MANIFEST` set during `rad init`.
- **FR-063**: The `RADIUS_RESOURCE_TYPES_MANIFEST` variable MUST contain the URL to the types.yaml file in the config repository (e.g., `https://github.com/zachcasper/radius-config/types.yaml`).
- **FR-064**: Recipes manifest MUST be stored as a GitHub Environment-scoped variable `RADIUS_RECIPES_MANIFEST` set during `rad environment create`.
- **FR-065**: The `RADIUS_RECIPES_MANIFEST` variable MUST contain the URL to the appropriate recipes manifest file in the config repository based on cloud provider and deployment tool (e.g., `https://github.com/zachcasper/radius-config/recipes-aws-terraform.yaml`).
- **FR-066**: Cloud provider configuration MUST be stored as GitHub Environment-scoped variables set during `rad environment create`.
- **FR-067**: Azure environment variables MUST include: `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP_NAME`, `AKS_CLUSTER_NAME`, `KUBERNETES_NAMESPACE`, `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`.
- **FR-067-A**: AWS environment variables MUST include: `AWS_ACCOUNT_ID`, `AWS_REGION`, `AWS_IAM_ROLE_NAME`, `EKS_CLUSTER_NAME`, `KUBERNETES_NAMESPACE`.
- **FR-067-B**: Environment files (`.radius/env.*.yaml`) MUST NOT be used; all environment configuration is stored in GitHub Environment variables.
- **FR-067-C**: Resource types files (`.radius/types.yaml`) MUST NOT be used; the types manifest reference is stored as a GitHub repository variable.
- **FR-068**: Application Definitions MUST be stored in `.radius/applications/<APP_NAME>.bicep`.
- **FR-068-A**: System MUST provide `rad app model` command that creates a sample application definition file at `.radius/applications/todolist.bicep` with a `Radius.Core/applications` resource, a `Radius.Compute/containers` resource, and a `Radius.Data/postgreSqlDatabases` resource. This is a placeholder for future AI-assisted modeling functionality.
- **FR-069**: Deployment plans and artifacts MUST be stored in `.radius/deploy/<APP_NAME>/<ENVIRONMENT_NAME>/<COMMIT_HASH>/`.
- **FR-070**: Deployment plan file MUST be stored as `.radius/deploy/<APP_NAME>/<ENVIRONMENT_NAME>/<COMMIT_HASH>/deploy.yaml`.
- **FR-071**: Workspaces MUST be stored in `~/.rad/config.yaml` with `current` property (renamed from `default`).
- **FR-072**: GitHub workspaces MUST have `connection.kind: github` and `connection.url` pointing to the repository.

#### Command Behavior Changes

- **FR-073**: In GitHub mode, `rad resource-type` commands MUST operate against the resource types manifest referenced by `RADIUS_RESOURCE_TYPES_MANIFEST`.
- **FR-074**: In GitHub mode, `rad environment` commands (`create`, `delete`, `list`) MUST operate against GitHub Environments via the GitHub API.
- **FR-075**: In GitHub mode, `rad recipe` commands MUST operate against the recipes manifest referenced by the environment's `RADIUS_RECIPES_MANIFEST` variable.
- **FR-076**: Users wanting Kubernetes-based Radius MUST use `rad install kubernetes` (not affected by new `rad init`).
- **FR-077**: Radius on GitHub MUST NOT use Resource Groups; that concept does not apply.

#### Plan File Structure (deploy.yaml)

- **FR-078**: Plan MUST include: application name, applicationDefinitionFile path, environment name.
- **FR-079**: Plan MUST include ordered steps array with: sequence number, resource (name, type, properties), recipe (name, kind, location), deploymentArtifacts path, expectedChanges (add, change, destroy), status.
- **FR-080**: Plan MUST include summary with: totalSteps, terraformSteps, bicepSteps, totalAdd, totalChange, totalDestroy, allVersionsPinned.
- **FR-080-A**: The status field on each step MUST transition through: `planned` → `deployed` → `destroyed`.

#### Deployment Record Structure

- **FR-081**: After `rad deployment apply` completes, the deployment plan (`deploy.yaml`) MUST be updated with execution results: startedAt, completedAt, status.
- **FR-082**: Deployment results MUST include git context: commit, commitShort, branch, isDirty.
- **FR-083**: Deployment results MUST include step-level details: timing, status (deployed/failed), changes applied, outputs, capturedResources.
- **FR-084**: Captured resources MUST be stored in their native format in `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/<step>/resources/`.
- **FR-085**: Deployment results MUST include summary: totalSteps, succeededSteps, failedSteps, skippedSteps, resource counts.

#### GitHub Actions Execution

- **FR-086**: GitHub Action runner MUST be capable of running k3d cluster (approximately 45 seconds startup, ~875 MiB download).
- **FR-087**: k3d cluster MUST map `/github_workspace` in containers to `${GITHUB_WORKSPACE}` for file access.
- **FR-088**: Radius control plane in k3d MUST be configured to use the types manifest from `RADIUS_RESOURCE_TYPES_MANIFEST` and the recipes manifest from the target environment's `RADIUS_RECIPES_MANIFEST` variable.
- **FR-088-A**: Radius installation on k3d MUST use `--skip-contour-install` flag (ingress not needed in ephemeral cluster) and `--set dashboard.enabled=false` (dashboard not needed in CI).
- **FR-089**: GitHub Actions workflow MUST leverage GitHub PR Checks for deployment status reporting.
#### CLI Workflow Dispatch UX

- **FR-089-C**: When a CLI command dispatches a GitHub Action workflow, the CLI MUST display a contextual status message describing the action (e.g., "Creating authentication test workflow...", "Creating deployment workflow...", "Creating deployment...", "Testing authentication to azure...").
- **FR-089-D**: After dispatching, the CLI MUST display an animated progress indicator (e.g., spinner) with a contextual status label (e.g., "Testing authentication to azure...", "Creating deployment...").
- **FR-089-E**: While the animated progress indicator is active, the CLI MUST prompt the user that they can press the `L` key to toggle real-time log streaming from the GitHub Action workflow run.
- **FR-089-F**: When the user presses `L`, the CLI MUST stream the workflow run logs to the terminal in real time (similar to `gh run watch`). Pressing `L` again MUST return to the animated progress indicator view.
- **FR-089-G**: The CLI MUST display the final workflow result (success or failure) with a summary when the workflow completes, regardless of whether log streaming was active.

- **FR-089-A**: All GitHub Actions workflow runs MUST be named using the following convention:
  - Create workflows: `Radius deployment create for <app_name> in <env> environment`
  - Apply workflows: `Radius deployment apply for <app_name> in <env> environment`
  - Destroy workflows: `Radius destroy for <app_name> in <env> environment`
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

#### Terraform State Management

- **FR-093**: Terraform state MUST be stored in a cloud backend (S3 for AWS, Azure Storage for Azure) rather than locally in the repository.
- **FR-094**: The `rad environment create` command MUST configure or create the state backend storage as part of OIDC setup.
- **FR-095**: State backend credentials MUST use the same OIDC authentication configured for deployments.
- **FR-096**: State backend MUST support state locking to prevent concurrent modification conflicts.
- **FR-097**: State backend location MUST be stored as a GitHub Environment-scoped variable.
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
- **FR-105**: Users MAY re-run `rad deployment create` and `rad deployment apply` after fixing issues to resume/retry the deployment.

#### CLI Command: rad app delete (GitHub mode)

- **FR-106**: Resource destruction MUST only destroy resources belonging to the specified application and environment.
- **FR-106-A**: In GitHub mode, `rad app delete` MUST dispatch a GitHub Action workflow that executes destroy operations using the stored deployment artifacts (e.g., `terraform destroy`) and OIDC credentials from the GitHub Environment.
- **FR-106-B**: The destroy workflow MUST update the deployment plan (`deploy.yaml`) step statuses from `deployed` to `destroyed` upon successful destruction.
- **FR-106-C**: The destroy workflow MUST delete captured resource files from `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/<step>/resources/` after successful destruction.
- **FR-106-D**: The CLI MUST display an animated progress indicator and support the `L` key for real-time log streaming, consistent with other workflow-dispatching commands (FR-089-C through FR-089-G).
- **FR-107**: If `--application` is omitted and multiple applications exist, the command MUST error with a message listing available applications.

#### Secret Management

- **FR-108**: Deployment workflows MAY reference GitHub Secrets using the syntax `${{ secrets.SECRET_NAME }}`.
- **FR-109**: The deployment workflow MUST inject referenced GitHub Secrets as environment variables at runtime.
- **FR-110**: Secret references MUST NOT be expanded or logged during plan generation.
- **FR-111**: The `rad init` command SHOULD document the GitHub Secrets reference syntax in generated workflow file comments.

#### Workflow Installation

- **FR-112**: The `rad init --github` command MUST generate `.github/workflows/radius-deployment-create.yml`, `.github/workflows/radius-deployment-apply.yml`, `.github/workflows/radius-destroy.yml`, and `.github/workflows/radius-auth-test.yml` workflow templates that use `rad deployment create`, `rad deployment apply`, `rad app delete`, and OIDC authentication verification respectively.
- **FR-113**: The generated workflows MUST reference GitHub Environment variables for cloud provider configuration and recipe manifests.
- **FR-113-A**: The generated workflow MUST use the `environment:` key in the job definition to scope GitHub Environment variables to the target deployment environment.
- **FR-114**: Generated workflow files MUST be included in the initial commit created by `rad init --github`.

#### Resource Group Defaults

- **FR-115**: The `rad` CLI MUST fall back to the `default` resource group when no `--group` flag is provided and no workspace scope is configured. This supports GitHub-mode workspaces (which do not set a `Scope` property) and ephemeral CI environments (where no `~/.rad/config.yaml` exists). The Radius control plane auto-creates the resource group on first use.

### Key Entities

- **Workspace**: User's working context stored in `~/.rad/config.yaml`; can be of kind `github` (URL-based connection) or `kubernetes` (context-based connection). GitHub workspaces connect to repository URLs; Kubernetes workspaces connect to cluster contexts with scope and environment references.

- **GitHub Environment**: A GitHub Environments API deployment target created via `rad environment create`. Stores cloud provider configuration (subscription IDs, cluster names, namespaces, OIDC credentials) and recipes manifest URL as environment-scoped variables. Corresponds to a deployment target such as "dev", "staging", or "production".

- **Config Repository**: External repository (e.g., `zachcasper/radius-config`) hosting resource types manifest (`types.yaml`) and recipes manifests (`recipes-aws-terraform.yaml`, `recipes-azure-bicep.yaml`, `recipes-azure-terraform.yaml`). Referenced via URLs stored in GitHub repository and environment variables.

- **Resource Type**: Definition of infrastructure resource schemas stored externally in the config repository or `radius-project/resource-types-contrib` repository. Referenced via the `RADIUS_RESOURCE_TYPES_MANIFEST` repository variable. Includes types like `Radius.Core/applications`, `Radius.Compute/containers`, `Radius.Data/postgreSqlDatabases`, etc.

- **Recipe**: Implementation template for provisioning resources. Referenced via the `RADIUS_RECIPES_MANIFEST` environment variable. Default deployment tools: Terraform for AWS, Bicep for Azure. Organized by provider and deployment tool in the config repository.

- **Application Definition**: Bicep-based declaration of application resources and their relationships; stored in `.radius/applications/<APP_NAME>.bicep`. Uses Radius resource types with environment parameter. Unchanged from current Radius application definition format.

- **Deployment Plan**: Ordered sequence of resource provisioning steps generated by the Radius control plane's plan API (invoked by `rad deployment create` workflow inside k3d); stored in `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/deploy.yaml`. Contains step ordering, expected changes, and references to deployment artifact directories with Terraform/Bicep configurations.

- **Deployment Artifacts**: Generated Terraform configurations (main.tf, providers.tf, variables.tf, terraform.tfvars.json) and plan outputs for each deployment step; stored in `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/<step>/artifacts/`. Created by `rad deployment create`; executed by `rad deployment apply`.

- **Deployed Resources**: Captured cloud resources in their native format after `rad deployment apply` completes; stored in `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/<step>/resources/`. Includes Kubernetes YAML manifests, AWS JSON descriptions, and Azure JSON definitions.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a GitHub repository for Radius in under 2 minutes using `rad init --github`.
- **SC-002**: Users can create a deployment environment with cloud provider OIDC setup (AWS or Azure) in under 10 minutes with guided prompts using `rad environment create`.
- **SC-003**: Users can create a deployment plan using `rad deployment create` and execute it using `rad deployment apply` with single commands each.
- **SC-004**: 95% of deployment attempts complete within 15 minutes for applications with 5 or fewer resources.
- **SC-005**: GitHub Action setup (k3d + Radius) completes in under 60 seconds with approximately 875 MiB download.
- **SC-006**: Failed deployments provide actionable error messages that identify the failing resource, step, and root cause.
- **SC-007**: Deployment plans and artifacts are committed to the repository, providing complete audit trail with expected changes, Terraform/Bicep configurations, and after apply, captured resource definitions.
- **SC-008**: Users can delete an environment and its associated resources using `rad environment delete`.
- **SC-009**: 90% of users can complete their first deployment without external documentation beyond CLI help text.
- **SC-010**: Workspace switching between GitHub and Kubernetes modes works seamlessly with appropriate command behavior.

## Assumptions

- Users have the GitHub CLI (`gh`) installed and authenticated on their workstation.
- Users have cloud provider CLIs (`aws` or `az`) installed and authenticated for OIDC setup.
- The GitHub repository has GitHub Actions enabled with sufficient minutes quota for workflow execution.
- A config repository (e.g., `zachcasper/radius-config`) hosts resource types and recipes manifests with stable URLs.
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
- Radius follows a two-phase deployment model (`rad deployment create` + `rad deployment apply`), not a GitOps model.

## Dependencies

- GitHub CLI (`gh`) must be available on user workstations for initialization, environment management, and deployment.
- GitHub Actions must be available and enabled for the repository.
- GitHub Environments API must be available for storing environment-scoped configuration.
- Config repository (e.g., `zachcasper/radius-config`) must host resource types and recipes manifests.
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

- **Two-phase deployment**: `rad deployment create` generates a deployment plan and artifacts; `rad deployment apply` executes the plan. This separation enables review, auditability, and incremental adoption.
- **Ephemeral control plane**: Runs on a k3d cluster in a GitHub Action when triggered by `rad deployment create` or `rad deployment apply`.
- **GitHub API storage**: Environment configuration stored as GitHub Environment-scoped variables; resource types manifest referenced via GitHub repository variable.
- **CLI consistency**: Uses the existing `rad` CLI with new commands (`rad init --github`, `rad environment create`, `rad deployment create`, `rad deployment apply`).
- **Git-native artifacts**: Application definitions (`.radius/applications/`), deployment plans and artifacts (`.radius/deploy/`) stored in the repository.
- **No persistent state**: No data is persistent in the Radius control plane; state is in GitHub (variables, environments) and the repository (definitions, artifacts).

---

## Appendix B: Configuration Data Model Examples

### B.1 GitHub Repository Variable

Set by `rad init --github`:

| Variable | Value | Scope |
|----------|-------|-------|
| `RADIUS_RESOURCE_TYPES_MANIFEST` | `https://github.com/zachcasper/radius-config/types.yaml` | Repository |

### B.2 GitHub Environment Variables (Azure)

Set by `rad environment create dev --provider azure` for an Azure environment:

| Variable | Value | Scope |
|----------|-------|-------|
| `AZURE_SUBSCRIPTION_ID` | `12345678-1234-1234-1234-123456789012` | Environment: dev |
| `AZURE_RESOURCE_GROUP_NAME` | `rg-radius-dev` | Environment: dev |
| `AKS_CLUSTER_NAME` | `aks-dev-cluster` | Environment: dev |
| `KUBERNETES_NAMESPACE` | `default` | Environment: dev |
| `AZURE_TENANT_ID` | `87654321-4321-4321-4321-210987654321` | Environment: dev |
| `AZURE_CLIENT_ID` | `abcdefgh-abcd-abcd-abcd-abcdefghijkl` | Environment: dev |
| `RADIUS_RECIPES_MANIFEST` | `https://github.com/zachcasper/radius-config/recipes-azure-bicep.yaml` | Environment: dev |

### B.3 GitHub Environment Variables (AWS)

Set by `rad environment create staging --provider aws` for an AWS environment:

| Variable | Value | Scope |
|----------|-------|-------|
| `AWS_ACCOUNT_ID` | `123456789012` | Environment: staging |
| `AWS_REGION` | `us-east-1` | Environment: staging |
| `AWS_IAM_ROLE_NAME` | `radius-github-oidc` | Environment: staging |
| `EKS_CLUSTER_NAME` | `eks-staging-cluster` | Environment: staging |
| `KUBERNETES_NAMESPACE` | `default` | Environment: staging |
| `RADIUS_RECIPES_MANIFEST` | `https://github.com/zachcasper/radius-config/recipes-aws-terraform.yaml` | Environment: staging |
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

---

## Appendix C: Deployment Structure

### C.1 Deployment Plan File (.radius/deploy/todolist/dev/abc1234/deploy.yaml)

```yaml
# Radius deployment plan
# Generated by Radius control plane
# Generated: 2026-02-05T21:35:58Z

application: todolist
applicationDefinitionFile: .radius/applications/todolist.bicep
environment: dev
commit: abc1234
steps:
  - sequence: 1
    resource:
      name: db
      type: Radius.Data/postgreSqlDatabases
      properties:
        application: todolist.id
        environment: environment
        size: S
    recipe:
      name: Radius.Data/postgreSqlDatabases
      kind: terraform
      location: git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/recipes/kubernetes/terraform?ref=v0.54.0
    deploymentArtifacts: .radius/deploy/todolist/dev/abc1234/001-db-terraform
    expectedChanges:
      add: 3
      change: 0
      destroy: 0
    status: planned
  - sequence: 2
    resource:
      name: frontend
      type: Radius.Compute/containers
      properties:
        application: todolist.id
        connections:
          postgresql:
            source: db.id
        container:
          image: ghcr.io/radius-project/samples/demo:latest
          ports:
            web:
              containerPort: 3000
        environment: environment
    recipe:
      name: Radius.Compute/containers
      kind: terraform
      location: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/kubernetes/terraform?ref=v0.54.0
    deploymentArtifacts: .radius/deploy/todolist/dev/abc1234/002-frontend-terraform
    expectedChanges:
      add: 2
      change: 0
      destroy: 0
    status: planned
summary:
  totalSteps: 2
  terraformSteps: 2
  bicepSteps: 0
  totalAdd: 5
  totalChange: 0
  totalDestroy: 0
  allVersionsPinned: false
```

The status field is planned → deployed → destroyed.

### C.2 Deployment Artifact Files

Each deployment step directory (e.g., `.radius/deploy/todolist/dev/abc1234/001-db-terraform/artifacts`) contains:

**main.tf:**

```hcl
# Radius resource deployment plan
# Generated by Radius control plane
# Resource: db (Radius.Data/postgreSqlDatabases)
# Generated: 2026-02-05T21:35:58Z

module "db" {
  source = "git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/recipes/kubernetes/terraform"

  # Pass the Radius context to the recipe module
  context = var.context
}

# Outputs from the recipe module
output "result" {
  description = "Result output from the recipe module"
  value       = try(module.db.result, null)
  sensitive   = true
}
```

**providers.tf:**
```hcl
# Radius resource deployment plan
# Generated by Radius control plane
# Resource: db (Radius.Data/postgreSqlDatabases)
# Generated: 2026-02-05T21:35:58Z

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
  }
}

provider "kubernetes" {
  config_path    = "~/.kube/config"
  config_context = "rad-kind"
}
```

**variables.tf:**
```hcl
# Radius resource deployment plan
# Generated by Radius control plane
# Variables for resource: db (Radius.Data/postgreSqlDatabases)
# Generated: 2026-02-05T21:36:10Z

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
```

**terraform.tfvars.json:**
```json
{
  "context": {
    "application": {
      "name": "todolist"
    },
    "aws": null,
    "azure": null,
    "environment": {
      "name": "default"
    },
    "resource": {
      "connections": {},
      "name": "db",
      "properties": {
        "application": "todolist.id",
        "environment": "environment",
        "size": "S"
      },
      "type": "Radius.Data/postgreSqlDatabases"
    },
    "runtime": {
      "kubernetes": {
        "environmentNamespace": "default",
        "namespace": "default"
      }
    }
  }
}
```

**terraform-context.txt:**
```
# Terraform Context
# Generated by Radius control plane
# Generated: 2026-02-05T21:35:58Z

## Terraform Version

terraform_version: 1.5.7

## Environment Variables

TF_CLI_CONFIG_FILE: (not set)
TF_BACKEND_CONFIG: (not set)

## Provider Installation

(no provider_installation block found in terraformrc)

## Resource Context

resource_name: db
resource_type: Radius.Data/postgreSqlDatabases
recipe_location: git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/recipes/kubernetes/terraform
application: todolist
environment: default

## Plan Results

plan_changed: true
```

**tf-plan.txt:** Contains the text output from `terraform plan` showing resources to be created/modified/destroyed.

**bicep-what-if.txt:** Contains the text output from `az deployent --what-if` showing resources to be created/modified/destroyed.

### C.3 Deployed Cloud Resources  

After deployment, each cloud resource is captured in it's native format (AWS resources in JSON, Azure resources in JSON, Kubernetes resources in YAML) and stored in `.radius/deploy/todolist/dev/abc1234/001-db-terraform/resources`).

- **Kubernetes resources**: YAML manifests (e.g., `deployment-db.yaml`, `service-db.yaml`)
- **AWS resources**: JSON resource descriptions
- **Azure resources**: JSON resource definitions

### C.4 Deletion Record

Same structure as deployment record, but with `status` reflecting destruction and `changes` showing resources destroyed rather than added. After an application is deleted, the resources in `.radius/deploy/todolist/dev/abc1234/001-db-terraform/resources` are deleted.
