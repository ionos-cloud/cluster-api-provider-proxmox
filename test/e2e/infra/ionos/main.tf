locals {
  nodes = {
    for i in range(var.node_count) : "${var.name_prefix}${i + 1}" => {
      index      = i
      private_ip = cidrhost(var.private_cidr, 11 + i)
    }
  }
  primary         = "${var.name_prefix}1"
  ssh_public_keys = concat([trimspace(tls_private_key.admin.public_key_openssh)], var.ssh_public_keys)
  output_dir      = coalesce(var.output_dir, "${path.module}/out")
  ssh_key_file    = abspath("${local.output_dir}/id_ed25519")
  known_hosts     = abspath("${local.output_dir}/known_hosts")

  etc_hosts = join("\n", concat(
    ["127.0.0.1 localhost", "::1 localhost ip6-localhost ip6-loopback"],
    [for name, n in local.nodes : "${n.private_ip} ${name}.${var.domain} ${name}"],
  ))

  firewall_rules = merge(
    { for pair in setproduct(var.allowed_source_ips, [22, 8006]) : "tcp-${pair[1]}-${pair[0]}" => {
      protocol = "TCP"
      source   = pair[0]
      port     = pair[1]
    } },
    { icmp = { protocol = "ICMP", source = "0.0.0.0/0", port = null } },
  )
}

data "ionoscloud_datacenter" "this" {
  id = var.datacenter_id
}

resource "random_password" "root" {
  length  = 32
  special = false
}

resource "tls_private_key" "admin" {
  algorithm = "ED25519"
}

resource "tls_private_key" "host" {
  for_each  = local.nodes
  algorithm = "ED25519"
}

resource "ionoscloud_lan" "public" {
  datacenter_id = var.datacenter_id
  name          = "${var.cluster_name}-public"
  public        = true
}

resource "ionoscloud_lan" "private" {
  datacenter_id = var.datacenter_id
  name          = "${var.cluster_name}-private"
  public        = false
}

resource "ionoscloud_server" "node" {
  for_each = local.nodes

  datacenter_id     = var.datacenter_id
  name              = each.key
  type              = "ENTERPRISE"
  cores             = var.cores
  ram               = var.ram_mb
  cpu_family        = coalesce(var.cpu_family, data.ionoscloud_datacenter.this.cpu_architecture[0].cpu_family)
  availability_zone = "AUTO"
  image_name        = var.image
  ssh_keys          = local.ssh_public_keys

  volume {
    name      = "${each.key}-root"
    size      = var.root_disk_gb
    disk_type = var.disk_type
    bus       = "VIRTIO"
    user_data = base64encode(join("\n", ["#cloud-config", yamlencode({
      hostname                  = each.key
      fqdn                      = "${each.key}.${var.domain}"
      prefer_fqdn_over_hostname = false
      manage_etc_hosts          = false
      disable_root              = false
      ssh_pwauth                = false
      ssh_authorized_keys       = local.ssh_public_keys
      chpasswd = {
        expire = false
        users  = [{ name = "root", password = random_password.root.result, type = "text" }]
      }
      ssh_deletekeys  = true
      ssh_genkeytypes = []
      ssh_keys = {
        ed25519_private = tls_private_key.host[each.key].private_key_openssh
        ed25519_public  = trimspace(tls_private_key.host[each.key].public_key_openssh)
      }
      write_files = [
        { path = "/etc/hosts", content = "${local.etc_hosts}\n" },
        {
          path        = "/usr/local/sbin/pve-bootstrap"
          permissions = "0700"
          encoding    = "b64"
          content = base64encode(templatefile("${path.module}/templates/pve-bootstrap.sh.tftpl", {
            index          = each.value.index
            node_count     = var.node_count
            node_name      = each.key
            domain         = var.domain
            private_ip     = each.value.private_ip
            private_cidr   = var.private_cidr
            private_prefix = split("/", var.private_cidr)[1]
            primary_ip     = local.nodes[local.primary].private_ip
            peer_ips       = join(",", [for n in local.nodes : n.private_ip])
            cluster_name   = var.cluster_name
            root_password  = random_password.root.result
            guest_bridge   = var.guest_bridge
            guest_cidr     = var.guest_cidr
            guest_prefix   = split("/", var.guest_cidr)[1]
            guest_gateway  = cidrhost(var.guest_cidr, 1)
            vxlan_tag      = var.vxlan_tag
            dns_servers    = join(" ", var.dns_servers)
          }))
        },
        { path = "/etc/systemd/system/pve-bootstrap.service", content = file("${path.module}/templates/pve-bootstrap.service") },
      ]
      runcmd = [["/usr/local/sbin/pve-bootstrap"]]
    })]))
  }

  nic {
    name            = "public"
    lan             = ionoscloud_lan.public.id
    dhcp            = true
    firewall_active = true
    firewall_type   = "INGRESS"
  }

  # bootstrap only runs on first boot; rebuild nodes explicitly with -replace
  lifecycle {
    ignore_changes = [volume[0].user_data]
  }
}

resource "ionoscloud_firewall" "public" {
  for_each = {
    for pair in setproduct(keys(local.nodes), keys(local.firewall_rules)) : "${pair[0]}-${pair[1]}" => {
      node = pair[0]
      rule = local.firewall_rules[pair[1]]
    }
  }

  datacenter_id    = var.datacenter_id
  server_id        = ionoscloud_server.node[each.value.node].id
  nic_id           = ionoscloud_server.node[each.value.node].primary_nic
  name             = each.key
  type             = "INGRESS"
  protocol         = each.value.rule.protocol
  source_ip        = each.value.rule.source == "0.0.0.0/0" ? null : each.value.rule.source
  port_range_start = each.value.rule.port
  port_range_end   = each.value.rule.port
}

resource "ionoscloud_nic" "private" {
  for_each = local.nodes

  datacenter_id = var.datacenter_id
  server_id     = ionoscloud_server.node[each.key].id
  name          = "private"
  lan           = ionoscloud_lan.private.id
  dhcp          = false
  ips           = [each.value.private_ip]
}

resource "ionoscloud_volume" "osd" {
  for_each = local.nodes

  datacenter_id = var.datacenter_id
  server_id     = ionoscloud_server.node[each.key].id
  name          = "${each.key}-osd"
  size          = var.osd_disk_gb
  disk_type     = var.disk_type
  bus           = "VIRTIO"
  licence_type  = "OTHER"
}

resource "local_sensitive_file" "ssh_private_key" {
  filename        = local.ssh_key_file
  content         = tls_private_key.admin.private_key_openssh
  file_permission = "0600"
}

resource "local_file" "ssh_public_key" {
  filename        = "${local.ssh_key_file}.pub"
  content         = tls_private_key.admin.public_key_openssh
  file_permission = "0644"
}

resource "local_file" "known_hosts" {
  filename        = local.known_hosts
  file_permission = "0644"
  content = join("", [
    for name, n in local.nodes :
    "${ionoscloud_server.node[name].primary_ip},${n.private_ip},${name} ${tls_private_key.host[name].public_key_openssh}"
  ])
}

resource "terraform_data" "bootstrap" {
  triggers_replace = [for s in ionoscloud_server.node : s.id]

  depends_on = [
    ionoscloud_firewall.public,
    ionoscloud_nic.private,
    ionoscloud_volume.osd,
    local_sensitive_file.ssh_private_key,
    local_file.known_hosts,
  ]

  provisioner "local-exec" {
    command = "${path.module}/scripts/wait-bootstrap.sh"
    environment = {
      HOSTS             = join(" ", [for s in ionoscloud_server.node : s.primary_ip])
      SSH_KEY           = local.ssh_key_file
      KNOWN_HOSTS       = local.known_hosts
      BOOTSTRAP_TIMEOUT = var.bootstrap_timeout_seconds
    }
  }
}

data "external" "bootstrap" {
  depends_on = [terraform_data.bootstrap]

  program = [
    "ssh", "-i", local.ssh_key_file,
    "-o", "UserKnownHostsFile=${local.known_hosts}",
    "-o", "StrictHostKeyChecking=yes",
    "-o", "BatchMode=yes",
    "root@${ionoscloud_server.node[local.primary].primary_ip}",
    "cat /root/pve-bootstrap/outputs.json",
  ]
}
