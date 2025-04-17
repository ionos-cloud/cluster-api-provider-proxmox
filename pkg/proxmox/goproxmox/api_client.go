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

// Package goproxmox implements a client for Proxmox resource lifecycle management.
package goproxmox

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"

	capmox "github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/proxmox"
)

var _ capmox.Client = &APIClient{}

// ErrVMIDFree is returned if the VMID is free.
var ErrVMIDFree = errors.New("VMID is free")

// APIClient Proxmox API client object.
type APIClient struct {
	*proxmox.Client
	logger logr.Logger
}

// NewAPIClient initializes a Proxmox API client. If the client is misconfigured, an error is returned.
func NewAPIClient(ctx context.Context, logger logr.Logger, baseURL string, options ...proxmox.Option) (*APIClient, error) {
	proxmoxAPIURL, err := url.JoinPath(baseURL, "api2", "json")
	if err != nil {
		return nil, fmt.Errorf("invalid proxmox base URL %q: %w", baseURL, err)
	}

	options = append(options, proxmox.WithLogger(capmox.Logger{}))
	upstreamClient := proxmox.NewClient(proxmoxAPIURL, options...)
	version, err := upstreamClient.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize proxmox api client: %w", err)
	}
	logger.Info("Proxmox client initialized")
	logger.Info("Proxmox server", "version", version.Release)

	return &APIClient{
		Client: upstreamClient,
		logger: logger,
	}, nil
}

// CloneVM clones a VM based on templateID and VMCloneRequest.
func (c *APIClient) CloneVM(ctx context.Context, templateID int, clone capmox.VMCloneRequest) (capmox.VMCloneResponse, error) {
	// get the node
	node, err := c.Node(ctx, clone.Node)
	if err != nil {
		return capmox.VMCloneResponse{}, fmt.Errorf("cannot find node with name %s: %w", clone.Node, err)
	}

	// get the vm template
	vmTemplate, err := node.VirtualMachine(ctx, templateID)
	if err != nil {
		return capmox.VMCloneResponse{}, fmt.Errorf("unable to find vm template: %w", err)
	}

	vmOptions := proxmox.VirtualMachineCloneOptions{
		NewID:       clone.NewID,
		Description: clone.Description,
		Format:      clone.Format,
		Full:        clone.Full,
		Name:        clone.Name,
		Pool:        clone.Pool,
		SnapName:    clone.SnapName,
		Storage:     clone.Storage,
		Target:      clone.Target,
	}
	newID, task, err := vmTemplate.Clone(ctx, &vmOptions)
	if err != nil {
		return capmox.VMCloneResponse{}, fmt.Errorf("unable to create new vm: %w", err)
	}

	return capmox.VMCloneResponse{NewID: int64(newID), Task: task}, nil
}

// ConfigureVM updates a VMs settings.
func (c *APIClient) ConfigureVM(ctx context.Context, vm *proxmox.VirtualMachine, options ...capmox.VirtualMachineOption) (*proxmox.Task, error) {
	task, err := vm.Config(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("unable to configure vm: %w", err)
	}
	return task, nil
}

// GetVM returns a VM based on nodeName and vmID.
func (c *APIClient) GetVM(ctx context.Context, nodeName string, vmID int64) (*proxmox.VirtualMachine, error) {
	node, err := c.Node(ctx, nodeName)
	if err != nil {
		return nil, fmt.Errorf("cannot find node with name %s: %w", nodeName, err)
	}

	vm, err := node.VirtualMachine(ctx, int(vmID))
	if err != nil {
		return nil, fmt.Errorf("cannot find vm with id %d: %w", vmID, err)
	}

	return vm, nil
}

// FindVMResource tries to find a VM by its ID on the whole cluster.
func (c *APIClient) FindVMResource(ctx context.Context, vmID uint64) (*proxmox.ClusterResource, error) {
	cluster, err := c.Cluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster status: %w", err)
	}

	vmResources, err := cluster.Resources(ctx, "vm")
	if err != nil {
		return nil, fmt.Errorf("could not list vm resources: %w", err)
	}

	for _, vm := range vmResources {
		if vm.VMID == vmID {
			return vm, nil
		}
	}

	return nil, fmt.Errorf("unable to find VM with ID %d on any of the nodes", vmID)
}

func isNotInAllowedNodes(allowedNodes []string, node string) bool {
	return !slices.Contains(allowedNodes, node)
}

// FindVMTemplatesByTags finds VM templates by tags across the whole cluster and ensures only one template per node.
func (c *APIClient) FindVMTemplatesByTags(ctx context.Context, templateTags []string, allowedNodes []string, localStorage bool) (map[string]int32, error) {
	templates := make(map[string]int32)

	// if for some reason there is not tags, we fail early and return error
	if len(templateTags) == 0 {
		return nil, fmt.Errorf("%w: no template tags defined", ErrTemplateNotFound)
	}
	sortedTags := make([]string, len(templateTags))
	for i, tag := range templateTags {
		// Proxmox VM tags are always lowercase
		sortedTags[i] = strings.ToLower(tag)
	}
	slices.Sort(sortedTags)
	uniqueTags := slices.Compact(sortedTags)

	cluster, err := c.Cluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster status: %w", err)
	}

	vmResources, err := cluster.Resources(ctx, "vm")
	if err != nil {
		return nil, fmt.Errorf("could not list VM resources: %w", err)
	}

	for _, vm := range vmResources {
		if vm.Template == 0 || len(vm.Tags) == 0 {
			continue
		}

		vmTags := strings.Split(vm.Tags, ";")
		slices.Sort(vmTags)

		// if localstorage template should be on all allowed nodes
		if localStorage && isNotInAllowedNodes(allowedNodes, vm.Node) {
			continue
		}

		if slices.Equal(vmTags, uniqueTags) {
			// check if we have multiple templates per node
			if _, exists := templates[vm.Node]; exists {
				return nil, fmt.Errorf("%w: multiple VM templates found on node %q with tags %q", ErrMultipleTemplatesFound, vm.Node, strings.Join(templateTags, ";"))
			}
			templates[vm.Node] = int32(vm.VMID)
		}
	}

	if (len(templates) != len(allowedNodes)) && localStorage {
		return nil, fmt.Errorf("found %d templates on allowedNodes %q with tags %q", len(templates), strings.Join(allowedNodes, ","), strings.Join(templateTags, ";"))
	}

	if !localStorage {
		if n := len(templates); n != 1 {
			return nil, fmt.Errorf("%s: found %d VM templates with tags %q", "ErrTemplateNotFound", n, strings.Join(templateTags, ";"))
		}
	}

	return templates, nil
}

// DeleteVM deletes a VM based on the nodeName and vmID.
func (c *APIClient) DeleteVM(ctx context.Context, nodeName string, vmID int64) (*proxmox.Task, error) {
	// A vmID can not be lower than 100.
	// If the provided vmID is lower (like -1 in issue #31), just error out without calling the API.
	if vmID < 100 {
		return nil, fmt.Errorf("vm with id %d does not exist", vmID)
	}

	node, err := c.Node(ctx, nodeName)
	if err != nil {
		return nil, fmt.Errorf("cannot find node with name %s: %w", nodeName, err)
	}

	cluster, err := c.Cluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster")
	}

	if vmidFree, err := cluster.CheckID(ctx, int(vmID)); vmidFree {
		return nil, ErrVMIDFree
	} else if err != nil {
		return nil, err
	}

	vm, err := node.VirtualMachine(ctx, int(vmID))
	if err != nil {
		return nil, fmt.Errorf("cannot find vm with id %d: %w", vmID, err)
	}

	if vm.IsRunning() {
		if _, err = vm.Stop(ctx); err != nil {
			return nil, fmt.Errorf("cannot stop vm id %d: %w", vmID, err)
		}
	}

	task, err := vm.Delete(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot delete vm with id %d: %w", vmID, err)
	}

	return task, nil
}

// GetAllNodeNames get all nodes in cluster against which we are authenticated.
func (c *APIClient) GetAllNodeNames(ctx context.Context) ([]string, error) {
	cluster, err := c.Client.Cluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster status: %w", err)
	}
	err = cluster.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster status: %w", err)
	}
	var nodes []string
	for _, node := range cluster.Nodes {
		nodes = appendIfMissing(nodes, node.Name)
	}

	return nodes, nil
}

// appendIfMissing make sure we add only uniq items to the slice.
func appendIfMissing(slice []string, item string) []string {
	if slices.Contains(slice, item) {
		return slice
	}
	return append(slice, item)
}

// CheckID checks if the vmid is available on the cluster.
// Returns true if the vmid is available, false if it is taken.
func (c *APIClient) CheckID(ctx context.Context, vmid int64) (bool, error) {
	cluster, err := c.Cluster(ctx)
	if err != nil {
		return false, fmt.Errorf("cannot get cluster")
	}
	return cluster.CheckID(ctx, int(vmid))
}

// GetTask returns a task associated with upID.
func (c *APIClient) GetTask(ctx context.Context, upID string) (*proxmox.Task, error) {
	task := proxmox.NewTask(proxmox.UPID(upID), c.Client)

	err := task.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get task with UPID %s: %w", upID, err)
	}

	return task, nil
}

// GetReservableMemoryBytes returns the memory that can be reserved by a new VM, in bytes.
func (c *APIClient) GetReservableMemoryBytes(ctx context.Context, nodeName string, nodeMemoryAdjustment uint64) (uint64, error) {
	node, err := c.Client.Node(ctx, nodeName)
	if err != nil {
		return 0, fmt.Errorf("cannot find node with name %s: %w", nodeName, err)
	}

	reservableMemory := uint64(float64(node.Memory.Total) / 100 * float64(nodeMemoryAdjustment))

	if nodeMemoryAdjustment == 0 {
		return node.Memory.Total, nil
	}

	vms, err := node.VirtualMachines(ctx)
	if err != nil {
		return 0, fmt.Errorf("cannot list vms for node %s: %w", nodeName, err)
	}

	for _, vm := range vms {
		// Ignore VM Templates, as they can't be started.
		if vm.Template {
			continue
		}
		if reservableMemory < vm.MaxMem {
			reservableMemory = 0
		} else {
			reservableMemory -= vm.MaxMem
		}
	}

	containers, err := node.Containers(ctx)
	if err != nil {
		return 0, fmt.Errorf("cannot list containers for node %s: %w", nodeName, err)
	}

	for _, ct := range containers {
		if reservableMemory < ct.MaxMem {
			reservableMemory = 0
		} else {
			reservableMemory -= ct.MaxMem
		}
	}

	return reservableMemory, nil
}

// ResizeDisk resizes a VM disk to the specified size.
func (c *APIClient) ResizeDisk(ctx context.Context, vm *proxmox.VirtualMachine, disk, size string) error {
	return vm.ResizeDisk(ctx, disk, size)
}

// ResumeVM resumes the VM.
func (c *APIClient) ResumeVM(ctx context.Context, vm *proxmox.VirtualMachine) (*proxmox.Task, error) {
	return vm.Resume(ctx)
}

// StartVM starts the VM.
func (c *APIClient) StartVM(ctx context.Context, vm *proxmox.VirtualMachine) (*proxmox.Task, error) {
	return vm.Start(ctx)
}

// TagVM tags the VM.
func (c *APIClient) TagVM(ctx context.Context, vm *proxmox.VirtualMachine, tag string) (*proxmox.Task, error) {
	return vm.AddTag(ctx, tag)
}

// UnmountCloudInitISO unmounts the cloud-init iso from VM.
func (c *APIClient) UnmountCloudInitISO(ctx context.Context, vm *proxmox.VirtualMachine, device string) error {
	err := vm.UnmountCloudInitISO(ctx, device)
	if err != nil {
		return fmt.Errorf("unable to unmount cloud-init iso: %w", err)
	}

	if vm.HasTag(proxmox.MakeTag(proxmox.TagCloudInit)) {
		_, err = vm.RemoveTag(ctx, proxmox.MakeTag(proxmox.TagCloudInit))
	}
	return err
}

// CloudInitStatus returns the cloud-init status of the VM.
func (c *APIClient) CloudInitStatus(ctx context.Context, vm *proxmox.VirtualMachine) (running bool, err error) {
	if err := c.QemuAgentStatus(ctx, vm); err != nil {
		return false, errors.Wrap(err, "error waiting for agent")
	}

	pid, err := vm.AgentExec(ctx, []string{"cloud-init", "status"}, "")
	if err != nil {
		return false, errors.Wrap(err, "unable to get cloud-init status")
	}

	status, err := vm.WaitForAgentExecExit(ctx, pid, 2)
	if err != nil {
		return false, errors.Wrap(err, "unable to wait for agent exec")
	}

	if status.Exited == 1 && status.ExitCode == 0 && strings.Contains(status.OutData, "running") {
		return true, nil
	}
	if status.Exited == 1 && status.ExitCode != 0 {
		return false, ErrCloudInitFailed
	}

	return false, nil
}

// QemuAgentStatus returns the qemu-agent status of the VM.
func (c *APIClient) QemuAgentStatus(ctx context.Context, vm *proxmox.VirtualMachine) error {
	if err := vm.WaitForAgent(ctx, 5); err != nil {
		return errors.Wrap(err, "error waiting for agent")
	}

	return nil
}
