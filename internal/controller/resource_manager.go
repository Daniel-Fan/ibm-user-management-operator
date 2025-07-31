/*
Copyright 2024.

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

package controller

import (
	"bytes"
	"context"
	"text/template"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1alpha1 "github.com/IBM/ibm-user-management-operator/api/v1alpha1"
	"github.com/IBM/ibm-user-management-operator/internal/controller/utils"
	"github.com/IBM/ibm-user-management-operator/internal/resources/images"
)

// ResourceManager provides centralized resource management operations
type ResourceManager struct {
	client   client.Client
	scheme   *runtime.Scheme
	instance *operatorv1alpha1.AccountIAM
}

// NewResourceManager creates a new ResourceManager instance
func NewResourceManager(client client.Client, scheme *runtime.Scheme, instance *operatorv1alpha1.AccountIAM) *ResourceManager {
	return &ResourceManager{
		client:   client,
		scheme:   scheme,
		instance: instance,
	}
}

// ManifestProcessor handles common manifest processing operations
type ManifestProcessor struct {
	resourceManager *ResourceManager
}

// NewManifestProcessor creates a new ManifestProcessor
func NewManifestProcessor(rm *ResourceManager) *ManifestProcessor {
	return &ManifestProcessor{resourceManager: rm}
}

// ProcessStaticManifests processes a list of static YAML manifests
func (mp *ManifestProcessor) ProcessStaticManifests(ctx context.Context, manifests []string) error {
	for _, manifest := range manifests {
		if err := mp.processStaticManifest(ctx, manifest); err != nil {
			return err
		}
	}
	return nil
}

// ProcessTemplateManifests processes manifests with template data injection
func (mp *ManifestProcessor) ProcessTemplateManifests(ctx context.Context, manifests []string, dataList ...interface{}) error {
	combinedData := utils.CombineData(dataList...)

	for _, manifest := range manifests {
		if err := mp.processTemplateManifest(ctx, manifest, combinedData); err != nil {
			return err
		}
	}
	return nil
}

// processStaticManifest processes a single static manifest
func (mp *ManifestProcessor) processStaticManifest(ctx context.Context, manifest string) error {
	object := &unstructured.Unstructured{}

	// Replace image references if present
	if images.ContainsImageReferences(manifest) {
		manifest = images.ReplaceInYAML(manifest)
	}

	// Unmarshal YAML
	if err := yaml.Unmarshal([]byte(manifest), object); err != nil {
		return err
	}

	// Set namespace and controller reference
	object.SetNamespace(mp.resourceManager.instance.Namespace)
	if err := controllerutil.SetControllerReference(mp.resourceManager.instance, object, mp.resourceManager.scheme); err != nil {
		return err
	}

	// Create or update the resource
	return mp.resourceManager.createOrUpdate(ctx, object)
}

// processTemplateManifest processes a single template manifest with data injection
func (mp *ManifestProcessor) processTemplateManifest(ctx context.Context, manifest string, data interface{}) error {
	object := &unstructured.Unstructured{}
	var buffer bytes.Buffer

	// Replace image references if present
	if images.ContainsImageReferences(manifest) {
		manifest = images.ReplaceInYAML(manifest)
	}

	// Execute template
	tmpl := template.Must(template.New("resource-template").Parse(manifest))
	if err := tmpl.Execute(&buffer, data); err != nil {
		return err
	}

	// Unmarshal processed YAML
	if err := yaml.Unmarshal(buffer.Bytes(), object); err != nil {
		return err
	}

	// Set namespace and controller reference
	object.SetNamespace(mp.resourceManager.instance.Namespace)
	if err := controllerutil.SetControllerReference(mp.resourceManager.instance, object, mp.resourceManager.scheme); err != nil {
		return err
	}

	// Create or update the resource
	return mp.resourceManager.createOrUpdate(ctx, object)
}

// createOrUpdate handles the create or update logic for a resource
func (rm *ResourceManager) createOrUpdate(ctx context.Context, obj *unstructured.Unstructured) error {
	// Use the existing createOrUpdate logic from the main controller
	// This maintains the same behavior while centralizing the logic
	reconciler := &AccountIAMReconciler{
		Client: rm.client,
		Scheme: rm.scheme,
	}
	return reconciler.createOrUpdate(ctx, obj)
}

// BatchProcessor handles batch operations for multiple resources
type BatchProcessor struct {
	manifestProcessor *ManifestProcessor
	maxConcurrency    int
}

// NewBatchProcessor creates a new BatchProcessor with controlled concurrency
func NewBatchProcessor(mp *ManifestProcessor, maxConcurrency int) *BatchProcessor {
	if maxConcurrency <= 0 {
		maxConcurrency = 1 // Default to sequential processing
	}
	return &BatchProcessor{
		manifestProcessor: mp,
		maxConcurrency:    maxConcurrency,
	}
}

// ProcessManifestsBatch processes manifests in controlled batches to improve performance
func (bp *BatchProcessor) ProcessManifestsBatch(ctx context.Context, manifests []string, templateData ...interface{}) error {
	// For now, process sequentially to maintain existing behavior
	// In the future, this can be optimized for concurrent processing

	if len(templateData) > 0 {
		return bp.manifestProcessor.ProcessTemplateManifests(ctx, manifests, templateData...)
	}
	return bp.manifestProcessor.ProcessStaticManifests(ctx, manifests)
}

// ResourceValidator provides validation for resources before creation/update
type ResourceValidator struct {
	client client.Client
}

// NewResourceValidator creates a new ResourceValidator
func NewResourceValidator(client client.Client) *ResourceValidator {
	return &ResourceValidator{client: client}
}

// ValidateResource performs basic validation on a resource before processing
func (rv *ResourceValidator) ValidateResource(obj *unstructured.Unstructured) error {
	// Basic validation - can be extended based on requirements
	if obj.GetName() == "" {
		klog.Warningf("Resource has no name, skipping validation")
	}

	if obj.GetNamespace() == "" {
		klog.V(2).Infof("Resource %s has no namespace - this may be a cluster-scoped resource", obj.GetName())
	}

	return nil
}
