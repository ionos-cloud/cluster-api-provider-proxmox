Describe 'helpers.sh — metadata functions'
  setup() { setup_fixture_repo; }
  cleanup() { cleanup_fixture_repo; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  Describe 'metadata_latest_contract'
    It 'returns the contract of the highest major.minor entry'
      When call metadata_latest_contract
      The output should equal 'v1beta2'
    End

    # Helper: write a two-entry metadata.yaml and assert the winner.
    # Usage: write_metadata_and_check winner_major winner_minor loser_major loser_minor expected
    write_metadata_and_check() {
      cat > "${METADATA_FILE}" <<YAML
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
  - major: $3
    minor: $4
    contract: loser
  - major: $1
    minor: $2
    contract: winner
YAML
    }

    It 'picks higher minor when major=0 (regression)'
      write_metadata_and_check 0 2 0 1
      When call metadata_latest_contract
      The output should equal 'winner'
    End

    It 'picks higher minor when major=0 and loser has minor=0'
      write_metadata_and_check 0 1 0 0
      When call metadata_latest_contract
      The output should equal 'winner'
    End

    It 'picks major=1 over major=0 even when minor is lower'
      write_metadata_and_check 1 0 0 2
      When call metadata_latest_contract
      The output should equal 'winner'
    End

    It 'picks major=2 over major=1 even when minor is lower'
      write_metadata_and_check 2 0 1 2
      When call metadata_latest_contract
      The output should equal 'winner'
    End

    It 'picks higher minor when major=2'
      write_metadata_and_check 2 2 2 1
      When call metadata_latest_contract
      The output should equal 'winner'
    End

    It 'handles single entry with major=0 minor=0'
      cat > "${METADATA_FILE}" <<YAML
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
  - major: 0
    minor: 0
    contract: only
YAML
      When call metadata_latest_contract
      The output should equal 'only'
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
