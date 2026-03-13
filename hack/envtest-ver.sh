#!/usr/bin/env bash
# envtest-ver.sh prints the ENVTEST_K8S_VERSION derived from go.mod.
# Called by the Makefile to compute ENVTEST_K8S_VERSION.

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"
gomod_make_envtest
