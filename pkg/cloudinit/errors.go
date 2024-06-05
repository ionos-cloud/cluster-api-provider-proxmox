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

import "github.com/pkg/errors"

var (
	// ErrMissingHostname returns an error if required hostname is empty.
	ErrMissingHostname = errors.New("hostname is not set")

	// ErrMissingInstanceID returns an error if required hostname is empty.
	ErrMissingInstanceID = errors.New("instance-id is not set")

	// ErrMissingIPAddress returns an error if required ip address is empty.
	ErrMissingIPAddress = errors.New("ip address is not set")

	// ErrMalformedIPAddress returns an error if ip address is malformed.
	ErrMalformedIPAddress = errors.New("malformed ip address")

	// ErrMissingGateway returns an error if required gateway is empty.
	ErrMissingGateway = errors.New("gateway is not set")

	// ErrConflictingMetrics returns an error if a metric for a route already exists.
	ErrConflictingMetrics = errors.New("metric already exists for default gateway")

	// ErrMissingMacAddress returns an error if required mac address is empty.
	ErrMissingMacAddress = errors.New("mac address is not set")

	// ErrMissingNetworkConfigData returns an error if required network config data is empty.
	ErrMissingNetworkConfigData = errors.New("network config data is not set")

	// ErrMissingIPAddresses returns an error if required ip addresses is empty.
	ErrMissingIPAddresses = errors.New("ip addresses is not set")

	// ErrMalformedRoute is returned if a route can not be assembled by netplan.
	ErrMalformedRoute = errors.New("route is malformed")

	// ErrMalformedFIBRule is returned if a FIB rule can not be assembled by netplan.
	ErrMalformedFIBRule = errors.New("routing policy is malformed")
)
