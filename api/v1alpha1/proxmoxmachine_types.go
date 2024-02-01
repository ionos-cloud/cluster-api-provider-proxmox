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

package v1alpha1

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/errors"
)

const (
	// ProxmoxMachineKind is the ProxmoxMachine kind.
	ProxmoxMachineKind = "ProxmoxMachine"

	// MachineFinalizer allows cleaning up resources associated with
	// ProxmoxMachine before removing it from the API Server.
	MachineFinalizer = "proxmoxmachine.infrastructure.cluster.x-k8s.io"

	// DefaultReconcilerRequeue is the default value for the reconcile retry.
	DefaultReconcilerRequeue = 10 * time.Second

	// DefaultNetworkDevice is the default network device name.
	DefaultNetworkDevice = "net0"

	// DefaultSuffix is the default suffix for the network device.
	DefaultSuffix = "inet"

	// IPV4Format is the IP v4 format.
	IPV4Format = "v4"

	// IPV6Format is the IP v6 format.
	IPV6Format = "v6"
)

// ProxmoxMachineSpec defines the desired state of ProxmoxMachine.
type ProxmoxMachineSpec struct {
	VirtualMachineCloneSpec `json:",inline"`

	// ProviderID is the virtual machine BIOS UUID formatted as
	// proxmox://6c3fa683-bef9-4425-b413-eaa45a9d6191
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// VirtualMachineID is the Proxmox identifier for the ProxmoxMachine vm.
	// +optional
	VirtualMachineID *int64 `json:"virtualMachineID,omitempty"`

	// NumSockets is the number of CPU sockets in a virtual machine.
	// Defaults to the property value in the template from which the virtual machine is cloned.
	// +kubebuilder:validation:Minimum=1
	// +optional
	NumSockets int32 `json:"numSockets,omitempty"`

	// NumCores is the number of cores per CPU socket in a virtual machine.
	// Defaults to the property value in the template from which the virtual machine is cloned.
	// +kubebuilder:validation:Minimum=1
	// +optional
	NumCores int32 `json:"numCores,omitempty"`

	// MemoryMiB is the size of a virtual machine's memory, in MiB.
	// Defaults to the property value in the template from which the virtual machine is cloned.
	// +kubebuilder:validation:MultipleOf=8
	// +optional
	MemoryMiB int32 `json:"memoryMiB,omitempty"`

	// Disks contains a set of disk configuration options,
	// which will be applied before the first startup.
	//
	// +optional
	Disks *Storage `json:"disks,omitempty"`

	// Network is the network configuration for this machine's VM.
	// +optional
	Network *NetworkSpec `json:"network,omitempty"`
}

// Storage is the physical storage on the node.
type Storage struct {
	// BootVolume defines the storage size for the boot volume.
	// This field is optional, and should only be set if you want
	// to change the size of the boot volume.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +optional
	BootVolume *DiskSize `json:"bootVolume,omitempty"`

	// TODO Intended to add handling for additional volumes,
	// which will be added to the node.
	// e.g. AdditionalVolumes []DiskSize.
}

// DiskSize is contains values for the disk device and size.
type DiskSize struct {
	// Disk is the name of the disk device, that should be resized.
	// Example values are: ide[0-3], scsi[0-30], sata[0-5].
	Disk string `json:"disk"`

	// Size defines the size in gigabyte.
	//
	// As Proxmox does not support shrinking, the size
	// must be bigger than the already configured size in the
	// template.
	//
	// +kubebuilder:validation:Minimum=5
	SizeGB int32 `json:"sizeGb"`
}

// TargetFileStorageFormat the target format of the cloned disk.
type TargetFileStorageFormat string

// Supported disk formats.
const (
	TargetStorageFormatRaw   TargetFileStorageFormat = "raw"
	TargetStorageFormatQcow2 TargetFileStorageFormat = "qcow2"
	TargetStorageFormatVmdk  TargetFileStorageFormat = "vmdk"
)

// VirtualMachineCloneSpec is information used to clone a virtual machine.
type VirtualMachineCloneSpec struct {
	// SourceNode is the initially selected proxmox node.
	// This node will be used to locate the template VM, which will
	// be used for cloning operations.
	//
	// Cloning will be performed according to the configuration.
	// Setting the `Target` field will tell Proxmox to clone the
	// VM on that target node.
	//
	// When Target is not set and the ProxmoxCluster contains
	// a set of `AllowedNodes`, the algorithm will instead evenly
	// distribute the VMs across the nodes from that list.
	//
	// If neither a `Target` nor `AllowedNodes` was set, the VM
	// will be cloned onto the same node as SourceNode.
	//
	// +kubebuilder:validation:MinLength=1
	SourceNode string `json:"sourceNode"`

	// TemplateID the vm_template vmid used for cloning a new VM.
	// +optional
	TemplateID *int32 `json:"templateID,omitempty"`

	// Description for the new VM.
	// +optional
	Description *string `json:"description,omitempty"`

	// Format for file storage. Only valid for full clone.
	// +kubebuilder:validation:Enum=raw;qcow2;vmdk
	// +kubebuilder:default=raw
	// +optional
	Format *TargetFileStorageFormat `json:"format,omitempty"`

	// Full Create a full copy of all disks.
	// This is always done when you clone a normal VM.
	// Create a Full clone by default.
	// +kubebuilder:default=true
	// +optional
	Full *bool `json:"full,omitempty"`

	// Pool Add the new VM to the specified pool.
	// +optional
	Pool *string `json:"pool,omitempty"`

	// SnapName The name of the snapshot.
	// +optional
	SnapName *string `json:"snapName,omitempty"`

	// Storage for full clone.
	// +optional
	Storage *string `json:"storage,omitempty"`

	// Target node. Only allowed if the original VM is on shared storage.
	// +optional
	Target *string `json:"target,omitempty"`
}

// NetworkSpec defines the virtual machine's network configuration.
type NetworkSpec struct {
	// Default is the default network device,
	// which will be used for the primary network interface.
	// net0 is always the default network device.
	// +optional
	Default *NetworkDevice `json:"default,omitempty"`

	// AdditionalDevices defines additional network devices bound to the virtual machine.
	// +optional
	// +listType=map
	// +listMapKey=name
	AdditionalDevices []AdditionalNetworkDevice `json:"additionalDevices,omitempty"`

	// VirtualNetworkDevices defines virtual network devices (e.g. bridges, vlans ...).
	VirtualNetworkDevices `json:",inline"`
}

// InterfaceConfig contains all configurables a network interface can have.
type InterfaceConfig struct {
	// IPv4PoolRef is a reference to an IPAM Pool resource, which exposes IPv4 addresses.
	// The network device will use an available IP address from the referenced pool.
	// This can be combined with `IPv6PoolRef` in order to enable dual stack.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.apiGroup == 'ipam.cluster.x-k8s.io'",message="ipv4PoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io"
	// +kubebuilder:validation:XValidation:rule="self.kind == 'InClusterIPPool' || self.kind == 'GlobalInClusterIPPool'",message="ipv4PoolRef allows either InClusterIPPool or GlobalInClusterIPPool"
	IPv4PoolRef *corev1.TypedLocalObjectReference `json:"ipv4PoolRef,omitempty"`

	// IPv6PoolRef is a reference to an IPAM pool resource, which exposes IPv6 addresses.
	// The network device will use an available IP address from the referenced pool.
	// this can be combined with `IPv4PoolRef` in order to enable dual stack.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.apiGroup == 'ipam.cluster.x-k8s.io'",message="ipv6PoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io"
	// +kubebuilder:validation:XValidation:rule="self.kind == 'InClusterIPPool' || self.kind == 'GlobalInClusterIPPool'",message="ipv6PoolRef allows either InClusterIPPool or GlobalInClusterIPPool"
	IPv6PoolRef *corev1.TypedLocalObjectReference `json:"ipv6PoolRef,omitempty"`

	// DNSServers contains information about nameservers to be used for this interface.
	// If this field is not set, it will use the default dns servers from the ProxmoxCluster.
	// +optional
	// +kubebuilder:validation:MinItems=1
	DNSServers []string `json:"dnsServers,omitempty"`
}

// RouteSpec describes an IPv4/IPv6 Route.
type RouteSpec struct {
	// To is the subnet to be routed.
	// +optional
	To string `json:"to,omitempty"`
	// Via is the gateway to the subnet.
	// +optional
	Via string `json:"via,omitempty"`
	// Metric is the priority of the route in the routing table.
	// +optional
	Metric uint32 `json:"metric,omitempty"`
	// Table is the routing table used for this route.
	// +optional
	Table uint32 `json:"table,omitempty"`
}

// RoutingPolicySpec is a linux FIB rule.
type RoutingPolicySpec struct {
	// To is the subnet of the target.
	// +optional
	To string `json:"to,omitempty"`

	// From is the subnet of the source.
	// +optional
	From string `json:"from,omitempty"`

	// Table is the routing table id.
	// +optional
	Table uint32 `json:"table,omitempty"`

	// Priority is the position in the ip rule fib table.
	// +kubebuilder:validation:Maximum=4294967295
	// +kubebuilder:validation:XValidation:message="Cowardly refusing to insert fib rule matching kernel rules",rule="(self > 0 && self < 32765) || (self > 32766)"
	// +optional
	Priority uint32 `json:"priority,omitempty"`
}

// VRFDevice defines Virtual Routing Flow devices.
type VRFDevice struct {
	// Interfaces is the list of proxmox network devices managed by this virtual device.
	Interfaces []string `json:"interfaces,omitempty"`

	// Name is the virtual network device name.
	// must be unique within the virtual machine.
	// +optional
	// +kubebuilder:validation:MinLength=3
	Name string `json:"name"`

	// Table is the ID of the routing table used for the l3mdev vrf device.
	// +kubebuilder:validation:Maximum=4294967295
	// +kubebuilder:validation:XValidation:message="Cowardly refusing to insert l3mdev rules into kernel tables",rule="(self > 0 && self < 254) || (self > 255)"
	Table uint32 `json:"table"`

	// InterfaceConfig contains all configurables a network interface can have.
	// +optional
	InterfaceConfig `json:",inline"`

	// Routes are the routes associated with the l3mdev policy.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Routes []RouteSpec `json:"routes,omitempty"`

	// RoutingPolicy is the l3mdev policy inserted into FiB.
	// +optional
	// +kubebuilder:validation:MinItems=1
	RoutingPolicy []RoutingPolicySpec `json:"routingPolicy,omitempty"`
}

// VirtualNetworkDevices defines linux software networking devices.
type VirtualNetworkDevices struct {
	// Definition of a Vrf Device.
	// +optional
	VRFs []VRFDevice `json:"vrfs,omitempty"`
}

// NetworkDevice defines the required details of a virtual machine network device.
type NetworkDevice struct {
	// Bridge is the network bridge to attach to the machine.
	// +kubebuilder:validation:MinLength=1
	Bridge string `json:"bridge"`

	// Model is the network device model.
	// +optional
	// +kubebuilder:validation:Enum=e1000;virtio;rtl8139;vmxnet3
	// +kubebuilder:default=virtio
	Model *string `json:"model,omitempty"`

	// MTU is the network device Maximum Transmission Unit.
	// Only works with virtio Model.
	// Set to 1 to inherit the MTU value from the underlying bridge.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65520
	MTU *uint16 `json:"mtu,omitempty"`
}

// AdditionalNetworkDevice the definition of a Proxmox network device.
// +kubebuilder:validation:XValidation:rule="self.ipv4PoolRef != null || self.ipv6PoolRef != null",message="at least one pool reference must be set, either ipv4PoolRef or ipv6PoolRef"
type AdditionalNetworkDevice struct {
	NetworkDevice `json:",inline"`

	// Name is the network device name.
	// must be unique within the virtual machine and different from the primary device 'net0'.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self != 'net0'",message="additional network devices doesn't allow net0"
	Name string `json:"name"`

	// IPv4PoolRef is a reference to an IPAM Pool resource, which exposes IPv4 addresses.
	// The network device will use an available IP address from the referenced pool.
	// This can be combined with `IPv6PoolRef` in order to enable dual stack.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.apiGroup == 'ipam.cluster.x-k8s.io'",message="ipv4PoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io"
	// +kubebuilder:validation:XValidation:rule="self.kind == 'InClusterIPPool' || self.kind == 'GlobalInClusterIPPool'",message="ipv4PoolRef allows either InClusterIPPool or GlobalInClusterIPPool"
	IPv4PoolRef *corev1.TypedLocalObjectReference `json:"ipv4PoolRef,omitempty"`

	// IPv6PoolRef is a reference to an IPAM pool resource, which exposes IPv6 addresses.
	// The network device will use an available IP address from the referenced pool.
	// this can be combined with `IPv4PoolRef` in order to enable dual stack.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.apiGroup == 'ipam.cluster.x-k8s.io'",message="ipv6PoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io"
	// +kubebuilder:validation:XValidation:rule="self.kind == 'InClusterIPPool' || self.kind == 'GlobalInClusterIPPool'",message="ipv6PoolRef allows either InClusterIPPool or GlobalInClusterIPPool"
	IPv6PoolRef *corev1.TypedLocalObjectReference `json:"ipv6PoolRef,omitempty"`

	// DNSServers contains information about nameservers to be used for this interface.
	// If this field is not set, it will use the default dns servers from the ProxmoxCluster.
	// +optional
	// +kubebuilder:validation:MinItems=1
	DNSServers []string `json:"dnsServers,omitempty"`
}

// ProxmoxMachineStatus defines the observed state of ProxmoxMachine.
type ProxmoxMachineStatus struct {
	// Ready indicates the Docker infrastructure has been provisioned and is ready.
	// +optional
	Ready bool `json:"ready"`

	// Addresses contains the Proxmox VM instance associated addresses.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// VMStatus is used to identify the virtual machine status.
	// +optional
	VMStatus VirtualMachineState `json:"vmStatus,omitempty"`

	// BootstrapDataProvided whether the virtual machine has an injected bootstrap data.
	// +optional
	BootstrapDataProvided *bool `json:"bootstrapDataProvided,omitempty"`

	// IPAddresses are the IP addresses used to access the virtual machine.
	// +optional
	IPAddresses map[string]IPAddress `json:"ipAddresses,omitempty"`

	// Network returns the network status for each of the machine's configured.
	// network interfaces.
	// +optional
	Network []NetworkStatus `json:"network,omitempty"`

	// ProxmoxNode is the name of the proxmox node, which was chosen for this
	// machine to be deployed on.
	// +optional
	ProxmoxNode *string `json:"proxmoxNode,omitempty"`

	// TaskRef is a managed object reference to a Task related to the ProxmoxMachine.
	// This value is set automatically at runtime and should not be set or
	// modified by users.
	// +optional
	TaskRef *string `json:"taskRef,omitempty"`

	// RetryAfter tracks the time we can retry queueing a task.
	// +optional
	RetryAfter metav1.Time `json:"retryAfter,omitempty"`

	// FailureReason will be set in the event that there is a terminal problem
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
	// Any transient errors that occur during the reconciliation of ProxmoxMachines
	// can be added as events to the ProxmoxMachine object and/or logged in the
	// controller's output.
	// +optional
	FailureReason *errors.MachineStatusError `json:"failureReason,omitempty"`

	// FailureMessage will be set in the event that there is a terminal problem
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
	// can be added as events to the ProxmoxMachine object and/or logged in the
	// controller's output.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Conditions defines current service state of the ProxmoxMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// IPAddress defines the IP addresses of a network interface.
type IPAddress struct {
	// IPV4 is the IP v4 address.
	// +optional
	IPV4 string `json:"ipv4,omitempty"`

	// IPV6 is the IP v6 address.
	// +optional
	IPV6 string `json:"ipv6,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=proxmoxmachines,scope=Namespaced,categories=cluster-api;proxmox,shortName=moxm
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this ProxmoxMachine belongs"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="Node",type="string",JSONPath=".status.proxmoxNode",description="Proxmox Node that the machine was deployed on"
// +kubebuilder:printcolumn:name="Provider_ID",type="string",JSONPath=".spec.providerID",description="Provider ID"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this ProxmoxMachine"

// ProxmoxMachine is the Schema for the proxmoxmachines API.
type ProxmoxMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:XValidation:rule="self.full && self.format != ''",message="Must set full=true when specifying format"
	Spec   ProxmoxMachineSpec   `json:"spec,omitempty"`
	Status ProxmoxMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProxmoxMachineList contains a list of ProxmoxMachine.
type ProxmoxMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxMachine `json:"items"`
}

// GetConditions returns the observations of the operational state of the ProxmoxMachine resource.
func (r *ProxmoxMachine) GetConditions() clusterv1.Conditions {
	return r.Status.Conditions
}

// SetConditions sets the underlying service state of the ProxmoxMachine to the predescribed clusterv1.Conditions.
func (r *ProxmoxMachine) SetConditions(conditions clusterv1.Conditions) {
	r.Status.Conditions = conditions
}

// GetVirtualMachineID get the Proxmox "vmid".
func (r *ProxmoxMachine) GetVirtualMachineID() int64 {
	if r.Spec.VirtualMachineID != nil {
		return *r.Spec.VirtualMachineID
	}
	return -1
}

// GetTemplateID get the Proxmox template "vmid" used to provision this machine.
func (r *ProxmoxMachine) GetTemplateID() int32 {
	if r.Spec.TemplateID != nil {
		return *r.Spec.TemplateID
	}
	return -1
}

// GetNode get the Proxmox node used to provision this machine.
func (r *ProxmoxMachine) GetNode() string {
	return r.Spec.SourceNode
}

// FormatSize returns the format required for the Proxmox API.
func (d *DiskSize) FormatSize() string {
	return fmt.Sprintf("%dG", d.SizeGB)
}

func init() {
	objectTypes = append(objectTypes, &ProxmoxMachine{}, &ProxmoxMachineList{})
}
