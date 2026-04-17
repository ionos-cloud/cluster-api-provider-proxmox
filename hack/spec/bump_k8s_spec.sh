Describe 'bump-k8s.sh'
  setup() { setup_fixture_repo; setup_go_mock; }
  cleanup() { cleanup_fixture_repo; cleanup_go_mock; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  It 'bumps k8s packages in go.mod'
    When run script ../bump-k8s.sh 0.33.0
    The status should be success
    The output should include 'go.mod: Updated require k8s.io/api v0.32.3 to v0.33.0'
  End

  It 'updates go.mod k8s.io/api version'
    bash ../bump-k8s.sh 0.33.0 >/dev/null 2>&1
    When call gomod_get_require 'k8s.io/api'
    The output should equal 'v0.33.0'
  End

  It 'removes existing k8s.io replace directives'
    bash ../bump-k8s.sh 0.33.0 >/dev/null 2>&1
    When call gomod_get_replace 'k8s.io/apimachinery'
    The output should equal ''
  End

  It 'reports removal of k8s.io replace directive'
    When run script ../bump-k8s.sh 0.33.0
    The status should be success
    The output should include 'go.mod: Removed replace k8s.io/apimachinery'
  End

  It 'updates k8s.io/code-generator replace to match k8s version'
    bash ../bump-k8s.sh 0.33.0 >/dev/null 2>&1
    When call gomod_get_replace 'k8s.io/code-generator'
    The output should equal 'v0.33.0'
  End

  It 'reports k8s.io/code-generator replace update'
    When run script ../bump-k8s.sh 0.33.0
    The status should be success
    The output should include 'Updated replace k8s.io/code-generator'
  End

  It 'does not modify ENVTEST_K8S_VERSION derivation'
    bash ../bump-k8s.sh 0.33.0 >/dev/null 2>&1
    When call makefile_get_envtest
    The output should equal '1.33'
  End

  It 'updates KUBERNETES_VERSION in e2e config'
    When run script ../bump-k8s.sh 0.33.0
    The status should be success
    The output should include 'Updated KUBERNETES_VERSION v1.32.3 to v1.33.0'
  End

  It 'writes the correct KUBERNETES_VERSION to e2e config'
    bash ../bump-k8s.sh 0.33.0 >/dev/null 2>&1
    When call e2econfig_get_k8s
    The output should equal 'v1.33.0'
  End

  It 'updates --kubernetes-version in docs'
    When run script ../bump-k8s.sh 0.33.0
    The status should be success
    The output should include 'Updated --kubernetes-version references to v1.33.0'
  End

  It 'writes the correct --kubernetes-version to docs'
    bash ../bump-k8s.sh 0.33.0 >/dev/null 2>&1
    When call docs_get_k8s
    The output should equal 'v1.33.0'
  End

  It 'rejects non-zero major version'
    When run script ../bump-k8s.sh 1.33.0
    The status should be failure
    The output should include 'k8s.io packages use major version 0'
  End

  It 'fails without arguments'
    When run script ../bump-k8s.sh
    The status should be failure
    The output should include 'Usage:'
  End
End
