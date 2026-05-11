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

package scheduler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

type fakeResourceClient struct {
	memory      map[string]uint64
	memoryTotal map[string]uint64
	cpus        map[string]int
	cpusTotal   map[string]int
}

func (c fakeResourceClient) GetReservableMemoryBytes(_ context.Context, nodeName string, _ int64) (uint64, uint64, error) {
	total := c.memoryTotal[nodeName]
	if total == 0 {
		total = c.memory[nodeName] // fallback: treat available as total for legacy tests
	}
	return c.memory[nodeName], total, nil
}

func (c fakeResourceClient) GetReservableCPUCores(_ context.Context, nodeName string, _ int64) (int, int, error) {
	total := c.cpusTotal[nodeName]
	if total == 0 {
		total = c.cpus[nodeName]
	}
	return c.cpus[nodeName], total, nil
}

// errorResourceClient always returns the given error for the specified method.
type errorResourceClient struct {
	fakeResourceClient
	memErr error
	cpuErr error
}

func (c errorResourceClient) GetReservableMemoryBytes(ctx context.Context, nodeName string, adj int64) (uint64, uint64, error) {
	if c.memErr != nil {
		return 0, 0, c.memErr
	}
	return c.fakeResourceClient.GetReservableMemoryBytes(ctx, nodeName, adj)
}

func (c errorResourceClient) GetReservableCPUCores(ctx context.Context, nodeName string, adj int64) (int, int, error) {
	if c.cpuErr != nil {
		return 0, 0, c.cpuErr
	}
	return c.fakeResourceClient.GetReservableCPUCores(ctx, nodeName, adj)
}

func miBytes(in int32) uint64 {
	return uint64(in) * 1024 * 1024
}

func TestSelectNode(t *testing.T) {
	allowedNodes := []string{"pve1", "pve2", "pve3"}
	var locations []infrav1.NodeLocation
	var requestMiB = int32(8)
	availableMem := map[string]uint64{
		"pve1": miBytes(20),
		"pve2": miBytes(30),
		"pve3": miBytes(15),
	}

	expectedNodes := []string{
		// initial pass: pick by memory headroom score
		"pve2", "pve1", "pve3",
		// second pass: pve3 out of memory; pending inflation drives the spread
		"pve2", "pve2", "pve1",
	}

	for i, expectedNode := range expectedNodes {
		t.Run(fmt.Sprintf("round %d", i+1), func(t *testing.T) {
			proxmoxMachine := &infrav1.ProxmoxMachine{
				Spec: infrav1.ProxmoxMachineSpec{
					MemoryMiB: &requestMiB,
				},
			}

			cl := fakeResourceClient{memory: availableMem}

			node, err := selectNode(context.Background(), cl, proxmoxMachine, locations, allowedNodes, &infrav1.SchedulerHints{})
			require.NoError(t, err)
			require.Equal(t, expectedNode, node)

			require.Greater(t, availableMem[node], miBytes(requestMiB))
			availableMem[node] -= miBytes(requestMiB)

			locations = append(locations, infrav1.NodeLocation{Node: node})
		})
	}

	t.Run("out of memory", func(t *testing.T) {
		proxmoxMachine := &infrav1.ProxmoxMachine{
			Spec: infrav1.ProxmoxMachineSpec{
				MemoryMiB: &requestMiB,
			},
		}

		cl := fakeResourceClient{memory: availableMem}

		node, err := selectNode(context.Background(), cl, proxmoxMachine, locations, allowedNodes, &infrav1.SchedulerHints{})
		require.ErrorAs(t, err, &InsufficientMemoryError{})
		require.Empty(t, node)

		expectMem := map[string]uint64{
			"pve1": miBytes(4), // 20 - 8 x 2
			"pve2": miBytes(6), // 30 - 8 x 3
			"pve3": miBytes(7), // 15 - 8 x 1
		}
		require.Equal(t, expectMem, availableMem)
	})
}

func TestSelectNodeEvenlySpread(t *testing.T) {
	// Verify that VMs are scheduled evenly across nodes when memory allows
	allowedNodes := []string{"pve1", "pve2", "pve3"}
	var locations []infrav1.NodeLocation
	var requestMiB = int32(8)
	availableMem := map[string]uint64{
		"pve1": miBytes(25), // enough for 3 VMs
		"pve2": miBytes(35), // enough for 4 VMs
		"pve3": miBytes(15), // enough for 1 VM
	}

	expectedNodes := []string{
		// initial pass: pick by memory headroom score
		"pve2", "pve1", "pve3",
		// second pass: pve3 out of memory; pending inflation spreads across pve1/pve2
		"pve2", "pve1", "pve2",
		// third pass: pve1 and pve2 have room for one more VM each
		"pve2", "pve1",
	}

	for i, expectedNode := range expectedNodes {
		t.Run(fmt.Sprintf("round %d", i+1), func(t *testing.T) {
			proxmoxMachine := &infrav1.ProxmoxMachine{
				Spec: infrav1.ProxmoxMachineSpec{
					MemoryMiB: &requestMiB,
				},
			}

			cl := fakeResourceClient{memory: availableMem}

			node, err := selectNode(context.Background(), cl, proxmoxMachine, locations, allowedNodes, &infrav1.SchedulerHints{})
			require.NoError(t, err)
			require.Equal(t, expectedNode, node)

			require.Greater(t, availableMem[node], miBytes(requestMiB))
			availableMem[node] -= miBytes(requestMiB)

			locations = append(locations, infrav1.NodeLocation{Node: node})
		})
	}

	t.Run("out of memory", func(t *testing.T) {
		proxmoxMachine := &infrav1.ProxmoxMachine{
			Spec: infrav1.ProxmoxMachineSpec{
				MemoryMiB: &requestMiB,
			},
		}

		cl := fakeResourceClient{memory: availableMem}

		node, err := selectNode(context.Background(), cl, proxmoxMachine, locations, allowedNodes, &infrav1.SchedulerHints{})
		require.ErrorAs(t, err, &InsufficientMemoryError{})
		require.Empty(t, node)

		expectMem := map[string]uint64{
			"pve1": miBytes(1), // 25 - 8 x 3
			"pve2": miBytes(3), // 35 - 8 x 4
			"pve3": miBytes(7), // 15 - 8 x 1
		}
		require.Equal(t, expectMem, availableMem)
	})
}

func TestScheduleVM(t *testing.T) {
	ctrlClient := setupClient()
	require.NotNil(t, ctrlClient)

	ipamHelper := &ipam.Helper{}

	proxmoxCluster := infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
		Spec: infrav1.ProxmoxClusterSpec{
			AllowedNodes: []string{"pve1", "pve2", "pve3"},
		},
		Status: infrav1.ProxmoxClusterStatus{
			NodeLocations: &infrav1.NodeLocations{
				ControlPlane: []infrav1.NodeLocation{},
				Workers: []infrav1.NodeLocation{
					{
						Node: "pve1",
						Machine: corev1.LocalObjectReference{
							Name: "foo-machine",
						},
					},
				},
			},
		},
	}

	err := ctrlClient.Create(context.Background(), &proxmoxCluster)
	require.NoError(t, err)

	proxmoxMachine := &infrav1.ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo-machine",
			Labels: map[string]string{
				"cluster.x-k8s.io/cluster-name": "bar",
			},
		},
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB: ptr.To(int32(10)),
		},
	}

	fakeProxmoxClient := proxmoxtest.NewMockClient(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "default",
		},
	}
	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Client: ctrlClient,
		Machine: &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-machine",
				Namespace: "default",
			},
		},
		Cluster: cluster,
		InfraCluster: &scope.ClusterScope{
			Cluster:        cluster,
			ProxmoxCluster: &proxmoxCluster,
			ProxmoxClient:  fakeProxmoxClient,
		},
		ProxmoxMachine: proxmoxMachine,
		IPAMHelper:     ipamHelper,
	})
	require.NoError(t, err)

	fakeProxmoxClient.EXPECT().GetReservableMemoryBytes(context.Background(), "pve1", int64(100)).Return(miBytes(60), miBytes(100), nil)
	fakeProxmoxClient.EXPECT().GetReservableMemoryBytes(context.Background(), "pve2", int64(100)).Return(miBytes(20), miBytes(100), nil)
	fakeProxmoxClient.EXPECT().GetReservableMemoryBytes(context.Background(), "pve3", int64(100)).Return(miBytes(20), miBytes(100), nil)

	node, err := ScheduleVM(context.Background(), machineScope)
	require.NoError(t, err)
	require.Equal(t, "pve1", node)
}

func TestToleranceScoringCPUDisabled(t *testing.T) {
	// When cpuAdjustment=0 CPU is not fetched and does not influence scoring;
	// the algorithm degrades to memory-only curve scoring regardless of tolerance hints.
	allowedNodes := []string{"pve1", "pve2", "pve3"}
	availableMem := map[string]uint64{
		"pve1": miBytes(20),
		"pve2": miBytes(30),
		"pve3": miBytes(15),
	}

	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(8)),
			NumSockets: ptr.To(int32(2)),
			NumCores:   ptr.To(int32(4)),
		},
	}

	cl := fakeResourceClient{memory: availableMem}

	hints := &infrav1.SchedulerHints{
		CPUAdjustment:   ptr.To(int64(0)),
		CPUTolerance:    ptr.To(int64(0)),
		MemoryTolerance: ptr.To(int64(100)),
	}
	node, err := selectNode(context.Background(), cl, proxmoxMachine, nil, allowedNodes, hints)
	require.NoError(t, err)
	require.Equal(t, "pve2", node)
}

func TestToleranceScoringDefaultsFollowMemory(t *testing.T) {
	// Default tolerance: memoryTolerance=0, cpuTolerance=100 -> CPU weight is zero,
	// memory drives the decision. Two nodes with identical CPU headroom but different
	// memory headroom must resolve to the node with more memory.
	allowedNodes := []string{"pve1", "pve2"}

	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(8)),
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(4)),
		},
	}

	cl := fakeResourceClient{
		memory:      map[string]uint64{"pve1": miBytes(350), "pve2": miBytes(200)},
		memoryTotal: map[string]uint64{"pve1": miBytes(400), "pve2": miBytes(400)},
		cpus:        map[string]int{"pve1": 50, "pve2": 50},
		cpusTotal:   map[string]int{"pve1": 64, "pve2": 64},
	}

	hints := &infrav1.SchedulerHints{CPUAdjustment: ptr.To(int64(300))}
	node, err := selectNode(context.Background(), cl, proxmoxMachine, nil, allowedNodes, hints)
	require.NoError(t, err)
	require.Equal(t, "pve1", node)
}

func TestToleranceScoringCPUTolerancePicksCPUHeadroom(t *testing.T) {
	// Opposite polarity: memoryTolerance=100 (weight 0), cpuTolerance=0 (weight 1).
	// Two nodes with identical memory headroom but different CPU headroom must resolve
	// to the node with more CPU headroom.
	allowedNodes := []string{"pve1", "pve2"}

	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(8)),
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(4)),
		},
	}

	cl := fakeResourceClient{
		memory:      map[string]uint64{"pve1": miBytes(200), "pve2": miBytes(200)},
		memoryTotal: map[string]uint64{"pve1": miBytes(400), "pve2": miBytes(400)},
		cpus:        map[string]int{"pve1": 80, "pve2": 40},
		cpusTotal:   map[string]int{"pve1": 96, "pve2": 96},
	}

	hints := &infrav1.SchedulerHints{
		CPUAdjustment:   ptr.To(int64(300)),
		MemoryTolerance: ptr.To(int64(100)),
		CPUTolerance:    ptr.To(int64(0)),
	}
	node, err := selectNode(context.Background(), cl, proxmoxMachine, nil, allowedNodes, hints)
	require.NoError(t, err)
	require.Equal(t, "pve1", node)
}

func TestToleranceScoringPenalizesOverbook(t *testing.T) {
	// Two nodes with the same physical capacity (32 cores, 200MiB) and the same
	// cpuAdjustment=200. pve1 has usage well inside the physical range; pve2 has
	// already crossed into the overcommit range. With cpuTolerance=0, pve1 must win
	// because headroom in the overcommit range contributes less to the score and
	// the non-linear curve heavily penalizes saturated hosts.
	allowedNodes := []string{"pve1", "pve2"}

	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(8)),
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(2)),
		},
	}

	// cpuAdjustment=200 => allocatableCPUs = phys * 2 = 64. Mem mirrors the pattern.
	// pve1: used_cpu=10 (phys range); pve2: used_cpu=40 (already past 32 physical cores).
	cl := fakeResourceClient{
		memory:      map[string]uint64{"pve1": miBytes(150), "pve2": miBytes(120)},
		memoryTotal: map[string]uint64{"pve1": miBytes(400), "pve2": miBytes(400)},
		cpus:        map[string]int{"pve1": 54, "pve2": 24},
		cpusTotal:   map[string]int{"pve1": 64, "pve2": 64},
	}

	hints := &infrav1.SchedulerHints{
		CPUAdjustment:   ptr.To(int64(200)),
		MemoryTolerance: ptr.To(int64(100)),
		CPUTolerance:    ptr.To(int64(0)),
	}
	node, err := selectNode(context.Background(), cl, proxmoxMachine, nil, allowedNodes, hints)
	require.NoError(t, err)
	require.Equal(t, "pve1", node)
}

func TestToleranceScoringHardConstraintCPU(t *testing.T) {
	// Request exceeds the CPU allocatable on every node -> InsufficientCPUError.
	allowedNodes := []string{"pve1", "pve2"}

	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(8)),
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(8)),
		},
	}

	cl := fakeResourceClient{
		memory:      map[string]uint64{"pve1": miBytes(500), "pve2": miBytes(500)},
		memoryTotal: map[string]uint64{"pve1": miBytes(500), "pve2": miBytes(500)},
		cpus:        map[string]int{"pve1": 2, "pve2": 4},
		cpusTotal:   map[string]int{"pve1": 16, "pve2": 16},
	}

	hints := &infrav1.SchedulerHints{CPUAdjustment: ptr.To(int64(100))}
	node, err := selectNode(context.Background(), cl, proxmoxMachine, nil, allowedNodes, hints)
	require.ErrorAs(t, err, &InsufficientCPUError{})
	require.Empty(t, node)
}

func TestToleranceScoringHardConstraintMemory(t *testing.T) {
	// Request exceeds the memory available on every node -> InsufficientMemoryError.
	allowedNodes := []string{"pve1"}

	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(100)),
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(2)),
		},
	}

	cl := fakeResourceClient{
		memory:      map[string]uint64{"pve1": miBytes(50)},
		memoryTotal: map[string]uint64{"pve1": miBytes(500)},
		cpus:        map[string]int{"pve1": 64},
		cpusTotal:   map[string]int{"pve1": 64},
	}

	hints := &infrav1.SchedulerHints{CPUAdjustment: ptr.To(int64(100))}
	node, err := selectNode(context.Background(), cl, proxmoxMachine, nil, allowedNodes, hints)
	require.ErrorAs(t, err, &InsufficientMemoryError{})
	require.Empty(t, node)
}

func TestToleranceScoringHeterogeneousRegression(t *testing.T) {
	// Regression for issue #724: on a heterogeneous cluster (some nodes with 64 cores,
	// others with 32 cores) scheduling N VMs with balanced tolerance must keep the
	// per-node CPU allocation ratio spread contained, unlike the legacy memory-only scheduler.
	// We schedule 24 identical VMs (2 cores, 8MiB each) across 4 nodes and assert that
	// the max_ratio / min_ratio across nodes stays below 2.0.
	allowedNodes := []string{"big1", "big2", "small1", "small2"}

	// physical capacities; adjustment=300 => allocatable = phys * 3.
	physCPU := map[string]int{
		"big1": 64, "big2": 64, "small1": 32, "small2": 32,
	}
	physMem := map[string]uint64{
		"big1": miBytes(504 * 1024), "big2": miBytes(504 * 1024),
		"small1": miBytes(504 * 1024), "small2": miBytes(504 * 1024),
	}

	availCPU := map[string]int{}
	availMem := map[string]uint64{}
	cpuTotals := map[string]int{}
	memTotals := map[string]uint64{}
	for n, phys := range physCPU {
		cpuTotals[n] = phys * 3
		availCPU[n] = phys * 3
	}
	for n, phys := range physMem {
		memTotals[n] = phys
		availMem[n] = phys
	}

	usedCPU := map[string]int{"big1": 0, "big2": 0, "small1": 0, "small2": 0}

	hints := &infrav1.SchedulerHints{
		CPUAdjustment:   ptr.To(int64(300)),
		MemoryTolerance: ptr.To(int64(0)),
		CPUTolerance:    ptr.To(int64(50)),
	}

	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(8 * 1024)),
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(2)),
		},
	}

	var locations []infrav1.NodeLocation
	for i := 0; i < 24; i++ {
		cl := fakeResourceClient{
			memory: availMem, memoryTotal: memTotals,
			cpus: availCPU, cpusTotal: cpuTotals,
		}
		node, err := selectNode(context.Background(), cl, proxmoxMachine, locations, allowedNodes, hints)
		require.NoError(t, err, "round %d", i)
		availCPU[node] -= 2
		availMem[node] -= miBytes(8 * 1024)
		usedCPU[node] += 2
		locations = append(locations, infrav1.NodeLocation{Node: node})
	}

	minRatio, maxRatio := 1e9, 0.0
	for n, used := range usedCPU {
		r := float64(used) / float64(physCPU[n])
		if r < minRatio {
			minRatio = r
		}
		if r > maxRatio {
			maxRatio = r
		}
	}
	t.Logf("CPU allocation per node: %v (ratios min=%.3f max=%.3f spread=%.3fx)", usedCPU, minRatio, maxRatio, maxRatio/minRatio)
	// The legacy memory-only scheduler on this scenario hits ~4.4x spread (see issue #724).
	// The current tolerance-weighted implementation measures at ~1.43x; the bound
	// at 1.5x catches any regression in the scoring curve or pending inflation.
	require.Less(t, maxRatio/minRatio, 1.5,
		"CPU allocation ratio spread too wide: min=%.2f max=%.2f used=%v", minRatio, maxRatio, usedCPU)
}

func TestToleranceScoringAvoidsClumpingOnRapidScale(t *testing.T) {
	// Regression: when multiple VMs are scheduled in rapid succession, the Proxmox
	// resource API lags behind (just-cloned VMs aren't yet reflected in availMem/
	// availCpu). The scheduler must use status.NodeLocations to count placements
	// already committed and inflate the effective usage, so subsequent decisions
	// spread across nodes instead of clumping on the first-chosen winner.
	allowedNodes := []string{"pve1", "pve2", "pve3"}

	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(8 * 1024)),
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(2)),
		},
	}

	// Three identical nodes. A naive scheduler that ignores locations would pick
	// the same node every time because the fake client returns constant values.
	cl := fakeResourceClient{
		memory:      map[string]uint64{"pve1": miBytes(100 * 1024), "pve2": miBytes(100 * 1024), "pve3": miBytes(100 * 1024)},
		memoryTotal: map[string]uint64{"pve1": miBytes(128 * 1024), "pve2": miBytes(128 * 1024), "pve3": miBytes(128 * 1024)},
		cpus:        map[string]int{"pve1": 60, "pve2": 60, "pve3": 60},
		cpusTotal:   map[string]int{"pve1": 96, "pve2": 96, "pve3": 96},
	}

	hints := &infrav1.SchedulerHints{
		CPUAdjustment:   ptr.To(int64(300)),
		MemoryTolerance: ptr.To(int64(0)),
		CPUTolerance:    ptr.To(int64(50)),
	}

	placements := map[string]int{}
	var locations []infrav1.NodeLocation
	for i := 0; i < 6; i++ {
		node, err := selectNode(context.Background(), cl, proxmoxMachine, locations, allowedNodes, hints)
		require.NoError(t, err, "round %d", i)
		placements[node]++
		locations = append(locations, infrav1.NodeLocation{Node: node})
	}

	// With 6 VMs across 3 identical nodes we expect perfect spread: 2 each.
	for _, n := range allowedNodes {
		require.Equalf(t, 2, placements[n], "node %s got %d placements, expected even spread (all: %v)", n, placements[n], placements)
	}
}

func TestSelectNodeMemoryQueryError(t *testing.T) {
	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB: ptr.To(int32(8)),
		},
	}

	cl := errorResourceClient{
		fakeResourceClient: fakeResourceClient{
			memory: map[string]uint64{"pve1": miBytes(100)},
		},
		memErr: fmt.Errorf("connection refused"),
	}

	node, err := selectNode(context.Background(), cl, proxmoxMachine, nil, []string{"pve1"}, &infrav1.SchedulerHints{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection refused")
	require.Empty(t, node)
}

func TestSelectNodeCPUQueryError(t *testing.T) {
	proxmoxMachine := &infrav1.ProxmoxMachine{
		Spec: infrav1.ProxmoxMachineSpec{
			MemoryMiB:  ptr.To(int32(8)),
			NumSockets: ptr.To(int32(1)),
			NumCores:   ptr.To(int32(4)),
		},
	}

	cl := errorResourceClient{
		fakeResourceClient: fakeResourceClient{
			memory: map[string]uint64{"pve1": miBytes(100)},
			cpus:   map[string]int{"pve1": 32},
		},
		cpuErr: fmt.Errorf("node unavailable"),
	}

	hints := &infrav1.SchedulerHints{CPUAdjustment: ptr.To(int64(100))}
	node, err := selectNode(context.Background(), cl, proxmoxMachine, nil, []string{"pve1"}, hints)
	require.Error(t, err)
	require.Contains(t, err.Error(), "node unavailable")
	require.Empty(t, node)
}

func TestInsufficientMemoryError_Error(t *testing.T) {
	err := InsufficientMemoryError{
		node:      "pve1",
		available: 10,
		requested: 20,
	}
	require.Equal(t, "cannot reserve 20B of memory on node pve1: 10B available memory left", err.Error())
}

func TestInsufficientCPUError_Error(t *testing.T) {
	err := InsufficientCPUError{
		node:      "pve1",
		available: 2,
		requested: 8,
	}
	require.Equal(t, "cannot reserve 8 CPU cores on node pve1: 2 available cores left", err.Error())
}

func setupClient() client.Client {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	return fakeClient
}
