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

// Package inject implements cloud-init ISO inject logic.
package inject

import (
	"context"

	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
)

// CloudInitISODevice default device used to inject cdrom iso.
const CloudInitISODevice = "ide0"

// ISOInjector used to Inject cloudinit userdata, metadata and network-config into a Proxmox VirtualMachine.
type ISOInjector struct {
	VirtualMachine *proxmox.VirtualMachine

	BootstrapData []byte

	MetaRenderer    cloudinit.Renderer
	NetworkRenderer cloudinit.Renderer

	IgnitionEnricher *ignition.Enricher
}

// Inject injects cloudinit userdata, metadata and network-config into a Proxmox VirtualMachine.
func (i *ISOInjector) Inject(ctx context.Context, format BootstrapDataFormat) error {
	switch format {
	case IgnitionFormat:
		return i.injectIgnition(ctx)
	case CloudConfigFormat:
		return i.injectCloudInit(ctx)
	default:
		return errors.New("unsupported format")
	}
}

func (i *ISOInjector) injectCloudInit(ctx context.Context) error {
	// Render metadata.
	metadata, err := i.MetaRenderer.Render()
	if err != nil {
		return errors.Wrap(err, "unable to render metadata")
	}

	// Render network-config.
	network, err := i.NetworkRenderer.Render()
	if err != nil {
		return errors.Wrap(err, "unable to render network-config")
	}

	// Inject an ISO with userdata, metadata and network-config into the VirtualMachine.
	err = i.VirtualMachine.CloudInit(ctx, CloudInitISODevice, string(i.BootstrapData), string(metadata), "", string(network))
	if err != nil {
		return errors.Wrap(err, "unable to inject CloudInit ISO")
	}

	return nil
}

func (i *ISOInjector) injectIgnition(ctx context.Context) error {
	if i.IgnitionEnricher == nil {
		return errors.New("ignition enricher is not defined")
	}

	if i.IgnitionEnricher.BootstrapData == nil {
		i.IgnitionEnricher.BootstrapData = i.BootstrapData
	}

	if i.MetaRenderer == nil {
		return errors.New("metadata renderer is not defined")
	}

	// Render metadata.
	metadata, err := i.MetaRenderer.Render()
	if err != nil {
		return errors.Wrap(err, "unable to render metadata")
	}

	bootstrapData, _, err := i.IgnitionEnricher.Enrich()
	if err != nil {
		return errors.Wrap(err, "unable to enrich ignition")
	}

	// Inject an ISO with ignition userdata, metadata and an empty network-config v1 into the VirtualMachine.
	err = i.VirtualMachine.CloudInit(ctx, CloudInitISODevice, string(bootstrapData), string(metadata), "", string(cloudinit.EmptyNetworkV1))
	if err != nil {
		return errors.Wrap(err, "unable to inject ignition userdata iso")
	}

	return nil
}
