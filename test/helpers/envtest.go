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

// Package helpers provides helper functions to run integration tests
// by pre-populating the required settings for envtest and loading
// required crds from different modules
package helpers

import (
	"context"
	"fmt"
	"path/filepath"
	goruntime "runtime"

	"golang.org/x/tools/go/packages"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(ipamicv1.AddToScheme(scheme))
	utilruntime.Must(ipamv1.AddToScheme(scheme))
	utilruntime.Must(admissionv1.AddToScheme(scheme))
}

// WebhookConfig contains host and port for the env test
// webhook serving address.
type WebhookConfig struct {
	Host string
	Port int
}

// TestEnvironment is used to wrap the testing setup for integration tests.
type TestEnvironment struct {
	manager.Manager
	client.Client
	Config        *rest.Config
	ProxmoxClient proxmox.Client
	WebhookConfig WebhookConfig

	env    *envtest.Environment
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTestEnvironment creates a new testing environment with a
// pre-configured manager, that can be used to register reconcilers.
func NewTestEnvironment(setupWebhook bool, pmClient proxmox.Client) *TestEnvironment {
	_, filename, _, ok := goruntime.Caller(0)
	if !ok {
		klog.Fatalf("Failed to get information for current file from runtime")
	}

	root := filepath.Dir(filename)

	crdsPaths := []string{
		filepath.Join(root, "..", "..", "config", "crd", "bases"),
	}

	if capiPaths := loadCRDsFromDependentModules(); capiPaths != nil {
		crdsPaths = append(crdsPaths, capiPaths...)
	}

	env := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     crdsPaths,
	}
	apiServer := env.ControlPlane.GetAPIServer()
	apiServer.Configure().Set("disable-admission-plugins", "NamespaceLifecycle")

	if setupWebhook {
		env.WebhookInstallOptions = envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join(root, "..", "..", "config", "webhook")},
		}
	}
	if _, err := env.Start(); err != nil {
		err = kerrors.NewAggregate([]error{err, env.Stop()})
		panic(err)
	}

	opts := ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
	}

	whio := &env.WebhookInstallOptions
	if setupWebhook {
		opts.WebhookServer = webhook.NewServer(webhook.Options{
			Host:    whio.LocalServingHost,
			Port:    whio.LocalServingPort,
			CertDir: whio.LocalServingCertDir,
		})
	}

	mgr, err := ctrl.NewManager(env.Config, opts)
	if err != nil {
		panic(fmt.Errorf("failed to create a new manager: %w", err))
	}

	return &TestEnvironment{
		env:           env,
		Manager:       mgr,
		Client:        mgr.GetClient(),
		Config:        mgr.GetConfig(),
		ProxmoxClient: pmClient,
		WebhookConfig: WebhookConfig{
			Host: whio.LocalServingHost,
			Port: whio.LocalServingPort,
		},
	}
}

// GetContext returns the context of the test environment.
// This context will be initialized once `StartManager` was called.
func (t *TestEnvironment) GetContext() context.Context {
	return t.ctx
}

// StartManager starts the manager.
func (t *TestEnvironment) StartManager(ctx context.Context) error {
	t.ctx, t.cancel = context.WithCancel(ctx)
	return t.Manager.Start(t.ctx)
}

// Cleanup removes objects from the TestEnvironment.
func (t *TestEnvironment) Cleanup(ctx context.Context, objs ...client.Object) error {
	errs := make([]error, 0, len(objs))
	for _, o := range objs {
		err := t.Client.Delete(ctx, o)
		if apierrors.IsNotFound(err) {
			// If the object is not found, it must've been garbage collected
			// already. For example, if we delete namespace first and then
			// objects within it.
			continue
		}
		errs = append(errs, err)
	}
	return kerrors.NewAggregate(errs)
}

// Stop shuts down the test environment and stops the manager.
func (t *TestEnvironment) Stop() error {
	t.cancel()
	return t.env.Stop()
}

func loadCRDsFromDependentModules() []string {
	// cluster api
	config := &packages.Config{
		Mode: packages.NeedModule,
	}

	clusterAPIPkgs, err := packages.Load(config, "sigs.k8s.io/cluster-api")
	if err != nil {
		return nil
	}

	// IPAM provider
	ipamPkgs, err := packages.Load(config, "sigs.k8s.io/cluster-api-ipam-provider-in-cluster")
	if err != nil {
		return nil
	}

	clusterAPIDir := clusterAPIPkgs[0].Module.Dir
	ipamDir := ipamPkgs[0].Module.Dir

	return []string{
		// cluster api crds
		filepath.Join(clusterAPIDir, "config", "crd", "bases"),
		filepath.Join(clusterAPIDir, "controlplane", "kubeadm", "config", "crd", "bases"),

		// ipam crds
		filepath.Join(ipamDir, "config", "crd", "bases"),
	}
}
