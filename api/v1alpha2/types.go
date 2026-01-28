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

package v1alpha2

// VirtualMachineState describes the state of a VM.
type VirtualMachineState string

const (
	// VirtualMachineStateNotFound is the string representing a VM that
	// cannot be located.
	VirtualMachineStateNotFound VirtualMachineState = "notfound"

	// VirtualMachineStatePending is the string representing a VM with an in-flight task.
	VirtualMachineStatePending VirtualMachineState = "pending"

	// VirtualMachineStateReady is the string representing a powered-on VM with reported IP addresses.
	VirtualMachineStateReady VirtualMachineState = "ready"

	// ProxmoxZoneLabel is a label key used for proxmox zone objects.
	ProxmoxZoneLabel string = "topology.kubernetes.io/proxmox-zone"

	// ProxmoxIPFamilyAnnotation is an annotation key used for denoting the ip family of InClusterIPPools.
	ProxmoxIPFamilyAnnotation string = "ipam.capmox.cluster.x-k8s.io/ip-family"

	// IPv4Type marks an object as belong to the IPv4 family.
	IPv4Type string = "ipv4"

	// IPv6Type marks an object as belong to the IPv6 family.
	IPv6Type string = "ipv6"

	// ProxmoxPoolRefCounterAnnotation allows multiple IP Addresses per IPPool.
	ProxmoxPoolRefCounterAnnotation string = "ipam.capmox.cluster.x-k8s.io/pool-ref-counter"

	// ProxmoxGatewayMetricAnnotation is an annotation key used for denoting metric of a gateway.
	ProxmoxGatewayMetricAnnotation string = "ipam.capmox.cluster.x-k8s.io/gateway-metric"

	// ProxmoxDefaultGatewayAnnotation marks an IPAddress spec as containing a default gateway.
	ProxmoxDefaultGatewayAnnotation string = "ipam.capmox.cluster.x-k8s.io/default-gateway"
)

// VirtualMachine represents data about a Proxmox virtual machine object.
type VirtualMachine struct {
	// node is the VM node.
	// +required
	// +kubebuilder:validation:MinLength=1
	Node string `json:"node,omitempty"`

	// name is the VM's name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`

	// vmID is the VM's ID.
	// +required
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:ExclusiveMinimum=false
	VMID int64 `json:"vmID,omitempty"`

	// state is the VM's state.
	// +required
	// +kubebuilder:validation:Enum=notfound;pending;ready
	State VirtualMachineState `json:"state,omitempty"`

	// network is the status of the VM's network devices.
	// +required
	// +listType=atomic
	Network []NetworkStatus `json:"network,omitempty"`
}

// NetworkStatus provides information about one of a VM's networks.
type NetworkStatus struct {
	// connected is a flag that indicates whether this network is currently
	// connected to the VM.
	// +required
	Connected *bool `json:"connected,omitempty"`

	// ipAddrs is one or more IP addresses reported by vm-tools.
	// +listType=set
	// +optional
	IPAddrs []string `json:"ipAddrs,omitempty"`

	// macAddr is the MAC address of the network device.
	// +required
	// +kubebuilder:validation:Pattern=`^([0-9A-Fa-f]{2}[:]){5}([0-9A-Fa-f]{2})$`
	// +kubebuilder:validation:MinLength=17
	// +kubebuilder:validation:MaxLength=17
	MACAddr string `json:"macAddr,omitempty"`

	// networkName is the name of the network.
	// +optional
	NetworkName NetName `json:"networkName,omitempty"`
}

// NetName is a formally verified Proxmox network name string.
// +kubebuilder:validation:Pattern=`^net[0-9]+$`
type NetName *string

// Zone is a formally verified Proxmox network zone name. Needs to adhere to Label rules.
// +kubebuilder:validation:Pattern=`^[a-z0-9A-Z](?:[a-z0-9A-Z-_.]{0,61}[a-z0-9A-Z])?$`
type Zone *string
