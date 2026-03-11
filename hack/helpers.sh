#!/usr/bin/env bash
# helpers.sh provides shared routines for the hack/ scripts.
# Source this file, do not execute it directly.

# Repo root, lazily resolved on first use.
REPO_ROOT="${REPO_ROOT:-$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)}"

# ---- version helpers ----

# ensure_v_prefix adds a leading 'v' if not already present.
ensure_v_prefix() { local v="$1"; [[ "${v}" == v* ]] && echo "${v}" || echo "v${v}"; }

# strip_v_prefix removes a leading 'v' if present.
strip_v_prefix() { echo "${1#v}"; }

# validate_version exits with an error if the argument is not a valid version.
# When called with a single argument, the patch component is required
# (major.minor.patch). Pass "false" as second argument to make the patch
# component optional (major.minor or major.minor.patch).
# Both forms accept an optional leading 'v' prefix.
validate_version() {
    local raw="$1"
    local require_patch="${2:-true}"
    local v
    v=$(strip_v_prefix "${raw}")
    if [[ "${require_patch}" == true ]]; then
        if ! [[ "${v}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo "ERROR: invalid version format '${raw}'"
            echo "Expected: major.minor.patch (e.g. 1.11.0 or v1.11.0)"
            exit 1
        fi
    else
        if ! [[ "${v}" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
            echo "ERROR: invalid version format '${raw}'"
            echo "Expected: major.minor (e.g. 1.26) or major.minor.patch (e.g. 1.26.0)"
            exit 1
        fi
    fi
}

# Convenience wrappers for validate_version.
validate_semver() { validate_version "$1"; }
validate_go_version() { validate_version "$1" false; }

# split_version sets MAJOR, MINOR and PATCH for a given semver string.
split_version() {
    local no_v
    no_v=$(strip_v_prefix "$1")
    # These globals are used by callers after invoking split_version.
    # shellcheck disable=SC2034
    MAJOR=$(echo "${no_v}" | cut -d. -f1)
    # shellcheck disable=SC2034
    MINOR=$(echo "${no_v}" | cut -d. -f2)
    # shellcheck disable=SC2034
    PATCH=$(echo "${no_v}" | cut -d. -f3)
}

# versions_differ returns 0 (true) when two non-empty versions are different.
# Usage: if versions_differ "$a" "$b"; then fail "mismatch"; fi
versions_differ() {
    [[ -n "$1" && -n "$2" && "$1" != "$2" ]]
}

# ---- version extraction: go.mod ----

# gomod_go_version returns the Go version from go.mod (e.g. "1.25.0").
gomod_go_version() {
    awk '/^go /{print $2; exit}' "${REPO_ROOT}/go.mod"
}

# gomod_require_version returns the version of a package from the require
# block in go.mod (direct or indirect). Returns empty string if not found.
gomod_require_version() {
    local pkg="$1"
    awk '/^\s+'"${pkg//\//\\/}"'\s+v/{print $2; exit}' "${REPO_ROOT}/go.mod"
}

# gomod_replace_version returns the target version from a replace directive
# for the given package in go.mod. Returns empty string if not found.
gomod_replace_version() {
    local pkg="$1"
    awk '/^\s+'"${pkg//\//\\/}"' =>/{for(i=1;i<=NF;i++) if($i ~ /^v[0-9]+\./) v=$i; print v}' "${REPO_ROOT}/go.mod"
}

# effective_version returns the version of a Go module as seen by the build.
# If a replace directive overrides the module, the replace target version is
# returned; otherwise the version from require (direct or indirect) is used.
effective_version() {
    local pkg="$1"
    local ver
    ver=$(gomod_replace_version "${pkg}")
    if [[ -n "${ver}" ]]; then
        echo "${ver}"
        return
    fi
    gomod_require_version "${pkg}"
}

# ---- version extraction: other files ----

# dockerfile_go_version returns the Go major.minor from the Dockerfile
# base image (e.g. "1.25").
dockerfile_go_version() {
    awk 'match($0, /^FROM golang:([0-9]+\.[0-9]+)/, m){print m[1]; exit}' "${REPO_ROOT}/Dockerfile"
}

# docs_go_version returns the Go major.minor listed in docs/Development.md
# (e.g. "1.25"). Returns empty string if not found.
docs_go_version() {
    awk 'match($0, /Go v([0-9]+\.[0-9]+)/, m){print m[1]; exit}' "${REPO_ROOT}/docs/Development.md"
}

# custom_gcl_version returns the golangci-lint version from .custom-gcl.yaml
# (e.g. "v2.9.0"). Returns empty string if the file does not exist.
custom_gcl_version() {
    local f="${REPO_ROOT}/.custom-gcl.yaml"
    if [[ -f "${f}" ]]; then
        awk '/^version:/{print $2; exit}' "${f}"
    fi
}

# makefile_envtest_version returns the ENVTEST_K8S_VERSION value from the
# Makefile (e.g. "1.30.0"). Returns empty string if not found.
makefile_envtest_version() {
    awk '/^ENVTEST_K8S_VERSION\s*=/{sub(/^ENVTEST_K8S_VERSION\s*=\s*/, ""); print; exit}' "${REPO_ROOT}/Makefile"
}

# ---- version extraction: metadata.yaml ----

METADATA_FILE="${REPO_ROOT}/test/e2e/data/shared/v1beta1/metadata.yaml"

# metadata_latest_contract returns the contract version of the releaseSeries
# entry with the highest major.minor in the e2e metadata file (e.g. "v1beta1").
metadata_latest_contract() {
    yq '[.releaseSeries[] | {"v": (.major * 1000 + .minor), "contract": .contract}] | sort_by(.v) | reverse | .[0].contract' "${METADATA_FILE}"
}

# metadata_has_release returns 0 (true) when a releaseSeries entry with the
# given major and minor version already exists in the e2e metadata file.
metadata_has_release() {
    local major="$1" minor="$2"
    yq -e '.releaseSeries[] | select(.major == '"${major}"' and .minor == '"${minor}"')' "${METADATA_FILE}" > /dev/null 2>&1
}

# metadata_add_release prepends a new releaseSeries entry to the e2e metadata
# file and prints a confirmation message.
metadata_add_release() {
    local major="$1" minor="$2" contract="$3"
    yq -i '.releaseSeries = [{"major": '"${major}"', "minor": '"${minor}"', "contract": "'"${contract}"'"}] + .releaseSeries' "${METADATA_FILE}"
    echo "test/e2e/data/shared/v1beta1/metadata.yaml: Added releaseSeries entry for v${major}.${minor} (${contract})"
}

# ---- version update helpers ----
# Each function updates a version in a file, prints "file: Updated … old to new"
# when a change is made, and stays silent on no-op.

# gomod_set_go_version updates the "go X.Y.Z" directive in go.mod.
gomod_set_go_version() {
    local new="$1" old
    old=$(gomod_go_version)
    sed -i -E "s/^go [0-9]+\.[0-9]+(\.[0-9]+)?/go ${new}/" "${REPO_ROOT}/go.mod"
    if [[ "${old}" != "${new}" ]]; then echo "go.mod: Updated go ${old} to ${new}"; fi
}

# gomod_set_require_version updates a package version in the require block.
gomod_set_require_version() {
    local pkg="$1" new="$2" old
    old=$(gomod_require_version "${pkg}")
    sed -i -E "s|(^\s+${pkg//\//\\/}[[:space:]]+)v[^ ]+|\1${new}|" "${REPO_ROOT}/go.mod"
    if [[ -n "${old}" && "${old}" != "${new}" ]]; then echo "go.mod: Updated require ${pkg} ${old} to ${new}"; fi
}

# gomod_set_replace_version updates the target version in a replace directive.
gomod_set_replace_version() {
    local pkg="$1" new="$2" old
    old=$(gomod_replace_version "${pkg}")
    sed -i -E "s|(${pkg//\//\\/} => ${pkg//\//\\/}) v[^ ]+|\1 ${new}|" "${REPO_ROOT}/go.mod"
    if [[ -n "${old}" && "${old}" != "${new}" ]]; then echo "go.mod: Updated replace ${pkg} ${old} to ${new}"; fi
}

# dockerfile_set_go_version updates the Go major.minor in the Dockerfile base image.
dockerfile_set_go_version() {
    local new="$1" old
    old=$(dockerfile_go_version)
    sed -i -E "s/^(FROM golang:)[0-9]+\.[0-9]+(.*)/\1${new}\2/" "${REPO_ROOT}/Dockerfile"
    if [[ "${old}" != "${new}" ]]; then echo "Dockerfile: Updated golang:${old} to golang:${new}"; fi
}

# docs_set_go_version updates the Go major.minor in docs/Development.md.
docs_set_go_version() {
    local new="$1" old
    old=$(docs_go_version)
    sed -i -E "s/(- Go v)[0-9]+\.[0-9]+/\1${new}/" "${REPO_ROOT}/docs/Development.md"
    if [[ -n "${old}" && "${old}" != "${new}" ]]; then echo "docs/Development.md: Updated Go v${old} to Go v${new}"; fi
}

# custom_gcl_set_version updates the version field in .custom-gcl.yaml.
custom_gcl_set_version() {
    local new="$1" f="${REPO_ROOT}/.custom-gcl.yaml" old
    if [[ -f "${f}" ]]; then
        old=$(custom_gcl_version)
        sed -i -E "s/^(version:) .+/\1 ${new}/" "${f}"
        if [[ -n "${old}" && "${old}" != "${new}" ]]; then echo ".custom-gcl.yaml: Updated golangci-lint ${old} to ${new}"; fi
    fi
}

# makefile_set_envtest_version updates ENVTEST_K8S_VERSION in the Makefile.
makefile_set_envtest_version() {
    local new="$1" old
    old=$(makefile_envtest_version)
    sed -i -E "s/^(ENVTEST_K8S_VERSION\s*=\s*).+/\1${new}/" "${REPO_ROOT}/Makefile"
    if [[ -n "${old}" && "${old}" != "${new}" ]]; then echo "Makefile: Updated ENVTEST_K8S_VERSION ${old} to ${new}"; fi
}

# ---- module helpers ----

# run_mod_tidy runs go mod tidy from the repo root.
run_mod_tidy() {
    (cd "${REPO_ROOT}" && go mod tidy)
}
