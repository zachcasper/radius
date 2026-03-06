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

package providers

import (
	"context"
	"errors"
	"os"

	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	KubernetesProviderName = "kubernetes"
)

var _ Provider = (*kubernetesProvider)(nil)

type kubernetesProvider struct{}

// BuildConfig generates the Terraform provider configuration for the Kubernetes provider.
//
// When RADIUS_TARGET_KUBECONFIG is set, the provider is configured to use that kubeconfig file
// so that Terraform recipes deploy Kubernetes resources to the external target cluster (e.g., AKS/EKS)
// instead of the local cluster where the Radius control plane is running.
//
// When not set, the provider uses in-cluster config (if available) or the default kubeconfig file.
func (p *kubernetesProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Check if a target kubeconfig is configured for external cluster deployment.
	if targetKubeconfigPath := os.Getenv(kubeutil.TargetKubeconfigEnvVar); targetKubeconfigPath != "" {
		logger.Info("Terraform kubernetes provider: using external target cluster kubeconfig",
			"kubeconfig", targetKubeconfigPath,
			"envVar", kubeutil.TargetKubeconfigEnvVar)
		return map[string]any{
			"config_path": targetKubeconfigPath,
		}, nil
	}

	logger.Info("Terraform kubernetes provider: RADIUS_TARGET_KUBECONFIG not set, using default cluster config")

	_, err := rest.InClusterConfig()
	if err != nil {
		// If in cluster config is not present, then use default kubeconfig file.
		if errors.Is(err, rest.ErrNotInCluster) {
			logger.Info("Terraform kubernetes provider: not in cluster, using local kubeconfig",
				"config_path", clientcmd.RecommendedHomeFile)
			return map[string]any{
				"config_path": clientcmd.RecommendedHomeFile,
			}, nil
		}

		return nil, err
	}

	logger.Info("Terraform kubernetes provider: using in-cluster config")
	// No additional config is needed if in cluster config is present.
	// https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs#in-cluster-config
	return nil, nil
}
