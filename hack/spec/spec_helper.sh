# spec_helper.sh — shared setup for ShellSpec tests of hack/ scripts.

# Absolute path to the fixtures directory.
FIXTURES_DIR="${SHELLSPEC_SPECDIR}/fixtures"

# setup_fixture_repo copies the fixture tree into a temporary directory and
# exports REPO_ROOT so that helpers.sh reads/writes the copy.
setup_fixture_repo() {
  FIXTURE_TMPDIR=$(mktemp -d "${TMPDIR:-/tmp}/capmox-spec.XXXXXX")
  cp -pR "${FIXTURES_DIR}/." "${FIXTURE_TMPDIR}/"
  # Copy hack scripts needed by the fixture Makefile (envtest-ver.sh).
  mkdir -p "${FIXTURE_TMPDIR}/hack"
  cp "${SHELLSPEC_SPECDIR}/../envtest-ver.sh" "${FIXTURE_TMPDIR}/hack/"
  cp "${SHELLSPEC_SPECDIR}/../helpers.sh" "${FIXTURE_TMPDIR}/hack/"
  export REPO_ROOT="${FIXTURE_TMPDIR}"
  # Re-evaluate paths set at source time by helpers.sh.
  METADATA_FILE="${REPO_ROOT}/metadata.yaml"
  E2E_CONFIG_DIR="${REPO_ROOT}/test/e2e/config"
  TEST_WORKFLOW_FILE="${REPO_ROOT}/.github/workflows/test.yml"
  return
}

# cleanup_fixture_repo removes the temporary fixture copy.
cleanup_fixture_repo() {
  if [[ -n "${FIXTURE_TMPDIR:-}" && -d "${FIXTURE_TMPDIR}" ]]; then
    rm -rf "${FIXTURE_TMPDIR}"
  fi
  return
}

# setup_go_mock creates a temporary directory with a mock `go` script that
# handles mod tidy (no-op), mod edit (delegates to real go), and list
# (delegates to real go list for reliable module resolution).
# Prepends the mock to PATH.
setup_go_mock() {
  GO_MOCK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/capmox-go-mock.XXXXXX")
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
  return
}

# cleanup_go_mock removes the mock directory and restores PATH.
cleanup_go_mock() {
  if [[ -n "${GO_MOCK_DIR:-}" && -d "${GO_MOCK_DIR}" ]]; then
    rm -rf "${GO_MOCK_DIR}"
  fi
  if [[ -n "${ORIGINAL_PATH:-}" ]]; then
    export PATH="${ORIGINAL_PATH}"
  fi
  return
}

# setup_docker_mock creates a temporary directory with a mock `docker`
# script emulating `docker buildx imagetools inspect`, and prepends it to
# PATH.
setup_docker_mock() {
  DOCKER_MOCK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/capmox-docker-mock.XXXXXX")
  cat > "${DOCKER_MOCK_DIR}/docker" <<'MOCK'
#!/usr/bin/env bash
if [[ "$1" == "buildx" && "$2" == "imagetools" && "$3" == "inspect" ]]; then
  echo '{"mediaType":"application/vnd.oci.image.index.v1+json","digest":"sha256:cafebabecafebabecafebabecafebabecafebabecafebabecafebabecafebabe","size":9050}'
  exit 0
fi
echo "mock docker: unexpected invocation: $*" >&2
exit 1
MOCK
  chmod +x "${DOCKER_MOCK_DIR}/docker"
  ORIGINAL_PATH_DOCKER="${PATH}"
  export PATH="${DOCKER_MOCK_DIR}:${PATH}"
  return
}

# cleanup_docker_mock removes the mock directory and restores PATH.
cleanup_docker_mock() {
  if [[ -n "${DOCKER_MOCK_DIR:-}" && -d "${DOCKER_MOCK_DIR}" ]]; then
    rm -rf "${DOCKER_MOCK_DIR}"
  fi
  if [[ -n "${ORIGINAL_PATH_DOCKER:-}" ]]; then
    export PATH="${ORIGINAL_PATH_DOCKER}"
  fi
  return
}
