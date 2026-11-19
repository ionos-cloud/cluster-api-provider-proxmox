# Proxmox VE e2e cluster on IONOS Cloud

Terraform module that turns an existing IONOS virtual data center into a Proxmox VE 9 cluster with Ceph.
It installs Proxmox on Debian 13 cloud images with cloud-init, so no custom images are needed.

## What you get

* `node_count` (default 3) dedicated-core servers. Each has one public NIC, one private NIC, a root volume and a Ceph OSD volume.
* A private LAN that carries corosync, Ceph public/cluster traffic and VXLAN.
* Public NICs reachable only on TCP 22 and 8006 from `allowed_source_ips` (required; single IPv4 addresses, or `0.0.0.0/0` for any), plus ICMP from anywhere.
* The Proxmox VE cluster, Ceph storage, guest network and CAPMOX token from [`../pve-bootstrap`](../pve-bootstrap/README.md). [`network.sh`](network.sh) is the IONOS network hook: the NIC with the default route is public (DHCP), the other one is private.

## Usage

```sh
export IONOS_TOKEN=...            # or -var ionos_token=...
terraform init
terraform apply -var datacenter_id=<vdc-id> \
  -var 'ssh_public_keys=["ssh-ed25519 AAAA... me"]' \
  -var 'allowed_source_ips=["203.0.113.10"]'
```

A full apply takes about 15–20 minutes. Most of that is package installation and the reboot into the Proxmox kernel. `bootstrap_timeout_seconds` (default 60 minutes) bounds the wait.

Terraform always generates an SSH key pair for itself and writes it to `out/id_ed25519`, along with `out/known_hosts` for the pinned host keys.
Keys from `ssh_public_keys` are authorized as well.
Once the bootstrap finishes, Terraform copies the CAPMOX token to `out/bootstrap.json`. Plan, refresh and destroy read that file and never connect to the nodes, so keep `out/` with the state.

## Outputs

```sh
terraform output nodes                  # public/private IPs, UI URL, ssh command
terraform output -raw root_password     # root@pam (UI/API); SSH is key-only
terraform output -json capmox           # PROXMOX_URL, token id/secret, bridge, gateway
```

## Troubleshooting

See [`../pve-bootstrap`](../pve-bootstrap/README.md#troubleshooting) for the bootstrap log and state markers.
If a node fails, `terraform apply` prints the last 50 lines of that node's log.

To rebuild, run `terraform destroy` and `terraform apply` for the whole cluster.

## Notes

* `cpu_family` defaults to the first family the VDC offers. Nested virtualization needs VMX/SVM, which was verified on `INTEL_XEON` (de/fra) and `INTEL_SKYLAKE` (de/txl).
* After the reboot, dhclient writes the IONOS resolvers to `/etc/resolv.conf`; `dns_servers` only applies during package installation.
* IONOS can refuse resources with `VDC-5-601 not enough physical resources` when a location is out of capacity. Use a VDC in another location.
* The root password, the SSH host private keys and the bootstrap script go into the servers' cloud-init user data. Anyone with read access to the VDC can see them.
* `terraform.tfstate` holds the root password and the CAPMOX token secret.
* Guest addresses in `guest_cidr` are reachable only from the nodes, not from the internet.
