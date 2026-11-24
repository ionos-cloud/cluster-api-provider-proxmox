/*
Copyright 2023-2026 IONOS Cloud.

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
	"fmt"
	"strings"

	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CloudInitISODevice is the device the cloud-init ISO is attached to on x86 VMs, whose
// machine types (i440fx, q35) all have an IDE bus.
const CloudInitISODevice = "ide0"

// maxSCSIDevice is the highest SCSI index Proxmox VE accepts (scsi0 to scsi30).
const maxSCSIDevice = 30

// CloudInitISODeviceFor picks the device for the cloud-init ISO from the VM's config. The
// aarch64 "virt" machine type has no IDE bus: a VM given ide0 fails to start with
// "Bus 'ide.0' not found" (upstream #548, #523). Such a VM gets the first free scsiN after
// its SCSI drives, so the boot disk on scsi0 keeps its slot. Every other config, a nil one
// included, keeps ide0, so x86 VMs are attached exactly as before.
//
// The machine type is the signal because go-proxmox v0.10.0 does not expose the "arch"
// key: Proxmox VE runs aarch64 guests only as "virt", and no x86 machine type is named so.
func CloudInitISODeviceFor(cfg *proxmox.VirtualMachineConfig) string {
	if cfg == nil || !isVirtMachine(cfg.Machine) {
		return CloudInitISODevice
	}
	for i := 0; i <= maxSCSIDevice; i++ {
		dev := fmt.Sprintf("scsi%d", i)
		if _, used := cfg.SCSIs[dev]; !used {
			return dev
		}
	}
	return CloudInitISODevice
}

// isVirtMachine reports whether a Proxmox "machine" value names the aarch64 virt machine
// type: "virt", a pinned "virt-8.1", with options ("virt,viommu=virtio") or in the long
// form "type=virt".
func isVirtMachine(machine string) bool {
	m := strings.ToLower(strings.TrimSpace(machine))
	if m == "" {
		return false
	}
	for _, part := range strings.Split(m, ",") {
		if typ, ok := strings.CutPrefix(part, "type="); ok {
			part = typ
		}
		if part == "virt" || strings.HasPrefix(part, "virt-") {
			return true
		}
	}
	return false
}

// device returns the device the ISO goes to for the injector's VM.
func (i *ISOInjector) device() string {
	if i.VirtualMachine == nil {
		return CloudInitISODevice
	}
	return CloudInitISODeviceFor(i.VirtualMachine.VirtualMachineConfig)
}

// ISOError is an error from Proxmox VE while uploading or attaching the ISO. Unlike an error
// rendering its contents, it can go away when retried.
type ISOError struct {
	Err error
}

func (e ISOError) Error() string { return e.Err.Error() }

func (e ISOError) Unwrap() error { return e.Err }

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
	logger := log.FromContext(ctx)

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

	logger.V(4).Info("CloudInit:", "network-config", string(network))

	// Inject an ISO with userdata, metadata and network-config into the VirtualMachine.
	device := i.device()
	logger.V(4).Info("attaching cloud-init ISO", "device", device)
	err = i.VirtualMachine.CloudInit(ctx, device, string(i.BootstrapData), string(metadata), "", string(network))
	if err != nil {
		return ISOError{errors.Wrap(err, "unable to inject CloudInit ISO")}
	}

	return nil
}

func (i *ISOInjector) injectIgnition(ctx context.Context) error {
	logger := log.FromContext(ctx)

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

	logger.V(4).Info("Ingnition", "bootstrapData", bootstrapData)

	// Inject an ISO with ignition userdata, metadata and an empty network-config v1 into the VirtualMachine.
	device := i.device()
	logger.V(4).Info("attaching ignition ISO", "device", device)
	err = i.VirtualMachine.CloudInit(ctx, device, string(bootstrapData), string(metadata), "", string(cloudinit.EmptyNetworkV1))
	if err != nil {
		return ISOError{errors.Wrap(err, "unable to inject ignition userdata iso")}
	}

	return nil
}
