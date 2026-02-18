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
	"fmt"

	"github.com/luthermonson/go-proxmox"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcilePowerState(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition) != infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason {
		// Machine is in the wrong state to reconcile, we only reconcile machines waiting to power on
		return false, nil
	}

	/*
		if !machineHasIPAddress(machineScope.ProxmoxMachine) {
			machineScope.V(4).Info("ip address not set for machine")
			// machine doesn't have an ip address yet
			// needs to reconcile again
			return true, nil
		}
	*/

	machineScope.V(4).Info("ensuring machine is started")

	t, err := startVirtualMachine(ctx, machineScope.InfraCluster.ProxmoxClient, machineScope.VirtualMachine)
	if err != nil {
		conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
			Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.ProxmoxMachineVirtualMachineProvisionedPoweringOnFailedReason,
			Message: fmt.Sprintf("%s", err),
		})
		return false, err
	}

	if t != nil {
		machineScope.ProxmoxMachine.Status.TaskRef = ptr.To(string(t.UPID))
		return true, nil
	}

	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status: metav1.ConditionFalse,
		Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForCloudInitReason,
	})
	return false, nil
}

func startVirtualMachine(ctx context.Context, client capmox.Client, vm *proxmox.VirtualMachine) (*proxmox.Task, error) {
	if vm.IsPaused() {
		t, err := client.ResumeVM(ctx, vm)
		if err != nil {
			return nil, fmt.Errorf("unable to resume the virtual machine %d: %w", vm.VMID, err)
		}

		return t, nil
	}

	if vm.IsStopped() || vm.IsHibernated() {
		t, err := client.StartVM(ctx, vm)
		if err != nil {
			return nil, fmt.Errorf("unable to start the virtual machine %d: %w", vm.VMID, err)
		}

		return t, nil
	}

	// nothing to do.
	return nil, nil
}
