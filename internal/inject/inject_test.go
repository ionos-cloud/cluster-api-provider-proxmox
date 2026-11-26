/*
Copyright 2024-2026 IONOS Cloud.

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

package inject

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jarcoal/httpmock"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
)

const (
	baseURLWithTrailingSlash = "http://pve.local.test/"
	bootstrapData            = `{
  "ignition": {
    "config": {},
    "security": {
      "tls": {}
    },
    "timeouts": {},
    "version": "2.3.0"
  },
  "networkd": {},
  "passwd": {
    "users": [
      {
        "name": "core",
        "sshAuthorizedKeys": [
          "ssh-ed25519 ..."
        ]
      }
    ]
  },
  "storage": {
    "files": [
      {
        "filesystem": "root",
        "path": "/etc/sudoers.d/core",
        "contents": {
          "source": "data:,core%20ALL%3D(ALL)%20NOPASSWD%3AALL%0A",
          "verification": {}
        },
        "mode": 384
      }
    ]
  },
  "systemd": {
    "units": [
      {
        "contents": "[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\nAfter=network.target\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n",
        "enabled": true,
        "name": "kubeadm.service"
      }
    ]
  }
}`
)

func TestISOInjectorInjectCloudInit(t *testing.T) {
	client := newTestClient(t)

	vm := &proxmox.VirtualMachine{
		Node: "pve",
		VMID: proxmox.StringOrUint64(100),
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{
			Agent:     "1",
			TagsSlice: []string{"my-vm"},
			Tags:      "my-vm",
		},
	}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/status`, "pve"),
		newJSONResponder(200, proxmox.Node{Name: "pve"}, 2))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/status/current`, "pve", 100),
		newJSONResponder(200, vm, 1))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, "pve", 100),
		newJSONResponder(200, vm.VirtualMachineConfig, 1))

	vm, err := client.GetVM(context.Background(), "pve", 100)
	require.NoError(t, err)

	injector := &ISOInjector{
		VirtualMachine: vm,
		BootstrapData:  []byte(""),
		MetaRenderer:   cloudinit.NewMetadata("xxx-xxxx", "my-custom-vm", "1.2.3", true),
		NetworkRenderer: cloudinit.NewNetworkConfig([]network.ConfigData{
			{
				Type:       "ethernet",
				Name:       "eth0",
				MacAddress: "aa:bb:cc:dd:ee:ff",
				IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.1.1.6/24")}},
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
				Routes:     []network.RoutingData{{To: netip.MustParsePrefix("0.0.0.0/0")}},
			},
		}),
	}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/storage`, "pve"),
		newJSONResponder(200, &proxmox.Storages{{Name: "iso", Content: "iso", Enabled: 1}}, 1))

	ptask := &proxmox.Task{
		UPID:      "UPID:pve:003B4235:1DF4ABCA:667C1C45:vncproxy:103:root@pam:",
		Type:      "upload",
		User:      "foo",
		Status:    "completed",
		Node:      "pve",
		IsRunning: false,
	}

	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/storage/iso/upload`, "pve"),
		newJSONResponder(200, ptask.UPID, 1))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/tasks/%s/status`, "pve", string(ptask.UPID)),
		newJSONResponder(200, ptask, 4))

	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, "pve", 100),
		newJSONResponder(200, ptask.UPID, 2))

	err = injector.Inject(context.Background(), "cloud-config")
	require.NoError(t, err)
}

func TestISOInjectorInjectCloudInit_Errors(t *testing.T) {
	vm := &proxmox.VirtualMachine{
		Node: "pve",
		VMID: proxmox.StringOrUint64(100),
	}
	injector := &ISOInjector{
		VirtualMachine: vm,
		BootstrapData:  []byte(""),
		MetaRenderer:   cloudinit.NewMetadata("xxx-xxxx", "", "", true),
		NetworkRenderer: cloudinit.NewNetworkConfig([]network.ConfigData{
			{
				Type:       "ethernet",
				Name:       "eth0",
				MacAddress: "aa:bb:cc:dd:ee:ff",
				IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.1.1.6/24")}},
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
				Routes: []network.RoutingData{{
					To:  netip.MustParsePrefix("0.0.0.0/0"),
					Via: netip.MustParseAddr("10.1.1.1"),
				}},
			},
		}),
	}

	// missing hostname
	err := injector.Inject(context.Background(), "cloud-config")
	require.Error(t, err)

	// missing network
	injector.MetaRenderer = cloudinit.NewMetadata("xxx-xxxx", "my-custom-vm", "1.2.3", false)
	injector.NetworkRenderer = cloudinit.NewNetworkConfig(nil)
	err = injector.Inject(context.Background(), "cloudinit")
	require.Error(t, err)
}

func TestISOInjectorInject_ISOError(t *testing.T) {
	client := newTestClient(t)

	vm := &proxmox.VirtualMachine{
		Node:                 "pve",
		VMID:                 proxmox.StringOrUint64(100),
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{},
	}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/status`, "pve"),
		newJSONResponder(200, proxmox.Node{Name: "pve"}, 3))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/status/current`, "pve", 100),
		newJSONResponder(200, vm, 1))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, "pve", 100),
		newJSONResponder(200, vm.VirtualMachineConfig, 1))

	vm, err := client.GetVM(context.Background(), "pve", 100)
	require.NoError(t, err)

	injector := &ISOInjector{
		VirtualMachine: vm,
		BootstrapData:  []byte(""),
		MetaRenderer:   cloudinit.NewMetadata("xxx-xxxx", "my-custom-vm", "1.2.3", true),
		NetworkRenderer: cloudinit.NewNetworkConfig([]network.ConfigData{
			{
				Type:       "ethernet",
				Name:       "eth0",
				MacAddress: "aa:bb:cc:dd:ee:ff",
				IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.1.1.6/24")}},
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
				Routes:     []network.RoutingData{{To: netip.MustParsePrefix("0.0.0.0/0")}},
			},
		}),
		IgnitionEnricher: &ignition.Enricher{
			BootstrapData: []byte(bootstrapData),
			Hostname:      "my-custom-vm",
			InstanceID:    "xxxx-xxx",
			ProviderID:    "proxmox://xxxx-xxx",
			Network: []network.ConfigData{
				{
					Name:       "eth0",
					IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.1.1.6/24")}},
					DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					Routes: []network.RoutingData{{
						To:  netip.MustParsePrefix("0.0.0.0/0"),
						Via: netip.MustParseAddr("10.1.1.1"),
					}},
				},
			},
		},
	}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/storage`, "pve"),
		newJSONResponder(200, &proxmox.Storages{{Name: "iso", Content: "iso", Enabled: 1}}, 2))

	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/storage/iso/upload`, "pve"),
		httpmock.NewStringResponder(http.StatusInternalServerError, ""))

	for _, format := range []BootstrapDataFormat{CloudConfigFormat, IgnitionFormat} {
		err = injector.Inject(context.Background(), format)
		require.ErrorAs(t, err, &ISOError{}, format)
		require.ErrorContains(t, err, "unable to inject", format)
		require.Error(t, errors.Unwrap(err), format)
	}
}

func TestISOInjectorInjectIgnition(t *testing.T) {
	client := newTestClient(t)

	vm := &proxmox.VirtualMachine{
		Node: "pve",
		VMID: proxmox.StringOrUint64(100),
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{
			Agent:     "1",
			TagsSlice: []string{"flatcar"},
			Tags:      "flatcar",
		},
	}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/status`, "pve"),
		newJSONResponder(200, proxmox.Node{Name: "pve"}, 2))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/status/current`, "pve", 100),
		newJSONResponder(200, vm, 1))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, "pve", 100),
		newJSONResponder(200, vm.VirtualMachineConfig, 1))

	vm, err := client.GetVM(context.Background(), "pve", 100)
	require.NoError(t, err)

	enricher := &ignition.Enricher{
		BootstrapData: []byte(bootstrapData),
		Hostname:      "my-custom-vm",
		InstanceID:    "xxxx-xxx",
		ProviderID:    "proxmox://xxxx-xxx",
		Network: []network.ConfigData{
			{
				Name:       "eth0",
				IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.1.1.6/24")}},
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
				Routes: []network.RoutingData{{
					To:  netip.MustParsePrefix("0.0.0.0/0"),
					Via: netip.MustParseAddr("10.1.1.1"),
				}},
			},
		},
	}

	injector := &ISOInjector{
		VirtualMachine:   vm,
		BootstrapData:    []byte(bootstrapData),
		MetaRenderer:     cloudinit.NewMetadata("xxx-xxxx", "my-custom-vm", "1.2.3", false),
		IgnitionEnricher: enricher,
	}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/storage`, "pve"),
		newJSONResponder(200, &proxmox.Storages{{Name: "iso", Content: "iso", Enabled: 1}}, 1))

	ptask := &proxmox.Task{
		UPID:      "UPID:pve:003B4235:1DF4ABCA:667C1C45:vncproxy:103:root@pam:",
		Type:      "upload",
		User:      "foo",
		Status:    "completed",
		Node:      "pve",
		IsRunning: false,
	}

	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/storage/iso/upload`, "pve"),
		newJSONResponder(200, ptask.UPID, 1))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/tasks/%s/status`, "pve", string(ptask.UPID)),
		newJSONResponder(200, ptask, 4))

	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, "pve", 100),
		newJSONResponder(200, ptask.UPID, 2))

	err = injector.Inject(context.Background(), "ignition")
	require.NoError(t, err)
}

func TestISOInjectorInjectIgnition_Errors(t *testing.T) {
	vm := &proxmox.VirtualMachine{
		Node: "pve",
		VMID: proxmox.StringOrUint64(100),
	}
	e := &ignition.Enricher{
		BootstrapData: []byte(bootstrapData),
		Hostname:      "my-custom-vm",
		InstanceID:    "xxxx-xxx",
		ProviderID:    "proxmox://xxxx-xxx",
		Network: []network.ConfigData{
			{
				Name:       "eth0",
				IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.1.1.9/24")}},
				DNSServers: []string{"10.1.1.1"},
				Routes: []network.RoutingData{{
					To:  netip.MustParsePrefix("0.0.0.0/0"),
					Via: netip.MustParseAddr("10.1.1.1"),
				}},
			},
		},
	}
	injector := &ISOInjector{
		VirtualMachine:   vm,
		MetaRenderer:     nil,
		IgnitionEnricher: e,
	}

	// missing metadata renderer
	e.BootstrapData = []byte(bootstrapData)
	err := injector.Inject(context.Background(), "ignition")
	require.Error(t, err)

	// missing hostname
	injector.MetaRenderer = cloudinit.NewMetadata("xxxx-xxxxx", "", "1.2.3", false)
	e.BootstrapData = []byte(bootstrapData)
	err = injector.Inject(context.Background(), "ignition")
	require.Error(t, err)

	// no bootstrapdata
	e.BootstrapData = nil
	injector.MetaRenderer = cloudinit.NewMetadata("xxxx-xxxxx", "my-custom-vm", "1.2.3", true)
	injector.BootstrapData = []byte("invalid")
	err = injector.Inject(context.Background(), "ignition")
	require.Error(t, err)

	// no enricher
	injector.IgnitionEnricher = nil
	err = injector.Inject(context.Background(), "ignition")
	require.Error(t, err, "ignition enricher is not defined")

	// enrich failed - invalid ignition
	e.BootstrapData = []byte("invalid")
	injector.IgnitionEnricher = e
	err = injector.Inject(context.Background(), "ignition")
	require.Error(t, err, "unable to enrich ignition")
}

func TestISOInjectorInject_Unsupported(t *testing.T) {
	vm := &proxmox.VirtualMachine{
		Node: "pve",
		VMID: proxmox.StringOrUint64(100),
	}
	injector := &ISOInjector{
		VirtualMachine: vm,
		BootstrapData:  []byte(""),
		MetaRenderer:   cloudinit.NewMetadata("xxx-xxxx", "", "1.2.3", false),
		NetworkRenderer: cloudinit.NewNetworkConfig([]network.ConfigData{
			{
				Type:       "ethernet",
				Name:       "eth0",
				MacAddress: "aa:bb:cc:dd:ee:ff",
				IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.1.1.6/24")}},
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
				Routes: []network.RoutingData{{
					To:  netip.MustParsePrefix("0.0.0.0/0"),
					Via: netip.MustParseAddr("10.1.1.1"),
				}},
			},
		}),
	}

	// unsupported format
	err := injector.Inject(context.Background(), "invalid")
	require.Error(t, err)
}

func newTestClient(t *testing.T) *goproxmox.APIClient {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)

	httpmock.RegisterResponder(http.MethodGet, baseURLWithTrailingSlash+"api2/json/version",
		newJSONResponder(200, proxmox.Version{Release: "test"}, 1))

	client, err := goproxmox.NewAPIClient(context.Background(), logr.Discard(), baseURLWithTrailingSlash)
	require.NoError(t, err)

	return client
}

func newJSONResponder(status int, data any, times int) httpmock.Responder {
	return httpmock.NewJsonResponderOrPanic(status, map[string]any{"data": data}).Times(times)
}

func TestCloudInitISODeviceFor(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *proxmox.VirtualMachineConfig
		want    string
		wantErr bool
	}{
		{name: "nil config", cfg: nil, want: "ide0"},
		{name: "x86 default machine", cfg: &proxmox.VirtualMachineConfig{SCSIs: map[string]string{"scsi0": "local-lvm:vm-100-disk-0,size=32G"}}, want: "ide0"},
		{name: "x86 q35", cfg: &proxmox.VirtualMachineConfig{Machine: "q35", SCSIs: map[string]string{"scsi0": "local-lvm:vm-100-disk-0,size=32G"}}, want: "ide0"},
		{name: "x86 pinned pc", cfg: &proxmox.VirtualMachineConfig{Machine: "pc-i440fx-8.1"}, want: "ide0"},
		{name: "x86 without IDE drives keeps ide0", cfg: &proxmox.VirtualMachineConfig{Machine: "q35", VirtIOs: map[string]string{"virtio0": "local-lvm:vm-100-disk-0,size=32G"}}, want: "ide0"},
		{name: "aarch64 virt with scsi0 boot disk", cfg: &proxmox.VirtualMachineConfig{Machine: "virt", SCSIs: map[string]string{"scsi0": "local-lvm:vm-100-disk-0,size=32G"}}, want: "scsi1"},
		{name: "aarch64 pinned virt", cfg: &proxmox.VirtualMachineConfig{Machine: "virt-8.1", SCSIs: map[string]string{"scsi0": "a", "scsi1": "b"}}, want: "scsi2"},
		{name: "aarch64 virt with options", cfg: &proxmox.VirtualMachineConfig{Machine: "virt,viommu=virtio", SCSIs: map[string]string{"scsi0": "a"}}, want: "scsi1"},
		{name: "aarch64 virt long form", cfg: &proxmox.VirtualMachineConfig{Machine: "type=virt"}, want: "scsi0"},
		{name: "aarch64 virt fills the first gap", cfg: &proxmox.VirtualMachineConfig{Machine: "Virt", SCSIs: map[string]string{"scsi0": "a", "scsi2": "c"}}, want: "scsi1"},
		{name: "aarch64 virt boot disk elsewhere", cfg: &proxmox.VirtualMachineConfig{Machine: "virt", VirtIOs: map[string]string{"virtio0": "a"}}, want: "scsi0"},
		{name: "virtual is not virt", cfg: &proxmox.VirtualMachineConfig{Machine: "virtual"}, want: "ide0"},
		{name: "aarch64 virt with every scsi slot taken errors", cfg: &proxmox.VirtualMachineConfig{Machine: "virt", SCSIs: fullSCSIs()}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CloudInitISODeviceFor(tc.cfg)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrNoSCSISlot)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	dev, reused, err := (&ISOInjector{}).device()
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, "ide0", dev)

	dev, reused, err = (&ISOInjector{VirtualMachine: &proxmox.VirtualMachine{VirtualMachineConfig: &proxmox.VirtualMachineConfig{Machine: "virt", SCSIs: map[string]string{"scsi0": "a"}}}}).device()
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, "scsi1", dev)

	// A retry after an accepted attach reuses the drive already carrying the ISO instead of
	// allocating the next free slot.
	dev, reused, err = (&ISOInjector{VirtualMachine: &proxmox.VirtualMachine{VMID: 100, VirtualMachineConfig: &proxmox.VirtualMachineConfig{Machine: "virt", SCSIs: map[string]string{"scsi0": "a", "scsi1": "local:iso/user-data-100.iso,media=cdrom"}}}}).device()
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, "scsi1", dev)
}

// fullSCSIs returns a SCSIs map with every slot scsi0..scsi30 occupied.
func fullSCSIs() map[string]string {
	m := make(map[string]string, maxSCSIDevice+1)
	for i := 0; i <= maxSCSIDevice; i++ {
		m[fmt.Sprintf("scsi%d", i)] = "occupied"
	}
	return m
}

// TestISOInjectorInjectCloudInit_ARM64Device attaches the ISO of an aarch64 "virt" VM on
// the first free SCSI slot instead of ide0, which the machine type does not have.
func TestISOInjectorInjectCloudInit_ARM64Device(t *testing.T) {
	client := newTestClient(t)

	vm := &proxmox.VirtualMachine{Node: "pve", VMID: proxmox.StringOrUint64(100)}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/status`, "pve"),
		newJSONResponder(200, proxmox.Node{Name: "pve"}, 2))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/status/current`, "pve", 100),
		newJSONResponder(200, vm, 1))

	// Raw config: the indexed drive keys are only populated by UnmarshalJSON.
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, "pve", 100),
		newJSONResponder(200, map[string]any{
			"agent":   "1",
			"machine": "virt",
			"scsihw":  "virtio-scsi-pci",
			"scsi0":   "local-lvm:vm-100-disk-0,size=32G",
			"boot":    "order=scsi0",
			"tags":    "my-vm",
		}, 1))

	vm, err := client.GetVM(context.Background(), "pve", 100)
	require.NoError(t, err)
	require.Equal(t, "virt", vm.VirtualMachineConfig.Machine)

	injector := &ISOInjector{
		VirtualMachine: vm,
		BootstrapData:  []byte(""),
		MetaRenderer:   cloudinit.NewMetadata("xxx-xxxx", "my-custom-vm", "1.2.3", true),
		NetworkRenderer: cloudinit.NewNetworkConfig([]network.ConfigData{
			{
				Type:       "ethernet",
				Name:       "eth0",
				MacAddress: "aa:bb:cc:dd:ee:ff",
				IPConfigs:  []network.IPConfig{{IPAddress: netip.MustParsePrefix("10.1.1.6/24")}},
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
				Routes:     []network.RoutingData{{To: netip.MustParsePrefix("0.0.0.0/0")}},
			},
		}),
	}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/storage`, "pve"),
		newJSONResponder(200, &proxmox.Storages{{Name: "iso", Content: "iso", Enabled: 1}}, 1))

	ptask := &proxmox.Task{
		UPID:       "UPID:pve:003B4235:1DF4ABCA:667C1C45:vncproxy:103:root@pam:",
		Type:       "upload",
		User:       "foo",
		Status:     "stopped",
		ExitStatus: "OK",
		Node:       "pve",
		IsRunning:  false,
	}

	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/storage/iso/upload`, "pve"),
		newJSONResponder(200, ptask.UPID, 1))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/tasks/%s/status`, "pve", string(ptask.UPID)),
		httpmock.NewJsonResponderOrPanic(200, map[string]any{"data": ptask}))

	var configBodies []string
	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, "pve", 100),
		func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			configBodies = append(configBodies, string(body))
			return httpmock.NewJsonResponse(200, map[string]any{"data": ptask.UPID})
		})

	require.NoError(t, injector.Inject(context.Background(), CloudConfigFormat))

	require.NotEmpty(t, configBodies)
	attached := false
	for _, body := range configBodies {
		require.NotContains(t, body, "ide0", "an aarch64 virt VM has no IDE bus")
		if strings.Contains(body, `"scsi1":"iso:iso/user-data-100.iso,media=cdrom"`) {
			attached = true
			require.Contains(t, body, `"boot":"order=scsi0;scsi1"`)
		}
	}
	require.True(t, attached, "the ISO must be attached on scsi1, got %q", configBodies)
}

// TestISOInjectorInjectCloudInit_RetryAfterAcceptedAttach covers a retry after an accepted
// attach whose task wait failed: the ISO is already on scsi1, so re-injection must reuse that
// drive and not upload or attach a second ISO (cleanup only removes the first) or duplicate
// the boot-order entry. The attach is treated as done, so no API call is made.
func TestISOInjectorInjectCloudInit_RetryAfterAcceptedAttach(t *testing.T) {
	_ = newTestClient(t) // activates httpmock for the duration of the test

	var apiCalls int
	httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		apiCalls++
		return nil, fmt.Errorf("unexpected API call on reinjection: %s %s", req.Method, req.URL)
	})

	vm := &proxmox.VirtualMachine{
		Node: "pve", VMID: proxmox.StringOrUint64(100),
		VirtualMachineConfig: &proxmox.VirtualMachineConfig{
			Machine: "virt",
			SCSIs: map[string]string{
				"scsi0": "local-lvm:vm-100-disk-0,size=32G",
				"scsi1": "iso:iso/user-data-100.iso,media=cdrom",
			},
			Boot: "order=scsi0;scsi1",
		},
	}

	injector := &ISOInjector{VirtualMachine: vm, BootstrapData: []byte("")}

	require.NoError(t, injector.Inject(context.Background(), CloudConfigFormat))
	require.Zero(t, apiCalls, "a retry after an accepted attach must not re-upload or re-attach the ISO")
}
