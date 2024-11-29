package inject

import (
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
)

// BootstrapDataFormat represents the format of the bootstrap data.
type BootstrapDataFormat string

const (
	// CloudConfigFormat represents the cloud-config format.
	CloudConfigFormat BootstrapDataFormat = cloudinit.FormatCloudConfig
	// IgnitionFormat represents the Ignition format.
	IgnitionFormat BootstrapDataFormat = ignition.FormatIgnition
)
