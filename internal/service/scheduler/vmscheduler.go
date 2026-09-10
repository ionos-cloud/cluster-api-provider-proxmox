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

// Package scheduler implements scheduling algorithms for Proxmox VMs.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/util"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
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
	// Use the default allowed nodes from the ProxmoxCluster.
	allowedNodes := machineScope.InfraCluster.ProxmoxCluster.Spec.AllowedNodes
	schedulerHints := machineScope.InfraCluster.ProxmoxCluster.Spec.SchedulerHints
	locations := machineScope.InfraCluster.ProxmoxCluster.Status.NodeLocations.Workers
	if util.IsControlPlaneMachine(machineScope.Machine) {
		locations = machineScope.InfraCluster.ProxmoxCluster.Status.NodeLocations.ControlPlane
	}

	// If ProxmoxMachine defines allowedNodes use them instead
	if len(machineScope.ProxmoxMachine.Spec.AllowedNodes) > 0 {
		allowedNodes = machineScope.ProxmoxMachine.Spec.AllowedNodes
	}

	// Restrict to the AZ nodes when CAPI has assigned a failure domain.
	if fd := machineScope.Machine.Spec.FailureDomain; fd != "" {
		azNodes := nodesForAvailabilityZone(machineScope.InfraCluster.ProxmoxCluster.Spec.AvailabilityZones, fd)
		allowedNodes = intersectNodes(allowedNodes, azNodes)
	}

	// Map every candidate node to its availability zone (nodes without a zone are treated as
	// their own single-node zone) so round-robin balancing happens per-AZ instead of per-node.
	// Without this, an AZ made of several Proxmox nodes could receive multiple replacement
	// workers during a rolling upgrade while other AZs get none, since load looked "spread out"
	// at the node level even though it wasn't spread across zones.
	nodeZone := nodeToZone(machineScope.InfraCluster.ProxmoxCluster.Spec.AvailabilityZones, allowedNodes)

	return selectNode(ctx, machineScope.InfraCluster.ProxmoxClient, machineScope.ProxmoxMachine, locations, allowedNodes, nodeZone, schedulerHints)
}

// nodeToZone maps each node to the name of the availability zone it belongs to. Nodes that
// aren't part of any configured availability zone are mapped to themselves, so they're treated
// as an independent single-node zone for balancing purposes.
func nodeToZone(azs []infrav1.AvailabilityZoneSpec, nodes []string) map[string]string {
	result := make(map[string]string, len(nodes))
	for _, n := range nodes {
		result[n] = n
	}
	for _, az := range azs {
		for _, n := range az.Nodes {
			if _, ok := result[n]; ok {
				result[n] = az.Name
			}
		}
	}
	return result
}

// nodesForAvailabilityZone returns the nodes listed in the availability zone with the
// given name, or nil if no such zone is configured.
func nodesForAvailabilityZone(azs []infrav1.AvailabilityZoneSpec, name string) []string {
	for _, az := range azs {
		if az.Name == name {
			return az.Nodes
		}
	}
	return nil
}

// intersectNodes returns the nodes present in both a and b, preserving the order of a.
func intersectNodes(a, b []string) []string {
	bset := make(map[string]struct{}, len(b))
	for _, n := range b {
		bset[n] = struct{}{}
	}
	var result []string
	for _, n := range a {
		if _, ok := bset[n]; ok {
			result = append(result, n)
		}
	}
	return result
}

func selectNode(
	ctx context.Context,
	client resourceClient,
	machine *infrav1.ProxmoxMachine,
	locations []infrav1.NodeLocation,
	allowedNodes []string,
	nodeZone map[string]string,
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

	requestedMemory := uint64(ptr.Deref(machine.Spec.MemoryMiB, 0)) * 1024 * 1024 // convert to bytes
	if requestedMemory > byMemory[0].AvailableMemory {
		// no more space on the node with the highest amount of available memory
		return "", InsufficientMemoryError{
			node:      byMemory[0].Name,
			available: byMemory[0].AvailableMemory,
			requested: requestedMemory,
		}
	}

	// count the existing vms per availability zone, so nodes belonging to an already loaded
	// zone are deprioritized as a group rather than individually.
	zoneCounter := make(map[string]int)
	for _, nl := range locations {
		zoneCounter[zoneOf(nodeZone, nl.Node)]++
	}

	for i, info := range byMemory {
		info.ScheduledVMs = zoneCounter[zoneOf(nodeZone, info.Name)]
		byMemory[i] = info
	}

	byReplicas := make(sortByReplicas, len(byMemory))
	copy(byReplicas, byMemory)

	sort.Sort(byReplicas)

	decision := byMemory[0].Name
	for _, info := range byReplicas {
		// distribute round-robin when memory allows it
		if requestedMemory < info.AvailableMemory {
			decision = info.Name
			break
		}
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

// zoneOf returns the zone a node belongs to, falling back to the node name itself when it has
// no entry in nodeZone (e.g. no availability zones are configured).
func zoneOf(nodeZone map[string]string, node string) string {
	if zone, ok := nodeZone[node]; ok {
		return zone
	}
	return node
}

type resourceClient interface {
	GetReservableMemoryBytes(context.Context, string, int64) (uint64, error)
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
