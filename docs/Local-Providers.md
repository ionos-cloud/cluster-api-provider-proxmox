# How to create local repositories for Cluster API providers to use with clusterctl

## Why?

You might want to use `clusterctl init` to install your provider. If you are in your development phase, you cannot yet make use of that,
as per default `clusterctl` will only try to find providers hardcoded in its source code.
Therefore you will have to write a configuration file which will point `clusterctl` to the URLs where your providers are located.
By convention, there must a `*-components.yaml` file, which is publicly available (e.g. Github releases).

There are a few naming and versioning conventions that need to be fulfilled in order to let clusterctl generate the correct manifests.

The following section will explain how to create a configuration file that will allow you to setup the Proxmox Cluster API provider on a Kubernetes cluster
using `clusterctl init`

## Guide

In order for clusterctl to make use of local providers, we need some kind of contract files, which we have to make available. These are:
`metadata.yaml` and `*-components.yaml`

The `*-components.yaml` file depends on the type of provider, e.g. `infrastructure-components.yaml, core-components.yaml`.

As an example of creating local repositories for Cluster API, kubeadm, and Proxmox, we first have to add a `clusterctl-settings.json` to our `proxmox` repository:

```json
{
  "name": "infrastructure-proxmox",
  "config": {
    "componentsFile": "infrastructure-components.yaml",
    "nextVersion": "v0.1.0"
  }
}
```

The labeling has to start with  `infrastructure-` as the clusterctl script expects precise labeling.
We also have to make sure that there is a `metadata.yaml` file in the root directory of the promox repository:

```yaml
# metadata.yaml
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
- major: 0
  minor: 1
  contract: v1beta1
```

Make sure that the `major` and `minor` version in `metadata.yaml` match `nextVersion` in your `clusterctl-settings.json` file.
Next we need to create another `clusterctl-settings.json` file in a locally checked out
[cluster-api](https://github.com/kubernetes-sigs/cluster-api) repository.

```json
{
  "providers": ["cluster-api", "infrastructure-proxmox", "bootstrap-kubeadm", "control-plane-kubeadm", "ipam-in-cluster"],
  "provider_repos": ["../cluster-api-provider-proxmox", "../cluster-api-ipam-provider-in-cluster/"]
}
```

The path to the different repositories must be from the root of the cluster-api repository.
Make sure to clone [IPAM provider](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster) in same directory.

Next we have to execute the helper script in the cluster-api repository to generate local providers:

```bash
# go to the root directory of cluster api
cd <cluster-api-repo-path>

# run the helper script
./cmd/clusterctl/hack/create-local-repository.py
```

Now you can find the generated files in:

`~/.config/cluster-api/dev-repository`

The cli tool will also tell you how to invoke the command properly. Make sure that you use the correct config file.

Prior to executing `clusterctl init`, make sure to export the required environment variables:

```
# required
export PROXMOX_URL=https://pve.example.com
export PROXMOX_TOKEN="<TOKEN_ID>"
export PROXMOX_SECRET="<TOKEN_SECRET>"

# optional
export CAPMOX_LOGLEVEL=4
```
