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
	"regexp"
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

// ErrNoSCSISlot is returned for an aarch64 "virt" VM, which has no IDE bus, when every SCSI
// slot (scsi0 to scsi30) is already occupied so there is nowhere to attach the cloud-init ISO.
var ErrNoSCSISlot = errors.New("no supported SCSI slot available for the cloud-init ISO")

// CloudInitISODeviceFor picks the device for the cloud-init ISO from the VM's config. The
// aarch64 "virt" machine type has no IDE bus: a VM given ide0 fails to start with
// "Bus 'ide.0' not found" (upstream #548, #523). Such a VM gets the first free scsiN after
// its SCSI drives, so the boot disk on scsi0 keeps its slot. Every other config, a nil one
// included, keeps ide0, so x86 VMs are attached exactly as before.
//
// A virt VM whose SCSI slots are all taken has nowhere to put the ISO: it returns
// ErrNoSCSISlot rather than falling back to ide0, which the machine type cannot use.
//
// The machine type is the signal because go-proxmox v0.10.0 does not expose the "arch"
// key: Proxmox VE runs aarch64 guests only as "virt", and no x86 machine type is named so.
func CloudInitISODeviceFor(cfg *proxmox.VirtualMachineConfig) (string, error) {
	if cfg == nil || !isVirtMachine(cfg.Machine) {
		return CloudInitISODevice, nil
	}
	for i := 0; i <= maxSCSIDevice; i++ {
		dev := fmt.Sprintf("scsi%d", i)
		if _, used := cfg.SCSIs[dev]; !used {
			return dev, nil
		}
	}
	return "", ErrNoSCSISlot
}

// existingCloudInitISODrive returns the drive already carrying the given cloud-init ISO file
// ("user-data-<vmid>.iso"), or "" when it is not attached anywhere. The lowest device wins.
func existingCloudInitISODrive(cfg *proxmox.VirtualMachineConfig, isoName string) string {
	if cfg == nil {
		return ""
	}
	pattern := regexp.MustCompile(`(^|[:/])` + regexp.QuoteMeta(isoName) + `(,|$)`)
	best := ""
	for _, m := range []map[string]string{cfg.IDEs, cfg.SCSIs, cfg.SATAs, cfg.VirtIOs} {
		for dev, value := range m {
			if pattern.MatchString(value) && (best == "" || dev < best) {
				best = dev
			}
		}
	}
	return best
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

// device returns the device the cloud-init ISO goes to for the injector's VM and whether that
// drive already carries the ISO (a retry after an accepted attach, which must not attach a
// second ISO on a fresh slot). An error means there is no device the ISO can be attached to.
func (i *ISOInjector) device() (device string, reused bool, err error) {
	if i.VirtualMachine == nil {
		return CloudInitISODevice, false, nil
	}
	cfg := i.VirtualMachine.VirtualMachineConfig
	iso := fmt.Sprintf(proxmox.UserDataISOFormat, i.VirtualMachine.VMID)
	if dev := existingCloudInitISODrive(cfg, iso); dev != "" {
		return dev, true, nil
	}
	dev, err := CloudInitISODeviceFor(cfg)
	return dev, false, err
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

	device, reused, err := i.device()
	if err != nil {
		return errors.Wrap(err, "unable to select a device for the cloud-init ISO")
	}
	if reused {
		// The ISO is already attached on this device: a previous attach succeeded but its
		// task wait failed, and reconciliation retried. Re-attaching would upload a second
		// ISO to a fresh slot (cleanup only removes the first) and add a duplicate boot-order
		// entry, so treat the attach as already done.
		logger.V(4).Info("cloud-init ISO already attached, skipping re-injection", "device", device)
		return nil
	}

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

	device, reused, err := i.device()
	if err != nil {
		return errors.Wrap(err, "unable to select a device for the ignition ISO")
	}
	if reused {
		// Already attached from an earlier accepted attach whose task wait failed; attaching
		// again would duplicate the ISO drive and the boot-order entry.
		logger.V(4).Info("ignition ISO already attached, skipping re-injection", "device", device)
		return nil
	}

	// Inject an ISO with ignition userdata, metadata and an empty network-config v1 into the VirtualMachine.
	logger.V(4).Info("attaching ignition ISO", "device", device)
	err = i.VirtualMachine.CloudInit(ctx, device, string(bootstrapData), string(metadata), "", string(cloudinit.EmptyNetworkV1))
	if err != nil {
		return ISOError{errors.Wrap(err, "unable to inject ignition userdata iso")}
	}

	return nil
}
