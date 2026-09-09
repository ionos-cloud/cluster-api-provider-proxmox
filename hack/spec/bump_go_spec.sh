Describe 'bump-go.sh'
  setup() { setup_fixture_repo; setup_go_mock; setup_docker_mock; return; }
  cleanup() { cleanup_docker_mock; cleanup_go_mock; cleanup_fixture_repo; return; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  It 'bumps Go version in all files'
    When run script ../bump-go.sh 1.26.0
    The status should be success
    The output should include 'go.mod: Updated go 1.25.0 to 1.26.0'
    The output should include 'Dockerfile: Updated golang:1.25 to golang:1.26'
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
    The error should include 'Usage:'
  End

  It 'fails with too many arguments'
    When run script ../bump-go.sh 1.26.0 sha256:cafebabe extra
    The status should be failure
    The error should include 'Usage:'
  End

  It 'fails with invalid version'
    When run script ../bump-go.sh 'not-a-version'
    The status should be failure
    The error should include 'invalid version format'
  End

  It 'fails with an invalid digest'
    When run script ../bump-go.sh 1.26.0 not-a-digest
    The status should be failure
    The error should include 'invalid sha256 digest'
  End

  It 'resolves and updates the test workflow image for the active minor'
    When run script ../bump-go.sh 1.25.3
    The status should be success
    The output should include '.github/workflows/test.yml: Updated golang@sha256:abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd (1.25.0-trixie) to golang@sha256:cafebabecafebabecafebabecafebabecafebabecafebabecafebabecafebabe (1.25.3-trixie)'
  End

  It 'does not touch the test workflow image on a Go minor bump'
    When run script ../bump-go.sh 1.26.0
    The status should be success
    The output should not include 'test.yml'
  End

  It 'uses a given digest directly, without invoking docker'
    printf '#!/usr/bin/env bash\nexit 1\n' > "${DOCKER_MOCK_DIR}/docker"
    When run script ../bump-go.sh 1.25.3 sha256:1234123412341234123412341234123412341234123412341234123412341234
    The status should be success
    The output should include 'to golang@sha256:1234123412341234123412341234123412341234123412341234123412341234'
  End
End
