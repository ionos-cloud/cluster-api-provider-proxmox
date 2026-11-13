variable "nodes" {
  description = "Proxmox nodes by hostname. The node with index 0 creates the cluster; the others join in index order."
  type = map(object({
    index      = number
    private_ip = string
  }))

  validation {
    condition     = length(var.nodes) >= 3
    error_message = "Ceph with size=3 pools needs at least 3 nodes."
  }

  validation {
    condition     = length([for n in var.nodes : n if n.index == 0]) == 1
    error_message = "Exactly one node must have index 0."
  }
}

variable "network_hook" {
  description = "Bash defining network_detect (runs first in phase 1; must write the public interface name to $STATE/public_if) and network_interfaces (prints /etc/network/interfaces stanzas for all NICs except lo)."
  type        = string
}

variable "ssh_authorized_keys" {
  description = "SSH public keys authorized for root on all nodes."
  type        = list(string)
}

variable "cluster_name" {
  description = "Proxmox VE cluster name."
  type        = string
  default     = "capmox-e2e"
}

variable "domain" {
  description = "DNS domain used for node FQDNs in /etc/hosts."
  type        = string
  default     = "capmox.internal"
}

variable "private_cidr" {
  description = "Subnet of the private network used for corosync, Ceph and VXLAN. Must contain every node's private_ip."
  type        = string
}

variable "guest_cidr" {
  description = "Subnet of the VXLAN guest network. The first host address is the NAT gateway on the index 0 node."
  type        = string
  default     = "10.20.0.0/16"
}

variable "guest_bridge" {
  description = "Name of the SDN VNet guests attach to (max 8 chars)."
  type        = string
  default     = "vmbr1"
}

variable "vxlan_tag" {
  description = "VXLAN network identifier of the guest VNet."
  type        = number
  default     = 100
}

variable "dns_servers" {
  description = "DNS resolvers written to /etc/resolv.conf while installing packages."
  type        = list(string)
  default     = ["1.1.1.1", "9.9.9.9"]
}
