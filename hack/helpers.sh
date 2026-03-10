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

# effective_version returns the version of a Go module as seen by the build.
# If a replace directive overrides the module, the replace target version is
# returned; otherwise the version from require (direct or indirect) is used.
effective_version() {
    local pkg="$1"
    local gomod="${REPO_ROOT}/go.mod"
    local replace_ver
    replace_ver=$(grep -E "^\s+${pkg} =>" "${gomod}" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | tail -1 || true)
    if [[ -n "${replace_ver}" ]]; then
        echo "${replace_ver}"
        return
    fi
    grep -E "^\s+${pkg}\s+v" "${gomod}" | awk '{print $2}' | head -1 || true
}

# ---- module helpers ----

# run_mod_tidy runs go mod tidy from the repo root.
run_mod_tidy() {
    (cd "${REPO_ROOT}" && go mod tidy)
}
