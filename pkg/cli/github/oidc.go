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

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

// OIDCSetup provides reusable OIDC setup functions for AWS and Azure.
// This struct encapsulates the dependencies needed for OIDC configuration.
type OIDCSetup struct {
	// Output is the output interface for logging.
	Output output.Interface
	// Prompter is the interface for user input.
	Prompter prompt.Interface
	// CmdRunner is the interface for running external commands.
	CmdRunner CommandRunner
	// Owner is the GitHub repository owner.
	Owner string
	// Repo is the GitHub repository name.
	Repo string
}

// NewOIDCSetup creates a new OIDCSetup instance.
func NewOIDCSetup(output output.Interface, prompter prompt.Interface, cmdRunner CommandRunner, owner, repo string) *OIDCSetup {
	return &OIDCSetup{
		Output:    output,
		Prompter:  prompter,
		CmdRunner: cmdRunner,
		Owner:     owner,
		Repo:      repo,
	}
}

// AWSOIDCResult contains the result of AWS OIDC setup.
type AWSOIDCResult struct {
	// AccountID is the AWS account ID.
	AccountID string
	// Region is the AWS region.
	Region string
	// RoleARN is the IAM role ARN with OIDC trust policy.
	RoleARN string
	// EKSClusterName is the selected EKS cluster name.
	EKSClusterName string
	// KubernetesNamespace is the Kubernetes namespace.
	KubernetesNamespace string
	// StateBackend contains S3 bucket and DynamoDB table info.
	StateBackend *AWSStateBackend
}

// AWSStateBackend contains AWS Terraform state backend configuration.
type AWSStateBackend struct {
	// Bucket is the S3 bucket name.
	Bucket string
	// Region is the S3 bucket region.
	Region string
	// DynamoDBTable is the DynamoDB table name for state locking.
	DynamoDBTable string
}

// AzureOIDCResult contains the result of Azure OIDC setup.
type AzureOIDCResult struct {
	// SubscriptionID is the Azure subscription ID.
	SubscriptionID string
	// TenantID is the Azure AD tenant ID.
	TenantID string
	// ClientID is the Azure AD application client ID.
	ClientID string
	// ResourceGroupName is the Azure resource group name.
	ResourceGroupName string
	// AKSClusterName is the selected AKS cluster name.
	AKSClusterName string
	// KubernetesNamespace is the Kubernetes namespace.
	KubernetesNamespace string
	// StateBackend contains Azure Storage account info.
	StateBackend *AzureStateBackend
}

// AzureStateBackend contains Azure Terraform state backend configuration.
type AzureStateBackend struct {
	// StorageAccountName is the Azure Storage account name.
	StorageAccountName string
	// ContainerName is the blob container name.
	ContainerName string
}

// SetupAWSOIDC configures AWS OIDC authentication for GitHub Actions.
// FR-024: Prompts for AWS account details, creates IAM OIDC provider and role.
func (s *OIDCSetup) SetupAWSOIDC(ctx context.Context) (*AWSOIDCResult, error) {
	s.Output.LogInfo("Setting up AWS OIDC authentication...")
	s.Output.LogInfo("")

	// FR-021: Verify AWS CLI authentication
	s.Output.LogInfo("Verifying AWS CLI authentication...")
	callerIdentity, err := s.CmdRunner.RunCommand(ctx, "aws", "sts", "get-caller-identity", "--output", "json")
	if err != nil {
		return nil, clierrors.Message("AWS CLI authentication failed. Run 'aws configure' to set up credentials.\nError: %v", err)
	}
	s.Output.LogInfo("AWS CLI authenticated.")

	// FR-028a: Prompt for account ID (default from caller identity)
	defaultAccountID := extractJSONField(callerIdentity, "Account")
	promptMsg := "AWS Account ID"
	if defaultAccountID != "" {
		promptMsg = fmt.Sprintf("AWS Account ID (detected: %s)", defaultAccountID)
	}
	accountID, err := s.Prompter.GetTextInput(promptMsg, prompt.TextInputOptions{
		Default:     defaultAccountID,
		Placeholder: defaultAccountID,
	})
	if err != nil {
		return nil, err
	}

	// FR-028b: Prompt for region (default from aws configure)
	defaultRegion, _ := s.CmdRunner.RunCommand(ctx, "aws", "configure", "get", "region")
	defaultRegion = strings.TrimSpace(defaultRegion)
	regionPrompt := "AWS Region"
	if defaultRegion != "" {
		regionPrompt = fmt.Sprintf("AWS Region (detected: %s)", defaultRegion)
	}
	region, err := s.Prompter.GetTextInput(regionPrompt, prompt.TextInputOptions{
		Default:     defaultRegion,
		Placeholder: defaultRegion,
	})
	if err != nil {
		return nil, err
	}

	// FR-020: Prompt whether to create new role or use existing
	createNewRole, err := s.Prompter.GetListInput([]string{"Create new IAM role", "Use existing IAM role ARN"}, "OIDC IAM Role")
	if err != nil {
		return nil, err
	}

	var roleARN string
	if createNewRole == "Use existing IAM role ARN" {
		roleARN, err = s.Prompter.GetTextInput("IAM Role ARN", prompt.TextInputOptions{})
		if err != nil {
			return nil, err
		}
	} else {
		roleARN, err = s.createAWSRole(ctx, accountID, region)
		if err != nil {
			return nil, err
		}
	}

	// Create state backend (S3 bucket + DynamoDB table)
	stateBackend, err := s.createAWSStateBackend(ctx, accountID, region)
	if err != nil {
		return nil, err
	}

	// FR-067-C: Prompt for EKS cluster selection
	eksClusterName, err := s.promptForEKSCluster(ctx, region)
	if err != nil {
		return nil, err
	}

	// Prompt for Kubernetes namespace
	kubernetesNamespace, err := s.Prompter.GetTextInput("Kubernetes namespace (press Enter for 'default')", prompt.TextInputOptions{
		Default: "default",
	})
	if err != nil {
		return nil, err
	}

	return &AWSOIDCResult{
		AccountID:           accountID,
		Region:              region,
		RoleARN:             roleARN,
		EKSClusterName:      eksClusterName,
		KubernetesNamespace: kubernetesNamespace,
		StateBackend:        stateBackend,
	}, nil
}

// SetupAzureOIDC configures Azure OIDC authentication for GitHub Actions.
// FR-028: Prompts for Azure details, creates AD application with federated credentials.
func (s *OIDCSetup) SetupAzureOIDC(ctx context.Context, envName string) (*AzureOIDCResult, error) {
	s.Output.LogInfo("Setting up Azure OIDC authentication...")
	s.Output.LogInfo("")

	// FR-027: Verify Azure CLI authentication
	s.Output.LogInfo("Verifying Azure CLI authentication...")
	accountInfo, err := s.CmdRunner.RunCommand(ctx, "az", "account", "show", "--output", "json")
	if err != nil {
		return nil, clierrors.Message("Azure CLI authentication failed. Run 'az login' to sign in.\nError: %v", err)
	}
	s.Output.LogInfo("Azure CLI authenticated.")

	defaultTenantID := extractJSONField(accountInfo, "tenantId")

	// FR-024: Prompt for subscription from list
	subscriptionID, tenantID, err := s.promptForAzureSubscription(ctx, defaultTenantID)
	if err != nil {
		return nil, err
	}

	// FR-025: Prompt for resource group
	resourceGroup, err := s.promptForAzureResourceGroup(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}

	// FR-026: Prompt for Azure AD application (list existing with create option, consistent with resource group prompt)
	clientID, err := s.promptForAzureApp(ctx, envName, subscriptionID, resourceGroup)
	if err != nil {
		return nil, err
	}

	// Create state backend (Azure Storage)
	stateBackend, err := s.createAzureStateBackend(ctx, subscriptionID, resourceGroup.Name)
	if err != nil {
		return nil, err
	}

	// FR-067-C: Prompt for AKS cluster selection
	aksClusterName, err := s.promptForAKSCluster(ctx, subscriptionID, resourceGroup.Name)
	if err != nil {
		return nil, err
	}

	// Prompt for Kubernetes namespace
	kubernetesNamespace, err := s.Prompter.GetTextInput("Kubernetes namespace (press Enter for 'default')", prompt.TextInputOptions{
		Default: "default",
	})
	if err != nil {
		return nil, err
	}

	return &AzureOIDCResult{
		SubscriptionID:      subscriptionID,
		TenantID:            tenantID,
		ClientID:            clientID,
		ResourceGroupName:   resourceGroup.Name,
		AKSClusterName:      aksClusterName,
		KubernetesNamespace: kubernetesNamespace,
		StateBackend:        stateBackend,
	}, nil
}

// SetAWSEnvironmentVariables sets the required environment variables for AWS on a GitHub Environment.
// FR-025: Sets AWS_ACCOUNT_ID, AWS_REGION, AWS_IAM_ROLE_NAME, KUBERNETES_CLUSTER, KUBERNETES_NAMESPACE.
func (s *OIDCSetup) SetAWSEnvironmentVariables(envName string, result *AWSOIDCResult) error {
	ghClient := NewClient()

	// Extract just the role name from the full ARN if needed
	// The ARN format is arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME
	roleName := result.RoleARN
	if idx := strings.LastIndex(roleName, "/"); idx >= 0 {
		roleName = roleName[idx+1:]
	}

	vars := map[string]string{
		"AWS_ACCOUNT_ID":       result.AccountID,
		"AWS_REGION":           result.Region,
		"AWS_IAM_ROLE_NAME":    roleName,
		"KUBERNETES_CLUSTER":   result.EKSClusterName,
		"KUBERNETES_NAMESPACE": result.KubernetesNamespace,
	}

	if result.StateBackend != nil {
		vars["TF_STATE_BUCKET"] = result.StateBackend.Bucket
		vars["TF_STATE_REGION"] = result.StateBackend.Region
		vars["TF_STATE_DYNAMODB_TABLE"] = result.StateBackend.DynamoDBTable
	}

	for key, value := range vars {
		s.Output.LogInfo("  Setting %s...", key)
		if err := ghClient.SetEnvironmentVariable(s.Owner, s.Repo, envName, key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}

	return nil
}

// SetAzureEnvironmentVariables sets the required environment variables for Azure on a GitHub Environment.
// FR-029: Sets AZURE_SUBSCRIPTION_ID, AZURE_RESOURCE_GROUP, AZURE_TENANT_ID, AZURE_CLIENT_ID, KUBERNETES_CLUSTER, KUBERNETES_NAMESPACE.
func (s *OIDCSetup) SetAzureEnvironmentVariables(envName string, result *AzureOIDCResult) error {
	ghClient := NewClient()

	vars := map[string]string{
		"AZURE_SUBSCRIPTION_ID": result.SubscriptionID,
		"AZURE_RESOURCE_GROUP":  result.ResourceGroupName,
		"AZURE_TENANT_ID":       result.TenantID,
		"AZURE_CLIENT_ID":       result.ClientID,
		"KUBERNETES_CLUSTER":    result.AKSClusterName,
		"KUBERNETES_NAMESPACE":  result.KubernetesNamespace,
	}

	if result.StateBackend != nil {
		vars["TF_STATE_STORAGE_ACCOUNT"] = result.StateBackend.StorageAccountName
		vars["TF_STATE_CONTAINER"] = result.StateBackend.ContainerName
	}

	for key, value := range vars {
		s.Output.LogInfo("  Setting %s...", key)
		if err := ghClient.SetEnvironmentVariable(s.Owner, s.Repo, envName, key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}

	return nil
}

// createAWSRole creates an IAM role with GitHub OIDC trust policy.
func (s *OIDCSetup) createAWSRole(ctx context.Context, accountID, region string) (string, error) {
	roleName := fmt.Sprintf("radius-%s-%s", s.Owner, s.Repo)

	s.Output.LogInfo("")
	s.Output.LogInfo("The following AWS resources will be created:")
	s.Output.LogInfo("  - IAM OIDC provider for GitHub Actions (if not exists)")
	s.Output.LogInfo("  - IAM role: %s", roleName)
	s.Output.LogInfo("")

	confirm, err := s.Prompter.GetListInput([]string{"Yes, create these resources", "No, cancel"}, "Confirm")
	if err != nil {
		return "", err
	}
	if confirm != "Yes, create these resources" {
		return "", clierrors.Message("Operation cancelled by user.")
	}

	// Create OIDC provider if not exists
	s.Output.LogInfo("Creating IAM OIDC provider for GitHub...")
	_, err = s.CmdRunner.RunCommand(ctx, "aws", "iam", "create-open-id-connect-provider",
		"--url", "https://token.actions.githubusercontent.com",
		"--client-id-list", "sts.amazonaws.com",
		"--thumbprint-list", "6938fd4d98bab03faadb97b34396831e3780aea1",
	)
	if err != nil && !strings.Contains(err.Error(), "EntityAlreadyExists") {
		return "", fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	// Create trust policy document
	trustPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::%s:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:%s/%s:*"
        }
      }
    }
  ]
}`, accountID, s.Owner, s.Repo)

	// Create the IAM role
	s.Output.LogInfo("Creating IAM role: %s...", roleName)
	_, err = s.CmdRunner.RunCommand(ctx, "aws", "iam", "create-role",
		"--role-name", roleName,
		"--assume-role-policy-document", trustPolicy,
		"--description", "Radius deployment role for GitHub Actions",
	)
	if err != nil && !strings.Contains(err.Error(), "EntityAlreadyExists") {
		return "", fmt.Errorf("failed to create IAM role: %w", err)
	}

	// Attach necessary policies
	// AWS limits roles to 10 managed policies. This set covers EKS, Terraform recipes,
	// and common AWS resources. For prototype/development use.
	s.Output.LogInfo("Attaching policies to role...")
	policies := []string{
		// Compute & networking
		"arn:aws:iam::aws:policy/AmazonEC2FullAccess",
		"arn:aws:iam::aws:policy/AmazonVPCFullAccess",
		// Storage & state
		"arn:aws:iam::aws:policy/AmazonS3FullAccess",
		"arn:aws:iam::aws:policy/AmazonDynamoDBFullAccess",
		// EKS: fetch cluster endpoint/CA, generate kubeconfig tokens
		"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
		"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		// Databases: RDS (PostgreSQL, MySQL, etc.) for Terraform recipes
		"arn:aws:iam::aws:policy/AmazonRDSFullAccess",
		// Caching: ElastiCache (Redis, Memcached) for Terraform recipes
		"arn:aws:iam::aws:policy/AmazonElastiCacheFullAccess",
		// Messaging: SQS for Terraform recipes
		"arn:aws:iam::aws:policy/AmazonSQSFullAccess",
		// IAM: Terraform may need to create service-linked roles
		"arn:aws:iam::aws:policy/IAMFullAccess",
	}
	for _, policy := range policies {
		_, err = s.CmdRunner.RunCommand(ctx, "aws", "iam", "attach-role-policy",
			"--role-name", roleName,
			"--policy-arn", policy,
		)
		if err != nil {
			s.Output.LogInfo("Warning: could not attach policy %s: %v", policy, err)
		}
	}

	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
	s.Output.LogInfo("IAM role created: %s", roleARN)

	return roleARN, nil
}

// createAWSStateBackend creates S3 bucket and DynamoDB table for Terraform state.
func (s *OIDCSetup) createAWSStateBackend(ctx context.Context, accountID, region string) (*AWSStateBackend, error) {
	bucketName := fmt.Sprintf("tfstate-%s-%s", s.Owner, s.Repo)
	tableName := fmt.Sprintf("tfstate-lock-%s-%s", s.Owner, s.Repo)

	s.Output.LogInfo("")
	s.Output.LogInfo("Creating Terraform state backend...")

	// FR-047, FR-093: Create S3 bucket for state storage
	s.Output.LogInfo("Creating S3 bucket: %s...", bucketName)
	_, err := s.CmdRunner.RunCommand(ctx, "aws", "s3api", "create-bucket",
		"--bucket", bucketName,
		"--region", region,
		"--create-bucket-configuration", fmt.Sprintf("LocationConstraint=%s", region),
	)
	if err != nil && !strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") {
		return nil, fmt.Errorf("failed to create S3 bucket: %w", err)
	}

	// Enable versioning
	_, err = s.CmdRunner.RunCommand(ctx, "aws", "s3api", "put-bucket-versioning",
		"--bucket", bucketName,
		"--versioning-configuration", "Status=Enabled",
	)
	if err != nil {
		s.Output.LogInfo("Warning: could not enable bucket versioning: %v", err)
	}

	// FR-096: Create DynamoDB table for state locking
	s.Output.LogInfo("Creating DynamoDB table for state locking: %s...", tableName)
	_, err = s.CmdRunner.RunCommand(ctx, "aws", "dynamodb", "create-table",
		"--table-name", tableName,
		"--attribute-definitions", "AttributeName=LockID,AttributeType=S",
		"--key-schema", "AttributeName=LockID,KeyType=HASH",
		"--billing-mode", "PAY_PER_REQUEST",
		"--region", region,
	)
	if err != nil && !strings.Contains(err.Error(), "ResourceInUseException") {
		return nil, fmt.Errorf("failed to create DynamoDB table: %w", err)
	}

	s.Output.LogInfo("Terraform state backend created.")

	return &AWSStateBackend{
		Bucket:        bucketName,
		Region:        region,
		DynamoDBTable: tableName,
	}, nil
}

// createAzureApp creates an Azure AD application with federated credentials.
func (s *OIDCSetup) createAzureApp(ctx context.Context, envName, subscriptionID string, resourceGroup *AzureResourceGroupSelection) (string, error) {
	appName := fmt.Sprintf("radius-%s-%s", s.Owner, s.Repo)

	s.Output.LogInfo("")
	s.Output.LogInfo("The following Azure resources will be created:")
	s.Output.LogInfo("  - Azure AD application: %s", appName)
	s.Output.LogInfo("  - Federated credential for GitHub Actions")
	if resourceGroup.IsNew {
		s.Output.LogInfo("  - Resource group: %s (in %s)", resourceGroup.Name, resourceGroup.Location)
	} else {
		s.Output.LogInfo("  - Resource group: %s (existing)", resourceGroup.Name)
	}
	s.Output.LogInfo("")

	confirm, err := s.Prompter.GetListInput([]string{"Yes, create these resources", "No, cancel"}, "Confirm")
	if err != nil {
		return "", err
	}
	if confirm != "Yes, create these resources" {
		return "", clierrors.Message("Operation cancelled by user.")
	}

	// Only create resource group if it's new
	if resourceGroup.IsNew {
		s.Output.LogInfo("Creating resource group: %s in %s...", resourceGroup.Name, resourceGroup.Location)
		_, err = s.CmdRunner.RunCommand(ctx, "az", "group", "create",
			"--name", resourceGroup.Name,
			"--location", resourceGroup.Location,
			"--subscription", subscriptionID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to create resource group: %w", err)
		}
	} else {
		s.Output.LogInfo("Using existing resource group: %s", resourceGroup.Name)
	}

	// Check if app already exists
	s.Output.LogInfo("Creating Azure AD application: %s...", appName)
	existingAppOutput, _ := s.CmdRunner.RunCommand(ctx, "az", "ad", "app", "list",
		"--display-name", appName,
		"--output", "json",
	)

	var clientID, objectID string
	var existingApps []map[string]interface{}
	if err := json.Unmarshal([]byte(existingAppOutput), &existingApps); err == nil && len(existingApps) > 0 {
		if appID, ok := existingApps[0]["appId"].(string); ok {
			clientID = appID
		}
		if objID, ok := existingApps[0]["id"].(string); ok {
			objectID = objID
		}
		s.Output.LogInfo("Using existing Azure AD application: %s", clientID)
	} else {
		appOutput, err := s.CmdRunner.RunCommand(ctx, "az", "ad", "app", "create",
			"--display-name", appName,
			"--output", "json",
		)
		if err != nil {
			return "", fmt.Errorf("failed to create Azure AD application: %w", err)
		}

		clientID = extractJSONField(appOutput, "appId")
		objectID = extractJSONField(appOutput, "id")
	}

	// Create service principal
	s.Output.LogInfo("Creating service principal...")
	_, err = s.CmdRunner.RunCommand(ctx, "az", "ad", "sp", "create",
		"--id", clientID,
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", fmt.Errorf("failed to create service principal: %w", err)
	}

	// Assign Contributor role at resource group level
	s.Output.LogInfo("Assigning Contributor role to resource group...")
	_, err = s.CmdRunner.RunCommand(ctx, "az", "role", "assignment", "create",
		"--assignee", clientID,
		"--role", "Contributor",
		"--scope", fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subscriptionID, resourceGroup.Name),
	)
	if err != nil {
		s.Output.LogInfo("Warning: could not assign Contributor role to resource group: %v", err)
	}

	// Assign Reader role at subscription level
	s.Output.LogInfo("Assigning Reader role to subscription...")
	_, err = s.CmdRunner.RunCommand(ctx, "az", "role", "assignment", "create",
		"--assignee", clientID,
		"--role", "Reader",
		"--scope", fmt.Sprintf("/subscriptions/%s", subscriptionID),
	)
	if err != nil {
		s.Output.LogInfo("Warning: could not assign Reader role to subscription: %v", err)
	}

	// FR-052: Create federated credential for GitHub Actions
	s.Output.LogInfo("Creating federated credential for GitHub Actions...")
	fedCredParams := fmt.Sprintf(`{
  "name": "github-actions-%s",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:%s/%s:environment:%s",
  "audiences": ["api://AzureADTokenExchange"],
  "description": "GitHub Actions OIDC for %s environment"
}`, envName, s.Owner, s.Repo, envName, envName)

	_, err = s.CmdRunner.RunCommand(ctx, "az", "ad", "app", "federated-credential", "create",
		"--id", objectID,
		"--parameters", fedCredParams,
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", fmt.Errorf("failed to create federated credential: %w", err)
	}

	s.Output.LogInfo("Azure AD application created: %s", clientID)
	s.Output.LogInfo("Client ID: %s", clientID)

	return clientID, nil
}

// createAzureStateBackend creates Azure Storage for Terraform state.
func (s *OIDCSetup) createAzureStateBackend(ctx context.Context, subscriptionID, resourceGroupName string) (*AzureStateBackend, error) {
	storageAccountName := sanitizeStorageAccountName(fmt.Sprintf("tfstateradius%s%s", s.Owner, s.Repo))
	containerName := "tfstate"

	s.Output.LogInfo("")
	s.Output.LogInfo("Creating Terraform state backend...")

	// Create storage account
	s.Output.LogInfo("Creating storage account: %s...", storageAccountName)
	_, err := s.CmdRunner.RunCommand(ctx, "az", "storage", "account", "create",
		"--name", storageAccountName,
		"--resource-group", resourceGroupName,
		"--subscription", subscriptionID,
		"--sku", "Standard_LRS",
		"--kind", "StorageV2",
		"--min-tls-version", "TLS1_2",
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, fmt.Errorf("failed to create storage account: %w", err)
	}

	// Create blob container
	s.Output.LogInfo("Creating blob container: %s...", containerName)
	_, err = s.CmdRunner.RunCommand(ctx, "az", "storage", "container", "create",
		"--name", containerName,
		"--account-name", storageAccountName,
		"--auth-mode", "login",
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, fmt.Errorf("failed to create storage container: %w", err)
	}

	s.Output.LogInfo("Terraform state backend created.")

	return &AzureStateBackend{
		StorageAccountName: storageAccountName,
		ContainerName:      containerName,
	}, nil
}

// promptForAzureSubscription prompts the user to select an Azure subscription.
func (s *OIDCSetup) promptForAzureSubscription(ctx context.Context, defaultTenantID string) (subscriptionID, tenantID string, err error) {
	s.Output.LogInfo("Fetching available subscriptions...")
	subscriptionsJSON, err := s.CmdRunner.RunCommand(ctx, "az", "account", "list", "--output", "json")
	if err != nil {
		return "", "", fmt.Errorf("failed to list Azure subscriptions: %w", err)
	}

	type azureSubscription struct {
		Name     string `json:"name"`
		ID       string `json:"id"`
		TenantID string `json:"tenantId"`
	}

	var subscriptions []azureSubscription
	if err := json.Unmarshal([]byte(subscriptionsJSON), &subscriptions); err != nil {
		return "", "", fmt.Errorf("failed to parse subscription list: %w", err)
	}

	if len(subscriptions) == 0 {
		return "", "", clierrors.Message("No Azure subscriptions found. Run 'az login' to sign in with access to subscriptions.")
	}

	options := make([]string, len(subscriptions))
	for i, sub := range subscriptions {
		options[i] = fmt.Sprintf("%s (%s)", sub.Name, sub.ID)
	}

	selected, err := s.Prompter.GetListInput(options, "Select Azure Subscription")
	if err != nil {
		return "", "", err
	}

	for _, sub := range subscriptions {
		displayName := fmt.Sprintf("%s (%s)", sub.Name, sub.ID)
		if selected == displayName {
			return sub.ID, sub.TenantID, nil
		}
	}

	return "", "", fmt.Errorf("selected subscription not found")
}

// AzureResourceGroupSelection holds the result of resource group selection.
type AzureResourceGroupSelection struct {
	Name     string
	Location string
	IsNew    bool
}

// promptForAzureResourceGroup prompts the user to select or create an Azure resource group.
func (s *OIDCSetup) promptForAzureResourceGroup(ctx context.Context, subscriptionID string) (*AzureResourceGroupSelection, error) {
	s.Output.LogInfo("Fetching resource groups...")
	rgJSON, err := s.CmdRunner.RunCommand(ctx, "az", "group", "list",
		"--subscription", subscriptionID,
		"--output", "json",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list resource groups: %w", err)
	}

	type azureResourceGroup struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	}

	var groups []azureResourceGroup
	if err := json.Unmarshal([]byte(rgJSON), &groups); err != nil {
		return nil, fmt.Errorf("failed to parse resource group list: %w", err)
	}

	options := []string{"Create new resource group"}
	for _, rg := range groups {
		options = append(options, fmt.Sprintf("%s (%s)", rg.Name, rg.Location))
	}

	selected, err := s.Prompter.GetListInput(options, "Select Resource Group")
	if err != nil {
		return nil, err
	}

	if selected == "Create new resource group" {
		name, err := s.Prompter.GetTextInput("Resource group name", prompt.TextInputOptions{
			Default: fmt.Sprintf("radius-%s-%s", s.Owner, s.Repo),
		})
		if err != nil {
			return nil, err
		}

		// Prompt for location when creating a new resource group
		location, err := s.promptForAzureLocation(ctx, subscriptionID)
		if err != nil {
			return nil, err
		}

		return &AzureResourceGroupSelection{
			Name:     name,
			Location: location,
			IsNew:    true,
		}, nil
	}

	// Extract name and location from display string (format: "name (location)")
	parts := strings.SplitN(selected, " (", 2)
	name := selected
	location := ""
	if len(parts) == 2 {
		name = parts[0]
		location = strings.TrimSuffix(parts[1], ")")
	}

	return &AzureResourceGroupSelection{
		Name:     name,
		Location: location,
		IsNew:    false,
	}, nil
}

// promptForAzureLocation prompts the user to select an Azure location/region.
func (s *OIDCSetup) promptForAzureLocation(ctx context.Context, subscriptionID string) (string, error) {
	s.Output.LogInfo("Fetching Azure locations...")
	locationsJSON, err := s.CmdRunner.RunCommand(ctx, "az", "account", "list-locations",
		"--subscription", subscriptionID,
		"--output", "json",
	)
	if err != nil {
		return "", fmt.Errorf("failed to list Azure locations: %w", err)
	}

	type azureLocation struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	}

	var locations []azureLocation
	if err := json.Unmarshal([]byte(locationsJSON), &locations); err != nil {
		return "", fmt.Errorf("failed to parse location list: %w", err)
	}

	// Sort locations by display name for easier selection
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].DisplayName < locations[j].DisplayName
	})

	// Build options with display names but return the internal name
	options := make([]string, len(locations))
	locationMap := make(map[string]string)
	for i, loc := range locations {
		options[i] = loc.DisplayName
		locationMap[loc.DisplayName] = loc.Name
	}

	selected, err := s.Prompter.GetListInput(options, "Select location for the resource group")
	if err != nil {
		return "", err
	}

	return locationMap[selected], nil
}

// promptForAzureApp prompts the user to select an existing Azure AD application or create a new one.
// Follows the same pattern as promptForAzureResourceGroup: "Create new" first, then existing items.
func (s *OIDCSetup) promptForAzureApp(ctx context.Context, envName, subscriptionID string, resourceGroup *AzureResourceGroupSelection) (string, error) {
	s.Output.LogInfo("Fetching Azure AD applications...")
	appsJSON, err := s.CmdRunner.RunCommand(ctx, "az", "ad", "app", "list",
		"--output", "json",
	)
	if err != nil {
		return "", fmt.Errorf("failed to list Azure AD applications: %w", err)
	}

	type azureApp struct {
		DisplayName string `json:"displayName"`
		AppID       string `json:"appId"`
	}

	var apps []azureApp
	if err := json.Unmarshal([]byte(appsJSON), &apps); err != nil {
		return "", fmt.Errorf("failed to parse application list: %w", err)
	}

	options := []string{"Create new Azure AD application"}
	for _, app := range apps {
		options = append(options, fmt.Sprintf("%s (%s)", app.DisplayName, app.AppID))
	}

	selected, err := s.Prompter.GetListInput(options, "Select Azure AD Application")
	if err != nil {
		return "", err
	}

	if selected == "Create new Azure AD application" {
		// Set the subscription context
		s.Output.LogInfo("Setting subscription context...")
		_, err = s.CmdRunner.RunCommand(ctx, "az", "account", "set",
			"--subscription", subscriptionID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to set subscription context: %w", err)
		}

		return s.createAzureApp(ctx, envName, subscriptionID, resourceGroup)
	}

	// Extract appId from selected display string
	for _, app := range apps {
		displayName := fmt.Sprintf("%s (%s)", app.DisplayName, app.AppID)
		if selected == displayName {
			return app.AppID, nil
		}
	}

	return "", fmt.Errorf("selected application not found")
}

// promptForEKSCluster prompts the user to select an EKS cluster.
func (s *OIDCSetup) promptForEKSCluster(ctx context.Context, region string) (string, error) {
	s.Output.LogInfo("Fetching EKS clusters...")
	clustersJSON, err := s.CmdRunner.RunCommand(ctx, "aws", "eks", "list-clusters",
		"--region", region,
		"--output", "json",
	)
	if err != nil {
		return "", fmt.Errorf("failed to list EKS clusters: %w", err)
	}

	type eksClusterList struct {
		Clusters []string `json:"clusters"`
	}

	var clusters eksClusterList
	if err := json.Unmarshal([]byte(clustersJSON), &clusters); err != nil {
		return "", fmt.Errorf("failed to parse EKS cluster list: %w", err)
	}

	if len(clusters.Clusters) == 0 {
		// Allow manual entry if no clusters found
		return s.Prompter.GetTextInput("EKS Cluster name", prompt.TextInputOptions{})
	}

	selected, err := s.Prompter.GetListInput(clusters.Clusters, "Select EKS Cluster")
	if err != nil {
		return "", err
	}

	return selected, nil
}

// promptForAKSCluster prompts the user to select an AKS cluster.
func (s *OIDCSetup) promptForAKSCluster(ctx context.Context, subscriptionID, resourceGroupName string) (string, error) {
	s.Output.LogInfo("Fetching AKS clusters...")
	clustersJSON, err := s.CmdRunner.RunCommand(ctx, "az", "aks", "list",
		"--resource-group", resourceGroupName,
		"--subscription", subscriptionID,
		"--output", "json",
	)
	if err != nil {
		return "", fmt.Errorf("failed to list AKS clusters: %w", err)
	}

	type aksCluster struct {
		Name string `json:"name"`
	}

	var clusters []aksCluster
	if err := json.Unmarshal([]byte(clustersJSON), &clusters); err != nil {
		return "", fmt.Errorf("failed to parse AKS cluster list: %w", err)
	}

	if len(clusters) == 0 {
		return s.Prompter.GetTextInput("AKS Cluster name", prompt.TextInputOptions{})
	}

	options := make([]string, len(clusters))
	for i, c := range clusters {
		options[i] = c.Name
	}

	selected, err := s.Prompter.GetListInput(options, "Select AKS Cluster")
	if err != nil {
		return "", err
	}

	return selected, nil
}

// Helper functions

func extractJSONField(jsonStr, field string) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return ""
	}
	if val, ok := data[field]; ok {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

func sanitizeStorageAccountName(name string) string {
	// Storage account names: 3-24 chars, lowercase letters and numbers only
	result := strings.ToLower(name)
	var sanitized strings.Builder
	for _, c := range result {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			sanitized.WriteRune(c)
		}
	}
	s := sanitized.String()
	if len(s) > 24 {
		s = s[:24]
	}
	return s
}
