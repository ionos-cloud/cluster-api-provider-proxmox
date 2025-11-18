/*
Copyright 2023-2025 IONOS Cloud.

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

	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
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

	t.Run("Fail to access endpoint", func(t *testing.T) {
		client := newTestClient(t)
		httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/status`,
			newJSONResponder(401, "Forbidden"))
		reservable, err := client.GetReservableMemoryBytes(context.Background(), "test", 0)
		require.Error(t, err)
		require.Equal(t, uint64(0), reservable)
		require.Equal(t,
			"cannot find node with name test: not authorized to access endpoint",
			err.Error())
	})

	t.Run("Fail to list VMs", func(t *testing.T) {
		client := newTestClient(t)
		httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/status`,
			newJSONResponder(200, proxmox.Node{Memory: proxmox.Memory{Total: 30}}))
		httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu`,
			newJSONResponder(401, nil))
		reservable, err := client.GetReservableMemoryBytes(context.Background(), "test", 1)
		require.Error(t, err)
		require.Equal(t, uint64(0), reservable)
		require.Equal(t,
			"cannot list vms for node test: not authorized to access endpoint",
			err.Error())
	})
}

func TestProxmoxAPIClient_CloneVM(t *testing.T) {
	tests := []struct {
		name  string
		http  []int
		fails bool
		err   string
	}{
		{name: "no node", http: []int{500, 200, 200, 200, 200, 200}, fails: true,
			err: "cannot find node with name test: 500"},
		{name: "no template", http: []int{200, 200, 403, 200, 200, 200}, fails: true,
			err: "unable to find vm template: not authorized to access endpoint"},
		{name: "clone fails", http: []int{200, 200, 200, 200, 500, 200}, fails: true,
			err: "unable to create new vm: 500"},
		{name: "no node", http: []int{200, 200, 200, 200, 200, 200}, fails: false,
			err: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)

			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/status`,
				newJSONResponder(test.http[0], proxmox.Node{}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/100/status/current`,
				newJSONResponder(test.http[1], proxmox.VirtualMachine{Node: "test"}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/100/config`,
				newJSONResponder(test.http[2], proxmox.VirtualMachineConfig{CPU: "kvm64"}))
			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/status`,
				newJSONResponder(test.http[3],
					proxmox.NodeStatuses{{Name: "test"}, {Name: "test2"}}))
			httpmock.RegisterResponder(http.MethodPost, `=~/nodes/test/qemu/0/clone`,
				newJSONResponder(test.http[4], nil))
			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/nextid`,
				newJSONResponder(test.http[5], "101"))

			clone := capmox.VMCloneRequest{Node: "test"}
			cloneresponse, err := client.CloneVM(context.Background(), 100, clone)

			if test.fails {
				require.Error(t, err)
				require.Equal(t, test.err, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, capmox.VMCloneResponse{NewID: 101, Task: nil},
					cloneresponse)
			}
		})
	}
}

func TestProxmoxAPIClient_ConfigureVM(t *testing.T) {
	tests := []struct {
		name  string
		http  []int
		fails bool
		err   string
	}{
		{name: "create conf task", fails: false, err: ""},
		{name: "conf error", fails: true,
			err: "unable to configure vm: not authorized to access endpoint"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)

			// "UPID:$node:$pid:$pstart:$startime:$dtype:$id:$user"
			upid := "UPID:test:00303F51:09D93CFE:61CCA568:download:test.iso:root@pam:"

			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/status`,
				newJSONResponder(200, proxmox.Node{}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/101/status/current`,
				newJSONResponder(200, proxmox.VirtualMachine{Node: "test", VMID: 101}))
			httpmock.RegisterResponder(http.MethodPost, `=~/nodes/test/qemu/101/config`,
				newJSONResponder(200, upid))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/101/config`,
				newJSONResponder(200, proxmox.VirtualMachineConfig{CPU: "kvm64"}))
			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/status`,
				newJSONResponder(200,
					proxmox.NodeStatuses{{Name: "test"}, {Name: "test2"}}))

			node, err := client.Client.Node(context.Background(), "test")
			require.NoError(t, err)
			vm, err := node.VirtualMachine(context.Background(), 101)
			require.NoError(t, err)

			if test.fails {
				httpmock.RegisterResponder(http.MethodPost, `=~/nodes/test/qemu/101/config`,
					newJSONResponder(403, upid))
			}
			//  These two are merely to use the variadic interface
			oName := capmox.VirtualMachineOption{Name: "name", Value: "RenameTest"}
			oMem := capmox.VirtualMachineOption{Name: "memory", Value: 4096}
			task, err := client.ConfigureVM(context.Background(), vm, oName, oMem)

			if test.fails {
				require.Error(t, err)
				require.Equal(t, test.err, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, "download", task.Type)
				require.Equal(t, upid, string(task.UPID))
				require.Equal(t, "root@pam", task.User)
			}
		})
	}
}

func TestProxmoxAPIClient_GetVM(t *testing.T) {
	tests := []struct {
		name  string
		node  string
		vmID  int64
		fails bool
		err   string
	}{
		{name: "get", node: "test", vmID: 101, fails: false, err: ""},
		{name: "node not found", node: "enoent", vmID: 101, fails: true,
			err: "cannot find node with name enoent: 500"},
		{name: "vm not found", node: "test", vmID: 102, fails: true,
			err: "cannot find vm with id 102: 500"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)

			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/status`,
				newJSONResponder(200, proxmox.Node{}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/enoent/status`,
				newJSONResponder(500, nil))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/101/status/current`,
				newJSONResponder(200, proxmox.VirtualMachine{Node: "test"}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/102/status/current`,
				newJSONResponder(500, nil))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/101/config`,
				newJSONResponder(200, proxmox.VirtualMachineConfig{CPU: "kvm64"}))
			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/status`,
				newJSONResponder(200,
					proxmox.NodeStatuses{{Name: "test"}, {Name: "test2"}}))

			vm, err := client.GetVM(context.Background(), test.node, test.vmID)

			if test.fails {
				require.Error(t, err)
				require.Equal(t, test.err, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, "kvm64", vm.VirtualMachineConfig.CPU)
				require.Equal(t, "test", vm.Node)
			}
		})
	}
}

func TestProxmoxAPIClient_FindVMResource(t *testing.T) {
	tests := []struct {
		name  string
		http  []int
		vmID  uint64
		fails bool
		err   string
	}{
		{name: "find", http: []int{200, 200}, vmID: 101, fails: false, err: ""},
		{name: "clusterstatus broken", http: []int{500, 200}, vmID: 101, fails: true,
			err: "cannot get cluster status: 500"},
		{name: "resourcelisting broken", http: []int{200, 500}, vmID: 102, fails: true,
			err: "could not list vm resources: 500"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)

			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/status`,
				newJSONResponder(test.http[0],
					proxmox.NodeStatuses{{Name: "test"}, {Name: "test2"}}))
			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/resources`,
				newJSONResponder(test.http[1], proxmox.ClusterResources{
					&proxmox.ClusterResource{VMID: 101},
				}))

			clusterResource, err := client.FindVMResource(context.Background(), test.vmID)

			if test.fails {
				require.Error(t, err)
				require.Equal(t, test.err, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, proxmox.ClusterResource{VMID: 101}, *clusterResource)
			}
		})
	}
}

func TestProxmoxAPIClient_FindVMTemplateByTags(t *testing.T) {
	proxmoxClusterResources := proxmox.ClusterResources{
		&proxmox.ClusterResource{VMID: 101, Name: "k8s-node01", Node: "capmox01", Tags: ""},
		&proxmox.ClusterResource{VMID: 102, Name: "k8s-node02", Node: "capmox02", Tags: ""},
		&proxmox.ClusterResource{VMID: 150, Name: "template-without-tags", Node: "capmox01", Tags: "", Template: uint64(1)},
		&proxmox.ClusterResource{VMID: 201, Name: "ubuntu-22.04-k8s-v1.28.3", Node: "capmox01", Tags: "template;capmox;v1.28.3", Template: uint64(1)},
		&proxmox.ClusterResource{VMID: 202, Name: "ubuntu-22.04-k8s-v1.30.2", Node: "capmox02", Tags: "capmox;template;v1.30.2", Template: uint64(1)},
		&proxmox.ClusterResource{VMID: 301, Name: "ubuntu-22.04-k8s-v1.29.2", Node: "capmox02", Tags: "capmox;template;v1.29.2", Template: uint64(1)},
		&proxmox.ClusterResource{VMID: 302, Name: "ubuntu-22.04-k8s-v1.29.2", Node: "capmox02", Tags: "capmox;template;v1.29.2", Template: uint64(1)},
	}
	tests := []struct {
		name             string
		http             []int
		vmTags           []string
		resolutionPolicy string
		fails            bool
		err              string
		vmTemplateNode   string
		vmTemplateID     int32
	}{
		{
			name:             "clusterstatus broken",
			http:             []int{500, 200},
			resolutionPolicy: "exact",
			fails:            true,
			err:              "cannot get cluster status: 500",
		},
		{
			name:             "resourcelisting broken",
			http:             []int{200, 500},
			resolutionPolicy: "exact",
			fails:            true,
			err:              "could not list vm resources: 500",
		},
		{
			name:             "find-template",
			http:             []int{200, 200},
			vmTags:           []string{"template", "capmox", "v1.28.3"},
			resolutionPolicy: "exact",
			fails:            false,
			err:              "",
			vmTemplateNode:   "capmox01",
			vmTemplateID:     201,
		},
		{
			name:             "find-template-nil",
			http:             []int{200, 200},
			vmTags:           nil,
			resolutionPolicy: "subset",
			fails:            true,
			err:              "VM template not found: found 4 VM templates with tags \"\"",
			vmTemplateNode:   "capmox01",
			vmTemplateID:     201,
		},
		{
			// Proxmox VM tags are always lowercase
			name:             "find-template-uppercase",
			http:             []int{200, 200},
			vmTags:           []string{"TEMPLATE", "CAPMOX", "v1.28.3"},
			resolutionPolicy: "exact",
			fails:            false,
			err:              "",
			vmTemplateNode:   "capmox01",
			vmTemplateID:     201,
		},
		{
			name:             "find-template-unordered",
			http:             []int{200, 200},
			vmTags:           []string{"template", "capmox", "v1.30.2"},
			resolutionPolicy: "exact",
			fails:            false,
			err:              "",
			vmTemplateNode:   "capmox02",
			vmTemplateID:     202,
		},
		{
			name:             "find-template-duplicate-tag",
			http:             []int{200, 200},
			vmTags:           []string{"template", "capmox", "capmox", "v1.30.2"},
			resolutionPolicy: "exact",
			fails:            false,
			err:              "",
			vmTemplateNode:   "capmox02",
			vmTemplateID:     202,
		},
		{
			name:             "find-multiple-templates-any-version",
			http:             []int{200, 200},
			vmTags:           []string{"template", "capmox"},
			resolutionPolicy: "subset",
			fails:            true,
			err:              "VM template not found: found 4 VM templates with tags \"template;capmox\"",
			vmTemplateID:     69,
			vmTemplateNode:   "nice",
		},
		{
			name:             "find-multiple-templates-v1.29.2",
			http:             []int{200, 200},
			vmTags:           []string{"template", "capmox", "v1.29.2"},
			resolutionPolicy: "exact",
			fails:            true,
			err:              "VM template not found: found 2 VM templates with tags \"template;capmox;v1.29.2\"",
			vmTemplateID:     69,
			vmTemplateNode:   "nice",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)

			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/status`,
				newJSONResponder(test.http[0], proxmox.NodeStatuses{}))
			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/resources`,
				newJSONResponder(test.http[1], proxmoxClusterResources))

			vmTemplateNode, vmTemplateID, err := client.FindVMTemplateByTags(context.Background(), test.vmTags, test.resolutionPolicy)

			if test.fails {
				require.Error(t, err)
				require.Equal(t, test.err, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, vmTemplateID, test.vmTemplateID)
				require.Equal(t, vmTemplateNode, test.vmTemplateNode)
			}
		})
	}
}

func TestProxmoxAPIClient_DeleteVM(t *testing.T) {
	tests := []struct {
		name  string
		node  string
		vmID  int64
		fails bool
		err   string
	}{
		{name: "delete", node: "test", vmID: 101, fails: false, err: ""},
		{name: "node not found", node: "enoent", vmID: 101, fails: true,
			err: "cannot find node with name enoent: 500"},
		{name: "delete fails", node: "test", vmID: 102, fails: true,
			err: "cannot delete vm with id 102: not authorized to access endpoint"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)

			// "UPID:$node:$pid:$pstart:$startime:$dtype:$id:$user"
			upid := "UPID:test:000D6BDA:041E0A54:654A5A1D:qmdestroy:101:root@pam:"

			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/nextid`,
				newJSONResponder(400, fmt.Sprintf("VM %d already exists", test.vmID)))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/status`,
				newJSONResponder(200, proxmox.Node{}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/enoent/status`,
				newJSONResponder(500, nil))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/101/status/current`,
				newJSONResponder(200, proxmox.VirtualMachine{Node: "test", VMID: 101}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/102/status/current`,
				newJSONResponder(200, proxmox.VirtualMachine{Node: "test", VMID: 102}))
			httpmock.RegisterResponder(http.MethodDelete, `=~/nodes/test/qemu/101`,
				newJSONResponder(200, upid))
			httpmock.RegisterResponder(http.MethodDelete, `=~/nodes/test/qemu/102`,
				newJSONResponder(403, nil))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/101/config`,
				newJSONResponder(200, proxmox.VirtualMachineConfig{CPU: "kvm64"}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/qemu/102/config`,
				newJSONResponder(200, proxmox.VirtualMachineConfig{CPU: "kvm64"}))
			httpmock.RegisterResponder(http.MethodGet, `=~/cluster/status`,
				newJSONResponder(200,
					proxmox.NodeStatuses{{Name: "test"}, {Name: "test2"}}))

			task, err := client.DeleteVM(context.Background(), test.node, test.vmID)

			if test.fails {
				require.Error(t, err)
				require.Equal(t, test.err, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, "qmdestroy", task.Type)
				require.Equal(t, "root@pam", task.User)
			}
		})
	}
}

func TestProxmoxAPIClient_GetTask(t *testing.T) {
	// "UPID:$node:$pid:$pstart:$startime:$dtype:$id:$user"
	upid := "UPID:test:000D6BDA:041E0A54:654A5A1D:qmdestroy:101:root@pam:"
	upid2 := "UPID:test:000D6BDA:041E0A54:654A5A1D:qmdestroy:102:root@pam:"
	tests := []struct {
		name  string
		fails bool
		err   string
	}{
		{name: "get", fails: false, err: ""},
		{name: "get fails", fails: true, err: fmt.Sprintf("cannot get task with UPID %s: 501", upid2)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)

			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/tasks/`+upid,
				newJSONResponder(200,
					proxmox.Task{UPID: proxmox.UPID(upid), ID: "101"}))
			httpmock.RegisterResponder(http.MethodGet, `=~/nodes/test/tasks/`,
				newJSONResponder(501, nil))

			if test.fails {
				_, err := client.GetTask(context.Background(), upid2)
				require.Error(t, err)
				require.Equal(t, test.err, err.Error())
			} else {
				task, err := client.GetTask(context.Background(), upid)
				require.NoError(t, err)
				require.Equal(t, upid, string(task.UPID))
				require.Equal(t, "101", task.ID)
			}
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
