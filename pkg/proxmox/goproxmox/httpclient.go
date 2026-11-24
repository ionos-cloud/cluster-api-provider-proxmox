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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// RequestTimeout bounds every request of a Proxmox API client built with NewHTTPClient:
// connection, TLS handshake, headers and body together. It is the process-wide default,
// set once at startup from --proxmox-request-timeout / PROXMOX_REQUEST_TIMEOUT, and
// DefaultTimeouts reads it. Zero means no overall bound; the dial, TLS handshake and
// response-header timeouts still apply then. The cloud-init ISO upload is the longest
// request the controller makes, so the default is generous.
var RequestTimeout = 2 * time.Minute

// Timeouts are the per-phase bounds of a Proxmox API HTTP client. A field left zero
// disables that bound.
type Timeouts struct {
	// Dial bounds establishing the TCP connection.
	Dial time.Duration
	// TLSHandshake bounds the TLS handshake on a new connection.
	TLSHandshake time.Duration
	// ResponseHeader bounds the wait for the response headers once the request, body
	// included, has been written. It is what turns a wedged node into an error instead
	// of a reconcile worker blocked for good.
	ResponseHeader time.Duration
	// Request bounds the whole request, response body included.
	Request time.Duration
}

// DefaultTimeouts returns the bounds the controller uses unless configured otherwise.
func DefaultTimeouts() Timeouts {
	return Timeouts{
		Dial:           10 * time.Second,
		TLSHandshake:   10 * time.Second,
		ResponseHeader: 30 * time.Second,
		Request:        RequestTimeout,
	}
}

// HTTPClientConfig describes the HTTP client NewHTTPClient builds.
type HTTPClientConfig struct {
	// InsecureSkipVerify disables verification of the Proxmox API's TLS certificate.
	InsecureSkipVerify bool
	// RootCAs verifies the Proxmox API's certificate; nil means the system roots.
	RootCAs *x509.CertPool
	// Timeouts bounds the requests; the zero value disables every bound, so callers
	// usually start from DefaultTimeouts.
	Timeouts Timeouts
}

// NewHTTPClient is the one constructor for the HTTP client behind every Proxmox API client
// of the controller, whether its credentials come from the environment or from a
// ProxmoxCluster's credentials Secret. Both paths get the same TLS handling and the same
// timeouts; go-proxmox's endpoint discovery probe receives this client too.
func NewHTTPClient(cfg HTTPClientConfig) *http.Client {
	dialer := &net.Dialer{
		Timeout:   cfg.Timeouts.Dial,
		KeepAlive: 30 * time.Second,
	}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   cfg.Timeouts.TLSHandshake,
		ResponseHeaderTimeout: cfg.Timeouts.ResponseHeader,
		ExpectContinueTimeout: time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.InsecureSkipVerify, // #nosec:G402 // Default retained, user can enable cert checking
			RootCAs:            cfg.RootCAs,
			MinVersion:         tls.VersionTLS12,
		},
	}
	return &http.Client{
		Transport: tr,
		Timeout:   cfg.Timeouts.Request,
	}
}

// ParseBool reads a true/false setting the same way wherever the controller takes one:
// the --proxmox-insecure flag, the PROXMOX_INSECURE and PROXMOX_ENDPOINT_DISCOVERY
// variables and the insecure and endpoint_discovery keys of a credentials Secret. It
// accepts, in any case and with surrounding white space, the spellings of
// strconv.ParseBool (1, 0, t, f, true, false) and the YAML-style words yes, no, y, n, on
// and off. Everything else, the empty string included, is an error, so that a value
// that was never a boolean cannot silently pick one side.
func ParseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes", "on":
		return true, nil
	case "0", "f", "false", "n", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean value %q: use true or false (1/0, t/f, yes/no, y/n and on/off are accepted too)", s)
}
