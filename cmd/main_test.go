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
	"os"
	"testing"

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
