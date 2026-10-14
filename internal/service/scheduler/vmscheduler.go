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
	"math"
	"sort"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api/util"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

const (
	// overbookDiscount is the per-unit score of headroom in the overcommit range,
	// relative to 1 point per unit in the physical range.
	overbookDiscount = 0.5
	// saturationExponent shapes the per-resource score as a power curve, so that
	// nodes closer to saturation score disproportionately less than empty nodes.
	saturationExponent = 2.0
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

// InsufficientCPUError is used when the scheduler cannot assign a VM to a node because it would
// exceed the node's CPU core limit.
type InsufficientCPUError struct {
	node      string
	available int
	requested int
}

func (err InsufficientCPUError) Error() string {
	return fmt.Sprintf("cannot reserve %d CPU cores on node %s: %d available cores left",
		err.requested, err.node, err.available)
}

// ScheduleVM decides which node to a ProxmoxMachine should be scheduled on.
// It requires the machine's ProxmoxCluster to have at least 1 allowed node.
func ScheduleVM(ctx context.Context, machineScope *scope.MachineScope) (string, error) {
	client := machineScope.InfraCluster.ProxmoxClient
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

	return selectNode(ctx, client, machineScope.ProxmoxMachine, locations, allowedNodes, schedulerHints)
}

// selectNode picks a node from the allowed list for a new VM. It always runs
// the same scoring algorithm: each candidate node gets a non-linear score
// based on remaining headroom after hypothetical placement, with the physical
// range worth more than the overcommit range and near-saturation penalized
// quadratically. Tolerance values are inverted into weights: higher tolerance
// for a resource means it contributes less to the decision (= the operator is
// fine saturating it). The node with the highest total score wins.
//
// Existing placements recorded in status.NodeLocations inflate the effective
// usage in the score calculation, so rapid-fire scheduling — Proxmox's
// resource API takes seconds to reflect just-cloned VMs — spreads across
// nodes instead of clumping on the first-chosen winner. The inflation only
// applies to the score, not to the hard-fit check, so already-up-to-date
// availability data is never double-counted.
//
// When cpuAdjustment is 0, CPU information is not fetched, CPU is not used
// as a hard-fit constraint, and CPU does not contribute to the score; the
// algorithm degrades to memory-only scoring with the same curve.
func selectNode(
	ctx context.Context,
	client resourceClient,
	machine *infrav1.ProxmoxMachine,
	locations []infrav1.NodeLocation,
	allowedNodes []string,
	schedulerHints *infrav1.SchedulerHints,
) (string, error) {
	memoryAdjustment := schedulerHints.GetMemoryAdjustment()
	cpuAdjustment := schedulerHints.GetCPUAdjustment()
	cpuEnabled := cpuAdjustment > 0

	nodes := make([]nodeInfo, len(allowedNodes))
	for i, nodeName := range allowedNodes {
		mem, allocMem, err := client.GetReservableMemoryBytes(ctx, nodeName, memoryAdjustment)
		if err != nil {
			return "", err
		}

		var cpus, allocCPUs int
		if cpuEnabled {
			cpus, allocCPUs, err = client.GetReservableCPUCores(ctx, nodeName, cpuAdjustment)
			if err != nil {
				return "", err
			}
		}

		nodes[i] = nodeInfo{
			Name:              nodeName,
			AvailableMemory:   mem,
			AllocatableMemory: allocMem,
			AvailableCPUs:     cpus,
			AllocatableCPUs:   allocCPUs,
		}
	}

	requestedMemory := uint64(ptr.Deref(machine.Spec.MemoryMiB, 0)) * 1024 * 1024 // convert to bytes
	requestedCPUs := ptr.Deref(machine.Spec.NumSockets, 0) * ptr.Deref(machine.Spec.NumCores, 0)

	memoryTolerance := schedulerHints.GetMemoryTolerance()
	cpuTolerance := schedulerHints.GetCPUTolerance()

	type scored struct {
		nodeInfo
		scoreMem   float64
		scoreCPU   float64
		scoreTotal float64
		pending    int
	}

	wMem := float64(100-memoryTolerance) / 100.0
	wCPU := float64(100-cpuTolerance) / 100.0

	// count the existing placements per node from NodeLocations
	pendingCount := make(map[string]int, len(locations))
	for _, nl := range locations {
		pendingCount[nl.Node]++
	}

	var candidates []scored
	for _, n := range nodes {
		// Hard-fit against the raw availability reported by Proxmox.
		// Pending inflation is applied only to the score below, not here,
		// so up-to-date availability data is never double-counted.
		if requestedMemory > n.AvailableMemory {
			continue
		}
		if cpuEnabled && requestedCPUs > int32(n.AvailableCPUs) {
			continue
		}

		pending := pendingCount[n.Name]
		pendingMem := uint64(pending) * requestedMemory

		physMem := float64(n.AllocatableMemory)
		if memoryAdjustment > 0 {
			physMem = float64(n.AllocatableMemory) * 100.0 / float64(memoryAdjustment)
		}
		usedMem := float64(n.AllocatableMemory-n.AvailableMemory+pendingMem) + float64(requestedMemory)
		scoreMem := resourceScore(physMem, float64(n.AllocatableMemory), usedMem)

		var scoreCPU float64
		if cpuEnabled {
			pendingCPU := int32(pending) * requestedCPUs
			physCPU := float64(n.AllocatableCPUs) * 100.0 / float64(cpuAdjustment)
			usedCPU := float64(int32(n.AllocatableCPUs-n.AvailableCPUs)+pendingCPU) + float64(requestedCPUs)
			scoreCPU = resourceScore(physCPU, float64(n.AllocatableCPUs), usedCPU)
		}

		candidates = append(candidates, scored{
			nodeInfo:   n,
			scoreMem:   scoreMem,
			scoreCPU:   scoreCPU,
			scoreTotal: wMem*scoreMem + wCPU*scoreCPU,
			pending:    pending,
		})
	}

	if len(candidates) == 0 {
		return "", buildInsufficientError(nodes, requestedMemory, requestedCPUs)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		// primary: curve-based score (higher wins); tie-break: raw memory
		// headroom (kicks in when many nodes saturate under pending inflation).
		if candidates[i].scoreTotal != candidates[j].scoreTotal {
			return candidates[i].scoreTotal > candidates[j].scoreTotal
		}
		return candidates[i].AvailableMemory > candidates[j].AvailableMemory
	})

	decision := candidates[0].Name

	if logger := logr.FromContextOrDiscard(ctx); logger.V(4).Enabled() {
		// only construct values when message should actually be logged
		type logEntry struct {
			Node       string  `json:"node"`
			Mem        uint64  `json:"availMem"`
			CPU        int     `json:"availCpu"`
			Pending    int     `json:"pending"`
			ScoreMem   float64 `json:"scoreMem"`
			ScoreCPU   float64 `json:"scoreCpu"`
			ScoreTotal float64 `json:"scoreTotal"`
		}
		entries := make([]logEntry, len(candidates))
		for i, c := range candidates {
			entries[i] = logEntry{
				Node:       c.Name,
				Mem:        c.AvailableMemory,
				CPU:        c.AvailableCPUs,
				Pending:    c.pending,
				ScoreMem:   c.scoreMem,
				ScoreCPU:   c.scoreCPU,
				ScoreTotal: c.scoreTotal,
			}
		}
		data, _ := json.Marshal(entries)
		logger.Info("Scheduler decision",
			"candidates", string(data),
			"requestedMemory", requestedMemory,
			"requestedCPUs", requestedCPUs,
			"memoryTolerance", memoryTolerance,
			"cpuTolerance", cpuTolerance,
			"resultNode", decision,
		)
	}

	return decision, nil
}

// resourceScore returns the non-linear headroom score for a single resource.
// A unit of free capacity in the physical range is worth 1 point; a unit in the
// overcommit range is worth overbookDiscount points. The raw ratio is normalised
// by the physical capacity and raised to saturationExponent so that nodes close
// to full are penalised disproportionately more than nodes with room to spare.
func resourceScore(phys, alloc, used float64) float64 {
	if phys <= 0 {
		return 0
	}
	freePhys := phys - used
	if freePhys < 0 {
		freePhys = 0
	}
	freeOver := alloc - math.Max(used, phys)
	if freeOver < 0 {
		freeOver = 0
	}
	raw := (freePhys + overbookDiscount*freeOver) / phys
	return math.Pow(raw, saturationExponent)
}

// buildInsufficientError returns the best-fitting error when no candidate fits
// the request: InsufficientMemoryError if no node has enough memory available,
// otherwise InsufficientCPUError pointing at the node with the largest CPU headroom.
func buildInsufficientError(nodes []nodeInfo, requestedMemory uint64, requestedCPUs int32) error {
	byMem := make([]nodeInfo, len(nodes))
	copy(byMem, nodes)
	sort.Sort(sortByAvailableMemory(byMem))
	if requestedMemory > byMem[0].AvailableMemory {
		return InsufficientMemoryError{
			node:      byMem[0].Name,
			available: byMem[0].AvailableMemory,
			requested: requestedMemory,
		}
	}
	bestCPU := nodes[0]
	for _, n := range nodes[1:] {
		if n.AvailableCPUs > bestCPU.AvailableCPUs {
			bestCPU = n
		}
	}
	return InsufficientCPUError{
		node:      bestCPU.Name,
		available: bestCPU.AvailableCPUs,
		requested: int(requestedCPUs),
	}
}

type resourceClient interface {
	GetReservableMemoryBytes(context.Context, string, int64) (available uint64, allocatable uint64, err error)
	GetReservableCPUCores(context.Context, string, int64) (available int, allocatable int, err error)
}

type nodeInfo struct {
	Name              string `json:"node"`
	AvailableMemory   uint64 `json:"mem"`
	AllocatableMemory uint64 `json:"allocMem"`
	AvailableCPUs     int    `json:"cpu"`
	AllocatableCPUs   int    `json:"allocCpu"`
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
