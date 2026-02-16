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

package v1alpha2

import (
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterapierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// ProxmoxMachineKind is the ProxmoxMachine kind.
	ProxmoxMachineKind = "ProxmoxMachine"

	// MachineFinalizer allows cleaning up resources associated with a
	// ProxmoxMachine before removing it from the API Server.
	MachineFinalizer = "proxmoxmachine.infrastructure.cluster.x-k8s.io"

	// DefaultReconcilerRequeue is the default value for the reconcile retry.
	DefaultReconcilerRequeue = 10 * time.Second

	// DefaultNetworkDevice is the default network device name.
	DefaultNetworkDevice = "net0"

	// DefaultSuffix is the default suffix for the network device.
	DefaultSuffix = "inet"

	// IPv4Format is the IP v4 format.
	IPv4Format = "v4"

	// IPv6Format is the IP v6 format.
	IPv6Format = "v6"
)

// ProxmoxMachineChecks defines possibibles checks to skip.
type ProxmoxMachineChecks struct {
	// skipCloudInitStatus skip checking CloudInit status which can be useful with specific Operating Systems like TalOS
	// +optional
	SkipCloudInitStatus *bool `json:"skipCloudInitStatus,omitempty"`
	// skipQemuGuestAgent skips checking QEMU Agent readiness which can be useful with specific Operating Systems like TalOS
	// +optional
	SkipQemuGuestAgent *bool `json:"skipQemuGuestAgent,omitempty"`
}

// ProxmoxMachineSpec defines the desired state of a ProxmoxMachine.
type ProxmoxMachineSpec struct {
	VirtualMachineCloneSpec `json:",inline"`

	// providerID is the virtual machine BIOS UUID formatted as
	// proxmox://6c3fa683-bef9-4425-b413-eaa45a9d6191
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// virtualMachineID is the Proxmox identifier for the ProxmoxMachine VM.
	// +optional
	VirtualMachineID *int64 `json:"virtualMachineID,omitempty"`

	// numSockets is the number of CPU sockets in a virtual machine.
	// Defaults to the property value in the template from which the virtual machine is cloned.
	// +kubebuilder:validation:Minimum=1
	// +optional
	NumSockets *int32 `json:"numSockets,omitempty"`

	// numCores is the number of cores per CPU socket in a virtual machine.
	// Defaults to the property value in the template from which the virtual machine is cloned.
	// +kubebuilder:validation:Minimum=1
	// +optional
	NumCores *int32 `json:"numCores,omitempty"`

	// memoryMiB is the size of a virtual machine's memory, in MiB.
	// Defaults to the property value in the template from which the virtual machine is cloned.
	// +kubebuilder:validation:MultipleOf=8
	// +optional
	MemoryMiB *int32 `json:"memoryMiB,omitempty"`

	// disks contains a set of disk configuration options,
	// which will be applied before the first startup.
	//
	// +optional
	Disks *Storage `json:"disks,omitempty"`

	// network is the network configuration for this machine's VM.
	// +required
	//nolint:kubeapilinter
	Network *NetworkSpec `json:"network,omitzero"`
	// Justification: We need to remove this pointer, but need to coordinate with the conversion hook.

	// vmIDRange is the range of VMIDs to use for VMs.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.end >= self.start",message="end should be greater than or equal to start"
	VMIDRange *VMIDRange `json:"vmIDRange,omitempty"`

	// checks defines possible checks to skip.
	// +optional
	Checks *ProxmoxMachineChecks `json:"checks,omitempty"`

	// metadataSettings defines the metadata settings for this machine's VM.
	// +optional
	MetadataSettings *MetadataSettings `json:"metadataSettings,omitempty"`

	// allowedNodes specifies all Proxmox nodes which will be considered
	// for operations. This implies that VMs can be cloned on different nodes from
	// the node which holds the VM template.
	//
	// This field is optional and should only be set if you want to restrict
	// the nodes where the VM can be cloned.
	// If not set, the ProxmoxCluster will be used to determine the nodes.
	// +optional
	// +listType=set
	AllowedNodes []string `json:"allowedNodes,omitempty"`

	// tags is a list of tags to be applied to the virtual machine.
	// +optional
	// +immutable
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Pattern=`^(?i)[a-z0-9_][a-z0-9_\-\+\.]*$`
	Tags []string `json:"tags,omitempty"`
}

// Storage is the physical storage on the node.
type Storage struct {
	// bootVolume defines the storage size for the boot volume.
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
	// disk is the name of the disk device that should be resized.
	// Example values are: ide[0-3], scsi[0-30], sata[0-5].
	// +kubebuilder:validation:MinLength=1
	// +required
	Disk string `json:"disk,omitempty"`

	// sizeGb defines the size in gigabytes.
	//
	// As Proxmox does not support shrinking, the size
	// must be bigger than the already configured size in the
	// template.
	//
	// +kubebuilder:validation:Minimum=5
	// +required
	SizeGB int32 `json:"sizeGb,omitempty"`
}

// TargetFileStorageFormat the target format of the cloned disk.
type TargetFileStorageFormat string

// Supported disk formats.
const (
	TargetStorageFormatRaw   TargetFileStorageFormat = "raw"
	TargetStorageFormatQcow2 TargetFileStorageFormat = "qcow2"
	TargetStorageFormatVmdk  TargetFileStorageFormat = "vmdk"
)

// TemplateSource defines the source of the template VM.
type TemplateSource struct {
	// sourceNode is the initially selected proxmox node.
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
	// +optional
	SourceNode *string `json:"sourceNode,omitempty"`

	// templateID the vm_template vmid used for cloning a new VM.
	// +optional
	TemplateID *int32 `json:"templateID,omitempty"`

	// templateSelector defines MatchTags for looking up VM templates.
	// +optional
	TemplateSelector *TemplateSelector `json:"templateSelector,omitempty"`
}

// VirtualMachineCloneSpec is information used to clone a virtual machine.
// +kubebuilder:validation:XValidation:rule="self.full || !has(self.format)",message="Must set full=true when specifying format"
// +kubebuilder:validation:XValidation:rule="self.full || !has(self.storage)",message="Must set full=true when specifying storage"
type VirtualMachineCloneSpec struct {
	TemplateSource `json:",inline"`

	// description for the new VM.
	// +optional
	Description *string `json:"description,omitempty"`

	// format for file storage. Only valid for full clone.
	// +kubebuilder:validation:Enum=raw;qcow2;vmdk
	// +optional
	Format *TargetFileStorageFormat `json:"format,omitempty"`

	// full Create a full copy of all disks.
	// This is always done when you clone a normal VM.
	// Create a Full clone by default.
	// +default=true
	// +optional
	Full *bool `json:"full,omitempty"`

	// pool Add the new VM to the specified pool.
	// +optional
	Pool *string `json:"pool,omitempty"`

	// snapName The name of the snapshot.
	// +optional
	SnapName *string `json:"snapName,omitempty"`

	// storage for full clone.
	// +optional
	Storage *string `json:"storage,omitempty"`

	// target node. Only allowed if the original VM is on shared storage.
	// +optional
	Target *string `json:"target,omitempty"`
}

// TemplateSelector defines MatchTags for looking up VM templates.
type TemplateSelector struct {
	// matchTags specifies all tags to look for when looking up the VM template.
	// Passed tags must be an exact 1:1 match with the tags on the template you want to use.
	// If multiple VM templates with the same set of tags are found, provisioning will fail.
	//
	// +listType=set
	// +kubebuilder:validation:items:Pattern=`^(?i)[a-z0-9_][a-z0-9_\-\+\.]*$`
	// +kubebuilder:validation:MinItems=1
	// +required
	MatchTags []string `json:"matchTags,omitempty"`
}

// NetworkSpec defines the virtual machine's network configuration.
type NetworkSpec struct {
	// zone is the cluster deployment zone this machine will refer to.
	// +optional
	Zone Zone `json:"zone,omitempty"`

	// networkDevices is a list of network devices.
	// +required
	// +listType=map
	// +listMapKey=name
	//nolint:kubeapilinter
	NetworkDevices []NetworkDevice `json:"networkDevices,omitempty"`
	// justification: NetworkDevice legitimately consists of optional fields.

	// VirtualNetworkDevices defines virtual network devices (e.g. bridges, vlans ...).
	VirtualNetworkDevices `json:",inline"`
}

// InterfaceConfig contains all configurables a network interface can have.
type InterfaceConfig struct {
	// ipPoolRef is a reference to an IPAM Pool resource, which exposes IPv4 addresses.
	// The network device will use an available IP address from the referenced pool.
	// This can be combined with `IPv6PoolRef` in order to enable dual stack.
	// +optional
	// +kubebuilder:validation:items:XValidation:rule="self.apiGroup == 'ipam.cluster.x-k8s.io'",message="ipPoolRef allows only IPAM apiGroup ipam.cluster.x-k8s.io"
	// +kubebuilder:validation:items:XValidation:rule="self.kind == 'InClusterIPPool' || self.kind == 'GlobalInClusterIPPool'",message="ipPoolRef allows either InClusterIPPool or GlobalInClusterIPPool"
	// +listType=atomic
	IPPoolRef []corev1.TypedLocalObjectReference `json:"ipPoolRef,omitempty"`

	// dnsServers contains information about nameservers to be used for this interface.
	// If this field is not set, it will use the default dns servers from the ProxmoxCluster.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	DNSServers []string `json:"dnsServers,omitempty"`

	// Routing is the common spec of routes and routing policies to all interfaces and VRFs.
	Routing `json:",inline"`

	// linkMtu is the network device Maximum Transmission Unit.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == 1 || (self >= 576 && self <= 65520)",message="invalid MTU value"
	LinkMTU MTU `json:"linkMtu,omitempty"`
}

// Routing is shared fields across devices and VRFs.
type Routing struct {
	// routes are the routes associated with this interface.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	//nolint:kubeapilinter
	Routes []RouteSpec `json:"routes,omitempty"`
	// justification: RouteSpec legitimately consists of optional fields.

	// routingPolicy is an interface-specific policy inserted into FIB (forwarding information base).
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	//nolint:kubeapilinter
	RoutingPolicy []RoutingPolicySpec `json:"routingPolicy,omitempty"`
	// justification: RoutingPolicySpec legitimately consists of optional fields.
}

// RouteSpec describes an IPv4/IPv6 Route.
type RouteSpec struct {
	// to is the subnet to be routed.
	// +optional
	To *string `json:"to,omitempty"`
	// via is the gateway to the subnet.
	// +optional
	Via *string `json:"via,omitempty"`
	// metric is the priority of the route in the routing table.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Metric *int32 `json:"metric,omitempty"`
	// table is the routing table used for this route.
	// +optional
	Table *int32 `json:"table,omitempty"`
}

// RoutingPolicySpec is a Linux FIB rule.
type RoutingPolicySpec struct {
	// to is the subnet of the target.
	// +optional
	To *string `json:"to,omitempty"`

	// from is the subnet of the source.
	// +optional
	From *string `json:"from,omitempty"`

	// table is the routing table ID.
	// +optional
	Table *int32 `json:"table,omitempty"`

	// priority is the position in the ip rule FIB table.
	// +kubebuilder:validation:Maximum=4294967295
	// +kubebuilder:validation:XValidation:message="Cowardly refusing to insert FIB rule matching kernel rules",rule="(self > 0 && self < 32765) || (self > 32766)"
	// +optional
	Priority *int64 `json:"priority,omitempty"`
}

// VRFDevice defines Virtual Routing Flow devices.
type VRFDevice struct {
	// interfaces is the list of proxmox network devices managed by this virtual device.
	// +optional
	// +listType=atomic
	Interfaces []NetName `json:"interfaces,omitempty"`

	// name is the virtual network device name.
	// Must be unique within the virtual machine.
	// +kubebuilder:validation:MinLength=3
	// +required
	Name string `json:"name,omitempty"`

	// table is the ID of the routing table used for the l3mdev vrf device.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	// +kubebuilder:validation:XValidation:message="Cowardly refusing to insert l3mdev rules into kernel tables",rule="(self > 0 && self < 254) || (self > 255)"
	// +required
	Table int32 `json:"table,omitempty"`

	// Routing is the common spec of routes and routing policies to all interfaces and VRFs.
	Routing `json:",inline"`
}

// VirtualNetworkDevices defines Linux software networking devices.
type VirtualNetworkDevices struct {
	// vrfs defines VRF Devices.
	// +optional
	// +listType=map
	// +listMapKey=name
	VRFs []VRFDevice `json:"vrfs,omitempty"`
}

// NetworkDevice defines the required details of a virtual machine network device.
type NetworkDevice struct {
	// bridge is the network bridge to attach to the machine.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Bridge *string `json:"bridge,omitempty"`

	// defaultIPv4 attaches the ipv4 host network to this interface.
	// +optional
	DefaultIPv4 *bool `json:"defaultIPv4,omitempty"`

	// defaultIPv6 attaches the ipv6 host network to this interface.
	// +optional
	DefaultIPv6 *bool `json:"defaultIPv6,omitempty"`

	// model is the network device model.
	// +optional
	// +kubebuilder:validation:Enum=e1000;virtio;rtl8139;vmxnet3
	// +default="virtio"
	Model *string `json:"model,omitempty"`

	// mtu is the network device Maximum Transmission Unit.
	// When set to 1, virtio devices inherit the MTU value from the underlying bridge.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == 1 || (self >= 576 && self <= 65520)",message="invalid MTU value"
	MTU MTU `json:"mtu,omitempty"`

	// vlan is the network L2 VLAN.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4094
	VLAN *int32 `json:"vlan,omitempty"`

	// name is the network device name.
	// +default="net0"
	// +optional
	Name NetName `json:"name,omitempty"`

	// InterfaceConfig contains all configurables a network interface can have.
	// +optional
	InterfaceConfig `json:",inline"`
}

// MTU is the network device Maximum Transmission Unit. MTUs below 1280 break IPv6.
type MTU *int32

// ProxmoxMachineStatus defines the observed state of a ProxmoxMachine.
type ProxmoxMachineStatus struct {
	// ready indicates the Docker infrastructure has been provisioned and is ready.
	// +optional
	Ready *bool `json:"ready,omitempty"`

	// addresses contains the Proxmox VM instance associated addresses.
	// +optional
	// +listType=atomic
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// vmStatus is used to identify the virtual machine status.
	// +optional
	VMStatus *VirtualMachineState `json:"vmStatus,omitempty"`

	// bootstrapDataProvided whether the virtual machine has an injected bootstrap data.
	// +optional
	BootstrapDataProvided *bool `json:"bootstrapDataProvided,omitempty"`

	// ipAddresses are the IP addresses used to access the virtual machine.
	// +optional
	// +listType=map
	// +listMapKey=net
	IPAddresses []IPAddressesSpec `json:"ipAddresses,omitempty"`

	// network returns the network status for each of the machine's configured.
	// network interfaces.
	// +optional
	// +listType=atomic
	Network []NetworkStatus `json:"network,omitempty"`

	// proxmoxNode is the name of the proxmox node, which was chosen for this
	// machine to be deployed on.
	// +optional
	ProxmoxNode *string `json:"proxmoxNode,omitempty"`

	// taskRef is a managed object reference to a Task related to the ProxmoxMachine.
	// This value is set automatically at runtime and should not be set or
	// modified by users.
	// +optional
	TaskRef *string `json:"taskRef,omitempty"`

	// retryAfter tracks the time we can retry queueing a task.
	// +optional
	RetryAfter *metav1.Time `json:"retryAfter,omitempty"`

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
	// Any transient errors that occur during the reconciliation of ProxmoxMachines
	// can be added as events to the ProxmoxMachine object and/or logged in the
	// controller's output.
	// +optional
	FailureReason *clusterapierrors.MachineStatusError `json:"failureReason,omitempty"`

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
	// can be added as events to the ProxmoxMachine object and/or logged in the
	// controller's output.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// conditions defines current service state of the ProxmoxMachine.
	// +optional
	//nolint:kubeapilinter
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
	// Justification: kubeapilinter returns a false positive on fields called Conditions
	// because type is assumed to be metav1.Conditions.
	// This needs to wait for cluster-api.v1beta2.
}

// IPAddressesSpec stores the IP addresses of a network interface. Used for status.
type IPAddressesSpec struct {
	// net is the proxmox network name these ipaddresses are attached to.
	// +kubebuilder:validation:Pattern=`^(net[0-9]+|default)$`
	// +kubebuilder:validation:MinLength=1
	// +required
	NetName string `json:"net,omitempty"`

	// ipv4 is the IPv4 address.
	// +listType=set
	// +optional
	IPv4 []string `json:"ipv4,omitempty"`

	// ipv6 is the IPv6 address.
	// +listType=set
	// +optional
	IPv6 []string `json:"ipv6,omitempty"`
}

// VMIDRange defines the range of VMIDs to use for VMs.
type VMIDRange struct {
	// start is the start of the VMID range to use for VMs.
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:ExclusiveMinimum=false
	// +kubebuilder:validation:Maximum=999999999
	// +kubebuilder:validation:ExclusiveMaximum=false
	// +required
	Start int64 `json:"start,omitempty"`

	// end is the end of the VMID range to use for VMs.
	// Only used if VMIDRangeStart is set.
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:ExclusiveMinimum=false
	// +kubebuilder:validation:Maximum=999999999
	// +kubebuilder:validation:ExclusiveMaximum=false
	// +required
	End int64 `json:"end,omitempty"`
}

// MetadataSettings defines the metadata settings for the machine.
type MetadataSettings struct {
	// providerIDInjection enables the injection of the `providerID` into the cloudinit metadata.
	// this will basically set the `provider-id` field in the metadata to `proxmox://<instanceID>`.
	// +required
	ProviderIDInjection *bool `json:"providerIDInjection,omitempty"`
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
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the Proxmox machine spec.
	// +kubebuilder:validation:XValidation:rule="[has(self.sourceNode), has(self.templateSelector)].exists_one(c, c)",message="must define either a SourceNode with a TemplateID or a TemplateSelector"
	// +kubebuilder:validation:XValidation:rule="[has(self.templateID), has(self.templateSelector)].exists_one(c, c)",message="must define either a SourceNode with a TemplateID or a TemplateSelector"
	// +required
	Spec ProxmoxMachineSpec `json:"spec,omitzero"`

	// status is the status of the Proxmox machine.
	// +optional
	//nolint:kubeapilinter
	Status ProxmoxMachineStatus `json:"status,omitempty,omitzero"`
	// Justification: this is the paradigm used by cluster-api.
}

// +kubebuilder:object:root=true

// ProxmoxMachineList contains a list of ProxmoxMachine.
type ProxmoxMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxMachine `json:"items"`
}

// GetIPAddresses returns the ipaddress status of a machine.
func (r *ProxmoxMachine) GetIPAddresses() []IPAddressesSpec {
	return r.Status.IPAddresses
}

// GetIPAddressesNet returns the ipaddresses status for a network or nil.
func (r *ProxmoxMachine) GetIPAddressesNet(name string) *IPAddressesSpec {
	addresses := r.GetIPAddresses()
	index := slices.IndexFunc(addresses, func(s IPAddressesSpec) bool {
		return name == s.NetName
	})

	if index < 0 {
		return nil
	}
	return &addresses[index]
}

// SetIPAddresses will set the ipAddress state of a machine.
func (r *ProxmoxMachine) SetIPAddresses(ipSpec IPAddressesSpec) {
	// Allocate a default spec so the first element is never empty.
	if len(r.Status.IPAddresses) == 0 {
		r.Status.IPAddresses = append(r.Status.IPAddresses, IPAddressesSpec{
			NetName: "default",
		})
	}
	index := slices.IndexFunc(r.Status.IPAddresses, func(s IPAddressesSpec) bool {
		return ipSpec.NetName == s.NetName
	})
	if index == -1 {
		r.Status.IPAddresses = append(r.Status.IPAddresses, ipSpec)
	} else {
		r.Status.IPAddresses[index] = ipSpec
	}
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

// GetTemplateSelectorTags get the tags, the desired vm template should have.
func (r *ProxmoxMachine) GetTemplateSelectorTags() []string {
	if r.Spec.TemplateSelector != nil {
		return r.Spec.TemplateSelector.MatchTags
	}
	return nil
}

// GetNode get the Proxmox node used to provision this machine.
func (r *ProxmoxMachine) GetNode() string {
	return ptr.Deref(r.Spec.SourceNode, "")
}

// FormatSize returns the format required for the Proxmox API.
func (d *DiskSize) FormatSize() string {
	return fmt.Sprintf("%dG", d.SizeGB)
}

func init() {
	objectTypes = append(objectTypes, &ProxmoxMachine{}, &ProxmoxMachineList{})
}
