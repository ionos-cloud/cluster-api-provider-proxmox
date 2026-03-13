Describe 'helpers.sh — go.mod functions'
  setup() { setup_fixture_repo; setup_go_mock; }
  cleanup() { cleanup_fixture_repo; cleanup_go_mock; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  Describe 'gomod_get_go'
    It 'returns the Go version from go.mod'
      When call gomod_get_go
      The output should equal '1.25.0'
    End
  End

  Describe 'gomod_get_require'
    It 'returns version for an existing package'
      When call gomod_get_require 'k8s.io/api'
      The output should equal 'v0.32.3'
    End

    It 'returns empty for a missing package'
      When call gomod_get_require 'example.com/nonexistent'
      The output should equal ''
    End

    It 'returns cluster-api version'
      When call gomod_get_require 'sigs.k8s.io/cluster-api'
      The output should equal 'v1.10.4'
    End

    It 'returns cluster-api/test version'
      When call gomod_get_require 'sigs.k8s.io/cluster-api/test'
      The output should equal 'v1.10.4'
    End
  End

  Describe 'gomod_get_replace'
    It 'returns version for an existing replace directive'
      When call gomod_get_replace 'sigs.k8s.io/cluster-api'
      The output should equal 'v1.10.4'
    End

    It 'returns golangci-lint replace version'
      When call gomod_get_replace 'github.com/golangci/golangci-lint/v2'
      The output should equal 'v2.9.0'
    End

    It 'returns k8s.io/apimachinery replace version'
      When call gomod_get_replace 'k8s.io/apimachinery'
      The output should equal 'v0.32.3'
    End

    It 'returns empty for a package without replace'
      When call gomod_get_replace 'k8s.io/api'
      The output should equal ''
    End
  End

  Describe 'gomod_get_version'
    It 'returns replace version when replace exists'
      When call gomod_get_version 'sigs.k8s.io/cluster-api'
      The output should equal 'v1.10.4'
    End

    It 'returns require version when no replace exists'
      When call gomod_get_version 'k8s.io/api'
      The output should equal 'v0.32.3'
    End
  End

  Describe 'gomod_has_version_match'
    It 'returns true when all packages have the same version'
      When call gomod_has_version_match 'k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go'
      The status should be success
    End

    It 'returns false when packages have different versions'
      gomod_set_require 'v0.33.0' 'k8s.io/api' >/dev/null
      When call gomod_has_version_match 'k8s.io/api' 'k8s.io/apimachinery'
      The status should be failure
    End
  End

  Describe 'gomod_make_envtest'
    It 'derives ENVTEST version from k8s.io/api'
      When call gomod_make_envtest
      The output should equal '1.32'
    End
  End

  Describe 'gomod_set_go'
    It 'updates the Go version'
      When call gomod_set_go '1.26.0'
      The output should include 'Updated go 1.25.0 to 1.26.0'
    End

    It 'writes the new version to the file'
      gomod_set_go '1.26.0' >/dev/null
      When call gomod_get_go
      The output should equal '1.26.0'
    End
  End

  Describe 'gomod_set_require'
    It 'updates a require version'
      When call gomod_set_require 'v0.33.0' 'k8s.io/api'
      The output should include 'Updated require k8s.io/api v0.32.3 to v0.33.0'
    End

    It 'writes the new version to the file'
      gomod_set_require 'v0.33.0' 'k8s.io/api' >/dev/null
      When call gomod_get_require 'k8s.io/api'
      The output should equal 'v0.33.0'
    End

    It 'updates multiple packages at once'
      When call gomod_set_require 'v0.33.0' 'k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go'
      The output should include 'Updated require k8s.io/api'
      The output should include 'Updated require k8s.io/apimachinery'
      The output should include 'Updated require k8s.io/client-go'
    End
  End

  Describe 'gomod_add_replace'
    It 'updates an existing replace version'
      When call gomod_add_replace 'v1.11.0' 'sigs.k8s.io/cluster-api'
      The output should include 'Updated replace sigs.k8s.io/cluster-api v1.10.4 to v1.11.0'
    End

    It 'writes the new version to the file'
      gomod_add_replace 'v1.11.0' 'sigs.k8s.io/cluster-api' >/dev/null
      When call gomod_get_replace 'sigs.k8s.io/cluster-api'
      The output should equal 'v1.11.0'
    End

    It 'adds a replace directive for a package without one'
      When call gomod_add_replace 'v0.33.0' 'k8s.io/api'
      The output should include 'Added replace k8s.io/api => k8s.io/api v0.33.0'
    End

    It 'the added directive is readable'
      gomod_add_replace 'v0.33.0' 'k8s.io/api' >/dev/null
      When call gomod_get_replace 'k8s.io/api'
      The output should equal 'v0.33.0'
    End

    It 'adds multiple packages at once'
      When call gomod_add_replace 'v0.33.0' 'k8s.io/api' 'k8s.io/client-go'
      The output should include 'Added replace k8s.io/api'
      The output should include 'Added replace k8s.io/client-go'
    End
  End
End
