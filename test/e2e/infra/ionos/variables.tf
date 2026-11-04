variable "ionos_token" {
  description = "IONOS Cloud API token. Falls back to the IONOS_TOKEN environment variable when null."
  type        = string
  default     = null
  sensitive   = true
}

variable "datacenter_id" {
  description = "ID of the existing IONOS virtual data center to deploy into."
  type        = string
}

variable "name_prefix" {
  description = "Prefix for node names; nodes are named <prefix>1..<prefix>N."
  type        = string
  default     = "pve"
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

variable "node_count" {
  description = "Number of Proxmox nodes."
  type        = number
  default     = 3

  validation {
    condition     = var.node_count >= 3
    error_message = "Ceph with size=3 pools needs at least 3 nodes."
  }
}

variable "cores" {
  description = "Dedicated cores per node."
  type        = number
  default     = 16
}

variable "ram_mb" {
  description = "RAM per node in MB."
  type        = number
  default     = 32768
}

variable "cpu_family" {
  description = "IONOS CPU family. Defaults to the first family offered by the data center; must expose VMX/SVM for nested virtualization."
  type        = string
  default     = null
}

variable "image" {
  description = "IONOS public image name or alias with cloud-init support."
  type        = string
  default     = "debian:13"
}

variable "disk_type" {
  description = "IONOS volume type for root and Ceph OSD volumes."
  type        = string
  default     = "SSD Standard"
}

variable "root_disk_gb" {
  description = "Root volume size per node in GB."
  type        = number
  default     = 50
}

variable "osd_disk_gb" {
  description = "Ceph OSD volume size per node in GB."
  type        = number
  default     = 100
}

variable "ssh_public_keys" {
  description = "Additional SSH public keys authorized for root on all nodes."
  type        = list(string)
  default     = []
}

variable "allowed_source_ips" {
  description = "Source IPv4 addresses allowed to reach SSH (22) and the Proxmox UI/API (8006) on the public NICs. \"0.0.0.0/0\" allows any source."
  type        = list(string)
  default     = ["0.0.0.0/0"]

  validation {
    # IONOS firewall rules match a single source address, not a CIDR
    condition     = alltrue([for ip in var.allowed_source_ips : ip == "0.0.0.0/0" || can(regex("^\\d{1,3}(\\.\\d{1,3}){3}$", ip))])
    error_message = "Each entry must be a single IPv4 address or \"0.0.0.0/0\"."
  }
}

variable "private_cidr" {
  description = "Subnet of the private LAN used for corosync, Ceph and VXLAN. Node N gets host address 10+N."
  type        = string
  default     = "10.10.0.0/24"
}

variable "guest_cidr" {
  description = "Subnet of the VXLAN guest network. The first host address is the NAT gateway on node 1."
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
  description = "DNS resolvers used while installing packages. After the reboot, DHCP on the public NIC provides the IONOS resolvers."
  type        = list(string)
  default     = ["1.1.1.1", "9.9.9.9"]
}

variable "output_dir" {
  description = "Directory for generated SSH key and known_hosts. Defaults to ./out inside this module."
  type        = string
  default     = null
}

variable "bootstrap_timeout_seconds" {
  description = "How long to wait for nodes to finish installing Proxmox and Ceph."
  type        = number
  default     = 3600
}
