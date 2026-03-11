Describe 'bump-k8s.sh'
  setup() { setup_fixture_repo; setup_go_mock; }
  cleanup() { cleanup_fixture_repo; cleanup_go_mock; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  It 'bumps k8s packages and ENVTEST version'
    When run script ../bump-k8s.sh 0.33.0
    The status should be success
    The output should include 'go.mod: Updated require k8s.io/api v0.32.3 to v0.33.0'
    The output should include 'Makefile: Updated ENVTEST_K8S_VERSION 1.32.3 to 1.33.0'
  End

  It 'updates go.mod k8s.io/api version'
    bash ../bump-k8s.sh 0.33.0 >/dev/null 2>&1
    When call gomod_require_version 'k8s.io/api'
    The output should equal 'v0.33.0'
  End

  It 'updates Makefile ENVTEST version'
    bash ../bump-k8s.sh 0.33.0 >/dev/null 2>&1
    When call makefile_envtest_version
    The output should equal '1.33.0'
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
