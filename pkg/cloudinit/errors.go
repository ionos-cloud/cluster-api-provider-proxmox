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

package cloudinit

import (
	"github.com/pkg/errors"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

var (
	// ErrMissingHostname returns an error if required hostname is empty.
	ErrMissingHostname = errors.New("hostname is not set")

	// ErrMissingInstanceID returns an error if required hostname is empty.
	ErrMissingInstanceID = errors.New("instance-id is not set")

	// The following are structural errors shared with other renderers; they
	// live in pkg/network and are re-exported here for backwards compatibility.

	// ErrMissingGateway returns an error if required gateway is empty.
	ErrMissingGateway = network.ErrMissingGateway

	// ErrConflictingMetrics returns an error if a metric for a route already exists.
	ErrConflictingMetrics = network.ErrConflictingMetrics

	// ErrMissingNetworkConfigData returns an error if required network config data is empty.
	ErrMissingNetworkConfigData = network.ErrMissingNetworkConfigData

	// ErrMalformedRoute is returned if a route can not be assembled by netplan.
	ErrMalformedRoute = network.ErrMalformedRoute

	// ErrMalformedFIBRule is returned if a FIB rule can not be assembled by netplan.
	ErrMalformedFIBRule = network.ErrMalformedFIBRule
)
