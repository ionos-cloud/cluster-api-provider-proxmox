terraform {
  required_version = ">= 1.5"

  # exact versions instead of a lock file: the lock file records registry hosts, which differ between Terraform and OpenTofu
  required_providers {
    ionoscloud = {
      source  = "ionos-cloud/ionoscloud"
      version = "6.7.41"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.9.1"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "4.4.2"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.9.1"
    }
  }
}

provider "ionoscloud" {
  token = var.ionos_token
}
