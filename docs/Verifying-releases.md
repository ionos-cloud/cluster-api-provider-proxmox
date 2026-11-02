# Verifying releases

Every tagged release publishes a multi-arch (amd64, arm64) image to
`ghcr.io/ionos-cloud/cluster-api-provider-proxmox`. The image carries a SLSA
provenance attestation and an SPDX SBOM attached to its OCI index, and is
signed with Sigstore (keyless, GitHub OIDC). Replace `v0.10.0` below with the
release you are verifying.

## Image

With the GitHub CLI:

```sh
gh attestation verify oci://ghcr.io/ionos-cloud/cluster-api-provider-proxmox:v0.10.0 --owner ionos-cloud
```

With cosign:

```sh
cosign verify ghcr.io/ionos-cloud/cluster-api-provider-proxmox:v0.10.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/ionos-cloud/cluster-api-provider-proxmox/\.github/workflows/container-image\.yaml@refs/tags/v'
```

Inspect the SBOM and provenance that buildx attached:

```sh
docker buildx imagetools inspect ghcr.io/ionos-cloud/cluster-api-provider-proxmox:v0.10.0 --format '{{ json .SBOM }}'
docker buildx imagetools inspect ghcr.io/ionos-cloud/cluster-api-provider-proxmox:v0.10.0 --format '{{ json .Provenance }}'
```
