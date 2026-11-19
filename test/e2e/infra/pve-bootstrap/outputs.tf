output "user_data" {
  description = "cloud-config user data per node. It contains the root password and the node's SSH host private key."
  value       = local.user_data
  sensitive   = true
}

output "primary" {
  description = "Name of the node that creates the cluster, runs the guest gateway and holds the bootstrap outputs."
  value       = local.primary
}

output "host_public_keys" {
  description = "SSH host public key per node, for known_hosts."
  value       = { for name, k in tls_private_key.host : name => trimspace(k.public_key_openssh) }
}

output "root_password" {
  description = "Password for root@pam on all nodes."
  value       = random_password.root.result
  sensitive   = true
}
