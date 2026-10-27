# Proxmox VE e2e cluster on IONOS Cloud

Terraform module that turns an existing IONOS virtual data center into a Proxmox VE 9 cluster with Ceph.
It installs Proxmox on Debian 13 cloud images with cloud-init, so no custom images are needed.

## What you get

* `node_count` (default 3) dedicated-core servers. Each has one public NIC, one private NIC, a root volume and a Ceph OSD volume.
* A private LAN that carries corosync, Ceph public/cluster traffic and VXLAN.
* Public NICs reachable only on TCP 22 and 8006 from `allowed_source_ips` (single IPv4 addresses; default: any), plus ICMP from anywhere.
* A Proxmox cluster with Ceph: one mon, mgr, MDS and OSD per node. Storage:
  * `ceph-vm`: RBD, for VM disks.
  * `cephfs`: CephFS, for ISO, snippets, templates and backups.
* An SDN VXLAN VNet (`guest_bridge`, default `vmbr1`) for guests across all nodes. Node 1 is the gateway (first address of `guest_cidr`) and SNATs guest traffic out of its public NIC. Guest NICs need MTU 1450.
* A `capmox@pve` API token for CAPMOX.

## Usage

```sh
export IONOS_TOKEN=...            # or -var ionos_token=...
terraform init
terraform apply -var datacenter_id=<vdc-id> \
  -var 'ssh_public_keys=["ssh-ed25519 AAAA... me"]' \
  -var 'allowed_source_ips=["203.0.113.10"]'
```

A full apply takes about 20–30 minutes. Most of that is package installation and the reboot into the Proxmox kernel.

Terraform always generates an SSH key pair for itself and writes it to `out/id_ed25519`, along with `out/known_hosts` for the pinned host keys.
Keys from `ssh_public_keys` are authorized as well.

## Outputs

```sh
terraform output nodes                  # public/private IPs, UI URL, ssh command
terraform output -raw root_password     # root@pam (UI/API); SSH is key-only
terraform output -json ceph             # fsid, mon hosts, client.admin key/keyring
terraform output -json capmox           # PROXMOX_URL, token id/secret, bridge, gateway
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
* `terraform.tfstate` holds the root password, the Ceph admin key and the CAPMOX token secret.
* The guest gateway runs only on node 1. If node 1 is down, guests lose egress.
* Guest addresses in `guest_cidr` are reachable only from the nodes, not from the internet.
