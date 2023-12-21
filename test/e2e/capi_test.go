//go:build e2e
// +build e2e

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

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
	capi_e2e "sigs.k8s.io/cluster-api/test/e2e"
)

var _ = Describe("Running the Cluster API E2E tests", func() {
	var (
		ctx = context.TODO()
	)

	BeforeEach(func() {
		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(e2eConfig.Variables).To(HaveKey(capi_e2e.KubernetesVersionUpgradeFrom))
		Expect(e2eConfig.Variables).To(HaveKey(capi_e2e.KubernetesVersionUpgradeTo))
		Expect(e2eConfig.Variables).To(HaveKey(capi_e2e.CPMachineTemplateUpgradeTo))
		Expect(e2eConfig.Variables).To(HaveKey(capi_e2e.WorkersMachineTemplateUpgradeTo))
	})

	AfterEach(func() {
	})

	Context("Running the quick-start spec", func() {
		capi_e2e.QuickStartSpec(ctx, func() capi_e2e.QuickStartSpecInput {
			return capi_e2e.QuickStartSpecInput{
				E2EConfig:             e2eConfig,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapClusterProxy: bootstrapClusterProxy,
				ArtifactFolder:        artifactFolder,
				SkipCleanup:           skipCleanup,
			}
		})
	})

	// TODO ClusterClass is not supported yet
	//Context("Running the quick-start spec with ClusterClass", func() {
	//	capi_e2e.QuickStartSpec(ctx, func() capi_e2e.QuickStartSpecInput {
	//		return capi_e2e.QuickStartSpecInput{
	//			E2EConfig:             e2eConfig,
	//			ClusterctlConfigPath:  clusterctlConfigPath,
	//			BootstrapClusterProxy: bootstrapClusterProxy,
	//			ArtifactFolder:        artifactFolder,
	//			SkipCleanup:           skipCleanup,
	//			Flavor:                pointer.String("topology"),
	//		}
	//	})
	//})

	Context("Should successfully remediate unhealthy machines with MachineHealthCheck", func() {
		capi_e2e.KCPRemediationSpec(ctx, func() capi_e2e.KCPRemediationSpecInput {
			return capi_e2e.KCPRemediationSpecInput{
				E2EConfig:             e2eConfig,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapClusterProxy: bootstrapClusterProxy,
				ArtifactFolder:        artifactFolder,
				SkipCleanup:           skipCleanup,
			}
		})
	})

	Context("Running the workload cluster upgrade spec [K8s-Upgrade]", func() {
		capi_e2e.ClusterUpgradeConformanceSpec(ctx, func() capi_e2e.ClusterUpgradeConformanceSpecInput {
			return capi_e2e.ClusterUpgradeConformanceSpecInput{
				E2EConfig:             e2eConfig,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapClusterProxy: bootstrapClusterProxy,
				ArtifactFolder:        artifactFolder,
				SkipCleanup:           skipCleanup,
				SkipConformanceTests:  true,
			}
		})
	})

	Context("Running KCP upgrade in a HA cluster [K8s-Upgrade]", func() {
		capi_e2e.ClusterUpgradeConformanceSpec(ctx, func() capi_e2e.ClusterUpgradeConformanceSpecInput {
			return capi_e2e.ClusterUpgradeConformanceSpecInput{
				E2EConfig:                e2eConfig,
				ClusterctlConfigPath:     clusterctlConfigPath,
				BootstrapClusterProxy:    bootstrapClusterProxy,
				ArtifactFolder:           artifactFolder,
				ControlPlaneMachineCount: pointer.Int64(3),
				WorkerMachineCount:       pointer.Int64(0),
				SkipCleanup:              skipCleanup,
				SkipConformanceTests:     true,
			}
		})
	})

})
