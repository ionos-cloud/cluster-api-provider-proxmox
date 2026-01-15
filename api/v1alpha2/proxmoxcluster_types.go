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

package v1alpha2

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1" //nolint:staticcheck
	"sigs.k8s.io/cluster-api/errors"                     //nolint:staticcheck
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ProxmoxClusterKind is the ProxmoxCluster kind.
	ProxmoxClusterKind = "ProxmoxCluster"
	// ClusterFinalizer allows cleaning up resources associated with a
	// ProxmoxCluster before removing it from the apiserver.
	ClusterFinalizer = "proxmoxcluster.infrastructure.cluster.x-k8s.io"
	// SecretFinalizer is the finalizer for ProxmoxCluster credentials secrets.
	SecretFinalizer = "proxmoxcluster.infrastructure.cluster.x-k8s.io/secret" //nolint:gosec
)

// ProxmoxClusterSpec defines the desired state of a ProxmoxCluster.
type ProxmoxClusterSpec struct {
	// controlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.port > 0 && self.port < 65536",message="port must be within 1-65535"
	ControlPlaneEndpoint *clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// externalManagedControlPlane can be enabled to allow externally managed Control Planes to patch the
	// Proxmox cluster with the Load Balancer IP provided by Control Plane provider.
	// +optional
	// +kubebuilder:default=false
	ExternalManagedControlPlane *bool `json:"externalManagedControlPlane,omitempty"`

	// allowedNodes specifies all Proxmox nodes which will be considered
	// for operations. This implies that VMs can be cloned on different nodes from
	// the node which holds the VM template.
	// +listType=set
	// +optional
	AllowedNodes []string `json:"allowedNodes,omitempty"`

	// schedulerHints allows to influence the decision on where a VM will be scheduled. For example by applying a multiplicator
	// to a node's resources, to allow for overprovisioning or to ensure a node will always have a safety buffer.
	// +optional
	SchedulerHints *SchedulerHints `json:"schedulerHints,omitempty"`

	// ipv4Config contains information about available IPv4 address pools and the gateway.
	// This can be combined with ipv6Config in order to enable dual stack.
	// Either IPv4Config or IPv6Config must be provided.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.addresses.size() > 0",message="IPv4Config addresses must be provided"
	IPv4Config *IPConfigSpec `json:"ipv4Config,omitempty"`

	// ipv6Config contains information about available IPv6 address pools and the gateway.
	// This can be combined with ipv4Config in order to enable dual stack.
	// Either IPv4Config or IPv6Config must be provided.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.addresses.size() > 0",message="IPv6Config addresses must be provided"
	IPv6Config *IPConfigSpec `json:"ipv6Config,omitempty"`

	// dnsServers contains information about nameservers used by the machines.
	// +required
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	DNSServers []string `json:"dnsServers,omitempty"`

	// zoneConfig defines a IPAddress config per deployment zone.
	// +listType=map
	// +listMapKey=zone
	// +optional
	ZoneConfigs []ZoneConfigSpec `json:"zoneConfig,omitempty"`

	// cloneSpec is the configuration pertaining to all items configurable
	// in the configuration and cloning of a proxmox VM. Multiple types of nodes can be specified.
	// +optional
	CloneSpec *ProxmoxClusterCloneSpec `json:"cloneSpec,omitempty"`

	// credentialsRef is a reference to a Secret that contains the credentials to use for provisioning this cluster. If not
	// supplied then the credentials of the controller will be used.
	// if no namespace is provided, the namespace of the ProxmoxCluster will be used.
	// +optional
	CredentialsRef *corev1.SecretReference `json:"credentialsRef,omitempty"`
}

// ZoneConfigSpec is the Network Configuration for further deployment zones.
type ZoneConfigSpec struct {
	// zone is the name of your deployment zone.
	// +required
	Zone Zone `json:"zone,omitempty"`

	// ipv4Config contains information about available IPv4 address pools and the gateway.
	// This can be combined with ipv6Config in order to enable dual stack.
	// Either IPv4Config or IPv6Config must be provided.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.addresses.size() > 0",message="IPv4Config addresses must be provided"
	IPv4Config *IPConfigSpec `json:"ipv4Config,omitempty"`

	// ipv6Config contains information about available IPv6 address pools and the gateway.
	// This can be combined with ipv4Config in order to enable dual stack.
	// Either IPv4Config or IPv6Config must be provided.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.addresses.size() > 0",message="IPv6Config addresses must be provided"
	IPv6Config *IPConfigSpec `json:"ipv6Config,omitempty"`

	// dnsServers contains information about nameservers used by the machines in this zone.
	// +required
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	DNSServers []string `json:"dnsServers,omitempty"`
}

// ProxmoxClusterClassSpec defines deployment templates for ClusterClass.
type ProxmoxClusterClassSpec struct {
	// machineType is the name of the template for ClusterClass.
	// +required
	// +kubebuilder:validation:MinLength=1
	MachineType string `json:"machineType,omitempty"`

	// proxmoxMachineSpec is the to be patched yaml object.
	ProxmoxMachineSpec `json:",inline"`
}

// ProxmoxClusterCloneSpec is the configuration pertaining to all items configurable
// in the configuration and cloning of a proxmox VM.
type ProxmoxClusterCloneSpec struct {
	// machineSpec is the map of machine specs.
	// +kubebuilder:validation:XValidation:rule="self.exists_one(x, x.machineType == \"controlPlane\")",message="Cowardly refusing to deploy cluster without control plane"
	// +listType=map
	// +listMapKey=machineType
	// +optional
	ProxmoxClusterClassSpec []ProxmoxClusterClassSpec `json:"machineSpec,omitempty,omitzero"`
	// Justification: This field intentionally violates API spec:
	// It exists only to store information for Cluster Classes, and is never accessed from within the controller.

	// sshAuthorizedKeys contains the authorized keys deployed to the PROXMOX VMs.
	// +listType=set
	// +optional
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`

	// virtualIPNetworkInterface is the interface the k8s control plane binds to.
	// +optional
	VirtualIPNetworkInterface *string `json:"virtualIPNetworkInterface,omitempty"`
}

// IPConfigSpec contains information about available IP config.
type IPConfigSpec struct {
	// addresses is a list of IP addresses that can be assigned. This set of addresses can be non-contiguous.
	// +required
	// +listType=set
	Addresses []string `json:"addresses,omitempty"`

	// prefix is the network prefix to use.
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=128
	Prefix int32 `json:"prefix,omitempty"`

	// gateway is the network gateway
	// +required
	// +kubebuilder:validation:MinLength=1
	Gateway string `json:"gateway,omitempty"`

	// metric is the route priority applied to the default gateway
	// +required
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=0
	Metric *int32 `json:"metric,omitempty"`
}

// SchedulerHints allows to pass the scheduler instructions to (dis)allow over- or enforce underprovisioning of resources.
type SchedulerHints struct {
	// memoryAdjustment allows to adjust a node's memory by a given percentage.
	// For example, setting it to 300 allows to allocate 300% of a host's memory for VMs,
	// and setting it to 95 limits memory allocation to 95% of a host's memory.
	// Setting it to 0 entirely disables scheduling memory constraints.
	// By default 100% of a node's memory will be used for allocation.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MemoryAdjustment *int64 `json:"memoryAdjustment,omitempty"`
}

// GetMemoryAdjustment returns the memory adjustment percentage to use within the scheduler.
func (sh *SchedulerHints) GetMemoryAdjustment() int64 {
	memoryAdjustment := int64(100)

	if sh != nil {
		memoryAdjustment = ptr.Deref(sh.MemoryAdjustment, 100)
	}

	return memoryAdjustment
}

// ProxmoxClusterStatus defines the observed state of a ProxmoxCluster.
type ProxmoxClusterStatus struct {
	// ready indicates that the cluster is ready.
	// +kubebuilder:default=false
	// +optional
	Ready *bool `json:"ready,omitempty"`

	// inClusterIpPoolRef is the reference to the created in-cluster IP pool.
	// +listType=atomic
	// +optional
	InClusterIPPoolRef []corev1.LocalObjectReference `json:"inClusterIpPoolRef,omitempty"`

	// inClusterZoneRef lists InClusterIPPools per proxmox-zone.
	// +optional
	// +listType=map
	// +listMapKey=zone
	InClusterZoneRef []InClusterZoneRef `json:"inClusterZoneRef,omitempty"`

	// nodeLocations keeps track of which nodes have been selected
	// for different machines.
	// +optional
	NodeLocations *NodeLocations `json:"nodeLocations,omitempty"`

	// failureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of ProxmoxCluster
	// can be added as events to the ProxmoxCluster object and/or logged in the
	// controller's output.
	// +optional
	FailureReason *errors.ClusterStatusError `json:"failureReason,omitempty"`

	// failureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of ProxmoxMachines
	// can be added as events to the ProxmoxCluster object and/or logged in the
	// controller's output.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// conditions defines the current service state of the ProxmoxCluster.
	// +optional
	//nolint:kubeapilinter
	Conditions *[]clusterv1.Condition `json:"conditions,omitempty"`
	// Justification: kubeapilinter returns a false positive on fields called Conditions
	// because type is assumed to be metav1.Conditions.
	// deepcopy-gen wrongly infers the type when this is a pointer to clusterv1.Conditions,
	// So we need to store *[]clusterv1.Condition to create correct deepcopy code.
}

// InClusterZoneRef holds the InClusterIPPools associated with a zone.
type InClusterZoneRef struct {
	// zone defines the deployment proxmox-zone.
	// +kubebuilder:default="default"
	// +required
	Zone Zone `json:"zone,omitempty"`

	// inClusterIpPoolRefV4 is the reference to the created in-cluster IP pool.
	// +optional
	InClusterIPPoolRefV4 *corev1.LocalObjectReference `json:"inClusterIpPoolRefV4,omitempty"`

	// inClusterIpPoolRefV6 is the reference to the created in-cluster IP pool.
	// +optional
	InClusterIPPoolRefV6 *corev1.LocalObjectReference `json:"inClusterIpPoolRefV6,omitempty"`
}

// NodeLocations holds information about the deployment state of
// control plane and worker nodes in Proxmox.
type NodeLocations struct {
	// controlPlane contains all deployed control plane nodes.
	// +optional
	// +listType=atomic
	ControlPlane []NodeLocation `json:"controlPlane,omitempty"`

	// workers contains all deployed worker nodes.
	// +optional
	// +listType=atomic
	Workers []NodeLocation `json:"workers,omitempty"`
}

// NodeLocation holds information about a single VM
// in Proxmox.
type NodeLocation struct {
	// machine is the reference to the ProxmoxMachine that the node is on.
	// +required
	Machine corev1.LocalObjectReference `json:"machine,omitempty"`

	// node is the Proxmox node.
	// +kubebuilder:validation:MinLength=1
	// +required
	Node string `json:"node,omitempty"`

	// zone is the zone the Machine is in.
	// +optional
	Zone Zone `json:"zone,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=proxmoxclusters,scope=Namespaced,categories=cluster-api,singular=proxmoxcluster
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Cluster infrastructure is ready"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.controlPlaneEndpoint",description="API Endpoint"

// ProxmoxCluster is the Schema for the proxmoxclusters API.
type ProxmoxCluster struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the Proxmox Cluster spec
	// +kubebuilder:validation:XValidation:rule="self.ipv4Config != null || self.ipv6Config != null",message="at least one ip config must be set, either ipv4Config or ipv6Config"
	// +required
	Spec ProxmoxClusterSpec `json:"spec,omitzero"`

	// status is the Proxmox Cluster status
	// +optional
	//nolint:kubeapilinter
	Status ProxmoxClusterStatus `json:"status,omitempty,omitzero"`
	// Justification: this is the paradigm used by cluster-api.
}

// +kubebuilder:object:root=true

// ProxmoxClusterList contains a list of ProxmoxCluster.
type ProxmoxClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxCluster `json:"items"`
}

// GetConditions returns the observations of the operational state of the ProxmoxCluster resource.
func (c *ProxmoxCluster) GetConditions() clusterv1.Conditions {
	conditions := ptr.Deref(c.Status.Conditions, []clusterv1.Condition{})

	return conditions
}

// SetConditions sets the underlying service state of the ProxmoxCluster to the predescribed clusterv1.Conditions.
func (c *ProxmoxCluster) SetConditions(conditions clusterv1.Conditions) {
	// This is required because deepcopy-gen incorrectly infers the type of conditions.
	// Justification: static assignment will not work because type assurance
	// can not cast from type clusterv1.Conditions to []clusterv1.Condition.
	//nolint:staticcheck
	var typeHelper []clusterv1.Condition
	typeHelper = conditions

	c.Status.Conditions = &typeHelper
}

// AddInClusterZoneRef will set the Zone references status for the provided pool.
func (c *ProxmoxCluster) AddInClusterZoneRef(pool client.Object) {
	if pool == nil || pool.GetName() == "" {
		c.Status.InClusterZoneRef = nil
		return
	}

	annotations := pool.GetAnnotations()
	poolType, exists := annotations[ProxmoxIPFamilyAnnotation]

	// Nothing to do, we can not detect ip family because that
	// code may error.
	if !exists {
		return
	}

	labels := pool.GetLabels()
	zone, exists := labels[ProxmoxZoneLabel]

	// Add to default zone (as that has no label yet)
	if !exists {
		zone = "default"
	}

	if c.Status.InClusterZoneRef == nil {
		c.Status.InClusterZoneRef = []InClusterZoneRef{{
			Zone: &zone,
		}}
	}

	index := slices.IndexFunc(c.Status.InClusterZoneRef, func(r InClusterZoneRef) bool {
		return *r.Zone == zone
	})

	if index < 0 {
		c.Status.InClusterZoneRef = append(c.Status.InClusterZoneRef, InClusterZoneRef{Zone: &zone})
		index = len(c.Status.InClusterZoneRef)
	}

	poolRef := corev1.LocalObjectReference{Name: pool.GetName()}
	if poolType == IPv4Type {
		c.Status.InClusterZoneRef[index].InClusterIPPoolRefV4 = &poolRef
	} else if poolType == IPv6Type {
		c.Status.InClusterZoneRef[index].InClusterIPPoolRefV6 = &poolRef
	}
}

// SetInClusterIPPoolRef will set the reference to the provided InClusterIPPool.
// If nil was provided, the status field will be cleared.
func (c *ProxmoxCluster) SetInClusterIPPoolRef(pool client.Object) {
	if pool == nil || pool.GetName() == "" {
		c.Status.InClusterIPPoolRef = nil
		return
	}

	if c.Status.InClusterIPPoolRef == nil {
		c.Status.InClusterIPPoolRef = []corev1.LocalObjectReference{
			{Name: pool.GetName()},
		}
	}

	found := false
	for _, ref := range c.Status.InClusterIPPoolRef {
		if ref.Name == pool.GetName() {
			found = true
		}
	}
	if !found {
		c.Status.InClusterIPPoolRef = append(c.Status.InClusterIPPoolRef, corev1.LocalObjectReference{Name: pool.GetName()})
	}

	// also add to Zone information
	c.AddInClusterZoneRef(pool)
}

// AddNodeLocation will add a node location to either the control plane or worker
// node locations based on the isControlPlane parameter.
func (c *ProxmoxCluster) AddNodeLocation(loc NodeLocation, isControlPlane bool) {
	if c.Status.NodeLocations == nil {
		c.Status.NodeLocations = new(NodeLocations)
	}

	if !c.HasMachine(loc.Machine.Name, isControlPlane) {
		c.addNodeLocation(loc, isControlPlane)
	}
}

// RemoveNodeLocation removes a node location from the status.
func (c *ProxmoxCluster) RemoveNodeLocation(machineName string, isControlPlane bool) {
	nodeLocations := c.Status.NodeLocations

	if nodeLocations == nil {
		return
	}

	if !c.HasMachine(machineName, isControlPlane) {
		return
	}

	if isControlPlane {
		for i, v := range nodeLocations.ControlPlane {
			if v.Machine.Name == machineName {
				nodeLocations.ControlPlane = append(nodeLocations.ControlPlane[:i], nodeLocations.ControlPlane[i+1:]...)
			}
		}
		return
	}

	for i, v := range nodeLocations.Workers {
		if v.Machine.Name == machineName {
			nodeLocations.Workers = append(nodeLocations.Workers[:i], nodeLocations.Workers[i+1:]...)
		}
	}
}

// UpdateNodeLocation will update the node location based on the provided machine name.
// If the node location does not exist, it will be added.
//
// The function returns true if the value was added or updated, otherwise false.
func (c *ProxmoxCluster) UpdateNodeLocation(machineName, node string, isControlPlane bool) bool {
	if !c.HasMachine(machineName, isControlPlane) {
		loc := NodeLocation{
			Node:    node,
			Machine: corev1.LocalObjectReference{Name: machineName},
		}
		c.AddNodeLocation(loc, isControlPlane)
		return true
	}

	locations := c.Status.NodeLocations.Workers
	if isControlPlane {
		locations = c.Status.NodeLocations.ControlPlane
	}

	for i, loc := range locations {
		if loc.Machine.Name == machineName {
			if loc.Node != node {
				locations[i].Node = node
				return true
			}

			return false
		}
	}

	return false
}

// HasMachine returns if true if a machine was found on any node.
func (c *ProxmoxCluster) HasMachine(machineName string, isControlPlane bool) bool {
	return c.GetNode(machineName, isControlPlane) != ""
}

// GetNode tries to return the Proxmox node for the provided machine name.
func (c *ProxmoxCluster) GetNode(machineName string, isControlPlane bool) string {
	if c.Status.NodeLocations == nil {
		return ""
	}

	if isControlPlane {
		for _, cpl := range c.Status.NodeLocations.ControlPlane {
			if cpl.Machine.Name == machineName {
				return cpl.Node
			}
		}
	} else {
		for _, wloc := range c.Status.NodeLocations.Workers {
			if wloc.Machine.Name == machineName {
				return wloc.Node
			}
		}
	}

	return ""
}

func (c *ProxmoxCluster) addNodeLocation(loc NodeLocation, isControlPlane bool) {
	if isControlPlane {
		c.Status.NodeLocations.ControlPlane = append(c.Status.NodeLocations.ControlPlane, loc)
		return
	}

	c.Status.NodeLocations.Workers = append(c.Status.NodeLocations.Workers, loc)
}

func init() {
	objectTypes = append(objectTypes, &ProxmoxCluster{}, &ProxmoxClusterList{})
}
