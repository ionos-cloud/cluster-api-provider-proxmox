#!/usr/bin/env bash
# release.sh bumps capmox version references in preparation for a release.
#
# Updates:
#   - clusterctl-settings.json: .config.nextVersion (v-prefixed)
#   - sonar-project.properties: sonar.projectVersion (no v-prefix)
#
# For new major.minor versions (not yet listed in metadata.yaml) it also:
#   - appends a releaseSeries entry to metadata.yaml (CAPI contract defaults
#     to the contract of the most recent existing entry when not supplied)
#   - updates the capmox sentinel (name: vX.Y.99) in test/e2e/config/proxmox-*.yaml
#
# Pre-release suffixes (-rc.0, -beta.1, ...) are preserved in
# clusterctl-settings.json and sonar-project.properties but stripped when
# checking/adding the releaseSeries entry and updating the e2e sentinel
# (both track major.minor only).
#
# Usage:   ./hack/release.sh <version> [<capi-contract>]
# Example: ./hack/release.sh 0.8.2
#          ./hack/release.sh 0.9.0 v1beta2
#          ./hack/release.sh 0.9.0-rc.0

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

if [[ $# -lt 1 || $# -gt 2 ]]; then
    echo "Usage: $0 <version> [<capi-contract>]"
    echo "Example: $0 0.8.2"
    echo "Example: $0 0.9.0 v1beta2"
    exit 1
fi

NEW_FULL=$(ensure_v_prefix "$1")
CONTRACT="${2:-}"

# Strip any pre-release suffix (-rc.0, -beta.1, ...) for major.minor lookups.
NEW_CORE="${NEW_FULL%%-*}"
validate_semver "${NEW_CORE}"

split_version "${NEW_CORE}"
CORE_MAJOR="${MAJOR}"
CORE_MINOR="${MINOR}"

# Always: clusterctl-settings.json (with v-prefix, suffix preserved).
clusterctl_set_version "${NEW_FULL}"

# Always: sonar-project.properties (no v-prefix, suffix preserved).
sonar_set_version "$(strip_v_prefix "${NEW_FULL}")"

# New major.minor: append to metadata.yaml + bump the e2e sentinel.
if ! metadata_has_release "${CORE_MAJOR}" "${CORE_MINOR}"; then
    if [[ -z "${CONTRACT}" ]]; then
        CONTRACT=$(metadata_latest_contract)
    fi
    metadata_add_release "${CORE_MAJOR}" "${CORE_MINOR}" "${CONTRACT}"
    e2econfig_set_capmox "v${CORE_MAJOR}.${CORE_MINOR}.99"
fi
