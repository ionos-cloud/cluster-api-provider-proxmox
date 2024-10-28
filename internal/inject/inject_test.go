package inject

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jarcoal/httpmock"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
)

const (
	testBaseURL   = "http://pve.local.test/" // regression test against trailing /
	bootstrapData = `{
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

	injector := &ISOInjector{
		VirtualMachine: vm,
		BootstrapData:  []byte(""),
		MetaRenderer:   cloudinit.NewMetadata("xxx-xxxx", "my-custom-vm"),
		NetworkRenderer: cloudinit.NewNetworkConfig([]cloudinit.NetworkConfigData{
			{
				Name:       "eth0",
				IPAddress:  "10.1.1.6/24",
				Gateway:    "10.1.1.1",
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
			},
		}),
	}

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/storage`, "pve"),
		newJSONResponder(200, &proxmox.Storages{{Name: "iso", Content: "iso"}}, 1))

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

	err = injector.Inject(context.Background(), "cloudinit")
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
		MetaRenderer:   cloudinit.NewMetadata("xxx-xxxx", ""),
		NetworkRenderer: cloudinit.NewNetworkConfig([]cloudinit.NetworkConfigData{
			{
				Name:       "eth0",
				IPAddress:  "10.1.1.6/24",
				Gateway:    "10.1.1.1",
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
			},
		}),
	}

	// missing hostname
	err := injector.Inject(context.Background(), "cloudinit")
	require.Error(t, err)

	// missing network
	injector.MetaRenderer = cloudinit.NewMetadata("xxx-xxxx", "my-custom-vm")
	injector.NetworkRenderer = cloudinit.NewNetworkConfig(nil)
	err = injector.Inject(context.Background(), "cloudinit")
	require.Error(t, err)
}

func TestISOInjectorInjectIgnition_Errors(t *testing.T) {
	client := newTestClient(t)

	vm := &proxmox.VirtualMachine{
		Node: "pve",
		VMID: proxmox.StringOrUint64(100),
	}
	e := &ignition.Enricher{
		BootstrapData: []byte(bootstrapData),
		Hostname:      "my-custom-vm",
		InstanceID:    "xxxx-xxx",
		ProviderID:    "proxmox://xxxx-xxx",
		Network: []cloudinit.NetworkConfigData{
			{
				Name:       "eth0",
				IPAddress:  "10.1.1.9/24",
				Gateway:    "10.1.1.1",
				DNSServers: []string{"10.1.1.1"},
			},
		},
	}
	injector := &ISOInjector{
		VirtualMachine: vm,

		IgnitionEnricher: e,
	}

	// no client
	err := injector.Inject(context.Background(), "ignition")
	require.Error(t, err)

	// no bootstrapdata
	e.BootstrapData = nil
	injector.BootstrapData = []byte(bootstrapData)
	injector.Client = client
	err = injector.Inject(context.Background(), "ignition")
	require.Error(t, err)

	// no enricher
	injector.IgnitionEnricher = nil
	err = injector.Inject(context.Background(), "ignition")
	require.Error(t, err, "ignition enricher is not defined")

	// enrich failed - invalid ignition
	injector.IgnitionEnricher = e
	e.BootstrapData = []byte("invalid")
	err = injector.Inject(context.Background(), "ignition")
	require.Error(t, err, "unable to enrich ignition")

	// ignition inject failed - no suitable client
	e.BootstrapData = []byte(bootstrapData)
	err = injector.Inject(context.Background(), "ignition")
	require.Error(t, err, "unable to inject ignition ISO")
}

func newTestClient(t *testing.T) *goproxmox.APIClient {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)

	httpmock.RegisterResponder(http.MethodGet, testBaseURL+"api2/json/version",
		newJSONResponder(200, proxmox.Version{Release: "test"}, 1))

	client, err := goproxmox.NewAPIClient(context.Background(), logr.Discard(), testBaseURL)
	require.NoError(t, err)

	return client
}

func newJSONResponder(status int, data any, times int) httpmock.Responder {
	return httpmock.NewJsonResponderOrPanic(status, map[string]any{"data": data}).Times(times)
}
