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
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
)

func TestParseBool(t *testing.T) {
	trueValues := []string{"true", "True", "TRUE", "tRuE", "t", "T", "1", "yes", "Yes", "YES", "y", "Y", "on", "ON", " true", "true\n", "\ttrue "}
	for _, v := range trueValues {
		got, err := ParseBool(v)
		require.NoError(t, err, "%q", v)
		require.True(t, got, "%q", v)
	}

	falseValues := []string{"false", "False", "FALSE", "fAlSe", "f", "F", "0", "no", "No", "NO", "n", "N", "off", "OFF", " false ", "false\n"}
	for _, v := range falseValues {
		got, err := ParseBool(v)
		require.NoError(t, err, "%q", v)
		require.False(t, got, "%q", v)
	}

	invalid := []string{"", " ", "2", "ja", "maybe", "truee", "yes please", "null", "~"}
	for _, v := range invalid {
		_, err := ParseBool(v)
		require.Error(t, err, "%q", v)
		require.ErrorContains(t, err, "invalid boolean value")
	}
}

func TestDefaultTimeouts_FollowRequestTimeout(t *testing.T) {
	saved := RequestTimeout
	t.Cleanup(func() { RequestTimeout = saved })

	RequestTimeout = 7 * time.Second
	got := DefaultTimeouts()
	require.Equal(t, 7*time.Second, got.Request)
	require.Equal(t, 10*time.Second, got.Dial)
	require.Equal(t, 10*time.Second, got.TLSHandshake)
	require.Equal(t, 30*time.Second, got.ResponseHeader)
}

func TestNewHTTPClient_TLSAndTimeouts(t *testing.T) {
	client := NewHTTPClient(HTTPClientConfig{InsecureSkipVerify: true, Timeouts: DefaultTimeouts()})
	tr, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.True(t, tr.TLSClientConfig.InsecureSkipVerify)
	require.Nil(t, tr.TLSClientConfig.RootCAs)
	require.Equal(t, uint16(tls.VersionTLS12), tr.TLSClientConfig.MinVersion)
	require.Equal(t, 10*time.Second, tr.TLSHandshakeTimeout)
	require.Equal(t, 30*time.Second, tr.ResponseHeaderTimeout)
	require.Equal(t, RequestTimeout, client.Timeout)

	secure := NewHTTPClient(HTTPClientConfig{})
	require.False(t, secure.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	require.Zero(t, secure.Timeout)
}

// hangingServer accepts requests and never writes response headers until the request is
// abandoned, like a node whose pveproxy stopped answering.
func hangingServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNewHTTPClient_ResponseHeaderTimeout(t *testing.T) {
	srv := hangingServer(t)
	client := NewHTTPClient(HTTPClientConfig{Timeouts: Timeouts{Dial: time.Second, ResponseHeader: 100 * time.Millisecond}})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api2/json/version", nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	require.ErrorContains(t, err, "timeout awaiting response headers")
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestNewHTTPClient_RequestTimeout(t *testing.T) {
	srv := hangingServer(t)
	client := NewHTTPClient(HTTPClientConfig{Timeouts: Timeouts{Request: 100 * time.Millisecond}})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api2/json/version", nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	require.ErrorContains(t, err, "Client.Timeout exceeded")
	require.Less(t, time.Since(start), 5*time.Second)
}

// The endpoint discovery probe is built from the same options as the client proper, so it
// must honour the same timeouts: a hanging node must not block controller startup.
func TestDiscoverClusterEndpoints_HangingNodeTimesOut(t *testing.T) {
	srv := hangingServer(t)
	httpClient := NewHTTPClient(HTTPClientConfig{Timeouts: Timeouts{Dial: time.Second, ResponseHeader: 100 * time.Millisecond}})

	start := time.Now()
	_, err := discoverClusterEndpoints(context.Background(), srv.URL, proxmox.WithHTTPClient(httpClient))
	require.Error(t, err)
	require.ErrorContains(t, err, "endpoint discovery")
	require.Less(t, time.Since(start), 5*time.Second)
}
