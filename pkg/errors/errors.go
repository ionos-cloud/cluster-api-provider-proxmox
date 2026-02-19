/*
Copyright 2026 IONOS Cloud.

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

// Package capmoxerrors defines error types and constants for the Proxmox provider.
// These are used to represent error states in the status of Cluster and Machine objects.
package capmoxerrors

// The following types and constants are imported from CAPI and will be removed at some point once we
// implement the conditions that will be required in CAPI v1beta2
// See https://github.com/kubernetes-sigs/cluster-api/issues/10784

// DeprecatedCAPIClusterStatusError defines errors states for Cluster objects.
type DeprecatedCAPIClusterStatusError string

const (
	// InvalidConfigurationClusterError indicates that the cluster
	// configuration is invalid.
	InvalidConfigurationClusterError DeprecatedCAPIClusterStatusError = "InvalidConfiguration"

	// UnsupportedChangeClusterError indicates that the cluster
	// spec has been updated in an unsupported way. That cannot be
	// reconciled.
	UnsupportedChangeClusterError DeprecatedCAPIClusterStatusError = "UnsupportedChange"

	// CreateClusterError indicates that an error was encountered
	// when trying to create the cluster.
	CreateClusterError DeprecatedCAPIClusterStatusError = "CreateError"

	// UpdateClusterError indicates that an error was encountered
	// when trying to update the cluster.
	UpdateClusterError DeprecatedCAPIClusterStatusError = "UpdateError"

	// DeleteClusterError indicates that an error was encountered
	// when trying to delete the cluster.
	DeleteClusterError DeprecatedCAPIClusterStatusError = "DeleteError"
)

// DeprecatedCAPIMachineStatusError defines errors states for Machine objects.
type DeprecatedCAPIMachineStatusError string

const (
	// InvalidConfigurationMachineError represents that the combination
	// of configuration in the MachineSpec is not supported by this cluster.
	// This is not a transient error, but
	// indicates a state that must be fixed before progress can be made.
	//
	// Example: the ProviderSpec specifies an instance type that doesn't exist.
	InvalidConfigurationMachineError DeprecatedCAPIMachineStatusError = "InvalidConfiguration"

	// InsufficientResourcesMachineError indicates that the machine
	// could not be created due to insufficient resources on the target host.
	InsufficientResourcesMachineError DeprecatedCAPIMachineStatusError = "InsufficientResources"

	// CreateMachineError indicates an error while trying to create a Node to match this
	// Machine. This may indicate a transient problem that will be fixed
	// automatically with time, such as a service outage, or a terminal
	// error during creation that doesn't match a more specific
	// MachineStatusError value.
	//
	// Example: timeout trying to connect to GCE.
	CreateMachineError DeprecatedCAPIMachineStatusError = "CreateError"

	// UpdateMachineError indicates an error while trying to update a Node that this
	// Machine represents. This may indicate a transient problem that will be
	// fixed automatically with time, such as a service outage,
	//
	// Example: error updating load balancers.
	UpdateMachineError DeprecatedCAPIMachineStatusError = "UpdateError"

	// DeleteMachineError indicates an error was encountered while trying to delete the Node that this
	// Machine represents. This could be a transient or terminal error, but
	// will only be observable if the provider's Machine controller has
	// added a finalizer to the object to more gracefully handle deletions.
	//
	// Example: cannot resolve EC2 IP address.
	DeleteMachineError DeprecatedCAPIMachineStatusError = "DeleteError"
)
