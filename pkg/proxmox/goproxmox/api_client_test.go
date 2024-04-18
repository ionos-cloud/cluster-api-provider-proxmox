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

package goproxmox

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jarcoal/httpmock"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
)

const testBaseURL = "http://pve.local.test/" // regression test against trailing /

func newTestClient(t *testing.T) *APIClient {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)

	httpmock.RegisterResponder(http.MethodGet, testBaseURL+"api2/json/version",
		newJSONResponder(200, proxmox.Version{Release: "test"}))

	client, err := NewAPIClient(context.Background(), logr.Discard(), testBaseURL)
	require.NoError(t, err)

	return client
}

func newJSONResponder(status int, data any) httpmock.Responder {
	return httpmock.NewJsonResponderOrPanic(status, map[string]any{"data": data}).Once()
}

func TestProxmoxAPIClient_GetReservableMemoryBytes(t *testing.T) {
	tests := []struct {
		name                 string
		maxMem               uint64 // memory size of already provisioned guest
		expect               uint64 // expected available memory of the host
		nodeMemoryAdjustment uint64 // factor like 1.0 to multiply host memory with for overprovisioning
	}{
		{
			name:                 "under zero - no overprovisioning",
			maxMem:               29,
			expect:               1,
			nodeMemoryAdjustment: 100,
		},
		{
			name:                 "exact zero - no overprovisioning",
			maxMem:               30,
			expect:               0,
			nodeMemoryAdjustment: 100,
		},
		{
			name:                 "over zero - no overprovisioning",
			maxMem:               31,
			expect:               0,
			nodeMemoryAdjustment: 100,
		},
		{
			name:                 "under zero - overprovisioning",
			maxMem:               58,
			expect:               2,
			nodeMemoryAdjustment: 200,
		},
		{
			name:                 "exact zero - overprovisioning",
			maxMem:               30 * 2,
			expect:               0,
			nodeMemoryAdjustment: 200,
		},
		{
			name:                 "over zero - overprovisioning",
			maxMem:               31 * 2,
			expect:               0,
			nodeMemoryAdjustment: 200,
		},
		{
			name:                 "scheduler disabled",
			maxMem:               100,
			expect:               30,
			nodeMemoryAdjustment: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/status`,
				newJSONResponder(200, proxmox.Node{Memory: proxmox.Memory{Total: 30}}))

			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu`,
				// Somehow, setting proxmox.VirtualMachines{} ALWAYS has `Template: true` when defined this way.
				// So it's better to just define a legitimate json response
				newJSONResponder(200, []interface{}{
					map[string]interface{}{
						"name":      "legit-worker",
						"maxmem":    test.maxMem,
						"vmid":      1111,
						"diskwrite": 0,
						"mem":       0,
						"uptime":    0,
						"disk":      0,
						"cpu":       0,
						"cpus":      1,
						"status":    "stopped",
						"netout":    0,
						"maxdisk":   0,
						"netin":     0,
						"diskread":  0,
					},
					map[string]interface{}{
						"name":      "template",
						"maxmem":    102400,
						"vmid":      2222,
						"diskwrite": 0,
						"mem":       0,
						"uptime":    0,
						"disk":      0,
						"cpu":       0,
						"template":  1,
						"cpus":      1,
						"status":    "stopped",
						"netout":    0,
						"maxdisk":   0,
						"netin":     0,
						"diskread":  0,
					}}))

			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/lxc`,
				newJSONResponder(200, proxmox.Containers{}))

			reservable, err := client.GetReservableMemoryBytes(context.Background(), "test", test.nodeMemoryAdjustment)
			require.NoError(t, err)
			require.Equal(t, test.expect, reservable)
		})
	}
}

func TestProxmoxAPIClient_CloudInitStatus(t *testing.T) {
	tests := []struct {
		name     string
		node     string  // node name
		vmid     int64   // vmid
		pid      float64 // pid of agent
		exited   int     // exited state
		exitcode int     // exitcode
		outData  string  // out-data
		running  bool    // expected running state
		err      error   // expected error
	}{
		{
			name:     "cloud-init success",
			node:     "pve",
			vmid:     1111,
			pid:      12234,
			exited:   1,
			exitcode: 0,
			outData:  "status: done\n",
			running:  false,
			err:      nil,
		},
		{
			name:     "cloud-init running",
			node:     "pve",
			vmid:     1111,
			pid:      12234,
			exited:   1,
			exitcode: 0,
			outData:  "status: running\n",
			running:  true,
			err:      nil,
		},
		{
			name:     "cloud-init failed",
			node:     "pve",
			vmid:     1111,
			pid:      12234,
			exited:   1,
			exitcode: 1,
			outData:  "status: error\n",
			running:  false,
			err:      ErrCloudInitFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)

			httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/status`, test.node),
				newJSONResponder(200, proxmox.Node{Name: "pve"}))

			httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/status/current`, test.node, test.vmid),
				newJSONResponder(200, proxmox.VirtualMachine{
					VMID: proxmox.StringOrUint64(test.vmid),
					Name: "legit-worker",
					Node: test.node,
				}))

			httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, test.node, test.vmid),
				newJSONResponder(200, proxmox.VirtualMachineConfig{
					Name: "legit-worker",
				}))

			vm, err := client.GetVM(context.Background(), test.node, test.vmid)
			require.NoError(t, err)
			require.NotNil(t, vm)

			// WaitForAgent mock
			httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/agent/get-osinfo`, vm.Node, vm.VMID),
				newJSONResponder(200,
					map[string]*proxmox.AgentOsInfo{
						"result": {
							ID:            "ubuntu",
							VersionID:     "22.04",
							Machine:       "x86_64",
							KernelRelease: "5.15.0-89-generic",
							KernelVersion: "#99-Ubuntu SMP Mon Oct 30 20:42:41 UTC 2023",
							Name:          "Ubuntu",
							Version:       "22.04.3 LTS (Jammy Jellyfish)",
							PrettyName:    "Ubuntu 22.04.3 LTS",
						},
					},
				))

			// AgentExec mock
			httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/qemu/%d/agent/exec\z`, vm.Node, vm.VMID),
				newJSONResponder(200,
					map[string]interface{}{
						"pid": test.pid,
					},
				))

			// AgentExecStatus mock
			httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/agent/exec-status\?pid=%v`, vm.Node, vm.VMID, test.pid),
				newJSONResponder(200,
					&proxmox.AgentExecStatus{
						Exited:   test.exited,
						ExitCode: test.exitcode,
						OutData:  test.outData,
					},
				))

			running, err := client.CloudInitStatus(context.Background(), vm)
			require.Equal(t, err, test.err)
			require.Equal(t, test.running, running)
		})
	}
}
