//go:build e2e
// +build e2e

/*
Copyright 2020 The Kubernetes Authors.

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
	"os"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	capi_e2e "sigs.k8s.io/cluster-api/test/e2e"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/test/framework/kubetest"
	"sigs.k8s.io/cluster-api/util"
)

var _ = Describe("Conformance Tests", func() {
	var (
		ctx                 = context.TODO()
		specName            = "conformance-tests"
		repoList            = ""
		namespace           *corev1.Namespace
		cancelWatches       context.CancelFunc
		result              *clusterctl.ApplyClusterTemplateAndWaitResult
		clusterName         string
		clusterctlLogFolder string
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		Expect(e2eConfig).ToNot(BeNil(), "Invalid argument. e2eConfig can't be nil when calling %s spec", specName)
		Expect(clusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. clusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(bootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. bootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid argument. artifactFolder can't be created for %s spec", specName)
		Expect(kubetestConfigFilePath).ToNot(BeNil(), "Invalid argument. kubetestConfigFilePath can't be nil")

		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(e2eConfig.Variables).To(HaveKey(capi_e2e.KubernetesVersion))
		Expect(e2eConfig.Variables).To(HaveKey(capi_e2e.CNIPath))
		//Expect(e2eConfig.Variables).To(HaveKey(CCMPath))

		clusterName = fmt.Sprintf("capmox-conf-%s", util.RandomString(6))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		clusterctlLogFolder = filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName())

		result = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	AfterEach(func() {
		if result.Cluster == nil {
			// this means the cluster failed to come up. We make an attempt to find the cluster to be able to fetch logs for the failed bootstrapping.
			_ = bootstrapClusterProxy.GetClient().Get(ctx, types.NamespacedName{Name: clusterName, Namespace: namespace.Name}, result.Cluster)
		}

		cleanInput := cleanupInput{
			SpecName:        specName,
			Cluster:         result.Cluster,
			ClusterProxy:    bootstrapClusterProxy,
			Namespace:       namespace,
			CancelWatches:   cancelWatches,
			IntervalsGetter: e2eConfig.GetIntervals,
			SkipCleanup:     skipCleanup,
			ArtifactFolder:  artifactFolder,
		}

		dumpSpecResourcesAndCleanup(ctx, cleanInput)
	})

	It("Should run conformance tests", func() {
		var err error

		kubernetesVersion := e2eConfig.GetVariable(capi_e2e.KubernetesVersion)

		flavor := clusterctl.DefaultFlavor
		// clusters with CI artifacts or PR artifacts are based on a known CI version
		// PR artifacts will replace the CI artifacts during kubeadm init
		if useCIArtifacts {
			//kubernetesVersion = kubernetesVersion
			//Expect(err).NotTo(HaveOccurred())
			Expect(os.Setenv("CI_VERSION", kubernetesVersion)).To(Succeed())

			flavor = "conformance-ci-artifacts"
		}

		workerMachineCount, err := strconv.ParseInt(e2eConfig.GetVariable("CONFORMANCE_WORKER_MACHINE_COUNT"), 10, 64)
		Expect(err).NotTo(HaveOccurred())
		controlPlaneMachineCount, err := strconv.ParseInt(e2eConfig.GetVariable("CONFORMANCE_CONTROL_PLANE_MACHINE_COUNT"), 10, 64)
		Expect(err).NotTo(HaveOccurred())

		By("Initializes the work cluster")
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: bootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                clusterctlLogFolder,
				ClusterctlConfigPath:     clusterctlConfigPath,
				KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   flavor,
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        kubernetesVersion,
				ControlPlaneMachineCount: pointer.Int64Ptr(controlPlaneMachineCount),
				WorkerMachineCount:       pointer.Int64Ptr(workerMachineCount),
			},
			WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, result)

		workloadProxy := bootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterName)
		ginkgoNodes, err := strconv.Atoi(e2eConfig.GetVariable("CONFORMANCE_NODES"))
		Expect(err).NotTo(HaveOccurred())

		kubetest.Run(context.Background(),
			kubetest.RunInput{
				ClusterProxy:         workloadProxy,
				NumberOfNodes:        int(workerMachineCount),
				ConfigFilePath:       kubetestConfigFilePath,
				KubeTestRepoListPath: repoList,
				GinkgoNodes:          ginkgoNodes,
			},
		)
	})
})
