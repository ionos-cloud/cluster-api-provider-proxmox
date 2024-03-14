/*
Copyright 2023-2024 IONOS Cloud.

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

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
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

func TestExtractNetworkModel(t *testing.T) {
	type match struct {
		test     string
		expected string
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "virtio"},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "virtio"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "virtio"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,foo", "virtio"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,foo=bar", "virtio"},
		{"foo=bar,e1000=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,foo=bar", "e1000"},
		{"foo=bar,e1000=a6:23:64:4d:84:Cb,bridge=vmbr1,mtu=9000,foo=bar", "e1000"},
	}

	badstrings := []string{
		"bridge=vmbr1",
		"virtio=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"a6:23:64:4d:84:Cb",
		"=",
		"",
	}

	for _, m := range goodstrings {
		model := extractNetworkModel(m.test)
		require.Equal(t, m.expected, model)
	}

	for _, s := range badstrings {
		model := extractNetworkModel(s)
		require.Empty(t, model)
	}
}

func TestExtractNetworkBridge(t *testing.T) {
	type match struct {
		test     string
		expected string
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "vmbr1"},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,foo", "vmbr1"},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,foo=bar", "vmbr1"},
	}

	badstrings := []string{
		"virtio=",
		"bridge=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"",
	}

	for _, m := range goodstrings {
		bridge := extractNetworkBridge(m.test)
		require.Equal(t, m.expected, bridge)
	}

	for _, s := range badstrings {
		bridge := extractNetworkBridge(s)
		require.Equal(t, "unknown", bridge)
	}
}

func TestExtractNetworkMTU(t *testing.T) {
	type match struct {
		test     string
		expected uint16
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", 1500},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", 1500},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500", 1500},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,foo", 9000},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,foo=bar", 9000},
	}

	badstrings := []string{
		"virtio=",
		"bridge=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"",
	}

	for _, m := range goodstrings {
		mtu := extractNetworkMTU(m.test)
		require.Equal(t, m.expected, mtu)
	}

	for _, s := range badstrings {
		mtu := extractNetworkMTU(s)
		require.Equal(t, uint16(0), mtu)
	}
}

func TestShouldUpdateNetworkDevices_NoNetworkConfig(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)

	require.False(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_MissingDefaultDeviceOnVM(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		Default: &infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")},
	}
	machineScope.SetVirtualMachine(newStoppedVM())

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_DefaultDeviceNeedsUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		Default: &infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_MissingAdditionalDeviceOnVM(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{Name: "net1", NetworkDevice: infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")}},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_AdditionalDeviceNeedsUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{Name: "net1", NetworkDevice: infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio")}},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("", "virtio=A6:23:64:4D:84:CB,bridge=vmbr0"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}

func TestShouldUpdateNetworkDevices_NoUpdate(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		Default: &infrav1alpha1.NetworkDevice{Bridge: "vmbr0", Model: ptr.To("virtio"), MTU: ptr.To(uint16(1500))},
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{Name: "net1", NetworkDevice: infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio"), MTU: ptr.To(uint16(1500))}},
		},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CD,bridge=vmbr0,mtu=1500", "virtio=A6:23:64:4D:84:CD,bridge=vmbr1,mtu=1500"))

	require.False(t, shouldUpdateNetworkDevices(machineScope))
}

func TestExtractNetworkVLAN(t *testing.T) {
	type match struct {
		test     string
		expected uint16
	}

	goodstrings := []match{
		{"virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,tag=100", 100},
		{"foo,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,tag=100", 100},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=1500,tag=100", 100},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,tag=200,foo", 200},
		{"foo=bar,virtio=A6:23:64:4D:84:CB,bridge=vmbr1,mtu=9000,tag=200,foo=bar", 200},
	}

	badstrings := []string{
		"virtio=",
		"bridge=",
		"uuid=7dd9b137-6a3c-4661-a4fa-375075e1776b",
		"",
	}

	for _, m := range goodstrings {
		vlan := extractNetworkVLAN(m.test)
		require.Equal(t, m.expected, vlan)
	}

	for _, s := range badstrings {
		vlan := extractNetworkVLAN(s)
		require.Equal(t, uint16(0), vlan)
	}
}

func TestShouldUpdateNetworkDevices_VLANChanged(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		Default: &infrav1alpha1.NetworkDevice{Bridge: "vmbr0", Model: ptr.To("virtio"), VLAN: ptr.To(uint16(100))},
	}
	machineScope.SetVirtualMachine(newVMWithNets("virtio=A6:23:64:4D:84:CB,bridge=vmbr0,tag=101"))

	require.True(t, shouldUpdateNetworkDevices(machineScope))
}
