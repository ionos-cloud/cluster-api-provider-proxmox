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

// Conditions and Reasons for ProxmoxCluster.
//
// The Ready condition is a summary condition that is set by the controller using
// conditions.SetSummaryCondition and aggregates the following conditions:
// - ProxmoxAvailable
// - Paused (managed by CAPI).
const (
	// ProxmoxClusterProxmoxAvailableCondition documents the availability of the
	// underlying Proxmox infrastructure used by the ProxmoxCluster.
	ProxmoxClusterProxmoxAvailableCondition = "ProxmoxAvailable"

	// ProxmoxClusterProxmoxAvailableProxmoxUnreachableReason documents a controller
	// detecting issues with Proxmox reachability.
	ProxmoxClusterProxmoxAvailableProxmoxUnreachableReason = "ProxmoxUnreachable"

	// ProxmoxClusterProxmoxAvailableMissingControlPlaneEndpointReason documents a
	// missing Control Plane endpoint when the cluster uses an externally managed
	// control plane.
	ProxmoxClusterProxmoxAvailableMissingControlPlaneEndpointReason = "MissingControlPlaneEndpoint"

	// ProxmoxClusterProxmoxAvailableDeletingReason documents a ProxmoxCluster being deleted.
	ProxmoxClusterProxmoxAvailableDeletingReason = "Deleting"
)

// Conditions and Reasons for ProxmoxMachine.
//
// The Ready condition is a summary condition that is set by the controller using
// conditions.SetSummaryCondition and aggregates the following conditions:
// - VirtualMachineProvisioned
// - Paused (managed by CAPI).
const (
	// ProxmoxMachineVirtualMachineProvisionedCondition documents the status of the
	// provisioning of a ProxmoxMachine and its underlying virtual machine.
	ProxmoxMachineVirtualMachineProvisionedCondition = "VirtualMachineProvisioned"

	// ProxmoxMachineVirtualMachineProvisionedCloningReason documents a ProxmoxMachine
	// currently executing the clone operation.
	ProxmoxMachineVirtualMachineProvisionedCloningReason = "Cloning"

	// ProxmoxMachineVirtualMachineProvisionedCloningFailedReason documents a ProxmoxMachine
	// controller detecting an error while cloning; these errors are usually transient
	// and the controller automatically retries.
	ProxmoxMachineVirtualMachineProvisionedCloningFailedReason = "CloningFailed"

	// ProxmoxMachineVirtualMachineProvisionedWaitingForDiskReconciliationReason documents
	// a ProxmoxMachine waiting for the disks to be resized.
	ProxmoxMachineVirtualMachineProvisionedWaitingForDiskReconciliationReason = "WaitingForDiskReconciliation"

	// ProxmoxMachineVirtualMachineProvisionedWaitingForStaticIPAllocationReason documents
	// a ProxmoxMachine waiting for the allocation of a static IP address.
	ProxmoxMachineVirtualMachineProvisionedWaitingForStaticIPAllocationReason = "WaitingForStaticIPAllocation"

	// ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapDataReconciliationReason
	// documents a ProxmoxMachine waiting for the reconciliation of bootstrap data
	// for cloud-init/ignition.
	ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapDataReconciliationReason = "WaitingForBootstrapDataReconciliation"

	// ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason documents a
	// ProxmoxMachine waiting for Proxmox to power it on.
	ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason = "WaitingForVMPowerUp"

	// ProxmoxMachineVirtualMachineProvisionedPoweringOnFailedReason documents a
	// ProxmoxMachine controller detecting an error while powering on; these errors
	// are usually transient and the controller automatically retries.
	ProxmoxMachineVirtualMachineProvisionedPoweringOnFailedReason = "PoweringOnFailed"

	// ProxmoxMachineVirtualMachineProvisionedWaitingForCloudInitReason documents a
	// ProxmoxMachine waiting for cloud-init to complete.
	ProxmoxMachineVirtualMachineProvisionedWaitingForCloudInitReason = "WaitingForCloudInit"

	// ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapReadyReason documents
	// a ProxmoxMachine waiting for the bootstrap process to complete.
	ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapReadyReason = "WaitingForBootstrapReady"

	// ProxmoxMachineVirtualMachineProvisionedWaitingForClusterAPIMachineAddressesReason
	// documents a ProxmoxMachine assigning host addresses for Cluster API.
	ProxmoxMachineVirtualMachineProvisionedWaitingForClusterAPIMachineAddressesReason = "WaitingForClusterAPIMachineAddresses"

	// ProxmoxMachineVirtualMachineProvisionedVMProvisionFailedReason documents a failure
	// during virtual machine provisioning.
	ProxmoxMachineVirtualMachineProvisionedVMProvisionFailedReason = "VMProvisionFailed"

	// ProxmoxMachineVirtualMachineProvisionedTaskFailedReason documents a Proxmox task
	// failure; the controller will automatically retry, but user intervention might
	// be required.
	ProxmoxMachineVirtualMachineProvisionedTaskFailedReason = "TaskFailed"

	// ProxmoxMachineVirtualMachineProvisionedDeletingReason documents a ProxmoxMachine
	// being deleted.
	ProxmoxMachineVirtualMachineProvisionedDeletingReason = "Deleting"

	// ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason documents a failure
	// during virtual machine deletion.
	ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason = "DeletionFailed"

	// ProxmoxMachineVirtualMachineProvisionedWaitingForVirtualMachineConfigReason documents
	// a ProxmoxMachine waiting for VirtualMachineConfig.
	ProxmoxMachineVirtualMachineProvisionedWaitingForVirtualMachineConfigReason = "WaitingForVirtualMachineConfig"
)
