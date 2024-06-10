# Usage

This is a guide on how to get started with Cluster API Provider for Proxmox Virtual Environment. To learn more about cluster API in more depth, check out the [Cluster API book](https://cluster-api.sigs.k8s.io/).

Table of contents
=================

<!--ts-->
   * [Usage](#usage)
      * [Dependencies](#dependencies)
      * [Quick start](#quick-start)
         * [Pre-requisites](#pre-requisites)
         * [Configuring and installing Cluster API Provider for Proxmox VE in a management cluster](#configuring-and-installing-cluster-api-provider-for-proxmox-ve-in-a-management-cluster)
         * [Create a Workload Cluster](#create-a-workload-cluster)
         * [Check the status of the cluster](#check-the-status-of-the-cluster)
         * [Access the cluster](#access-the-cluster)
         * [Cluster templates](#cluster-templates)
            * [Flavor with Cilium CNI](#flavor-with-cilium-cni)
            * [Additional flavors](#additional-flavors)
         * [Cleaning a cluster](#cleaning-a-cluster)
         * [Custom cluster templates](#custom-cluster-templates)
      * [Using Cluster Classes](#using-cluster-classes)

<!-- Added by: root, at: Fri Dec 10 13:11:36 CET 2021 -->

<!--te-->

## Dependencies

In order to deploy a K8s cluster with CAPMOX, you require the following:

* Proxmox VE template in order to be able to create a cluster.

  * You can build VM template using [image-builder](https://github.com/kubernetes-sigs/image-builder)
    * **we recommend using** [the Proxmox VE builder](https://image-builder.sigs.k8s.io/capi/providers/proxmox).
      See our [troubleshooting docs](Troubleshooting.md#imagebuilder-environment-variables) for more information.
    * OR by [Building Raw Images](https://image-builder.sigs.k8s.io/capi/providers/proxmox)

* clusterctl, which you can download it from Cluster API (CAPI) [releases](https://github.com/kubernetes-sigs/cluster-api/releases) on GitHub.

* Kubernetes cluster for running your CAPMOX controller

* Proxmox VE Bridge e.g. `vmbr0` with an IP Range for VMs.

* [cluster-api provider IPAM `in-cluster`](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster): we rely on this IPAM provider to efficiently manage IPv4 and / or IPv6 addresses for machines without DHCP. This also makes dual-stack setups possible
   
## Quick start

### Prerequisites

In order to install Cluster API Provider for Proxmox VE, you need to have a Kubernetes cluster up and running, and `clusterctl` installed.


We need to add the IPAM provider to your clusterctl config file `~/.cluster-api/clusterctl.yaml`:

```yaml
providers:
  - name: in-cluster
    url: https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/releases/v0.1.0/ipam-components.yaml
    type: IPAMProvider
```

### Configuring and installing Cluster API Provider for Proxmox VE in a management cluster

Before you can create a cluster, you need to configure your management cluster. 
This is done by setting up the environment variables for CAPMOX and generating a cluster manifest.

---
**_NOTE_**: It is strongly recommended to use dedicated Proxmox VE user + API token. It can either be created through the UI, or by executing
```
pveum user add capmox@pve
pveum aclmod / -user capmox@pve -role PVEVMAdmin
pveum user token add capmox@pve capi -privsep 0
```
on your Proxmox VE node.

---

clusterctl requires the following variables, which should be set in `~/.cluster-api/clusterctl.yaml` as the following:

```env
## -- Controller settings -- ##
PROXMOX_URL: "https://pve.example:8006"                       # The Proxmox VE host
PROXMOX_TOKEN: "root@pam!capi"                                # The Proxmox VE TokenID for authentication
PROXMOX_SECRET: "REDACTED"                                    # The secret associated with the TokenID


## -- Required workload cluster default settings -- ##
PROXMOX_SOURCENODE: "pve"                                     # The node that hosts the VM template to be used to provision VMs
TEMPLATE_VMID: "100"                                          # The template VM ID used for cloning VMs
ALLOWED_NODES: "[pve1,pve2,pve3, ...]"                        # The Proxmox VE nodes used for VM deployments
VM_SSH_KEYS: "ssh-ed25519 ..., ssh-ed25519 ..."               # The ssh authorized keys used to ssh to the machines.

## -- networking configuration-- ##
CONTROL_PLANE_ENDPOINT_IP: "10.10.10.4"                       # The IP that kube-vip is going to use as a control plane endpoint
NODE_IP_RANGES: "[10.10.10.5-10.10.10.50, ...]"               # The IP ranges for Cluster nodes
GATEWAY: "10.10.10.1"                                         # The gateway for the machines network-config.
IP_PREFIX: "25"                                               # Subnet Mask in CIDR notation for your node IP ranges
DNS_SERVERS: "[8.8.8.8,8.8.4.4]"                              # The dns nameservers for the machines network-config.
BRIDGE: "vmbr1"                                               # The network bridge device for Proxmox VE VMs

## -- xl nodes -- ##
BOOT_VOLUME_DEVICE: "scsi0"                                   # The device used for the boot disk.
BOOT_VOLUME_SIZE: "100"                                       # The size of the boot disk in GB.
NUM_SOCKETS: "2"                                              # The number of sockets for the VMs.
NUM_CORES: "4"                                                # The number of cores for the VMs.
MEMORY_MIB: "8048"                                            # The memory size for the VMs.

EXP_CLUSTER_RESOURCE_SET: "true"                              # This enables the ClusterResourceSet feature that we are using to deploy CNI
CLUSTER_TOPOLOGY: "true"                                      # This enables experimental ClusterClass templating
```

the `CONTROL_PLANE_ENDPOINT_IP` is an IP that must be on the same subnet as the control plane machines
`CONTROL_PLANE_ENDPOINT_IP` is mandatory

the `EXP_CLUSTER_RESOURCE_SET` is required if you want to deploy CNI using cluster resource sets (mandatory in the cilium and calico flavors).

Once you have access to a management cluster, you can initialize Cluster API with the following:
```
clusterctl init --infrastructure proxmox --ipam in-cluster --core cluster-api:v1.6.1
```

**Note:** The Proxmox credentials are optional when installing the provider,
but they are required when creating a cluster.

### Create a Workload Cluster
To create a new cluster, you need to generate a cluster manifest.

```bash
$ clusterctl generate cluster proxmox-quickstart \
    --infrastructure proxmox \
    --kubernetes-version v1.27.8 \
    --control-plane-machine-count 1 \
    --worker-machine-count 3 > cluster.yaml

# Create the workload cluster in the current namespace on the management cluster
$ kubectl apply -f cluster.yaml
```

### Check the status of the cluster
```
$ clusterctl describe cluster proxmox-quickstart
```

Wait until the cluster is ready. This can take a few minutes.

### Access the cluster
you can use the following command to get the kubeconfig:
```
clusterctl get kubeconfig proxmox-quickstart > proxmox-quickstart.kubeconfig
```

If you do not have CNI yet, you can use the following command to install a CNI:
```
KUBECONFIG=proxmox-quickstart.kubeconfig kubectl apply -f https://docs.projectcalico.org/manifests/calico.yaml
```

After that you should see your nodes become ready:
```
KUBECONFIG=proxmox-quickstart.kubeconfig kubectl get nodes
```

### Cluster templates

We provide various templates for creating clusters. Some of these templates
provide you with a [CNI](#https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/) already.

For templates using `CNI`s you're required to create `ConfigMaps` to make `ClusterResourceSets` available.

We provide the following templates:

| Flavor              | Tepmlate File                                        | CRS File                                                  |
|---------------------|------------------------------------------------------|-----------------------------------------------------------|
| cilium              | templates/cluster-template-cilium.yaml               | templates/crs/cni/cilium.yaml                             |
| calico              | templates/cluster-template-calico.yaml               | templates/crs/cni/calico.yaml                             |
| multiple-vlans      | templates/cluster-template-multiple-vlans.yaml       | -                                                         |
| default             | templates/cluster-template.yaml                      | -                                                         |
| cilium loadbalancer | templates/cluster-template-cilium-load-balancer.yaml | templates/crs/cni/cilium.yaml, templates/crs/metallb.yaml |
| external-creds      | templates/cluster-template-external-creds.yaml       |                                                           |

For more information about advanced clusters please check our [advanced setups docs](advanced-setups.md).

#### External Credentials

The `external-creds` flavor is used to create a cluster with external credentials.
This is useful when you want to use different Proxmox Datacenters.

you will need these environment variables to generate a cluster with external credentials:

```env
PROXMOX_URL: "https://pve.example:8006"                       # The Proxmox VE host
PROXMOX_TOKEN: "root@pam!capi"                                # The Proxmox VE TokenID for authentication
PROXMOX_SECRET: "REDACTED"                                    # The secret associated with the TokenID
```

However, to use external-credentials in your own Cluster manifests, you need to create a secret
and reference it in the cluster manifest.
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: "my-cluster"
spec:
  controlPlaneEndpoint:
    host: ${CONTROL_PLANE_ENDPOINT_IP}
    port: 6443
  # ...  
  credentialsRef:
    name: "my-cluster-proxmox-credentials"
---
apiVersion: v1
stringData:
  secret: ${PROXMOX_SECRET}
  token: ${PROXMOX_TOKEN}
  url: ${PROXMOX_URL}
kind: Secret
metadata:
  name: my-cluster-proxmox-credentials
  labels:
    # Custom IONOS Label
    platform.ionos.com/secret-type: "proxmox-credentials"
```

#### Flavor with Cilium CNI
Before this cluster can be deployed, `cilium` needs to be configured. As a first step we
need to generate a manifest. Simply use our makefile:

```
make crs-cilium

```
Now install the ConfigMap into your k8s:

```
kubectl create cm cilium  --from-file=data=templates/crs/cni/cilium.yaml
```

Now, you can create a cluster using the cilium flavor:

```bash
$ clusterctl generate cluster proxmox-cilium \
--infrastructure proxmox \
--kubernetes-version v1.27.8 \
--control-plane-machine-count 1 \
--worker-machine-count 3 \
--flavor cilium > cluster.yaml

$ kubectl apply -f cluster.yaml
```

#### Additional flavors

1. Create the CRS file for your flavor:

```bash
make crs-$FLAVOR_NAME
```
2. Create the configmap for the CRS.
   You have to name the configmap like the name of the template:

```bash
kubectl create cm $FLAVOR_NAME --from-file=data=$CRS_FILE
```
3. Generate & Apply the cluster manifest.

```bash
$ clusterctl generate cluster proxmox-crs \
--infrastructure proxmox \
--kubernetes-version v1.27.8 \
--control-plane-machine-count 1 \
--worker-machine-count 3 \
--flavor $FLAVOR_NAME > cluster-crs.yaml

kubectl apply -f cluster-crs.yaml
```

### Cleaning a cluster
```
kubectl delete cluster proxmox-quickstart
```

### Custom cluster templates

If you need anything specific that requires a more complex setup, we recommend to use custom templates:

```
$ clusterctl generate custom-cluster proxmox-quickstart \
    --infrastructure proxmox \
    --kubernetes-version v1.27.8 \
    --control-plane-machine-count 1 \
    --worker-machine-count 3 \
    --from ~/workspace/custom-cluster-template.yaml > custom-cluster.yaml
```

## Using Cluster Classes
[ClusterClass](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/)
is an experimental feature to manage clusters without templating. In this case, you only
need to write the cluster definition (referring to the cluster class), and all required resources
are automatically created for you.

This feature requires [CLUSTER_TOPOLOGY](https://cluster-api.sigs.k8s.io/tasks/experimental-features/experimental-features#enabling-experimental-features-on-tilt)
to be set in your capi controller and in the environment of clusterctl.

We provide the following ClusterClasses:

| Flavor         | Template File                                   | CRS File                      | Example Cluster Manifest     |
|----------------| ----------------------------------------------- |-------------------------------|-------------------------------
| cilium         | templates/cluster-class-cilium.yaml             | templates/crs/cni/cilium.yaml | examples/cluster-cilium.yaml |
| calico         | templates/cluster-class-calico.yaml             | templates/crs/cni/calico.yaml | examples/cluster-calico.yaml |
| default        | templates/cluster-class.yaml                    | -                             | examples/cluster.yaml        |

### Creating a cluster from a ClusterClass
1. Choose a ClusterClass
All ClusterClasses provide the same features except for the CNI they refer to. The base ClusterClass
also does not provide MachineHealthChecks as those can not be successful until a CNI is deployed.

We recommend that you start with a ClusterClass which defines a CNI. Please
refer to [CNI Cilium](#flavor-with-cilium-cni) for details on how to get started.

Apply the ClusterClass custom resource definition so you can create cluster manifests:

```bash
kubectl apply -f templates/cluster-class-cilium.yaml
```

2. Write the cluster manifest
An example can be found in [examples/cluster-cilum.yaml](../examples/cluster-cilium.yaml).

Important fields:
- `.metadata.name: cluster-name` the name of the cluster to be generated.
- `.spec.topology.class: `proxmox-clusterclass-cilium-v0.1.0` the clusterClass used for generating resources.
- `.spec.topology.version: 1.25.10` The k8s version used by kubeadm.

All possible fields refer to [CAPMOX environment variables](#capmox-environment-variables).

3. Preview the cluster topology

```bash
clusterctl alpha topology plan -f examples/cluster-cilium.yaml -o out/
```

The to-be-created resources will be located in `out/created`.

4. Apply the Cluster Manifest

```bash
kubectl apply -f mycluster.yaml
```

If you run into issues, refer to [Cluster Health and deployment status](#cluster-health-and-deployment-status).
