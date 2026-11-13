locals {
  primary = one([for name, n in var.nodes : name if n.index == 0])

  etc_hosts = join("\n", concat(
    ["127.0.0.1 localhost", "::1 localhost ip6-localhost ip6-loopback"],
    [for name, n in var.nodes : "${n.private_ip} ${name}.${var.domain} ${name}"],
  ))
}

resource "random_password" "root" {
  length  = 32
  special = false
}

resource "tls_private_key" "host" {
  for_each  = var.nodes
  algorithm = "ED25519"
}

locals {
  user_data = {
    for name, n in var.nodes : name => join("\n", ["#cloud-config", yamlencode({
      hostname                  = name
      fqdn                      = "${name}.${var.domain}"
      prefer_fqdn_over_hostname = false
      manage_etc_hosts          = false
      disable_root              = false
      ssh_pwauth                = false
      ssh_authorized_keys       = var.ssh_authorized_keys
      chpasswd = {
        expire = false
        users  = [{ name = "root", password = random_password.root.result, type = "text" }]
      }
      ssh_deletekeys  = true
      ssh_genkeytypes = []
      ssh_keys = {
        ed25519_private = tls_private_key.host[name].private_key_openssh
        ed25519_public  = trimspace(tls_private_key.host[name].public_key_openssh)
      }
      write_files = [
        { path = "/etc/hosts", content = "${local.etc_hosts}\n" },
        {
          path        = "/usr/local/sbin/pve-bootstrap"
          permissions = "0700"
          encoding    = "b64"
          content = base64encode(templatefile("${path.module}/pve-bootstrap.sh.tftpl", {
            index          = n.index
            node_count     = length(var.nodes)
            node_name      = name
            domain         = var.domain
            private_ip     = n.private_ip
            private_cidr   = var.private_cidr
            private_prefix = split("/", var.private_cidr)[1]
            primary_ip     = var.nodes[local.primary].private_ip
            peer_ips       = join(",", [for p in var.nodes : p.private_ip])
            cluster_name   = var.cluster_name
            root_password  = random_password.root.result
            guest_bridge   = var.guest_bridge
            guest_cidr     = var.guest_cidr
            guest_prefix   = split("/", var.guest_cidr)[1]
            guest_gateway  = cidrhost(var.guest_cidr, 1)
            vxlan_tag      = var.vxlan_tag
            dns_servers    = join(" ", var.dns_servers)
            network_hook   = var.network_hook
          }))
        },
        { path = "/etc/systemd/system/pve-bootstrap.service", content = file("${path.module}/pve-bootstrap.service") },
      ]
      runcmd = [["/usr/local/sbin/pve-bootstrap"]]
    })])
  }
}
