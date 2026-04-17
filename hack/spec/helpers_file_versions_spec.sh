Describe 'helpers.sh — file version functions'
  setup() { setup_fixture_repo; }
  cleanup() { cleanup_fixture_repo; }
  BeforeEach 'setup'
  AfterEach 'cleanup'

  Include '../helpers.sh'

  Describe 'dockerfile_get_go'
    It 'returns the Go major.minor from Dockerfile'
      When call dockerfile_get_go
      The output should equal '1.25'
    End
  End

  Describe 'dockerfile_set_go'
    It 'updates the Dockerfile Go version'
      When call dockerfile_set_go '1.26'
      The output should include 'Updated golang:1.25 to golang:1.26'
    End

    It 'writes the new version to the file'
      dockerfile_set_go '1.26' >/dev/null
      When call dockerfile_get_go
      The output should equal '1.26'
    End
  End

  Describe 'docs_get_go'
    It 'returns the Go major.minor from docs'
      When call docs_get_go
      The output should equal '1.25'
    End
  End

  Describe 'docs_set_go'
    It 'updates the docs Go version'
      When call docs_set_go '1.26'
      The output should include 'Updated Go v1.25 to Go v1.26'
    End

    It 'writes the new version to the file'
      docs_set_go '1.26' >/dev/null
      When call docs_get_go
      The output should equal '1.26'
    End
  End

  Describe 'golangcikal_get_go'
    It 'returns the Go major.minor from .golangci-kal.yml'
      When call golangcikal_get_go
      The output should equal '1.25'
    End
  End

  Describe 'golangcikal_set_go'
    It 'updates the .golangci-kal.yml Go version'
      When call golangcikal_set_go '1.26'
      The output should include 'Updated Go 1.25 to 1.26'
    End

    It 'writes the new version to the file'
      golangcikal_set_go '1.26' >/dev/null
      When call golangcikal_get_go
      The output should equal '1.26'
    End

    It 'preserves double-quoted string style'
      golangcikal_set_go '1.26' >/dev/null
      When call cat "${REPO_ROOT}/.golangci-kal.yml"
      The output should include 'go: "1.26"'
    End
  End

  Describe 'customgcl_get_version'
    It 'returns the golangci-lint version'
      When call customgcl_get_version
      The output should equal 'v2.9.0'
    End
  End

  Describe 'customgcl_set_version'
    It 'updates the golangci-lint version'
      When call customgcl_set_version 'v2.10.0'
      The output should include 'Updated golangci-lint v2.9.0 to v2.10.0'
    End

    It 'writes the new version to the file'
      customgcl_set_version 'v2.10.0' >/dev/null
      When call customgcl_get_version
      The output should equal 'v2.10.0'
    End
  End

  Describe 'makefile_get_envtest'
    It 'returns the computed ENVTEST_K8S_VERSION via make'
      When call makefile_get_envtest
      The output should equal '1.32'
    End
  End

  Describe 'e2econfig_get_k8s'
    It 'returns the KUBERNETES_VERSION default from e2e config'
      When call e2econfig_get_k8s
      The output should equal 'v1.32.3'
    End
  End

  Describe 'e2econfig_set_k8s'
    It 'updates the KUBERNETES_VERSION in e2e config files'
      When call e2econfig_set_k8s 'v1.33.0'
      The output should include 'Updated KUBERNETES_VERSION v1.32.3 to v1.33.0'
    End

    It 'writes the new version to both files'
      e2econfig_set_k8s 'v1.33.0' >/dev/null
      When call e2econfig_get_k8s
      The output should equal 'v1.33.0'
    End
  End

  Describe 'e2econfig_get_capi'
    It 'returns the cluster-api version from e2e config'
      When call e2econfig_get_capi
      The output should equal 'v1.10.4'
    End
  End

  Describe 'e2econfig_get_capmox'
    It 'returns the capmox sentinel from e2e config'
      When call e2econfig_get_capmox
      The output should equal 'v0.8.99'
    End
  End

  Describe 'e2econfig_set_capmox'
    It 'updates the capmox sentinel in e2e config files'
      When call e2econfig_set_capmox 'v0.9.99'
      The output should include 'Updated capmox v0.8.99 to v0.9.99'
    End

    It 'writes the new sentinel to both files'
      e2econfig_set_capmox 'v0.9.99' >/dev/null
      When call e2econfig_get_capmox
      The output should equal 'v0.9.99'
    End

    It 'updates both ci and dev files'
      e2econfig_set_capmox 'v0.9.99' >/dev/null
      _dev_sentinel() { yq '.providers[] | select(.type == "InfrastructureProvider") | .versions[0].name' "${E2E_CONFIG_DIR}/proxmox-dev.yaml"; }
      When call _dev_sentinel
      The output should equal 'v0.9.99'
    End
  End

  Describe 'clusterctl_get_version'
    It 'returns the capmox nextVersion'
      When call clusterctl_get_version
      The output should equal 'v0.8.1'
    End
  End

  Describe 'clusterctl_set_version'
    It 'updates nextVersion'
      When call clusterctl_set_version 'v0.8.2'
      The output should include 'Updated nextVersion v0.8.1 to v0.8.2'
    End

    It 'writes the new value'
      clusterctl_set_version 'v0.8.2' >/dev/null
      When call clusterctl_get_version
      The output should equal 'v0.8.2'
    End
  End

  Describe 'sonar_get_version'
    It 'returns the sonar.projectVersion'
      When call sonar_get_version
      The output should equal '0.8.1'
    End
  End

  Describe 'sonar_set_version'
    It 'updates sonar.projectVersion'
      When call sonar_set_version '0.8.2'
      The output should include 'Updated sonar.projectVersion 0.8.1 to 0.8.2'
    End

    It 'writes the new value'
      sonar_set_version '0.8.2' >/dev/null
      When call sonar_get_version
      The output should equal '0.8.2'
    End
  End

  Describe 'e2econfig_set_capi'
    It 'updates the cluster-api version in e2e config files'
      When call e2econfig_set_capi 'v1.11.0'
      The output should include 'Updated cluster-api v1.10.4 to v1.11.0'
    End

    It 'writes the new version to the files'
      e2econfig_set_capi 'v1.11.0' >/dev/null
      When call e2econfig_get_capi
      The output should equal 'v1.11.0'
    End

    It 'updates both name and download URL'
      e2econfig_set_capi 'v1.11.0' >/dev/null
      When call cat "${E2E_CONFIG_DIR}/proxmox-ci.yaml"
      The output should include 'name: v1.11.0'
      The output should include 'download/v1.11.0/'
      The output should not include 'v1.10.4'
    End
  End

  Describe 'docs_get_k8s'
    It 'returns the first --kubernetes-version from docs'
      When call docs_get_k8s
      The output should equal 'v1.32.3'
    End
  End

  Describe 'docs_set_k8s'
    It 'updates --kubernetes-version references in docs'
      When call docs_set_k8s 'v1.33.0'
      The output should include 'Updated --kubernetes-version references to v1.33.0'
    End

    It 'writes the new version to docs files'
      docs_set_k8s 'v1.33.0' >/dev/null
      When call docs_get_k8s
      The output should equal 'v1.33.0'
    End
  End
End
