/*
Copyright 2024-2025 IONOS Cloud.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package goproxmox

import "github.com/pkg/errors"

var (
	// ErrCloudInitFailed is returned when cloud-init failed execution.
	ErrCloudInitFailed = errors.New("cloud-init failed execution")

	// ErrCloudInitRecoverable is returned when cloud-init finished but logged recoverable
	// errors (exit code 2 since cloud-init 23.4). The guest is configured and usable; the
	// error wraps the status output so the warning can be surfaced to the user.
	ErrCloudInitRecoverable = errors.New("cloud-init finished with recoverable errors")

	// ErrTemplateNotFound is returned when a VM template is not found.
	ErrTemplateNotFound = errors.New("VM template not found")
)
