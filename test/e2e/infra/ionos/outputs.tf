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

output "capmox_env_file" {
  description = "Environment file for make test-e2e: PROXMOX_URL, PROXMOX_TOKEN, PROXMOX_SECRET and the guest network settings. Written by the bootstrap wait, not kept in state."
  value       = local.env_file
}
