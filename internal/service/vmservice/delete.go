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
	"context"
	"strings"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

// DeleteVM implements the logic of destroying a VM.
func DeleteVM(ctx context.Context, machineScope *scope.MachineScope) error {
	vmID := machineScope.ProxmoxMachine.GetVirtualMachineID()
	node := machineScope.LocateProxmoxNode()

	if _, err := machineScope.InfraCluster.ProxmoxClient.DeleteVM(ctx, node, vmID); err != nil {
		if VMNotFound(err) {
			// remove machine from cluster status
			machineScope.InfraCluster.ProxmoxCluster.RemoveNodeLocation(machineScope.Name(), util.IsControlPlaneMachine(machineScope.Machine))
			// The VM is deleted so remove the finalizer.
			ctrlutil.RemoveFinalizer(machineScope.ProxmoxMachine, infrav1alpha1.MachineFinalizer)
			return machineScope.InfraCluster.PatchObject()
		}
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition, clusterv1.DeletionFailedReason, clusterv1.ConditionSeverityWarning, "")
		return err
	}

	return nil
}

// VMNotFound checks if the given err is related to that the VM is not found in Proxmox.
func VMNotFound(err error) bool {
	return strings.Contains(err.Error(), "does not exist")
}
