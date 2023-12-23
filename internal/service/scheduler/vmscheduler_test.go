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

package scheduler

import (
	"context"
	"fmt"
	"testing"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/stretchr/testify/require"
)

type fakeResourceClient map[string]nodeInfo

func (c fakeResourceClient) GetReservableResources(_ context.Context, nodeName string, _ uint64, _ uint64) (uint64, uint64, error) {
	return c[nodeName].AvailableMemory, c[nodeName].AvailableCPU, nil
}

func miBytes(in uint64) uint64 {
	return in * 1024 * 1024
}

func TestSelectNode(t *testing.T) {
	allowedNodes := []string{"pve1", "pve2", "pve3"}
	var locations []infrav1.NodeLocation
	const requestMiB = 8
	const requestCores = 2
	cpuAdjustment := uint64(100)

	schedulerHints := &infrav1.SchedulerHints{
		// This defaults to true in our CRD
		PreferLowerGuestCount: true,
		CPUAdjustment:         &cpuAdjustment,
	}
	availableResources := map[string]nodeInfo{
		"pve1": {AvailableMemory: miBytes(20), AvailableCPU: uint64(16)},
		"pve2": {AvailableMemory: miBytes(30), AvailableCPU: uint64(16)},
		"pve3": {AvailableMemory: miBytes(15), AvailableCPU: uint64(16)},
	}

	expectedNodes := []string{
		// initial round-robin: everyone has enough memory
		"pve2", "pve1", "pve3",
		// second round-robin: pve3 out of memory
		"pve2", "pve1", "pve2",
	}

	for i, expectedNode := range expectedNodes {
		t.Run(fmt.Sprintf("round %d", i+1), func(t *testing.T) {
			proxmoxMachine := &infrav1.ProxmoxMachine{
				Spec: infrav1.ProxmoxMachineSpec{
					MemoryMiB: requestMiB,
					NumCores:  requestCores,
				},
			}

			client := fakeResourceClient(availableResources)

			node, err := selectNode(context.Background(), client, proxmoxMachine, locations, allowedNodes, schedulerHints)
			require.NoError(t, err)
			require.Equal(t, expectedNode, node)

			require.Greater(t, availableResources[node].AvailableMemory, miBytes(requestMiB))
			if entry, ok := availableResources[node]; ok {
				entry.AvailableMemory -= miBytes(requestMiB)
				entry.AvailableCPU -= requestCores
				availableResources[node] = entry
			}

			locations = append(locations, infrav1.NodeLocation{Node: node})
		})
	}

	t.Run("out of resources", func(t *testing.T) {
		proxmoxMachine := &infrav1.ProxmoxMachine{
			Spec: infrav1.ProxmoxMachineSpec{
				MemoryMiB: requestMiB,
				NumCores:  requestCores,
			},
		}

		client := fakeResourceClient(availableResources)

		node, err := selectNode(context.Background(), client, proxmoxMachine, locations, allowedNodes, schedulerHints)
		require.ErrorAs(t, err, &InsufficientResourcesError{})
		require.Empty(t, node)

		expectResources := map[string]nodeInfo{
			"pve1": {AvailableMemory: miBytes(4), AvailableCPU: uint64(12)}, // 20 - 8 x 2
			"pve2": {AvailableMemory: miBytes(6), AvailableCPU: uint64(10)}, // 30 - 8 x 3
			"pve3": {AvailableMemory: miBytes(7), AvailableCPU: uint64(14)}, // 15 - 8 x 1
		}

		require.Equal(t, expectResources, availableResources)
	})
}
