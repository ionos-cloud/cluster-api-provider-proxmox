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

package controller

import (
	"context"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	clustererrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
)

var (
	clusterName   = "test-cluster"
	testFinalizer = "cluster-test.cluster.x-k8s.io"
)

var _ = Describe("Controller Test", func() {
	g := NewWithT(GinkgoT())

	BeforeEach(func() {
		gvk := infrav1.GroupVersion.WithKind(reflect.TypeOf(infrav1.ProxmoxCluster{}).Name())

		cl := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: testNS,
				UID:       "1000",
			},
			Spec: clusterv1.ClusterSpec{
				Paused: ptr.To(false),
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					Kind:     gvk.Kind,
					Name:     clusterName,
					APIGroup: gvk.Group,
				},
			},
		}

		ctrlutil.AddFinalizer(cl, testFinalizer)
		g.Expect(k8sClient.Create(context.Background(), cl)).To(Succeed())
	})

	AfterEach(func() {
		var cl clusterv1.Cluster
		g.Expect(k8sClient.Get(testEnv.GetContext(), client.ObjectKey{Name: "test", Namespace: testNS}, &cl)).To(Succeed())
		ctrlutil.RemoveFinalizer(&cl, testFinalizer)
		g.Expect(k8sClient.Update(testEnv.GetContext(), &cl)).To(Succeed())

		g.Eventually(func(g Gomega) {
			err := k8sClient.Get(testEnv.GetContext(), client.ObjectKey{Name: "test", Namespace: testNS}, &clusterv1.Cluster{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(time.Second * 10).
			WithPolling(time.Second).
			Should(Succeed())
	})

	Context("IPAM tests", func() {
		It("Should successfully create IPAM related resources", func() {
			cl := buildProxmoxCluster(clusterName)
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cl)).NotTo(HaveOccurred())

			helper := ipam.NewHelper(k8sClient, &cl)

			defer cleanupResources(testEnv.GetContext(), g, cl)

			assertClusterIsReady(testEnv.GetContext(), g, clusterName)

			g.Eventually(func(g Gomega) {
				pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPv4Format)
				g.Expect(err).ToNot(HaveOccurred())

				config := cl.Spec.IPv4Config
				g.Expect(pool.Spec.Addresses).To(ConsistOf(config.Addresses))
				g.Expect(config.Gateway).To(BeEquivalentTo(pool.Spec.Gateway))
				g.Expect(pool.Spec.Prefix).To(BeEquivalentTo(24))

				// check if status was updated
				g.Expect(k8sClient.Get(testEnv.GetContext(), client.ObjectKeyFromObject(&cl), &cl)).To(Succeed())
				g.Expect(cl.Status.InClusterIPPoolRef).NotTo(BeNil())
				g.Expect(cl.Status.InClusterIPPoolRef[0].Name).To(BeEquivalentTo(pool.GetName()))
			}).WithTimeout(time.Second * 10).
				WithPolling(time.Second).
				Should(Succeed())
		})
		It("Should successfully create IPAM IPv6 related resources", func() {
			cl := buildProxmoxCluster(clusterName)
			cl.Spec.IPv6Config = &infrav1.IPConfigSpec{
				Addresses: []string{"2001:db8::/64"},
				Prefix:    64,
				Gateway:   "2001:db8::1",
			}
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cl)).NotTo(HaveOccurred())

			helper := ipam.NewHelper(k8sClient, &cl)

			defer cleanupResources(testEnv.GetContext(), g, cl)

			assertClusterIsReady(testEnv.GetContext(), g, clusterName)

			g.Eventually(func(g Gomega) {
				pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPv6Format)
				g.Expect(err).ToNot(HaveOccurred())

				config := cl.Spec.IPv6Config
				g.Expect(pool.Spec.Addresses).To(ConsistOf(config.Addresses))
				g.Expect(config.Gateway).To(BeEquivalentTo(pool.Spec.Gateway))
				g.Expect(pool.Spec.Prefix).To(BeEquivalentTo(64))

				// check if status was updated
				g.Expect(k8sClient.Get(testEnv.GetContext(), client.ObjectKeyFromObject(&cl), &cl)).To(Succeed())
				g.Expect(cl.Status.InClusterIPPoolRef).NotTo(BeNil())
				g.Expect(cl.Status.InClusterIPPoolRef[1].Name).To(BeEquivalentTo(pool.GetName()))
			}).WithTimeout(time.Second * 10).
				WithPolling(time.Second).
				Should(Succeed())
		})
		It("Should successfully assign ControlPlaneEndpoint", func() {
			cl := buildProxmoxCluster(clusterName)

			g.Expect(k8sClient.Create(testEnv.GetContext(), &cl)).NotTo(HaveOccurred())
			helper := ipam.NewHelper(k8sClient, &cl)

			defer cleanupResources(testEnv.GetContext(), g, cl)

			assertClusterIsReady(testEnv.GetContext(), g, clusterName)

			g.Eventually(func(g Gomega) {
				pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPv4Format)
				g.Expect(err).ToNot(HaveOccurred())

				config := cl.Spec.IPv4Config
				g.Expect(pool.Spec.Addresses).To(ConsistOf(config.Addresses))
				g.Expect(config.Gateway).To(BeEquivalentTo(pool.Spec.Gateway))
				g.Expect(pool.Spec.Prefix).To(BeEquivalentTo(24))

				g.Expect(k8sClient.Get(testEnv.GetContext(), client.ObjectKeyFromObject(&cl), &cl)).To(Succeed())
				g.Expect(cl.Status.InClusterIPPoolRef).NotTo(BeNil())
				g.Expect(cl.Status.InClusterIPPoolRef[0].Name).To(BeEquivalentTo(pool.GetName()))
			}).WithTimeout(time.Second * 10).
				WithPolling(time.Second).
				Should(Succeed())

			pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPv4Format)
			g.Expect(err).ToNot(HaveOccurred())
			// create an IPAddress.
			g.Expect(k8sClient.Create(testEnv.GetContext(), dummyIPAddress(k8sClient, &cl, pool.GetName()))).To(Succeed())

			g.Eventually(func(g Gomega) {
				pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPv4Format)
				g.Expect(err).ToNot(HaveOccurred())

				ipAddr, err := helper.GetIPAddress(testEnv.GetContext(), client.ObjectKeyFromObject(&cl))
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(ipAddr).ToNot(BeNil())
				g.Expect(ipAddr.Spec.PoolRef.Name).To(BeEquivalentTo(pool.GetName()))
				g.Expect(ipAddr.Spec.Address).ToNot(BeEmpty())
				g.Expect(ptr.Deref(ipAddr.Spec.Prefix, 0)).To(BeEquivalentTo(pool.Spec.Prefix))
				g.Expect(ipAddr.Spec.Gateway).To(BeEquivalentTo(pool.Spec.Gateway))

				// check controlPlaneEndpoint is updated
				g.Expect(cl.Spec.ControlPlaneEndpoint.IsZero()).NotTo(BeTrue())
				g.Expect(cl.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(ControlPlaneEndpointPort))
				g.Expect(cl.Spec.ControlPlaneEndpoint.Host).To(BeEquivalentTo(ipAddr.Spec.Address))
			}).WithTimeout(time.Second * 10).
				WithPolling(time.Second).
				Should(Succeed())
		})
		It("Should reconcile failed cluster state", func() {
			cl := buildProxmoxCluster(clusterName)
			cl.Status.FailureReason = ptr.To(clustererrors.InvalidConfigurationClusterError)
			cl.Status.FailureMessage = ptr.To("No credentials found, ProxmoxCluster missing credentialsRef")
			g.Expect(k8sClient.Create(testEnv.GetContext(), &cl)).NotTo(HaveOccurred())
			g.Expect(k8sClient.Status().Update(testEnv.GetContext(), &cl)).NotTo(HaveOccurred())

			defer cleanupResources(testEnv.GetContext(), g, cl)

			g.Eventually(func(g Gomega) {
				var res infrav1.ProxmoxCluster
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: testNS,
					Name:      clusterName,
				}, &res)).To(Succeed())

				g.Expect(res.Status.FailureReason).To(BeNil())
				g.Expect(res.Status.FailureMessage).To(BeNil())
			}).WithTimeout(time.Second * 20).
				WithPolling(time.Second).
				Should(Succeed())

		})
	})
})

var _ = Describe("External Credentials Tests", func() {
	Context("Reconcile an ProxmoxCluster", func() {
		It("create and destroy a cluster", func() {
			secret := createSecret()
			proxmoxCluster := createProxmoxCluster()
			setCredentialsRefOnProxmoxCluster(proxmoxCluster, secret)
			capiCluster := createOwnerCluster(proxmoxCluster)
			proxmoxCluster = refreshCluster(proxmoxCluster)
			setCapiClusterOwnerRefOnProxmoxCluster(proxmoxCluster, capiCluster)

			assertProxmoxClusterHasFinalizer(proxmoxCluster, infrav1.ClusterFinalizer)
			assertSecretHasNumberOfOwnerRefs(secret, 1)
			assertSecretHasOwnerRef(secret, proxmoxCluster.Name)
			assertSecretHasFinalizer(secret, infrav1.SecretFinalizer)
			assertProxmoxClusterIsReady(proxmoxCluster)

			deleteCapiCluster(capiCluster)
			deleteProxmoxCluster(proxmoxCluster)

			assertSecretHasOwnerRef(secret, proxmoxCluster.Name)
			assertSecretHasFinalizer(secret, infrav1.SecretFinalizer)

			cleanup(proxmoxCluster, capiCluster, secret)
		})

		It("multiple clusters can set ownerRef on secret", func() {
			secret := createSecret()
			setRandomOwnerRefOnSecret(secret, "another-cluster")

			//  First cluster
			proxmoxCluster1 := createProxmoxCluster()
			setCredentialsRefOnProxmoxCluster(proxmoxCluster1, secret)
			capiCluster1 := createOwnerCluster(proxmoxCluster1)
			proxmoxCluster1 = refreshCluster(proxmoxCluster1)
			setCapiClusterOwnerRefOnProxmoxCluster(proxmoxCluster1, capiCluster1)
			assertProxmoxClusterIsReady(proxmoxCluster1)
			assertProxmoxClusterHasFinalizer(proxmoxCluster1, infrav1.ClusterFinalizer)

			//  Second cluster
			proxmoxCluster2 := createProxmoxCluster()
			setCredentialsRefOnProxmoxCluster(proxmoxCluster2, secret)
			capiCluster2 := createOwnerCluster(proxmoxCluster2)
			proxmoxCluster2 = refreshCluster(proxmoxCluster2)
			setCapiClusterOwnerRefOnProxmoxCluster(proxmoxCluster2, capiCluster2)
			assertProxmoxClusterIsReady(proxmoxCluster2)
			assertProxmoxClusterHasFinalizer(proxmoxCluster2, infrav1.ClusterFinalizer)

			// Check owner references
			assertSecretHasNumberOfOwnerRefs(secret, 3)
			assertSecretHasOwnerRef(secret, proxmoxCluster1.Name)
			assertSecretHasOwnerRef(secret, proxmoxCluster2.Name)
			assertSecretHasOwnerRef(secret, "another-cluster")
			assertSecretHasFinalizer(secret, infrav1.SecretFinalizer)

			// Delete second cluster
			deleteCapiCluster(capiCluster2)
			deleteProxmoxCluster(proxmoxCluster2)

			// Check owner references
			assertSecretHasNumberOfOwnerRefs(secret, 2)
			assertSecretHasOwnerRef(secret, proxmoxCluster1.Name)
			assertSecretHasOwnerRef(secret, "another-cluster")
			assertSecretHasFinalizer(secret, infrav1.SecretFinalizer)

			// Delete first cluster
			deleteCapiCluster(capiCluster1)
			deleteProxmoxCluster(proxmoxCluster1)

			// Check owner references
			assertSecretHasNumberOfOwnerRefs(secret, 1)
			assertSecretHasOwnerRef(secret, "another-cluster")
			assertSecretHasFinalizer(secret, infrav1.SecretFinalizer)

			cleanup(proxmoxCluster1, capiCluster1, proxmoxCluster2, capiCluster2, secret)
		})
	})

	It("should remove ProxmoxCluster finalizer if the secret does not exist", func() {
		proxmoxCluster := createProxmoxCluster()
		setRandomCredentialsRefOnProxmoxCluster(proxmoxCluster)

		capiCluster := createOwnerCluster(proxmoxCluster)
		proxmoxCluster = refreshCluster(proxmoxCluster)
		setCapiClusterOwnerRefOnProxmoxCluster(proxmoxCluster, capiCluster)

		assertProxmoxClusterIsNotReady(proxmoxCluster)
		assertProxmoxClusterHasFinalizer(proxmoxCluster, infrav1.ClusterFinalizer)

		By("deleting the proxmoxcluster while the secret is gone")
		deleteCapiCluster(capiCluster)
		deleteProxmoxCluster(proxmoxCluster)
		assertProxmoxClusterIsDeleted(proxmoxCluster)
	})
})

func cleanupResources(ctx context.Context, g Gomega, cl infrav1.ProxmoxCluster) {
	g.Expect(k8sClient.Delete(context.Background(), &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: testNS}})).To(Succeed())
	g.Expect(k8sClient.Delete(ctx, &cl)).To(Succeed())
	g.Expect(k8sClient.DeleteAllOf(ctx, &ipamicv1.InClusterIPPool{}, client.InNamespace(testNS))).To(Succeed())
	g.Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: cl.GetName(), Namespace: cl.GetNamespace()}, &infrav1.ProxmoxCluster{})
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(Succeed())
}

func buildProxmoxCluster(name string) infrav1.ProxmoxCluster {
	cl := infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       "test",
					UID:        "1000",
				},
			},
		},
		Spec: infrav1.ProxmoxClusterSpec{
			ControlPlaneEndpoint: &clusterv1.APIEndpoint{
				Host: "10.10.10.11",
				Port: 6443,
			},
			IPv4Config: &infrav1.IPConfigSpec{
				Addresses: []string{
					"10.10.10.2-10.10.10.10",
					"10.10.10.100-10.10.10.125",
					"10.10.10.192/64",
				},
				Gateway: "10.10.10.1",
				Prefix:  24,
			},
			DNSServers: []string{"8.8.8.8", "8.8.4.4"},
		},
	}

	return cl
}

func assertClusterIsReady(ctx context.Context, g Gomega, clusterName string) {
	g.Eventually(func(g Gomega) {
		var res infrav1.ProxmoxCluster
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: testNS,
			Name:      clusterName,
		}, &res)).To(Succeed())

		g.Expect(ptr.Deref(res.Status.Ready, false)).To(BeTrue())
	}).WithTimeout(time.Second * 20).
		WithPolling(time.Second).
		Should(Succeed())
}

func dummyIPAddress(client client.Client, owner client.Object, poolName string) *ipamv1.IPAddress {
	gvk, err := apiutil.GVKForObject(new(ipamicv1.InClusterIPPool), client.Scheme())
	if err != nil {
		panic(err)
	}
	return &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.GetName(),
			Namespace: owner.GetNamespace(),
		},
		Spec: ipamv1.IPAddressSpec{
			ClaimRef: ipamv1.IPAddressClaimReference{
				Name: owner.GetName(),
			},
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: gvk.Group,
				Kind:     gvk.Kind,
				Name:     poolName,
			},
			Address: "10.10.10.11",
			Prefix:  ptr.To[int32](24),
			Gateway: "10.10.10.1",
		},
	}
}

func createSecret() *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "secret-",
			Namespace:    "default",
		},
		Data: map[string][]byte{
			"url":    []byte("url"),
			"token":  []byte("token"),
			"secret": []byte("secret"),
		},
	}
	Expect(k8sClient.Create(testEnv.GetContext(), secret)).To(Succeed())
	return secret
}

func createProxmoxCluster() *infrav1.ProxmoxCluster {
	proxmoxCluster := &infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "proxmox-test-",
			Namespace:    "default",
		},
		Spec: infrav1.ProxmoxClusterSpec{
			IPv4Config: &infrav1.IPConfigSpec{
				Addresses: []string{
					"10.10.10.2-10.10.10.10",
					"10.10.10.100-10.10.10.125",
					"10.10.10.192/64",
				},
				Gateway: "10.10.10.1",
				Prefix:  24,
			},
			DNSServers: []string{"8.8.8.8", "8.8.4.4"},
		},
	}
	Expect(testEnv.Create(testEnv.GetContext(), proxmoxCluster)).To(Succeed())
	return proxmoxCluster
}

func setCredentialsRefOnProxmoxCluster(proxmoxCluster *infrav1.ProxmoxCluster, secret *corev1.Secret) {
	Eventually(func() error {
		ph, err := patch.NewHelper(proxmoxCluster, testEnv)
		Expect(err).ShouldNot(HaveOccurred())
		proxmoxCluster.Spec.CredentialsRef = &corev1.SecretReference{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}
		return ph.Patch(testEnv.GetContext(), proxmoxCluster, patch.WithStatusObservedGeneration{})
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeNil())
}

func setRandomCredentialsRefOnProxmoxCluster(proxmoxCluster *infrav1.ProxmoxCluster) {
	Eventually(func() error {
		ph, err := patch.NewHelper(proxmoxCluster, testEnv)
		Expect(err).ShouldNot(HaveOccurred())
		proxmoxCluster.Spec.CredentialsRef = &corev1.SecretReference{
			Name:      util.RandomString(6),
			Namespace: util.RandomString(6),
		}
		return ph.Patch(testEnv.GetContext(), proxmoxCluster, patch.WithStatusObservedGeneration{})
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeNil())
}

func createOwnerCluster(proxmoxCluster *infrav1.ProxmoxCluster) *clusterv1.Cluster {
	capiCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Namespace:    "default",
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: infrav1.GroupVersion.Group,
				Kind:     "ProxmoxCluster",
				Name:     proxmoxCluster.Name,
			},
		},
	}
	ctrlutil.AddFinalizer(capiCluster, "cluster.cluster.x-k8s.io")
	Expect(testEnv.Create(testEnv.GetContext(), capiCluster)).To(Succeed())
	return capiCluster
}

func setCapiClusterOwnerRefOnProxmoxCluster(proxmoxCluster *infrav1.ProxmoxCluster, capiCluster *clusterv1.Cluster) {
	Eventually(func() error {
		ph, err := patch.NewHelper(proxmoxCluster, testEnv)
		Expect(err).ShouldNot(HaveOccurred())
		proxmoxCluster.OwnerReferences = append(proxmoxCluster.OwnerReferences, metav1.OwnerReference{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
			Name:       capiCluster.Name,
			UID:        (types.UID)(util.RandomString(6)),
		})
		return ph.Patch(testEnv.GetContext(), proxmoxCluster, patch.WithStatusObservedGeneration{})
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeNil())
}

func setRandomOwnerRefOnSecret(secret *corev1.Secret, ownerRef string) {
	Eventually(func() error {
		ph, err := patch.NewHelper(secret, testEnv)
		Expect(err).ShouldNot(HaveOccurred())
		secret.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       "ProxmoxCluster",
				Name:       ownerRef,
				UID:        (types.UID)(util.RandomString(6)),
			},
		}
		return ph.Patch(testEnv.GetContext(), secret, patch.WithStatusObservedGeneration{})
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeNil())
}

func refreshCluster(proxmoxCluster *infrav1.ProxmoxCluster) *infrav1.ProxmoxCluster {
	key := client.ObjectKey{Namespace: proxmoxCluster.Namespace, Name: proxmoxCluster.Name}
	Expect(testEnv.Get(testEnv.GetContext(), key, proxmoxCluster)).To(Succeed())
	return proxmoxCluster
}

func deleteProxmoxCluster(proxmoxCluster *infrav1.ProxmoxCluster) {
	Eventually(func() bool {
		err := testEnv.Delete(testEnv.GetContext(), proxmoxCluster)
		return err == nil
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func deleteCapiCluster(cluster *clusterv1.Cluster) {
	Eventually(func() bool {
		err := testEnv.Delete(testEnv.GetContext(), cluster)
		return err == nil
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func assertProxmoxClusterHasFinalizer(proxmoxCluster *infrav1.ProxmoxCluster, finalizer string) {
	key := client.ObjectKey{Namespace: proxmoxCluster.Namespace, Name: proxmoxCluster.Name}
	Eventually(func() bool {
		if err := testEnv.Get(testEnv.GetContext(), key, proxmoxCluster); err != nil {
			return false
		}
		return ctrlutil.ContainsFinalizer(proxmoxCluster, finalizer)
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func assertSecretHasFinalizer(secret *corev1.Secret, finalizer string) {
	key := client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}
	Eventually(func() bool {
		if err := testEnv.Get(testEnv.GetContext(), key, secret); err != nil {
			return false
		}
		return ctrlutil.ContainsFinalizer(secret, finalizer)
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func assertSecretHasOwnerRef(secret *corev1.Secret, ownerRef string) {
	key := client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}
	Eventually(func() bool {
		if err := testEnv.Get(testEnv.GetContext(), key, secret); err != nil {
			return false
		}
		for _, ref := range secret.OwnerReferences {
			if ref.Name == ownerRef {
				return true
			}
		}
		return false
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func assertSecretHasNumberOfOwnerRefs(secret *corev1.Secret, nrOfOwnerRefs int) {
	key := client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}
	Eventually(func() bool {
		if err := testEnv.Get(testEnv.GetContext(), key, secret); err != nil {
			return false
		}
		return len(secret.OwnerReferences) == nrOfOwnerRefs
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func assertProxmoxClusterIsReady(proxmoxCluster *infrav1.ProxmoxCluster) {
	key := client.ObjectKey{Namespace: proxmoxCluster.Namespace, Name: proxmoxCluster.Name}
	Eventually(func() bool {
		if err := testEnv.Get(testEnv.GetContext(), key, proxmoxCluster); err != nil {
			return false
		}
		return conditions.IsTrue(proxmoxCluster, string(infrav1.ProxmoxClusterReady))
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func assertProxmoxClusterIsNotReady(proxmoxCluster *infrav1.ProxmoxCluster) {
	key := client.ObjectKey{Namespace: proxmoxCluster.Namespace, Name: proxmoxCluster.Name}
	Eventually(func() bool {
		if err := testEnv.Get(testEnv.GetContext(), key, proxmoxCluster); err != nil {
			return false
		}
		return conditions.IsFalse(proxmoxCluster, string(infrav1.ProxmoxClusterReady))
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func assertProxmoxClusterIsDeleted(proxmoxCluster *infrav1.ProxmoxCluster) {
	key := client.ObjectKey{Namespace: proxmoxCluster.Namespace, Name: proxmoxCluster.Name}
	Eventually(func() bool {
		err := testEnv.Get(testEnv.GetContext(), key, proxmoxCluster)
		return apierrors.IsNotFound(err)
	}).WithTimeout(time.Second * 10).
		WithPolling(time.Second).
		Should(BeTrue())
}

func cleanup(objs ...client.Object) {
	Expect(testEnv.Cleanup(testEnv.GetContext(), objs...)).To(Succeed())
}
