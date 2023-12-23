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

func TestProxmoxAPIClient_GetReservableResources(t *testing.T) {
	nodeMemory := uint64(30)
	nodeCPUs := 16
	tests := []struct {
		name                 string
		guestMaxMemory       uint64 // memory size of already provisioned guest
		guestCPUs            uint64
		expectMemory         uint64 // expected available memory of the host
		expectCPUs           uint64
		nodeMemoryAdjustment uint64 // factor like 1.0 to multiply host memory with for overprovisioning
		nodeCPUAdjustment    uint64
	}{
		{
			name:                 "under zero - no overprovisioning",
			guestMaxMemory:       29,
			guestCPUs:            1,
			expectMemory:         nodeMemory - 29,
			expectCPUs:           uint64(nodeCPUs - 1),
			nodeMemoryAdjustment: 100,
			nodeCPUAdjustment:    100,
		},
		{
			name:                 "exact zero - no overprovisioning",
			guestMaxMemory:       30,
			guestCPUs:            1,
			expectMemory:         0,
			expectCPUs:           uint64(nodeCPUs - 1),
			nodeMemoryAdjustment: 100,
			nodeCPUAdjustment:    100,
		},
		{
			name:                 "over zero - no overprovisioning",
			guestMaxMemory:       31,
			guestCPUs:            1,
			expectMemory:         0,
			expectCPUs:           uint64(nodeCPUs - 1),
			nodeMemoryAdjustment: 100,
			nodeCPUAdjustment:    100,
		},
		{
			name:                 "under zero - overprovisioning",
			guestMaxMemory:       58,
			guestCPUs:            1,
			expectMemory:         2,
			expectCPUs:           uint64(nodeCPUs*2 - 1),
			nodeMemoryAdjustment: 200,
			nodeCPUAdjustment:    200,
		},
		{
			name:                 "exact zero - overprovisioning",
			guestMaxMemory:       30,
			guestCPUs:            1,
			expectMemory:         nodeMemory*2 - 30,
			expectCPUs:           uint64(nodeCPUs*2 - 1),
			nodeMemoryAdjustment: 200,
			nodeCPUAdjustment:    200,
		},
		{
			name:                 "over zero - overprovisioning",
			guestMaxMemory:       31,
			guestCPUs:            1,
			expectMemory:         nodeMemory*2 - 31,
			expectCPUs:           uint64(nodeCPUs*2 - 1),
			nodeMemoryAdjustment: 200,
			nodeCPUAdjustment:    200,
		},
		{
			name:                 "scheduler disabled",
			guestMaxMemory:       100,
			guestCPUs:            1,
			expectMemory:         nodeMemory,
			expectCPUs:           uint64(nodeCPUs),
			nodeMemoryAdjustment: 0,
			nodeCPUAdjustment:    0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/status`,
				newJSONResponder(200,
					proxmox.Node{
						Memory: proxmox.Memory{
							Total: nodeMemory,
						},
						CPUInfo: proxmox.CPUInfo{
							CPUs: nodeCPUs,
						},
					}))

			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu`,
				// Somehow, setting proxmox.VirtualMachines{} ALWAYS has `Template: true` when defined this way.
				// So it's better to just define a legitimate json response
				newJSONResponder(200, []interface{}{
					map[string]interface{}{
						"name":      "legit-worker",
						"maxmem":    test.guestMaxMemory,
						"vmid":      1111,
						"diskwrite": 0,
						"mem":       0,
						"uptime":    0,
						"disk":      0,
						"cpu":       0,
						"cpus":      test.guestCPUs,
						"status":    "stopped",
						"netout":    0,
						"maxdisk":   0,
						"netin":     0,
						"diskread":  0,
					},
					map[string]interface{}{
						"name":           "template",
						"guestMaxMemory": 102400,
						"vmid":           2222,
						"diskwrite":      0,
						"mem":            0,
						"uptime":         0,
						"disk":           0,
						"cpu":            0,
						"template":       1,
						"cpus":           42,
						"status":         "stopped",
						"netout":         0,
						"maxdisk":        0,
						"netin":          0,
						"diskread":       0,
					}}))

			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/lxc`,
				newJSONResponder(200, proxmox.Containers{}))

			reservableMem, reservableCPUs, err := client.GetReservableResources(context.Background(), "test", test.nodeMemoryAdjustment, test.nodeCPUAdjustment)
			require.NoError(t, err)
			require.Equal(t, test.expectMemory, reservableMem)
			require.Equal(t, test.expectCPUs, reservableCPUs)
		})
	}
}
