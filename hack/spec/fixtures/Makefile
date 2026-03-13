ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')

.PHONY: print-envtest-ver
print-envtest-ver:
	@echo $(ENVTEST_K8S_VERSION)

.PHONY: test
test:
	go test ./...
