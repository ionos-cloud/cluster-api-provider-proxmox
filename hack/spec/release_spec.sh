Describe 'release.sh'
  setup() { setup_fixture_repo; setup_go_mock; }
  cleanup() { cleanup_fixture_repo; cleanup_go_mock; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  Describe 'patch bump (existing major.minor)'
    It 'reports clusterctl + sonar updates'
      When run script ../release.sh 0.8.2
      The status should be success
      The output should include 'clusterctl-settings.json: Updated nextVersion v0.8.1 to v0.8.2'
      The output should include 'sonar-project.properties: Updated sonar.projectVersion 0.8.1 to 0.8.2'
    End

    It 'writes nextVersion to clusterctl-settings.json'
      bash ../release.sh 0.8.2 >/dev/null 2>&1
      When call clusterctl_get_version
      The output should equal 'v0.8.2'
    End

    It 'writes projectVersion to sonar-project.properties'
      bash ../release.sh 0.8.2 >/dev/null 2>&1
      When call sonar_get_version
      The output should equal '0.8.2'
    End

    It 'does not touch metadata.yaml'
      When run script ../release.sh 0.8.2
      The status should be success
      The output should not include 'metadata.yaml'
    End

    It 'does not touch the e2e sentinel'
      bash ../release.sh 0.8.2 >/dev/null 2>&1
      When call e2econfig_get_capmox
      The output should equal 'v0.8.99'
    End

    It 'accepts a v-prefixed input'
      When run script ../release.sh v0.8.2
      The status should be success
      The output should include 'Updated nextVersion v0.8.1 to v0.8.2'
    End
  End

  Describe 'minor bump (new major.minor)'
    It 'appends a new releaseSeries entry with the latest contract'
      When run script ../release.sh 0.9.0
      The status should be success
      The output should include 'metadata.yaml: Added releaseSeries entry for v0.9 (v1beta2)'
    End

    It 'accepts an explicit contract override'
      When run script ../release.sh 0.9.0 v1beta3
      The status should be success
      The output should include 'metadata.yaml: Added releaseSeries entry for v0.9 (v1beta3)'
    End

    It 'bumps the e2e sentinel to the new major.minor'
      bash ../release.sh 0.9.0 >/dev/null 2>&1
      When call e2econfig_get_capmox
      The output should equal 'v0.9.99'
    End

    It 'reports the e2e sentinel update'
      When run script ../release.sh 0.9.0
      The status should be success
      The output should include 'Updated capmox v0.8.99 to v0.9.99'
    End
  End

  Describe 'pre-release (suffix stripped for metadata+sentinel)'
    It 'writes the full v-prefixed version (suffix preserved) to clusterctl'
      bash ../release.sh 0.9.0-rc.0 >/dev/null 2>&1
      When call clusterctl_get_version
      The output should equal 'v0.9.0-rc.0'
    End

    It 'writes the full version (no v, suffix preserved) to sonar'
      bash ../release.sh 0.9.0-rc.0 >/dev/null 2>&1
      When call sonar_get_version
      The output should equal '0.9.0-rc.0'
    End

    It 'adds releaseSeries entry for the core major.minor'
      bash ../release.sh 0.9.0-rc.0 >/dev/null 2>&1
      When call metadata_has_release 0 9
      The status should be success
    End

    It 'bumps the e2e sentinel using the core major.minor'
      bash ../release.sh 0.9.0-rc.0 >/dev/null 2>&1
      When call e2econfig_get_capmox
      The output should equal 'v0.9.99'
    End
  End

  Describe 'input validation'
    It 'fails without arguments'
      When run script ../release.sh
      The status should be failure
      The output should include 'Usage:'
    End

    It 'fails with too many arguments'
      When run script ../release.sh 0.9.0 v1beta2 extra
      The status should be failure
      The output should include 'Usage:'
    End

    It 'fails with an invalid version'
      When run script ../release.sh 'not-a-version'
      The status should be failure
      The output should include 'invalid version format'
    End
  End
End
