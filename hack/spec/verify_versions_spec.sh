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
End
