/*
Copyright 2026 IONOS Cloud.

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

package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

func machineTemplateWithSourceNode(node string, annotations map[string]string) *infrav1.ProxmoxMachineTemplate {
	tmpl := &infrav1.ProxmoxMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "test-template", Annotations: annotations},
	}
	tmpl.Spec.Template.Spec.SourceNode = ptr.To(node)
	return tmpl
}

// TestProxmoxMachineTemplateValidateUpdate verifies that ValidateUpdate enforces
// spec immutability for normal updates but skips the check for Cluster API
// topology-controller dry-run requests, as required by the CAPI InfraMachine
// provider contract (see issue #735).
func TestProxmoxMachineTemplateValidateUpdate(t *testing.T) {
	webhook := &ProxmoxMachineTemplate{}
	oldTmpl := machineTemplateWithSourceNode("node-a", nil)

	tests := []struct {
		name        string
		newTmpl     *infrav1.ProxmoxMachineTemplate
		dryRun      bool
		wantErr     bool
		errContains string
	}{
		{
			name:    "no spec change is allowed",
			newTmpl: machineTemplateWithSourceNode("node-a", nil),
			dryRun:  false,
			wantErr: false,
		},
		{
			name:        "spec change on a normal update is rejected",
			newTmpl:     machineTemplateWithSourceNode("node-b", nil),
			dryRun:      false,
			wantErr:     true,
			errContains: "immutable",
		},
		{
			name:        "spec change on a plain (non-topology) dry-run is still rejected",
			newTmpl:     machineTemplateWithSourceNode("node-b", nil),
			dryRun:      true,
			wantErr:     true,
			errContains: "immutable",
		},
		{
			name:    "spec change on a topology-controller dry-run is allowed",
			newTmpl: machineTemplateWithSourceNode("node-b", map[string]string{clusterv1.TopologyDryRunAnnotation: ""}),
			dryRun:  true,
			wantErr: false,
		},
		{
			name:        "topology dry-run annotation without a dry-run request is rejected",
			newTmpl:     machineTemplateWithSourceNode("node-b", map[string]string{clusterv1.TopologyDryRunAnnotation: ""}),
			dryRun:      false,
			wantErr:     true,
			errContains: "immutable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(tt.dryRun)}}
			ctx := admission.NewContextWithRequest(context.Background(), req)

			_, err := webhook.ValidateUpdate(ctx, oldTmpl, tt.newTmpl)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestProxmoxMachineTemplateValidateUpdateMissingRequest ensures a missing
// admission.Request in the context yields a BadRequest rather than a panic.
func TestProxmoxMachineTemplateValidateUpdateMissingRequest(t *testing.T) {
	webhook := &ProxmoxMachineTemplate{}
	oldTmpl := machineTemplateWithSourceNode("node-a", nil)
	newTmpl := machineTemplateWithSourceNode("node-b", nil)

	_, err := webhook.ValidateUpdate(context.Background(), oldTmpl, newTmpl)
	require.Error(t, err)
	require.Contains(t, err.Error(), "admission.Request")
}
