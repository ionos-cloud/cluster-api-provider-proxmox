package ignition

import (
	"testing"

	ignition "github.com/flatcar/ignition/config/v2_3"
	"github.com/stretchr/testify/require"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
)

func TestEnricher_Enrich(t *testing.T) {
	bootstrapData := `{
	  "ignition": {
		"config": {},
		"security": {
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

	e := &Enricher{
		BootstrapData: []byte(bootstrapData),
		Hostname:      "my-custom-vm",
		InstanceID:    "xxxx-xxx",
		ProviderID:    "proxmox://xxxx-xxx",
		Network: []types.NetworkConfigData{
			{
				Name:        "eth0",
				IPAddress:   "10.1.1.9/24",
				IPV6Address: "2001:db8::1/64",
				Gateway6:    "2001:db8::1",
				Gateway:     "10.1.1.1",
				DNSServers:  []string{"10.1.1.1"},
			},
		},
	}

	userdata, reports, err := e.Enrich()
	require.NoError(t, err)
	require.Empty(t, reports)
	require.NotEmpty(t, userdata)

	cfg, _, err := ignition.Parse(userdata)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Len(t, cfg.Networkd.Units, 1)

	// wrong ignition
	e.BootstrapData = []byte(`{}`)
	_, _, err = e.Enrich()
	require.Error(t, err, "parsing ignition Config")
}
