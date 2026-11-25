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
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
)

// Endpoints names the Proxmox API endpoints a client may use besides its primary URL.
//
// Every node of a quorate Proxmox cluster serves the whole cluster-wide API and forwards
// node-specific calls to the node that owns them, and API tokens are cluster-wide. A client
// given only one node's URL is therefore tied to that node: when it is down, or when it cannot
// reach the node a request is about, the request fails although the rest of the cluster could
// have served it. With more endpoints the client fails over to another node for such requests
// (see proxmox.WithAdditionalEndpoints for the exact rules).
type Endpoints struct {
	// Additional lists further API base URLs of the same cluster, such as
	// "https://pve2:8006". The path is optional; "/api2/json" is appended when missing.
	Additional []string
	// Discover adds the API of every online cluster node, read from /cluster/status through
	// the primary URL and addressed by the node's cluster IP at the primary's scheme and
	// port. The token needs Sys.Audit on / for the status call; without it nothing is
	// discovered and the client works as before.
	Discover bool
}

// ParseEndpointList splits a comma separated list of URLs, dropping empty entries.
func ParseEndpointList(s string) []string {
	var out []string
	for _, u := range strings.Split(s, ",") {
		if u = strings.TrimSpace(u); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// NewAPIClientWithEndpoints is NewAPIClient with the endpoints of eps registered as failover
// targets. With neither additional URLs nor discovery it behaves exactly like NewAPIClient.
func NewAPIClientWithEndpoints(ctx context.Context, logger logr.Logger, baseURL string, eps Endpoints, options ...proxmox.Option) (*APIClient, error) {
	extra := slices.Clone(eps.Additional)
	if eps.Discover {
		discovered, err := discoverClusterEndpoints(ctx, baseURL, options...)
		if err != nil {
			return nil, err
		}
		if len(discovered) == 0 {
			logger.Info("Proxmox endpoint discovery found no other online nodes; the token may lack Sys.Audit on /, or this is a single node")
		} else {
			logger.Info("Proxmox endpoints discovered", "endpoints", discovered)
		}
		extra = append(extra, discovered...)
	}

	apiURLs := make([]string, 0, len(extra))
	for _, u := range extra {
		apiURL, err := apiBaseURL(u)
		if err != nil {
			return nil, fmt.Errorf("invalid additional proxmox URL %q: %w", u, err)
		}
		apiURLs = append(apiURLs, apiURL)
	}
	if len(apiURLs) > 0 {
		options = append(options, proxmox.WithAdditionalEndpoints(apiURLs...))
	}
	return NewAPIClient(ctx, logger, baseURL, options...)
}

// discoverClusterEndpoints reads /cluster/status through the primary URL and returns the API
// base URL of every other online node.
func discoverClusterEndpoints(ctx context.Context, baseURL string, options ...proxmox.Option) ([]string, error) {
	apiURL, err := apiBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxmox base URL %q: %w", baseURL, err)
	}
	probe := proxmox.NewClient(apiURL, options...)
	cluster, err := probe.Cluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to read the proxmox cluster status for endpoint discovery: %w", err)
	}
	return clusterEndpoints(baseURL, cluster.Nodes)
}

// clusterEndpoints turns the node entries of /cluster/status into API base URLs: one per
// online node with a known IP, at the primary's scheme and port, the primary's own host left
// out, sorted by node name.
func clusterEndpoints(primary string, nodes proxmox.NodeStatuses) ([]string, error) {
	pu, err := url.Parse(primary)
	if err != nil || pu.Scheme == "" || pu.Host == "" {
		return nil, fmt.Errorf("invalid proxmox base URL %q", primary)
	}
	port := pu.Port()
	if port == "" {
		port = "8006"
	}
	primaryHost := pu.Hostname()

	sorted := slices.DeleteFunc(slices.Clone(nodes), func(n *proxmox.NodeStatus) bool { return n == nil })
	slices.SortFunc(sorted, func(a, b *proxmox.NodeStatus) int { return strings.Compare(a.Name, b.Name) })

	var out []string
	for _, n := range sorted {
		if n.Online != 1 || n.IP == "" || n.IP == primaryHost {
			continue
		}
		u := url.URL{Scheme: pu.Scheme, Host: net.JoinHostPort(n.IP, port)}
		out = append(out, u.String())
	}
	return out, nil
}

// apiBaseURL returns u with the "/api2/json" path the client speaks, added when absent.
func apiBaseURL(u string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(u))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("scheme and host are required")
	}
	if strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/api2/json") {
		return strings.TrimRight(parsed.String(), "/"), nil
	}
	return url.JoinPath(parsed.String(), "api2", "json")
}
