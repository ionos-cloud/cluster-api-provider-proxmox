/*
Copyright 2023-2025 IONOS Cloud.

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
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1alpha2 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcilePowerState(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if !machineHasIPAddress(machineScope.ProxmoxMachine) {
		machineScope.V(4).Info("ip address not set for machine")
		// machine doesn't have an ip address yet
		// needs to reconcile again
		return true, nil
	}

	machineScope.V(4).Info("ensuring machine is started")
	conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition, infrav1alpha2.PoweringOnReason, clusterv1.ConditionSeverityInfo, "")

	t, err := startVirtualMachine(ctx, machineScope.InfraCluster.ProxmoxClient, machineScope.VirtualMachine)
	if err != nil {
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha2.VMProvisionedCondition, infrav1alpha2.PoweringOnFailedReason, clusterv1.ConditionSeverityInfo, "%s", err)
		return false, err
	}

	if t != nil {
		machineScope.ProxmoxMachine.Status.TaskRef = ptr.To(string(t.UPID))
		return true, nil
	}

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
