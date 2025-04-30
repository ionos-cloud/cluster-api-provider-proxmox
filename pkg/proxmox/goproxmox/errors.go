package goproxmox

import "github.com/pkg/errors"

var (
	// ErrCloudInitFailed is returned when cloud-init failed execution.
	ErrCloudInitFailed = errors.New("cloud-init failed execution")

	// ErrTemplateNotFound is returned when a VM template is not found.
	ErrTemplateNotFound = errors.New("VM template not found")

	// ErrMultipleTemplatesFound is returned when multiple VM templates are found.
	ErrMultipleTemplatesFound = errors.New("Multiple templates found")

	// ErrNoNode is returned when no target or alowedNodes is specified.
	ErrNoNode = errors.New("No target or allowedNodes specified")
)
