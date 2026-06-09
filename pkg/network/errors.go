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

package network

import "errors"

// These errors describe structural problems with a Network that hold
// regardless of which renderer consumes it. Renderer specific validation
// (e.g. missing MAC addresses) live with the renderer.
var (
	// ErrMissingNetworkConfigData is returned if there is no network config data.
	ErrMissingNetworkConfigData = errors.New("network config data is not set")

	// ErrMissingGateway is returned if no device contributes a default gateway.
	ErrMissingGateway = errors.New("gateway is not set")

	// ErrConflictingMetrics is returned if two routes collide within the same
	// routing table.
	ErrConflictingMetrics = errors.New("route already exists for destination/metric in routing table")
	// ErrMalformedRoute is returned if a route can not be assembled.
	ErrMalformedRoute = errors.New("route is malformed")

	// ErrMalformedFIBRule is returned if a FIB rule can not be assembled.
	ErrMalformedFIBRule = errors.New("routing policy is malformed")
)
