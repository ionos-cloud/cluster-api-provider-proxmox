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

  It 'detects golangci-lint version mismatch in .custom-gcl.yaml'
    customgcl_set_version 'v2.8.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'golangci-lint version mismatch'
  End

  It 'detects golangci-lint require/replace drift (dependabot bump)'
    gomod_set_require 'v2.10.0' 'github.com/golangci/golangci-lint/v2' >/dev/null
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

  It 'detects capmox clusterctl/sonar drift'
    sonar_set_version '0.8.2' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'capmox version mismatch'
  End

  It 'detects capmox major.minor not listed in metadata.yaml'
    clusterctl_set_version 'v0.99.0' >/dev/null
    sonar_set_version '0.99.0' >/dev/null
    e2econfig_set_capmox 'v0.99.99' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'v0.99 is not listed in metadata.yaml'
  End

  It 'detects capmox e2e sentinel mismatch'
    e2econfig_set_capmox 'v0.7.99' >/dev/null
    When run script ../verify-versions.sh
    The status should be failure
    The output should include 'capmox e2e sentinel mismatch'
  End

  It 'accepts a pre-release clusterctl version whose core is in metadata'
    clusterctl_set_version 'v0.8.2-rc.0' >/dev/null
    sonar_set_version '0.8.2-rc.0' >/dev/null
    When run script ../verify-versions.sh
    The status should be success
  End
End
