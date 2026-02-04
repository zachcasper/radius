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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	"github.com/radius-project/radius/pkg/cli/git/deploy"
)

// ManifestCapture captures Kubernetes manifests for drift detection.
type ManifestCapture struct {
	// clientset is the Kubernetes clientset.
	clientset kubernetes.Interface

	// dynamicClient is the dynamic client for unstructured resources.
	dynamicClient dynamic.Interface

	// namespace is the default namespace for resources.
	namespace string
}

// NewManifestCapture creates a new ManifestCapture.
func NewManifestCapture(kubeContext, namespace string) (*ManifestCapture, error) {
	// Load kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: kubeContext,
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &ManifestCapture{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		namespace:     namespace,
	}, nil
}

// CaptureResource captures a single Kubernetes resource manifest.
func (c *ManifestCapture) CaptureResource(ctx context.Context, kind, name, namespace string) (*deploy.CapturedResource, error) {
	if namespace == "" {
		namespace = c.namespace
	}

	gvr, err := c.getGVR(kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get GVR for kind %s: %w", kind, err)
	}

	// Get the resource
	var obj *unstructured.Unstructured
	if namespace != "" {
		obj, err = c.dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = c.dynamicClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get resource %s/%s: %w", kind, name, err)
	}

	// Strip managed fields and other metadata
	c.stripMetadata(obj)

	// Store as structured object (will be serialized as JSON in deployment record)
	return &deploy.CapturedResource{
		ResourceID:   fmt.Sprintf("%s/%s/%s", namespace, kind, name),
		ResourceType: kind,
		Provider:     "kubernetes",
		Name:         name,
		Namespace:    namespace,
		RawManifest:  obj.Object,
	}, nil
}

// CaptureFromTerraformState captures manifests for all Kubernetes resources in terraform state.
// This function is deprecated - use the executor's direct capture instead.
func (c *ManifestCapture) CaptureFromTerraformState(ctx context.Context, resourceType string, sequence int, resources []KubernetesResource) ([]deploy.CapturedResource, error) {
	var captured []deploy.CapturedResource

	for _, resource := range resources {
		result, err := c.CaptureResource(ctx, resource.Kind, resource.Name, resource.Namespace)
		if err != nil {
			// Log warning but continue
			continue
		}
		result.RadiusResourceType = resourceType
		result.DeploymentStep = sequence
		captured = append(captured, *result)
	}

	return captured, nil
}

// KubernetesResource represents a Kubernetes resource to capture.
type KubernetesResource struct {
	Kind      string
	Name      string
	Namespace string
}

// getGVR returns the GroupVersionResource for a given kind.
func (c *ManifestCapture) getGVR(kind string) (schema.GroupVersionResource, error) {
	// Common Kubernetes resource mappings
	mappings := map[string]schema.GroupVersionResource{
		"Deployment":        {Group: "apps", Version: "v1", Resource: "deployments"},
		"StatefulSet":       {Group: "apps", Version: "v1", Resource: "statefulsets"},
		"DaemonSet":         {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"ReplicaSet":        {Group: "apps", Version: "v1", Resource: "replicasets"},
		"Service":           {Group: "", Version: "v1", Resource: "services"},
		"ConfigMap":         {Group: "", Version: "v1", Resource: "configmaps"},
		"Secret":            {Group: "", Version: "v1", Resource: "secrets"},
		"PersistentVolumeClaim": {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		"ServiceAccount":    {Group: "", Version: "v1", Resource: "serviceaccounts"},
		"Ingress":           {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		"NetworkPolicy":     {Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
		"Job":               {Group: "batch", Version: "v1", Resource: "jobs"},
		"CronJob":           {Group: "batch", Version: "v1", Resource: "cronjobs"},
	}

	gvr, ok := mappings[kind]
	if !ok {
		return schema.GroupVersionResource{}, fmt.Errorf("unknown resource kind: %s", kind)
	}

	return gvr, nil
}

// stripMetadata removes metadata that would cause false diff results.
func (c *ManifestCapture) stripMetadata(obj *unstructured.Unstructured) {
	// Remove managed fields
	obj.SetManagedFields(nil)

	// Remove resource version, UID, etc.
	metadata := obj.Object["metadata"].(map[string]interface{})
	delete(metadata, "resourceVersion")
	delete(metadata, "uid")
	delete(metadata, "creationTimestamp")
	delete(metadata, "generation")
	delete(metadata, "selfLink")

	// Remove status section
	delete(obj.Object, "status")

	// Strip annotations that change frequently
	annotations := obj.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		delete(annotations, "deployment.kubernetes.io/revision")
		obj.SetAnnotations(annotations)
	}

	// Strip secret data for security
	if obj.GetKind() == "Secret" {
		delete(obj.Object, "data")
	}
}

// parseKubernetesResourceType converts terraform resource type to Kubernetes kind.
func parseKubernetesResourceType(resourceType string) string {
	// Terraform kubernetes provider resource types
	mappings := map[string]string{
		"kubernetes_deployment":              "Deployment",
		"kubernetes_deployment_v1":           "Deployment",
		"kubernetes_stateful_set":            "StatefulSet",
		"kubernetes_stateful_set_v1":         "StatefulSet",
		"kubernetes_daemon_set":              "DaemonSet",
		"kubernetes_daemon_set_v1":           "DaemonSet",
		"kubernetes_service":                 "Service",
		"kubernetes_service_v1":              "Service",
		"kubernetes_config_map":              "ConfigMap",
		"kubernetes_config_map_v1":           "ConfigMap",
		"kubernetes_secret":                  "Secret",
		"kubernetes_secret_v1":               "Secret",
		"kubernetes_persistent_volume_claim": "PersistentVolumeClaim",
		"kubernetes_persistent_volume_claim_v1": "PersistentVolumeClaim",
		"kubernetes_service_account":         "ServiceAccount",
		"kubernetes_service_account_v1":      "ServiceAccount",
		"kubernetes_ingress":                 "Ingress",
		"kubernetes_ingress_v1":              "Ingress",
		"kubernetes_job":                     "Job",
		"kubernetes_job_v1":                  "Job",
		"kubernetes_cron_job":                "CronJob",
		"kubernetes_cron_job_v1":             "CronJob",
	}

	if kind, ok := mappings[resourceType]; ok {
		return kind
	}
	return ""
}

// GetRawManifest retrieves the raw manifest for a Kubernetes resource as a YAML string.
func (c *ManifestCapture) GetRawManifest(ctx context.Context, kind, name, namespace string) (string, error) {
	captured, err := c.CaptureResource(ctx, kind, name, namespace)
	if err != nil {
		return "", err
	}
	// Convert structured object to YAML string
	if obj, ok := captured.RawManifest.(map[string]any); ok {
		yamlBytes, err := yaml.Marshal(obj)
		if err != nil {
			return "", fmt.Errorf("failed to marshal manifest to YAML: %w", err)
		}
		return string(yamlBytes), nil
	}
	// If already a string, return as-is
	if str, ok := captured.RawManifest.(string); ok {
		return str, nil
	}
	return "", fmt.Errorf("unexpected manifest type: %T", captured.RawManifest)
}
