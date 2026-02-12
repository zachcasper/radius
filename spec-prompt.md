# Radius on GitHub

Modify Radius to support **two modes of operation**:

1. **Radius on Kubernetes** – the existing centralized Radius control plane.
2. **Radius on GitHub** – a new mode of operation where all data and processing runs in a GitHub repository.

This specification focuses exclusively on **Radius on GitHub**.

## Radius on GitHub Overview

Radius on GitHub is tightly integrated with GitHub and leverages GitHub Actions and GitHub Pull Requests (PR) and PR checks.


## User Experience

### Prerequisites

* The user has a GitHub repository containing an application's source code. The repository does not have any infrastructure as code or any references to a cloud platform. 
* The user has `gh` installed and is authenticated to GitHub
* If using AWS, the `aws` CLI is installed and authenticated
* If using Azure, the `azure` CLI is installed and authenticated

## Step 1: Install Radius

The user installs Radius on their workstation just as they do today.

## Step 2: Initialize the GitHub repository

The user runs:

```bash
$ rad init

Usage:
  rad init --provider <aws|azure> --deployment-tool <terraform|bicep> [--environment <name>|-e <name>|--env <name>]

Flags:
  --provider           Cloud provider for deployment (required: aws or azure)
  --deployment-tool    Deployment tool to use (required: terraform or bicep)
  --environment, -e, --env
                       Optional environment name; defaults to "default"
```

> [!NOTE]
>
> The current `rad init` command is completely replaced by this new functionality. Users who want to install the Kubernetes-based version of Radius can still use `rad install kubernetes`. The new `rad init` command is modeled after `git init` and is not interactive.

The command:

1. Confirms the current working directory is a Git repository with a GitHub origin. If not, it warns the user that the current working directory must be a Git repository cloned from a GitHub repository and instructs the user to either create a repository in GitHub (online or via `gh repo create`) then clone the repository or clone an existing repository.
2. Confirms that `gh` is authenticated by running `gh auth status`.
3. Updated the existing, or creates a new `.rad/config.yaml` and add a new workspace with the same name as the repository name.
4. Creates `.radius/types.yaml` using the structure in the configuration data model below. The system populates types.yaml with Resource Types from the Radius resource-types-contrib repository using git sparse-checkout to clone/fetch only the .yaml files in non-hidden directories.

1. Creates `.radius/recipes.yaml` using correct manifest from the configuration data model section below based on the --provider and --deployment-tool flag.
2. Creates `.radius/env.<ENVIRONMENT_NAME>.yaml`. If the --environment flag was omitted, use 'default' for the environment name. The environment file is fully populated with the `rad environment connect` command next.
3. Performs a `git add` and `git commit` with an appropriate message and a trailer:

```
Radius-Action: init
```

In order to support this new user experience, Radius is modified with these changes:

#### Resource Types

* Resource Types are no longer stored within the Radius data store. They are stored in an external Git repository as specified in the `.radius/types.yaml` file.
* All `rad resource-type` commands do not work with the current workspace is of kind `github`. A warning is provided to the user that Resource Types are defined in `.radius/types.yaml` when using a GitHub workspace.
* A new Radius.Core/applications Resource Type is created in the resource-type-contrib repository with this schema:

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

#### Environments

* Environments are no longer stored within the Radius data store. They are stored in an external Git repository as specified in one or more `.radius/env.<NAME>.yaml` file.
* All `rad environment` commands do not work with the current workspace is of kind `github`. A warning is provided to the user that Environments are defined in `.radius/env.<NAME>.yaml` when using a GitHub workspace.

#### Recipes

* Recipes are no longer stored within the Radius data store. They are stored in an external Git repository as specified in the YAML file referenced in the `recipes` property of an environment YAML file.
* All `rad recipe` and `rad recipe-pack` commands do not work with the current workspace is of kind `github`. A warning is provided to the user that Recipes are defined in `.radius/recipes.yaml` or which ever file is referenced in the `recipes` property of the `.radius/env.<NAME>.yaml` file when using a GitHub workspace.

## Step 3: Authenticate GitHub to AWS and Azure

The user needs to setup OIDC authentication between GitHub and AWS or GitHub and Azure.

The user runs:

```bash
$ rad environment connect

Usage:
  rad environment connect [--environment <name>|-e <name>|--env <name>]

Flags:
  --environment, -e, --env
                       Name of the environment to connect; defaults to current workspace environment
```

If the environment kind is AWS, this command:

* Guides the user through connecting AWS to GitHub via OIDC.
* Operated on the `default` environment unless the environment flag is set.
* Checks that the current directory is a GitHub workspace and provides an error message if not.
* Confirms that the environment exists if specified, or default environment exists if not.
* Prompts the user to use their default AWS account ID (captured via `aws sts get-caller-identity --query Account --output text`) or provide an alternative account ID
* Prompts the user to use their default region (captured from `aws configure get region`) or provide an alternate region
* Prompts the user if they want to provide an existing AWS IAM Role ARN for OIDC access or create a new role in AWS.
  * If new, verify the AWS CLI is present and the user is authenticated by running `aws sts get-caller-identity`
  * Summarize the AWS commands to execute and ask the user to confirm they would like to create the IAM role
* Writes the account ID, region, ARN, to the environment file  `.radius/env.<ENV>.yaml`.
  - No AWS secret keys are stored locally.
* Future enhancement: 
  * Kick off a workflow in GitHub to test authentication

If the environment kind is Azure, this command:

* Guides the user through connecting Azure to GitHub via OIDC.
* Operated on the `default` environment unless the environment flag is set.
* Checks that the current directory is a GitHub workspace and provides an error message if not.
* Confirms that the environment exists if specified, or default environment exists if not.
* Prompts the user which subscription to use (captured from `az account list`)
* Prompt the user to input the resource group to use.
* Prompts the user to provide an existing Azure AD application / managed identity or create a new one that GitHub Actions can use.
  * If new, verify the Azure CLI is present and the user is authenticated by running `az account show`
  * Summarize the commands to execute and ask the user to confirm they would like to configure an Azure AD federated credential linking the GitHub repository/workflow to the Azure AD app.
* Writes the subscription ID, tenant ID, client ID, and resource group to the environment file  `.radius/env.<ENV>.yaml`.
  - No AWS secret keys are stored locally.
* Future enhancement: 
  * Kick off a workflow in GitHub to test authentication

Finally, the command performs a `git add` and `git commit` with an appropriate message and a trailer:

```
Radius-Action: environment-connect
```

## Step 4: Model an application

This step is implemented by a separate project. It outputs an `.radius/model/<APPLICATION_NAME>.bicep`. The user must `git add` and `git commit` before proceeding.

## Step 5: Prepare to deploy the app

The user runs:

```bash
$ rad pr create --environment dev

Usage:
  rad pr create --environment <name>|-e <name>|--env <name> [--application <name>|-a <name>|--app <name>]

Flags:
  --environment, -e, --env
                       Name of the environment to plan and deploy (required)
  --application, -a, --app
                       Optional application name; if omitted, plan for all applications
```

The command initiates a remote workflow in GitHub which:

* In a GitHub Action runner:

  * Create a branch in the GitHub repository for this deployment called `deploy/<APPLICATION_NAME>/<ENVIRONMENT_NAME>-<timestamp>` 
  * Install k3d
  * Install Radius CLI
  * Install Kubectl
  * Create k3d cluster with a hostPath volume mapping `/github_workspace` in all containers to `${GITHUB_WORKSPACE}`
  * Install Radius control plane which uses Resource Types, Recipes, and Environments defined in the repo
  * --- This should take ~45 seconds and downloads ~875 MiB ---
  * If an application was not specified, for each application in `.radius/model` run the new command `rad plan deploy /github_workspace/.radius/model/app.bicep --environment <ENVIRONMENT_NAME>`
    * If an application was specified, run `rad plan deploy` for only that application

  * `rad plan deploy` generates the plan.yaml file and deployment step directories as specified in the application model section below stored in `/github_workspace/.radius/plan`

  * Creates a GitHub PR with the new plan.yaml and deployment steps.

## Step 6: Deploy the app

The user runs:

```bash
$ rad pr merge

Usage:
  rad pr merge [--pr <number>] [--yes]

Flags:
  --pr                  Optional GitHub Pull Request number to merge; if omitted, merges the latest PR created by the CLI
  --yes                 Automatically merge the PR without review (use with caution)
```

The command initiates a remote workflow in GitHub which:

* In a GitHub Action runner:

  * Sets up Radius in a k3d cluster as before
  * Runs a modified `rad deploy /github_workspace/.radius/plan/plan.yaml --environment <ENVIRONMENT_NAME>` which takes the plan and deployment artifacts and deploys to the environment

  * `rad deploy` generates the deployment record as specified in the application model section below stored in `/github_workspace/.radius/deploy`

  * If the deployment is successful,
    * Merges the PR
    * Deletes the branch
  * If the deployment has an error, update the PR with the deployment logs and error outputs

## Step 7: Destroy the app

The user runs:

```bash
$ rad pr destroy --environment dev

Usage:
  rad pr destroy --environment <name>|-e <name>|--env <name> [--application <name>|-a <name>|--app <name>] [--commit <commit>] [--yes]

Flags:
  --environment, -e, --env
                        Name of the environment to delete resources from (required)
  --application, -a, --app
                        Optional application name; if omitted, deletes all applications in the environment
  --commit             Optional deployment commit hash; defaults to the latest deployment
  --yes                Automatically merge the destruction PR without review
```

The command initiates a remote workflow in GitHub which:

* In a GitHub Action runner:

  * Create a branch in the GitHub repository for this deployment called `destroy/<APPLICATION_NAME>/<ENVIRONMENT_NAME>-<timestamp>` 
  * Sets up Radius in a k3d cluster as before
  * If an application was not specified, for each application in `.radius/model` run the new command `rad plan destroy /github_workspace/.radius/model/app.bicep --environment <ENVIRONMENT_NAME>`
    * If an application was specified, run `rad plan destroy` for only that application
  * `rad plan destroy` generates the plan.yaml file and deployment step directories as specified in the application model section below stored in `/github_workspace/.radius/plan`
  * Creates a GitHub PR with the new plan.yaml and deployment steps.

Then the user:

```bash
$ rad pr merge
```

The command initiates a remote workflow in GitHub which:

* In a GitHub Action runner:

  * Sets up Radius in a k3d cluster as before  
  * Runs a new  `rad destroy /github_workspace/.radius/plan/plan.yaml --environment <ENVIRONMENT_NAME>` which takes the plan and deployment artifacts and deletes the resources in the environment
  * `rad destroy` generates the deployment record as specified in the application model section below stored in `/github_workspace/.radius/deploy`
    * If the destroy is successful,
      * Merges the PR
      * Deletes the branch

  * If the destroy has an error, update the PR with the deployment logs and error outputs

## Radius Execution Model

Radius on GitHub should:

- Run on a k3d cluster in a GitHub Action when triggered (to plan a deployment or execute a deployment)
- Leverage GitHub PR Checks
- Use the existing `rad` CLI
- Be inspired by the `git` and `gh` CLI tools
- Store all data in files in a GitHub repository (or cloned on the users workstation)

---

## Radius Configuration Data Model

Radius on GitHub configuration is stored in the GitHub repository. No data is persistent in the Radius control plane.

- **Resource Types**

  - Resource Type definitions are stored in a remote Git repository (not in the Radius repo)

  - `.radius/types.yaml` is a YAML manifest of remote locations of Resource Type definitions

  - There is one and only one `.radius/types.yaml`

    ```yaml
    # Radius Resource Types manifest
    # Generated by rad init
    # Generated: 2026-02-05T21:35:58Z
    
    types:
      Radius.Core/applications:
        definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Core/applications/applications.yaml?ref=v0.54.0
    
      Radius.Compute/containers:
        definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/container.yaml?ref=v0.54.0
    
      Radius.Compute/persistentVolumes:
        definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/persistentVolumes/persistentVolumes.yaml?ref=v0.54.0
    
      Radius.Compute/routes:
        definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/routes/routes.yaml?ref=v0.54.0
    
      Radius.Security/secrets:
        definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Security/secrets/secrets.yaml?ref=v0.54.0
    
      Radius.Data/mySqlDatabases:
        definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/mySqlDatabases/mySqlDatabases.yaml?ref=v0.54.0
    
      Radius.Data/postgreSqlDatabases:
        definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/postgreSqlDatabases.yaml?ref=v0.54.0
    ```

- **Recipes**

  - Recipes are stored in either a remote Git repository (Terraform) or an OCI registry (Bicep)

  - `.radius/recipes.yaml` is a YAML manifest of Recipe kind and locations

  - There can be multiple Recipe manifest files (they are referenced in the environment definition)

    ```yaml
    # Radius Recipe manifest for AWS using Terraform
    # Generated by rad init
    # Generated: 2026-02-05T21:35:58Z
    
    recipes:
      Radius.Compute/containers:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/aws/terraform?ref=v0.54.0
    
      Radius.Compute/persistentVolumes:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/persistentVolumes/recipes/aws/terraform?ref=v0.54.0
    
      Radius.Compute/routes:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/routes/recipes/aws/terraform?ref=v0.54.0
    
      Radius.Security/secrets:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Security/secrets/recipes/aws/terraform?ref=v0.54.0
    
      Radius.Data/mySqlDatabases:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/mySqlDatabases/recipes/aws/terraform?ref=v0.54.0
    
      Radius.Data/postgreSqlDatabases:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/recipes/aws/terraform?ref=v0.54.0
    ```

    or

      ```yaml
    # Radius Recipe manifest for Azure using Terraform
    # Generated by rad init
    # Generated: 2026-02-05T21:35:58Z
    
    recipes:
      Radius.Compute/containers:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/azure/terraform?ref=v0.54.0
    
      Radius.Compute/persistentVolumes:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/persistentVolumes/recipes/azure/terraform?ref=v0.54.0
    
      Radius.Compute/routes:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/routes/recipes/azure/terraform?ref=v0.54.0
    
      Radius.Security/secrets:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Security/secrets/recipes/azure/terraform?ref=v0.54.0
    
      Radius.Data/mySqlDatabases:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/mySqlDatabases/recipes/azure/terraform?ref=v0.54.0
    
      Radius.Data/postgreSqlDatabases:
        recipeKind: terraform
        recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/recipes/azure/terraform?ref=v0.54.0
      ```

     or   

      ```yaml
    # Radius Recipe manifest for Azure using Bicep
    # Generated by rad init
    # Generated: 2026-02-05T21:35:58Z
    
    recipes:
      Radius.Compute/containers:
        recipeKind: bicep
        recipeLocation: https://grcr.io/radius-project/recipes/azure/containers:0.54.0
    
      Radius.Compute/persistentVolumes:
        recipeKind: bicep
        recipeLocation: https://grcr.io/radius-project/recipes/azure/persistentVolumes:0.54.0
    
      Radius.Compute/routes:
        recipeKind: bicep
        recipeLocation: https://grcr.io/radius-project/recipes/azure/routes:0.54.0
    
      Radius.Security/secrets:
        recipeKind: bicep
        recipeLocation: https://grcr.io/radius-project/recipes/azure/secrets:0.54.0
    
      Radius.Data/mySqlDatabases:
        recipeKind: bicep
        recipeLocation: https://grcr.io/radius-project/recipes/azure/mySqlDatabases:0.54.0
    
      Radius.Data/postgreSqlDatabases:
        recipeKind: bicep
        recipeLocation: https://grcr.io/radius-project/recipes/azure/postgreSqlDatabases:0.54.0
      ```

- **Environments**

  - One or more YAML files named `.radius/env.<ENV_NAME>.yaml`

    ```yaml
    # Radius Environment 
    # Generated by rad init
    # Generated: 2026-02-05T21:35:58Z
    
    name: <ENV_NAME>
    kind: <aws|azure>
    recipes: .radius/recipes.yaml
    recipeParameters:
      # Optional
      # Radius.Compute/routes:
      #  - gatewayName: nginx-gateway
      #  - gatewayNamespace: nginx-gateway
    provider: 
      # Run `rad environment connect` to populate
      aws:
        accountId: <ACCOUNT_ID>
        region: <REGION>
        oidcRoleARN: arn:aws:iam::<ACCOUNT_ID>:role/<ROLE_NAME>
      azure:
        subscriptionId: <SUBSCRIPTION_ID>
        resourceGroupName: <RESOURCE_GROUP_NAME>
        tenantId: <TENANT_ID>
        clientId: <CLIENT_ID>
        oidcEnabled: true
    ```

* **Workspaces**

  * Uses the existing `~/.rad/config.yaml` file

  * These properties are renamed for clarity:

    * `default` --> `current`

  * A new kind is available called `github` which has a `url` property

    ```yaml
    # Radius CLI configuration 
    # Generated by rad init
    # Generated: 2026-02-05T21:35:58Z
    
    workspaces:
      current: <CURRENT_WORKSPACE_NAME>
      items:
        <GITHUB_WORKSPACE_NAME>:
          connection:
            url: https://github.com/<USER>/<REPOSITORY>
            kind: github
        <CONTROL_PLANE_NAME_WORKSPACE_NAME>:
          connection:
            context: rad-bank
            kind: kubernetes
          environment: commercial-test
          scope: /planes/radius/local/resourceGroups/commercial
    ```

* **Resource Groups**

  * Radius on GitHub does not have Resource Groups

## Application Data Model

* **Application Model**

  * The application model has not changed. It is the same `<APP_NAME>.bicep` that Radius uses today and is stored in `.radius/model`.

    ```yaml
    extension radius
    
    param environment
    
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

* **Plan**

  * Deployment plans are stored in the `.radius/plan/<APP_NAME>/<ENVIRONMENT_NAME>` directory

  * In the event there are multiple applications in the same repo, the application name is in the path

  * The `.radius/plan/<APP_NAME>/<ENVIRONMENT_NAME>/plan.yaml` file looks like:

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
                containers:
                    frontend:
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

  * For each deployment step, there is a `.radius/plan/<APPLICATION_NAME>/<ENVIRONMENT_NAME>/XXX-<RESOURCE_NAME>-<RECIPE_KIND>` directory. In each directory there is:

    * `main.tf`: A Terraform configuration for deploying the resource. Similar to:

      ```
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

    * `providers.tf`: The Terraform providers required for this deployment

      ```
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

    * `terraform-context.txt`: Environmental data 

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

    * `terraform.tfvars.json`: The set of Terraform variables for main.tf.

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

    * `tfplan.txt`: The text output from `terraform plan`.

      ```yaml
      Terraform used the selected providers to generate the following execution
      plan. Resource actions are indicated with the following symbols:
        + create
      
      Terraform will perform the following actions:
      
        # module.db.kubernetes_deployment.postgresql will be created
        + resource "kubernetes_deployment" "postgresql" {
            + id               = (known after apply)
            + wait_for_rollout = true
      
            + metadata {
                + generation       = (known after apply)
                + name             = "db"
                + namespace        = "default"
                + resource_version = (known after apply)
                + uid              = (known after apply)
              }
      
            + spec {
                + min_ready_seconds         = 0
                + paused                    = false
                + progress_deadline_seconds = 600
                + replicas                  = (known after apply)
                + revision_history_limit    = 10
      
                + selector {
                    + match_labels = {
                        + "app" = "postgres"
                      }
                  }
      
                + template {
                    + metadata {
                        + generation       = (known after apply)
                        + labels           = {
                            + "app" = "postgres"
                          }
                        + name             = (known after apply)
                        + resource_version = (known after apply)
                        + uid              = (known after apply)
                      }
                    + spec {
                        + automount_service_account_token  = true
                        + dns_policy                       = "ClusterFirst"
                        + enable_service_links             = true
                        + host_ipc                         = false
                        + host_network                     = false
                        + host_pid                         = false
                        + hostname                         = (known after apply)
                        + node_name                        = (known after apply)
                        + restart_policy                   = "Always"
                        + scheduler_name                   = (known after apply)
                        + service_account_name             = (known after apply)
                        + share_process_namespace          = false
                        + termination_grace_period_seconds = 30
      
                        + container {
                            + image                      = "postgres:16-alpine"
                            + image_pull_policy          = (known after apply)
                            + name                       = "postgres"
                            + stdin                      = false
                            + stdin_once                 = false
                            + termination_message_path   = "/dev/termination-log"
                            + termination_message_policy = (known after apply)
                            + tty                        = false
      
                            + env {
                                + name  = "POSTGRES_PASSWORD"
                                + value = (sensitive value)
                              }
                            + env {
                                + name  = "POSTGRES_USER"
                                + value = "postgres"
                              }
                            + env {
                                + name  = "POSTGRES_DB"
                                + value = "postgres_db"
                              }
      
                            + port {
                                + container_port = 5432
                                + protocol       = "TCP"
                              }
      
                            + resources {
                                + limits   = (known after apply)
                                + requests = {
                                    + "memory" = "512Mi"
                                  }
                              }
                          }
                      }
                  }
              }
          }
      
        # module.db.kubernetes_service.postgres will be created
        + resource "kubernetes_service" "postgres" {
            + id                     = (known after apply)
            + status                 = (known after apply)
            + wait_for_load_balancer = true
      
            + metadata {
                + generation       = (known after apply)
                + name             = "db"
                + namespace        = "default"
                + resource_version = (known after apply)
                + uid              = (known after apply)
              }
      
            + spec {
                + allocate_load_balancer_node_ports = true
                + cluster_ip                        = (known after apply)
                + cluster_ips                       = (known after apply)
                + external_traffic_policy           = (known after apply)
                + health_check_node_port            = (known after apply)
                + internal_traffic_policy           = (known after apply)
                + ip_families                       = (known after apply)
                + ip_family_policy                  = (known after apply)
                + publish_not_ready_addresses       = false
                + selector                          = {
                    + "app" = "postgres"
                  }
                + session_affinity                  = "None"
                + type                              = "ClusterIP"
      
                + port {
                    + node_port   = (known after apply)
                    + port        = 5432
                    + protocol    = "TCP"
                    + target_port = "5432"
                  }
              }
          }
      
        # module.db.random_password.password will be created
        + resource "random_password" "password" {
            + bcrypt_hash = (sensitive value)
            + id          = (known after apply)
            + length      = 16
            + lower       = true
            + min_lower   = 0
            + min_numeric = 0
            + min_special = 0
            + min_upper   = 0
            + number      = true
            + numeric     = true
            + result      = (sensitive value)
            + special     = false
            + upper       = true
          }
      
      Plan: 3 to add, 0 to change, 0 to destroy.
      
      Changes to Outputs:
        + result = (sensitive value)
      ```

    * `variables.tf`: The values of the variables.

      ```
      # Radius resource deployment plan
      # Generated by rad plan
      # Variables for resource: db (Radius.Data/postgreSqlDatabases)
      # Generated: 2026-02-05T21:36:10Z
      
      # Radius context variable - contains all deployment context information
      # See: https://docs.radapp.io/reference/context-schema/
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

    * `.terraform.lock.hcl`: A copy of the Terraform lock file.

      ```
      # This file is maintained automatically by "terraform init".
      # Manual edits may be lost in future updates.
      
      provider "registry.terraform.io/hashicorp/kubernetes" {
        version     = "2.38.0"
        constraints = ">= 2.0.0, ~> 2.0"
        hashes = [
          "h1:soK8Lt0SZ6dB+HsypFRDzuX/npqlMU6M0fvyaR1yW0k=",
          "zh:0af928d776eb269b192dc0ea0f8a3f0f5ec117224cd644bdacdc682300f84ba0",
          "zh:1be998e67206f7cfc4ffe77c01a09ac91ce725de0abaec9030b22c0a832af44f",
          "zh:326803fe5946023687d603f6f1bab24de7af3d426b01d20e51d4e6fbe4e7ec1b",
          "zh:4a99ec8d91193af961de1abb1f824be73df07489301d62e6141a656b3ebfff12",
          "zh:5136e51765d6a0b9e4dbcc3b38821e9736bd2136cf15e9aac11668f22db117d2",
          "zh:63fab47349852d7802fb032e4f2b6a101ee1ce34b62557a9ad0f0f0f5b6ecfdc",
          "zh:924fb0257e2d03e03e2bfe9c7b99aa73c195b1f19412ca09960001bee3c50d15",
          "zh:b63a0be5e233f8f6727c56bed3b61eb9456ca7a8bb29539fba0837f1badf1396",
          "zh:d39861aa21077f1bc899bc53e7233262e530ba8a3a2d737449b100daeb303e4d",
          "zh:de0805e10ebe4c83ce3b728a67f6b0f9d18be32b25146aa89116634df5145ad4",
          "zh:f569b65999264a9416862bca5cd2a6177d94ccb0424f3a4ef424428912b9cb3c",
          "zh:faf23e45f0090eef8ba28a8aac7ec5d4fdf11a36c40a8d286304567d71c1e7db",
        ]
      }
      
      provider "registry.terraform.io/hashicorp/random" {
        version = "3.8.1"
        hashes = [
          "h1:u8AKlWVDTH5r9YLSeswoVEjiY72Rt4/ch7U+61ZDkiQ=",
          "zh:08dd03b918c7b55713026037c5400c48af5b9f468f483463321bd18e17b907b4",
          "zh:0eee654a5542dc1d41920bbf2419032d6f0d5625b03bd81339e5b33394a3e0ae",
          "zh:229665ddf060aa0ed315597908483eee5b818a17d09b6417a0f52fd9405c4f57",
          "zh:2469d2e48f28076254a2a3fc327f184914566d9e40c5780b8d96ebf7205f8bc0",
          "zh:37d7eb334d9561f335e748280f5535a384a88675af9a9eac439d4cfd663bcb66",
          "zh:741101426a2f2c52dee37122f0f4a2f2d6af6d852cb1db634480a86398fa3511",
          "zh:78d5eefdd9e494defcb3c68d282b8f96630502cac21d1ea161f53cfe9bb483b3",
          "zh:a902473f08ef8df62cfe6116bd6c157070a93f66622384300de235a533e9d4a9",
          "zh:b85c511a23e57a2147355932b3b6dce2a11e856b941165793a0c3d7578d94d05",
          "zh:c5172226d18eaac95b1daac80172287b69d4ce32750c82ad77fa0768be4ea4b8",
          "zh:dab4434dba34aad569b0bc243c2d3f3ff86dd7740def373f2a49816bd2ff819b",
          "zh:f49fd62aa8c5525a5c17abd51e27ca5e213881d58882fd42fec4a545b53c9699",
        ]
      }
      ```

* **Deployment**

  * Deployment records are stored in the `.radius/deploy/<APP_NAME>/<ENVIRONMENT_NAME>/<GIT_COMMIT_HASH>` directory

  * They are named `deploy-<GIT_COMMIT_HASH>.json` 

  * In the event there are multiple applications in the same repo, the application name is in the path

  * The `deploy-<GIT_COMMIT_HASH>.json` looks like:

    ```json
    {
      "application": "todolist",
      "environment": {
        "name": "default",
        "environmentFile": ".env",
        "kubernetesContext": "rad-bank",
        "kubernetesNamespace": "commercial-dev"
      },
      "startedAt": "2026-02-11T16:41:48.199174-06:00",
      "completedAt": "2026-02-11T16:41:59.109376-06:00",
      "status": "succeeded",
      "git": {
        "commit": "da97a8da2de4af2e6273bce692f7a4fc2061b538",
        "commitShort": "da97a8da",
        "branch": "master",
        "isDirty": true
      },
      "plan": {
        "planFile": "/Users/zcasper/radius/dev/repo-radius-test/.radius/plan/todolist/default/plan.yaml",
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
                "host": "db.commercial-dev.svc.cluster.local",
                "password": "2OSdIHHsYFYnLG3T",
                "port": 5432,
                "username": "postgres"
              }
            }
          },
          "capturedResources": [
            {
              "resourceId": "commercial-dev/deployment/db",
              "resourceDefinitionFile": "deployment-db.yaml"
            },
            {
              "resourceId": "commercial-dev/service/db",
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
                "/planes/kubernetes/local/namespaces/commercial-dev/providers/apps/Deployment/frontend",
                "/planes/kubernetes/local/namespaces/commercial-dev/providers/core/Service/frontend-frontend"
              ]
            }
          },
          "capturedResources": [
            {
              "resourceId": "commercial-dev/deployment/frontend",
              "resourceDefinitionFile": "deployment-frontend.yaml"
            },
            {
              "resourceId": "commercial-dev/service/frontend-frontend",
              "resourceDefinitionFile": "service-frontend-frontend.yaml"
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

  * For each deployed resource, the raw resource output is captured right after deployment. This output is in the platform-native format. For Kubernetes, it is the YAML manifests. For Azure and AWS, it is the JSON dump of the resource. For example, deploying the application above yields:

    * `deployment-db.yaml`
    * `deployment-frontend.yaml`
    * `service-db.yaml`
    * `service-frontend.yaml`

* **Destroy**

  * Destroy records are stored in the `.radius/deploy/<APP_NAME>/<ENVIRONMENT_NAME>/<GIT_COMMIT_HASH>` directory
  * They are named `destroy-<GIT_COMMIT_HASH>.json` 

## Limitations/Out of Scope

* Only supports Terraform, Bicep will be supported in the future for Azure only
* Can only deploy to AKS or EKS, no local dev or on-premises yet

## Future Enhancements

1. Perform a authentication test via a GitHub Action test right after configuring OIDC
2. Add an MCP server and tools to enable 

```bash
copilot> initialize this repository to use Radius
copilot> Create a model for this application using Radius
copilot> model this application
copilot> analyze this application using Radius
copilot> create a PR to deploy this app
```

3. During the plan, use the Copilot SDK to summarize the PR.

4. Visualization in the PR (part of the app graph workstream)


 
