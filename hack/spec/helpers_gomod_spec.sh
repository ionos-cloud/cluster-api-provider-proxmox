Describe 'helpers.sh — go.mod functions'
  setup() { setup_fixture_repo; }
  cleanup() { cleanup_fixture_repo; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  Describe 'gomod_go_version'
    It 'returns the Go version from go.mod'
      When call gomod_go_version
      The output should equal '1.25.0'
    End
  End

  Describe 'gomod_require_version'
    It 'returns version for an existing package'
      When call gomod_require_version 'k8s.io/api'
      The output should equal 'v0.32.3'
    End

    It 'returns empty for a missing package'
      When call gomod_require_version 'example.com/nonexistent'
      The output should equal ''
    End

    It 'returns cluster-api version'
      When call gomod_require_version 'sigs.k8s.io/cluster-api'
      The output should equal 'v1.10.4'
    End

    It 'returns cluster-api/test version'
      When call gomod_require_version 'sigs.k8s.io/cluster-api/test'
      The output should equal 'v1.10.4'
    End
  End

  Describe 'gomod_replace_version'
    It 'returns version for an existing replace directive'
      When call gomod_replace_version 'sigs.k8s.io/cluster-api'
      The output should equal 'v1.10.4'
    End

    It 'returns golangci-lint replace version'
      When call gomod_replace_version 'github.com/golangci/golangci-lint/v2'
      The output should equal 'v2.9.0'
    End

    It 'returns empty for a package without replace'
      When call gomod_replace_version 'k8s.io/api'
      The output should equal ''
    End
  End

  Describe 'effective_version'
    It 'returns replace version when replace exists'
      When call effective_version 'sigs.k8s.io/cluster-api'
      The output should equal 'v1.10.4'
    End

    It 'returns require version when no replace exists'
      When call effective_version 'k8s.io/api'
      The output should equal 'v0.32.3'
    End
  End

  Describe 'gomod_set_go_version'
    It 'updates the Go version'
      When call gomod_set_go_version '1.26.0'
      The output should include 'Updated go 1.25.0 to 1.26.0'
    End

    It 'writes the new version to the file'
      gomod_set_go_version '1.26.0' >/dev/null
      When call gomod_go_version
      The output should equal '1.26.0'
    End
  End

  Describe 'gomod_set_require_version'
    It 'updates a require version'
      When call gomod_set_require_version 'k8s.io/api' 'v0.33.0'
      The output should include 'Updated require k8s.io/api v0.32.3 to v0.33.0'
    End

    It 'writes the new version to the file'
      gomod_set_require_version 'k8s.io/api' 'v0.33.0' >/dev/null
      When call gomod_require_version 'k8s.io/api'
      The output should equal 'v0.33.0'
    End
  End

  Describe 'gomod_set_replace_version'
    It 'updates a replace version'
      When call gomod_set_replace_version 'sigs.k8s.io/cluster-api' 'v1.11.0'
      The output should include 'Updated replace sigs.k8s.io/cluster-api v1.10.4 to v1.11.0'
    End

    It 'writes the new version to the file'
      gomod_set_replace_version 'sigs.k8s.io/cluster-api' 'v1.11.0' >/dev/null
      When call gomod_replace_version 'sigs.k8s.io/cluster-api'
      The output should equal 'v1.11.0'
    End
  End
End
