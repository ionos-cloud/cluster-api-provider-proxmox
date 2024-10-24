package goproxmox

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_Ignition(t *testing.T) {
	client := newTestClient(t)

	// test data
	node := "pve"
	vmid := int64(100)

	userdata := `{
  "ignition": {
    "version": "3.2.0"
  },
  "storage": {
    "files": [
      {
        "path": "/etc/hostname",
        "contents": {
          "source": "data:,my-flatcar-hostname",
          "verification": {}
        },
        "mode": 420
      }
    ]
  }
}`

	// mocks
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/status`, node),
		newJSONResponder(200, proxmox.Node{Name: "pve"}))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/status/current`, node, vmid),
		newJSONResponder(200, &proxmox.VirtualMachine{
			Node: node,
			VMID: proxmox.StringOrUint64(vmid),
			VirtualMachineConfig: &proxmox.VirtualMachineConfig{
				Agent:     "1",
				TagsSlice: []string{"flatcar"},
				Tags:      "flatcar",
			},
		}))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/qemu/%d/config`, node, vmid),
		newJSONResponder(200, &proxmox.VirtualMachineConfig{
			Name:      "test",
			TagsSlice: []string{"flatcar"},
			Tags:      "flatcar",
		}))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/storage`, node),
		newJSONResponder(200, &proxmox.Storages{{Name: "iso", Content: "iso"}}))

	ptask := &proxmox.Task{
		UPID:      "UPID:pve:003B4235:1DF4ABCA:667C1C45:vncproxy:103:root@pam:",
		Type:      "upload",
		User:      "foo",
		Status:    "completed",
		Node:      "pve",
		IsRunning: false,
	}

	httpmock.RegisterResponder(http.MethodPost, fmt.Sprintf(`=~/nodes/%s/storage/iso/upload`, node),
		newJSONResponder(200, ptask.UPID))

	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf(`=~/nodes/%s/tasks/%s/status`, node, string(ptask.UPID)),
		newJSONResponderTimes(200, ptask, 2))

	httpmock.RegisterResponder(http.MethodPost, testBaseURL+fmt.Sprintf(`/nodes/%s/qemu/%d/config`, node, vmid),
		newJSONResponder(200, ptask))

	vm, err := client.GetVM(context.Background(), node, vmid)
	require.NoError(t, err)

	// test
	err = client.Ignition(context.Background(), vm, "ide0", userdata)
	require.Error(t, err)
}

func newJSONResponderTimes(status int, data any, times int) httpmock.Responder {
	return httpmock.NewJsonResponderOrPanic(status, map[string]any{"data": data}).Times(times)
}
