package goproxmox

import "github.com/pkg/errors"

var (
	// ErrCloudInitFailed is returned when cloud-init failed execution.
	ErrCloudInitFailed = errors.New("cloud-init failed execution")
)
