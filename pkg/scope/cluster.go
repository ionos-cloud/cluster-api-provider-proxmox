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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
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
		if clusterScope.ProxmoxCluster.Spec.CredentialsRef == nil {
			// Fail the cluster if no credentials found.
			conditions.Set(clusterScope.ProxmoxCluster, metav1.Condition{
				Type:    infrav1.ProxmoxClusterProxmoxAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.ProxmoxClusterProxmoxAvailableProxmoxUnreachableReason,
				Message: "No credentials found, ProxmoxCluster missing credentialsRef",
			})

			if err = clusterScope.Close(); err != nil {
				return nil, err
			}
			return nil, errors.New("No credentials found, ProxmoxCluster missing credentialsRef")
		}
		// using proxmoxcluster.spec.credentialsRef
		pmoxClient, err := clusterScope.setupProxmoxClient(context.TODO())
		if err != nil {
			return nil, errors.Wrap(err, "Unable to initialize ProxmoxClient")
		}
		clusterScope.ProxmoxClient = pmoxClient
	}

	return clusterScope, nil
}

func (s *ClusterScope) setupProxmoxClient(ctx context.Context) (capmox.Client, error) {
	// get the credentials secret
	secret := corev1.Secret{}
	namespace := s.ProxmoxCluster.Spec.CredentialsRef.Namespace
	if len(namespace) == 0 {
		namespace = s.ProxmoxCluster.GetNamespace()
	}
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      s.ProxmoxCluster.Spec.CredentialsRef.Name,
	}, &secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			conditions.Set(s.ProxmoxCluster, metav1.Condition{
				Type:    infrav1.ProxmoxClusterProxmoxAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.ProxmoxClusterProxmoxAvailableProxmoxUnreachableReason,
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
		}},
	)
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
