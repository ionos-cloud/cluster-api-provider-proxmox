//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

func Byf(format string, a ...interface{}) {
	By(fmt.Sprintf(format, a...))
}

func setupSpecNamespace(ctx context.Context, specName string, clusterProxy framework.ClusterProxy, artifactFolder string) (*corev1.Namespace, context.CancelFunc) {
	Byf("Creating a namespace for hosting the %q test spec", specName)
	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:   clusterProxy.GetClient(),
		ClientSet: clusterProxy.GetClientSet(),
		Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
		LogFolder: filepath.Join(artifactFolder, "clusters", clusterProxy.GetName()),
	})

	return namespace, cancelWatches
}

func createSecret(ctx context.Context, name, namespace string, data map[string][]byte, clusterProxy framework.ClusterProxy) error {
	Byf("Creating secret %s in namespace %s", name, namespace)

	secret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}

	return clusterProxy.GetClient().Create(ctx, secret)
}

type cleanupInput struct {
	SpecName             string
	ClusterProxy         framework.ClusterProxy
	ArtifactFolder       string
	ClusterctlConfigPath string
	Namespace            *corev1.Namespace
	CancelWatches        context.CancelFunc
	Cluster              *clusterv1.Cluster
	IntervalsGetter      func(spec, key string) []interface{}
	SkipCleanup          bool
	AdditionalCleanup    func()
}

func dumpSpecResourcesAndCleanup(ctx context.Context, input cleanupInput) {
	defer func() {
		input.CancelWatches()
	}()

	if input.Cluster == nil {
		By("Unable to dump workload cluster logs as the cluster is nil")
	} else {
		Byf("Dumping logs from the %q workload cluster", input.Cluster.Name)
		input.ClusterProxy.CollectWorkloadClusterLogs(ctx, input.Cluster.Namespace, input.Cluster.Name, filepath.Join(input.ArtifactFolder, "clusters", input.Cluster.Name))
	}

	Byf("Dumping all the Cluster API resources in the %q namespace", input.Namespace.Name)
	// Dump all Cluster API related resources to artifacts before deleting them.
	framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
		Lister:               input.ClusterProxy.GetClient(),
		KubeConfigPath:       input.ClusterProxy.GetKubeconfigPath(),
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Namespace:            input.Namespace.Name,
		LogPath:              filepath.Join(input.ArtifactFolder, "clusters", input.ClusterProxy.GetName(), "resources"),
	})

	if input.SkipCleanup {
		return
	}

	Byf("Deleting all clusters in the %s namespace", input.Namespace.Name)
	// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
	// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
	// instead of DeleteClusterAndWait
	framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
		ClusterProxy:         input.ClusterProxy,
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Namespace:            input.Namespace.Name,
	}, input.IntervalsGetter(input.SpecName, "wait-delete-cluster")...)

	Byf("Deleting namespace used for hosting the %q test spec", input.SpecName)
	framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
		Deleter: input.ClusterProxy.GetClient(),
		Name:    input.Namespace.Name,
	})

	if input.AdditionalCleanup != nil {
		Byf("Running additional cleanup for the %q test spec", input.SpecName)
		input.AdditionalCleanup()
	}
}

// verifyProxmoxMachineAddresses lists all ProxmoxMachine objects for a given cluster
// and verifies that each machine has populated Status.Addresses with at least one
// MachineHostName and one MachineInternalIP entry.
// This catches state machine bugs where reconcileMachineAddresses is skipped.
func verifyProxmoxMachineAddresses(ctx context.Context, clusterProxy framework.ClusterProxy, namespace, clusterName string) {
	Byf("Listing ProxmoxMachine objects for cluster %q in namespace %q", clusterName, namespace)
	machineList := &infrav1.ProxmoxMachineList{}
	Expect(clusterProxy.GetClient().List(ctx, machineList,
		client.InNamespace(namespace),
		client.MatchingLabels{clusterv1.ClusterNameLabel: clusterName},
	)).To(Succeed(), "Failed to list ProxmoxMachines")

	Expect(machineList.Items).ToNot(BeEmpty(), "Expected at least one ProxmoxMachine for cluster %q", clusterName)

	for i := range machineList.Items {
		machine := &machineList.Items[i]
		Byf("Verifying addresses for ProxmoxMachine %q", machine.Name)

		Expect(machine.Status.Addresses).ToNot(BeEmpty(),
			"ProxmoxMachine %q has no addresses in Status.Addresses", machine.Name)

		hasHostName := false
		hasInternalIP := false
		for _, addr := range machine.Status.Addresses {
			switch addr.Type {
			case clusterv1.MachineHostName:
				hasHostName = true
			case clusterv1.MachineInternalIP:
				hasInternalIP = true
			}
		}

		Expect(hasHostName).To(BeTrue(),
			"ProxmoxMachine %q has no MachineHostName address", machine.Name)
		Expect(hasInternalIP).To(BeTrue(),
			"ProxmoxMachine %q has no MachineInternalIP address", machine.Name)
	}
}
