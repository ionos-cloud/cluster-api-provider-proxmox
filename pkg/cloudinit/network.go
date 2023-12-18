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

import (
	"net/netip"
)

const (
	/* network-config template. */
	networkConfigTPl = `network:
  version: 2
  renderer: networkd
  ethernets:
  {{- range $index, $element := .NetworkConfigData }}
    eth{{ $index }}:
      match:
        macaddress: {{ $element.MacAddress }}
      {{- if $element.DHCP4 }}
      dhcp4: true
      {{- else }}
      dhcp4: 'no'
      {{- end }}
      {{- if $element.DHCP6 }}
      dhcp6: true
      {{- else }}
      dhcp6: 'no'
      {{- end }}
      {{- if or (and (not $element.DHCP4) $element.IPAddress) (and (not $element.DHCP6) $element.IPV6Address) }}
      addresses:
      {{- if $element.IPAddress }}
        - {{ $element.IPAddress }}
      {{- end }}
      {{- if $element.IPV6Address }}
        - {{ $element.IPV6Address }}
	  {{- end }}
      routes:
      {{- if $element.Gateway }}
        - to: 0.0.0.0/0
          via: {{ $element.Gateway }}
	  {{- end }}
      {{- if $element.Gateway6 }}
        - to: '::/0'
          via: {{ $element.Gateway6 }}
	  {{- end }}
      {{- end }}
      {{- if $element.DNSServers }}
      nameservers:
        addresses:
        {{- range $element.DNSServers }}
          - {{ . }}
        {{- end -}}
      {{- end -}}
  {{- end -}}`
)

// NetworkConfig provides functionality to render machine network-config.
type NetworkConfig struct {
	data BaseCloudInitData
}

// NewNetworkConfig returns a new NetworkConfig object.
func NewNetworkConfig(configs []NetworkConfigData) *NetworkConfig {
	nc := new(NetworkConfig)
	nc.data = BaseCloudInitData{
		NetworkConfigData: configs,
	}
	return nc
}

// Render returns rendered network-config.
func (r *NetworkConfig) Render() ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}

	// render network-config
	return render("network-config", networkConfigTPl, r.data)
}

func (r *NetworkConfig) validate() error {
	if len(r.data.NetworkConfigData) == 0 {
		return ErrMissingNetworkConfigData
	}
	for _, d := range r.data.NetworkConfigData {
		if !d.DHCP4 && !d.DHCP6 && len(d.IPAddress) == 0 && len(d.IPV6Address) == 0 {
			return ErrMissingIPAddress
		}
		if d.MacAddress == "" {
			return ErrMissingMacAddress
		}

		if !d.DHCP4 && len(d.IPAddress) > 0 {
			err := validIPAddress(d.IPAddress)
			if err != nil {
				return err
			}
			if d.Gateway == "" {
				return ErrMissingGateway
			}
		}

		if !d.DHCP6 && len(d.IPV6Address) > 0 {
			err6 := validIPAddress(d.IPV6Address)
			if err6 != nil {
				return err6
			}
			if d.Gateway6 == "" {
				return ErrMissingGateway
			}
		}
	}
	return nil
}

func validIPAddress(input string) error {
	if input == "" {
		return ErrMissingIPAddress
	}
	_, err := netip.ParsePrefix(input)
	if err != nil {
		return ErrMalformedIPAddress
	}
	return nil
}
