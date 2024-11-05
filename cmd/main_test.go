package main

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	infrastructurev1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
)

func TestSetupReconcilers(t *testing.T) {
	proxmoxClient := proxmoxtest.NewMockClient(t)

	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, clusterv1.AddToScheme(s))
	require.NoError(t, infrastructurev1alpha1.AddToScheme(s))
	require.NoError(t, ipamicv1.AddToScheme(s))
	require.NoError(t, ipamv1.AddToScheme(s))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: s})
	require.NoError(t, err)
	require.NotNil(t, mgr)

	err = setupReconcilers(context.Background(), mgr, proxmoxClient)
	require.NoError(t, err)
}

func TestSetupProxmoxClient_NoClient(t *testing.T) {
	// No client should be returned if the ProxmoxURL is not set
	ProxmoxURL = ""
	cl, err := setupProxmoxClient(context.Background(), logr.Discard())
	require.NoError(t, err)
	require.Nil(t, cl)
}

func TestInitFlagsAndEnv(t *testing.T) {
	// Test that the flags are initialized
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Test that the flags are overridden by environment variables
	err := os.Setenv("PROXMOX_URL", "https://example.com")
	require.NoError(t, err)
	err = os.Setenv("PROXMOX_TOKEN", "root@pam")
	require.NoError(t, err)
	err = os.Setenv("PROXMOX_SECRET", "password")
	require.NoError(t, err)

	initFlagsAndEnv(&pflag.FlagSet{})
	require.Equal(t, "https://example.com", ProxmoxURL)
	require.Equal(t, "root@pam", ProxmoxTokenID)
	require.Equal(t, "password", ProxmoxSecret)
}
