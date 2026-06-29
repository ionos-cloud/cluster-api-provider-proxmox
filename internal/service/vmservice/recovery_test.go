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
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

func TestReconcileAddressRecovery_RecoversStatus(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: ptr.To(true),
		}},
	}

	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)

	// already-running VM with empty status, e.g. restored from backup
	machineScope.SetVirtualMachine(newRunningVM())
	require.Empty(t, machineScope.ProxmoxMachine.GetIPAddresses())

	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)

	// status.ipAddresses republished
	require.NotNil(t, machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice))
	require.Equal(t,
		infrav1.IPAddressesSpec{NetName: string(infrav1.DefaultNetworkDevice), IPv4: []string{"10.10.10.10"}, IPv6: nil},
		*machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice),
	)

	// status.addresses republished (hostname + internal IP)
	require.Contains(t, machineScope.ProxmoxMachine.Status.Addresses, clusterv1.MachineAddress{
		Type:    clusterv1.MachineInternalIP,
		Address: "10.10.10.10",
	})

	// recovery must not create any IPAM objects: the single pre-created claim remains
	claims := getIPAddressClaimsPerPool(t, kubeClient, machineScope, defaultPool.Name)
	require.NotNil(t, claims)
	require.Len(t, *claims, 1)

	// recovery must not advance/alter the provisioning state machine
	require.Equal(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason,
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
}

func TestReconcileAddressRecovery_NoopWhenVMNotRunning(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: ptr.To(true),
		}},
	}
	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)

	machineScope.SetVirtualMachine(newStoppedVM())

	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)
	require.Empty(t, machineScope.ProxmoxMachine.GetIPAddresses())
	require.Empty(t, machineScope.ProxmoxMachine.Status.Addresses)
}

func TestReconcileAddressRecovery_NoopWhenVMNil(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: ptr.To(true),
		}},
	}
	createIPPools(t, kubeClient, machineScope)

	// VirtualMachine intentionally not set on the scope
	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)
	require.Empty(t, machineScope.ProxmoxMachine.GetIPAddresses())
}

func TestReconcileAddressRecovery_NoopWhenStatusAlreadyPopulated(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: ptr.To(true),
		}},
	}
	existing := infrav1.IPAddressesSpec{NetName: string(infrav1.DefaultNetworkDevice), IPv4: []string{"10.20.30.40"}}
	machineScope.ProxmoxMachine.SetIPAddresses(existing)

	machineScope.SetVirtualMachine(newRunningVM())

	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)

	// unchanged (idempotent no-op)
	require.Equal(t, existing, *machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice))
}

func TestReconcileAddressRecovery_NoopWhenNetworkNil(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	machineScope.ProxmoxMachine.Spec.Network = nil
	machineScope.SetVirtualMachine(newRunningVM())

	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)
	require.Empty(t, machineScope.ProxmoxMachine.GetIPAddresses())
}

func TestReconcileAddressRecovery_SkipsWhenClaimMissing(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: ptr.To(true),
		}},
	}
	createIPPools(t, kubeClient, machineScope)
	// no IPAddressClaim / IPAddress created -> claim resolves as missing

	machineScope.SetVirtualMachine(newRunningVM())

	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)

	// no status written and no IPAM object created
	require.Empty(t, machineScope.ProxmoxMachine.GetIPAddresses())
	require.Empty(t, machineScope.ProxmoxMachine.Status.Addresses)
	require.Empty(t, getIPAddressClaims(t, kubeClient, machineScope))
}
