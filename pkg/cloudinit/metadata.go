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

package cloudinit

import (
	"encoding/json"
)

const (
	metadataTPl = `instance-id: {{ .InstanceID }}
local-hostname: {{ .Hostname }}
hostname: {{ .Hostname }}
{{- if .ProviderIDInjection }}
provider-id: proxmox://{{ .InstanceID }}
{{- end }}
{{- if .KubernetesVersion }}
kubernetes-version: {{ .KubernetesVersion }}
{{- end }}
`
)

// Metadata provides functionality to render machine metadata.
//
// TODO: Metadata does not belong in the cloudinit package. Instance ID,
// hostname, provider ID and Kubernetes version are machine identity, a concern
// separate from cloud-init network-config rendering, and they are not specific
// to the cloud-init format. This type — together with its ErrMissingHostname /
// ErrMissingInstanceID sentinels in errors.go — should move into its own
// package.
type Metadata struct {
	data BaseCloudInitData
}

// NewMetadata returns a new Metadata object.
func NewMetadata(instanceID, hostname string, kubernetesVersion string, injectProviderID bool) *Metadata {
	ci := new(Metadata)
	ci.data = BaseCloudInitData{
		Hostname:            hostname,
		InstanceID:          instanceID,
		KubernetesVersion:   kubernetesVersion,
		ProviderIDInjection: injectProviderID,
	}
	return ci
}

// Render returns rendered metadata.
func (r *Metadata) Render() (metadata []byte, err error) {
	if err = r.Validate(); err != nil {
		return nil, err
	}

	return render("metadata", metadataTPl, r.data)
}

// Inspect returns a jsonified version for inspection.
func (r *Metadata) Inspect() ([]byte, error) {
	return json.Marshal(r.data)
}

// Validate reports whether the metadata is complete enough to render.
func (r *Metadata) Validate() error {
	if r.data.Hostname == "" {
		return ErrMissingHostname
	}
	if r.data.InstanceID == "" {
		return ErrMissingInstanceID
	}
	return nil
}
