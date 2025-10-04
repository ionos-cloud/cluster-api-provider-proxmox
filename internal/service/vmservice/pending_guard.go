// Unlike VMs, which have a unique ID for tracking, PVE provides no server-side ID for configuration items like disks.
// When configuration items are specified, duplicate requests for the same slot (eg "scsi1") occur, and PVE creates
// spurious duplicates (eg "Unused Disks"). This "pending guard" mechanism avoids. We queue the item (eg disk) to be
// added, mark it as pending, and skip further adds for that slot until the TTL expires. While implemented for
// additionalVolumes, this is generalsed to other VM configuration items. "slot" is  applicable to NICs (net#),
// USB passthrough (usb#), serial ports (serial#), PCI devices (hostpci#), and ISO/CDROMs.

package vmservice

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

var (
	pendingAdds         sync.Map
	pendingTTL          = 15 * time.Second
	pendingGuardEnabled = false // defaults to off
)

// Toggle the guard and clear any leftover keys:
func EnablePendingGuard(enable bool) {
	pendingGuardEnabled = enable
	pendingAdds.Range(func(key, _ any) bool {
		pendingAdds.Delete(key)
		return true
	})
}

// Build a key. Fall-back to machineScope & slot if identifying fields are missing:
func buildPendingKey(machineScope *scope.MachineScope, slotName string) string {
	namespace, machineName, machineUID := "", "", ""
	if machineScope != nil && machineScope.ProxmoxMachine != nil {
		namespace = machineScope.ProxmoxMachine.Namespace
		machineName = machineScope.ProxmoxMachine.Name
		machineUID = string(machineScope.ProxmoxMachine.UID)
	}
	slot := strings.ToLower(strings.TrimSpace(slotName))
	if namespace == "" && machineName == "" && machineUID == "" {
		return fmt.Sprintf("addr=%p|slot=%s", machineScope, slot)
	}
	return fmt.Sprintf(
		"ns=%s|name=%s|uid=%s|slot=%s",
		namespace, machineName, machineUID, slot,
	)
}

func isPending(machineScope *scope.MachineScope, slot string) bool {
	if !pendingGuardEnabled {
		return false
	}
	key := buildPendingKey(machineScope, slot)
	if raw, found := pendingAdds.Load(key); found {
		if deadline, ok := raw.(time.Time); ok {
			if time.Now().Before(deadline) {
				return true // don't queue another
			}
			pendingAdds.Delete(key) // delete expired
		} else {
			pendingAdds.Delete(key)
		}
	}
	return false
}

func markPending(machineScope *scope.MachineScope, slot string) {
	if !pendingGuardEnabled {
		return
	}
	key := buildPendingKey(machineScope, slot)
	pendingAdds.Store(key, time.Now().Add(pendingTTL))
}

func clearPending(machineScope *scope.MachineScope, slot string) {
	if !pendingGuardEnabled {
		return
	}
	key := buildPendingKey(machineScope, slot)
	pendingAdds.Delete(key)
}
