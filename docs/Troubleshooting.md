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
