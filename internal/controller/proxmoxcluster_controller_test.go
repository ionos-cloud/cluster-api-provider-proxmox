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
	"context"
	"reflect"
	"time"

	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"

	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
)

const (
	timeout = time.Second * 30
)

var (
	clusterName   = "test-cluster"
	testFinalizer = "cluster-test.cluster.x-k8s.io"
)

var _ = Describe("Proxmox ClusterReconciler", func() {
	BeforeEach(func() {})
	AfterEach(func() {})

	Context("Reconcile an ProxmoxCluster", func() {
		It("should create a cluster", func() {
			secret := createSecret()

			proxmoxCluster := createProxmoxClusterWithSecret(secret)
			defer func() {
				Expect(testEnv.Cleanup(testEnv.GetContext(), proxmoxCluster)).To(Succeed())
			}()

			// Create the CAPI cluster (owner) object
			capiCluster := createOwnerCluster(proxmoxCluster)
			defer func() {
				Expect(testEnv.Cleanup(testEnv.GetContext(), capiCluster)).To(Succeed())
			}()

			key := client.ObjectKey{Namespace: proxmoxCluster.Namespace, Name: proxmoxCluster.Name}
			Eventually(func() error {
				return testEnv.Get(testEnv.GetContext(), key, proxmoxCluster)
			}, timeout).Should(BeNil())

			By("setting the OwnerRef on the ProxmoxCluster")
			Eventually(func() error {
				ph, err := patch.NewHelper(proxmoxCluster, testEnv)
				Expect(err).ShouldNot(HaveOccurred())
				proxmoxCluster.OwnerReferences = append(proxmoxCluster.OwnerReferences, metav1.OwnerReference{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       capiCluster.Name,
					UID:        "1234",
				})
				return ph.Patch(testEnv.GetContext(), proxmoxCluster, patch.WithStatusObservedGeneration{})
			}, timeout).Should(BeNil())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), key, proxmoxCluster); err != nil {
					return false
				}
				return ctrlutil.ContainsFinalizer(proxmoxCluster, infrav1.ClusterFinalizer)
			}, timeout).Should(BeTrue())

			// checking cluster is setting the ownerRef on the secret
			secretKey := client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}
			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				for _, ref := range secret.OwnerReferences {
					if ref.Name == proxmoxCluster.Name {
						return true
					}
				}
				return false
			}, timeout).Should(BeTrue())

			By("setting the Proxmox ProxmoxClusterReady condition to true")
			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), key, proxmoxCluster); err != nil {
					return false
				}
				return conditions.IsTrue(proxmoxCluster, infrav1.ProxmoxClusterReady)
			}, timeout).Should(BeTrue())
		})

		It("multiple clusters can set ownerRef on secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "secret-",
					Namespace:    "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: infrav1.GroupVersion.String(),
							Kind:       "ProxmoxCluster",
							Name:       "another-cluster",
							UID:        "some-uid",
						},
					},
				},
				Data: map[string][]byte{
					"url":    []byte("url"),
					"token":  []byte("token"),
					"secret": []byte("secret"),
				},
			}
			Expect(testEnv.Create(testEnv.GetContext(), secret)).To(Succeed())

			//  First cluster

			proxmoxCluster1 := &infrav1.ProxmoxCluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "proxmox-test2",
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
					CredentialsRef: &corev1.SecretReference{
						Name:      secret.Name,
						Namespace: secret.Namespace,
					},
				},
			}
			Expect(testEnv.Create(testEnv.GetContext(), proxmoxCluster1)).To(Succeed())
			defer func() {
				Expect(testEnv.Cleanup(testEnv.GetContext(), proxmoxCluster1)).To(Succeed())
			}()

			key1 := client.ObjectKey{Namespace: proxmoxCluster1.Namespace, Name: proxmoxCluster1.Name}
			Eventually(func() error {
				return testEnv.Get(testEnv.GetContext(), key1, proxmoxCluster1)
			}, timeout).Should(BeNil())

			capiCluster1 := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test2-1-",
					Namespace:    "default",
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{
						APIVersion: infrav1.GroupVersion.String(),
						Kind:       "ProxmoxCluster",
						Name:       proxmoxCluster1.Name,
					},
				},
			}
			// Create the CAPI cluster (owner) object
			Expect(testEnv.Create(testEnv.GetContext(), capiCluster1)).To(Succeed())
			defer func() {
				Expect(testEnv.Cleanup(testEnv.GetContext(), capiCluster1)).To(Succeed())
			}()

			//  Second cluster

			proxmoxCluster2 := &infrav1.ProxmoxCluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "proxmox-test2",
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
					CredentialsRef: &corev1.SecretReference{
						Name:      secret.Name,
						Namespace: secret.Namespace,
					},
				},
			}
			Expect(testEnv.Create(testEnv.GetContext(), proxmoxCluster2)).To(Succeed())
			defer func() {
				Expect(testEnv.Cleanup(testEnv.GetContext(), proxmoxCluster2)).To(Succeed())
			}()

			key2 := client.ObjectKey{Namespace: proxmoxCluster2.Namespace, Name: proxmoxCluster2.Name}
			Eventually(func() error {
				return testEnv.Get(testEnv.GetContext(), key2, proxmoxCluster2)
			}, timeout).Should(BeNil())

			capiCluster2 := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test2-2-",
					Namespace:    "default",
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{
						APIVersion: infrav1.GroupVersion.String(),
						Kind:       "ProxmoxCluster",
						Name:       proxmoxCluster2.Name,
					},
				},
			}
			// Create the CAPI cluster (owner) object
			Expect(testEnv.Create(testEnv.GetContext(), capiCluster2)).To(Succeed())
			defer func() {
				Expect(testEnv.Cleanup(testEnv.GetContext(), capiCluster2)).To(Succeed())
			}()

			By("setting the OwnerRef on the ProxmoxCluster")
			Eventually(func() bool {
				ph, err := patch.NewHelper(proxmoxCluster1, testEnv)
				Expect(err).ShouldNot(HaveOccurred())
				proxmoxCluster1.OwnerReferences = append(
					proxmoxCluster1.OwnerReferences,
					metav1.OwnerReference{
						Kind:       "Cluster",
						APIVersion: clusterv1.GroupVersion.String(),
						Name:       capiCluster1.Name,
						UID:        "1234",
					})
				Expect(ph.Patch(testEnv.GetContext(), proxmoxCluster1, patch.WithStatusObservedGeneration{})).ShouldNot(HaveOccurred())
				return true
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				ph, err := patch.NewHelper(proxmoxCluster2, testEnv)
				Expect(err).ShouldNot(HaveOccurred())
				proxmoxCluster2.OwnerReferences = append(
					proxmoxCluster2.OwnerReferences,
					metav1.OwnerReference{
						Kind:       "Cluster",
						APIVersion: clusterv1.GroupVersion.String(),
						Name:       capiCluster2.Name,
						UID:        "4321",
					})
				Expect(ph.Patch(testEnv.GetContext(), proxmoxCluster2, patch.WithStatusObservedGeneration{})).ShouldNot(HaveOccurred())
				return true
			}, timeout).Should(BeTrue())

			// checking cluster is setting the ownerRef on the secret
			secretKey := client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}
			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				for _, ref := range secret.OwnerReferences {
					if ref.Name == proxmoxCluster1.Name {
						return true
					}
				}
				return false
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				for _, ref := range secret.OwnerReferences {
					if ref.Name == proxmoxCluster2.Name {
						return true
					}
				}
				return false
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				for _, ref := range secret.OwnerReferences {
					if ref.Name == "another-cluster" {
						return true
					}
				}
				return false
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				return len(secret.OwnerReferences) == 3
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				return ctrlutil.ContainsFinalizer(secret, infrav1.SecretFinalizer)
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				err := testEnv.Delete(testEnv.GetContext(), proxmoxCluster2)
				return err == nil
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				return len(secret.OwnerReferences) == 2
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				return ctrlutil.ContainsFinalizer(secret, infrav1.SecretFinalizer)
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				err := testEnv.Delete(testEnv.GetContext(), proxmoxCluster1)
				return err == nil
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				return len(secret.OwnerReferences) == 1
			}, timeout).Should(BeTrue())

			Eventually(func() bool {
				if err := testEnv.Get(testEnv.GetContext(), secretKey, secret); err != nil {
					return false
				}
				return ctrlutil.ContainsFinalizer(secret, infrav1.SecretFinalizer)
			}, timeout).Should(BeTrue())
		})
	})

	It("should remove ProxmoxCluster finalizer if the secret does not exist", func() {
		proxmoxCluster := &infrav1.ProxmoxCluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "proxmox-test3",
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
				CredentialsRef: &corev1.SecretReference{
					Name:      "bla",
					Namespace: "bla",
				},
			},
		}
		Expect(testEnv.Create(testEnv.GetContext(), proxmoxCluster)).To(Succeed())

		capiCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test3-",
				Namespace:    "default",
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: &corev1.ObjectReference{
					APIVersion: infrav1.GroupVersion.String(),
					Kind:       "ProxmoxCluster",
					Name:       proxmoxCluster.Name,
				},
			},
		}
		// Create the CAPI cluster (owner) object
		Expect(testEnv.Create(testEnv.GetContext(), capiCluster)).To(Succeed())

		key := client.ObjectKey{Namespace: proxmoxCluster.Namespace, Name: proxmoxCluster.Name}
		Eventually(func() error {
			return testEnv.Get(testEnv.GetContext(), key, proxmoxCluster)
		}, timeout).Should(BeNil())

		Eventually(func() bool {
			if err := testEnv.Get(testEnv.GetContext(), key, proxmoxCluster); err != nil {
				return false
			}
			return !ctrlutil.ContainsFinalizer(proxmoxCluster, infrav1.ClusterFinalizer)
		}, timeout).Should(BeTrue())

		By("deleting the vspherecluster while the secret is gone")
		Eventually(func() bool {
			err := testEnv.Delete(testEnv.GetContext(), proxmoxCluster)
			return err == nil
		}, timeout).Should(BeTrue())

		Eventually(func() bool {
			err := testEnv.Get(testEnv.GetContext(), key, proxmoxCluster)
			return apierrors.IsNotFound(err)
		}, timeout).Should(BeTrue())
	})
})

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
				Paused: false,
				InfrastructureRef: &corev1.ObjectReference{
					Kind:       gvk.Kind,
					Namespace:  testNS,
					Name:       clusterName,
					APIVersion: gvk.GroupVersion().String(),
				},
			},
		}

		controllerutil.AddFinalizer(cl, testFinalizer)
		g.Expect(k8sClient.Create(context.Background(), cl)).To(Succeed())
	})

	AfterEach(func() {
		var cl clusterv1.Cluster
		g.Expect(k8sClient.Get(testEnv.GetContext(), client.ObjectKey{Name: "test", Namespace: testNS}, &cl)).To(Succeed())
		controllerutil.RemoveFinalizer(&cl, testFinalizer)
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
				pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPV4Format)
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
		It("Should successfully create IPAM IPV6 related resources", func() {
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
				pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPV6Format)
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
				pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPV4Format)
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

			pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPV4Format)
			g.Expect(err).ToNot(HaveOccurred())
			// create an IPAddress.
			g.Expect(k8sClient.Create(testEnv.GetContext(), dummyIPAddress(k8sClient, &cl, pool.GetName()))).To(Succeed())

			g.Eventually(func(g Gomega) {
				pool, err := helper.GetDefaultInClusterIPPool(testEnv.GetContext(), infrav1.IPV4Format)
				g.Expect(err).ToNot(HaveOccurred())

				ipAddr, err := helper.GetIPAddress(testEnv.GetContext(), client.ObjectKeyFromObject(&cl))
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(ipAddr).ToNot(BeNil())
				g.Expect(ipAddr.Spec.PoolRef.Name).To(BeEquivalentTo(pool.GetName()))
				g.Expect(ipAddr.Spec.Address).ToNot(BeEmpty())
				g.Expect(ipAddr.Spec.Prefix).To(BeEquivalentTo(pool.Spec.Prefix))
				g.Expect(ipAddr.Spec.Gateway).To(BeEquivalentTo(pool.Spec.Gateway))

				// check controlPlaneEndpoint is updated
				g.Expect(cl.Spec.ControlPlaneEndpoint.IsZero()).NotTo(BeTrue())
				g.Expect(cl.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(ControlPlaneEndpointPort))
				g.Expect(cl.Spec.ControlPlaneEndpoint.Host).To(BeEquivalentTo(ipAddr.Spec.Address))
			}).WithTimeout(time.Second * 10).
				WithPolling(time.Second).
				Should(Succeed())
		})
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

		g.Expect(res.Status.Ready).To(BeTrue())
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
			ClaimRef: corev1.LocalObjectReference{
				Name: owner.GetName(),
			},
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(gvk.GroupVersion().String()),
				Kind:     gvk.Kind,
				Name:     poolName,
			},
			Address: "10.10.10.11",
			Prefix:  24,
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
	Expect(testEnv.Create(testEnv.GetContext(), secret)).To(Succeed())
	return secret
}

func createProxmoxClusterWithSecret(secret *corev1.Secret) *infrav1.ProxmoxCluster {
	proxmoxCluster := &infrav1.ProxmoxCluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "proxmox-test1",
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
			CredentialsRef: &corev1.SecretReference{
				Name:      secret.Name,
				Namespace: secret.Namespace,
			},
		},
	}
	Expect(testEnv.Create(testEnv.GetContext(), proxmoxCluster)).To(Succeed())
	return proxmoxCluster
}

func createOwnerCluster(proxmoxCluster *infrav1.ProxmoxCluster) *clusterv1.Cluster {
	capiCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test1-",
			Namespace:    "default",
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       "ProxmoxCluster",
				Name:       proxmoxCluster.Name,
			},
		},
	}
	Expect(testEnv.Create(testEnv.GetContext(), capiCluster)).To(Succeed())
	return capiCluster
}
