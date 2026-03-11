module github.com/ionos-cloud/cluster-api-provider-proxmox

go 1.25.0

replace (
	github.com/golangci/golangci-lint/v2 => github.com/golangci/golangci-lint/v2 v2.9.0
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.10.4
)

require (
	k8s.io/api v0.32.3
	k8s.io/apimachinery v0.32.3
	k8s.io/client-go v0.32.3
	sigs.k8s.io/cluster-api v1.10.4
	sigs.k8s.io/cluster-api/test v1.10.4
)

require (
	github.com/golangci/golangci-lint/v2 v2.9.0 // indirect
)
