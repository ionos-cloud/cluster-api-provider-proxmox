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

// Package scope defines the capmox scopes used for reconciliation.
package scope

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/tlshelper"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	Client         client.Client
	Logger         *logr.Logger
	Cluster        *clusterv1.Cluster
	ProxmoxCluster *infrav1.ProxmoxCluster
	ProxmoxClient  capmox.Client
	ControllerName string
	IPAMHelper     *ipam.Helper
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	*logr.Logger
	client      client.Client
	patchHelper *patch.Helper

	Cluster        *clusterv1.Cluster
	ProxmoxCluster *infrav1.ProxmoxCluster

	ProxmoxClient  capmox.Client
	controllerName string

	// zoneProxmoxClients caches Proxmox clients built from an availability zone's
	// own credentialsRef, keyed by zone name.
	zoneProxmoxClients map[string]capmox.Client

	IPAMHelper *ipam.Helper
}

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	if params.Client == nil {
		return nil, errors.New("Client is required when creating a ClusterScope")
	}
	if params.Cluster == nil {
		return nil, errors.New("Cluster is required when creating a ClusterScope")
	}
	if params.ProxmoxCluster == nil {
		return nil, errors.New("ProxmoxCluster is required when creating a ClusterScope")
	}
	if params.IPAMHelper == nil {
		return nil, errors.New("IPAMHelper is required when creating a ClusterScope")
	}
	if params.Logger == nil {
		logger := log.FromContext(context.Background())
		params.Logger = &logger
	}

	clusterScope := &ClusterScope{
		Logger:         params.Logger,
		client:         params.Client,
		Cluster:        params.Cluster,
		ProxmoxCluster: params.ProxmoxCluster,
		controllerName: params.ControllerName,
		ProxmoxClient:  params.ProxmoxClient,
		IPAMHelper:     params.IPAMHelper,
	}

	helper, err := patch.NewHelper(params.ProxmoxCluster, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init patch helper")
	}

	clusterScope.patchHelper = helper

	if clusterScope.ProxmoxClient == nil {
		switch {
		case clusterScope.ProxmoxCluster.Spec.CredentialsRef != nil:
			// using proxmoxcluster.spec.credentialsRef
			pmoxClient, err := clusterScope.setupProxmoxClient(context.TODO(), clusterScope.ProxmoxCluster.Spec.CredentialsRef)
			if err != nil {
				return nil, errors.Wrap(err, "Unable to initialize ProxmoxClient")
			}
			clusterScope.ProxmoxClient = pmoxClient
		case defaultProxmoxClientNeeded(clusterScope.ProxmoxCluster):
			// No global credentials, no spec.credentialsRef, and at least one allowed node
			// isn't covered by an availability zone with its own credentialsRef: fail the
			// cluster since there is no way to reach that node.
			conditions.Set(clusterScope.ProxmoxCluster, metav1.Condition{
				Type:    infrav1.ProxmoxClusterProxmoxAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.ProxmoxClusterProxmoxAvailableCredentialsNotFoundReason,
				Message: "No credentials found, ProxmoxCluster missing credentialsRef",
			})

			if err = clusterScope.Close(); err != nil {
				return nil, err
			}
			return nil, errors.New("No credentials found, ProxmoxCluster missing credentialsRef")
		default:
			// Every allowed node is covered by an availability zone with its own
			// credentialsRef (physically separate Proxmox VE clusters, one per zone), so
			// no default client is required. Leave ProxmoxClient nil; GetProxmoxClient
			// returns a clear error if it's ever requested for an uncovered node/zone.
		}
	}

	return clusterScope, nil
}

// defaultProxmoxClientNeeded reports whether the ProxmoxCluster's default Proxmox client
// (global controller credentials or spec.credentialsRef) is required, i.e. whether there are
// no availability zones, any zone without its own credentialsRef, or an allowed node not
// covered by any availability zone.
func defaultProxmoxClientNeeded(proxmoxCluster *infrav1.ProxmoxCluster) bool {
	azs := proxmoxCluster.Spec.AvailabilityZones
	if len(azs) == 0 {
		return true
	}

	coveredNodes := make(map[string]struct{})
	for _, az := range azs {
		if az.CredentialsRef == nil {
			return true
		}
		for _, node := range az.Nodes {
			coveredNodes[node] = struct{}{}
		}
	}

	for _, node := range proxmoxCluster.Spec.AllowedNodes {
		if _, ok := coveredNodes[node]; !ok {
			return true
		}
	}

	return false
}

// GetProxmoxClient returns the Proxmox client to use for the given availability zone.
// If zone is empty, or the zone doesn't define its own credentialsRef, the ProxmoxCluster's
// default client is returned. This allows each availability zone to be backed by a
// physically separate Proxmox cluster with its own API endpoint and credentials.
func (s *ClusterScope) GetProxmoxClient(ctx context.Context, zone string) (capmox.Client, error) {
	if zone == "" {
		return s.defaultProxmoxClientOrError()
	}

	az := availabilityZoneByName(s.ProxmoxCluster.Spec.AvailabilityZones, zone)
	if az == nil || az.CredentialsRef == nil {
		return s.defaultProxmoxClientOrError()
	}

	if pmoxClient, ok := s.zoneProxmoxClients[zone]; ok {
		return pmoxClient, nil
	}

	pmoxClient, err := s.setupProxmoxClient(ctx, az.CredentialsRef)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to initialize ProxmoxClient for availability zone %q", zone)
	}

	if s.zoneProxmoxClients == nil {
		s.zoneProxmoxClients = make(map[string]capmox.Client)
	}
	s.zoneProxmoxClients[zone] = pmoxClient

	return pmoxClient, nil
}

func availabilityZoneByName(azs []infrav1.AvailabilityZoneSpec, name string) *infrav1.AvailabilityZoneSpec {
	for i := range azs {
		if azs[i].Name == name {
			return &azs[i]
		}
	}
	return nil
}

// GetProxmoxClientForNode returns the Proxmox client responsible for the given Proxmox VE
// node, resolving the availability zone (if any) that the node belongs to. This should be
// preferred over GetProxmoxClient when the target node is already known (e.g. once a Machine
// has been scheduled), since a Machine's spec.failureDomain isn't always set (for example on
// plain MachineDeployment workers, which CAPI doesn't automatically spread across zones).
func (s *ClusterScope) GetProxmoxClientForNode(ctx context.Context, node string) (capmox.Client, error) {
	return s.GetProxmoxClient(ctx, zoneForNode(s.ProxmoxCluster.Spec.AvailabilityZones, node))
}

// zoneForNode returns the name of the availability zone that contains the given node, or
// "" if the node isn't listed in any availability zone.
func zoneForNode(azs []infrav1.AvailabilityZoneSpec, node string) string {
	for _, az := range azs {
		if slices.Contains(az.Nodes, node) {
			return az.Name
		}
	}
	return ""
}

// defaultProxmoxClientOrError returns the ProxmoxCluster's default client, or an error if
// none was configured (e.g. every availability zone defines its own credentialsRef and no
// default was needed at scope creation, but a node/zone without dedicated credentials was
// requested anyway).
func (s *ClusterScope) defaultProxmoxClientOrError() (capmox.Client, error) {
	if s.ProxmoxClient == nil {
		return nil, errors.New("no default ProxmoxClient configured: set ProxmoxCluster.spec.credentialsRef, the controller's global credentials, or a credentialsRef on every availability zone/allowed node")
	}
	return s.ProxmoxClient, nil
}

func (s *ClusterScope) setupProxmoxClient(ctx context.Context, credentialsRef *corev1.SecretReference) (capmox.Client, error) {
	// get the credentials secret
	secret := corev1.Secret{}
	namespace := credentialsRef.Namespace
	if len(namespace) == 0 {
		namespace = s.ProxmoxCluster.GetNamespace()
	}
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      credentialsRef.Name,
	}, &secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			conditions.Set(s.ProxmoxCluster, metav1.Condition{
				Type:    infrav1.ProxmoxClusterProxmoxAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.ProxmoxClusterProxmoxAvailableCredentialsNotFoundReason,
				Message: "credentials secret not found",
			})
		}
		return nil, errors.Wrap(err, "failed to get credentials secret")
	}

	token := string(secret.Data["token"])
	tokenSecret := string(secret.Data["secret"])
	url := string(secret.Data["url"])

	tlsInsecure, tlsInsecureSet := secret.Data["insecure"]
	tlsRootCA := secret.Data["root_ca"]

	rootCerts, err := tlshelper.SystemRootsWithCert(tlsRootCA)
	if err != nil {
		return nil, fmt.Errorf("loading cert pool: %w", err)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			// When "insecure" is unset we retain the pre-v0.7 behavior of
			// setting the connection insecure. If it is set we compare
			// against YAML true-ish values.
			//
			// #nosec:G402 // Intended to enable insecure mode for unknown CAs
			InsecureSkipVerify: !tlsInsecureSet || slices.Contains([]string{"1", "on", "true", "yes", "y"}, strings.ToLower(string(tlsInsecure))),
			RootCAs:            rootCerts,
		},
	}

	httpClient := &http.Client{Transport: tr}
	return goproxmox.NewAPIClient(ctx, *s.Logger, url,
		proxmox.WithHTTPClient(httpClient),
		proxmox.WithAPIToken(token, tokenSecret),
	)
}

// Name returns the CAPI cluster name.
func (s *ClusterScope) Name() string {
	return s.Cluster.Name
}

// Namespace returns the cluster namespace.
func (s *ClusterScope) Namespace() string {
	return s.Cluster.Namespace
}

// InfraClusterName returns the name of the Proxmox cluster.
func (s *ClusterScope) InfraClusterName() string {
	return s.ProxmoxCluster.Name
}

// KubernetesClusterName is the name of the Kubernetes cluster. For the cluster
// scope this is the same as the CAPI cluster name.
func (s *ClusterScope) KubernetesClusterName() string {
	return s.Cluster.Name
}

// PatchObject persists the cluster configuration and status.
func (s *ClusterScope) PatchObject() error {
	// always update the readyCondition.
	_ = conditions.SetSummaryCondition(s.ProxmoxCluster, s.ProxmoxCluster, "Ready",
		conditions.ForConditionTypes{infrav1.ProxmoxClusterProxmoxAvailableCondition},
	)

	return s.patchHelper.Patch(context.TODO(), s.ProxmoxCluster,
		patch.WithOwnedConditions{Conditions: []string{
			"Ready",
			infrav1.ProxmoxClusterProxmoxAvailableCondition,
		}})
}

// ListProxmoxMachinesForCluster returns all the ProxmoxMachines that belong to this cluster.
func (s *ClusterScope) ListProxmoxMachinesForCluster(ctx context.Context) ([]infrav1.ProxmoxMachine, error) {
	var machineList infrav1.ProxmoxMachineList

	err := s.client.List(ctx, &machineList, client.InNamespace(s.Namespace()), client.MatchingLabels{
		clusterv1.ClusterNameLabel: s.Name(),
	})
	if err != nil {
		return nil, err
	}

	return machineList.Items, nil
}

// Close closes the current scope persisting the cluster configuration and status.
func (s *ClusterScope) Close() error {
	return s.PatchObject()
}

// SetReady sets the ProxmoxCluster as provisioned.
func (s *ClusterScope) SetReady() {
	s.ProxmoxCluster.Status.Initialization.Provisioned = new(true)
}
