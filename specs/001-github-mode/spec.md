# Feature Specification: Radius on GitHub

**Feature Branch**: `001-github-mode`  
**Created**: 2026-02-12  
**Status**: Draft  
**Input**: User description: "Radius on GitHub - a new mode of operation where all data and processing runs in a GitHub repository"

## Overview

Radius on GitHub is a new mode of operation that enables users to deploy cloud applications using Radius without requiring a centralized Kubernetes-based control plane. All configuration, plans, and deployment records are stored directly in the GitHub repository, leveraging GitHub Actions for execution and GitHub Pull Requests for review and approval workflows.

This mode complements the existing "Radius on Kubernetes" mode, giving users the choice of how they want to operate Radius based on their infrastructure preferences.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize Repository for Radius (Priority: P1)

A developer wants to use Radius with their existing GitHub repository containing application source code. They run `rad init` to set up the repository with Radius configuration files, enabling GitHub-based deployments without needing to install or manage a Kubernetes control plane.

**Why this priority**: This is the entry point for all Radius on GitHub functionality. Without initialization, no other features can be used. It establishes the foundational configuration structure.

**Independent Test**: Can be fully tested by running `rad init` on a fresh GitHub repository clone and verifying all configuration files are created and committed.

**Acceptance Scenarios**:

1. **Given** a cloned GitHub repository without Radius configuration, **When** the user runs `rad init --provider aws --deployment-tool terraform`, **Then** the system creates `.radius/types.yaml`, `.radius/recipes.yaml`, and `.radius/env.default.yaml` files with AWS/Terraform configuration.

2. **Given** a cloned GitHub repository without Radius configuration, **When** the user runs `rad init --provider azure --deployment-tool bicep --environment production`, **Then** the system creates configuration files with Azure/Bicep configuration and the environment is named "production".

3. **Given** a directory that is not a Git repository, **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to initialize a Git repository first.

4. **Given** a Git repository without a GitHub remote, **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to add a GitHub remote.

5. **Given** the user is not authenticated with the GitHub CLI, **When** the user runs `rad init`, **Then** the system displays an error message instructing the user to authenticate using `gh auth login`.

6. **Given** successful initialization, **When** the command completes, **Then** the changes are committed with a message containing the trailer `Radius-Action: init`.

---

### User Story 2 - Connect Cloud Provider Authentication (Priority: P1)

After initializing the repository, a developer needs to establish secure authentication between GitHub Actions and their cloud provider (AWS or Azure). They run `rad environment connect` which guides them through setting up OIDC-based authentication, eliminating the need to store long-lived credentials.

**Why this priority**: Deployments cannot occur without cloud authentication. This is a critical prerequisite for all deployment operations.

**Independent Test**: Can be fully tested by running `rad environment connect` after initialization and verifying OIDC configuration is created in the cloud provider and environment file is updated.

**Acceptance Scenarios**:

1. **Given** an AWS environment without OIDC configured, **When** the user runs `rad environment connect`, **Then** the system prompts for AWS account ID and region, offers to create a new IAM role or use existing, and writes credentials to `.radius/env.default.yaml`.

2. **Given** an Azure environment without OIDC configured, **When** the user runs `rad environment connect`, **Then** the system prompts for subscription selection, resource group, and offers to create federated credentials or use existing.

3. **Given** AWS CLI is not installed or authenticated, **When** the user chooses to create a new IAM role, **Then** the system displays an error with instructions to install/authenticate AWS CLI.

4. **Given** successful OIDC configuration, **When** the command completes, **Then** the environment file is updated and changes are committed with trailer `Radius-Action: environment-connect`.

5. **Given** an environment that already has OIDC configured, **When** the user runs `rad environment connect`, **Then** the system offers to update or replace existing configuration.

---

### User Story 3 - Create Deployment Plan via Pull Request (Priority: P2)

A developer has modeled their application and wants to deploy it to an environment. They run `rad pr create` which triggers a GitHub Action that generates a detailed deployment plan and creates a Pull Request for review.

**Why this priority**: This enables the core value proposition of reviewable, auditable deployments through the familiar PR workflow. It depends on P1 stories being complete.

**Independent Test**: Can be fully tested by having an application model in place and running `rad pr create`, then verifying a PR is created with plan files.

**Acceptance Scenarios**:

1. **Given** a valid application model in `.radius/model/`, **When** the user runs `rad pr create --environment dev`, **Then** a GitHub Action creates a deployment branch and generates plan files in `.radius/plan/<app>/<env>/`.

2. **Given** a deployment plan is generated, **When** the GitHub Action completes, **Then** a Pull Request is created with the plan.yaml and all deployment artifacts for review.

3. **Given** multiple applications exist in the repository, **When** the user runs `rad pr create --environment dev` without specifying an application, **Then** plans are generated for all applications.

4. **Given** the user specifies a specific application, **When** the user runs `rad pr create --environment dev --application frontend`, **Then** only that application's plan is generated.

5. **Given** the plan generation encounters an error, **When** the GitHub Action fails, **Then** error details are captured and displayed in the workflow logs.

---

### User Story 4 - Deploy Application via PR Merge (Priority: P2)

After reviewing the deployment plan in a Pull Request, a developer merges it to trigger the actual deployment. They can use `rad pr merge` to initiate this process.

**Why this priority**: This completes the deployment workflow, delivering the core value of actually provisioning resources in the cloud.

**Independent Test**: Can be fully tested by having a deployment PR and running `rad pr merge`, then verifying resources are provisioned and deployment records are created.

**Acceptance Scenarios**:

1. **Given** a deployment PR exists, **When** the user runs `rad pr merge`, **Then** the PR is merged and a GitHub Action executes the deployment.

2. **Given** deployment succeeds, **When** execution completes, **Then** deployment records are stored in `.radius/deploy/<app>/<env>/<commit>/`, the PR is merged, and the branch is deleted.

3. **Given** deployment fails, **When** execution completes, **Then** error logs are captured, the PR is updated with failure details, and the branch is preserved for investigation.

4. **Given** no deployment PR exists, **When** the user runs `rad pr merge`, **Then** an appropriate error message is displayed.

5. **Given** the user wants to skip review, **When** the user runs `rad pr merge --yes`, **Then** the PR is merged automatically without manual review.

---

### User Story 5 - Destroy Application Resources (Priority: P3)

A developer needs to tear down deployed resources for an application. They run `rad pr destroy` to create a destruction plan PR, then merge it to execute the teardown.

**Why this priority**: Resource cleanup is important but secondary to initial deployment capability. It completes the full lifecycle management.

**Independent Test**: Can be fully tested by having deployed resources and running `rad pr destroy`, then verifying resources are removed and destruction records are created.

**Acceptance Scenarios**:

1. **Given** an application is deployed, **When** the user runs `rad pr destroy --environment dev`, **Then** a destruction plan is generated and a PR is created.

2. **Given** a destruction PR exists, **When** the user runs `rad pr merge`, **Then** resources are destroyed and destruction records are stored.

3. **Given** multiple applications are deployed, **When** the user runs `rad pr destroy --environment dev`, **Then** destruction plans are generated for all applications.

4. **Given** a specific application to destroy, **When** the user runs `rad pr destroy --environment dev --application frontend`, **Then** only that application's destruction plan is generated.

5. **Given** the user wants to skip review, **When** the user runs `rad pr destroy --environment dev --yes`, **Then** destruction proceeds automatically after plan generation.

---

### User Story 6 - Manage Workspaces Across Repositories (Priority: P3)

A developer works with multiple repositories, some using Radius on GitHub and others using Radius on Kubernetes. The workspace configuration allows seamless switching between different contexts.

**Why this priority**: Multi-repository support enhances user experience but is not essential for single-repo usage.

**Independent Test**: Can be fully tested by configuring multiple workspaces in `~/.rad/config.yaml` and switching between them.

**Acceptance Scenarios**:

1. **Given** multiple workspaces are configured, **When** the user switches workspaces, **Then** subsequent commands use the selected workspace configuration.

2. **Given** a GitHub workspace is current, **When** the user runs resource-type commands, **Then** a warning indicates these are managed via `.radius/types.yaml`.

3. **Given** a Kubernetes workspace is current, **When** the user runs commands, **Then** they operate against the Kubernetes control plane as before.

---

### Edge Cases

- What happens when the GitHub repository has no Actions enabled? System displays an error with instructions to enable GitHub Actions.
- What happens when OIDC role permissions are insufficient? Deployment fails with clear error message indicating missing permissions.
- What happens when a deployment is in progress and another is requested? The system queues or rejects based on concurrency settings.
- What happens when the plan file format is corrupted or manually edited incorrectly? Deployment validation fails with specific parsing errors.
- What happens when network connectivity to GitHub is lost during deployment? The GitHub Action retries or fails gracefully with state preserved.

## Requirements *(mandatory)*

### Functional Requirements

#### Initialization

- **FR-001**: System MUST replace the current `rad init` command with new functionality that initializes a GitHub repository for Radius.
- **FR-002**: System MUST require `--provider` flag (aws or azure) and `--deployment-tool` flag (terraform or bicep).
- **FR-003**: System MUST validate the current directory is a Git repository with a GitHub origin.
- **FR-004**: System MUST verify GitHub CLI (`gh`) is authenticated.
- **FR-005**: System MUST create `.radius/types.yaml` populated with Resource Type definitions from an external repository.
- **FR-006**: System MUST create `.radius/recipes.yaml` with recipes appropriate for the selected provider and deployment tool.
- **FR-007**: System MUST create `.radius/env.<ENVIRONMENT_NAME>.yaml` with placeholder for OIDC configuration.
- **FR-008**: System MUST update or create `~/.rad/config.yaml` with a workspace of kind `github`.
- **FR-009**: System MUST commit changes with a trailer `Radius-Action: init`.

#### Environment Connection

- **FR-010**: System MUST provide `rad environment connect` command for configuring cloud OIDC authentication.
- **FR-011**: For AWS environments, system MUST prompt for account ID, region, and offer to create or use existing IAM role.
- **FR-012**: For Azure environments, system MUST prompt for subscription, resource group, and offer to create or use existing federated credentials.
- **FR-013**: System MUST NOT store cloud secret keys locally; only OIDC-related identifiers are stored.
- **FR-014**: System MUST commit environment changes with trailer `Radius-Action: environment-connect`.

#### Deployment Planning

- **FR-015**: System MUST provide `rad pr create` command that triggers a remote GitHub Action workflow.
- **FR-016**: The GitHub Action MUST create a deployment branch named `deploy/<app>/<env>-<timestamp>`.
- **FR-017**: The GitHub Action MUST generate `plan.yaml` with ordered deployment steps.
- **FR-018**: For each deployment step, system MUST generate deployment artifacts (main.tf, providers.tf, variables.tf, terraform.tfvars.json, tfplan.txt, terraform-context.txt, .terraform.lock.hcl).
- **FR-019**: System MUST create a Pull Request containing all plan and artifact files.

#### Deployment Execution

- **FR-020**: System MUST provide `rad pr merge` command to merge deployment PRs.
- **FR-021**: On PR merge, a GitHub Action MUST execute deployments using the plan and artifacts.
- **FR-022**: System MUST store deployment records in `.radius/deploy/<app>/<env>/<commit>/deploy-<commit>.json`.
- **FR-023**: System MUST capture deployed resource definitions (YAML for Kubernetes, JSON for cloud resources).
- **FR-024**: On successful deployment, system MUST merge the PR and delete the branch.
- **FR-025**: On failed deployment, system MUST update the PR with error details and preserve the branch.

#### Destruction

- **FR-026**: System MUST provide `rad pr destroy` command that generates destruction plans.
- **FR-027**: Destruction branch MUST be named `destroy/<app>/<env>-<timestamp>`.
- **FR-028**: On destruction PR merge, system MUST execute destruction using the plan.
- **FR-029**: System MUST store destruction records in `.radius/deploy/<app>/<env>/<commit>/destroy-<commit>.json`.

#### Configuration Storage

- **FR-030**: Resource Types MUST be stored in `.radius/types.yaml` referencing external definitions.
- **FR-031**: Recipes MUST be stored in `.radius/recipes.yaml` or files referenced from environment files.
- **FR-032**: Environments MUST be stored in `.radius/env.<NAME>.yaml` files.
- **FR-033**: Application Models MUST be stored in `.radius/model/<APP_NAME>.bicep`.
- **FR-034**: Plans MUST be stored in `.radius/plan/<APP_NAME>/<ENVIRONMENT_NAME>/`.
- **FR-035**: Deployments MUST be stored in `.radius/deploy/<APP_NAME>/<ENVIRONMENT_NAME>/<COMMIT>/`.

#### Command Behavior Changes

- **FR-036**: `rad resource-type` commands MUST display a warning when workspace kind is `github`.
- **FR-037**: `rad environment` commands MUST display a warning when workspace kind is `github`.
- **FR-038**: `rad recipe` commands MUST display a warning when workspace kind is `github`.
- **FR-039**: Users wanting Kubernetes-based Radius MUST use `rad install kubernetes`.

### Key Entities

- **Workspace**: User working context stored in `~/.rad/config.yaml`; can be of kind `github` (URL-based) or `kubernetes` (context-based).

- **Resource Type**: Definition of infrastructure resource schemas; stored externally and referenced via `.radius/types.yaml`.

- **Recipe**: Implementation template for provisioning resources; referenced via `.radius/recipes.yaml` from Git repositories (Terraform) or OCI registries (Bicep).

- **Environment**: Target deployment context with cloud provider configuration; stored in `.radius/env.<NAME>.yaml` with OIDC authentication details.

- **Application Model**: Bicep-based declaration of application resources and their relationships; stored in `.radius/model/`.

- **Deployment Plan**: Ordered sequence of resource provisioning steps with all artifacts needed for execution; stored in `.radius/plan/`.

- **Deployment Record**: Complete audit of a deployment execution including timing, outputs, and captured resources; stored in `.radius/deploy/`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a GitHub repository for Radius in under 5 minutes from a fresh clone.
- **SC-002**: Users can complete cloud provider OIDC setup in under 10 minutes with guided prompts.
- **SC-003**: Deployment plans are visible and reviewable in standard GitHub Pull Request interface.
- **SC-004**: 95% of deployment attempts complete within 15 minutes for applications with 5 or fewer resources.
- **SC-005**: Failed deployments provide actionable error messages that identify the failing resource and root cause.
- **SC-006**: Deployment records provide complete audit trail with resource definitions captured.
- **SC-007**: Users can successfully destroy all deployed resources with a single command sequence.
- **SC-008**: 90% of users can complete their first deployment without external documentation beyond CLI help text.

## Assumptions

- Users have the GitHub CLI (`gh`) installed and authenticated on their workstation.
- Users have cloud provider CLIs (`aws` or `az`) installed and authenticated for OIDC setup.
- The GitHub repository has GitHub Actions enabled with sufficient minutes quota.
- The `radius-project/resource-types-contrib` repository contains current Resource Type and Recipe definitions.
- GitHub Action runners have sufficient resources to run k3d clusters (approximately 45 seconds startup, ~875 MiB download).
- Users are familiar with Git workflows and GitHub Pull Requests.

## Constraints

- Only Terraform deployment tool is supported initially; Bicep support for Azure is a future enhancement.
- Only AKS (Azure Kubernetes Service) and EKS (Amazon Elastic Kubernetes Service) are supported as deployment targets; local development and on-premises environments are future enhancements.
- Resource Groups concept from Radius on Kubernetes does not apply to Radius on GitHub.
- The existing `rad init` command functionality is replaced; existing Kubernetes-based users must use `rad install kubernetes`.

## Dependencies

- GitHub CLI (`gh`) must be available on user workstations.
- GitHub Actions must be available and have workflow support for the organization/repository.
- External repository `radius-project/resource-types-contrib` must contain Resource Type and Recipe definitions.
- Cloud providers (AWS/Azure) must support OIDC federation with GitHub Actions.

## Future Enhancements

- Authentication test workflow triggered automatically after OIDC configuration.
- MCP server and tools for Copilot integration (e.g., "initialize this repository to use Radius", "create a model for this application").
- Automated PR summarization using Copilot SDK during planning.
- Application visualization in PR comments (part of app graph workstream).
- Bicep recipe support for Azure deployments.
- Local development and on-premises deployment targets.
