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

// Package inject implements cloud-init ISO inject logic.
package inject

import (
	"context"

	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
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
	Client           capmox.Client
}

// Inject injects cloudinit userdata, metadata and network-config into a Proxmox VirtualMachine.
func (i *ISOInjector) Inject(ctx context.Context, format string) error {
	switch format {
	case "ignition":
		return i.injectIgnition(ctx)
	default:
		return i.injectCloudInit(ctx)
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
	if i.Client == nil {
		return errors.New("proxmox client is not defined")
	}
	if i.IgnitionEnricher == nil {
		return errors.New("ignition enricher is not defined")
	}

	if i.IgnitionEnricher.BootstrapData == nil {
		i.IgnitionEnricher.BootstrapData = i.BootstrapData
	}

	bootstrapData, _, err := i.IgnitionEnricher.Enrich()
	if err != nil {
		return errors.Wrap(err, "unable to enrich ignition")
	}

	// Inject an ISO as config-2 with user_data as an ignition into the VirtualMachine.
	err = i.Client.Ignition(ctx, i.VirtualMachine, CloudInitISODevice, string(bootstrapData))
	if err != nil {
		return errors.Wrap(err, "unable to inject ignition ISO")
	}
	return nil
}
