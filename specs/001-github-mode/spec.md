# Feature Specification: Radius on GitHub

**Feature Branch**: `001-github-mode`  
**Created**: 2026-02-12  
**Status**: Draft  
**Input**: User description: "Radius on GitHub - a new mode of operation where all data and processing runs in a GitHub repository, complementing the existing Radius on Kubernetes mode"

## Clarifications

### Session 2026-02-12

- Q: Where should Terraform state be stored for deployments executed in GitHub Actions? → A: Cloud backend (S3 for AWS, Azure Storage for Azure) with OIDC authentication; future enhancement to allow custom existing backends.
- Q: How should concurrent deployments to the same environment be handled? → A: Lock-based rejection - if a deployment is in progress, subsequent attempts fail immediately with an error prompting the user to try again later.
- Q: What happens when a deployment partially fails? → A: Leave successfully deployed resources in place; report which resources failed; user decides whether to fix and re-deploy or destroy.
- Q: What scope should `rad pr destroy` target when multiple applications exist in an environment? → A: Single application only; require `--application` flag to specify which application to destroy.
- Q: How should sensitive values (API keys, connection strings) be handled in configuration? → A: Reference GitHub Secrets by name in configuration; workflow injects values at runtime.

## Overview

Radius on GitHub is a new operational mode that enables users to deploy cloud applications using Radius without requiring a centralized Kubernetes-based control plane. All configuration, plans, and deployment records are stored directly in the GitHub repository, leveraging GitHub Actions for execution and GitHub Pull Requests for review and approval workflows.

This mode complements the existing "Radius on Kubernetes" mode, giving users the choice of how they want to operate Radius based on their infrastructure preferences.

### Key Characteristics

- **Git-native storage**: All configuration, plans, and deployment records stored in the repository
- **GitHub Actions execution**: Deployments run in ephemeral k3d clusters within GitHub Action runners
- **PR-based workflows**: Deployment plans reviewed and approved through standard GitHub Pull Requests
- **OIDC authentication**: Secure, credential-free authentication to AWS and Azure via OIDC federation
- **CLI-driven**: Familiar `rad` CLI commands inspired by `git` and `gh` patterns

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize Repository for Radius (Priority: P1)

A developer wants to use Radius with their existing GitHub repository containing application source code. They run `rad init` to set up the repository with Radius configuration files, enabling GitHub-based deployments without needing to install or manage a Kubernetes control plane.

**Why this priority**: This is the entry point for all Radius on GitHub functionality. Without initialization, no other features can be used. It establishes the foundational configuration structure that all subsequent operations depend on.

**Independent Test**: Can be fully tested by running `rad init` on a fresh GitHub repository clone and verifying all configuration files are created, the workspace is registered, and changes are committed.

**Acceptance Scenarios**:

1. **Given** a cloned GitHub repository without Radius configuration, **When** the user runs `rad init --provider aws --deployment-tool terraform`, **Then** the system creates `.radius/types.yaml`, `.radius/recipes.yaml`, and `.radius/env.default.yaml` files with AWS/Terraform configuration, updates `~/.rad/config.yaml` with a new `github` kind workspace, and commits the changes with trailer `Radius-Action: init`.

2. **Given** a cloned GitHub repository without Radius configuration, **When** the user runs `rad init --provider azure --deployment-tool bicep --environment production`, **Then** the system creates configuration files with Azure/Bicep configuration, the environment is named "production", and the workspace is registered.

3. **Given** a cloned GitHub repository without Radius configuration, **When** the user runs `rad init --provider aws --deployment-tool terraform`, **Then** `.radius/types.yaml` is populated with Resource Types fetched from the `radius-project/resource-types-contrib` repository using git sparse-checkout.

4. **Given** a directory that is not a Git repository, **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to initialize a Git repository first or clone from GitHub.

5. **Given** a Git repository without a GitHub remote (origin), **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to add a GitHub remote or use `gh repo create`.

6. **Given** the user is not authenticated with the GitHub CLI, **When** the user runs `rad init`, **Then** the system verifies `gh auth status` and displays an error message instructing the user to authenticate using `gh auth login`.

7. **Given** successful initialization, **When** the command completes, **Then** the changes are staged with `git add` and committed with a message containing the trailer `Radius-Action: init`.

---

### User Story 2 - Connect Cloud Provider Authentication (Priority: P1)

After initializing the repository, a developer needs to establish secure authentication between GitHub Actions and their cloud provider (AWS or Azure). They run `rad environment connect` which guides them through setting up OIDC-based authentication, eliminating the need to store long-lived credentials.

**Why this priority**: Deployments cannot occur without cloud authentication. This is a critical prerequisite for all deployment operations. OIDC provides secure, credential-free authentication that is essential for production use.

**Independent Test**: Can be fully tested by running `rad environment connect` after initialization and verifying OIDC configuration is created in the cloud provider (or existing role is validated) and the environment file is updated with the necessary identifiers.

**Acceptance Scenarios**:

1. **Given** an AWS environment without OIDC configured, **When** the user runs `rad environment connect`, **Then** the system prompts for AWS account ID (defaulting to `aws sts get-caller-identity`), region (defaulting to `aws configure get region`), and offers to create a new IAM role or use existing ARN.

2. **Given** the user chooses to create a new AWS IAM role, **When** the user confirms, **Then** the system verifies AWS CLI authentication via `aws sts get-caller-identity`, summarizes the commands to execute, and creates the IAM role with OIDC trust policy for GitHub Actions.

3. **Given** successful AWS OIDC configuration, **When** the command completes, **Then** the environment file `.radius/env.<ENV>.yaml` is updated with `accountId`, `region`, and `oidcRoleARN`, and changes are committed with trailer `Radius-Action: environment-connect`.

4. **Given** an Azure environment without OIDC configured, **When** the user runs `rad environment connect`, **Then** the system prompts for subscription (from `az account list`), resource group name, and offers to create a new Azure AD application with federated credentials or use existing.

5. **Given** the user chooses to create a new Azure AD application, **When** the user confirms, **Then** the system verifies Azure CLI authentication via `az account show`, summarizes the commands, and creates the AD application with federated credential linking the GitHub repository/workflow.

6. **Given** successful Azure OIDC configuration, **When** the command completes, **Then** the environment file is updated with `subscriptionId`, `tenantId`, `clientId`, `resourceGroupName`, and `oidcEnabled: true`.

7. **Given** AWS CLI is not installed or authenticated, **When** the user chooses to create a new IAM role, **Then** the system displays an error with instructions to install/authenticate AWS CLI.

8. **Given** an environment that already has OIDC configured, **When** the user runs `rad environment connect`, **Then** the system offers to update or replace existing configuration.

9. **Given** the current directory is not a GitHub workspace, **When** the user runs `rad environment connect`, **Then** the system displays an error message indicating the command requires a GitHub workspace.

---

### User Story 3 - Create Deployment Plan via Pull Request (Priority: P2)

A developer has modeled their application and wants to deploy it to an environment. They run `rad pr create` which triggers a GitHub Action that generates a detailed deployment plan and creates a Pull Request for review.

**Why this priority**: This enables the core value proposition of reviewable, auditable deployments through the familiar PR workflow. It depends on P1 stories being complete but delivers the primary deployment planning capability.

**Independent Test**: Can be fully tested by having an application model in place and running `rad pr create`, then verifying a PR is created with plan files that can be reviewed.

**Acceptance Scenarios**:

1. **Given** a valid application model in `.radius/model/`, **When** the user runs `rad pr create --environment dev`, **Then** the CLI triggers a GitHub Action workflow that creates a deployment branch named `deploy/<app>/<env>-<timestamp>`.

2. **Given** the GitHub Action is running, **When** the workflow executes, **Then** it installs k3d, Radius CLI, and kubectl, creates a k3d cluster with hostPath volume mapping, and installs the Radius control plane using the repository's configuration files.

3. **Given** the Radius control plane is running in the GitHub Action, **When** `rad plan deploy` executes, **Then** it generates `plan.yaml` with ordered deployment steps and creates deployment artifact directories for each resource (containing main.tf, providers.tf, variables.tf, terraform.tfvars.json, tfplan.txt, terraform-context.txt, .terraform.lock.hcl).

4. **Given** a deployment plan is generated, **When** the GitHub Action completes, **Then** a Pull Request is created with the `plan.yaml` and all deployment artifacts in `.radius/plan/<app>/<env>/` for review.

5. **Given** multiple applications exist in the repository, **When** the user runs `rad pr create --environment dev` without specifying an application, **Then** plans are generated for all applications in `.radius/model/`.

6. **Given** the user specifies a specific application, **When** the user runs `rad pr create --environment dev --application frontend`, **Then** only that application's plan is generated.

7. **Given** the plan generation encounters an error, **When** the GitHub Action fails, **Then** error details are captured and displayed in the workflow logs and the PR (if created) includes error information.

---

### User Story 4 - Deploy Application via PR Merge (Priority: P2)

After reviewing the deployment plan in a Pull Request, a developer merges it to trigger the actual deployment. They can use `rad pr merge` to initiate this process.

**Why this priority**: This completes the deployment workflow, delivering the core value of actually provisioning resources in the cloud.

**Independent Test**: Can be fully tested by having a deployment PR and running `rad pr merge`, then verifying resources are provisioned in the target environment and deployment records are created.

**Acceptance Scenarios**:

1. **Given** a deployment PR exists, **When** the user runs `rad pr merge`, **Then** the PR is merged and a GitHub Action workflow is triggered to execute the deployment.

2. **Given** the deployment workflow is running, **When** `rad deploy` executes with the plan.yaml, **Then** it provisions resources using the pre-generated Terraform artifacts in sequence order.

3. **Given** deployment succeeds, **When** execution completes, **Then** deployment records are stored in `.radius/deploy/<app>/<env>/<commit>/deploy-<commit>.json`, resource definitions are captured (YAML for Kubernetes, JSON for cloud), the PR is merged, and the branch is deleted.

4. **Given** deployment fails, **When** execution completes with errors, **Then** error logs are captured, the PR is updated with failure details and deployment logs, and the branch is preserved for investigation.

5. **Given** no deployment PR exists, **When** the user runs `rad pr merge`, **Then** an appropriate error message is displayed indicating no PR is available to merge.

6. **Given** the user wants to skip review, **When** the user runs `rad pr merge --yes`, **Then** the PR is merged automatically without manual review confirmation.

7. **Given** a specific PR number, **When** the user runs `rad pr merge --pr 42`, **Then** that specific PR is merged instead of the latest CLI-created PR.

---

### User Story 5 - Destroy Application Resources (Priority: P3)

A developer needs to tear down deployed resources for an application. They run `rad pr destroy` to create a destruction plan PR, then merge it to execute the teardown.

**Why this priority**: Resource cleanup is important but secondary to initial deployment capability. It completes the full lifecycle management.

**Independent Test**: Can be fully tested by having deployed resources and running `rad pr destroy`, then verifying a destruction PR is created and resources are removed after merge.

**Acceptance Scenarios**:

1. **Given** an application is deployed, **When** the user runs `rad pr destroy --environment dev --application frontend`, **Then** a destruction plan is generated by `rad plan destroy` and a PR is created on branch `destroy/frontend/dev-<timestamp>`.

2. **Given** a destruction PR exists, **When** the user runs `rad pr merge`, **Then** `rad destroy` executes using the plan, resources are destroyed, and destruction records are stored in `.radius/deploy/<app>/<env>/<commit>/destroy-<commit>.json`.

3. **Given** multiple applications are deployed, **When** the user runs `rad pr destroy --environment dev` without specifying `--application`, **Then** the command fails with an error prompting the user to specify which application to destroy.

4. **Given** a specific application to destroy, **When** the user runs `rad pr destroy --environment dev --application frontend`, **Then** only that application's destruction plan is generated.

5. **Given** a specific deployment commit, **When** the user runs `rad pr destroy --environment dev --application frontend --commit abc123`, **Then** the destruction targets that specific deployment version.

6. **Given** the user wants to skip review, **When** the user runs `rad pr destroy --environment dev --application frontend --yes`, **Then** destruction proceeds automatically after plan generation.

7. **Given** destruction fails, **When** execution completes with errors, **Then** error logs are captured, the PR is updated with failure details, and the branch is preserved.

---

### User Story 6 - Manage Workspaces Across Repositories (Priority: P3)

A developer works with multiple repositories, some using Radius on GitHub and others using Radius on Kubernetes. The workspace configuration allows seamless switching between different contexts.

**Why this priority**: Multi-repository support enhances user experience but is not essential for single-repo usage.

**Independent Test**: Can be fully tested by configuring multiple workspaces in `~/.rad/config.yaml` and switching between them while verifying command behavior changes appropriately.

**Acceptance Scenarios**:

1. **Given** multiple workspaces are configured (some `github` kind, some `kubernetes` kind), **When** the user switches workspaces, **Then** subsequent commands use the selected workspace's configuration and connection type.

2. **Given** a GitHub workspace is current, **When** the user runs `rad resource-type` commands, **Then** a warning indicates Resource Types are managed via `.radius/types.yaml` when using a GitHub workspace.

3. **Given** a GitHub workspace is current, **When** the user runs `rad environment` commands, **Then** a warning indicates Environments are defined in `.radius/env.<NAME>.yaml`.

4. **Given** a GitHub workspace is current, **When** the user runs `rad recipe` commands, **Then** a warning indicates Recipes are defined in `.radius/recipes.yaml` or the file referenced in the environment.

5. **Given** a Kubernetes workspace is current, **When** the user runs commands, **Then** they operate against the Kubernetes control plane as before with no warnings.

6. **Given** the user wants to use Kubernetes-based Radius, **When** they run `rad install kubernetes`, **Then** the traditional Kubernetes control plane is installed (the new `rad init` does not replace this path).

---

### User Story 7 - View and Understand Deployment Plans (Priority: P3)

A developer or reviewer needs to understand what changes a deployment will make before approving the PR. The deployment plan should be clear and auditable.

**Why this priority**: Auditability is important for production deployments but builds on the core deployment functionality.

**Independent Test**: Can be fully tested by creating a deployment PR and reviewing the plan files to verify they clearly document expected changes.

**Acceptance Scenarios**:

1. **Given** a deployment PR is created, **When** a reviewer opens the PR, **Then** they can see `plan.yaml` with application name, environment, and ordered deployment steps.

2. **Given** a deployment step in the plan, **When** the reviewer examines the step, **Then** they can see the resource name, type, properties, recipe information, deployment artifacts path, and expected changes (add/change/destroy counts).

3. **Given** a deployment artifact directory, **When** the reviewer examines the files, **Then** they can see the Terraform configuration (main.tf), providers (providers.tf), variables (variables.tf, terraform.tfvars.json), and the plan output (tfplan.txt).

4. **Given** the plan summary, **When** the reviewer examines it, **Then** they can see total steps, Terraform vs Bicep steps, total resources to add/change/destroy, and whether all versions are pinned.

---

### Edge Cases

- What happens when the GitHub repository has no Actions enabled? System displays an error with instructions to enable GitHub Actions in repository settings.
- What happens when OIDC role permissions are insufficient? Deployment fails with clear error message indicating missing permissions and which permissions are needed.
- What happens when a deployment is in progress and another is requested? The system rejects with error if same app/environment has an active deployment; allows parallel deployments to different environments.
- What happens when the plan file format is corrupted or manually edited incorrectly? Deployment validation fails with specific parsing errors before execution begins.
- What happens when network connectivity to GitHub is lost during deployment? The GitHub Action retries or fails gracefully with state preserved in Terraform state.
- What happens when the resource-types-contrib repository is unavailable during `rad init`? Error with clear message and retry instructions.
- What happens when the user runs commands meant for Kubernetes mode in a GitHub workspace? Commands display warning that the functionality is managed through repository files.
- What happens when Terraform state conflicts occur? Deployment fails with instructions to resolve state conflicts manually.
- What happens when the k3d cluster fails to start in GitHub Actions? Workflow fails with diagnostic information about resource constraints or configuration issues.

## Requirements *(mandatory)*

### Functional Requirements

#### CLI Command: rad init

- **FR-001**: System MUST replace the current `rad init` command with new functionality that initializes a GitHub repository for Radius on GitHub mode.
- **FR-002**: System MUST require `--provider` flag with values `aws` or `azure` as a required parameter.
- **FR-003**: System MUST require `--deployment-tool` flag with values `terraform` or `bicep` as a required parameter.
- **FR-004**: System MUST accept optional `--environment` (or `-e` or `--env`) flag; defaulting to "default" if omitted.
- **FR-005**: System MUST validate the current directory is a Git repository by checking for `.git` directory.
- **FR-006**: System MUST validate the Git repository has a GitHub remote (origin) by parsing the remote URL.
- **FR-007**: System MUST verify GitHub CLI (`gh`) is authenticated by running `gh auth status`.
- **FR-008**: System MUST create `.radius/types.yaml` populated with Resource Type definitions fetched from `radius-project/resource-types-contrib` repository using git sparse-checkout to clone/fetch only the .yaml files in non-hidden directories.
- **FR-009**: System MUST create `.radius/recipes.yaml` with recipes appropriate for the selected provider (aws/azure) and deployment tool (terraform/bicep).
- **FR-009-A**: System MUST include a recipe entry in `.radius/recipes.yaml` for every resource type defined in `.radius/types.yaml` that has a recipes directory in the repository (e.g., types like `Radius.Core/applications` that are metadata-only do not have recipes). Recipe locations MUST point to the same `radius-project/resource-types-contrib` repository from which types were fetched. For example, if the containers resource type is defined at `https://github.com/radius-project/resource-types-contrib/blob/main/Compute/containers/containers.yaml`, the associated Kubernetes/Terraform recipe location would be `git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/kubernetes/terraform`. Recipe paths follow the pattern: `<Namespace>/<TypeName>/recipes/<target>/<tool>/` where target is `kubernetes`, `aws`, or `azure` and tool is `terraform` (Bicep recipes are a future enhancement). Version tags are not included in recipe locations (versioning is a future enhancement).
- **FR-010**: System MUST create `.radius/env.<ENVIRONMENT_NAME>.yaml` with placeholder structure for OIDC configuration.
- **FR-011**: System MUST update or create `~/.rad/config.yaml` with a new workspace of kind `github` using the repository URL as connection; workspace name MUST match the repository name.
- **FR-012**: System MUST rename the workspace config property `default` to `current` for clarity.
- **FR-013**: System MUST commit changes with `git add` and `git commit` including the trailer `Radius-Action: init`.
- **FR-014**: System MUST NOT be interactive; all configuration comes from command-line flags.

#### CLI Command: rad environment connect

- **FR-015**: System MUST provide `rad environment connect` command for configuring cloud OIDC authentication.
- **FR-016**: System MUST accept optional `--environment` (or `-e` or `--env`) flag; defaulting to current workspace environment.
- **FR-017**: System MUST validate the current directory is a GitHub workspace before proceeding.
- **FR-018**: For AWS environments, system MUST prompt for account ID (defaulting to `aws sts get-caller-identity --query Account --output text`).
- **FR-019**: For AWS environments, system MUST prompt for region (defaulting to `aws configure get region`).
- **FR-020**: For AWS environments, system MUST prompt whether to use existing IAM Role ARN or create a new role.
- **FR-021**: For AWS new role creation, system MUST verify AWS CLI authentication via `aws sts get-caller-identity`.
- **FR-022**: For AWS new role creation, system MUST summarize commands to execute and request user confirmation.
- **FR-023**: For AWS environments, system MUST write `accountId`, `region`, and `oidcRoleARN` to `.radius/env.<ENV>.yaml`.
- **FR-024**: For Azure environments, system MUST prompt for subscription (from `az account list`).
- **FR-025**: For Azure environments, system MUST prompt for resource group name.
- **FR-026**: For Azure environments, system MUST prompt whether to use existing Azure AD application or create a new one.
- **FR-027**: For Azure new application creation, system MUST verify Azure CLI authentication via `az account show`.
- **FR-028**: For Azure new application creation, system MUST summarize commands and request user confirmation to create federated credential.
- **FR-029**: For Azure environments, system MUST write `subscriptionId`, `tenantId`, `clientId`, `resourceGroupName`, and `oidcEnabled: true` to environment file.
- **FR-030**: System MUST NOT store cloud secret keys locally; only OIDC-related identifiers are stored.
- **FR-031**: System MUST commit environment changes with trailer `Radius-Action: environment-connect`.

#### CLI Command: rad pr create

- **FR-032**: System MUST provide `rad pr create` command that triggers a remote GitHub Action workflow.
- **FR-033**: System MUST require `--environment` (or `-e` or `--env`) flag as required parameter.
- **FR-034**: System MUST accept optional `--application` (or `-a` or `--app`) flag; if omitted, plan for all applications.
- **FR-035**: The triggered GitHub Action MUST create a deployment branch named `deploy/<APPLICATION_NAME>/<ENVIRONMENT_NAME>-<timestamp>`.
- **FR-036**: The GitHub Action MUST install k3d, Radius CLI, and kubectl on the runner.
- **FR-037**: The GitHub Action MUST create a k3d cluster with hostPath volume mapping `/github_workspace` to `${GITHUB_WORKSPACE}`.
- **FR-038**: The GitHub Action MUST install Radius control plane configured to use Resource Types, Recipes, and Environments from the repository.
- **FR-039**: The GitHub Action MUST execute `rad plan deploy` for each application (or specified application).
- **FR-040**: The `rad plan deploy` command MUST generate `plan.yaml` with ordered deployment steps.
- **FR-041**: For each deployment step, system MUST generate deployment artifacts in directory `XXX-<RESOURCE_NAME>-<RECIPE_KIND>`.
- **FR-042**: Deployment artifacts MUST include: main.tf, providers.tf, variables.tf, terraform.tfvars.json, tfplan.txt, terraform-context.txt, .terraform.lock.hcl.
- **FR-043**: System MUST create a GitHub Pull Request containing all plan and artifact files.

#### CLI Command: rad pr merge

- **FR-044**: System MUST provide `rad pr merge` command to merge deployment PRs.
- **FR-045**: System MUST accept optional `--pr` flag to specify PR number; if omitted, merge the latest CLI-created PR.
- **FR-046**: System MUST accept optional `--yes` flag to merge without review confirmation.
- **FR-047**: On PR merge, a GitHub Action MUST execute deployments using the plan and artifacts.
- **FR-048**: The `rad deploy` command MUST execute deployment steps in sequence using pre-generated Terraform artifacts.
- **FR-049**: System MUST store deployment records in `.radius/deploy/<app>/<env>/<commit>/deploy-<commit>.json`.
- **FR-050**: System MUST capture deployed resource definitions in platform-native format (YAML for Kubernetes, JSON for cloud).
- **FR-051**: On successful deployment, system MUST merge the PR and delete the deployment branch.
- **FR-052**: On failed deployment, system MUST update the PR with deployment logs and error outputs without merging.

#### CLI Command: rad pr destroy

- **FR-053**: System MUST provide `rad pr destroy` command that generates destruction plans.
- **FR-054**: System MUST require `--environment` (or `-e` or `--env`) flag as required parameter.
- **FR-055**: System MUST require `--application` (or `-a` or `--app`) flag specifying the single application to destroy.
- **FR-056**: System MUST accept optional `--commit` flag to target specific deployment commit; defaults to latest deployment.
- **FR-057**: System MUST accept optional `--yes` flag to merge destruction PR without review.
- **FR-058**: Destruction branch MUST be named `destroy/<APPLICATION_NAME>/<ENVIRONMENT_NAME>-<timestamp>`.
- **FR-059**: The `rad plan destroy` command MUST generate destruction plan.yaml.
- **FR-060**: On destruction PR merge, `rad destroy` MUST execute destruction using the plan.
- **FR-061**: System MUST store destruction records in `.radius/deploy/<app>/<env>/<commit>/destroy-<commit>.json`.

#### Configuration Data Storage

- **FR-062**: Resource Types MUST be stored in `.radius/types.yaml` referencing external definitions via `definitionLocation` with git URL format.
- **FR-063**: There MUST be exactly one `.radius/types.yaml` file per repository.
- **FR-064**: Recipes MUST be stored in `.radius/recipes.yaml` or files referenced from environment `recipes` property.
- **FR-065**: Recipe entries MUST include `recipeKind` (terraform or bicep) and `recipeLocation` (git URL or OCI registry URL).
- **FR-066**: Environments MUST be stored in `.radius/env.<NAME>.yaml` files.
- **FR-067**: Environment files MUST include: name, kind (aws or azure), recipes reference, optional recipeParameters, and provider configuration.
- **FR-068**: Application Models MUST be stored in `.radius/model/<APP_NAME>.bicep`.
- **FR-069**: Plans MUST be stored in `.radius/plan/<APP_NAME>/<ENVIRONMENT_NAME>/`.
- **FR-070**: Deployments MUST be stored in `.radius/deploy/<APP_NAME>/<ENVIRONMENT_NAME>/<COMMIT>/`.
- **FR-071**: Workspaces MUST be stored in `~/.rad/config.yaml` with `current` property (renamed from `default`).
- **FR-072**: GitHub workspaces MUST have `connection.kind: github` and `connection.url` pointing to the repository.

#### Command Behavior Changes

- **FR-073**: `rad resource-type` commands MUST display a warning when current workspace kind is `github`.
- **FR-074**: `rad environment` commands MUST display a warning when current workspace kind is `github`.
- **FR-075**: `rad recipe` and `rad recipe-pack` commands MUST display a warning when current workspace kind is `github`.
- **FR-076**: Users wanting Kubernetes-based Radius MUST use `rad install kubernetes` (not affected by new `rad init`).
- **FR-077**: Radius on GitHub MUST NOT use Resource Groups; that concept does not apply.

#### Plan File Structure (plan.yaml)

- **FR-078**: Plan MUST include: application name, applicationModelFile path, environment name.
- **FR-079**: Plan MUST include ordered steps array with: sequence number, resource (name, type, properties), recipe (name, kind, location), deploymentArtifacts path, expectedChanges (add, change, destroy), status.
- **FR-080**: Plan MUST include summary with: totalSteps, terraformSteps, bicepSteps, totalAdd, totalChange, totalDestroy, allVersionsPinned.

#### Deployment Record Structure

- **FR-081**: Deployment record MUST include: application, environment details, startedAt, completedAt, status.
- **FR-082**: Deployment record MUST include git context: commit, commitShort, branch, isDirty.
- **FR-083**: Deployment record MUST include plan reference: planFile, planCommit, generatedAt.
- **FR-084**: Deployment record MUST include steps array with: sequence, name, resourceType, tool, status, timing, changes, outputs, capturedResources.
- **FR-085**: Deployment record MUST include summary: totalSteps, succeededSteps, failedSteps, skippedSteps, resource counts.

#### GitHub Actions Execution

- **FR-086**: GitHub Action runner MUST be capable of running k3d cluster (approximately 45 seconds startup, ~875 MiB download).
- **FR-087**: k3d cluster MUST map `/github_workspace` in containers to `${GITHUB_WORKSPACE}` for file access.
- **FR-088**: Radius control plane in k3d MUST be configured to use repository-based types, recipes, and environments.
- **FR-089**: GitHub Actions workflow MUST leverage GitHub PR Checks for deployment status reporting.

#### Resource Type Extensions

- **FR-090**: A new `Radius.Core/applications` Resource Type MUST be created in the `radius-project/resource-types-contrib` repository.
- **FR-091**: The `Radius.Core/applications` Resource Type MUST have an `environment` property of type string.
- **FR-092**: Application models MUST declare an application resource using `Radius.Core/applications@2026-03-01-preview`.

#### Terraform State Management

- **FR-093**: Terraform state MUST be stored in a cloud backend (S3 for AWS, Azure Storage for Azure) rather than locally in the repository.
- **FR-094**: The `rad environment connect` command MUST configure or create the state backend storage as part of OIDC setup.
- **FR-095**: State backend credentials MUST use the same OIDC authentication configured for deployments.
- **FR-096**: State backend MUST support state locking to prevent concurrent modification conflicts.
- **FR-097**: State backend location MUST be recorded in the environment file `.radius/env.<NAME>.yaml`.

#### Concurrent Deployment Handling

- **FR-098**: The deployment workflow MUST acquire an environment-scoped lock before starting execution.
- **FR-099**: If a lock already exists for the target environment, the deployment workflow MUST fail immediately with an error.
- **FR-100**: The error message MUST clearly indicate another deployment is in progress and prompt the user to try again later.
- **FR-101**: The lock MUST be released when the deployment completes (success or failure).

#### Partial Deployment Failure Handling

- **FR-102**: When a deployment partially fails, successfully deployed resources MUST remain in place (no auto-rollback).
- **FR-103**: The deployment record MUST clearly identify which resources succeeded and which failed.
- **FR-104**: The deployment workflow MUST exit with a failure status when any resource fails to deploy.
- **FR-105**: Users MAY re-run `rad pr create` after fixing issues to resume/retry the deployment.

#### Destroy Scope

- **FR-106**: The `rad pr destroy` command MUST only destroy resources belonging to the specified application.
- **FR-107**: If `--application` is omitted, the command MUST error with a message listing available applications.

#### Secret Management

- **FR-108**: Configuration files MAY reference GitHub Secrets using the syntax `${{ secrets.SECRET_NAME }}`.
- **FR-109**: The deployment workflow MUST inject referenced GitHub Secrets as environment variables at runtime.
- **FR-110**: Secret references MUST NOT be expanded or logged during plan generation (`rad pr create`).
- **FR-111**: The `rad init` command SHOULD document the GitHub Secrets reference syntax in generated configuration file comments.

#### Workflow Installation

- **FR-112**: The `rad init github` command MUST generate `.github/workflows/radius-deploy.yml` workflow template for handling deployment PRs.
- **FR-113**: The `rad init github` command MUST generate `.github/workflows/radius-destroy.yml` workflow template for handling destruction PRs.
- **FR-114**: Generated workflow files MUST be included in the initial commit created by `rad init github`.

### Key Entities

- **Workspace**: User's working context stored in `~/.rad/config.yaml`; can be of kind `github` (URL-based connection) or `kubernetes` (context-based connection). GitHub workspaces connect to repository URLs; Kubernetes workspaces connect to cluster contexts with scope and environment references.

- **Resource Type**: Definition of infrastructure resource schemas stored externally in `radius-project/resource-types-contrib` repository; referenced via `.radius/types.yaml` with versioned git URLs. Includes types like `Radius.Core/applications`, `Radius.Compute/containers`, `Radius.Data/postgreSqlDatabases`, etc.

- **Recipe**: Implementation template for provisioning resources; referenced via `.radius/recipes.yaml` with `recipeKind` (terraform/bicep) and `recipeLocation` (git URL for Terraform, OCI registry URL for Bicep). Organized by provider (aws/azure) and deployment tool.

- **Environment**: Target deployment context with cloud provider configuration; stored in `.radius/env.<NAME>.yaml` with kind (aws/azure), recipes reference, optional recipeParameters, and provider-specific OIDC configuration (accountId/region/oidcRoleARN for AWS; subscriptionId/tenantId/clientId/resourceGroupName for Azure).

- **Application Model**: Bicep-based declaration of application resources and their relationships; stored in `.radius/model/<APP_NAME>.bicep`. Uses Radius resource types with environment parameter. Unchanged from current Radius application model format.

- **Deployment Plan**: Ordered sequence of resource provisioning steps generated by `rad plan deploy`; stored in `.radius/plan/<APP>/<ENV>/`. Contains plan.yaml with step ordering and individual deployment artifact directories with Terraform configurations.

- **Deployment Record**: Complete audit of a deployment execution; stored in `.radius/deploy/<APP>/<ENV>/<COMMIT>/`. Contains timing information, git context, step-by-step results, terraform outputs, and captured resource definitions.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a GitHub repository for Radius in under 5 minutes from a fresh clone, including resource type fetching.
- **SC-002**: Users can complete cloud provider OIDC setup (AWS or Azure) in under 10 minutes with guided prompts.
- **SC-003**: Deployment plans are visible and reviewable in standard GitHub Pull Request interface with clear change summaries.
- **SC-004**: 95% of deployment attempts complete within 15 minutes for applications with 5 or fewer resources.
- **SC-005**: GitHub Action setup (k3d + Radius) completes in under 60 seconds with approximately 875 MiB download.
- **SC-006**: Failed deployments provide actionable error messages that identify the failing resource, step, and root cause.
- **SC-007**: Deployment records provide complete audit trail with timing, resource definitions, and terraform outputs captured.
- **SC-008**: Users can successfully destroy all deployed resources with a single `rad pr destroy` + `rad pr merge` sequence.
- **SC-009**: 90% of users can complete their first deployment without external documentation beyond CLI help text.
- **SC-010**: Workspace switching between GitHub and Kubernetes modes works seamlessly with appropriate command behavior.

## Assumptions

- Users have the GitHub CLI (`gh`) installed and authenticated on their workstation.
- Users have cloud provider CLIs (`aws` or `az`) installed and authenticated for OIDC setup.
- The GitHub repository has GitHub Actions enabled with sufficient minutes quota for workflow execution.
- The `radius-project/resource-types-contrib` repository contains current Resource Type and Recipe definitions with stable versioning.
- GitHub Action runners have sufficient resources to run k3d clusters (standard GitHub-hosted runners are adequate).
- Users are familiar with Git workflows and GitHub Pull Requests.
- Application modeling is handled by a separate project/workflow that outputs `.radius/model/<APP>.bicep` files.
- Users commit application model changes before running `rad pr create`.

## Constraints

- Only Terraform deployment tool is supported initially; Bicep support for Azure is a future enhancement.
- Only AKS (Azure Kubernetes Service) and EKS (Amazon Elastic Kubernetes Service) are supported as deployment targets initially.
- Local development and on-premises deployment targets are not supported in initial release.
- Resource Groups concept from Radius on Kubernetes does not apply to Radius on GitHub.
- The existing `rad init` command functionality is replaced; existing Kubernetes-based users must use `rad install kubernetes`.
- Single active deployment per application/environment combination to avoid state conflicts.

## Dependencies

- GitHub CLI (`gh`) must be available on user workstations for initialization and PR operations.
- GitHub Actions must be available and enabled for the repository.
- External repository `radius-project/resource-types-contrib` must contain Resource Type and Recipe definitions.
- Cloud providers (AWS/Azure) must support OIDC federation with GitHub Actions.
- k3d must be installable and runnable on GitHub Action runners.
- Terraform must be available for recipe execution (installed in GitHub Actions).

## Out of Scope / Limitations

- Bicep recipe support (future enhancement for Azure only)
- Local development deployment targets
- On-premises deployment targets  
- Multi-cloud deployments within single application
- Automatic rollback on deployment failure
- Cost estimation for deployment plans
- Integration with external secret managers

## Future Enhancements

- Authentication test workflow triggered automatically after OIDC configuration to verify setup
- MCP server and tools for Copilot integration (e.g., "initialize this repository to use Radius", "create a model for this application", "create a PR to deploy this app")
- Automated PR summarization using Copilot SDK during planning phase
- Application visualization in PR comments (part of app graph workstream)
- Bicep recipe support for Azure deployments - add Bicep recipes to `resource-types-contrib` repository and update recipe generation to support `recipeKind: bicep` with OCI registry locations (e.g., `oci://ghcr.io/radius-project/recipes/azure/containers:v0.54.0`)
- Local development and on-premises deployment targets
- Parallel deployment support for independent resources
- Deployment approval workflows with required reviewers
- Custom Terraform backend configuration - allow users to specify their own existing S3/Azure Storage backend instead of auto-provisioned storage
- Add version number tag to Resource Type definition locations - support `?ref=<version>` suffix on `definitionLocation` URLs (e.g., `git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/container.yaml?ref=v0.54.0`) to pin resource type definitions to specific versions
- Add version number tag to recipe locations - support `?ref=<version>` suffix on Terraform recipe URLs and `:<version>` suffix on OCI/Bicep recipe URLs to pin recipes to specific versions

---

## Appendix A: Radius Execution Model

Radius on GitHub operates with these principles:

- **Ephemeral control plane**: Runs on a k3d cluster in a GitHub Action when triggered (to plan a deployment or execute a deployment)
- **PR-based workflow**: Leverages GitHub PR Checks for deployment approval
- **CLI consistency**: Uses the existing `rad` CLI with new commands inspired by `git` and `gh` CLI tools
- **Git-native storage**: All data stored in files in a GitHub repository (or cloned on the user's workstation)
- **No persistent state**: No data is persistent in the Radius control plane; all state is in the repository

---

## Appendix B: New Resource Type - Radius.Core/applications

A new `Radius.Core/applications` Resource Type MUST be created in the `radius-project/resource-types-contrib` repository with this schema:

```yaml
namespace: Radius.Core
types:
  applications:
    apiVersions:
      "2026-03-01-preview":
        schema:
          type: object
          properties:
            environment:
              type: string
            # Additional properties will be added in the future including owner and description
```

This resource type allows applications to be declared as first-class resources in the application model.

---

## Appendix C: Configuration Data Model Examples

### C.1 Resource Types Manifest (.radius/types.yaml)

```yaml
# Radius Resource Types manifest
# Generated by rad init
# Generated: 2026-02-05T21:35:58Z

types:
  Radius.Core/applications:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Core/applications/applications.yaml

  Radius.Compute/containers:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/container.yaml

  Radius.Compute/persistentVolumes:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/persistentVolumes/persistentVolumes.yaml

  Radius.Compute/routes:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/routes/routes.yaml

  Radius.Security/secrets:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Security/secrets/secrets.yaml

  Radius.Data/mySqlDatabases:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/mySqlDatabases/mySqlDatabases.yaml

  Radius.Data/postgreSqlDatabases:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/postgreSqlDatabases.yaml
```

### C.2 Recipes Manifest (.radius/recipes.yaml)

**AWS with Terraform:**
```yaml
# Radius Recipe manifest for AWS using Terraform
# Generated by rad init
# Generated: 2026-02-05T21:35:58Z

recipes:
  Radius.Compute/containers:
    recipeKind: terraform
    recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/aws/terraform?ref=v0.54.0

  Radius.Data/postgreSqlDatabases:
    recipeKind: terraform
    recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/recipes/aws/terraform?ref=v0.54.0
```

**Azure with Terraform:**
```yaml
# Radius Recipe manifest for Azure using Terraform
# Generated by rad init
# Generated: 2026-02-05T21:35:58Z

recipes:
  Radius.Compute/containers:
    recipeKind: terraform
    recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/azure/terraform?ref=v0.54.0

  Radius.Data/postgreSqlDatabases:
    recipeKind: terraform
    recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/recipes/azure/terraform?ref=v0.54.0
```

**Azure with Bicep:**
```yaml
# Radius Recipe manifest for Azure using Bicep
# Generated by rad init
# Generated: 2026-02-05T21:35:58Z

recipes:
  Radius.Compute/containers:
    recipeKind: bicep
    recipeLocation: https://ghcr.io/radius-project/recipes/azure/containers:0.54.0

  Radius.Data/postgreSqlDatabases:
    recipeKind: bicep
    recipeLocation: https://ghcr.io/radius-project/recipes/azure/postgreSqlDatabases:0.54.0
```

### C.3 Environment File (.radius/env.default.yaml)

```yaml
# Radius Environment
# Generated by rad init
# Generated: 2026-02-05T21:35:58Z

name: default
kind: aws  # or azure
recipes: .radius/recipes.yaml
recipeParameters:
  # Optional recipe-specific parameters
  # Radius.Compute/routes:
  #   gatewayName: nginx-gateway
  #   gatewayNamespace: nginx-gateway
provider:
  # Run `rad environment connect` to populate
  aws:
    accountId: <ACCOUNT_ID>
    region: <REGION>
    oidcRoleARN: arn:aws:iam::<ACCOUNT_ID>:role/<ROLE_NAME>
  # OR for Azure:
  # azure:
  #   subscriptionId: <SUBSCRIPTION_ID>
  #   resourceGroupName: <RESOURCE_GROUP_NAME>
  #   tenantId: <TENANT_ID>
  #   clientId: <CLIENT_ID>
  #   oidcEnabled: true
```

### C.4 Workspace Configuration (~/.rad/config.yaml)

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

### C.5 Application Model (.radius/model/todolist.bicep)

```bicep
extension radius

param environment string

resource app 'Radius.Core/applications@2026-03-01-preview' = {
  name: 'todolist'
  properties: {
    environment: environment
  }
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    environment: environment
    container: {
      image: 'ghcr.io/radius-project/samples/demo:latest'
      ports: {
        web: {
          containerPort: 3000
        }
      }
    }
    connections: {
      postgresql: {
        source: postgresql.id
      }
    }
  }
}

resource postgresql 'Radius.Data/postgreSqlDatabases@2025-08-01-preview' = {
  name: 'postgresql'
  properties: {
    application: app.id
    environment: environment
    size: 'S'
  }
}
```

---

## Appendix D: Deployment Plan Structure

### D.1 Plan File (.radius/plan/todolist/dev/plan.yaml)

```yaml
# Radius deployment plan
# Generated by rad plan
# Generated: 2026-02-05T21:35:58Z

application: todolist
applicationModelFile: .radius/model/todolist.bicep
environment: dev
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
    deploymentArtifacts: .radius/plan/todolist/dev/001-db-terraform
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
    deploymentArtifacts: .radius/plan/todolist/dev/002-frontend-terraform
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

### D.2 Deployment Artifact Files

Each deployment step directory (e.g., `.radius/plan/todolist/dev/001-db-terraform/`) contains:

**main.tf:**
```hcl
# Radius resource deployment plan
# Generated by rad plan
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
# Generated by rad plan
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
# Generated by rad plan
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
# Generated by rad plan
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

**tfplan.txt:** Contains the text output from `terraform plan` showing resources to be created/modified/destroyed.

**.terraform.lock.hcl:** Contains the Terraform provider lock file with pinned versions and hashes.

---

## Appendix E: Deployment Record Structure

### E.1 Deployment Record (.radius/deploy/todolist/default/<commit>/deploy-<commit>.json)

```json
{
  "application": "todolist",
  "environment": {
    "name": "default",
    "environmentFile": ".radius/env.default.yaml",
    "kubernetesContext": "rad-k3d",
    "kubernetesNamespace": "default"
  },
  "startedAt": "2026-02-11T16:41:48.199174-06:00",
  "completedAt": "2026-02-11T16:41:59.109376-06:00",
  "status": "succeeded",
  "git": {
    "commit": "da97a8da2de4af2e6273bce692f7a4fc2061b538",
    "commitShort": "da97a8da",
    "branch": "main",
    "isDirty": false
  },
  "plan": {
    "planFile": ".radius/plan/todolist/default/plan.yaml",
    "planCommit": "da97a8da2de4af2e6273bce692f7a4fc2061b538",
    "generatedAt": "2026-02-11T22:41:14.901269Z"
  },
  "steps": [
    {
      "sequence": 1,
      "name": "db",
      "resourceType": "Radius.Data/postgreSqlDatabases@2025-08-01-preview",
      "tool": "terraform",
      "status": "succeeded",
      "startedAt": "2026-02-11T16:41:48.203849-06:00",
      "completedAt": "2026-02-11T16:41:54.068321-06:00",
      "duration": 5864439792,
      "changes": {
        "add": 3,
        "change": 0,
        "destroy": 0
      },
      "outputs": {
        "result": {
          "values": {
            "database": "postgres_db",
            "host": "db.default.svc.cluster.local",
            "password": "REDACTED",
            "port": 5432,
            "username": "postgres"
          }
        }
      },
      "capturedResources": [
        {
          "resourceId": "default/deployment/db",
          "resourceDefinitionFile": "deployment-db.yaml"
        },
        {
          "resourceId": "default/service/db",
          "resourceDefinitionFile": "service-db.yaml"
        }
      ]
    },
    {
      "sequence": 2,
      "name": "frontend",
      "resourceType": "Radius.Compute/containers@2025-08-01-preview",
      "tool": "terraform",
      "status": "succeeded",
      "startedAt": "2026-02-11T16:41:54.068354-06:00",
      "completedAt": "2026-02-11T16:41:59.10935-06:00",
      "duration": 5040969000,
      "changes": {
        "add": 2,
        "change": 0,
        "destroy": 0
      },
      "outputs": {
        "result": {
          "resources": [
            "/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/frontend",
            "/planes/kubernetes/local/namespaces/default/providers/core/Service/frontend"
          ]
        }
      },
      "capturedResources": [
        {
          "resourceId": "default/deployment/frontend",
          "resourceDefinitionFile": "deployment-frontend.yaml"
        },
        {
          "resourceId": "default/service/frontend",
          "resourceDefinitionFile": "service-frontend.yaml"
        }
      ]
    }
  ],
  "resources": [],
  "summary": {
    "totalSteps": 2,
    "succeededSteps": 2,
    "failedSteps": 0,
    "skippedSteps": 0,
    "totalResources": 5,
    "resourcesAdded": 5,
    "resourcesChanged": 0,
    "resourcesDestroyed": 0
  }
}
```

### E.2 Captured Resource Files

For each deployed resource, the raw resource output is captured in platform-native format:

- **Kubernetes resources**: YAML manifests (e.g., `deployment-db.yaml`, `service-db.yaml`)
- **AWS resources**: JSON resource descriptions
- **Azure resources**: JSON resource definitions

### E.3 Destruction Record (.radius/deploy/todolist/default/<commit>/destroy-<commit>.json)

Same structure as deployment record, but with `status` reflecting destruction and `changes` showing resources destroyed rather than added.
