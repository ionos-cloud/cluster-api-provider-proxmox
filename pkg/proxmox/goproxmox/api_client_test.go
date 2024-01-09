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
