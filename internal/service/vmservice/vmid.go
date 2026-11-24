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

package vmservice

import (
	"context"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

// Proxmox hands out VMIDs with GET /cluster/nextid, which keeps returning the same lowest free
// ID until a VM config with that ID exists. A clone creates that config only once its task
// runs, so two machines cloned close together both get the same ID: the second clone fails
// on the first one's disk image, and its cleanup removes that image, which also destroys the
// first machine's VM. The controller is the only party allocating IDs for its machines, so it
// remembers every ID it handed out until the VM is found under its name, and allocates under
// one lock. A reservation that is never released (the clone was never issued, the process
// restarted) expires after vmidReservationTTL.

// vmidReservationTTL bounds how long a handed-out VMID stays reserved without the VM appearing.
const vmidReservationTTL = 10 * time.Minute

// vmidScanLimit bounds how many IDs above the cluster's next free one are tried.
const vmidScanLimit = 1000

type vmidReservations struct {
	mu  sync.Mutex
	ids map[int64]time.Time
	now func() time.Time
}

var reservedVMIDs = &vmidReservations{ids: map[int64]time.Time{}, now: time.Now}

// held reports whether id is reserved and the reservation has not expired. Expired entries
// are dropped. The caller holds mu.
func (r *vmidReservations) held(id int64) bool {
	at, ok := r.ids[id]
	if !ok {
		return false
	}
	if r.now().Sub(at) > vmidReservationTTL {
		delete(r.ids, id)
		return false
	}
	return true
}

func (r *vmidReservations) reserve(id int64) {
	r.ids[id] = r.now()
}

func (r *vmidReservations) release(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ids, id)
}

func (r *vmidReservations) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = map[int64]time.Time{}
}

// releaseVMID forgets the reservation for id; the VM exists or the clone was abandoned.
func releaseVMID(id int64) {
	if id > 0 {
		reservedVMIDs.release(id)
	}
}

// getVMID picks the VMID for the machine's clone: the first ID of spec.vmIDRange, or the
// cluster's next free ID without a range, that no ProxmoxMachine of the cluster records, no
// other machine of this controller holds a reservation for, and Proxmox reports free. The ID
// is reserved until the VM is found (see ensureVirtualMachine) or the clone fails.
func getVMID(ctx context.Context, scope *scope.MachineScope) (int64, error) {
	reservedVMIDs.mu.Lock()
	defer reservedVMIDs.mu.Unlock()

	usedVMIDs, err := getUsedVMIDs(ctx, scope)
	if err != nil {
		return 0, err
	}

	start, end := int64(0), int64(0)
	if r := scope.ProxmoxMachine.Spec.VMIDRange; r != nil && r.Start != 0 && r.End != 0 {
		start, end = r.Start, r.End
	} else {
		next, err := scope.InfraCluster.ProxmoxClient.NextID(ctx)
		if err != nil {
			return 0, err
		}
		start, end = next, next+vmidScanLimit-1
	}

	for id := start; id <= end; id++ {
		if slices.Contains(usedVMIDs, id) || reservedVMIDs.held(id) {
			continue
		}
		free, err := scope.InfraCluster.ProxmoxClient.CheckID(ctx, id)
		if err != nil {
			return 0, err
		}
		if !free {
			continue
		}
		reservedVMIDs.reserve(id)
		return id, nil
	}

	if scope.ProxmoxMachine.Spec.VMIDRange != nil {
		return 0, ErrNoVMIDInRangeFree
	}
	return 0, ErrNoVMIDFree
}

// cloudInitVolume matches a Proxmox cloud-init volume, "vm-<vmid>-cloudinit", in a drive's
// value, whatever the storage ("local-lvm:vm-114-cloudinit", "nfs:114/vm-114-cloudinit.qcow2").
var cloudInitVolume = regexp.MustCompile(`(^|[:/])vm-\d+-cloudinit(\.|,|$)`)

// cloudInitDrives lists the drives of a VM config that carry a Proxmox cloud-init volume,
// sorted by device name. Such a drive comes from the template the VM was cloned from; the
// provider delivers cloud-init through its own ISO and must not leave a second datasource.
func cloudInitDrives(cfg *proxmox.VirtualMachineConfig) []string {
	if cfg == nil {
		return nil
	}
	var out []string
	for _, m := range []map[string]string{cfg.IDEs, cfg.SCSIs, cfg.SATAs, cfg.VirtIOs} {
		for dev, value := range m {
			if cloudInitVolume.MatchString(value) {
				out = append(out, dev)
			}
		}
	}
	slices.Sort(out)
	return out
}
