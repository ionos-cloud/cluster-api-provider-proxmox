# Proxmox VE cluster bootstrap

Provider-neutral Terraform module that renders cloud-init user data for a Proxmox VE 9 cluster with Ceph on Debian 13 cloud images.
It creates no infrastructure. A provider wrapper such as [`../ionos`](../ionos) creates the machines, passes each node its `user_data`, and waits for the bootstrap.

## What the nodes set up

* A Proxmox cluster on the private network. The node with `index` 0 creates it; the others join one at a time in index order.
* Ceph with one mon, mgr, MDS and OSD per node. Each node uses its first blank disk as the OSD. Storage:
  * `ceph-vm`: RBD, for VM disks.
  * `cephfs`: CephFS, for ISO, snippets, templates and backups. It's the only storage that accepts ISOs, so CAPMOX's cloud-init ISOs always land there.
  * `cephfs-images`: a shared directory on CephFS for qcow2 VM disks. The CAPMOX e2e cluster templates clone as qcow2, which RBD can't hold.
* An SDN VXLAN VNet (`guest_bridge`, default `vmbr1`) for guests across all nodes. The index 0 node is the gateway (first address of `guest_cidr`) and SNATs guest traffic out of its public interface. The VNet MTU is 1450; virtio guests pick it up automatically (`host_mtu`).
  Guests can reach only the Proxmox API (8006) and ICMP on the gateway node, and nothing on the private network.
* A `capmox@pve` API token for CAPMOX with the least-privilege ACLs from [docs/advanced-setups.md](../../../../docs/advanced-setups.md#proxmox-rbac-with-least-privileges). `PVEVMAdmin` sits on `/vms` instead of on pools, because the e2e cluster templates don't set `pool`.

## Network hook

Each provider brings its own NIC layout, so the wrapper passes `network_hook`, a Bash snippet that defines two functions:

* `network_detect` runs first in phase 1, before any packages are installed. It waits for the NICs and writes the public interface name to `$STATE/public_if`.
* `network_interfaces` prints the `/etc/network/interfaces` stanzas for every NIC except `lo`. The private NIC needs `$PRIVATE_IP/$PRIVATE_PREFIX`.

The hook can use the script's helpers (`retry`, `log`) and variables (`$STATE`, `$PRIVATE_IP`, `$PRIVATE_PREFIX`, ...). See [`../ionos/network.sh`](../ionos/network.sh).

## Bootstrap

Phase 1 runs from cloud-init. It installs Proxmox VE and Ceph, switches networking from netplan to ifupdown2, and reboots.
Phase 2 runs from a systemd unit and builds the cluster, Ceph, storage, SDN and token. The index 0 node writes the CAPMOX token to `/root/pve-bootstrap/outputs.json`.

During package installation, nodes use the static resolvers from `dns_servers`, because systemd-resolved loses its upstreams once ifupdown2 takes over the NICs.
Nested virtualization needs VMX/SVM. Without it, the bootstrap fails at the start of phase 1.

## Troubleshooting

Each node logs its bootstrap to `/var/log/pve-bootstrap.log`. State markers live in `/root/pve-bootstrap/`:

* `phase1`: packages installed and the node rebooted.
* `done`: the node finished its bootstrap.
* `failed`: the last bootstrap run stopped on an error. Re-running `/usr/local/sbin/pve-bootstrap` clears it.

The bootstrap only runs on first boot, so changes to this module don't touch existing nodes.
Rebuilding a single node isn't supported: a replaced index 0 node would start a new cluster, and any other replaced node would collide with its old cluster membership.

## Notes

* `user_data` contains the root password, the node's SSH host private key and the bootstrap script.
* The guest gateway runs only on the index 0 node. If it's down, guests lose egress.
* The e2e cluster templates full-clone as qcow2 without setting `storage`, so clones land on the template's storage. Put the templates (`TEMPLATE_VMID`, `TEMPLATE_VMID_FLATCAR`, `TEMPLATEID_UPGRADE_TO` in `test/e2e/config/proxmox-ci.yaml`) on `cephfs-images`.
