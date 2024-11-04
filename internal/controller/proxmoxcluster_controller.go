/*
Copyright 2023 IONOS Cloud.

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

// Package controller implements controller types.
package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clustererrors "sigs.k8s.io/cluster-api/errors"
	clusterutil "sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

const (
	// ControlPlaneEndpointPort default API server port.
	ControlPlaneEndpointPort = 6443
)

// ProxmoxClusterReconciler reconciles a ProxmoxCluster object.
type ProxmoxClusterReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ProxmoxClient proxmox.Client
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProxmoxClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.ProxmoxCluster{}).
		WithEventFilter(predicates.ResourceNotPaused(ctrl.LoggerFrom(ctx))).
		Watches(&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterutil.ClusterToInfrastructureMapFunc(ctx, infrav1alpha1.GroupVersion.WithKind(infrav1alpha1.ProxmoxClusterKind), mgr.GetClient(), &infrav1alpha1.ProxmoxCluster{})),
			builder.WithPredicates(predicates.ClusterUnpaused(ctrl.LoggerFrom(ctx)))).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(ctrl.LoggerFrom(ctx))).
		Complete(r)
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters/finalizers,verbs=update

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *ProxmoxClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)

	proxmoxCluster := &infrav1alpha1.ProxmoxCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, proxmoxCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Get owner cluster
	cluster, err := clusterutil.GetOwnerCluster(ctx, r.Client, proxmoxCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		logger.Info("Waiting for Cluster Controller to set OwnerRef on ProxmoxCluster")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, logger)

	if annotations.IsPaused(cluster, proxmoxCluster) {
		logger.Info("ProxmoxCluster or owning Cluster is marked as paused, not reconciling")

		return ctrl.Result{}, nil
	}

	// Create the scope.
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Client:         r.Client,
		Logger:         &logger,
		Cluster:        cluster,
		ProxmoxCluster: proxmoxCluster,
		ControllerName: "proxmoxcluster",
		ProxmoxClient:  r.ProxmoxClient,
		IPAMHelper:     ipam.NewHelper(r.Client, proxmoxCluster.DeepCopy()),
	})

	if err != nil {
		return reconcile.Result{}, errors.Errorf("failed to create scope: %+v", err)
	}

	// Always close the scope when exiting this function so we can persist any ProxmoxCluster changes.
	defer func() {
		if err := clusterScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Handle deleted clusters
	if !proxmoxCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, clusterScope)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, clusterScope)
}

func (r *ProxmoxClusterReconciler) reconcileDelete(ctx context.Context, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	// We want to prevent deletion unless the owning cluster was flagged for deletion.
	if clusterScope.Cluster.DeletionTimestamp.IsZero() {
		clusterScope.Error(errors.New("deletion was requested but owning cluster wasn't deleted"), "Unable to delete ProxmoxCluster")
		// We stop reconciling here. It will be triggered again once the owning cluster was deleted.
		return reconcile.Result{}, nil
	}

	clusterScope.Logger.V(4).Info("Reconciling ProxmoxCluster delete")
	// Deletion usually should be triggered through the deletion of the owning cluster.
	// If the ProxmoxCluster was also flagged for deletion (e.g. deletion using the manifest file)
	// we should only allow to remove the finalizer when there are no ProxmoxMachines left.
	machines, err := r.listProxmoxMachinesForCluster(ctx, clusterScope)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "could not retrieve proxmox machines for cluster %q", clusterScope.InfraClusterName())
	}

	// Requeue if there are one or more machines left.
	if len(machines) > 0 {
		clusterScope.Info("waiting for machines to be deleted", "remaining", len(machines))
		return ctrl.Result{RequeueAfter: infrav1alpha1.DefaultReconcilerRequeue}, nil
	}

	if err := r.reconcileDeleteCredentialsSecret(ctx, clusterScope); err != nil {
		return reconcile.Result{}, err
	}

	clusterScope.Info("cluster deleted successfully")
	ctrlutil.RemoveFinalizer(clusterScope.ProxmoxCluster, infrav1alpha1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

func (r *ProxmoxClusterReconciler) reconcileNormal(ctx context.Context, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	clusterScope.Logger.Info("Reconciling ProxmoxCluster")

	// If the ProxmoxCluster doesn't have our finalizer, add it.
	ctrlutil.AddFinalizer(clusterScope.ProxmoxCluster, infrav1alpha1.ClusterFinalizer)

	if clusterScope.ProxmoxCluster.Spec.ExternalManagedControlPlane {
		if clusterScope.ProxmoxCluster.Spec.ControlPlaneEndpoint == nil {
			clusterScope.Logger.Info("ProxmoxCluster is not ready, missing or waiting for a ControlPlaneEndpoint")
	
			conditions.MarkFalse(clusterScope.ProxmoxCluster, infrav1alpha1.ProxmoxClusterReady, infrav1alpha1.MissingControlPlaneEndpointReason, clusterv1.ConditionSeverityWarning, "The ProxmoxCluster is missing or waiting for a ControlPlaneEndpoint")
	
			return ctrl.Result{Requeue: true}, nil
		}
		if clusterScope.ProxmoxCluster.Spec.ControlPlaneEndpoint.Host == "" {
			clusterScope.Logger.Info("ProxmoxCluster is not ready, missing or waiting for a ControlPlaneEndpoint host")
	
			conditions.MarkFalse(clusterScope.ProxmoxCluster, infrav1alpha1.ProxmoxClusterReady, infrav1alpha1.MissingControlPlaneEndpointReason, clusterv1.ConditionSeverityWarning, "The ProxmoxCluster is missing or waiting for a ControlPlaneEndpoint host")
	
			return ctrl.Result{Requeue: true}, nil
		}
		if clusterScope.ProxmoxCluster.Spec.ControlPlaneEndpoint.Port == 0 {
			clusterScope.Logger.Info("ProxmoxCluster is not ready, missing or waiting for a ControlPlaneEndpoint port")
	
			conditions.MarkFalse(clusterScope.ProxmoxCluster, infrav1alpha1.ProxmoxClusterReady, infrav1alpha1.MissingControlPlaneEndpointReason, clusterv1.ConditionSeverityWarning, "The ProxmoxCluster is missing or waiting for a ControlPlaneEndpoint port")
	
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// when a Cluster is marked failed cause the Proxmox client is nil.
	// the cluster doesn't reconcile the failed state if we restart the controller.
	// so we need to check if the ProxmoxClient is not nil and the ProxmoxCluster has a failure reason.
	err := r.reconcileFailedClusterState(clusterScope)
	if err != nil {
		return ctrl.Result{}, err
	}

	res, err := r.reconcileIPAM(ctx, clusterScope)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !res.IsZero() {
		return res, nil
	}

	if err := r.reconcileNormalCredentialsSecret(ctx, clusterScope); err != nil {
		conditions.MarkFalse(clusterScope.ProxmoxCluster, infrav1alpha1.ProxmoxClusterReady, infrav1alpha1.ProxmoxUnreachableReason, clusterv1.ConditionSeverityError, err.Error())
		if apierrors.IsNotFound(err) {
			clusterScope.ProxmoxCluster.Status.FailureMessage = ptr.To("credentials secret not found")
			clusterScope.ProxmoxCluster.Status.FailureReason = ptr.To(clustererrors.InvalidConfigurationClusterError)
		}
		return reconcile.Result{}, err
	}

	conditions.MarkTrue(clusterScope.ProxmoxCluster, infrav1alpha1.ProxmoxClusterReady)

	clusterScope.ProxmoxCluster.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *ProxmoxClusterReconciler) reconcileFailedClusterState(clusterScope *scope.ClusterScope) error {
	if clusterScope.ProxmoxClient != nil &&
		clusterScope.ProxmoxCluster.Status.FailureReason != nil &&
		clusterScope.ProxmoxCluster.Status.FailureMessage != nil &&
		ptr.Deref(clusterScope.ProxmoxCluster.Status.FailureReason, "") == clustererrors.InvalidConfigurationClusterError &&
		strings.Contains(ptr.Deref(clusterScope.ProxmoxCluster.Status.FailureMessage, ""), "No credentials found") {
		// clear the failure reason
		clusterScope.ProxmoxCluster.Status.FailureMessage = nil
		clusterScope.ProxmoxCluster.Status.FailureReason = nil
		if err := clusterScope.Close(); err != nil {
			return err
		}

		cHelper, err := patch.NewHelper(clusterScope.Cluster, r.Client)
		if err != nil {
			return errors.Wrap(err, "failed to init patch helper")
		}
		clusterScope.Cluster.Status.FailureMessage = nil
		clusterScope.Cluster.Status.FailureReason = nil
		if err = cHelper.Patch(context.TODO(), clusterScope.Cluster); err != nil {
			return err
		}
		return errors.New("reconciling cluster failure state")
	}

	return nil
}

func (r *ProxmoxClusterReconciler) reconcileIPAM(ctx context.Context, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	if err := clusterScope.IPAMHelper.CreateOrUpdateInClusterIPPool(ctx); err != nil {
		if errors.Is(err, ipam.ErrMissingAddresses) {
			clusterScope.Info("Missing addresses in cluster IPAM config, not reconciling")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if clusterScope.ProxmoxCluster.Spec.IPv4Config != nil {
		poolV4, err := clusterScope.IPAMHelper.GetDefaultInClusterIPPool(ctx, infrav1alpha1.IPV4Format)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{RequeueAfter: infrav1alpha1.DefaultReconcilerRequeue}, nil
			}

			return ctrl.Result{}, err
		}
		clusterScope.ProxmoxCluster.SetInClusterIPPoolRef(poolV4)
	}
	if clusterScope.ProxmoxCluster.Spec.IPv6Config != nil {
		poolV6, err := clusterScope.IPAMHelper.GetDefaultInClusterIPPool(ctx, infrav1alpha1.IPV6Format)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{RequeueAfter: infrav1alpha1.DefaultReconcilerRequeue}, nil
			}

			return ctrl.Result{}, err
		}
		clusterScope.ProxmoxCluster.SetInClusterIPPoolRef(poolV6)
	}

	return reconcile.Result{}, nil
}

func (r *ProxmoxClusterReconciler) listProxmoxMachinesForCluster(ctx context.Context, clusterScope *scope.ClusterScope) ([]infrav1alpha1.ProxmoxMachine, error) {
	var machineList infrav1alpha1.ProxmoxMachineList

	err := r.List(ctx, &machineList, client.InNamespace(clusterScope.Namespace()), client.MatchingLabels{
		clusterv1.ClusterNameLabel: clusterScope.Name(),
	})

	if err != nil {
		return nil, err
	}

	return machineList.Items, nil
}

func (r *ProxmoxClusterReconciler) reconcileNormalCredentialsSecret(ctx context.Context, clusterScope *scope.ClusterScope) error {
	proxmoxCluster := clusterScope.ProxmoxCluster
	if !hasCredentialsRef(proxmoxCluster) {
		return nil
	}

	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: getNamespaceFromProxmoxCluster(proxmoxCluster),
		Name:      proxmoxCluster.Spec.CredentialsRef.Name,
	}
	err := r.Client.Get(ctx, secretKey, secret)
	if err != nil {
		return err
	}

	helper, err := patch.NewHelper(secret, r.Client)
	if err != nil {
		return err
	}

	// Ensure the ProxmoxCluster is an owner and that the APIVersion is up-to-date.
	secret.SetOwnerReferences(clusterutil.EnsureOwnerRef(secret.GetOwnerReferences(),
		metav1.OwnerReference{
			APIVersion: infrav1alpha1.GroupVersion.String(),
			Kind:       "ProxmoxCluster",
			Name:       proxmoxCluster.Name,
			UID:        proxmoxCluster.UID,
		},
	))

	// Ensure the finalizer is added.
	if !ctrlutil.ContainsFinalizer(secret, infrav1alpha1.SecretFinalizer) {
		ctrlutil.AddFinalizer(secret, infrav1alpha1.SecretFinalizer)
	}

	return helper.Patch(ctx, secret)
}

func (r *ProxmoxClusterReconciler) reconcileDeleteCredentialsSecret(ctx context.Context, clusterScope *scope.ClusterScope) error {
	proxmoxCluster := clusterScope.ProxmoxCluster
	if !hasCredentialsRef(proxmoxCluster) {
		return nil
	}

	logger := ctrl.LoggerFrom(ctx)

	// Remove finalizer on Identity Secret
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: getNamespaceFromProxmoxCluster(proxmoxCluster),
		Name:      proxmoxCluster.Spec.CredentialsRef.Name,
	}
	if err := r.Client.Get(ctx, secretKey, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	helper, err := patch.NewHelper(secret, r.Client)
	if err != nil {
		return err
	}

	ownerRef := metav1.OwnerReference{
		APIVersion: infrav1alpha1.GroupVersion.String(),
		Kind:       "ProxmoxCluster",
		Name:       proxmoxCluster.Name,
		UID:        proxmoxCluster.UID,
	}

	if len(secret.GetOwnerReferences()) > 1 {
		// Remove the ProxmoxCluster from the OwnerRef.
		secret.SetOwnerReferences(clusterutil.RemoveOwnerRef(secret.GetOwnerReferences(), ownerRef))
	} else if clusterutil.HasOwnerRef(secret.GetOwnerReferences(), ownerRef) && ctrlutil.ContainsFinalizer(secret, infrav1alpha1.SecretFinalizer) {
		// There is only one OwnerRef, the current ProxmoxCluster. Remove the Finalizer (if present).
		logger.Info(fmt.Sprintf("Removing finalizer %s", infrav1alpha1.SecretFinalizer), "Secret", klog.KObj(secret))
		ctrlutil.RemoveFinalizer(secret, infrav1alpha1.SecretFinalizer)
	}

	return helper.Patch(ctx, secret)
}

func hasCredentialsRef(proxmoxCluster *infrav1alpha1.ProxmoxCluster) bool {
	return proxmoxCluster != nil && proxmoxCluster.Spec.CredentialsRef != nil
}

func getNamespaceFromProxmoxCluster(proxmoxCluster *infrav1alpha1.ProxmoxCluster) string {
	namespace := proxmoxCluster.Spec.CredentialsRef.Namespace
	if len(namespace) == 0 {
		namespace = proxmoxCluster.GetNamespace()
	}
	return namespace
}
