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

const (
	metadataTPl = `instance-id: {{ .InstanceID }}
local-hostname: {{ .Hostname }}
hostname: {{ .Hostname }}
{{- if .ProviderIDInjection }}
provider-id: proxmox://{{ .InstanceID }}
{{- end }}
`
)

// Metadata provides functionality to render machine metadata.
type Metadata struct {
	data BaseCloudInitData
}

// NewMetadata returns a new Metadata object.
func NewMetadata(instanceID, hostname string, injectProviderID bool) *Metadata {
	ci := new(Metadata)
	ci.data = BaseCloudInitData{
		Hostname:            hostname,
		InstanceID:          instanceID,
		ProviderIDInjection: injectProviderID,
	}
	return ci
}

// Render returns rendered metadata.
func (r *Metadata) Render() (metadata []byte, err error) {
	if err = r.validate(); err != nil {
		return nil, err
	}

	return render("metadata", metadataTPl, r.data)
}

func (r *Metadata) validate() error {
	if r.data.Hostname == "" {
		return ErrMissingHostname
	}
	if r.data.InstanceID == "" {
		return ErrMissingInstanceID
	}
	return nil
}
