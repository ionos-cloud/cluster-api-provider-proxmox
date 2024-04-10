# Troubleshooting

## API token
To check if your api token works, you can use the following curl
```
. envfile
curl -v -H "Authorization: PVEAPIToken=$PROXMOX_TOKEN=$PROXMOX_SECRET" ${PROXMOX_URL%/}/api2/json/
```
## kind/Docker cgroups v2
Kind [requires](https://serverfault.com/questions/1053187/systemd-fails-to-run-in-a-docker-container-when-using-cgroupv2-cgroupns-priva/1054414#1054414)
[hybrid cgroups](https://github.com/systemd/systemd/blob/main/docs/CGROUP_DELEGATION.md)
when ran in Docker on Linux. A cgroupv2-only setup will break on
systemd-247 shipped in kind's container. Docker/runc also requires that there
are no anonymous cgroupv1 filesystems mounted, otherwise cgroup namespace
isolation will fail. If you require these, use Podman, but be aware that this
breaks various assumptions in `./cluster-api/Tiltfile`.

## Kind/Docker and cgroups v1 and controller type none
If you mount cgroups with no controller attached, this will break Docker/runc
in case of creating a new cgroup namespace, as runc expects that every cgroup's
name is also its type (with the exception of the systemd cgroup).

As an example:
```
/ # mkdir /sys/fs/cgroup/broken
/ # mount -t cgroup -o none,name=broken broken /sys/fs/cgroup/broken/
/ # docker --log-level debug container start be42
Error response from daemon: failed to create task for container: failed to create shim task: OCI runtime create failed: runc create failed: unable to start container process: error during container init: error mounting "cgroup" to rootfs at "/sys/fs/cgroup": mount cgroup:/sys/fs/cgroup/foobar (via /proc/self/fd/7), flags: 0xe, data: foobar: invalid argument: unknown
Error: failed to start containers: be42
```

## Kind/Docker without systemd cgroup
This breaks because Docker requires a systemd directory in cgroups, as it
remounts /sys/fs/cgroup read-only on entering the remapped namespace. If the
directory does not exist, mounting systemd will fail. This leads to systemd
in the container breaking on startup (obviously):
```
[(Linux without systemd)]
/ # docker --log-level debug start be42
INFO: ensuring we can execute mount/umount even with userns-remap
INFO: remounting /sys read-only
INFO: making mounts shared
INFO: detected cgroup v1
INFO: detected cgroupns
[...]
INFO: starting init
Failed to mount cgroup at /sys/fs/cgroup/systemd: Operation not permitted
[!!!!!!] Failed to mount API filesystems.
Exiting PID 1...
```

A fix is to create this directory, then start Docker.

## Kind/Podman
TODO

## Kind/cluster-api incompatibility
If you encounter errors like
* `missing MachineDeployment strategy` on your `MachineDeployment`
* `failed to call webhook: the server could not find the requested resource` in your capmox-controller's logs
or others, please check the image tag of the `capi-controller-manager` Deployment and compare it against our [compatibility matrix](https://github.com/ionos-cloud/cluster-api-provider-proxmox/blob/main/README.md#compatibility-with-cluster-api-and-kubernetes-versions).
```
kubectl get deployment/capi-controller-manager -o yaml | yq '.spec.template.spec.containers[].image'
```
If your capi-controller is too new, you can pass a `--core cluster-api:v1.6.1` during `clusterctl init`, to force an older version. By default it installs the latest version from the [kubernetes-sigs/cluster-api](https://github.com/kubernetes-sigs/cluster-api) project.

## Calico fails in IPVS mode with loadBalancers to expose services
Calico unfortunately does not test connectivity when it choses a node ip to use for IPVS communication.
This can be altered manually. More on this topic in [Calicos documentation](https://docs.tigera.io/calico/latest/networking/ipam/ip-autodetection#autodetection-methods).

## Machine deletion deadlock
Sometimes machines do not delete because some resource needs to be reconciled before
deletion can happen, but these resources can not reconcile (for example nodes may not drain).
To fix deletion deadlocks in such cases:
 - Remove `ipaddresses` and `ipaddressclaims` for the relevant machines
 - Remove the `proxmoxmachine` finalizer by editing `proxmoxmachines <machine>`
 - Delete the `proxmoxmachine`
 - Remove the `machine` finalizer by editing `machines <machine>`
 - Delete the `machine`

After these steps, VMs may linger in proxmox. Carefully remove those.

## Imagebuilder Environment Variables
[Proxmox VE Image Builder](https://image-builder.sigs.k8s.io/capi/providers/proxmox) and CAPMOX differ in their use of environment variables.
Trying to use CAPMOX's variables will lead to [image building failure](https://github.com/ionos-cloud/cluster-api-provider-proxmox/issues/52).
The image builder uses `PROXMOX_USERNAME` as the token name and `PROXMOX_TOKEN` as the token's secret, whereas CAPMOX uses `PROXMOX_TOKEN` as
the token name and `PROXMOX_SECRET` as the token's secret UUID.
The CAPMOX way of implementing authentication is closer to the [Proxmox API Token Documentation](https://pve.proxmox.com/wiki/Proxmox_VE_API#api_tokens),
therefore this pitfall will likely keep on existing.

## IPv6 only cluster, kube-vip fails with "unable to detect default interface"
Older versions of `kube-vip` do not consider the IPv6 routing table and therefore IPv6 interface detection fails.
Update `kube-vip` to version `0.7.2`.

Example log:
```
time="2024-03-14T11:48:58Z" level=info msg="Starting kube-vip.io [v0.5.10]"
time="2024-03-14T11:48:58Z" level=info msg="namespace [kube-system], Mode: [ARP], Features(s): Control Plane:[true], Services:[false]"
time="2024-03-14T11:48:58Z" level=info msg="No interface is specified for VIP in config, auto-detecting default Interface"
....
time="2024-03-14T11:52:30Z" level=fatal msg="unable to detect default interface -> [Unable to find default route]"
```

## Nodes fail to deploy/have wrong node-ip with mixed interface models
Kubelet chooses the first interface to acquire a node-ip for kubeadm. The first
interface is defined by the in-kernel order, which is defined by the order the
pci bus is scanned and drivers are loaded.

As an example:
```
kubectl get nodes -o wide
NAME                               STATUS     ROLES                AGE   VERSION   INTERNAL-IP   EXTERNAL-IP   OS-IMAGE             KERNEL-VERSION      CONTAINER-RUNTIME
test-cluster-control-plane-gcgc6   Ready      control-plane        11h   v1.26.7   10.0.1.69    <none>        Ubuntu 22.04.3 LTS   5.15.0-89-generic   containerd://1.7.6
test-cluster-load-balancer-c8rd2   Ready      load-balancer,node   11h   v1.26.7   10.0.2.155   <none>        Ubuntu 22.04.3 LTS   5.15.0-89-generic   containerd://1.7.6
test-cluster-load-balancer-wqbcg   Ready      load-balancer,node   11h   v1.26.7   10.0.2.152   <none>        Ubuntu 22.04.3 LTS   5.15.0-89-generic   containerd://1.7.6
test-cluster-worker-hbm8s          Ready      node                 11h   v1.26.7   10.0.1.71    <none>        Ubuntu 22.04.3 LTS   5.15.0-89-generic   containerd://1.7.6
test-cluster-worker-n2vbc          NotReady   node                 17m   v1.26.7   10.0.1.73    <none>        Ubuntu 22.04.3 LTS   5.15.0-89-generic   containerd://1.7.6
```

The load-balancers have an `e1000` interface as their default network, whereas `ens19` and `ens20` are `virtio`
```
root@test-cluster-load-balancer-zrjx8:~# ip -o l sh
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1000\    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: ens19: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 9000 qdisc prio state UP mode DEFAULT group default qlen 1000\    link/ether 0a:97:89:e5:7f:1d brd ff:ff:ff:ff:ff:ff\    altname enp0s19
3: ens20: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 9000 qdisc prio master vrf-ext state UP mode DEFAULT group default qlen 1000\    link/ether 9a:58:08:40:a2:70 brd ff:ff:ff:ff:ff:ff\    altname enp0s20
4: ens18: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 9000 qdisc prio state UP mode DEFAULT group default qlen 1000\    link/ether 16:7a:ee:74:23:0d brd ff:ff:ff:ff:ff:ff\    altname enp0s18
```

This is the order the interfaces are created in:
```
root@test-cluster-load-balancer-zrjx8:~# dmesg -t | grep eth
virtio_net virtio2 ens19: renamed from eth0
virtio_net virtio3 ens20: renamed from eth1
e1000 0000:00:12.0 eth0: (PCI:33MHz:32-bit) 16:7a:ee:74:23:0d
e1000 0000:00:12.0 eth0: Intel(R) PRO/1000 Network Connection
e1000 0000:00:12.0 ens18: renamed from eth0
```

If you absolutely must mix interface types, make sure that the default network interface is the one that comes up first.

## Machine deletion deadlock
Sometimes machines do not delete because some resource needs to be reconciled before
deletion can happen, but these resources can not reconcile (for example nodes may not drain).
To fix deletion deadlocks in such cases:
 - Remove `ipaddresses` and `ipaddressclaims` for the relevant machines
 - Remove the `proxmoxmachine` finalizer by editing `proxmoxmachines <machine>`
 - Delete the `proxmoxmachine`
 - Remove the `machine` finalizer by editing `machines <machine>`
 - Delete the `machine`

After these steps, VMs may linger in proxmox. Carefully remove those.
