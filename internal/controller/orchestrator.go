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
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/IBM/ibm-user-management-operator/api/v1alpha1"
	"github.com/IBM/ibm-user-management-operator/internal/controller/utils"
	"github.com/IBM/ibm-user-management-operator/internal/resources"
	odlm "github.com/IBM/operand-deployment-lifecycle-manager/v4/api/v1alpha1"
)

// ReconciliationPhase represents different phases of reconciliation
type ReconciliationPhase string

const (
	PhasePrerequisites ReconciliationPhase = "prerequisites"
	PhaseOperands      ReconciliationPhase = "operands"
	PhaseConfiguration ReconciliationPhase = "configuration"
	PhaseUI            ReconciliationPhase = "ui"
)

// ReconciliationContext holds the context and dependencies for reconciliation
type ReconciliationContext struct {
	Client          client.Client
	Scheme          *runtime.Scheme
	Config          *rest.Config
	Recorder        record.EventRecorder
	Instance        *operatorv1alpha1.AccountIAM
	BootstrapData   *BootstrapSecret
	IntegrationData *IntegrationConfig
	ResourceManager *ResourceManager
	StatusChecker   *utils.ResourceStatusChecker
}

// NewReconciliationContext creates a new reconciliation context
func NewReconciliationContext(
	client client.Client,
	scheme *runtime.Scheme,
	config *rest.Config,
	recorder record.EventRecorder,
	instance *operatorv1alpha1.AccountIAM,
) *ReconciliationContext {
	resourceManager := NewResourceManager(client, scheme, instance)
	statusChecker := utils.NewResourceStatusChecker(client)

	return &ReconciliationContext{
		Client:          client,
		Scheme:          scheme,
		Config:          config,
		Recorder:        recorder,
		Instance:        instance,
		ResourceManager: resourceManager,
		StatusChecker:   statusChecker,
	}
}

// PhaseExecutor defines the interface for executing reconciliation phases
type PhaseExecutor interface {
	Execute(ctx context.Context, reconcilationCtx *ReconciliationContext) error
	GetPhase() ReconciliationPhase
}

// PrerequisitesPhase handles the prerequisites phase
type PrerequisitesPhase struct{}

func (p *PrerequisitesPhase) GetPhase() ReconciliationPhase {
	return PhasePrerequisites
}

func (p *PrerequisitesPhase) Execute(ctx context.Context, reconcilationCtx *ReconciliationContext) error {
	klog.Infof("Executing prerequisites phase")

	// Create the original reconciler temporarily to reuse existing logic
	reconciler := &AccountIAMReconciler{
		Client:   reconcilationCtx.Client,
		Scheme:   reconcilationCtx.Scheme,
		Config:   reconcilationCtx.Config,
		Recorder: reconcilationCtx.Recorder,
	}

	return reconciler.verifyPrereq(ctx, reconcilationCtx.Instance)
}

// OperandsPhase handles the operands phase
type OperandsPhase struct{}

func (o *OperandsPhase) GetPhase() ReconciliationPhase {
	return PhaseOperands
}

func (o *OperandsPhase) Execute(ctx context.Context, reconcilationCtx *ReconciliationContext) error {
	klog.Infof("Executing operands phase")

	// Create the original reconciler temporarily to reuse existing logic
	reconciler := &AccountIAMReconciler{
		Client:   reconcilationCtx.Client,
		Scheme:   reconcilationCtx.Scheme,
		Config:   reconcilationCtx.Config,
		Recorder: reconcilationCtx.Recorder,
	}

	return reconciler.reconcileOperandResources(ctx, reconcilationCtx.Instance)
}

// ConfigurationPhase handles the configuration phase
type ConfigurationPhase struct{}

func (c *ConfigurationPhase) GetPhase() ReconciliationPhase {
	return PhaseConfiguration
}

func (c *ConfigurationPhase) Execute(ctx context.Context, reconcilationCtx *ReconciliationContext) error {
	klog.Infof("Executing configuration phase")

	// Create the original reconciler temporarily to reuse existing logic
	reconciler := &AccountIAMReconciler{
		Client:   reconcilationCtx.Client,
		Scheme:   reconcilationCtx.Scheme,
		Config:   reconcilationCtx.Config,
		Recorder: reconcilationCtx.Recorder,
	}

	return reconciler.configIM(ctx, reconcilationCtx.Instance)
}

// UIPhase handles the UI phase
type UIPhase struct{}

func (u *UIPhase) GetPhase() ReconciliationPhase {
	return PhaseUI
}

func (u *UIPhase) Execute(ctx context.Context, reconcilationCtx *ReconciliationContext) error {
	klog.Infof("Executing UI phase")

	// Create the original reconciler temporarily to reuse existing logic
	reconciler := &AccountIAMReconciler{
		Client:   reconcilationCtx.Client,
		Scheme:   reconcilationCtx.Scheme,
		Config:   reconcilationCtx.Config,
		Recorder: reconcilationCtx.Recorder,
	}

	return reconciler.reconcileUI(ctx, reconcilationCtx.Instance)
}

// PhaseOrchestrator manages the execution of reconciliation phases
type PhaseOrchestrator struct {
	phases []PhaseExecutor
}

// NewPhaseOrchestrator creates a new phase orchestrator with default phases
func NewPhaseOrchestrator() *PhaseOrchestrator {
	return &PhaseOrchestrator{
		phases: []PhaseExecutor{
			&PrerequisitesPhase{},
			&OperandsPhase{},
			&ConfigurationPhase{},
			&UIPhase{},
		},
	}
}

// ExecutePhases executes all reconciliation phases in order
func (po *PhaseOrchestrator) ExecutePhases(ctx context.Context, reconcilationCtx *ReconciliationContext) error {
	for _, phase := range po.phases {
		startTime := time.Now()
		klog.Infof("Starting phase: %s", phase.GetPhase())

		if err := phase.Execute(ctx, reconcilationCtx); err != nil {
			klog.Errorf("Failed to execute phase %s: %v", phase.GetPhase(), err)
			return err
		}

		duration := time.Since(startTime)
		klog.Infof("Completed phase %s in %v", phase.GetPhase(), duration)
	}

	return nil
}

// StatusUpdater handles status updates with retry logic
type StatusUpdater struct {
	client     client.Client
	maxRetries int
	retryDelay time.Duration
}

// NewStatusUpdater creates a new status updater
func NewStatusUpdater(client client.Client) *StatusUpdater {
	return &StatusUpdater{
		client:     client,
		maxRetries: 3,
		retryDelay: time.Second,
	}
}

// UpdateStatus updates the AccountIAM status with retry logic
func (su *StatusUpdater) UpdateStatus(ctx context.Context, instance *operatorv1alpha1.AccountIAM,
	statusChecker *utils.ResourceStatusChecker) error {

	// Get standard resource batches
	batches := utils.GetStandardResourceBatches(instance.Namespace)

	// Check all resource statuses
	batchResults, allReady := statusChecker.CheckResourceBatches(ctx, batches)

	// Flatten all resource statuses
	var managedResources []odlm.ResourceStatus
	for _, statuses := range batchResults {
		managedResources = append(managedResources, statuses...)
	}

	// Update instance status
	accountIAMService := odlm.OperandStatus{
		ObjectName: instance.Name,
		Kind:       resources.UserMgmtCR,
		APIVersion: resources.OperatorIBMApiVersion,
		Namespace:  instance.Namespace,
		Status:     resources.PhaseRunning,
	}

	if !allReady {
		accountIAMService.Status = resources.StatusNotReady
	}

	accountIAMService.ManagedResources = managedResources
	instance.Status.Service = accountIAMService

	// Update with retry logic
	var updateErr error
	for i := 0; i < su.maxRetries; i++ {
		updateErr = su.client.Status().Update(ctx, instance)
		if updateErr == nil {
			klog.Infof("Successfully updated AccountIAM status after %d attempts", i+1)
			return nil
		}

		klog.Errorf("Failed to update AccountIAM status (attempt %d/%d): %v", i+1, su.maxRetries, updateErr)
		if i < su.maxRetries-1 {
			time.Sleep(su.retryDelay)
		}
	}

	return updateErr
}
