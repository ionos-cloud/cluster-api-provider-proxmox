# Proxmox VE e2e cluster on IONOS Cloud

Terraform module that turns an existing IONOS virtual data center into a Proxmox VE 9 cluster with Ceph.
It installs Proxmox on Debian 13 cloud images with cloud-init, so no custom images are needed.

## What you get

* `node_count` (default 3) dedicated-core servers. Each has one public NIC, one private NIC, a root volume and a Ceph OSD volume.
* A private LAN that carries corosync, Ceph public/cluster traffic and VXLAN.
* Public NICs reachable only on TCP 22 and 8006 from `allowed_source_ips` (single IPv4 addresses, required; `"0.0.0.0/0"` opens them to any source), plus ICMP from anywhere.
* A Proxmox cluster with Ceph: one mon, mgr, MDS and OSD per node. Storage:
  * `ceph-vm`: RBD, for VM disks.
  * `cephfs`: CephFS, for ISO, snippets, templates and backups.
  * `cephfs-images`: a shared directory on CephFS for qcow2 VM disks. The CAPMOX e2e cluster templates clone as qcow2, which RBD can't hold.
* An SDN VXLAN VNet (`guest_bridge`, default `vmbr1`) for guests across all nodes. Node 1 is the gateway (first address of `guest_cidr`) and SNATs guest traffic out of its public NIC. Guests can reach node 1 only on 8006 (and ICMP), and cannot reach the private LAN. The VNet MTU is 1450; virtio guests pick it up automatically (`host_mtu`).
* A `capmox@pve` API token for CAPMOX with the least privileges from [docs/advanced-setups.md](../../../../docs/advanced-setups.md#proxmox-rbac-with-least-privileges):

  | Path | Role |
  | ---- | ---- |
  | `/` | `Sys.Audit` (not propagated) |
  | `/nodes` | `Sys.Audit` |
  | `/vms` | `PVEVMAdmin` |
  | `/sdn/zones/e2e/<guest_bridge>` | `PVESDNUser` |
  | `/storage/cephfs` | `PVEDatastoreAdmin` (bootstrap ISOs) |
  | `/storage/cephfs-images`, `/storage/ceph-vm` | `Datastore.AllocateSpace` |

  VMs are not pool-scoped yet, because the e2e cluster templates don't set a pool.

## Usage

```sh
export IONOS_TOKEN=...            # or -var ionos_token=...
terraform init
terraform apply -var datacenter_id=<vdc-id> \
  -var 'ssh_public_keys=["ssh-ed25519 AAAA... me"]' \
  -var 'allowed_source_ips=["203.0.113.10"]'
```

A full apply takes about 20–30 minutes. Most of that is package installation and the reboot into the Proxmox kernel.
The apply fails if the nodes are not done after `bootstrap_timeout_seconds` (default 60 minutes).
It needs `ssh` and `jq` on the machine running Terraform.

Terraform always generates an SSH key pair for itself and writes it to `out/id_ed25519`, along with `out/known_hosts` for the pinned host keys.
Keys from `ssh_public_keys` are authorized as well.

## Outputs

```sh
terraform output nodes                  # public/private IPs, UI URL, ssh command
terraform output -raw root_password     # root@pam (UI/API); SSH is key-only
terraform output -raw capmox_env_file   # out/capmox.env
```

Once the nodes are bootstrapped, the apply fetches the CAPMOX token once and writes `out/capmox.env` (mode 0600).
It holds `PROXMOX_URL`, `PROXMOX_TOKEN`, `PROXMOX_SECRET` and the guest network settings the e2e config reads (`BRIDGE`, `GATEWAY`, `IP_PREFIX`, `NODE_IP_RANGES`, `CONTROL_PLANE_ENDPOINT_IP`, `PROXMOX_SOURCENODE`, `ALLOWED_NODES`).
The token secret is not in the Terraform state, and `plan` and `destroy` never connect to the nodes.

## Templates

The module does not create the VM templates the e2e suite clones (`TEMPLATE_VMID` 104, `TEMPLATEID_UPGRADE_TO` 102, `TEMPLATE_VMID_FLATCAR` 1001).
The e2e cluster templates clone with `format: qcow2` and no `storage`, so the clone lands on the template's storage.
Import each template onto `cephfs-images` (on node 1, which is `PROXMOX_SOURCENODE`), for example:

```sh
qm create 104 --name ubuntu-k8s --memory 2048 --net0 virtio,bridge=vmbr1 --scsihw virtio-scsi-pci
qm disk import 104 ubuntu-k8s.qcow2 cephfs-images --format qcow2
qm set 104 --scsi0 cephfs-images:104/vm-104-disk-0.qcow2 --boot order=scsi0
qm template 104
```

## Running the e2e suite

```sh
set -a; . test/e2e/infra/ionos/out/capmox.env; set +a
make test-e2e
```

## Troubleshooting

Each node logs its bootstrap to `/var/log/pve-bootstrap.log`. State markers live in `/root/pve-bootstrap/`:

* `phase1`: packages installed and the node rebooted.
* `done`: the node finished its bootstrap.
* `failed`: the last bootstrap run stopped on an error. Re-running `/usr/local/sbin/pve-bootstrap` clears it.

If a node fails, `terraform apply` prints the last 50 lines of that node's log.

Changes to the bootstrap template don't touch existing nodes, because the bootstrap only runs on first boot.
Rebuilding a single node isn't supported: a replaced node 1 would start a new cluster, and a replaced node 2 or 3 would collide with its old cluster membership.
To rebuild, run `terraform destroy` and `terraform apply` for the whole cluster.

## Notes

* `cpu_family` defaults to the first family the VDC offers. Nested virtualization needs VMX/SVM, which was verified on `INTEL_XEON` (de/fra) and `INTEL_SKYLAKE` (de/txl).
* During package installation, nodes use static resolvers from `dns_servers`, because systemd-resolved loses its DHCP upstreams once ifupdown2 takes over the NICs. After the reboot, dhclient writes the IONOS resolvers to `/etc/resolv.conf`.
* IONOS can refuse resources with `VDC-5-601 not enough physical resources` when a location is out of capacity. Use a VDC in another location.
* The root password, the SSH host private keys and the bootstrap script go into the servers' cloud-init user data. Anyone with read access to the VDC can see them.
* `terraform.tfstate` holds the root password and the SSH keys. `out/capmox.env` holds the CAPMOX token secret.
* The guest gateway runs only on node 1. If node 1 is down, guests lose egress.
* Guest addresses in `guest_cidr` are reachable only from the nodes, not from the internet.
