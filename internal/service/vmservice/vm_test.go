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
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	capierrors "sigs.k8s.io/cluster-api/errors"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/scheduler"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func TestReconcileVM_EverythingReady(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)
	machineScope.ProxmoxMachine.Status.Ready = true

	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil).Once()
	proxmoxClient.EXPECT().CloudInitStatus(context.Background(), vm).Return(false, nil).Once()
	proxmoxClient.EXPECT().QemuAgentStatus(context.Background(), vm).Return(nil).Once()

	result, err := ReconcileVM(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, infrav1alpha1.VirtualMachineStateReady, result.State)
	require.Equal(t, "10.10.10.10", machineScope.ProxmoxMachine.Status.Addresses[1].Address)
}

func TestReconcileVM_QemuAgentCheckDisabled(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)
	machineScope.ProxmoxMachine.Status.Ready = true
	machineScope.ProxmoxMachine.Spec.Checks = &infrav1alpha1.ProxmoxMachineChecks{
		SkipQemuGuestAgent: ptr.To(true),
	}

	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil).Once()
	// proxmoxClient.EXPECT().CloudInitStatus(context.Background(), vm).Return(false, nil).Once()

	result, err := ReconcileVM(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, infrav1alpha1.VirtualMachineStateReady, result.State)
	require.Equal(t, "10.10.10.10", machineScope.ProxmoxMachine.Status.Addresses[1].Address)
}

func TestReconcileVM_CloudInitCheckDisabled(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)
	machineScope.ProxmoxMachine.Status.Ready = true
	machineScope.ProxmoxMachine.Spec.Checks = &infrav1alpha1.ProxmoxMachineChecks{
		SkipCloudInitStatus: ptr.To(true),
	}

	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil).Once()
	proxmoxClient.EXPECT().QemuAgentStatus(context.Background(), vm).Return(nil)

	result, err := ReconcileVM(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, infrav1alpha1.VirtualMachineStateReady, result.State)
	require.Equal(t, "10.10.10.10", machineScope.ProxmoxMachine.Status.Addresses[1].Address)
}

func TestReconcileVM_InitCheckDisabled(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)
	machineScope.ProxmoxMachine.Status.Ready = true
	machineScope.ProxmoxMachine.Spec.Checks = &infrav1alpha1.ProxmoxMachineChecks{
		SkipCloudInitStatus: ptr.To(true),
		SkipQemuGuestAgent:  ptr.To(true),
	}

	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil).Once()

	result, err := ReconcileVM(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, infrav1alpha1.VirtualMachineStateReady, result.State)
	require.Equal(t, "10.10.10.10", machineScope.ProxmoxMachine.Status.Addresses[1].Address)
}

func TestEnsureVirtualMachine_CreateVM_FullOptions(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Description = ptr.To("test vm")
	machineScope.ProxmoxMachine.Spec.Format = ptr.To(infrav1alpha1.TargetStorageFormatRaw)
	machineScope.ProxmoxMachine.Spec.Full = ptr.To(true)
	machineScope.ProxmoxMachine.Spec.Pool = ptr.To("pool")
	machineScope.ProxmoxMachine.Spec.SnapName = ptr.To("snap")
	machineScope.ProxmoxMachine.Spec.Storage = ptr.To("storage")
	machineScope.ProxmoxMachine.Spec.Target = ptr.To("node2")
	expectedOptions := proxmox.VMCloneRequest{
		Node:        "node1",
		Name:        "test",
		Description: "test vm",
		Format:      "raw",
		Full:        1,
		Pool:        "pool",
		SnapName:    "snap",
		Storage:     "storage",
		Target:      "node2",
	}
	response := proxmox.VMCloneResponse{NewID: 123, Task: newTask()}
	proxmoxClient.EXPECT().CloneVM(context.Background(), 123, expectedOptions).Return(response, nil).Once()

	requeue, err := ensureVirtualMachine(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	require.Equal(t, "node2", *machineScope.ProxmoxMachine.Status.ProxmoxNode)
	require.True(t, machineScope.InfraCluster.ProxmoxCluster.HasMachine(machineScope.Name(), false))
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}

func TestEnsureVirtualMachine_CreateVM_SelectNode(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.AllowedNodes = []string{"node1", "node2", "node3"}

	selectNextNode = func(context.Context, *scope.MachineScope) (string, error) {
		return "node3", nil
	}
	t.Cleanup(func() { selectNextNode = scheduler.ScheduleVM })

	expectedOptions := proxmox.VMCloneRequest{Node: "node1", Name: "test", Target: "node3"}
	response := proxmox.VMCloneResponse{NewID: 123, Task: newTask()}
	proxmoxClient.EXPECT().CloneVM(context.Background(), 123, expectedOptions).Return(response, nil).Once()

	requeue, err := ensureVirtualMachine(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)

	require.Equal(t, "node3", *machineScope.ProxmoxMachine.Status.ProxmoxNode)
	require.True(t, machineScope.InfraCluster.ProxmoxCluster.HasMachine(machineScope.Name(), false))
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
}

func TestEnsureVirtualMachine_CreateVM_SelectNode_InsufficientMemory(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.AllowedNodes = []string{"node1"}

	selectNextNode = func(context.Context, *scope.MachineScope) (string, error) {
		return "", fmt.Errorf("error: %w", scheduler.InsufficientMemoryError{})
	}
	t.Cleanup(func() { selectNextNode = scheduler.ScheduleVM })

	_, err := ensureVirtualMachine(context.Background(), machineScope)
	require.Error(t, err)

	require.False(t, machineScope.InfraCluster.ProxmoxCluster.HasMachine(machineScope.Name(), false))
	requireConditionIsFalse(t, machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition)
	require.True(t, machineScope.HasFailed())
}

func TestEnsureVirtualMachine_FindVM(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.SetVirtualMachineID(123)
	vm := newStoppedVM()
	vm.VirtualMachineConfig.SMBios1 = "uuid=56603c36-46b9-4608-90ae-c731c15eae64"

	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil).Once()

	requeue, err := ensureVirtualMachine(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)

	require.Equal(t, vm, machineScope.VirtualMachine)
	require.Equal(t, "proxmox://56603c36-46b9-4608-90ae-c731c15eae64", machineScope.GetProviderID())
}

func TestEnsureVirtualMachine_UpdateVMLocation_Error(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.SetVirtualMachineID(123)

	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(nil, fmt.Errorf("not found")).Once()
	proxmoxClient.EXPECT().FindVMResource(context.Background(), uint64(123)).Return(nil, fmt.Errorf("unavailalbe")).Once()

	_, err := ensureVirtualMachine(context.Background(), machineScope)
	require.Error(t, err)
}

func TestReconcileVirtualMachineConfig_NoConfig(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	vm := newStoppedVM()
	machineScope.SetVirtualMachine(vm)

	requeue, err := reconcileVirtualMachineConfig(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)
}

func TestReconcileVirtualMachineConfig_ApplyConfig(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.NumSockets = 4
	machineScope.ProxmoxMachine.Spec.NumCores = 4
	machineScope.ProxmoxMachine.Spec.MemoryMiB = 16 * 1024
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		Default: &infrav1alpha1.NetworkDevice{Bridge: "vmbr0", Model: ptr.To("virtio"), MTU: ptr.To(uint16(1500))},
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{
				Name:          "net1",
				NetworkDevice: infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio"), MTU: ptr.To(uint16(1500))},
			},
		},
	}

	vm := newStoppedVM()
	task := newTask()
	machineScope.SetVirtualMachine(vm)
	expectedOptions := []interface{}{
		proxmox.VirtualMachineOption{Name: optionSockets, Value: machineScope.ProxmoxMachine.Spec.NumSockets},
		proxmox.VirtualMachineOption{Name: optionCores, Value: machineScope.ProxmoxMachine.Spec.NumCores},
		proxmox.VirtualMachineOption{Name: optionMemory, Value: machineScope.ProxmoxMachine.Spec.MemoryMiB},
		proxmox.VirtualMachineOption{Name: "net0", Value: formatNetworkDevice("virtio", "vmbr0", ptr.To(uint16(1500)), nil)},
		proxmox.VirtualMachineOption{Name: "net1", Value: formatNetworkDevice("virtio", "vmbr1", ptr.To(uint16(1500)), nil)},
	}

	proxmoxClient.EXPECT().ConfigureVM(context.Background(), vm, expectedOptions...).Return(task, nil).Once()

	requeue, err := reconcileVirtualMachineConfig(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.EqualValues(t, task.UPID, *machineScope.ProxmoxMachine.Status.TaskRef)
}

func TestReconcileDisks_RunningVM(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Disks = &infrav1alpha1.Storage{
		BootVolume: &infrav1alpha1.DiskSize{Disk: "ide0", SizeGB: 100},
	}
	machineScope.SetVirtualMachine(newRunningVM())

	require.NoError(t, reconcileDisks(context.Background(), machineScope))
}

func TestReconcileDisks_ResizeDisk(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.Disks = &infrav1alpha1.Storage{
		BootVolume: &infrav1alpha1.DiskSize{Disk: "ide0", SizeGB: 100},
	}
	vm := newStoppedVM()
	machineScope.SetVirtualMachine(vm)

	proxmoxClient.EXPECT().ResizeDisk(context.Background(), vm, "ide0", machineScope.ProxmoxMachine.Spec.Disks.BootVolume.FormatSize()).Return(nil)

	require.NoError(t, reconcileDisks(context.Background(), machineScope))
}

func TestReconcileMachineAddresses_IPV4(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.SetVirtualMachine(vm)
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)

	require.NoError(t, reconcileMachineAddresses(machineScope))
	require.Equal(t, machineScope.ProxmoxMachine.Status.Addresses[0].Address, machineScope.ProxmoxMachine.GetName())
	require.Equal(t, machineScope.ProxmoxMachine.Status.Addresses[1].Address, "10.10.10.10")
}

func TestReconcileMachineAddresses_IPV6(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.IPv4Config = nil
	machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config = &infrav1alpha1.IPConfigSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}

	vm := newRunningVM()
	machineScope.SetVirtualMachine(vm)
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV6: "2001:db8::2"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)

	require.NoError(t, reconcileMachineAddresses(machineScope))
	require.Equal(t, machineScope.ProxmoxMachine.Status.Addresses[0].Address, machineScope.ProxmoxMachine.GetName())
	require.Equal(t, machineScope.ProxmoxMachine.Status.Addresses[1].Address, "2001:db8::2")
}

func TestReconcileMachineAddresses_DualStack(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config = &infrav1alpha1.IPConfigSpec{
		Addresses: []string{"2001:db8::/64"},
		Prefix:    64,
		Gateway:   "2001:db8::1",
	}

	vm := newRunningVM()
	machineScope.SetVirtualMachine(vm)
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10", IPV6: "2001:db8::2"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)

	require.NoError(t, reconcileMachineAddresses(machineScope))
	require.Equal(t, machineScope.ProxmoxMachine.Status.Addresses[0].Address, machineScope.ProxmoxMachine.GetName())
	require.Equal(t, machineScope.ProxmoxMachine.Status.Addresses[1].Address, "10.10.10.10")
	require.Equal(t, machineScope.ProxmoxMachine.Status.Addresses[2].Address, "2001:db8::2")
}

func TestReconcileVirtualMachineConfigVLAN(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.NumSockets = 4
	machineScope.ProxmoxMachine.Spec.NumCores = 4
	machineScope.ProxmoxMachine.Spec.MemoryMiB = 16 * 1024
	machineScope.ProxmoxMachine.Spec.Network = &infrav1alpha1.NetworkSpec{
		Default: &infrav1alpha1.NetworkDevice{Bridge: "vmbr0", Model: ptr.To("virtio"), VLAN: ptr.To(uint16(100))},
		AdditionalDevices: []infrav1alpha1.AdditionalNetworkDevice{
			{
				Name:          "net1",
				NetworkDevice: infrav1alpha1.NetworkDevice{Bridge: "vmbr1", Model: ptr.To("virtio"), VLAN: ptr.To(uint16(100))},
			},
		},
	}

	vm := newStoppedVM()
	task := newTask()
	machineScope.SetVirtualMachine(vm)
	expectedOptions := []interface{}{
		proxmox.VirtualMachineOption{Name: optionSockets, Value: machineScope.ProxmoxMachine.Spec.NumSockets},
		proxmox.VirtualMachineOption{Name: optionCores, Value: machineScope.ProxmoxMachine.Spec.NumCores},
		proxmox.VirtualMachineOption{Name: optionMemory, Value: machineScope.ProxmoxMachine.Spec.MemoryMiB},
		proxmox.VirtualMachineOption{Name: "net0", Value: formatNetworkDevice("virtio", "vmbr0", nil, ptr.To(uint16(100)))},
		proxmox.VirtualMachineOption{Name: "net1", Value: formatNetworkDevice("virtio", "vmbr1", nil, ptr.To(uint16(100)))},
	}

	proxmoxClient.EXPECT().ConfigureVM(context.TODO(), vm, expectedOptions...).Return(task, nil).Once()

	requeue, err := reconcileVirtualMachineConfig(context.TODO(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.EqualValues(t, task.UPID, *machineScope.ProxmoxMachine.Status.TaskRef)
}

func TestReconcileDisks_UnmountCloudInitISO(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)

	vm := newRunningVM()
	vm.VirtualMachineConfig.IDE0 = "local:iso/cloud-init.iso,media=cdrom"
	machineScope.SetVirtualMachine(vm)

	proxmoxClient.EXPECT().UnmountCloudInitISO(context.Background(), vm, "ide0").Return(nil)

	require.NoError(t, unmountCloudInitISO(context.Background(), machineScope))
}

func TestReconcileVM_CloudInitFailed(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)
	machineScope.ProxmoxMachine.Status.Ready = true

	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil).Once()
	proxmoxClient.EXPECT().CloudInitStatus(context.Background(), vm).Return(false, goproxmox.ErrCloudInitFailed).Once()
	proxmoxClient.EXPECT().QemuAgentStatus(context.Background(), vm).Return(nil).Once()

	_, err := ReconcileVM(context.Background(), machineScope)
	require.Error(t, err, "unknown error")
	require.Equal(t, machineScope.ProxmoxMachine.Status.FailureReason, ptr.To(capierrors.MachineStatusError("BootstrapFailed")))
	require.Equal(t, machineScope.ProxmoxMachine.Status.FailureMessage, ptr.To("cloud-init failed execution"))
}

func TestReconcileVM_CloudInitRunning(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	vm := newRunningVM()
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.IPAddresses = map[string]infrav1alpha1.IPAddress{infrav1alpha1.DefaultNetworkDevice: {IPV4: "10.10.10.10"}}
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)
	machineScope.ProxmoxMachine.Status.Ready = true

	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil).Once()
	proxmoxClient.EXPECT().CloudInitStatus(context.Background(), vm).Return(true, nil).Once()
	proxmoxClient.EXPECT().QemuAgentStatus(context.Background(), vm).Return(nil).Once()

	result, err := ReconcileVM(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, infrav1alpha1.VirtualMachineStatePending, result.State)
}
