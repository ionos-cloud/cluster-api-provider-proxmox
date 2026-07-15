Describe 'helpers.sh — pure functions'
  Include '../helpers.sh'

  Describe 'ensure_v_prefix'
    It 'adds v prefix when missing'
      When call ensure_v_prefix '1.2.3'
      The output should equal 'v1.2.3'
    End

    It 'keeps existing v prefix'
      When call ensure_v_prefix 'v1.2.3'
      The output should equal 'v1.2.3'
    End
  End

  Describe 'strip_v_prefix'
    It 'removes v prefix'
      When call strip_v_prefix 'v1.2.3'
      The output should equal '1.2.3'
    End

    It 'returns unchanged when no v prefix'
      When call strip_v_prefix '1.2.3'
      The output should equal '1.2.3'
    End
  End

  Describe 'validate_semver'
    It 'accepts valid semver'
      When call validate_semver '1.2.3'
      The status should be success
    End

    It 'accepts valid semver with v prefix'
      When call validate_semver 'v1.2.3'
      The status should be success
    End

    It 'rejects missing patch'
      When run validate_semver '1.2'
      The status should be failure
      The error should include 'invalid version format'
    End

    It 'rejects non-numeric'
      When run validate_semver 'abc'
      The status should be failure
      The error should include 'invalid version format'
    End
  End

  Describe 'validate_sha256'
    It 'accepts a valid digest'
      When call validate_sha256 'cafebabecafebabecafebabecafebabecafebabecafebabecafebabecafebabe'
      The status should be success
    End

    It 'accepts a valid digest with sha256: prefix'
      When call validate_sha256 'sha256:cafebabecafebabecafebabecafebabecafebabecafebabecafebabecafebabe'
      The status should be success
    End

    It 'rejects a malformed digest'
      When run validate_sha256 'not-a-digest'
      The status should be failure
      The error should include 'invalid sha256 digest'
    End
  End

  Describe 'validate_capi'
    It 'accepts a valid contract'
      When call validate_capi 'v1beta2'
      The status should be success
    End

    It 'rejects an invalid contract'
      When run validate_capi 'beta2'
      The status should be failure
      The error should include 'invalid cluster-api contract'
    End
  End

  Describe 'split_version'
    It 'splits version into MAJOR MINOR PATCH'
      When call split_version 'v1.10.4'
      The variable MAJOR should equal '1'
      The variable MINOR should equal '10'
      The variable PATCH should equal '4'
    End

    It 'handles version without v prefix'
      When call split_version '0.32.3'
      The variable MAJOR should equal '0'
      The variable MINOR should equal '32'
      The variable PATCH should equal '3'
    End

    It 'ignores a pre-release suffix'
      When call split_version 'v1.11.0-rc.0'
      The variable MAJOR should equal '1'
      The variable MINOR should equal '11'
      The variable PATCH should equal '0'
    End
  End
End
