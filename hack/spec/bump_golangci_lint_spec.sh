Describe 'bump-golangci-lint.sh'
  setup() { setup_fixture_repo; setup_go_mock; }
  cleanup() { cleanup_fixture_repo; cleanup_go_mock; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  It 'bumps golangci-lint version'
    When run script ../bump-golangci-lint.sh v2.10.0
    The status should be success
    The output should include 'go.mod: Updated replace github.com/golangci/golangci-lint/v2 v2.9.0 to v2.10.0'
    The output should include '.custom-gcl.yaml: Updated golangci-lint v2.9.0 to v2.10.0'
  End

  It 'updates go.mod replace version'
    bash ../bump-golangci-lint.sh v2.10.0 >/dev/null 2>&1
    When call gomod_replace_version 'github.com/golangci/golangci-lint/v2'
    The output should equal 'v2.10.0'
  End

  It 'updates .custom-gcl.yaml version'
    bash ../bump-golangci-lint.sh v2.10.0 >/dev/null 2>&1
    When call custom_gcl_version
    The output should equal 'v2.10.0'
  End

  It 'fails without arguments'
    When run script ../bump-golangci-lint.sh
    The status should be failure
    The output should include 'Usage:'
  End
End
