Describe 'helpers.sh — metadata functions'
  setup() { setup_fixture_repo; }
  cleanup() { cleanup_fixture_repo; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  Describe 'metadata_latest_contract'
    It 'returns the contract of the highest major.minor entry'
      When call metadata_latest_contract
      The output should equal 'v1beta1'
    End
  End

  Describe 'metadata_has_release'
    It 'returns success for an existing entry'
      When call metadata_has_release 1 10
      The status should be success
    End

    It 'returns failure for a missing entry'
      When call metadata_has_release 1 99
      The status should be failure
    End
  End

  Describe 'metadata_add_release'
    It 'adds a new releaseSeries entry'
      When call metadata_add_release 1 11 'v1beta2'
      The output should include 'Added releaseSeries entry for v1.11'
    End

    It 'makes the new entry discoverable'
      metadata_add_release 1 11 'v1beta2' >/dev/null
      When call metadata_has_release 1 11
      The status should be success
    End
  End
End
