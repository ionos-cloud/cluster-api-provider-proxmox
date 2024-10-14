/*
Copyright 2023-2024 IONOS Cloud.

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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/test/helpers"
	// +kubebuilder:scaffold:imports

	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	testEnv    *helpers.TestEnvironment
	managerCtx = ctrl.SetupSignalHandler()
	testNS     = "test-ns"

	k8sClient     client.Client
	proxmoxClient *proxmoxtest.MockClient
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	sc, rc := GinkgoConfiguration()
	sc.FailFast = true
	rc.NoColor = true
	RunSpecs(t, "Controller Suite", sc, rc)
}

var _ = BeforeSuite(func() {
	proxmoxClient = proxmoxtest.NewMockClient(GinkgoT())
	testEnv = helpers.NewTestEnvironment(false, proxmoxClient)
	// TODO: do I need this?
	cache := testEnv.GetCache()

	indexFunc := func(obj client.Object) []string {
		return []string{obj.(*ipamv1.IPAddress).Spec.PoolRef.Name}
	}

	if err := cache.IndexField(testEnv.GetContext(), &ipamv1.IPAddress{}, "spec.poolRef.name", indexFunc); err != nil {
		panic(err)
	}

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// +kubebuilder:scaffold:scheme

	cachingClient, err := client.New(testEnv.GetConfig(), client.Options{Scheme: testEnv.Scheme()})
	Expect(err).NotTo(HaveOccurred())
	Expect(cachingClient).NotTo(BeNil())

	k8sClient = cachingClient

	proxmoxClusterReconciler := ProxmoxClusterReconciler{
		Client:        k8sClient,
		Scheme:        testEnv.GetScheme(),
		Recorder:      &record.FakeRecorder{},
		ProxmoxClient: testEnv.ProxmoxClient,
	}
	Expect(proxmoxClusterReconciler.SetupWithManager(testEnv.GetContext(), testEnv.Manager)).To(Succeed())

	proxmoxMachineReconciler := ProxmoxMachineReconciler{
		Client:        k8sClient,
		Scheme:        testEnv.GetScheme(),
		Recorder:      &record.FakeRecorder{},
		ProxmoxClient: testEnv.ProxmoxClient,
	}
	Expect(proxmoxMachineReconciler.SetupWithManager(testEnv.Manager)).To(Succeed())

	go func() {
		defer GinkgoRecover()
		err := testEnv.StartManager(managerCtx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
