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

// main is the main package for the Cluster API Proxmox Provider.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/utils/env"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	infrastructurev1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/controller"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/webhook"
	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	metricsAddr          string
	enableLeaderElection bool
	enableWebhooks       bool
	probeAddr            string

	// ProxmoxURL env variable that defines the Proxmox host.
	ProxmoxURL string
	// ProxmoxTokenID env variable that defines the Proxmox token id.
	ProxmoxTokenID string
	// ProxmoxSecret env variable that defines the Proxmox secret for the given token id.
	ProxmoxSecret string
)

func init() {
	_ = clusterv1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = infrastructurev1alpha1.AddToScheme(scheme)
	_ = ipamicv1.AddToScheme(scheme)
	_ = ipamv1.AddToScheme(scheme)

	//+kubebuilder:scaffold:scheme
}

func main() {
	ctrl.SetLogger(klog.Background())

	setupLog.Info("starting capmox")
	initFlagsAndEnv(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: metricsAddr},
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "controller-leader-elect-capmox",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Initialize event recorder.
	record.InitFromRecorder(mgr.GetEventRecorderFor("proxmox-controller"))

	setupLog.Info(fmt.Sprintf("feature gates: %+v\n", feature.Gates))

	// Set up the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	pmoxClient, err := setupProxmoxClient(ctx, mgr.GetLogger())
	if err != nil {
		setupLog.Error(err, "unable to setup proxmox API client")
		os.Exit(1)
	}

	if setupErr := setupReconcilers(ctx, mgr, pmoxClient); setupErr != nil {
		setupLog.Error(err, "unable to setup reconcilers")
		os.Exit(1)
	}

	if enableWebhooks {
		if err = (&webhook.ProxmoxCluster{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ProxmoxCluster")
			os.Exit(1)
		}
		if err = (&webhook.ProxmoxMachine{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ProxmoxMachine")
			os.Exit(1)
		}
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager, client capmox.Client) error {
	if err := (&controller.ProxmoxClusterReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("proxmoxcluster-controller"),
		ProxmoxClient: client,
	}).SetupWithManager(ctx, mgr); err != nil {
		return fmt.Errorf("setting up ProxmoxCluster controller: %w", err)
	}
	if err := (&controller.ProxmoxMachineReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("proxmoxmachine-controller"),
		ProxmoxClient: client,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up ProxmoxMachine controller: %w", err)
	}

	return nil
}

func setupProxmoxClient(ctx context.Context, logger logr.Logger) (capmox.Client, error) {
	// we return nil if the env variables are not set
	// so the proxmoxcontroller can create the client later from spec.credentialsRef
	// or fail the cluster if no credentials found
	if ProxmoxURL == "" || ProxmoxTokenID == "" || ProxmoxSecret == "" {
		return nil, nil
	}
	// TODO, check if we need to delete tls config
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	httpClient := &http.Client{Transport: tr}
	return goproxmox.NewAPIClient(ctx, logger, ProxmoxURL,
		proxmox.WithHTTPClient(httpClient),
		proxmox.WithAPIToken(ProxmoxTokenID, ProxmoxSecret),
	)
}

func initFlagsAndEnv(fs *pflag.FlagSet) {
	klog.InitFlags(nil)

	ProxmoxURL = env.GetString("PROXMOX_URL", "")
	ProxmoxTokenID = env.GetString("PROXMOX_TOKEN", "")
	ProxmoxSecret = env.GetString("PROXMOX_SECRET", "")

	fs.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	fs.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.BoolVar(&enableWebhooks, "enable-webhooks", true,
		"If true, run webhook server alongside manager")

	feature.MutableGates.AddFlag(fs)
}
