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

package goproxmox

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jarcoal/httpmock"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
)

func TestClusterEndpoints(t *testing.T) {
	nodes := proxmox.NodeStatuses{
		{Name: "pve3", IP: "10.0.0.3", Online: 1},
		{Name: "pve1", IP: "10.0.0.1", Online: 1}, // the primary itself
		{Name: "pve2", IP: "10.0.0.2", Online: 1},
		{Name: "pve4", IP: "10.0.0.4", Online: 0}, // offline
		{Name: "pve5", Online: 1},                 // no address known
		nil,
	}

	got, err := clusterEndpoints("https://10.0.0.1:8006", nodes)
	require.NoError(t, err)
	require.Equal(t, []string{"https://10.0.0.2:8006", "https://10.0.0.3:8006"}, got)

	got, err = clusterEndpoints("https://pve.example", nodes)
	require.NoError(t, err)
	require.Equal(t, []string{"https://10.0.0.1:8006", "https://10.0.0.2:8006", "https://10.0.0.3:8006"}, got,
		"a primary named by host name cannot be matched to a node IP and is kept; the client ignores a duplicate")

	got, err = clusterEndpoints("http://10.0.0.1:9006/api2/json", nodes)
	require.NoError(t, err)
	require.Equal(t, []string{"http://10.0.0.2:9006", "http://10.0.0.3:9006"}, got, "scheme and port follow the primary")

	_, err = clusterEndpoints("not a url", nodes)
	require.Error(t, err)
}

func TestAPIBaseURL(t *testing.T) {
	got, err := apiBaseURL(" https://pve2:8006/api2/json ")
	require.NoError(t, err)
	require.Equal(t, "https://pve2:8006/api2/json", got, "surrounding whitespace is trimmed")
	for in, want := range map[string]string{
		"https://pve2:8006":            "https://pve2:8006/api2/json",
		"https://pve2:8006/":           "https://pve2:8006/api2/json",
		"https://pve2:8006/api2/json/": "https://pve2:8006/api2/json",
	} {
		got, err := apiBaseURL(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "pve2:8006", "/api2/json"} {
		_, err := apiBaseURL(in)
		require.Error(t, err, in)
	}
}

func TestParseEndpointList(t *testing.T) {
	require.Nil(t, ParseEndpointList(""))
	require.Nil(t, ParseEndpointList(" , "))
	require.Equal(t, []string{"https://a:8006", "https://b:8006"}, ParseEndpointList(" https://a:8006, https://b:8006 ,"))
}

func TestNewAPIClientWithEndpoints_Discovery(t *testing.T) {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)

	httpmock.RegisterResponder(http.MethodGet, testBaseURL+"api2/json/cluster/status",
		httpmock.NewJsonResponderOrPanic(200, map[string]any{"data": []map[string]any{
			{"type": "cluster", "name": "lab", "quorate": 1, "version": 3},
			{"type": "node", "name": "pve1", "ip": "10.0.0.1", "online": 1, "local": 1},
			{"type": "node", "name": "pve2", "ip": "10.0.0.2", "online": 1},
			{"type": "node", "name": "pve3", "ip": "10.0.0.3", "online": 0},
		}}))
	httpmock.RegisterResponder(http.MethodGet, testBaseURL+"api2/json/version",
		newJSONResponder(200, proxmox.Version{Release: "test"}))

	client, err := NewAPIClientWithEndpoints(context.Background(), logr.Discard(), testBaseURL,
		Endpoints{Discover: true, Additional: []string{"https://pve9:8006"}})
	require.NoError(t, err)
	require.NotNil(t, client)

	info := httpmock.GetCallCountInfo()
	require.Equal(t, 1, info["GET "+testBaseURL+"api2/json/cluster/status"], "discovery reads the cluster status once")
	require.Equal(t, 1, info["GET "+testBaseURL+"api2/json/version"], "the client is initialised once")
}

func TestNewAPIClientWithEndpoints_NoEndpointsBehavesLikeNewAPIClient(t *testing.T) {
	client := newTestClient(t)
	require.NotNil(t, client)

	httpmock.RegisterResponder(http.MethodGet, testBaseURL+"api2/json/version",
		newJSONResponder(200, proxmox.Version{Release: "test"}))
	c2, err := NewAPIClientWithEndpoints(context.Background(), logr.Discard(), testBaseURL, Endpoints{})
	require.NoError(t, err)
	require.NotNil(t, c2)
	require.Equal(t, 0, httpmock.GetCallCountInfo()["GET "+testBaseURL+"api2/json/cluster/status"])
}

func TestNewAPIClientWithEndpoints_InvalidAdditionalURL(t *testing.T) {
	_, err := NewAPIClientWithEndpoints(context.Background(), logr.Discard(), testBaseURL,
		Endpoints{Additional: []string{"pve2:8006"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid additional proxmox URL")
}
