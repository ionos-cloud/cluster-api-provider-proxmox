Describe 'helpers.sh — pure functions'
  Include '../helpers.sh'

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
End
