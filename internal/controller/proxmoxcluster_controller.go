/*
Copyright 2023-2026 IONOS Cloud.

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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	// temporary replacement for "sigs.k8s.io/cluster-api/util" until v1beta2.
	clusterutil "github.com/ionos-cloud/cluster-api-provider-proxmox/capiv1beta1/util"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/capiv1beta1/util/annotations"

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

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/consts"
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
		For(&infrav1.ProxmoxCluster{}).
		WithEventFilter(predicates.ResourceNotPaused(r.Scheme, ctrl.LoggerFrom(ctx))).
		Watches(&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterutil.ClusterToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind(infrav1.ProxmoxClusterKind), mgr.GetClient(), &infrav1.ProxmoxCluster{})),
			builder.WithPredicates(predicates.ClusterUnpaused(r.Scheme, ctrl.LoggerFrom(ctx)))).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(r.Scheme, ctrl.LoggerFrom(ctx))).
		Complete(r)
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters/finalizers,verbs=update

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;patch

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

	proxmoxCluster := &infrav1.ProxmoxCluster{}
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
	machines, err := clusterScope.ListProxmoxMachinesForCluster(ctx)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "could not retrieve proxmox machines for cluster %q", clusterScope.InfraClusterName())
	}

	// Requeue if there are one or more machines left.
	if len(machines) > 0 {
		clusterScope.Info("waiting for machines to be deleted", "remaining", len(machines))
		return ctrl.Result{RequeueAfter: infrav1.DefaultReconcilerRequeue}, nil
	}

	if err := r.reconcileDeleteCredentialsSecret(ctx, clusterScope); err != nil {
		return reconcile.Result{}, err
	}

	clusterScope.Info("cluster deleted successfully")
	ctrlutil.RemoveFinalizer(clusterScope.ProxmoxCluster, infrav1.ClusterFinalizer)
	return ctrl.Result{}, nil
}

func (r *ProxmoxClusterReconciler) reconcileNormal(ctx context.Context, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	clusterScope.Logger.Info("Reconciling ProxmoxCluster")

	// If the ProxmoxCluster doesn't have our finalizer, add it.
	ctrlutil.AddFinalizer(clusterScope.ProxmoxCluster, infrav1.ClusterFinalizer)

	if ptr.Deref(clusterScope.ProxmoxCluster.Spec.ExternalManagedControlPlane, false) {
		if clusterScope.ProxmoxCluster.Spec.ControlPlaneEndpoint.Host == "" {
			clusterScope.Logger.Info("ProxmoxCluster is not ready, missing or waiting for a ControlPlaneEndpoint host")

			conditions.Set(clusterScope.ProxmoxCluster, metav1.Condition{
				Type:    infrav1.ProxmoxClusterProxmoxAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.ProxmoxClusterProxmoxAvailableMissingControlPlaneEndpointReason,
				Message: "The ProxmoxCluster is missing or waiting for a ControlPlaneEndpoint host",
			})

			return ctrl.Result{Requeue: true}, nil
		}
		if clusterScope.ProxmoxCluster.Spec.ControlPlaneEndpoint.Port == 0 {
			clusterScope.Logger.Info("ProxmoxCluster is not ready, missing or waiting for a ControlPlaneEndpoint port")

			conditions.Set(clusterScope.ProxmoxCluster, metav1.Condition{
				Type:    infrav1.ProxmoxClusterProxmoxAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.ProxmoxClusterProxmoxAvailableMissingControlPlaneEndpointReason,
				Message: "The ProxmoxCluster is missing or waiting for a ControlPlaneEndpoint port",
			})

			return ctrl.Result{Requeue: true}, nil
		}
	}

	// when a Cluster is marked failed cause the Proxmox client is nil.
	// the cluster doesn't reconcile the failed state if we restart the controller.
	// so we need to check if the ProxmoxClient is not nil and the ProxmoxCluster has a failure reason.
	err := r.reconcileFailedClusterState(ctx, clusterScope)
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
		conditions.Set(clusterScope.ProxmoxCluster, metav1.Condition{
			Type:    infrav1.ProxmoxClusterProxmoxAvailableCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.ProxmoxClusterProxmoxAvailableProxmoxUnreachableReason,
			Message: fmt.Sprintf("%s", err),
		})
		return reconcile.Result{}, err
	}

	conditions.Set(clusterScope.ProxmoxCluster, metav1.Condition{
		Type:   infrav1.ProxmoxClusterProxmoxAvailableCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1beta2.ProvisionedReason,
	})

	clusterScope.ProxmoxCluster.Status.Initialization.Provisioned = ptr.To(true)

	return ctrl.Result{}, nil
}

func (r *ProxmoxClusterReconciler) reconcileFailedClusterState(ctx context.Context, clusterScope *scope.ClusterScope) error {
	if clusterScope.ProxmoxClient != nil {
		cond := conditions.Get(clusterScope.ProxmoxCluster, infrav1.ProxmoxClusterProxmoxAvailableCondition)
		if cond != nil && cond.Status == metav1.ConditionFalse && cond.Reason == infrav1.ProxmoxClusterProxmoxAvailableProxmoxUnreachableReason {
			// Clear the failure condition on the proxmox cluster.
			conditions.Set(clusterScope.ProxmoxCluster, metav1.Condition{
				Type:   infrav1.ProxmoxClusterProxmoxAvailableCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1beta2.ProvisionedReason,
			})
			if err := clusterScope.PatchObject(); err != nil {
				return err
			}

			// Clear the failure reason on the root cluster if present.
			if ptr.Deref(clusterScope.Cluster.Status.FailureMessage, "") != "" {
				newCluster := clusterScope.Cluster.DeepCopy()
				newCluster.Status.FailureMessage = nil
				newCluster.Status.FailureReason = nil

				err := r.Status().Patch(ctx, newCluster, client.MergeFrom(clusterScope.Cluster))
				if err != nil {
					return errors.Wrapf(err, "failed to patch cluster %s/%s", newCluster.Namespace, newCluster.Name)
				}
			}

			return errors.New("reconciling cluster failure state")
		}
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

	proxmoxCluster := clusterScope.ProxmoxCluster
	ipPools := []string{}
	if proxmoxCluster.Spec.IPv4Config != nil {
		ipPools = append(ipPools, ipam.InClusterPoolFormat(proxmoxCluster, nil, infrav1.IPv4Format))
	}
	if proxmoxCluster.Spec.IPv6Config != nil {
		ipPools = append(ipPools, ipam.InClusterPoolFormat(proxmoxCluster, nil, infrav1.IPv6Format))
	}
	for _, zone := range proxmoxCluster.Spec.ZoneConfigs {
		if zone.IPv4Config != nil {
			ipPools = append(ipPools, ipam.InClusterPoolFormat(proxmoxCluster, zone.Zone, infrav1.IPv4Format))
		}
		if zone.IPv6Config != nil {
			ipPools = append(ipPools, ipam.InClusterPoolFormat(proxmoxCluster, zone.Zone, infrav1.IPv6Format))
		}
	}

	for _, poolName := range ipPools {
		pool, err := clusterScope.IPAMHelper.GetIPPool(ctx, corev1.TypedLocalObjectReference{
			APIGroup: consts.GetIPAMInClusterAPIGroup(),
			Name:     poolName,
			Kind:     consts.GetInClusterIPPoolKind(),
		})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{RequeueAfter: infrav1.DefaultReconcilerRequeue}, nil
			}

			return ctrl.Result{}, err
		}
		clusterScope.ProxmoxCluster.SetInClusterIPPoolRef(pool)
	}

	return reconcile.Result{}, nil
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
			APIVersion: infrav1.GroupVersion.String(),
			Kind:       "ProxmoxCluster",
			Name:       proxmoxCluster.Name,
			UID:        proxmoxCluster.UID,
		},
	))

	// Ensure the finalizer is added.
	if !ctrlutil.ContainsFinalizer(secret, infrav1.SecretFinalizer) {
		ctrlutil.AddFinalizer(secret, infrav1.SecretFinalizer)
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
		APIVersion: infrav1.GroupVersion.String(),
		Kind:       "ProxmoxCluster",
		Name:       proxmoxCluster.Name,
		UID:        proxmoxCluster.UID,
	}

	if len(secret.GetOwnerReferences()) > 1 {
		// Remove the ProxmoxCluster from the OwnerRef.
		secret.SetOwnerReferences(clusterutil.RemoveOwnerRef(secret.GetOwnerReferences(), ownerRef))
	} else if clusterutil.HasOwnerRef(secret.GetOwnerReferences(), ownerRef) && ctrlutil.ContainsFinalizer(secret, infrav1.SecretFinalizer) {
		// There is only one OwnerRef, the current ProxmoxCluster. Remove the Finalizer (if present).
		logger.Info(fmt.Sprintf("Removing finalizer %s", infrav1.SecretFinalizer), "Secret", klog.KObj(secret))
		ctrlutil.RemoveFinalizer(secret, infrav1.SecretFinalizer)
	}

	return helper.Patch(ctx, secret)
}

func hasCredentialsRef(proxmoxCluster *infrav1.ProxmoxCluster) bool {
	return proxmoxCluster != nil && proxmoxCluster.Spec.CredentialsRef != nil
}

func getNamespaceFromProxmoxCluster(proxmoxCluster *infrav1.ProxmoxCluster) string {
	namespace := proxmoxCluster.Spec.CredentialsRef.Namespace
	if len(namespace) == 0 {
		namespace = proxmoxCluster.GetNamespace()
	}
	return namespace
}
