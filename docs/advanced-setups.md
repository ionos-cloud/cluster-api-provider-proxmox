## Advanced Setups

To get started with CAPMOX please refer to the [Getting Started](Usage.md#quick-start) section.

## DHCP

If you want to use DHCP to assign ip addresses for the machines, you can use `flavor=dhcp` when generating a new cluster.

first we need to define variables:  
```bash
# The node that hosts the VM template to be used to provision VMs
export PROXMOX_SOURCENODE="stg-ceph01"
# The template VM ID used for cloning VMs
export TEMPLATE_VMID=164
# The Proxmox VE nodes used for VM deployments
export ALLOWED_NODES="[stg-ceph01,stg-ceph02,stg-ceph04,stg-ceph04,stg-ceph05]"
# The ssh authorized keys used to ssh to the machines.
export VM_SSH_KEYS="ssh-ed25519 ..., ssh-ed25519 ..."
# The IP that kube-vip is going to use as a control plane endpoint
export CONTROL_PLANE_ENDPOINT_IP="10.10.10.4"   
# The dns nameservers for the machines network-config.
export DNS_SERVERS="[10.4.1.1]"
# The device used for the boot disk.   
export BOOT_VOLUME_DEVICE=scsi0
# The size of the boot disk in GB.
export BOOT_VOLUME_SIZE=100
# The number of sockets for the VMs.
export NUM_SOCKETS=2
# The number of cores for the VMs.
export NUM_CORES=4
# The memory size for the VMs.
export MEMORY_MIB=16384
# The network bridge device for Proxmox VE VMs
export BRIDGE=vmbr0
```

#### Generate a Cluster

```bash
clusterctl generate cluster test-dhcp  \
  --infrastructure proxmox \
  --kubernetes-version v1.28.5  \
  --control-plane-machine-count=1 \
  --worker-machine-count=2 \
  --flavor=dhcp > cluster.yaml
```

## Multiple NICs

If you want to create VMs with multiple network devices,
You will need to create `InClusterPool` or `GlobalInClusterPool` to manage IPs.

here is a `GlobalInClusterPool` example:

```yaml
apiVersion: ipam.cluster.x-k8s.io/v1alpha2
kind: GlobalInClusterIPPool
metadata:
  name: shared-inclusterippool
spec:
  addresses: ${NODE_SECONDARY_IP_RANGES}
  prefix: ${SECONDARY_IP_PREFIX}
  gateway: ${SECONDARY_GATEWAY}
```

In the cluster template flavor=multiple-vlans you can define a secondary network device for the VMs.
To do that you will need to set extra environment variables along with the required ones:

```bash
# The secondary IP ranges for Cluster nodes
export NODE_SECONDARY_IP_RANGES="[10.10.10.100-10.10.10.150]"
# The Subnet Mask in CIDR notation for your node secondary IP ranges
export SECONDARY_IP_PREFIX=24
# The secondary gateway for the machines network-config
export SECONDARY_GATEWAY="10.10.10.254"
# The secondary dns nameservers for the machines network-config
export SECONDARY_DNS_SERVERS="[8.8.8.8, 8.8.4.4]"
# The Proxmox secondary network bridge for VMs
export SECONDARY_BRIDGE=vmbr2
```

#### Generate a Cluster

```bash
clusterctl generate cluster test-multiple-vlans  \
  --infrastructure proxmox \
  --kubernetes-version v1.28.3  \
  --control-plane-machine-count=1 \
  --worker-machine-count=2 \
  --flavor=multiple-vlans > cluster.yaml
```

## Dual Stack

Regarding dual-stack support, you can use the following environment variables to define the IPv6 ranges for the VMs:

```bash
# The IPv6 ranges for Cluster nodes
export NODE_IPV6_RANGES="[2001:db8:1::1-2001:db8:1::10]"
# The Subnet Mask in CIDR notation for your node IPv6 ranges
export IPV6_PREFIX=64
# The ipv6 gateway for the machines network-config.
export IPV6_GATEWAY="2001:db8:1::1"
```

#### Generate a Cluster

```bash
clusterctl generate cluster test-duacl-stack  \
  --infrastructure proxmox \
  --kubernetes-version v1.28.3  \
  --control-plane-machine-count=1 \
  --worker-machine-count=2 \
  --flavor=dual-stack > cluster.yaml
```

## Node over-/ underprovisioning

By default our scheduler only allows to allocate as much memory to guests as the host has. This might not be a desirable behaviour in all cases. For example, one might to explicitly want to overprovision their host's memory, or to reserve bit of the host's memory for itself.

This behaviour can be configured in the `ProxmoxCluster` CR through the field `.spec.schedulerHints.memoryAdjustment`.

For example, setting it to `0` (zero), entirely disables scheduling based on memory. Alternatively, if you set it to any value greater than `0`, the scheduler will treat your host as it would have `${value}%` of memory. In real numbers that would mean, if you have a host with 64GB of memory and set the number to `300`, the scheduler would allow you to provision guests with a total of 192GB memory and therefore overprovision the host. (Use with caution! It's strongly suggested to have memory ballooning configured everywhere.). Or, if you were to set it to `95` for example, it would treat your host as it would only have 60,8GB of memory, and leave the remaining 3,2GB for the host.


## Notes

* Clusters with IPV6 only is supported.
* Multiple NICs & Dual-stack setups can be mixed together.
* If you're looking for more customized setups, you can create your own cluster template and use it with the `clusterctl generate cluster` command, by passing it `--from yourtemplate.yaml`.

## API Reference

Please refer to the API reference:
* [CAPMOX API Reference](https://doc.crds.dev/github.com/ionos-cloud/cluster-api-provider-proxmox).
