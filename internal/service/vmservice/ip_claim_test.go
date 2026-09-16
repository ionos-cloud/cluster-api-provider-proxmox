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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
)

func TestIPAddressClaimConflictDiagnostics(t *testing.T) {
	machineScope, _, _ := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedCloningReason)
	claim := &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          map[string]string{clusterv1.ClusterNameLabel: "other-cluster"},
			Annotations:     map[string]string{infrav1.ProxmoxPoolOffsetAnnotation: "7"},
			OwnerReferences: []metav1.OwnerReference{{Name: "old-machine", UID: "old-machine-uid"}},
		},
		Spec:   ipamv1.IPAddressClaimSpec{PoolRef: ipamv1.IPPoolReference{APIGroup: "other.example", Kind: "OtherPool", Name: "other-pool"}},
		Status: ipamv1.IPAddressClaimStatus{AddressRef: ipamv1.IPAddressReference{Name: "other-address"}},
	}
	address := &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          map[string]string{clusterv1.ClusterNameLabel: "address-cluster"},
			OwnerReferences: []metav1.OwnerReference{{Name: "old-claim", UID: "old-claim-uid"}},
		},
		Spec: ipamv1.IPAddressSpec{
			ClaimRef: ipamv1.IPAddressClaimReference{Name: "other-claim"},
			PoolRef:  ipamv1.IPPoolReference{APIGroup: "address.example", Kind: "AddressPool", Name: "address-pool"},
		},
	}
	claimDef := ipam.IPClaimDef{
		PoolRef:     corev1.TypedLocalObjectReference{APIGroup: new("expected.example"), Kind: "ExpectedPool", Name: "expected-pool"},
		Device:      infrav1.DefaultNetworkDevice,
		Annotations: map[string]string{infrav1.ProxmoxPoolOffsetAnnotation: "0"},
	}
	tests := []struct {
		reason ipam.IPAddressClaimConflictReason
		want   []string
	}{
		{ipam.ConflictClaimDeleting, []string{"deletion is in progress", "allocation may be released", "wait for IPAM cleanup"}},
		{ipam.ConflictOwnerMismatch, []string{machineScope.Name(), string(machineScope.ProxmoxMachine.UID), "old-machine", "old-machine-uid"}},
		{ipam.ConflictPoolMismatch, []string{"expected.example/ExpectedPool", "expected-pool", "other.example/OtherPool", "other-pool"}},
		{ipam.ConflictClaimCluster, []string{machineScope.Cluster.Name, "other-cluster", "claim cluster label"}},
		{ipam.ConflictClaimAnnotations, []string{infrav1.ProxmoxPoolOffsetAnnotation + ":0", infrav1.ProxmoxPoolOffsetAnnotation + ":7"}},
		{ipam.ConflictAddressMissing, []string{"referenced IPAddress", "other-address", "to exist"}},
		{ipam.ConflictAddressRef, []string{"expected claim addressRef \"expected-claim\"", "actual value is \"other-address\""}},
		{ipam.ConflictAddressClaimRef, []string{"expected IPAddress claimRef \"expected-claim\"", "actual value is \"other-claim\""}},
		{ipam.ConflictAddressPoolRef, []string{"expected.example/ExpectedPool", "expected-pool", "address.example/AddressPool", "address-pool"}},
		{ipam.ConflictAddressCluster, []string{machineScope.Cluster.Name, "address-cluster", "IPAddress cluster label"}},
		{ipam.ConflictAddressOwner, []string{"IPAddress controller owner", "old-claim", "old-claim-uid"}},
		{"UnknownConflict", []string{"inspect the restored IPAddressClaim and IPAddress"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			resolution := ipam.IPAddressClaimResolution{
				ClaimName: "expected-claim", Claim: claim, ConflictingAddress: address,
				Status: ipam.ClaimConflict, ConflictReason: tt.reason,
			}
			before := *conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
			message := reportIPAddressClaimConflict(machineScope, resolution, claimDef)
			require.Equal(t, before, *conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition), "recovery diagnostics must not change provisioning state")
			require.Contains(t, message, "expected-claim")
			require.Contains(t, message, string(tt.reason))
			for _, fragment := range tt.want {
				require.Contains(t, message, fragment)
			}
		})
	}
}

func TestIPAddressClaimConflictDiagnosticsAddressFallbacks(t *testing.T) {
	machineScope, _, _ := setupReconcilerTest(t)
	address := func(claimName string) *ipamv1.IPAddress {
		return &ipamv1.IPAddress{Spec: ipamv1.IPAddressSpec{ClaimRef: ipamv1.IPAddressClaimReference{Name: claimName}}}
	}
	tests := []struct {
		name       string
		resolution ipam.IPAddressClaimResolution
		actual     string
	}{
		{"conflicting address takes precedence", ipam.IPAddressClaimResolution{ConflictingAddress: address("conflicting"), Address: address("resolved"), OrphanedAddress: address("orphaned")}, "conflicting"},
		{"resolved address", ipam.IPAddressClaimResolution{Address: address("resolved"), OrphanedAddress: address("orphaned")}, "resolved"},
		{"orphaned address", ipam.IPAddressClaimResolution{OrphanedAddress: address("orphaned")}, "orphaned"},
		{"no address available", ipam.IPAddressClaimResolution{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.resolution.ClaimName = "expected-claim"
			tt.resolution.ConflictReason = ipam.ConflictAddressClaimRef
			message := reportIPAddressClaimConflict(machineScope, tt.resolution, ipam.IPClaimDef{})
			require.Contains(t, message, "expected IPAddress claimRef \"expected-claim\"")
			require.Contains(t, message, "actual value is \""+tt.actual+"\"")
		})
	}
}

func TestIPAddressClaimAPIErrorsDoNotPublishOrMutateAllocations(t *testing.T) {
	tests := []struct {
		name       string
		recovery   bool
		failReadAt int
	}{
		{name: "provisioning/resolution read", failReadAt: 1},
		{name: "provisioning/adoption revalidation read", failReadAt: 2},
		{name: "provisioning/adoption update"},
		{name: "recovery/resolution read", recovery: true, failReadAt: 1},
		{name: "recovery/adoption revalidation read", recovery: true, failReadAt: 2},
		{name: "recovery/adoption update", recovery: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testIPAddressClaimAPIFailure(t, tt.recovery, tt.failReadAt)
		})
	}
}

func testIPAddressClaimAPIFailure(t *testing.T, recovery bool, failReadAt int) {
	t.Helper()
	ctx := context.Background()
	machineScope, _, kubeClient := setupReconcilerTestWithCondition(t, infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForStaticIPAllocationReason)
	pool := corev1.TypedLocalObjectReference{
		APIGroup: new(ipamicv1.GroupVersion.String()), Kind: "InClusterIPPool",
		Name: getDefaultPoolRefs(machineScope).InClusterIPPoolRefV4.Name,
	}
	machineScope.ProxmoxMachine.Spec.Network = &infrav1.NetworkSpec{
		NetworkDevices: []infrav1.NetworkDevice{{Name: infrav1.DefaultNetworkDevice, DefaultIPv4: new(true)}},
	}
	createIPAddress(t, kubeClient, machineScope, infrav1.DefaultNetworkDevice, "10.10.10.10", 0, &pool)
	key := client.ObjectKey{Namespace: machineScope.Namespace(), Name: ipam.IPAddressFormat(machineScope.Name(), infrav1.DefaultNetworkDevice, 0, infrav1.DefaultSuffix)}
	var claim ipamv1.IPAddressClaim
	require.NoError(t, kubeClient.Get(ctx, key, &claim))
	claim.OwnerReferences = nil
	require.NoError(t, kubeClient.Update(ctx, &claim))
	claimBefore := claim.DeepCopy()
	var address ipamv1.IPAddress
	require.NoError(t, kubeClient.Get(ctx, key, &address))
	addressBefore := address.DeepCopy()
	conditionBefore := *conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
	readErr := errors.New("injected API read failure")
	updateErr := apierrors.NewConflict(schema.GroupResource{Group: ipamv1.GroupVersion.Group, Resource: "ipaddressclaims"}, key.Name, errors.New("resource version changed"))
	failingClient := &claimFailureClient{
		Client: kubeClient, failReadAt: failReadAt, readErr: readErr, updateErr: updateErr,
	}
	machineScope.IPAMHelper = ipam.NewHelper(failingClient, machineScope.InfraCluster.ProxmoxCluster)
	var err error
	if recovery {
		machineScope.SetVirtualMachine(newRunningVM())
		err = reconcileAddressRecovery(ctx, machineScope)
	} else {
		_, err = reconcileIPAddresses(ctx, machineScope)
	}
	if failReadAt == 0 {
		require.ErrorIs(t, err, updateErr)
		require.True(t, apierrors.IsConflict(err))
		require.Equal(t, 1, failingClient.updates)
	} else {
		require.ErrorIs(t, err, readErr)
		require.Zero(t, failingClient.updates)
	}
	if failReadAt != 1 {
		require.Contains(t, err.Error(), "unable to adopt IPAddressClaim")
		require.Contains(t, err.Error(), key.Name)
	}
	require.Empty(t, machineScope.ProxmoxMachine.Status.IPAddresses)
	require.Empty(t, machineScope.ProxmoxMachine.Status.Addresses)
	require.Equal(t, conditionBefore, *conditions.Get(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition))
	require.NoError(t, kubeClient.Get(ctx, key, &claim))
	require.Equal(t, claimBefore, &claim)
	require.NoError(t, kubeClient.Get(ctx, key, &address))
	require.Equal(t, addressBefore, &address)
	var claims ipamv1.IPAddressClaimList
	require.NoError(t, kubeClient.List(ctx, &claims))
	require.Len(t, claims.Items, 1)
}

// claimFailureClient fails a selected claim read or its subsequent adoption
// update, while allowing pool, cluster and address lookups to succeed.
type claimFailureClient struct {
	client.Client
	failReadAt int
	claimReads int
	updates    int
	readErr    error
	updateErr  error
}

func (c *claimFailureClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*ipamv1.IPAddressClaim); ok {
		c.claimReads++
		if c.claimReads == c.failReadAt {
			return c.readErr
		}
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *claimFailureClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if _, ok := obj.(*ipamv1.IPAddressClaim); ok {
		c.updates++
		return c.updateErr
	}
	return c.Client.Update(ctx, obj, opts...)
}
