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

package cloudinit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	expectedValidMetadata = `instance-id: 9a82e2ca-4294-11ee-be56-0242ac120002
local-hostname: proxmox-control-plane
hostname: proxmox-control-plane
provider-id: proxmox://9a82e2ca-4294-11ee-be56-0242ac120002
proxmox-node: proxmox-node-0
`
)

func TestMetadata_Render(t *testing.T) {
	type want struct {
		metadata string
		err      error
	}

	cases := map[string]struct {
		reason string
		args   BaseCloudInitData
		want   want
	}{
		"ValidCloudinit": {
			reason: "rendering metadata, network-config",
			args: BaseCloudInitData{
				InstanceID:  "9a82e2ca-4294-11ee-be56-0242ac120002",
				Hostname:    "proxmox-control-plane",
				ProxmoxNode: "proxmox-node-0",
			},
			want: want{
				metadata: expectedValidMetadata,
				err:      nil,
			},
		},
		"InvalidCloudinitMissingProxmoxNode": {
			reason: "instance-id is not set",
			args: BaseCloudInitData{
				InstanceID: "some-id",
				Hostname:   "some-hostname",
			},
			want: want{
				metadata: "",
				err:      ErrMissingProxmoxNode,
			},
		},
		"InvalidCloudinitMissingInstanceID": {
			reason: "instance-id is not set",
			args: BaseCloudInitData{
				Hostname: "some-hostname",
			},
			want: want{
				metadata: "",
				err:      ErrMissingInstanceID,
			},
		},
		"InvalidCloudinitMissingHostname": {
			reason: "hostname is not set",
			args:   BaseCloudInitData{},
			want: want{
				metadata: "",
				err:      ErrMissingHostname,
			},
		},
	}

	for n, tc := range cases {
		t.Run(n, func(t *testing.T) {
			ci := NewMetadata(tc.args)
			metadata, err := ci.Render()
			require.ErrorIs(t, err, tc.want.err)
			require.Equal(t, tc.want.metadata, string(metadata))
		})
	}
}
