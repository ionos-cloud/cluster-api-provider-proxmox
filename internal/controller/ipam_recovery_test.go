/*
Copyright 2024-2026 IONOS Cloud.

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

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
)

func TestOwnerlessClaimEventsRetryIncompleteRecovery(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, infrav1.AddToScheme(scheme))
	candidate := &infrav1.ProxmoxMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "candidate", Namespace: "cluster-ns", Labels: map[string]string{clusterv1.ClusterNameLabel: "cluster"}},
		Spec:       infrav1.ProxmoxMachineSpec{Network: &infrav1.NetworkSpec{}},
	}
	recovered := candidate.DeepCopy()
	recovered.Name = "recovered"
	recovered.Status.IPAddresses = []infrav1.IPAddressesSpec{{NetName: "net0", IPv4: []string{"10.0.0.1"}}}
	otherCluster := candidate.DeepCopy()
	otherCluster.Name = "other-cluster"
	otherCluster.Labels[clusterv1.ClusterNameLabel] = "other"
	otherNamespace := candidate.DeepCopy()
	otherNamespace.Namespace = "other-ns"
	noNetwork := candidate.DeepCopy()
	noNetwork.Name = "no-network"
	noNetwork.Spec.Network = nil
	deleting := candidate.DeepCopy()
	deleting.Name = "deleting"
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"test.example/retain"}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(candidate, recovered, otherCluster, otherNamespace, noNetwork, deleting).Build()
	reconciler := &ProxmoxMachineReconciler{Client: cl}
	claim := &ipamv1.IPAddressClaim{ObjectMeta: metav1.ObjectMeta{Name: "restored", Namespace: candidate.Namespace, Labels: candidate.Labels}}
	require.Equal(t, []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(candidate)}}, reconciler.ownerlessClaimToMachines(context.Background(), claim))
	address := &ipamv1.IPAddress{ObjectMeta: metav1.ObjectMeta{
		Name: "restored-address", Namespace: candidate.Namespace, Labels: candidate.Labels,
		OwnerReferences: []metav1.OwnerReference{{APIVersion: ipamv1.GroupVersion.String(), Kind: "IPAddressClaim", Name: claim.Name, Controller: new(true)}},
	}}
	require.Equal(t, []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(candidate)}}, reconciler.incompleteRecoveryMachines(context.Background(), address))
	address.Labels = nil
	require.Empty(t, reconciler.incompleteRecoveryMachines(context.Background(), address))
	claim.OwnerReferences = []metav1.OwnerReference{{Name: candidate.Name}}
	require.Empty(t, reconciler.ownerlessClaimToMachines(context.Background(), claim))
	claim.OwnerReferences = nil
	claim.Labels = nil
	require.Empty(t, reconciler.ownerlessClaimToMachines(context.Background(), claim))
}

func TestAddressRestoredAfterClaimRetriesRecovery(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, infrav1.AddToScheme(scheme))
	require.NoError(t, clusterv1.AddToScheme(scheme))
	require.NoError(t, ipamv1.AddToScheme(scheme))
	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "test"}}
	infraCluster := &infrav1.ProxmoxCluster{ObjectMeta: metav1.ObjectMeta{
		Name: cluster.Name, Namespace: cluster.Namespace,
		OwnerReferences: []metav1.OwnerReference{{APIVersion: clusterv1.GroupVersion.String(), Kind: "Cluster", Name: cluster.Name}},
	}}
	machine := &infrav1.ProxmoxMachine{ObjectMeta: metav1.ObjectMeta{
		Name: "machine", Namespace: cluster.Namespace, UID: "machine-uid", Labels: map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
	}, Spec: infrav1.ProxmoxMachineSpec{Network: &infrav1.NetworkSpec{}}}
	def := ipam.IPClaimDef{Device: infrav1.DefaultNetworkDevice, PoolRef: corev1.TypedLocalObjectReference{
		APIGroup: new("ipam.cluster.x-k8s.io"), Kind: "InClusterIPPool", Name: "pool",
	}, Annotations: map[string]string{infrav1.ProxmoxPoolOffsetAnnotation: "0"}}
	name := ipam.IPAddressFormat(machine.Name, def.Device, 0, infrav1.DefaultSuffix)
	claim := &ipamv1.IPAddressClaim{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: machine.Namespace, UID: "claim-uid", Labels: machine.Labels, Annotations: def.Annotations,
	}, Spec: ipamv1.IPAddressClaimSpec{PoolRef: ipamv1.IPPoolReference{APIGroup: *def.PoolRef.APIGroup, Kind: def.PoolRef.Kind, Name: def.PoolRef.Name}},
		Status: ipamv1.IPAddressClaimStatus{AddressRef: ipamv1.IPAddressReference{Name: name}}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, infraCluster, machine, claim).Build()
	reconciler := &ProxmoxMachineReconciler{Client: cl}
	helper := ipam.NewHelper(cl, infraCluster)
	wantRequests := []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(machine)}}
	require.Equal(t, wantRequests, reconciler.ownerlessClaimToMachines(ctx, claim))
	resolution, err := helper.ResolveIPAddressClaim(ctx, machine, def)
	require.NoError(t, err)
	require.Equal(t, ipam.ConflictAddressMissing, resolution.ConflictReason)

	// No claim event follows: only its previously referenced address arrives.
	address := &ipamv1.IPAddress{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: machine.Namespace, Labels: machine.Labels,
		OwnerReferences: []metav1.OwnerReference{{APIVersion: ipamv1.GroupVersion.String(), Kind: "IPAddressClaim", Name: claim.Name, UID: claim.UID, Controller: new(true)}},
	}, Spec: ipamv1.IPAddressSpec{ClaimRef: ipamv1.IPAddressClaimReference{Name: claim.Name}, PoolRef: claim.Spec.PoolRef, Address: "10.0.0.10", Prefix: new(int32(24))}}
	require.NoError(t, cl.Create(ctx, address))
	require.Equal(t, wantRequests, reconciler.incompleteRecoveryMachines(ctx, address))
	resolution, adopted, err := helper.AdoptIPAddressClaim(ctx, machine, def, true)
	require.NoError(t, err)
	require.True(t, adopted)
	require.Equal(t, ipam.ClaimResolved, resolution.Status)
	require.Equal(t, address.Spec.Address, resolution.Address.Spec.Address)
}
