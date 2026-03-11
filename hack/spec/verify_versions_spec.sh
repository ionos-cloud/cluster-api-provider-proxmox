Describe 'verify-versions.sh'
  setup() { setup_fixture_repo; }
  cleanup() { cleanup_fixture_repo; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  It 'passes with consistent fixtures'
    When run script ../verify-versions.sh
    The status should be success
  End

  It 'detects Go version mismatch in Dockerfile'
    dockerfile_set_go_version '1.24' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'Go version mismatch'
  End

  It 'detects Go version mismatch in docs'
    docs_set_go_version '1.24' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'Go version mismatch'
  End

  It 'detects golangci-lint version mismatch'
    custom_gcl_set_version 'v2.8.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'golangci-lint version mismatch'
  End

  It 'detects k8s.io ENVTEST_K8S_VERSION mismatch'
    makefile_set_envtest_version '1.30.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'ENVTEST_K8S_VERSION mismatch'
  End

  It 'detects cluster-api require/replace mismatch'
    gomod_set_require_version 'sigs.k8s.io/cluster-api' 'v1.11.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'cluster-api version mismatch'
  End
End
