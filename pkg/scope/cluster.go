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

// Package scope defines the capmox scopes used for reconciliation.
package scope

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clustererrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	Client         client.Client
	Logger         *logr.Logger
	Cluster        *clusterv1.Cluster
	ProxmoxCluster *infrav1alpha1.ProxmoxCluster
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
	ProxmoxCluster *infrav1alpha1.ProxmoxCluster

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

	if clusterScope.ProxmoxClient == nil && clusterScope.ProxmoxCluster.Spec.CredentialsRef == nil {
		// Fail the cluster if no credentials found.
		// set failure reason
		clusterScope.ProxmoxCluster.Status.FailureMessage = ptr.To("No credentials found, ProxmoxCluster missing credentialsRef")
		clusterScope.ProxmoxCluster.Status.FailureReason = ptr.To(clustererrors.InvalidConfigurationClusterError)

		if err = clusterScope.Close(); err != nil {
			return nil, err
		}
		return nil, errors.New("No credentials found, ProxmoxCluster missing credentialsRef")
	} else if clusterScope.ProxmoxCluster.Spec.CredentialsRef != nil {
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
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: s.ProxmoxCluster.Spec.CredentialsRef.Namespace,
		Name:      s.ProxmoxCluster.Spec.CredentialsRef.Name,
	}, &secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// set failure reason
			s.ProxmoxCluster.Status.FailureMessage = ptr.To("credentials secret not found")
			s.ProxmoxCluster.Status.FailureReason = ptr.To(clustererrors.InvalidConfigurationClusterError)
		}
		return nil, errors.Wrap(err, "failed to get credentials secret")
	}

	token := string(secret.Data["token"])
	tokenSecret := string(secret.Data["secret"])
	url := string(secret.Data["url"])

	// TODO, check if we need to delete tls config
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
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

// ControlPlaneEndpoint returns the ControlPlaneEndpoint for the associated ProxmoxCluster.
func (s *ClusterScope) ControlPlaneEndpoint() clusterv1.APIEndpoint {
	return s.ProxmoxCluster.Spec.ControlPlaneEndpoint
}

// PatchObject persists the cluster configuration and status.
func (s *ClusterScope) PatchObject() error {
	// always update the readyCondition.
	conditions.SetSummary(s.ProxmoxCluster,
		conditions.WithConditions(
			infrav1alpha1.ProxmoxClusterReady,
		),
	)

	return s.patchHelper.Patch(context.TODO(), s.ProxmoxCluster)
}

// Close closes the current scope persisting the cluster configuration and status.
func (s *ClusterScope) Close() error {
	return s.PatchObject()
}
