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
	"strings"

	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/service/taskservice"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

// errVMNotOwned means the recorded VMID is free, or holds a container or another machine's VM.
var errVMNotOwned = errors.New("vmid is not held by this machine's vm")

// DeleteVM destroys the machine's VM one Proxmox task per reconcile: a stop while the VM
// runs, then the destroy, and finally the check that the VMID is free again. Each task is
// recorded in status.taskRef so the next pass waits for it instead of issuing it again
// (the deletion path is entered on every Machine and ProxmoxMachine event, and a destroy of
// a VM whose stop has not finished fails with "VM <id> is running - destroy failed"). A
// failed task sets the DeletionFailed reason with the task's exit status and is retried
// after status.retryAfter.
//
// Proxmox hands a freed VMID to the next clone within seconds, so the recorded ID is never
// trusted on its own: the VM is located cluster-wide and destroyed only when its config
// carries this machine's name. The finalizer comes off once the VM is confirmed gone, or
// once the ID turns out never to have been this machine's.
//
// It returns requeue when a task runs or the VM is still there.
func DeleteVM(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	markDeleting(machineScope, false)

	// A stop or destroy issued by an earlier pass: wait for it, and after a failure let
	// RetryAfter pace the retry (DeletionFailed is set by the task service meanwhile).
	inFlight, err := taskservice.ReconcileInFlightTask(ctx, machineScope)
	switch {
	case errors.Is(err, taskservice.ErrTaskNotFound):
		// Proxmox has no record of the task, so it cannot be waited for; the VM's own
		// state decides what to do next.
		machineScope.Info("Proxmox does not know the recorded task; checking the vm instead", "task", *machineScope.ProxmoxMachine.Status.TaskRef)
		machineScope.ProxmoxMachine.Status.TaskRef = nil
	case err != nil:
		return false, err
	case inFlight:
		return true, nil
	}

	vmID := machineScope.GetVirtualMachineID()
	if vmID <= 0 {
		// The machine never got a VM.
		return false, releaseMachine(machineScope, vmID)
	}

	vm, err := locateOwnedVM(ctx, machineScope, vmID)
	switch {
	case errors.Is(err, errVMNotOwned):
		return false, releaseMachine(machineScope, vmID)
	case errors.Is(err, ErrVMNotInitialized):
		machineScope.V(4).Info("vm has no name yet; waiting before deciding whether it is this machine's", "vmID", vmID)
		return true, nil
	case err != nil:
		return false, err
	}

	// A task on the VM that this pass does not know about (the status update recording a
	// stop or destroy was lost, or a clone is still writing the VM) holds its config lock;
	// a second stop or destroy would only fail against it.
	if vm.Lock == lockClone {
		machineScope.V(4).Info("waiting for the clone of the vm to finish before deleting it", "vmID", vmID)
		return true, nil
	}
	busy, err := machineScope.InfraCluster.ProxmoxClient.HasActiveTask(ctx, vm)
	if err != nil {
		return false, err
	}
	if busy {
		machineScope.V(4).Info("waiting for an active task on the vm before deleting it", "vmID", vmID)
		return true, nil
	}

	client := machineScope.InfraCluster.ProxmoxClient
	if vm.Status != proxmox.StatusVirtualMachineStopped {
		task, err := client.StopVM(ctx, vm)
		if err != nil {
			markDeletionFailed(machineScope, err)
			return false, err
		}
		machineScope.Info("stopping vm", "vmID", vmID, "node", vm.Node, "task", task.UPID)
		machineScope.ProxmoxMachine.Status.TaskRef = new(string(task.UPID))
		markDeleting(machineScope, true)
		return true, nil
	}

	task, err := client.DeleteVM(ctx, vm.Node, vmID)
	if err != nil {
		if VMNotFound(err) {
			// Gone between the lookup and the destroy.
			return false, releaseMachine(machineScope, vmID)
		}
		markDeletionFailed(machineScope, err)
		return false, err
	}
	// Keep the freed ID out of the next clones for a while; see quarantineVMID.
	quarantineVMID(vmID)
	if task == nil {
		return true, nil
	}
	machineScope.Info("destroying vm", "vmID", vmID, "node", vm.Node, "task", task.UPID)
	machineScope.ProxmoxMachine.Status.TaskRef = new(string(task.UPID))
	markDeleting(machineScope, true)
	return true, nil
}

// locateOwnedVM returns the VM that holds vmID when it is this machine's. It returns
// errVMNotOwned when the ID is free or holds a container or a VM with another name, and
// ErrVMNotInitialized while the VM has no name yet (a clone in progress, ours or the next
// holder's). Any other error means the state could not be read, and a VM that was not
// confirmed is never destroyed.
func locateOwnedVM(ctx context.Context, machineScope *scope.MachineScope, vmID int64) (*proxmox.VirtualMachine, error) {
	client := machineScope.InfraCluster.ProxmoxClient

	free, err := client.CheckID(ctx, vmID)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to check whether vmid %d is free", vmID)
	}
	if free {
		machineScope.Info("vmid is free, the vm is gone", "vmID", vmID)
		return nil, errVMNotOwned
	}

	// Cluster-wide, since the VM may have been migrated off the recorded node.
	rsc, err := client.FindVMResource(ctx, uint64(vmID))
	if err != nil {
		return nil, err
	}
	// Containers share the ID space, and a machine is never a container.
	if rsc.Type != "qemu" {
		machineScope.Logger.Error(errVMNotOwned, "refusing to destroy: the vmid recorded for this machine is held by a container",
			"vmID", vmID, "type", rsc.Type, "name", rsc.Name, "node", rsc.Node)
		return nil, errVMNotOwned
	}

	// /cluster/resources reports the name from statistics that lag the config by up to
	// about ten seconds, so just after the ID was reused it can still show the previous
	// holder, which may be this machine. Only the VM config on the node is current.
	vm, err := client.GetVM(ctx, rsc.Node, vmID)
	if err != nil {
		return nil, err
	}

	switch vm.Name {
	case machineScope.Name():
		return vm, nil
	case "", placeholderVMName(vmID):
		return nil, ErrVMNotInitialized
	default:
		machineScope.Logger.Error(errVMNotOwned, "refusing to destroy: the vmid recorded for this machine is held by a vm with another name",
			"vmID", vmID, "vmName", vm.Name, "node", rsc.Node)
		return nil, errVMNotOwned
	}
}

// releaseMachine lets the ProxmoxMachine go: the VM is confirmed gone, or the recorded ID was
// never this machine's.
func releaseMachine(machineScope *scope.MachineScope, vmID int64) error {
	quarantineVMID(vmID)
	machineScope.InfraCluster.ProxmoxCluster.RemoveNodeLocation(machineScope.Name(), util.IsControlPlaneMachine(machineScope.Machine))
	ctrlutil.RemoveFinalizer(machineScope.ProxmoxMachine, infrav1.MachineFinalizer)
	return machineScope.InfraCluster.PatchObject()
}

// markDeleting sets the Deleting reason. Unless force is set, a DeletionFailed reason is left
// in place so the user keeps seeing what failed while the retry waits.
func markDeleting(machineScope *scope.MachineScope, force bool) {
	reason := conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
	if !force && (reason == infrav1.ProxmoxMachineVirtualMachineProvisionedDeletingReason ||
		reason == infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason) {
		return
	}
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status: metav1.ConditionFalse,
		Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedDeletingReason,
	})
}

func markDeletionFailed(machineScope *scope.MachineScope, err error) {
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status:  metav1.ConditionFalse,
		Reason:  infrav1.ProxmoxMachineVirtualMachineProvisionedDeletionFailedReason,
		Message: err.Error(),
	})
}

// VMNotFound checks if the given err is related to that the VM is not found in Proxmox.
func VMNotFound(err error) bool {
	return strings.Contains(err.Error(), "does not exist")
}
