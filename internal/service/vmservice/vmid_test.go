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

package vmservice

import (
	"context"
	"sync"
	"testing"
	"time"

	goproxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
)

// Two machines cloned back to back must not get the same VMID, even though Proxmox keeps
// answering the same next free ID until the first clone's config exists.
func TestGetVMID_ReservesHandedOutIDs(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	proxmoxClient.EXPECT().CheckID(mock.Anything, int64(101)).Return(true, nil).Once()

	first, err := getVMID(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, int64(100), first)

	second, err := getVMID(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, int64(101), second)

	// Once the first VM exists (or its clone was abandoned) the ID is free again.
	releaseVMID(first)
	third, err := getVMID(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, int64(100), third)
}

func TestGetVMID_ReservationExpires(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	proxmoxClient.EXPECT().CheckID(mock.Anything, int64(101)).Return(true, nil).Maybe()

	now := time.Now()
	reservedVMIDs.now = func() time.Time { return now }
	t.Cleanup(func() { reservedVMIDs.now = time.Now })

	first, err := getVMID(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, int64(100), first)

	now = now.Add(vmidReservationTTL / 2)
	second, err := getVMID(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, int64(101), second, "a live reservation is honoured")

	now = now.Add(vmidReservationTTL)
	third, err := getVMID(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, int64(100), third, "an expired reservation is dropped")
}

func TestGetVMID_RangeSkipsTakenAndReservedIDs(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	machineScope.ProxmoxMachine.Spec.VMIDRange = &infrav1.VMIDRange{Start: 200, End: 202}
	proxmoxClient.EXPECT().CheckID(mock.Anything, int64(200)).Return(false, nil).Times(3)
	proxmoxClient.EXPECT().CheckID(mock.Anything, int64(201)).Return(true, nil).Once()
	proxmoxClient.EXPECT().CheckID(mock.Anything, int64(202)).Return(true, nil).Once()

	first, err := getVMID(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, int64(201), first, "200 is taken in Proxmox")

	second, err := getVMID(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, int64(202), second, "201 is reserved by the first machine")

	_, err = getVMID(context.Background(), machineScope)
	require.ErrorIs(t, err, ErrNoVMIDInRangeFree)
}

func TestGetVMID_ConcurrentAllocationsAreDistinct(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	proxmoxClient.EXPECT().CheckID(mock.Anything, mock.Anything).Return(true, nil).Maybe()

	const n = 32
	ids := make(chan int64, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			id, err := getVMID(context.Background(), machineScope)
			require.NoError(t, err)
			ids <- id
		})
	}
	wg.Wait()
	close(ids)

	seen := map[int64]bool{}
	for id := range ids {
		require.False(t, seen[id], "vmid %d handed out twice", id)
		seen[id] = true
	}
	require.Len(t, seen, n)
}

func TestGetVMID_NextIDError(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTest(t)
	// Register before the setup's Maybe expectation can answer: a fresh mock has the setup's
	// unlimited expectation first, so use a client of our own.
	proxmoxClient.ExpectedCalls = nil
	proxmoxClient.EXPECT().NextID(mock.Anything).Return(int64(0), goproxmox.ErrNotAuthorized).Once()

	_, err := getVMID(context.Background(), machineScope)
	require.ErrorIs(t, err, goproxmox.ErrNotAuthorized)
}

// A clone that failed for a transient reason leaves CloningFailed behind. The retried clone
// must put the machine back into Cloning, or every later step skips.
func TestEnsureVirtualMachine_CloningFailedRetryReentersCloning(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningFailedReason)
	expectedOptions := proxmox.VMCloneRequest{Node: "node1", NewID: 100, Name: "test", Full: true}
	proxmoxClient.EXPECT().CloneVM(context.Background(), 123, expectedOptions).
		Return(proxmox.VMCloneResponse{NewID: 100, Task: newTask()}, nil).Once()

	requeue, err := ensureVirtualMachine(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.Equal(t, int64(100), machineScope.GetVirtualMachineID())
	require.Equal(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason,
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
}

// A failed clone keeps CloningFailed and releases the ID it had reserved.
func TestEnsureVirtualMachine_CloneErrorReleasesVMID(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
	expectedOptions := proxmox.VMCloneRequest{Node: "node1", NewID: 100, Name: "test", Full: true}
	proxmoxClient.EXPECT().CloneVM(context.Background(), 123, expectedOptions).
		Return(proxmox.VMCloneResponse{}, goproxmox.ErrNotAuthorized).Once()

	_, err := ensureVirtualMachine(context.Background(), machineScope)
	require.Error(t, err)
	require.Equal(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningFailedReason,
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))

	reservedVMIDs.mu.Lock()
	defer reservedVMIDs.mu.Unlock()
	require.False(t, reservedVMIDs.held(100))
}

// The VM exists, so its reservation is gone.
func TestEnsureVirtualMachine_FoundVMReleasesVMID(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
	vm := newRunningVM()
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	reservedVMIDs.mu.Lock()
	reservedVMIDs.reserve(int64(vm.VMID))
	reservedVMIDs.mu.Unlock()
	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(vm.VMID)).Return(vm, nil).Once()

	requeue, err := ensureVirtualMachine(context.Background(), machineScope)
	require.NoError(t, err)
	require.False(t, requeue)

	reservedVMIDs.mu.Lock()
	defer reservedVMIDs.mu.Unlock()
	require.False(t, reservedVMIDs.held(int64(vm.VMID)))
}

// A reason no step handles, with a VM that exists but was never started, is what the
// unguarded state machine turned into a "ready" machine. It must be an error instead.
func TestReconcileVM_StoppedVMWithUnhandledReasonIsNotReady(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningFailedReason)
	vm := newStoppedVM()
	vm.VMID = 123
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil)
	proxmoxClient.EXPECT().HasActiveTask(context.Background(), vm).Return(false, nil)

	result, err := ReconcileVM(context.Background(), machineScope)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not running")
	require.NotEqual(t, infrav1.VirtualMachineStateReady, result.State)
	require.Nil(t, machineScope.ProxmoxMachine.Status.Initialization.Provisioned)
}

// A machine that was provisioned earlier and whose VM is stopped is still reported, as before.
func TestReconcileVM_ProvisionedStoppedVMStaysReady(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapReadyReason)
	vm := newStoppedVM()
	vm.VMID = 123
	machineScope.SetVirtualMachineID(int64(vm.VMID))
	machineScope.ProxmoxMachine.Status.Initialization.Provisioned = new(true)
	proxmoxClient.EXPECT().GetVM(context.Background(), "node1", int64(123)).Return(vm, nil)

	result, err := ReconcileVM(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, infrav1.VirtualMachineStateReady, result.State)
}

func TestReconcileVirtualMachineConfig_RemovesInheritedCloudInitDrive(t *testing.T) {
	machineScope, proxmoxClient, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
	vm := newStoppedVM()
	vm.VirtualMachineConfig.IDEs = map[string]string{
		"ide0": "local:iso/user-data-123.iso,media=cdrom",
		"ide2": "nfs-images:123/vm-123-cloudinit.qcow2,media=cdrom",
	}
	vm.VirtualMachineConfig.SCSIs = map[string]string{
		"scsi0": "nfs-images:123/vm-123-disk-0.qcow2,size=20G",
		"scsi1": "local-lvm:vm-123-cloudinit,media=cdrom",
	}
	machineScope.SetVirtualMachine(vm)
	task := newTask()
	proxmoxClient.EXPECT().ConfigureVM(context.Background(), vm,
		proxmox.VirtualMachineOption{Name: optionDelete, Value: "ide2,scsi1"}).Return(task, nil).Once()

	requeue, err := reconcileVirtualMachineConfig(context.Background(), machineScope)
	require.NoError(t, err)
	require.True(t, requeue)
	require.EqualValues(t, task.UPID, *machineScope.ProxmoxMachine.Status.TaskRef)
	require.Equal(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForDiskReconciliationReason,
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
}

func TestCloudInitDrives(t *testing.T) {
	require.Nil(t, cloudInitDrives(nil))
	require.Empty(t, cloudInitDrives(&goproxmox.VirtualMachineConfig{}))
	require.Empty(t, cloudInitDrives(&goproxmox.VirtualMachineConfig{
		IDEs:  map[string]string{"ide0": "local:iso/user-data-7.iso,media=cdrom", "ide2": "none,media=cdrom"},
		SCSIs: map[string]string{"scsi0": "local-lvm:vm-7-disk-0,size=32G"},
	}), "the provider's ISO and a plain disk are not cloud-init drives")
	require.Equal(t, []string{"ide2", "sata1", "scsi3", "virtio2"}, cloudInitDrives(&goproxmox.VirtualMachineConfig{
		IDEs:    map[string]string{"ide2": "local-lvm:vm-7-cloudinit,media=cdrom"},
		SCSIs:   map[string]string{"scsi3": "tank:vm-7-cloudinit,media=cdrom,size=4M"},
		SATAs:   map[string]string{"sata1": "nfs:7/vm-7-cloudinit.qcow2,media=cdrom"},
		VirtIOs: map[string]string{"virtio2": "ceph:vm-7-cloudinit"},
	}))
	require.Empty(t, cloudInitDrives(&goproxmox.VirtualMachineConfig{
		SCSIs: map[string]string{"scsi0": "local-lvm:vm-7-cloudinit-data,size=1G"},
	}), "a disk whose name merely starts with the cloud-init prefix is kept")
}
