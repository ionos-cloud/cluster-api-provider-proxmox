output "nodes" {
  description = "Proxmox nodes with their public and private addresses."
  value = {
    for name, n in local.nodes : name => {
      public_ip  = ionoscloud_server.node[name].primary_ip
      private_ip = n.private_ip
      web_ui     = "https://${ionoscloud_server.node[name].primary_ip}:8006"
      ssh        = "ssh -i ${local.ssh_key_file} -o UserKnownHostsFile=${local.known_hosts} root@${ionoscloud_server.node[name].primary_ip}"
    }
  }
}

output "root_password" {
  description = "Password for root@pam (web UI and API). SSH is key-only."
  value       = random_password.root.result
  sensitive   = true
}

output "ssh_private_key_file" {
  description = "Generated SSH private key authorized for root on all nodes."
  value       = local.ssh_key_file
}

output "ceph" {
  description = "Ceph cluster identity and client.admin credentials."
  value = {
    fsid      = data.external.bootstrap.result.ceph_fsid
    mon_hosts = [for n in local.nodes : n.private_ip]
    admin_key = data.external.bootstrap.result.ceph_admin_key
    keyring   = "[client.admin]\n\tkey = ${data.external.bootstrap.result.ceph_admin_key}\n"
  }
  sensitive = true
}

output "capmox" {
  description = "Proxmox API endpoint and token for CAPMOX (PROXMOX_URL, PROXMOX_TOKEN, PROXMOX_SECRET)."
  value = {
    url          = "https://${ionoscloud_server.node[local.primary].primary_ip}:8006"
    token_id     = data.external.bootstrap.result.capmox_token_id
    token_secret = data.external.bootstrap.result.capmox_token_secret
    bridge       = var.guest_bridge
    guest_cidr   = var.guest_cidr
    gateway      = cidrhost(var.guest_cidr, 1)
    nodes        = keys(local.nodes)
  }
  sensitive = true
}
