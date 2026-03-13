# spec_helper.sh — shared setup for ShellSpec tests of hack/ scripts.

# Absolute path to the fixtures directory.
FIXTURES_DIR="${SHELLSPEC_SPECDIR}/fixtures"

# setup_fixture_repo copies the fixture tree into a temporary directory and
# exports REPO_ROOT so that helpers.sh reads/writes the copy.
setup_fixture_repo() {
  FIXTURE_TMPDIR=$(mktemp -d)
  cp -a "${FIXTURES_DIR}/." "${FIXTURE_TMPDIR}/"
  # Copy hack scripts needed by the fixture Makefile (envtest-ver.sh).
  mkdir -p "${FIXTURE_TMPDIR}/hack"
  cp "${SHELLSPEC_SPECDIR}/../envtest-ver.sh" "${FIXTURE_TMPDIR}/hack/"
  cp "${SHELLSPEC_SPECDIR}/../helpers.sh" "${FIXTURE_TMPDIR}/hack/"
  export REPO_ROOT="${FIXTURE_TMPDIR}"
  # Re-evaluate paths set at source time by helpers.sh.
  METADATA_FILE="${REPO_ROOT}/metadata.yaml"
  E2E_METADATA_FILE="${REPO_ROOT}/test/e2e/data/shared/v1beta1/metadata.yaml"
  E2E_CONFIG_DIR="${REPO_ROOT}/test/e2e/config"
}

# cleanup_fixture_repo removes the temporary fixture copy.
cleanup_fixture_repo() {
  if [[ -n "${FIXTURE_TMPDIR:-}" && -d "${FIXTURE_TMPDIR}" ]]; then
    rm -rf "${FIXTURE_TMPDIR}"
  fi
}

# setup_go_mock creates a temporary directory with a mock `go` script that
# handles mod tidy (no-op), mod edit (delegates to real go), and list
# (delegates to real go list for reliable module resolution).
# Prepends the mock to PATH.
setup_go_mock() {
  GO_MOCK_DIR=$(mktemp -d)
  command -v go > "${GO_MOCK_DIR}/real-go-path"
  cat > "${GO_MOCK_DIR}/go" <<'MOCK'
#!/usr/bin/env bash
REAL_GO=$(cat "$(dirname "$0")/real-go-path")
case "${1:-}" in
  mod)
    case "${2:-}" in
      tidy) exit 0 ;;
      edit) exec "${REAL_GO}" "$@" ;;
    esac
    ;;
  list) exec "${REAL_GO}" "$@" ;;
esac
echo "mock go: unexpected invocation: $*" >&2
exit 1
MOCK
  chmod +x "${GO_MOCK_DIR}/go"
  ORIGINAL_PATH="${PATH}"
  export PATH="${GO_MOCK_DIR}:${PATH}"
}

# cleanup_go_mock removes the mock directory and restores PATH.
cleanup_go_mock() {
  if [[ -n "${GO_MOCK_DIR:-}" && -d "${GO_MOCK_DIR}" ]]; then
    rm -rf "${GO_MOCK_DIR}"
  fi
  if [[ -n "${ORIGINAL_PATH:-}" ]]; then
    export PATH="${ORIGINAL_PATH}"
  fi
}
