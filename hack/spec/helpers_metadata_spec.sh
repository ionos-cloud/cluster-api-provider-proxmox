Describe 'helpers.sh — metadata functions'
  setup() { setup_fixture_repo; return; }
  cleanup() { cleanup_fixture_repo; return; }
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
      return
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
    It 'returns success for an existing capmox entry'
      When call metadata_has_release 0 8
      The status should be success
    End

    It 'returns failure for a missing capmox entry'
      When call metadata_has_release 0 99
      The status should be failure
    End
  End

  Describe 'metadata_add_release'
    It 'adds a new capmox releaseSeries entry'
      When call metadata_add_release 0 9 'v1beta2'
      The output should include 'metadata.yaml: Added releaseSeries entry for v0.9'
    End

    It 'makes the new entry discoverable'
      metadata_add_release 0 9 'v1beta2' >/dev/null
      When call metadata_has_release 0 9
      The status should be success
    End

    It 'appends (chronological order, newest last)'
      metadata_add_release 0 9 'v1beta2' >/dev/null
      _last_minor() { yq '.releaseSeries[-1].minor' "${METADATA_FILE}"; return; }
      When call _last_minor
      The output should equal '9'
    End
  End

  Describe 'e2emetadata_contracts'
    It 'lists one contract per catalog directory'
      When call e2emetadata_contracts
      The output should include 'v1beta1'
      The output should include 'v1beta2'
    End
  End

  Describe 'e2emetadata_contract_has_release'
    It 'returns success for an entry present in the named catalog'
      When call e2emetadata_contract_has_release 1 10 v1beta2
      The status should be success
    End

    It 'returns failure for an entry absent from the named catalog'
      When call e2emetadata_contract_has_release 1 99 v1beta2
      The status should be failure
    End
  End

  Describe 'e2emetadata_has_release'
    It 'returns success when the entry is in every catalog'
      When call e2emetadata_has_release v1.10.0
      The status should be success
    End

    It 'fails and reports the catalogs missing the entry'
      When call e2emetadata_has_release v1.11.0
      The status should be failure
      The error should include 'missing from e2e metadata catalog(s): v1beta1 v1beta2'
    End

    It 'fails when the entry is present in only some catalogs'
      e2emetadata_contract_add_release 1 11 v1beta2 >/dev/null
      When call e2emetadata_has_release v1.11.0
      The status should be failure
      The error should include 'v1beta1'
    End
  End

  Describe 'e2emetadata_add_release'
    It 'adds the entry to every contract catalog'
      When call e2emetadata_add_release v1.11.0
      The output should include 'test/e2e/data/shared/v1beta1/metadata.yaml: Added releaseSeries entry for v1.11 (v1beta1)'
      The output should include 'test/e2e/data/shared/v1beta2/metadata.yaml: Added releaseSeries entry for v1.11 (v1beta2)'
    End

    It 'makes the new entry discoverable in every catalog'
      e2emetadata_add_release v1.11.0 >/dev/null
      When call e2emetadata_has_release v1.11.0
      The status should be success
    End

    It 'is idempotent (skips catalogs that already have it)'
      e2emetadata_add_release v1.11.0 >/dev/null
      When call e2emetadata_add_release v1.11.0
      The output should equal ''
    End

    It 'prepends within a catalog (newest first)'
      e2emetadata_add_release v1.11.0 >/dev/null
      _first_minor() { yq '.releaseSeries[0].minor' "${REPO_ROOT}/test/e2e/data/shared/v1beta2/metadata.yaml"; return; }
      When call _first_minor
      The output should equal '11'
    End
  End
End
