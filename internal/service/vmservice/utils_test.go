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

package vmservice

import (
	"testing"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestExtractUUID(t *testing.T) {
	type match struct {
		test     string
		expected string
	}

	goodstrings := []match{
		{"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b", "7dd9b137-6a3c-4661-a4fa-375075e1776b"},
		{"foo=bar,uuid=71A5f8b4-5d30-43a3-b902-242393ad80b5,baz=quux", "71a5f8b4-5d30-43a3-b902-242393ad80b5"},
		{",uuid=e80432e2-2b5c-4539-af97-852aaa7e84d7", "e80432e2-2b5c-4539-af97-852aaa7e84d7"},
	}

	badstrings := []string{
		"fuuid=4594e7d0-3aa6-4235-95b2-6b0018192b0a",
		"uuid=123abc-def",
		"uid=8a689fee-1653-40ec-a4bf-e88b8dabacd6",
		"uuid",
		"",
		"foobar",
	}

	for _, m := range goodstrings {
		require.Equal(t, m.expected, extractUUID(m.test))
	}

	for _, s := range badstrings {
		require.Equal(t, "", extractUUID(s))
	}
}

func TestExtractNetworkModelAndBridge(t *testing.T) {
	type match struct {
		test           string
		expectedModel  string
		expectedBridge string
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1", "virtio", "vmbr1"},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1", "virtio", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1", "virtio", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,foo", "virtio", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,foo=bar", "virtio", "vmbr1"},
	}

	badstrings := []string{
		"bridge=vmbr1",
		"virtio=,bridge=vmbr1",
		"virtio=,bridge=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"",
	}

	for _, m := range goodstrings {
		model, bridge := extractNetworkModelAndBridge(m.test)
		require.Equal(t, m.expectedModel, model)
		require.Equal(t, m.expectedBridge, bridge)
	}

	for _, s := range badstrings {
		model, bridge := extractNetworkModelAndBridge(s)
		require.Empty(t, model)
		require.Empty(t, bridge)
	}
}

func TestShouldUpdateNetworkDevices_NoNetworkConfig(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	require.False(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_MissingDefaultDeviceOnVM(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		Default: &infrav1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")},
	}
	machineScope.SetVirtualMachine(newStoppedVM())

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_DefaultDeviceNeedsUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		Default: &infrav1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_MissingAdditionalDeviceOnVM(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		AdditionalDevices: []infrav1.AdditionalNetworkDevice{
			{Name: "net1", NetworkDevice: infrav1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")}},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_AdditionalDeviceNeedsUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		AdditionalDevices: []infrav1.AdditionalNetworkDevice{
			{Name: "net1", NetworkDevice: infrav1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")}},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("", "virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_NoUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		Default: &infrav1.NetworkDevice{Bridge: "vmbr0", Model: ptr.To("virtio")},
		AdditionalDevices: []infrav1.AdditionalNetworkDevice{
			{Name: "net1", NetworkDevice: infrav1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")}},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CD,bridge=vmbr0", "virtio=A6:23:64:4D:84:CD,bridge=vmbr1"))

	require.False(t, shouldUpdateNetworkDevices(machineScope))
}
