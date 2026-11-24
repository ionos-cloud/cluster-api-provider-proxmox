/*
Copyright 2026 IONOS Cloud.

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

package scope

import (
	"net/http"
	"sync"

	"k8s.io/apimachinery/pkg/types"

	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
)

// credentialsClients holds one Proxmox client per credentials Secret.
//
// A ClusterScope is built for every reconcile. Building its client each time also built a
// new http.Transport each time, and a Transport keeps its idle keep-alive connections open
// until it is closed, so every reconcile leaked a connection and its goroutines.
var credentialsClients = newClientCache()

type clientCacheEntry struct {
	resourceVersion string
	client          capmox.Client
	transport       *http.Transport
}

type clientCache struct {
	mu      sync.Mutex
	entries map[types.NamespacedName]clientCacheEntry
}

func newClientCache() *clientCache {
	return &clientCache{entries: map[types.NamespacedName]clientCacheEntry{}}
}

// get returns the client built from the Secret at resourceVersion, building it with create
// if there is none. A client built from an older version of the Secret is replaced and its
// idle connections are closed.
func (c *clientCache) get(secret types.NamespacedName, resourceVersion string,
	create func() (capmox.Client, *http.Transport, error),
) (capmox.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[secret]; ok {
		if entry.resourceVersion == resourceVersion {
			return entry.client, nil
		}
		entry.transport.CloseIdleConnections()
		delete(c.entries, secret)
	}

	pmoxClient, transport, err := create()
	if err != nil {
		return nil, err
	}
	c.entries[secret] = clientCacheEntry{
		resourceVersion: resourceVersion,
		client:          pmoxClient,
		transport:       transport,
	}
	return pmoxClient, nil
}
