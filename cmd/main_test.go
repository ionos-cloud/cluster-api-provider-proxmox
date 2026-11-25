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

package main

import (
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ipamicv1 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/goproxmox"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox/proxmoxtest"
)

func TestSetupReconcilers(t *testing.T) {
	proxmoxClient := proxmoxtest.NewMockClient(t)

	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, clusterv1.AddToScheme(s))
	require.NoError(t, infrav1.AddToScheme(s))
	require.NoError(t, ipamicv1.AddToScheme(s))
	require.NoError(t, ipamv1.AddToScheme(s))

	c := mockGetConfig(s)
	require.NotNil(t, c)

	mgr, err := ctrl.NewManager(c, ctrl.Options{Scheme: s})
	require.NoError(t, err)
	require.NotNil(t, mgr)

	err = setupReconcilers(context.Background(), mgr, proxmoxClient)
	require.NoError(t, err)
}

func mockGetConfig(s *runtime.Scheme) *rest.Config {
	// Return a basic rest.Config, here we use empty values for fields since we're not connecting to a real cluster
	return &rest.Config{
		Host:    "http://localhost:8080",
		APIPath: "api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schema.GroupVersion{Version: "v1"},
			NegotiatedSerializer: serializer.WithoutConversionCodecFactory{CodecFactory: serializer.NewCodecFactory(s)},
		},
	}
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
	t.Setenv("PROXMOX_URL", "https://example.com")
	t.Setenv("PROXMOX_TOKEN", "root@pam")
	t.Setenv("PROXMOX_SECRET", "password")
	t.Setenv("PROXMOX_INSECURE", "No")
	t.Setenv("PROXMOX_ENDPOINT_DISCOVERY", " yes ")
	t.Setenv("PROXMOX_REQUEST_TIMEOUT", "45s")

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	require.NoError(t, initFlagsAndEnv(fs))
	require.Equal(t, "https://example.com", ProxmoxURL)
	require.Equal(t, "root@pam", ProxmoxTokenID)
	require.Equal(t, "password", ProxmoxSecret)
	require.False(t, proxmoxInsecure)
	require.True(t, ProxmoxEndpointDiscovery)
	require.Equal(t, 45*time.Second, proxmoxRequestTimeout)

	// The flag accepts the same spellings as the variable and overrides it.
	require.NoError(t, fs.Parse([]string{"--proxmox-insecure=on", "--proxmox-request-timeout=1m"}))
	require.True(t, proxmoxInsecure)
	require.Equal(t, time.Minute, proxmoxRequestTimeout)
	require.NoError(t, fs.Parse([]string{"--proxmox-insecure=F"}))
	require.False(t, proxmoxInsecure)
	require.NoError(t, fs.Parse([]string{"--proxmox-insecure"}))
	require.True(t, proxmoxInsecure)
	require.ErrorContains(t, fs.Parse([]string{"--proxmox-insecure=maybe"}), "invalid boolean value")
}

func TestInitFlagsAndEnv_Defaults(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	for _, name := range []string{"PROXMOX_INSECURE", "PROXMOX_ENDPOINT_DISCOVERY", "PROXMOX_REQUEST_TIMEOUT"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}

	require.NoError(t, initFlagsAndEnv(pflag.NewFlagSet("test", pflag.ContinueOnError)))
	require.True(t, proxmoxInsecure, "insecure stays the default when PROXMOX_INSECURE is unset")
	require.False(t, ProxmoxEndpointDiscovery)
	require.Equal(t, goproxmox.RequestTimeout, proxmoxRequestTimeout)
}

func TestInitFlagsAndEnv_InvalidValues(t *testing.T) {
	tests := []struct {
		name, value, wantErr string
	}{
		{"PROXMOX_INSECURE", "", "PROXMOX_INSECURE"},
		{"PROXMOX_INSECURE", "maybe", "PROXMOX_INSECURE"},
		{"PROXMOX_INSECURE", "2", "invalid boolean value"},
		{"PROXMOX_ENDPOINT_DISCOVERY", "ja", "PROXMOX_ENDPOINT_DISCOVERY"},
		{"PROXMOX_REQUEST_TIMEOUT", "soon", "PROXMOX_REQUEST_TIMEOUT"},
		{"PROXMOX_REQUEST_TIMEOUT", "-1s", "must not be negative"},
	}
	for _, tc := range tests {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			t.Setenv(tc.name, tc.value)
			err := initFlagsAndEnv(pflag.NewFlagSet("test", pflag.ContinueOnError))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestInitFlagsAndEnv_InsecureMatrix pins that the variable, the flag and the Secret key
// (goproxmox.ParseBool) agree on every value.
func TestInitFlagsAndEnv_InsecureMatrix(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"true", true}, {"True", true}, {"TRUE", true}, {"tRuE", true}, {"t", true}, {"T", true}, {"1", true},
		{"yes", true}, {"YES", true}, {"y", true}, {"on", true}, {"On", true}, {" true", true}, {"true\n", true},
		{"false", false}, {"False", false}, {"FALSE", false}, {"f", false}, {"F", false}, {"0", false},
		{"no", false}, {"NO", false}, {"n", false}, {"off", false}, {"Off", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			t.Setenv("PROXMOX_INSECURE", tc.value)
			require.NoError(t, initFlagsAndEnv(pflag.NewFlagSet("test", pflag.ContinueOnError)))
			require.Equal(t, tc.want, proxmoxInsecure, "env")

			var flagValue boolValue
			require.NoError(t, flagValue.Set(tc.value))
			require.Equal(t, tc.want, bool(flagValue), "flag")

			secretValue, err := goproxmox.ParseBool(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, secretValue, "secret")
		})
	}
}

func TestSetupProxmoxClient_UsesSharedHTTPClient(t *testing.T) {
	// A Proxmox API that never answers must not block startup: the shared HTTP client
	// carries the response-header timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ProxmoxURL = srv.URL
	ProxmoxTokenID = "root@pam!capi"
	ProxmoxSecret = "secret"
	ProxmoxAdditionalURLs = ""
	ProxmoxEndpointDiscovery = false
	t.Cleanup(func() { ProxmoxURL, ProxmoxTokenID, ProxmoxSecret = "", "", "" })

	saved := goproxmox.RequestTimeout
	goproxmox.RequestTimeout = 200 * time.Millisecond
	t.Cleanup(func() { goproxmox.RequestTimeout = saved })

	start := time.Now()
	cl, err := setupProxmoxClient(context.Background(), logr.Discard())
	require.Error(t, err)
	require.Nil(t, cl)
	require.ErrorContains(t, err, "unable to initialize proxmox api client")
	require.Less(t, time.Since(start), 5*time.Second)
}
