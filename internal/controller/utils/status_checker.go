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

package utils

import (
	"context"

	"github.com/IBM/ibm-user-management-operator/internal/resources"
	odlm "github.com/IBM/operand-deployment-lifecycle-manager/v4/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceStatusChecker provides a unified interface for checking resource status
type ResourceStatusChecker struct {
	client client.Client
}

// NewResourceStatusChecker creates a new ResourceStatusChecker
func NewResourceStatusChecker(client client.Client) *ResourceStatusChecker {
	return &ResourceStatusChecker{client: client}
}

// ResourceType represents the type of Kubernetes resource
type ResourceType string

const (
	ResourceTypeRedis          ResourceType = "redis"
	ResourceTypeOperandRequest ResourceType = "operandrequest"
	ResourceTypeJob            ResourceType = "job"
	ResourceTypeService        ResourceType = "service"
	ResourceTypeSecret         ResourceType = "secret"
	ResourceTypeRoute          ResourceType = "route"
	ResourceTypeDeployment     ResourceType = "deployment"
)

// ResourceInfo contains information about a resource to check
type ResourceInfo struct {
	Name      string
	Namespace string
	Type      ResourceType
}

// CheckResourceStatus checks the status of a single resource
func (rsc *ResourceStatusChecker) CheckResourceStatus(ctx context.Context, info ResourceInfo) (odlm.ResourceStatus, bool) {
	switch info.Type {
	case ResourceTypeRedis:
		return rsc.checkRedisStatus(ctx, info.Namespace)
	case ResourceTypeOperandRequest:
		return rsc.checkOperandRequestStatus(ctx, info.Namespace)
	case ResourceTypeJob:
		return rsc.checkJobStatus(ctx, info.Name, info.Namespace)
	case ResourceTypeService:
		return rsc.checkServiceStatus(ctx, info.Name, info.Namespace)
	case ResourceTypeSecret:
		return rsc.checkSecretStatus(ctx, info.Name, info.Namespace)
	case ResourceTypeRoute:
		return rsc.checkRouteStatus(ctx, info.Name, info.Namespace)
	case ResourceTypeDeployment:
		return rsc.checkDeploymentStatus(ctx, info.Name, info.Namespace)
	default:
		klog.Errorf("Unknown resource type: %s", info.Type)
		return odlm.ResourceStatus{
			ObjectName: info.Name,
			Kind:       string(info.Type),
			APIVersion: "unknown",
			Namespace:  info.Namespace,
			Status:     resources.StatusNotReady,
		}, false
	}
}

// CheckMultipleResources checks the status of multiple resources efficiently
func (rsc *ResourceStatusChecker) CheckMultipleResources(ctx context.Context, resourceInfos []ResourceInfo) ([]odlm.ResourceStatus, bool) {
	var statuses []odlm.ResourceStatus
	allReady := true

	for _, info := range resourceInfos {
		status, ready := rsc.CheckResourceStatus(ctx, info)
		statuses = append(statuses, status)
		if !ready {
			allReady = false
		}
	}

	return statuses, allReady
}

// checkRedisStatus checks Redis resource status
func (rsc *ResourceStatusChecker) checkRedisStatus(ctx context.Context, namespace string) (odlm.ResourceStatus, bool) {
	return GetRedisResourceStatus(ctx, rsc.client, namespace)
}

// checkOperandRequestStatus checks OperandRequest status
func (rsc *ResourceStatusChecker) checkOperandRequestStatus(ctx context.Context, namespace string) (odlm.ResourceStatus, bool) {
	return GetOperandRequestStatus(ctx, rsc.client, namespace)
}

// checkJobStatus checks Job status
func (rsc *ResourceStatusChecker) checkJobStatus(ctx context.Context, jobName, namespace string) (odlm.ResourceStatus, bool) {
	return GetJobStatus(ctx, rsc.client, jobName, namespace)
}

// checkServiceStatus checks Service status
func (rsc *ResourceStatusChecker) checkServiceStatus(ctx context.Context, serviceName, namespace string) (odlm.ResourceStatus, bool) {
	return GetServiceStatus(ctx, rsc.client, serviceName, namespace)
}

// checkSecretStatus checks Secret status
func (rsc *ResourceStatusChecker) checkSecretStatus(ctx context.Context, secretName, namespace string) (odlm.ResourceStatus, bool) {
	return GetSecretStatus(ctx, rsc.client, secretName, namespace)
}

// checkRouteStatus checks Route status
func (rsc *ResourceStatusChecker) checkRouteStatus(ctx context.Context, routeName, namespace string) (odlm.ResourceStatus, bool) {
	return GetRouteStatus(ctx, rsc.client, routeName, namespace)
}

// checkDeploymentStatus checks Deployment status
func (rsc *ResourceStatusChecker) checkDeploymentStatus(ctx context.Context, deploymentName, namespace string) (odlm.ResourceStatus, bool) {
	deployment := &appsv1.Deployment{}
	if err := rsc.client.Get(ctx, types.NamespacedName{
		Name:      deploymentName,
		Namespace: namespace,
	}, deployment); err != nil {
		if k8serrors.IsNotFound(err) {
			return odlm.ResourceStatus{
				ObjectName: deploymentName,
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				Namespace:  namespace,
				Status:     resources.StatusNotReady,
			}, false
		}
		klog.Errorf("Failed to get Deployment %s in namespace %s: %v", deploymentName, namespace, err)
		return odlm.ResourceStatus{
			ObjectName: deploymentName,
			Kind:       "Deployment",
			APIVersion: "apps/v1",
			Namespace:  namespace,
			Status:     resources.StatusNotReady,
		}, false
	}

	ready := deployment.Status.ReadyReplicas == deployment.Status.Replicas &&
		deployment.Status.Replicas > 0

	status := resources.PhaseRunning
	if !ready {
		status = resources.StatusNotReady
	}

	return odlm.ResourceStatus{
		ObjectName: deploymentName,
		Kind:       "Deployment",
		APIVersion: "apps/v1",
		Namespace:  namespace,
		Status:     status,
	}, ready
}

// ResourceBatch represents a batch of resources to check
type ResourceBatch struct {
	Name      string
	Resources []ResourceInfo
}

// CheckResourceBatches checks multiple batches of resources
func (rsc *ResourceStatusChecker) CheckResourceBatches(ctx context.Context, batches []ResourceBatch) (map[string][]odlm.ResourceStatus, bool) {
	result := make(map[string][]odlm.ResourceStatus)
	allBatchesReady := true

	for _, batch := range batches {
		statuses, ready := rsc.CheckMultipleResources(ctx, batch.Resources)
		result[batch.Name] = statuses
		if !ready {
			allBatchesReady = false
		}
	}

	return result, allBatchesReady
}

// GetStandardResourceBatches returns standard resource batches for AccountIAM
func GetStandardResourceBatches(namespace string) []ResourceBatch {
	return []ResourceBatch{
		{
			Name: "infrastructure",
			Resources: []ResourceInfo{
				{Name: "", Namespace: namespace, Type: ResourceTypeRedis},
				{Name: "", Namespace: namespace, Type: ResourceTypeOperandRequest},
			},
		},
		{
			Name: "jobs",
			Resources: []ResourceInfo{
				{Name: resources.CreateDBJob, Namespace: namespace, Type: ResourceTypeJob},
				{Name: resources.DBMigrationJob, Namespace: namespace, Type: ResourceTypeJob},
				{Name: resources.IMConfigJob, Namespace: namespace, Type: ResourceTypeJob},
			},
		},
		{
			Name: "services",
			Resources: []ResourceInfo{
				{Name: resources.AccountIAM, Namespace: namespace, Type: ResourceTypeService},
				{Name: resources.AccountIAMUIService, Namespace: namespace, Type: ResourceTypeService},
				{Name: resources.AccountIAMUIAPIService, Namespace: namespace, Type: ResourceTypeService},
			},
		},
		{
			Name: "secrets",
			Resources: []ResourceInfo{
				{Name: resources.BootstrapSecret, Namespace: namespace, Type: ResourceTypeSecret},
				{Name: resources.AccountIAMDBSecret, Namespace: namespace, Type: ResourceTypeSecret},
				{Name: resources.AccountIAMConfigSecret, Namespace: namespace, Type: ResourceTypeSecret},
				{Name: resources.AccountIAMOidcClientAuth, Namespace: namespace, Type: ResourceTypeSecret},
				{Name: resources.AccountIAMOKDAuth, Namespace: namespace, Type: ResourceTypeSecret},
				{Name: resources.AccountIAMUISecrets, Namespace: namespace, Type: ResourceTypeSecret},
				{Name: resources.IMOIDCCrendential, Namespace: namespace, Type: ResourceTypeSecret},
				{Name: resources.IMAPISecret, Namespace: namespace, Type: ResourceTypeSecret},
				{Name: resources.AccountIAMCACert, Namespace: namespace, Type: ResourceTypeSecret},
			},
		},
		{
			Name: "routes",
			Resources: []ResourceInfo{
				{Name: resources.AccountIAM, Namespace: namespace, Type: ResourceTypeRoute},
				{Name: resources.AccountIAMUIRoute, Namespace: namespace, Type: ResourceTypeRoute},
				{Name: resources.AccountIAMUIAPIInstance, Namespace: namespace, Type: ResourceTypeRoute},
			},
		},
	}
}
