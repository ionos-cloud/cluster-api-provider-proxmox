/*
Copyright 2024-2026 IONOS Cloud.

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

package ignition

import (
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
)

var (
	// The following are structural errors shared with other renderers; they
	// live in pkg/network and are re-exported here for convenience.

	// ErrMissingGateway returns an error if no device contributes a default gateway.
	ErrMissingGateway = network.ErrMissingGateway

	// ErrConflictingMetrics returns an error if a metric for a route already exists.
	ErrConflictingMetrics = network.ErrConflictingMetrics

	// ErrMissingNetworkConfigData returns an error if required network config data is empty.
	ErrMissingNetworkConfigData = network.ErrMissingNetworkConfigData

	// ErrMalformedRoute is returned if a route can not be assembled.
	ErrMalformedRoute = network.ErrMalformedRoute

	// ErrMalformedFIBRule is returned if a FIB rule can not be assembled.
	ErrMalformedFIBRule = network.ErrMalformedFIBRule
)
