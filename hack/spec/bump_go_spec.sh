Describe 'bump-go.sh'
  setup() { setup_fixture_repo; setup_go_mock; }
  cleanup() { cleanup_fixture_repo; cleanup_go_mock; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  It 'bumps Go version in all files'
    When run script ../bump-go.sh 1.26.0
    The status should be success
    The output should include 'go.mod: Updated go 1.25.0 to 1.26.0'
    The output should include 'Dockerfile: Updated golang:1.25 to golang:1.26'
    The output should include 'docs/Development.md: Updated Go v1.25 to Go v1.26'
    The output should include '.golangci-kal.yml: Updated Go 1.25 to 1.26'
  End

  It 'updates .golangci-kal.yml run.go'
    bash ../bump-go.sh 1.26.0 >/dev/null 2>&1
    When call golangcikal_get_go
    The output should equal '1.26'
  End

  It 'updates go.mod go directive'
    bash ../bump-go.sh 1.26.0 >/dev/null 2>&1
    When call gomod_get_go
    The output should equal '1.26.0'
  End

  It 'fails without arguments'
    When run script ../bump-go.sh
    The status should be failure
    The output should include 'Usage:'
  End

  It 'fails with invalid version'
    When run script ../bump-go.sh 'not-a-version'
    The status should be failure
    The output should include 'invalid version format'
  End
End
