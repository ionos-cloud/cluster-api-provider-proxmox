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

import clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1" //nolint:staticcheck

const (
	// VMProvisionedCondition documents the status of the provisioning of a ProxmoxMachine and its underlying ProxmoxVM.
	VMProvisionedCondition clusterv1.ConditionType = "VMProvisioned"

	// VMProvisionFailedReason used for failures during instance provisioning.
	VMProvisionFailedReason = "VMProvisionFailed"

	// VMTerminatedReason used when vm is being terminated.
	VMTerminatedReason = "VMTerminated"

	// WaitingForClusterInfrastructureReason (Severity=Info) documents a ProxmoxMachine waiting for the cluster
	// infrastructure to be ready before starting the provisioning process.
	//
	// NOTE: This reason does not apply to ProxmoxVM (this state happens before the ProxmoxVM is actually created).
	WaitingForClusterInfrastructureReason = "WaitingForClusterInfrastructure"

	// WaitingForBootstrapDataReason (Severity=Info) documents a ProxmoxMachine waiting for the bootstrap
	// script to be ready before starting the provisioning process.
	//
	// NOTE: This reason does not apply to ProxmoxVM (this state happens before the ProxmoxVM is actually created).
	WaitingForBootstrapDataReason = "WaitingForBootstrapData"

	// WaitingForStaticIPAllocationReason (Severity=Info) documents a ProxmoxVM waiting for the allocation of
	// a static IP address.
	WaitingForStaticIPAllocationReason = "WaitingForStaticIPAllocation"

	// CloningReason documents (Severity=Info) a ProxmoxMachine/ProxmoxVM currently executing the clone operation.
	CloningReason = "Cloning"

	// CloningFailedReason (Severity=Warning) documents a ProxmoxMachine/ProxmoxVM controller detecting
	// an error while provisioning; those kind of errors are usually transient and failed provisioning
	// are automatically re-tried by the controller.
	CloningFailedReason = "CloningFailed"

	// PoweringOnReason documents (Severity=Info) a ProxmoxMachine/ProxmoxVM currently executing the power on sequence.
	PoweringOnReason = "PoweringOn"

	// PoweringOnFailedReason (Severity=Warning) documents a ProxmoxMachine/ProxmoxVM controller detecting
	// an error while powering on; those kind of errors are usually transient and failed provisioning
	// are automatically re-tried by the controller.
	PoweringOnFailedReason = "PoweringOnFailed"

	// VMProvisionStarted used for starting vm provisioning.
	VMProvisionStarted = "VMProvisionStarted"

	// TaskFailure (Severity=Warning) documents a ProxmoxMachine/Proxmox task failure; the reconcile look will automatically
	// retry the operation, but a user intervention might be required to fix the problem.
	TaskFailure = "TaskFailure"

	// WaitingForNetworkAddressesReason (Severity=Info) documents a ProxmoxMachine waiting for the the machine network
	// settings to be reported after machine being powered on.
	//
	// NOTE: This reason does not apply to ProxmoxVM (this state happens after the ProxmoxVM is in ready state).
	WaitingForNetworkAddressesReason = "WaitingForNetworkAddresses"

	// NotFoundReason (Severity=Warning) documents the ProxmoxVM not found.
	NotFoundReason = "NotFound"

	// UnknownReason (Severity=Warning) documents the ProxmoxVM Unknown.
	UnknownReason = "Unknown"

	// MissingControlPlaneEndpointReason (Severity=Warning) documents the missing Control Plane endpoint when Cluster is backed by an externally managed Control Plane.
	MissingControlPlaneEndpointReason = "MissingControlPlaneEndpoint"
)

const (
	// ProxmoxClusterReady documents the status of ProxmoxCluster and its underlying resources.
	ProxmoxClusterReady clusterv1.ConditionType = "ClusterReady"

	// ProxmoxUnreachableReason (Severity=Error) documents a controller detecting
	// issues with Proxmox reachability.
	ProxmoxUnreachableReason = "ProxmoxUnreachable"
)
