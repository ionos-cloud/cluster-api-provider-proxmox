/*
Copyright 2023-2026 IONOS Cloud.

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
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
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
	node := (&proxmox.Node{}).New(c.Client, clone.Node)
	if err := node.Status(ctx); err != nil {
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
	node := (&proxmox.Node{}).New(c.Client, nodeName)
	if err := node.Status(ctx); err != nil {
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

// FindVMTemplateByTags tries to find a VMID by its tags across the whole cluster.
func (c *APIClient) FindVMTemplateByTags(ctx context.Context, templateTags []string, matchPolicy string) (string, int32, error) {
	logger := log.FromContext(ctx)

	cluster, err := c.Cluster(ctx)
	if err != nil {
		return "", -1, fmt.Errorf("cannot get cluster status: %w", err)
	}
	vmResources, err := cluster.Resources(ctx, "vm")
	if err != nil {
		return "", -1, fmt.Errorf("could not list vm resources: %w", err)
	}

	for i, tag := range templateTags {
		// Proxmox VM tags are always lowercase
		templateTags[i] = strings.ToLower(tag)
	}
	// compact templateTags because of collisions after lowercasing
	slices.Sort(templateTags)
	templateTags = slices.Compact(templateTags)

	var vmTemplate *proxmox.ClusterResource
	matches, bestDistance := 0, int(^uint(0)>>1)
NEXT_VM:
	for _, vm := range vmResources {
		if vm.Template == 0 || len(vm.Tags) == 0 {
			continue NEXT_VM
		}

		vmTagMap := make(map[string]string)
		for _, tag := range strings.Split(vm.Tags, ";") {
			vmTagMap[strings.ToLower(strings.TrimSpace(tag))] = ""
		}

		logger.V(4).Info("VM Template Tags", "Name", vm.Name, "Tags", maps.Values(vmTagMap))

		for _, tag := range templateTags {
			if _, exists := vmTagMap[tag]; !exists {
				continue NEXT_VM
			}
		}

		// distance is always >= 0 because all other cases already jump to NEXT_VM.
		distance := len(vmTagMap) - len(templateTags)
		switch infrav1.TemplateMatchPolicy(matchPolicy) {
		case infrav1.TemplateMatchPolicyExact:
			if distance != 0 {
				continue NEXT_VM
			}
		case infrav1.TemplateMatchPolicyBest:
			if distance > bestDistance {
				continue NEXT_VM
			}
			bestDistance = distance
		}

		matches++
		vmTemplate = vm
	}

	if matches != 1 {
		return "", -1, fmt.Errorf("%w: found %d VM templates with tags %q", ErrTemplateNotFound, matches, strings.Join(templateTags, ";"))
	}

	return vmTemplate.Node, int32(vmTemplate.VMID), nil
}

// DeleteVM deletes a VM based on the nodeName and vmID. When purge is true the
// VM is deleted with purge=1, which instructs Proxmox to drop the VM from any
// HA, replication and backup-job configuration it is referenced by. Callers that
// register VMs as Proxmox HA resources must pass purge=true, since PVE otherwise
// rejects the deletion of an HA-managed VM with "unable to remove VM <id> - used
// in HA resources and purge parameter not set" (issue #216).
func (c *APIClient) DeleteVM(ctx context.Context, nodeName string, vmID int64, purge bool) (*proxmox.Task, error) {
	// A vmID can not be lower than 100.
	// If the provided vmID is lower (like -1 in issue #31), just error out without calling the API.
	if vmID < 100 {
		return nil, fmt.Errorf("vm with id %d does not exist", vmID)
	}

	node := (&proxmox.Node{}).New(c.Client, nodeName)
	if err := node.Status(ctx); err != nil {
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

	// When the caller opted into purge, the VM is known to be HA-managed, so we
	// delete it with purge=1 directly: a plain delete first would be rejected by
	// PVE ("used in HA resources and purge parameter not set") on every HA VM and
	// needlessly spam the node's task log.
	//
	// See https://github.com/ionos-cloud/cluster-api-provider-proxmox/issues/216
	if purge {
		task, err := c.deleteVMWithPurge(ctx, vm)
		if err != nil {
			return nil, fmt.Errorf("cannot delete vm with id %d using purge: %w", vmID, err)
		}
		return task, nil
	}

	task, err := vm.Delete(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot delete vm with id %d: %w", vmID, err)
	}

	return task, nil
}

// deleteVMWithPurge deletes a VM passing purge=1, which instructs Proxmox to
// also remove the VM from any HA, replication and backup-job configuration it
// is referenced by. go-proxmox's VirtualMachine.Delete does not expose the
// purge parameter, so the request is issued directly against the API. Proxmox
// DELETE endpoints take options as query parameters, so purge=1 is appended to
// the path and the request goes through the version-stable Client.Delete.
func (c *APIClient) deleteVMWithPurge(ctx context.Context, vm *proxmox.VirtualMachine) (*proxmox.Task, error) {
	var upid proxmox.UPID
	path := fmt.Sprintf("/nodes/%s/qemu/%d?purge=1", vm.Node, vm.VMID)
	if err := c.Delete(ctx, path, &upid); err != nil {
		return nil, err
	}

	return proxmox.NewTask(upid, c.Client), nil
}

// EnsureHAResource registers the VM as a Proxmox HA resource with the requested
// state. It is idempotent: a missing resource is created, an existing one whose
// state differs is updated, and a resource already in the desired state is left
// untouched. state defaults to "started" when empty.
//
// Proxmox HA is managed through /cluster/ha/resources on every supported PVE
// version (the migration from HA groups to HA rules in PVE 9 did not change
// this endpoint), so no version-specific handling is required here.
func (c *APIClient) EnsureHAResource(ctx context.Context, vmID int64, state string) error {
	if state == "" {
		state = "started"
	}
	sid := fmt.Sprintf("vm:%d", vmID)

	var resources []struct {
		SID   string `json:"sid"`
		State string `json:"state"`
	}
	if err := c.Get(ctx, "/cluster/ha/resources", &resources); err != nil {
		return fmt.Errorf("cannot list HA resources: %w", err)
	}

	for _, r := range resources {
		if r.SID != sid {
			continue
		}
		if r.State == state {
			return nil
		}
		if err := c.Put(ctx, "/cluster/ha/resources/"+sid, map[string]string{"state": state}, nil); err != nil {
			return fmt.Errorf("cannot update HA resource %s: %w", sid, err)
		}
		return nil
	}

	if err := c.Post(ctx, "/cluster/ha/resources", map[string]string{"sid": sid, "state": state}, nil); err != nil {
		return fmt.Errorf("cannot create HA resource %s: %w", sid, err)
	}
	return nil
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
func (c *APIClient) GetReservableMemoryBytes(ctx context.Context, nodeName string, nodeMemoryAdjustment int64) (uint64, error) {
	node := (&proxmox.Node{}).New(c.Client, nodeName)

	if err := node.Status(ctx); err != nil {
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
func (c *APIClient) ResizeDisk(ctx context.Context, vm *proxmox.VirtualMachine, disk, size string) (*proxmox.Task, error) {
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
