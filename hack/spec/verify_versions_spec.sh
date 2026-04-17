Describe 'verify-versions.sh'
  setup() { setup_fixture_repo; setup_go_mock; }
  cleanup() { cleanup_fixture_repo; cleanup_go_mock; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  It 'passes with consistent fixtures'
    When run script ../verify-versions.sh
    The status should be success
  End

  It 'detects Go version mismatch in Dockerfile'
    dockerfile_set_go '1.24' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'Go version mismatch'
  End

  It 'detects Go version mismatch in docs'
    docs_set_go '1.24' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'Go version mismatch'
  End

  It 'detects Go version mismatch in .golangci-kal.yml'
    golangcikal_set_go '1.24' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'Go version mismatch'
    The output should include '.golangci-kal.yml'
  End

  It 'detects golangci-lint version mismatch'
    customgcl_set_version 'v2.8.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'golangci-lint version mismatch'
  End

  It 'passes with dynamic ENVTEST_K8S_VERSION derived from k8s.io/api'
    When run script ../verify-versions.sh
    The status should be success
  End

  It 'detects cluster-api require/replace mismatch'
    gomod_set_require 'v1.11.0' 'sigs.k8s.io/cluster-api' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'cluster-api version mismatch'
  End

  It 'detects KUBERNETES_VERSION mismatch in e2e config'
    e2econfig_set_k8s 'v1.30.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'KUBERNETES_VERSION mismatch'
  End

  It 'detects --kubernetes-version mismatch in docs'
    docs_set_k8s 'v1.30.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'docs --kubernetes-version mismatch'
  End

  It 'detects cluster-api version mismatch in e2e config'
    e2econfig_set_capi 'v1.9.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'cluster-api version mismatch'
    The output should include 'e2e config'
  End

  It 'detects k8s.io/code-generator replace version mismatch'
    gomod_add_replace 'v0.31.0' 'k8s.io/code-generator' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'k8s.io/code-generator version mismatch'
  End
End
