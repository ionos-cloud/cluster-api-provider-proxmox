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

# validate_semver exits with an error if the argument is not a valid
# major.minor.patch version (with or without 'v' prefix).
validate_semver() {
    local raw="$1"
    local v
    v=$(ensure_v_prefix "${raw}")
    if ! [[ "${v}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "ERROR: invalid version format '${raw}'"
        echo "Expected: major.minor.patch (e.g. 1.11.0 or v1.11.0)"
        exit 1
    fi
}

# split_version sets MAJOR, MINOR and PATCH for a given semver string.
split_version() {
    local no_v
    no_v=$(strip_v_prefix "$1")
    MAJOR=$(echo "${no_v}" | cut -d. -f1)
    MINOR=$(echo "${no_v}" | cut -d. -f2)
    PATCH=$(echo "${no_v}" | cut -d. -f3)
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

# ---- module helpers ----

# run_mod_tidy runs go mod tidy from the repo root.
run_mod_tidy() {
    (cd "${REPO_ROOT}" && go mod tidy)
}
