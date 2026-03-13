#!/usr/bin/env bash
# helpers.sh provides shared routines for the hack/ scripts.
# Source this file, do not execute it directly.

# Repo root, lazily resolved on first use.
REPO_ROOT="${REPO_ROOT:-$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)}"

# sedi performs portable in-place sed (avoids GNU vs BSD sed -i incompatibility).
sedi() { sed -E "$@" > "$2.tmp" && mv "$2.tmp" "$2"; }

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

# ---- go.mod getters ----

# gomod_get_go returns the Go version from go.mod (e.g. "1.25.0").
gomod_get_go() {
    awk '/^go /{print $2; exit}' "${REPO_ROOT}/go.mod"
}

# gomod_get_replace returns the target version from a replace directive
# for the given package in go.mod.
# Returns empty string if not found or if there is no replace for this package.
gomod_get_require() {
    local pkg="$1"
    (cd "${REPO_ROOT}" && go list -m -f '{{.Version}}' "${pkg}" 2>/dev/null) || true
}

# gomod_get_replace returns the target version from a replace directive
# for the given package in go.mod.
# Returns empty string if not found or if there is no replace for this package.
gomod_get_replace() {
    local pkg="$1"
    (cd "${REPO_ROOT}" && go list -m -f '{{if .Replace}}{{.Replace.Version}}{{end}}' "${pkg}" 2>/dev/null) || true
}

# gomod_get_version returns the effective version of a Go module as seen by
# the build, taking replace directives into account.
gomod_get_version() {
    local pkg="$1"
    (cd "${REPO_ROOT}" && go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' "${pkg}" 2>/dev/null) || true
}

# gomod_has_version_match returns 0 (true) when all listed packages resolve
# to the same effective version.
gomod_has_version_match() {
    if [[ $# -eq 0 ]]; then return 0; fi
    local versions=()
    for pkg in "$@"; do
        versions+=("$(gomod_get_version "${pkg}")")
    done
    local count
    count=$(printf '%s\n' "${versions[@]}" | sort -u | wc -l)
    [[ "${count}" -le 1 ]]
}

# gomod_make_envtest returns the ENVTEST_K8S_VERSION derived from the
# effective k8s.io/api version in go.mod (e.g. "1.32").
gomod_make_envtest() {
    local ver
    ver=$(gomod_get_version 'k8s.io/api')
    if [[ -z "${ver}" ]]; then
        echo "ERROR: k8s.io/api not found in go.mod" >&2
        return 1
    fi
    split_version "${ver}"
    echo "1.${MINOR}"
}

# ---- go.mod setters ----
# Helpers that accept packages take the version as the first argument followed
# by one or more package names: gomod_set_require <version> <pkg>...

# gomod_set_go updates the "go X.Y.Z" directive in go.mod.
gomod_set_go() {
    local new="$1" old
    old=$(gomod_get_go)
    (cd "${REPO_ROOT}" && go mod edit -go="${new}")
    if [[ "${old}" != "${new}" ]]; then echo "go.mod: Updated go ${old} to ${new}"; fi
}

# gomod_set_require updates one or more package versions in the require block.
# Usage: gomod_set_require <version> <pkg>...
gomod_set_require() {
    local new="$1"; shift
    local args=() msgs=()
    for pkg in "$@"; do
        local old
        old=$(gomod_get_require "${pkg}")
        args+=("-require=${pkg}@${new}")
        if [[ -n "${old}" && "${old}" != "${new}" ]]; then
            msgs+=("go.mod: Updated require ${pkg} ${old} to ${new}")
        fi
    done
    (cd "${REPO_ROOT}" && go mod edit "${args[@]}")
    for msg in "${msgs[@]}"; do echo "${msg}"; done
}

# gomod_add_replace adds or updates replace directives for one or more packages.
# Usage: gomod_add_replace <version> <pkg>...
gomod_add_replace() {
    local new="$1"; shift
    local args=() msgs=()
    for pkg in "$@"; do
        local old
        old=$(gomod_get_replace "${pkg}")
        args+=("-replace=${pkg}=${pkg}@${new}")
        if [[ -n "${old}" && "${old}" != "${new}" ]]; then
            msgs+=("go.mod: Updated replace ${pkg} ${old} to ${new}")
        elif [[ -z "${old}" ]]; then
            msgs+=("go.mod: Added replace ${pkg} => ${pkg} ${new}")
        fi
    done
    (cd "${REPO_ROOT}" && go mod edit "${args[@]}")
    for msg in "${msgs[@]}"; do echo "${msg}"; done
}

# gomod_del_replace removes replace directives for one or more packages.
# Usage: gomod_del_replace <pkg>...
gomod_del_replace() {
    local args=() msgs=()
    for pkg in "$@"; do
        local old
        old=$(gomod_get_replace "${pkg}")
        if [[ -n "${old}" ]]; then
            args+=("-dropreplace=${pkg}")
            msgs+=("go.mod: Removed replace ${pkg} ${old}")
        fi
    done
    if [[ ${#args[@]} -gt 0 ]]; then
        (cd "${REPO_ROOT}" && go mod edit "${args[@]}")
    fi
    for msg in "${msgs[@]}"; do echo "${msg}"; done
}

# gomod_tidy runs go mod tidy from the repo root.
gomod_tidy() {
    (cd "${REPO_ROOT}" && go mod tidy)
}

# ---- version extraction: other files ----

# dockerfile_get_go returns the Go major.minor from the Dockerfile
# base image (e.g. "1.25").
dockerfile_get_go() {
    awk '/^FROM golang:[0-9]+\.[0-9]+/{match($0, /[0-9]+\.[0-9]+/); print substr($0, RSTART, RLENGTH); exit}' "${REPO_ROOT}/Dockerfile"
}

# docs_get_go returns the Go major.minor listed in docs/Development.md
# (e.g. "1.25"). Returns empty string if not found.
docs_get_go() {
    awk '/Go v[0-9]+\.[0-9]+/{match($0, /v[0-9]+\.[0-9]+/); print substr($0, RSTART+1, RLENGTH-1); exit}' "${REPO_ROOT}/docs/Development.md"
}

# customgcl_get_version returns the golangci-lint version from .custom-gcl.yaml
# (e.g. "v2.9.0"). Returns empty string if the file does not exist.
customgcl_get_version() {
    local f="${REPO_ROOT}/.custom-gcl.yaml"
    if [[ -f "${f}" ]]; then
        awk '/^version:/{print $2; exit}' "${f}"
    fi
}

# makefile_get_envtest returns the ENVTEST_K8S_VERSION value from the
# Makefile (e.g. "1.32").
makefile_get_envtest() {
    make -C "${REPO_ROOT}" --no-print-directory print-envtest-ver 2>/dev/null
}

# ---- version update: other files ----
# Each function updates a version in a file, prints "file: Updated … old to new"
# when a change is made, and stays silent on no-op.

# dockerfile_set_go updates the Go major.minor in the Dockerfile base image.
dockerfile_set_go() {
    local new="$1" old
    old=$(dockerfile_get_go)
    sedi "s/^(FROM golang:)[0-9]+\.[0-9]+(.*)/\1${new}\2/" "${REPO_ROOT}/Dockerfile"
    if [[ "${old}" != "${new}" ]]; then echo "Dockerfile: Updated golang:${old} to golang:${new}"; fi
}

# docs_set_go updates the Go major.minor in docs/Development.md.
docs_set_go() {
    local new="$1" old
    old=$(docs_get_go)
    sedi "s/(- Go v)[0-9]+\.[0-9]+/\1${new}/" "${REPO_ROOT}/docs/Development.md"
    if [[ -n "${old}" && "${old}" != "${new}" ]]; then echo "docs/Development.md: Updated Go v${old} to Go v${new}"; fi
}

# customgcl_set_version updates the version field in .custom-gcl.yaml.
customgcl_set_version() {
    local new="$1" f="${REPO_ROOT}/.custom-gcl.yaml" old
    if [[ -f "${f}" ]]; then
        old=$(customgcl_get_version)
        sedi "s/^(version:) .+/\1 ${new}/" "${f}"
        if [[ -n "${old}" && "${old}" != "${new}" ]]; then echo ".custom-gcl.yaml: Updated golangci-lint ${old} to ${new}"; fi
    fi
}

# ---- version extraction: e2e config ----

# E2E config files contain KUBERNETES_VERSION defaults and CAPI provider
# version references that need to stay in sync with go.mod.
E2E_CONFIG_DIR="${REPO_ROOT}/test/e2e/config"

# e2econfig_get_k8s returns the default KUBERNETES_VERSION from the first
# e2e config file (e.g. "v1.32.2").
e2econfig_get_k8s() {
    yq '.variables.KUBERNETES_VERSION | match("v[0-9]+\.[0-9]+\.[0-9]+") | .string' "${E2E_CONFIG_DIR}/proxmox-ci.yaml"
}

# e2econfig_set_k8s updates the KUBERNETES_VERSION default in all e2e config
# files and prints a confirmation message.
e2econfig_set_k8s() {
    local new="$1" old
    old=$(e2econfig_get_k8s)
    for f in "${E2E_CONFIG_DIR}/proxmox-ci.yaml" "${E2E_CONFIG_DIR}/proxmox-dev.yaml"; do
        if [[ -f "${f}" ]]; then
            # shellcheck disable=SC2016 # literal ${KUBERNETES_VERSION:-...} is intentional
            yq -i '.variables.KUBERNETES_VERSION = "${KUBERNETES_VERSION:-'"${new}"'}"' "${f}"
        fi
    done
    if [[ -n "${old}" && "${old}" != "${new}" ]]; then echo "test/e2e/config: Updated KUBERNETES_VERSION ${old} to ${new}"; fi
}

# ---- version extraction: docs kubernetes-version ----

# docs_get_k8s returns the first --kubernetes-version value found in docs
# (e.g. "v1.31.6"). Returns empty string if not found.
docs_get_k8s() {
    grep -roh -- '--kubernetes-version v[0-9]\+\.[0-9]\+\.[0-9]\+' "${REPO_ROOT}/docs/" 2>/dev/null \
        | head -1 | awk '{print $2}'
}

# docs_set_k8s updates all --kubernetes-version references in docs and prints
# a confirmation message.
docs_set_k8s() {
    local new="$1"
    local changed=false
    while IFS= read -r f; do
        if grep -q -- '--kubernetes-version v[0-9]' "${f}"; then
            sedi "s/(--kubernetes-version )v[0-9]+\.[0-9]+\.[0-9]+/\1${new}/g" "${f}"
            changed=true
        fi
    done < <(find "${REPO_ROOT}/docs" -name '*.md' -type f)
    if [[ "${changed}" == true ]]; then echo "docs: Updated --kubernetes-version references to ${new}"; fi
}
