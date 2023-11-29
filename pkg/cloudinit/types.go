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

package cloudinit

// BaseCloudInitData is shared across all the various types of files written to disk.
// used to render cloudinit.
type BaseCloudInitData struct {
	Hostname          string
	InstanceID        string
	NetworkConfigData []NetworkConfigData
	IPAddresses       string
}

// NetworkConfigData is used to render network-config.
type NetworkConfigData struct {
	MacAddress  string
	IPAddress   string
	IPV6Address string
	Gateway     string
	Gateway6    string
	DNSServers  []string
}
