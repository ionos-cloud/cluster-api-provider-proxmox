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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// ProxmoxClusterKind the ProxmoxCluster kind.
	ProxmoxClusterKind = "ProxmoxCluster"
	// ClusterFinalizer allows cleaning up resources associated with
	// ProxmoxCluster before removing it from the apiserver.
	ClusterFinalizer = "proxmoxcluster.infrastructure.cluster.x-k8s.io"
)

// ProxmoxClusterSpec defines the desired state of ProxmoxCluster.
type ProxmoxClusterSpec struct {
	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`

	// AllowedNodes specifies all Proxmox nodes which will be considered
	// for operations. This implies that VMs can be cloned on different nodes from
	// the node which holds the VM template.
	// +optional
	AllowedNodes []string `json:"allowedNodes,omitempty"`

	// SchedulerHints allows to influence the decision on where a VM will be scheduled. For example by applying a multiplicator
	// to a node's resources, to allow for overprovisioning or to ensure a node will always have a safety buffer.
	// +optional
	SchedulerHints *SchedulerHints `json:"schedulerHints,omitempty"`

	ClusterNetworkConfig `json:",inline"`
}

// ClusterNetworkConfig represents information about the cluster network configuration.
type ClusterNetworkConfig struct {
	// IPv4Config contains information about available IPV4 address pools and the gateway.
	// this can be combined with ipv6Config to enable dual stack.
	// either IPv4Config or IPv6Config must be provided.
	// +optional
	IPv4Config *IPConfig `json:"ipv4Config,omitempty"`

	// IPv6Config contains information about available IPV6 address pools and the gateway.
	// this can be combined with ipv4Config to enable dual stack.
	// either IPv4Config or IPv6Config must be provided.
	// +optional
	IPv6Config *IPConfig `json:"ipv6Config,omitempty"`

	// DNSServers contains information about nameservers used by machines network-config.
	// +kubebuilder:validation:MinItems=1
	DNSServers []string `json:"dnsServers"`
}

// IPConfig contains information about available IP config.
type IPConfig struct {
	// Addresses is a list of IP addresses that can be assigned. This set of
	// addresses can be non-contiguous.
	// mutually exclusive with DHCP.
	// +kubebuilder:validation:MinItems=1
	Addresses []string `json:"addresses,omitempty"`

	// Prefix is the network prefix to use.
	// +kubebuilder:validation:Maximum=128
	// +optional
	Prefix int `json:"prefix,omitempty"`

	// Gateway
	// +optional
	Gateway string `json:"gateway,omitempty"`

	// DHCP indicates that DHCP should be used to assign IP addresses.
	// mutually exclusive with Addresses.
	// +optional
	// +kubebuilder:default=false
	DHCP bool `json:"dhcp,omitempty"`
}

// SchedulerHints allows to pass the scheduler instructions to (dis)allow over- or enforce underprovisioning of resources.
type SchedulerHints struct {
	// MemoryAdjustment allows to adjust a node's memory by a given percentage.
	// For example, setting it to 300 allows to allocate 300% of a host's memory for VMs,
	// and setting it to 95 limits memory allocation to 95% of a host's memory.
	// Setting it to 0 entirely disables scheduling memory constraints.
	// By default 100% of a node's memory will be used for allocation.
	// +optional
	MemoryAdjustment *uint64 `json:"memoryAdjustment,omitempty"`
}

// GetMemoryAdjustment returns the memory adjustment percentage to use within the scheduler.
func (sh *SchedulerHints) GetMemoryAdjustment() uint64 {
	memoryAdjustment := uint64(100)

	if sh != nil {
		memoryAdjustment = ptr.Deref(sh.MemoryAdjustment, 100)
	}

	return memoryAdjustment
}

// ProxmoxClusterStatus defines the observed state of ProxmoxCluster.
type ProxmoxClusterStatus struct {
	// Ready indicates that the cluster is ready.
	// +optional
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// InClusterIPPoolRef is the reference to the created in cluster ip pool
	// +optional
	InClusterIPPoolRef []corev1.LocalObjectReference `json:"inClusterIpPoolRef,omitempty"`

	// NodeLocations keeps track of which nodes have been selected
	// for different machines.
	// +optional
	NodeLocations *NodeLocations `json:"nodeLocations,omitempty"`

	// Conditions defines current service state of the ProxmoxCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// NodeLocations holds information about the deployment state of
// control plane and worker nodes in Proxmox.
type NodeLocations struct {
	// ControlPlane contains all deployed control plane nodes
	// +optional
	ControlPlane []NodeLocation `json:"controlPlane,omitempty"`

	// Workers contains all deployed worker nodes
	// +optional
	Workers []NodeLocation `json:"workers,omitempty"`
}

// NodeLocation holds information about a single VM
// in Proxmox.
type NodeLocation struct {
	// Machine is the reference of the proxmoxmachine
	Machine corev1.LocalObjectReference `json:"machine"`

	// Node is the Proxmox node
	Node string `json:"node"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=proxmoxclusters,scope=Namespaced,categories=cluster-api,singular=proxmoxcluster
//+kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Cluster infrastructure is ready"
//+kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.controlPlaneEndpoint",description="API Endpoint"

// ProxmoxCluster is the Schema for the proxmoxclusters API.
type ProxmoxCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:XValidation:rule="self.ipv4Config != null || self.ipv6Config != null",message="at least one ip config must be set, either ipv4Config or ipv6Config"
	Spec   ProxmoxClusterSpec   `json:"spec,omitempty"`
	Status ProxmoxClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProxmoxClusterList contains a list of ProxmoxCluster.
type ProxmoxClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxCluster `json:"items"`
}

// GetConditions returns the observations of the operational state of the ProxmoxCluster resource.
func (c *ProxmoxCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the underlying service state of the ProxmoxCluster to the predescribed clusterv1.Conditions.
func (c *ProxmoxCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

func init() {
	objectTypes = append(objectTypes, &ProxmoxCluster{}, &ProxmoxClusterList{})
}
