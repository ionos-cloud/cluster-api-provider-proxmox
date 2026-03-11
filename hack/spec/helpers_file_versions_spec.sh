Describe 'helpers.sh — file version functions'
  setup() { setup_fixture_repo; }
  cleanup() { cleanup_fixture_repo; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  Describe 'dockerfile_go_version'
    It 'returns the Go major.minor from Dockerfile'
      When call dockerfile_go_version
      The output should equal '1.25'
    End
  End

  Describe 'dockerfile_set_go_version'
    It 'updates the Dockerfile Go version'
      When call dockerfile_set_go_version '1.26'
      The output should include 'Updated golang:1.25 to golang:1.26'
    End

    It 'writes the new version to the file'
      dockerfile_set_go_version '1.26' >/dev/null
      When call dockerfile_go_version
      The output should equal '1.26'
    End
  End

  Describe 'docs_go_version'
    It 'returns the Go major.minor from docs'
      When call docs_go_version
      The output should equal '1.25'
    End
  End

  Describe 'docs_set_go_version'
    It 'updates the docs Go version'
      When call docs_set_go_version '1.26'
      The output should include 'Updated Go v1.25 to Go v1.26'
    End

    It 'writes the new version to the file'
      docs_set_go_version '1.26' >/dev/null
      When call docs_go_version
      The output should equal '1.26'
    End
  End

  Describe 'custom_gcl_version'
    It 'returns the golangci-lint version'
      When call custom_gcl_version
      The output should equal 'v2.9.0'
    End
  End

  Describe 'custom_gcl_set_version'
    It 'updates the golangci-lint version'
      When call custom_gcl_set_version 'v2.10.0'
      The output should include 'Updated golangci-lint v2.9.0 to v2.10.0'
    End

    It 'writes the new version to the file'
      custom_gcl_set_version 'v2.10.0' >/dev/null
      When call custom_gcl_version
      The output should equal 'v2.10.0'
    End
  End

  Describe 'makefile_envtest_version'
    It 'returns the ENVTEST_K8S_VERSION'
      When call makefile_envtest_version
      The output should equal '1.32.3'
    End
  End

  Describe 'makefile_set_envtest_version'
    It 'updates the ENVTEST_K8S_VERSION'
      When call makefile_set_envtest_version '1.33.0'
      The output should include 'Updated ENVTEST_K8S_VERSION 1.32.3 to 1.33.0'
    End

    It 'writes the new version to the file'
      makefile_set_envtest_version '1.33.0' >/dev/null
      When call makefile_envtest_version
      The output should equal '1.33.0'
    End
  End
End
