/*
Copyright 2023-2024 IONOS Cloud.

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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/taskservice"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/vmservice"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

// ProxmoxMachineReconciler reconciles a ProxmoxMachine object.
type ProxmoxMachineReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ProxmoxClient proxmox.Client
}

// AddProxmoxMachineReconciler adds the cluster controller to the provided manager.\
func AddProxmoxMachineReconciler(ctx context.Context, mgr ctrl.Manager, client capmox.Client, options controller.Options) error {
	reconciler := &ProxmoxMachineReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("proxmoxmachine-controller"),
		ProxmoxClient: client,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.ProxmoxMachine{}).
		WithOptions(options).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1alpha1.GroupVersion.WithKind(infrav1alpha1.ProxmoxMachineKind))),
		).
		Complete(reconciler)
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch

// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *ProxmoxMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)

	// Fetch the ProxmoxMachine instance.
	proxmoxMachine := &infrav1alpha1.ProxmoxMachine{}
	err := r.Get(ctx, req.NamespacedName, proxmoxMachine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, proxmoxMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("machine", klog.KObj(machine))

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		logger.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	if annotations.IsPaused(cluster, proxmoxMachine) {
		logger.Info("ProxmoxMachine or linked Cluster is marked as paused, not reconciling")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("cluster", klog.KObj(cluster))

	infraCluster, err := r.getInfraCluster(ctx, &logger, cluster, proxmoxMachine)
	if err != nil {
		return ctrl.Result{}, errors.Errorf("error getting infra provider cluster or control plane object: %v", err)
	}
	if infraCluster == nil {
		logger.Info("ProxmoxCluster is not ready yet")
		return ctrl.Result{}, nil
	}

	// Create the machine scope
	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Client:         r.Client,
		Cluster:        cluster,
		Machine:        machine,
		InfraCluster:   infraCluster,
		ProxmoxMachine: proxmoxMachine,
		IPAMHelper:     ipam.NewHelper(r.Client, infraCluster.ProxmoxCluster),
		Logger:         &logger,
	})
	if err != nil {
		logger.Error(err, "failed to create scope")
		return ctrl.Result{}, err
	}

	// Always close the scope when exiting this function, so we can persist any ProxmoxMachine changes.
	defer func() {
		if err := machineScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !proxmoxMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, machineScope)
	}

	return r.reconcileNormal(ctx, machineScope, infraCluster)
}

func (r *ProxmoxMachineReconciler) reconcileDelete(ctx context.Context, machineScope *scope.MachineScope) (ctrl.Result, error) {
	machineScope.Logger.Info("Handling deleted ProxmoxMachine")
	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")

	err := vmservice.DeleteVM(ctx, machineScope)
	if err != nil {
		return reconcile.Result{}, err
	}
	// VM is being deleted
	return reconcile.Result{RequeueAfter: infrav1alpha1.DefaultReconcilerRequeue}, nil
}

func (r *ProxmoxMachineReconciler) reconcileNormal(ctx context.Context, machineScope *scope.MachineScope, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	clusterScope.Logger.V(4).Info("Reconciling ProxmoxMachine")

	// If the ProxmoxMachine is in an error state, return early.
	if machineScope.HasFailed() {
		machineScope.Info("Error state detected, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	if !machineScope.Cluster.Status.InfrastructureReady {
		machineScope.Info("Cluster infrastructure is not ready yet")
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition, infrav1alpha1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data is available and populated.
	if machineScope.Machine.Spec.Bootstrap.DataSecretName == nil {
		machineScope.Info("Bootstrap data secret reference is not yet available")
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition, infrav1alpha1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// If the ProxmoxMachine doesn't have our finalizer, add it.
	if ctrlutil.AddFinalizer(machineScope.ProxmoxMachine, infrav1alpha1.MachineFinalizer) {
		// Register the finalizer after first read operation from Proxmox to avoid orphaning Proxmox resources on delete
		if err := machineScope.PatchObject(); err != nil {
			machineScope.Error(err, "unable to patch object")
			return ctrl.Result{}, err
		}
	}

	// find the vm
	// Get or create the VM.
	vm, err := vmservice.ReconcileVM(ctx, machineScope)
	if err != nil {
		if requeueErr := new(taskservice.RequeueError); errors.As(err, &requeueErr) {
			machineScope.Error(err, "Requeue requested")
			return reconcile.Result{RequeueAfter: requeueErr.RequeueAfter()}, nil
		}
		machineScope.Logger.Error(err, "error reconciling VM")
		return reconcile.Result{}, errors.Wrapf(err, "failed to reconcile VM")
	}
	machineScope.ProxmoxMachine.Status.VMStatus = vm.State

	// Do not proceed until the backend VM is marked ready.
	if vm.State != infrav1alpha1.VirtualMachineStateReady {
		machineScope.Logger.Info(
			"VM state is not reconciled",
			"expectedVMState", infrav1alpha1.VirtualMachineStateReady,
			"actualVMState", vm.State)
		return reconcile.Result{RequeueAfter: infrav1alpha1.DefaultReconcilerRequeue}, nil
	}

	// TODO, check if we need to add some labels to the machine.

	machineScope.SetReady()
	conditions.MarkTrue(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
	machineScope.Logger.Info("ProxmoxMachine is ready")

	return reconcile.Result{}, nil
}

func (r *ProxmoxMachineReconciler) getInfraCluster(ctx context.Context, logger *logr.Logger, cluster *clusterv1.Cluster, proxmoxMachine *infrav1alpha1.ProxmoxMachine) (*scope.ClusterScope, error) {
	var clusterScope *scope.ClusterScope
	var err error

	proxmoxCluster := &infrav1alpha1.ProxmoxCluster{}

	infraClusterName := client.ObjectKey{
		Namespace: proxmoxMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}

	if err := r.Client.Get(ctx, infraClusterName, proxmoxCluster); err != nil {
		// ProxmoxCluster is not ready
		return nil, nil //nolint:nilerr
	}

	// Create the cluster scope
	clusterScope, err = scope.NewClusterScope(scope.ClusterScopeParams{
		Client:         r.Client,
		Logger:         logger,
		Cluster:        cluster,
		ProxmoxCluster: proxmoxCluster,
		ControllerName: "proxmoxmachine",
		ProxmoxClient:  r.ProxmoxClient,
		IPAMHelper:     ipam.NewHelper(r.Client, proxmoxCluster),
	})
	if err != nil {
		return nil, err
	}

	return clusterScope, nil
}
