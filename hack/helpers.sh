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
validate_go_version() { validate_version "$1" false; }

# ---- go.mod getters ----

# gomod_get_go returns the Go version from go.mod (e.g. "1.25.0").
gomod_get_go() {
    awk '/^go /{print $2; exit}' "${REPO_ROOT}/go.mod"
}

# gomod_get_replace returns the target version from a replace directive
# for the given package in go.mod.
# Returns empty string if not found or if there is no replace for this package.
gomod_get_replace() {
    local pkg="$1"
    (cd "${REPO_ROOT}" && go list -m -f '{{if .Replace}}{{.Replace.Version}}{{end}}' "${pkg}" 2>/dev/null) || true
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
