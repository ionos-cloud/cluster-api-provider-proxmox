locals {
  nodes = {
    for i in range(var.node_count) : "${var.name_prefix}${i + 1}" => {
      index      = i
      private_ip = cidrhost(var.private_cidr, 11 + i)
    }
  }
  ssh_public_keys = concat([trimspace(tls_private_key.admin.public_key_openssh)], var.ssh_public_keys)
  output_dir      = coalesce(var.output_dir, "${path.module}/out")
  ssh_key_file    = abspath("${local.output_dir}/id_ed25519")
  known_hosts     = abspath("${local.output_dir}/known_hosts")
  outputs_file    = abspath("${local.output_dir}/bootstrap.json")
  bootstrap       = jsondecode(data.local_sensitive_file.bootstrap.content)

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

resource "tls_private_key" "admin" {
  algorithm = "ED25519"
}

module "pve" {
  source = "../pve-bootstrap"

  nodes               = local.nodes
  network_hook        = file("${path.module}/network.sh")
  ssh_authorized_keys = local.ssh_public_keys
  cluster_name        = var.cluster_name
  domain              = var.domain
  private_cidr        = var.private_cidr
  guest_cidr          = var.guest_cidr
  guest_bridge        = var.guest_bridge
  vxlan_tag           = var.vxlan_tag
  dns_servers         = var.dns_servers
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
    user_data = base64encode(module.pve.user_data[each.key])
  }

  nic {
    name            = "public"
    lan             = ionoscloud_lan.public.id
    dhcp            = true
    firewall_active = true
    firewall_type   = "INGRESS"
  }

  # bootstrap only runs on first boot; single nodes can't be rebuilt, recreate the whole cluster
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
    "${ionoscloud_server.node[name].primary_ip},${n.private_ip},${name} ${module.pve.host_public_keys[name]}\n"
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
      PRIMARY           = ionoscloud_server.node[module.pve.primary].primary_ip
      OUTPUTS_FILE      = local.outputs_file
      SSH_KEY           = local.ssh_key_file
      KNOWN_HOSTS       = local.known_hosts
      BOOTSTRAP_TIMEOUT = var.bootstrap_timeout_seconds
    }
  }
}

# read from disk so plan, refresh and destroy work without reaching the nodes
data "local_sensitive_file" "bootstrap" {
  depends_on = [terraform_data.bootstrap]
  filename   = local.outputs_file
}
