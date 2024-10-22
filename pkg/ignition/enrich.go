package ignition

import (
	"encoding/json"
	"fmt"
	"net/url"

	ignition "github.com/flatcar/ignition/config/v2_3"
	ignitionTypes "github.com/flatcar/ignition/config/v2_3/types"
	"github.com/pkg/errors"
	"k8s.io/utils/ptr"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
)

type Enricher struct {
	BootstrapData []byte
	Hostname      string
	Zone          string
	ProxmoxNode   string
	InstanceID    string
	ProviderID    string
	Network       []cloudinit.NetworkConfigData
}

func (e *Enricher) Enrich() ([]byte, string, error) {
	enrichConfig, err := e.getEnrichConfig()
	if err != nil {
		return nil, "", errors.Wrapf(err, "getting enrich config")
	}

	return buildIgnitionConfig(e.BootstrapData, enrichConfig)
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

	for i, net := range e.Network {
		ign.Networkd.Units = append(ign.Networkd.Units, ignitionTypes.Networkdunit{
			Name:     fmt.Sprintf("%d%d-eth%d.network", i, i, i),
			Contents: getNetworkdUnit(net),
		})
	}

	return ign, nil
}

func getNetworkdUnit(net cloudinit.NetworkConfigData) string {
	str := fmt.Sprintf("[Match]\nMACAddress=%s\n\n[Link]\nName=%s\n\n[Network]\nAddress=%s\nGateway=%s\n", net.MacAddress, net.Name, net.IPAddress, net.Gateway)
	if net.DNSServers != nil {
		for _, dns := range net.DNSServers {
			str += fmt.Sprintf("DNS=%s\n", dns)
		}
	}
	return str
}

func (e *Enricher) getProxmoxEnvContent() string {
	content := fmt.Sprintf("COREOS_CUSTOM_HOSTNAME=%s\nCOREOS_CUSTOM_ZONE=%s\nCOREOS_CUSTOM_INSTANCE_ID=%s\nCOREOS_CUSTOM_PROVIDER_ID=%s\nCOREOS_CUSTOM_NODE=%s\nCOREOS_CUSTOM_PRIVATE_IPV4=%s", e.Hostname, e.Zone, e.InstanceID, e.ProviderID, e.ProxmoxNode, e.Network[0].IPAddress)

	return url.PathEscape(content)
}

func buildIgnitionConfig(bootstrapData []byte, enrichConfig *ignitionTypes.Config) ([]byte, string, error) {
	// We control bootstrapData config, so treat it as strict.
	ign, _, err := convertToIgnition(bootstrapData, true)
	if err != nil {
		return nil, "", errors.Wrapf(err, "converting bootstrap-data to Ignition")
	}

	var clcWarnings string
	if enrichConfig != nil {
		ign = ignition.Append(ign, *enrichConfig)
	}

	userData, err := json.Marshal(&ign)
	if err != nil {
		return nil, "", errors.Wrapf(err, "marshaling generated Ignition config into JSON")
	}

	fmt.Println("userData: ", string(userData))

	return userData, clcWarnings, nil
}

func convertToIgnition(data []byte, strict bool) (ignitionTypes.Config, string, error) {
	cfg, reports, err := ignition.Parse(data)
	if err != nil {
		return ignitionTypes.Config{}, "", errors.Wrapf(err, "parsing ignition Config")
	}
	if reports.IsFatal() {
		return ignitionTypes.Config{}, "", fmt.Errorf("error parsing ignition Config: %v", reports.String())
	}

	return cfg, reports.String(), nil
}
