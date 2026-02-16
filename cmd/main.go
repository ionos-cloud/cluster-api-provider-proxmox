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

// main is the main package for the Cluster API Proxmox Provider.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/utils/env"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/flags"
	"sigs.k8s.io/cluster-api/util/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/controller"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/tlshelper"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/webhook"
	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	enableLeaderElection        bool
	leaderElectionLeaseDuration time.Duration
	leaderElectionRenewDeadline time.Duration
	leaderElectionRetryPeriod   time.Duration
	enableWebhooks              bool
	probeAddr                   string
	managerOptions              = flags.ManagerOptions{}

	// ProxmoxURL env variable that defines the Proxmox host.
	ProxmoxURL string
	// ProxmoxTokenID env variable that defines the Proxmox token id.
	ProxmoxTokenID string
	// ProxmoxSecret env variable that defines the Proxmox secret for the given token id.
	ProxmoxSecret string

	proxmoxInsecure     bool
	proxmoxRootCertFile string
)

func init() {
	_ = clusterv1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)
	_ = infrav1alpha1.AddToScheme(scheme)
	_ = ipamicv1.AddToScheme(scheme)
	_ = ipamv1.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

func main() {
	ctrl.SetLogger(klog.Background())

	setupLog.Info("starting capmox")
	initFlagsAndEnv(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	_, metricsOptions, err := flags.GetManagerOptions(managerOptions)
	if err != nil {
		setupLog.Error(err, "Unable to start manager: invalid flags")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: *metricsOptions,
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaseDuration:          &leaderElectionLeaseDuration,
		RenewDeadline:          &leaderElectionRenewDeadline,
		RetryPeriod:            &leaderElectionRetryPeriod,
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

	// TODO: do I need this?
	cache := mgr.GetCache()

	indexFunc := func(obj client.Object) []string {
		return []string{obj.(*ipamv1.IPAddress).Spec.PoolRef.Name}
	}

	if err = cache.IndexField(ctx, &ipamv1.IPAddress{}, "spec.poolRef.name", indexFunc); err != nil {
		panic(err)
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
		if err = (&webhook.ProxmoxMachineTemplate{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ProxmoxMachineTemplate")
			os.Exit(1)
		}
		if err = (&webhook.ProxmoxClusterTemplate{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ProxmoxClusterTemplate")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

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

func setupReconcilers(ctx context.Context, mgr ctrl.Manager, proxmoxClient capmox.Client) error {
	if err := (&controller.ProxmoxClusterReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("proxmoxcluster-controller"),
		ProxmoxClient: proxmoxClient,
	}).SetupWithManager(ctx, mgr); err != nil {
		return fmt.Errorf("setting up ProxmoxCluster controller: %w", err)
	}
	if err := (&controller.ProxmoxMachineReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("proxmoxmachine-controller"),
		ProxmoxClient: proxmoxClient,
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

	rootCerts, err := tlshelper.SystemRootsWithFile(proxmoxRootCertFile)
	if err != nil {
		return nil, fmt.Errorf("loading cert pool: %w", err)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: proxmoxInsecure, // #nosec:G402 // Default retained, user can enable cert checking
			RootCAs:            rootCerts,
		},
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

	fs.BoolVar(&proxmoxInsecure, "proxmox-insecure",
		env.GetString("PROXMOX_INSECURE", "true") == "true",
		"Skip TLS verification when connecting to Proxmox")
	fs.StringVar(&proxmoxRootCertFile, "proxmox-root-cert-file", "",
		"Root-Certificate to use to verify server TLS certificate")

	fs.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.DurationVar(&leaderElectionLeaseDuration, "leader-elect-lease-duration", 15*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")
	fs.DurationVar(&leaderElectionRenewDeadline, "leader-elect-renew-deadline", 10*time.Second,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)")
	fs.DurationVar(&leaderElectionRetryPeriod, "leader-elect-retry-period", 2*time.Second,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")
	fs.BoolVar(&enableWebhooks, "enable-webhooks", true,
		"If true, run webhook server alongside manager")

	flags.AddManagerOptions(fs, &managerOptions)

	feature.MutableGates.AddFlag(fs)
}
