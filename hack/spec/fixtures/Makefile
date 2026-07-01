ENVTEST_K8S_VERSION ?= $(shell hack/envtest-ver.sh)

.PHONY: print-envtest-ver
print-envtest-ver:
	@echo $(ENVTEST_K8S_VERSION)

.PHONY: test
test:
	go test ./...
