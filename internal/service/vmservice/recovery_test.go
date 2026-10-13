/*
Copyright 2026 IONOS Cloud.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	ipam "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
)

func TestReconcileAddressRecovery_RecoversStatus(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: new(true),
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

func TestReconcileAddressRecovery_AdoptsValidatedOwnerlessResolvedClaim(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: new(true),
		}},
	}
	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)
	claimName := ipam.IPAddressFormat(machineScope.Name(), infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix)
	var claim ipamv1.IPAddressClaim
	require.NoError(t, kubeClient.Get(context.Background(), client.ObjectKey{Name: claimName, Namespace: machineScope.Namespace()}, &claim))
	claim.OwnerReferences = nil
	require.NoError(t, kubeClient.Update(context.Background(), &claim))
	machineScope.SetVirtualMachine(newRunningVM())

	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)

	require.NoError(t, kubeClient.Get(context.Background(), client.ObjectKeyFromObject(&claim), &claim))
	require.Len(t, claim.OwnerReferences, 1)
	require.Equal(t, machineScope.Name(), claim.OwnerReferences[0].Name)
	require.Equal(t, machineScope.ProxmoxMachine.UID, claim.OwnerReferences[0].UID)
	require.Equal(t, []string{"10.10.10.10"}, machineScope.ProxmoxMachine.GetIPAddressesNet(infrav1.DefaultNetworkDevice).IPv4)
	require.Equal(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason,
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
}

func TestReconcileAddressRecovery_RejectsTerminatingClaim(t *testing.T) {
	for _, ownerless := range []bool{true, false} {
		name := "owned"
		if ownerless {
			name = "ownerless"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
			pool := corev1.TypedLocalObjectReference{
				APIGroup: new(ipamicv1.GroupVersion.String()),
				Kind:     "InClusterIPPool",
				Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
			}
			machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
				NetworkDevices: []infrav1.NetworkDevice{{Name: infrav1.DefaultNetworkDevice, DefaultIPv4: new(true)}},
			}
			createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &pool)
			key := client.ObjectKey{
				Name:      ipam.IPAddressFormat(machineScope.Name(), infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix),
				Namespace: machineScope.Namespace(),
			}
			var claim ipamv1.IPAddressClaim
			require.NoError(t, kubeClient.Get(ctx, key, &claim))
			claim.Finalizers = []string{"test.finalizer"}
			if ownerless {
				claim.OwnerReferences = nil
			}
			require.NoError(t, kubeClient.Update(ctx, &claim))
			require.NoError(t, kubeClient.Delete(ctx, &claim))
			require.NoError(t, kubeClient.Get(ctx, key, &claim))
			require.False(t, claim.DeletionTimestamp.IsZero())
			before := claim.DeepCopy()
			conditionBefore := *conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
			machineScope.SetVirtualMachine(newRunningVM())

			require.NoError(t, reconcileAddressRecovery(ctx, machineScope))
			require.Empty(t, machineScope.ProxmoxMachine.Status.IPAddresses)
			require.Empty(t, machineScope.ProxmoxMachine.Status.Addresses)
			require.NoError(t, kubeClient.Get(ctx, key, &claim))
			require.Equal(t, before, &claim)
			require.Equal(t, conditionBefore, *conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
		})
	}
}

func TestReconcileAddressRecovery_DoesNotAdoptOwnerlessPendingClaim(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: new(true),
		}},
	}
	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	ipClaimDef := ipam.IPClaimDef{
		PoolRef: defaultPool,
		Device:  infrav1.DefaultNetworkDevice,
		Annotations: map[string]string{
			infrav1.ProxmoxPoolOffsetAnnotation:     "0",
			infrav1.ProxmoxDefaultGatewayAnnotation: "true",
		},
	}
	require.NoError(t, machineScope.IPAMHelper.CreateIPAddressClaim(context.Background(), machineScope.ProxmoxMachine, ipClaimDef))
	claimName := ipam.IPAddressFormat(machineScope.Name(), infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix)
	var claim ipamv1.IPAddressClaim
	require.NoError(t, kubeClient.Get(context.Background(), client.ObjectKey{Name: claimName, Namespace: machineScope.Namespace()}, &claim))
	claim.OwnerReferences = nil
	require.NoError(t, kubeClient.Update(context.Background(), &claim))
	machineScope.SetVirtualMachine(newRunningVM())

	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)

	require.NoError(t, kubeClient.Get(context.Background(), client.ObjectKeyFromObject(&claim), &claim))
	require.Empty(t, claim.OwnerReferences)
	require.Empty(t, machineScope.ProxmoxMachine.GetIPAddresses())
	require.Equal(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason,
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
}

func TestReconcileAddressRecovery_DoesNotAdoptPendingClaimWithExistingAddress(t *testing.T) {
	ctx := context.Background()
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
	pool := corev1.TypedLocalObjectReference{
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     "InClusterIPPool",
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{Name: infrav1.DefaultNetworkDevice, DefaultIPv4: new(true)}},
	}
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &pool)
	key := client.ObjectKey{
		Name:      ipam.IPAddressFormat(machineScope.Name(), infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix),
		Namespace: machineScope.Namespace(),
	}
	var claim ipamv1.IPAddressClaim
	require.NoError(t, kubeClient.Get(ctx, key, &claim))
	claim.OwnerReferences = nil
	claim.Status.AddressRef.Name = ""
	require.NoError(t, kubeClient.Update(ctx, &claim))
	before := claim.DeepCopy()
	machineScope.SetVirtualMachine(newRunningVM())

	require.NoError(t, reconcileAddressRecovery(ctx, machineScope))
	require.NoError(t, kubeClient.Get(ctx, key, &claim))
	require.Equal(t, before, &claim)
	require.Empty(t, machineScope.ProxmoxMachine.Status.IPAddresses)
	require.Empty(t, machineScope.ProxmoxMachine.Status.Addresses)
}

func TestReconcileAddressRecovery_RepairsIndependentlyWithoutPublishingPartialStatus(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	extraPool := corev1.TypedLocalObjectReference{
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     "restored-extra-pool",
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: new(true),
			InterfaceConfig: infrav1.InterfaceConfig{
				IPPoolRef: []corev1.TypedLocalObjectReference{extraPool},
			},
		}},
	}
	createIPPools(t, kubeClient, machineScope)
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &defaultPool)

	defaultClaimName := ipam.IPAddressFormat(machineScope.Name(), infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix)
	var defaultClaim ipamv1.IPAddressClaim
	require.NoError(t, kubeClient.Get(context.Background(), client.ObjectKey{Name: defaultClaimName, Namespace: machineScope.Namespace()}, &defaultClaim))
	defaultClaim.OwnerReferences = nil
	require.NoError(t, kubeClient.Update(context.Background(), &defaultClaim))

	conflictingClaimName := ipam.IPAddressFormat(machineScope.Name(), infrav1.DefaultNetworkDevice, 1, infrav1.DefaultSuffix)
	conflictingClaim := &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      conflictingClaimName,
			Namespace: machineScope.Namespace(),
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: machineScope.Cluster.Name,
			},
			Annotations: map[string]string{
				infrav1.ProxmoxPoolOffsetAnnotation: "1",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       infrav1.ProxmoxMachineKind,
				Name:       machineScope.Name(),
				UID:        k8stypes.UID("stale-machine-uid"),
				Controller: new(true),
			}},
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: ipamicv1.GroupVersion.Group,
				Kind:     extraPool.Kind,
				Name:     extraPool.Name,
			},
		},
	}
	require.NoError(t, kubeClient.Create(context.Background(), conflictingClaim))
	machineScope.SetVirtualMachine(newRunningVM())

	err := reconcileAddressRecovery(context.Background(), machineScope)
	require.NoError(t, err)

	require.NoError(t, kubeClient.Get(context.Background(), client.ObjectKeyFromObject(&defaultClaim), &defaultClaim))
	require.Len(t, defaultClaim.OwnerReferences, 1)
	require.Equal(t, machineScope.ProxmoxMachine.UID, defaultClaim.OwnerReferences[0].UID)
	require.Empty(t, machineScope.ProxmoxMachine.GetIPAddresses())
	require.Empty(t, machineScope.ProxmoxMachine.Status.Addresses)
	require.Equal(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason,
		conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
}

func TestReconcileVM_RecoveryConflictDoesNotEnableAllocation(t *testing.T) {
	for _, conflictDevice := range []infrav1.NetName{"net0", "net1"} {
		t.Run(string(conflictDevice), func(t *testing.T) {
			ctx := context.Background()
			machineScope, proxmoxClient, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
			pool := corev1.TypedLocalObjectReference{
				APIGroup: new(ipamicv1.GroupVersion.String()),
				Kind:     "InClusterIPPool",
				Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
			}
			machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
				NetworkDevices: []infrav1.NetworkDevice{
					{Name: "net0", InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{pool}}},
					{Name: "net1", InterfaceConfig: infrav1.InterfaceConfig{IPPoolRef: []corev1.TypedLocalObjectReference{pool}}},
				},
			}
			createIPAddress(t, kubeClient, machineScope, conflictDevice, "10.10.10.10", 0, &pool)
			key := client.ObjectKey{
				Name:      ipam.IPAddressFormat(machineScope.Name(), conflictDevice, 0, infrav1.DefaultSuffix),
				Namespace: machineScope.Namespace(),
			}
			var claim ipamv1.IPAddressClaim
			require.NoError(t, kubeClient.Get(ctx, key, &claim))
			claim.OwnerReferences[0].UID = k8stypes.UID("stale-machine-uid")
			require.NoError(t, kubeClient.Update(ctx, &claim))
			claimBefore := claim.DeepCopy()
			conditionBefore := *conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
			vm := newRunningVM()
			machineScope.SetVirtualMachineID(int64(vm.VMID))
			proxmoxClient.EXPECT().GetVM(ctx, "node1", int64(vm.VMID)).Return(vm, nil).Twice()

			// Exercise downstream provisioning and a subsequent reconciliation, not
			// just the recovery helper: neither may create the other device's claim.
			for range 2 {
				_, err := ReconcileVM(ctx, machineScope)
				require.NoError(t, err)
				var claims ipamv1.IPAddressClaimList
				require.NoError(t, kubeClient.List(ctx, &claims))
				require.Len(t, claims.Items, 1)
				require.NoError(t, kubeClient.Get(ctx, key, &claim))
				require.Equal(t, claimBefore, &claim)
				require.Equal(t, conditionBefore, *conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
				require.Empty(t, machineScope.ProxmoxMachine.Status.IPAddresses)
				require.Empty(t, machineScope.ProxmoxMachine.Status.Addresses)
			}
		})
	}
}

func TestReconcileAddressRecovery_NoopWhenVMNotRunning(t *testing.T) {
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)

	defaultPool := corev1.TypedLocalObjectReference{
		APIGroup: new(ipamicv1.GroupVersion.String()),
		Kind:     reflect.ValueOf(ipamicv1.InClusterIPPool{}).Type().Name(),
		Name:     getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{
			Name:        infrav1.DefaultNetworkDevice,
			DefaultIPv4: new(true),
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
			DefaultIPv4: new(true),
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
			DefaultIPv4: new(true),
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
			DefaultIPv4: new(true),
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
