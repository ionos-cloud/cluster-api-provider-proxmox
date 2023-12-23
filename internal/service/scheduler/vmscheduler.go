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

// Package scheduler implements scheduling algorithms for Proxmox VMs.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
	"sigs.k8s.io/cluster-api/util"
)

// InsufficientResourcesError is used when the scheduler cannot assign a VM to a node because no node
// would be able to provide the requested resources.
type InsufficientResourcesError struct {
	requestedMemory uint64
	requestedCores  uint64
}

func (err InsufficientResourcesError) Error() string {
	return fmt.Sprintf("cannot reserve %dB of memory and/or %d vCores in cluster",
		err.requestedMemory, err.requestedCores)
}

// ScheduleVM decides which node to a ProxmoxMachine should be scheduled on.
// It requires the machine's ProxmoxCluster to have at least 1 allowed node.
func ScheduleVM(ctx context.Context, machineScope *scope.MachineScope) (string, error) {
	client := machineScope.InfraCluster.ProxmoxClient
	allowedNodes := machineScope.InfraCluster.ProxmoxCluster.Spec.AllowedNodes
	schedulerHints := machineScope.InfraCluster.ProxmoxCluster.Spec.SchedulerHints
	locations := machineScope.InfraCluster.ProxmoxCluster.Status.NodeLocations.Workers
	if util.IsControlPlaneMachine(machineScope.Machine) {
		locations = machineScope.InfraCluster.ProxmoxCluster.Status.NodeLocations.ControlPlane
	}

	return selectNode(ctx, client, machineScope.ProxmoxMachine, locations, allowedNodes, schedulerHints)
}

func selectNode(
	ctx context.Context,
	client resourceClient,
	machine *infrav1.ProxmoxMachine,
	locations []infrav1.NodeLocation,
	allowedNodes []string,
	schedulerHints *infrav1.SchedulerHints,
) (string, error) {
	var nodes []nodeInfo

	requestedMemory := uint64(machine.Spec.MemoryMiB) * 1024 * 1024 // convert to bytes
	requestedCores := uint64(machine.Spec.NumCores)

	for _, nodeName := range allowedNodes {
		mem, cpu, err := client.GetReservableResources(
			ctx,
			nodeName,
			schedulerHints.GetMemoryAdjustment(),
			schedulerHints.GetCPUAdjustment(),
		)
		if err != nil {
			return "", err
		}

		// if MemoryAdjustment is explicitly set to 0 (zero), pretend we have enough mem for the guest
		if schedulerHints.GetMemoryAdjustment() == 0 {
			mem = requestedMemory
		}
		// if CPUAdjustment is explicitly set to 0 (zero), pretend we have enough cpu for the guest
		if schedulerHints.GetCPUAdjustment() == 0 {
			cpu = requestedCores
		}

		node := nodeInfo{Name: nodeName, AvailableMemory: mem, AvailableCPU: cpu}
		if node.AvailableMemory >= requestedMemory && node.AvailableCPU >= requestedCores {
			nodes = append(nodes, node)
		}
	}

	if len(nodes) == 0 {
		return "", InsufficientResourcesError{requestedMemory, requestedCores}
	}

	// Sort nodes by free memory and then free CPU in descending order
	byResources := make(sortByResources, len(nodes))
	copy(byResources, nodes)
	sort.Sort(byResources)

	decision := byResources[0].Name

	// count the existing vms per node
	nodeCounter := make(map[string]int)
	for _, nl := range locations {
		nodeCounter[nl.Node]++
	}

	for i, info := range byResources {
		info.ScheduledVMs = nodeCounter[info.Name]
		byResources[i] = info
	}

	byReplicas := make(sortByReplicas, len(byResources))
	copy(byReplicas, byResources)

	sort.Sort(byReplicas)

	// if memory allocation allows it, pick the node with the least amount of guests
	if schedulerHints.PreferLowerGuestCount {
		decision = byReplicas[0].Name
	}

	if logger := logr.FromContextOrDiscard(ctx); logger.V(4).Enabled() {
		// only construct values when message should actually be logged
		logger.Info("Scheduler decision",
			"byReplicas", byReplicas.String(),
			"byResources", byResources.String(),
			"requestedMemory", requestedMemory,
			"requestedCores", requestedCores,
			"resultNode", decision,
			"schedulerHints", schedulerHints,
		)
	}

	return decision, nil
}

type resourceClient interface {
	GetReservableResources(context.Context, string, uint64, uint64) (uint64, uint64, error)
}

type nodeInfo struct {
	Name            string `json:"node"`
	AvailableMemory uint64 `json:"mem"`
	AvailableCPU    uint64 `json:"cpu"`
	ScheduledVMs    int    `json:"vms"`
}

type sortByReplicas []nodeInfo

func (a sortByReplicas) Len() int      { return len(a) }
func (a sortByReplicas) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByReplicas) Less(i, j int) bool {
	return a[i].ScheduledVMs < a[j].ScheduledVMs
}

func (a sortByReplicas) String() string {
	o, _ := json.Marshal(a)
	return string(o)
}

type sortByResources []nodeInfo

func (a sortByResources) Len() int      { return len(a) }
func (a sortByResources) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByResources) Less(i, j int) bool {
	// Compare by free memory and free CPU in descending order
	if a[i].AvailableMemory != a[j].AvailableMemory {
		return a[i].AvailableMemory > a[j].AvailableMemory
	}

	// If free memory is equal, sort by free CPU in descending order
	return a[i].AvailableCPU > a[j].AvailableCPU || (a[i].AvailableCPU == a[j].AvailableCPU && a[i].ScheduledVMs < a[j].ScheduledVMs)
}

func (a sortByResources) String() string {
	o, _ := json.Marshal(a)
	return string(o)
}
