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

package kubernetes

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ManifestCapture captures Kubernetes manifests for drift detection.
type ManifestCapture struct {
	// namespace is the default namespace for resources.
	namespace string

	// kubeContext is the Kubernetes context.
	kubeContext string
}

// NewManifestCapture creates a new ManifestCapture.
func NewManifestCapture(kubeContext, namespace string) (*ManifestCapture, error) {
	return &ManifestCapture{
		kubeContext: kubeContext,
		namespace:   namespace,
	}, nil
}

// GetRawManifest retrieves the raw manifest for a Kubernetes resource as a YAML string.
// This uses kubectl to fetch the resource directly from the cluster.
func (c *ManifestCapture) GetRawManifest(ctx context.Context, kind, name, namespace string) (string, error) {
	if namespace == "" {
		namespace = c.namespace
	}

	// Build kubectl command to get resource as YAML
	args := []string{"get", kind, name, "-n", namespace, "-o", "yaml"}
	if c.kubeContext != "" {
		args = append([]string{"--context", c.kubeContext}, args...)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get resource %s/%s: %w", kind, name, err)
	}

	return string(output), nil
}

// ParseResourceID parses a resource ID in the format "namespace/kind/name" and returns its components.
func ParseResourceID(resourceID string) (namespace, kind, name string, err error) {
	parts := strings.Split(resourceID, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid resource ID format: %s (expected namespace/kind/name)", resourceID)
	}
	return parts[0], parts[1], parts[2], nil
}
