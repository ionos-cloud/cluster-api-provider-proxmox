/*
Copyright 2023-2025 IONOS Cloud.

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

// Package ignition implements an enricher for Ignition configs.
package ignition

import (
	"encoding/json"
	"fmt"
	"net/url"

	ignition "github.com/flatcar/ignition/config/v2_3"
	ignitionTypes "github.com/flatcar/ignition/config/v2_3/types"
	"github.com/pkg/errors"
	"k8s.io/utils/ptr"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
)

// Enricher is responsible for enriching the Ignition config with additional data.
type Enricher struct {
	BootstrapData     []byte
	Hostname          string
	InstanceID        string
	ProviderID        string
	Network           []types.NetworkConfigData
	KubernetesVersion string
}

// Enrich enriches the Ignition config with additional data.
func (e *Enricher) Enrich() ([]byte, string, error) {
	ign, err := e.getEnrichConfig()
	if err != nil {
		return nil, "", errors.Wrap(err, "getting enrich config")
	}

	return buildIgnitionConfig(e.BootstrapData, ign)
}

func (e *Enricher) getEnrichConfig() (*ignitionTypes.Config, error) {
	ign := &ignitionTypes.Config{
		Storage: ignitionTypes.Storage{
			Files: []ignitionTypes.File{
				{
					Node: ignitionTypes.Node{
						Filesystem: "root",
						Path:       "/etc/hostname",
						Overwrite:  ptr.To(true),
					},
					FileEmbedded1: ignitionTypes.FileEmbedded1{
						Mode: ptr.To(0644),
						Contents: ignitionTypes.FileContents{
							Source: fmt.Sprintf("data:,%s", e.Hostname),
						},
					},
				},
				{
					Node: ignitionTypes.Node{
						Filesystem: "root",
						Path:       "/etc/proxmox-env",
						Overwrite:  ptr.To(true),
					},
					FileEmbedded1: ignitionTypes.FileEmbedded1{
						Mode: ptr.To(420),
						Contents: ignitionTypes.FileContents{
							Source: fmt.Sprintf("data:,%s", e.getProxmoxEnvContent()),
						},
					},
				},
			},
		},
		Networkd: ignitionTypes.Networkd{
			Units: []ignitionTypes.Networkdunit{},
		},
		Systemd: ignitionTypes.Systemd{
			Units: []ignitionTypes.Unit{
				{
					Name:   "systemd-resolved.service",
					Enable: true,
				},
			},
		},
	}

	// populate networkd units
	nets, err := RenderNetworkConfigData(e.Network)
	if err != nil {
		return nil, errors.Wrap(err, "rendering networkd units")
	}

	for name, contents := range nets {
		ign.Networkd.Units = append(ign.Networkd.Units, ignitionTypes.Networkdunit{
			Name:     name,
			Contents: string(contents),
		})
	}

	return ign, nil
}

func (e *Enricher) getProxmoxEnvContent() string {
	content := fmt.Sprintf("COREOS_CUSTOM_HOSTNAME=%s\nCOREOS_CUSTOM_INSTANCE_ID=%s\nCOREOS_CUSTOM_PROVIDER_ID=%s", e.Hostname, e.InstanceID, e.ProviderID)
	// TODO: consider adding a kube-vip config field to NetworkConfigData
	for _, network := range e.Network {
		for _, ipconfig := range network.IPConfigs {
			if ipconfig.IPAddress.Addr().Is4() && ipconfig.Default {
				content += fmt.Sprintf("\nCOREOS_CUSTOM_PRIVATE_IPV4=%s", ipconfig.IPAddress.String())
			}
			if ipconfig.IPAddress.Addr().Is6() && ipconfig.Default {
				content += fmt.Sprintf("\nCOREOS_CUSTOM_PRIVATE_IPV6=%s", ipconfig.IPAddress.String())
			}
		}
	}
	return url.PathEscape(content)
}

func buildIgnitionConfig(bootstrapData []byte, enrichConfig *ignitionTypes.Config) ([]byte, string, error) {
	// We control bootstrapData config, so treat it as strict.
	ign, reports, err := convertToIgnition(bootstrapData, false)
	if err != nil {
		return nil, "", errors.Wrapf(err, "converting bootstrap-data to Ignition")
	}

	if enrichConfig != nil {
		ign = ignition.Append(ign, *enrichConfig)
	}

	userData, err := json.Marshal(&ign)
	if err != nil {
		return nil, "", errors.Wrapf(err, "marshaling generated Ignition config into JSON")
	}

	return userData, reports, nil
}

func convertToIgnition(data []byte, strict bool) (ignitionTypes.Config, string, error) {
	cfg, reports, err := ignition.Parse(data)
	if err != nil {
		return ignitionTypes.Config{}, "", errors.Wrapf(err, "parsing Ignition config")
	}
	if strict && len(reports.Entries) > 0 || reports.IsFatal() {
		return ignitionTypes.Config{}, "", fmt.Errorf("error parsing Ignition config: %v", reports.String())
	}

	return cfg, reports.String(), nil
}
