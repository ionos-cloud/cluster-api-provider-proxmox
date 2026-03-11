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
      The output should include 'invalid version format'
    End

    It 'rejects non-numeric'
      When run validate_semver 'abc'
      The status should be failure
      The output should include 'invalid version format'
    End
  End

  Describe 'validate_go_version'
    It 'accepts major.minor'
      When call validate_go_version '1.25'
      The status should be success
    End

    It 'accepts major.minor.patch'
      When call validate_go_version '1.25.0'
      The status should be success
    End

    It 'rejects single number'
      When run validate_go_version '1'
      The status should be failure
      The output should include 'invalid version format'
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
  End

  Describe 'versions_differ'
    It 'returns true when versions differ'
      When call versions_differ 'v1.0.0' 'v2.0.0'
      The status should be success
    End

    It 'returns false when versions are the same'
      When call versions_differ 'v1.0.0' 'v1.0.0'
      The status should be failure
    End

    It 'returns false when first arg is empty'
      When call versions_differ '' 'v1.0.0'
      The status should be failure
    End

    It 'returns false when second arg is empty'
      When call versions_differ 'v1.0.0' ''
      The status should be failure
    End
  End
End
