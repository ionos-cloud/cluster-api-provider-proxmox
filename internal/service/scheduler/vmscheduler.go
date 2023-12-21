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

// InsufficientMemoryError is used when the scheduler cannot assign a VM to a node because it would
// exceed the node's memory limit.
type InsufficientMemoryError struct {
	node      string
	available uint64
	requested uint64
}

func (err InsufficientMemoryError) Error() string {
	return fmt.Sprintf("cannot reserve %dB of memory on node %s: %dB available memory left",
		err.requested, err.node, err.available)
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
	byMemory := make(sortByAvailableMemory, len(allowedNodes))
	for i, nodeName := range allowedNodes {
		mem, err := client.GetReservableMemoryBytes(ctx, nodeName, schedulerHints.GetMemoryAdjustment())
		if err != nil {
			return "", err
		}
		byMemory[i] = nodeInfo{Name: nodeName, AvailableMemory: mem}
	}

	sort.Sort(byMemory)

	requestedMemory := uint64(machine.Spec.MemoryMiB) * 1024 * 1024 // convert to bytes
	if requestedMemory > byMemory[0].AvailableMemory {
		// no more space on the node with the highest amount of available memory
		return "", InsufficientMemoryError{
			node:      byMemory[0].Name,
			available: byMemory[0].AvailableMemory,
			requested: requestedMemory,
		}
	}

	// count the existing vms per node
	nodeCounter := make(map[string]int)
	for _, nl := range locations {
		nodeCounter[nl.Node]++
	}

	for i, info := range byMemory {
		info.ScheduledVMs = nodeCounter[info.Name]
		byMemory[i] = info
	}

	byReplicas := make(sortByReplicas, len(byMemory))
	copy(byReplicas, byMemory)

	sort.Sort(byReplicas)

	decision := byMemory[0].Name
	if requestedMemory < byReplicas[0].AvailableMemory {
		// distribute round-robin when memory allows it
		decision = byReplicas[0].Name
	}

	if logger := logr.FromContextOrDiscard(ctx); logger.V(4).Enabled() {
		// only construct values when message should actually be logged
		logger.Info("Scheduler decision",
			"byReplicas", byReplicas.String(),
			"byMemory", byMemory.String(),
			"requestedMemory", requestedMemory,
			"resultNode", decision,
		)
	}

	return decision, nil
}

type resourceClient interface {
	GetReservableMemoryBytes(context.Context, string, uint64) (uint64, error)
}

type nodeInfo struct {
	Name            string `json:"node"`
	AvailableMemory uint64 `json:"mem"`
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

type sortByAvailableMemory []nodeInfo

func (a sortByAvailableMemory) Len() int      { return len(a) }
func (a sortByAvailableMemory) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByAvailableMemory) Less(i, j int) bool {
	// more available memory = lower index
	return a[i].AvailableMemory > a[j].AvailableMemory
}

func (a sortByAvailableMemory) String() string {
	o, _ := json.Marshal(a)
	return string(o)
}
