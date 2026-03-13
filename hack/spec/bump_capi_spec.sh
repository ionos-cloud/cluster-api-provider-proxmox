Describe 'bump-capi.sh'
  setup() { setup_fixture_repo; setup_go_mock; }
  cleanup() { cleanup_fixture_repo; cleanup_go_mock; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  It 'bumps cluster-api version'
    When run script ../bump-capi.sh 1.11.0 v1beta2
    The status should be success
    The output should include 'go.mod: Updated require sigs.k8s.io/cluster-api v1.10.4 to v1.11.0'
    The output should include 'go.mod: Updated replace sigs.k8s.io/cluster-api v1.10.4 to v1.11.0'
    The output should include 'Added releaseSeries entry for v1.11'
  End

  It 'updates go.mod require version'
    bash ../bump-capi.sh 1.11.0 v1beta2 >/dev/null 2>&1
    When call gomod_get_require 'sigs.k8s.io/cluster-api'
    The output should equal 'v1.11.0'
  End

  It 'updates cluster-api version in e2e config'
    When run script ../bump-capi.sh 1.11.0 v1beta2
    The status should be success
    The output should include 'Updated cluster-api v1.10.4 to v1.11.0'
  End

  It 'writes the correct cluster-api version to e2e config'
    bash ../bump-capi.sh 1.11.0 v1beta2 >/dev/null 2>&1
    When call e2econfig_get_capi
    The output should equal 'v1.11.0'
  End

  It 'does not duplicate existing metadata entry'
    When run script ../bump-capi.sh 1.10.5 v1beta1
    The status should be success
    The output should not include 'Added releaseSeries'
  End

  It 'fails without arguments'
    When run script ../bump-capi.sh
    The status should be failure
    The output should include 'Usage:'
  End

  It 'fails with invalid version'
    When run script ../bump-capi.sh 'bad' v1beta1
    The status should be failure
    The output should include 'invalid version format'
  End
End
