/*
Copyright 2023-2025 IONOS Cloud.

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

// Package proxmox defines Proxmox Client interface.
package proxmox

import (
	"context"

	"github.com/luthermonson/go-proxmox"
)

// Client Global Proxmox client interface.
type Client interface {
	CloneVM(ctx context.Context, templateID int, clone VMCloneRequest) (VMCloneResponse, error)

	ConfigureVM(ctx context.Context, vm *proxmox.VirtualMachine, options ...VirtualMachineOption) (*proxmox.Task, error)

	FindVMResource(ctx context.Context, vmID uint64) (*proxmox.ClusterResource, error)
	FindVMTemplateByTags(ctx context.Context, templateTags []string) (string, int32, error)

	CheckID(ctx context.Context, vmID int64) (bool, error)

	GetVM(ctx context.Context, nodeName string, vmID int64) (*proxmox.VirtualMachine, error)

	DeleteVM(ctx context.Context, nodeName string, vmID int64) (*proxmox.Task, error)

	GetTask(ctx context.Context, upID string) (*proxmox.Task, error)

	GetReservableMemoryBytes(ctx context.Context, nodeName string, nodeMemoryAdjustment int64) (uint64, error)

	ResizeDisk(ctx context.Context, vm *proxmox.VirtualMachine, disk, size string) (*proxmox.Task, error)

	ResumeVM(ctx context.Context, vm *proxmox.VirtualMachine) (*proxmox.Task, error)

	StartVM(ctx context.Context, vm *proxmox.VirtualMachine) (*proxmox.Task, error)

	TagVM(ctx context.Context, vm *proxmox.VirtualMachine, tag string) (*proxmox.Task, error)

	UnmountCloudInitISO(ctx context.Context, vm *proxmox.VirtualMachine, device string) error

	CloudInitStatus(ctx context.Context, vm *proxmox.VirtualMachine) (bool, error)

	QemuAgentStatus(ctx context.Context, vm *proxmox.VirtualMachine) error
}
