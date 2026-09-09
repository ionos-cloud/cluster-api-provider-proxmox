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

package scope

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/kubernetes/ipam"
)

// MachineScopeParams defines the input parameters used to create a new MachineScope.
type MachineScopeParams struct {
	Client         client.Client
	Logger         *logr.Logger
	Cluster        *clusterv1.Cluster
	Machine        *clusterv1.Machine
	InfraCluster   *ClusterScope
	ProxmoxMachine *infrav1.ProxmoxMachine
	IPAMHelper     *ipam.Helper
}

// MachineScope defines a scope defined around a machine and its cluster.
type MachineScope struct {
	*logr.Logger
	client      client.Client
	patchHelper *patch.Helper

	Cluster        *clusterv1.Cluster
	Machine        *clusterv1.Machine
	InfraCluster   *ClusterScope
	ProxmoxMachine *infrav1.ProxmoxMachine
	IPAMHelper     *ipam.Helper
	VirtualMachine *proxmox.VirtualMachine

	// allowedNodes is the placement node list resolved once by
	// resolvePlacement. Immutable after scope construction.
	allowedNodes []string

	// zone is the zone resolved once by resolvePlacement. Immutable after
	// scope construction. nil means the default zone.
	zone *string

	// failureDomainErr records the FailureDomainNotFoundError encountered
	// by resolvePlacement, if any. Kept on the scope instead of failing
	// construction so reconciliation paths that must proceed without the
	// zone (e.g. deletion) still get a usable scope.
	failureDomainErr error
}

// NewMachineScope creates a new MachineScope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewMachineScope(params MachineScopeParams) (*MachineScope, error) {
	if params.Client == nil {
		return nil, errors.New("Client is required when creating a MachineScope")
	}
	if params.Machine == nil {
		return nil, errors.New("Machine is required when creating a MachineScope")
	}
	if params.Cluster == nil {
		return nil, errors.New("Cluster is required when creating a MachineScope")
	}
	if params.ProxmoxMachine == nil {
		return nil, errors.New("ProxmoxMachine is required when creating a MachineScope")
	}
	if params.InfraCluster == nil {
		return nil, errors.New("ProxmoxCluster is required when creating a MachineScope")
	}
	if params.IPAMHelper == nil {
		return nil, errors.New("IPAMHelper is required when creating a MachineScope")
	}
	if params.Logger == nil {
		logger := log.FromContext(context.Background())
		params.Logger = &logger
	}

	helper, err := patch.NewHelper(params.ProxmoxMachine, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init patch helper")
	}
	m := &MachineScope{
		Logger:      params.Logger,
		client:      params.Client,
		patchHelper: helper,

		Cluster:        params.Cluster,
		Machine:        params.Machine,
		InfraCluster:   params.InfraCluster,
		ProxmoxMachine: params.ProxmoxMachine,
		IPAMHelper:     params.IPAMHelper,
	}
	m.failureDomainErr = m.resolvePlacement()
	return m, nil
}

// FailureDomainNotFoundError is returned when the CAPI Machine references a
// failure domain that is not configured in the ProxmoxCluster zones.
type FailureDomainNotFoundError struct {
	FailureDomain string
}

func (err FailureDomainNotFoundError) Error() string {
	return fmt.Sprintf("failure domain %q not configured in ProxmoxCluster zones", err.FailureDomain)
}

// resolvePlacement resolves the machine's placement inputs once, at scope
// construction. The precedence chains are:
//
//   - allowed nodes: failure domain zone nodes > ProxmoxMachine spec > ProxmoxCluster spec
//   - zone: failure domain > Spec.Network.Zone > nil (the default zone)
//
// When the referenced failure domain is not configured in the ProxmoxCluster
// zones it returns a FailureDomainNotFoundError; the remaining fields are
// still resolved from the specs so the scope stays usable for paths that
// must proceed anyway, such as deletion.
func (m *MachineScope) resolvePlacement() error {
	m.allowedNodes = m.ProxmoxMachine.Spec.AllowedNodes
	if len(m.allowedNodes) == 0 {
		m.allowedNodes = m.InfraCluster.ProxmoxCluster.Spec.AllowedNodes
	}
	if m.ProxmoxMachine.Spec.Network != nil {
		m.zone = m.ProxmoxMachine.Spec.Network.Zone
	}

	faildom := m.Machine.Spec.FailureDomain
	if faildom == "" {
		return nil
	}

	m.zone = new(faildom)
	if nodes := m.InfraCluster.ProxmoxCluster.GetZoneNodes(faildom); len(nodes) > 0 {
		m.allowedNodes = nodes
		return nil
	}

	// The zone may exist without an explicit node list; only a missing zone
	// is an error.
	for _, zc := range m.InfraCluster.ProxmoxCluster.Spec.ZoneConfigs {
		if ptr.Deref(zc.Zone, "") == faildom {
			return nil
		}
	}

	return FailureDomainNotFoundError{FailureDomain: faildom}
}

// AllowedNodes returns the Proxmox nodes the machine may be placed on,
// resolved once at scope construction. An empty result means any node.
func (m *MachineScope) AllowedNodes() []string {
	return m.allowedNodes
}

// Zone returns the zone the machine belongs to, resolved once at scope
// construction. nil means the default zone.
func (m *MachineScope) Zone() *string {
	return m.zone
}

// FailureDomainError returns the FailureDomainNotFoundError encountered
// while resolving placement, or nil. Callers that are about to provision
// must treat a non-nil result as "failure domain not ready".
func (m *MachineScope) FailureDomainError() error {
	return m.failureDomainErr
}

// Name returns the ProxmoxMachine name.
func (m *MachineScope) Name() string {
	return m.ProxmoxMachine.Name
}

// Namespace returns the namespace name.
func (m *MachineScope) Namespace() string {
	return m.ProxmoxMachine.Namespace
}

// IsControlPlane returns true if the machine is a control plane.
func (m *MachineScope) IsControlPlane() bool {
	return util.IsControlPlaneMachine(m.Machine)
}

// Role returns the machine role from the labels.
func (m *MachineScope) Role() string {
	if util.IsControlPlaneMachine(m.Machine) {
		return "control-plane"
	}
	return "node"
}

// LocateProxmoxNode will attempt to get information about the currently deployed Proxmox node.
func (m *MachineScope) LocateProxmoxNode() string {
	if status := m.ProxmoxMachine.Status.ProxmoxNode; status != nil {
		return *status
	}

	node := m.InfraCluster.ProxmoxCluster.GetNode(m.Name(), util.IsControlPlaneMachine(m.Machine))
	if node == "" {
		node = m.ProxmoxMachine.GetSourceNode()
	}

	return node
}

// GetProviderID returns the ProxmoxMachine providerID from the spec.
func (m *MachineScope) GetProviderID() string {
	return m.ProxmoxMachine.Spec.ProviderID
}

// GetVirtualMachineID returns the ProxmoxMachine vmid from the spec.
func (m *MachineScope) GetVirtualMachineID() int64 {
	return m.ProxmoxMachine.GetVirtualMachineID()
}

// SetProviderID sets the ProxmoxMachine providerID in spec.
func (m *MachineScope) SetProviderID(biosUUID string) {
	providerID := fmt.Sprintf("proxmox://%s", biosUUID)
	m.ProxmoxMachine.Spec.ProviderID = providerID
}

// SetVirtualMachineID sets the ProxmoxMachine instanceID in spec.
func (m *MachineScope) SetVirtualMachineID(vmID int64) {
	m.ProxmoxMachine.Spec.VirtualMachineID = new(vmID)
}

// SetReady sets the ProxmoxMachine Ready Status.
func (m *MachineScope) SetReady() {
	m.ProxmoxMachine.Status.Initialization.Provisioned = new(true)
}

// SetNotReady sets the ProxmoxMachine Ready Status to false.
func (m *MachineScope) SetNotReady() {
	m.ProxmoxMachine.Status.Initialization.Provisioned = new(false)
}

// SetAnnotation sets a key value annotation on the ProxmoxMachine.
func (m *MachineScope) SetAnnotation(key, value string) {
	if m.ProxmoxMachine.Annotations == nil {
		m.ProxmoxMachine.Annotations = map[string]string{}
	}
	m.ProxmoxMachine.Annotations[key] = value
}

// HasFailed returns the failure state of the machine scope.
func (m *MachineScope) HasFailed() bool {
	cond := conditions.Get(m.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition)
	return cond != nil && cond.Status == metav1.ConditionFalse &&
		cond.Reason == infrav1.ProxmoxMachineVirtualMachineProvisionedVMProvisionFailedReason
}

// SetVirtualMachine sets the Proxmox VirtualMachine object to the machinescope.
func (m *MachineScope) SetVirtualMachine(vm *proxmox.VirtualMachine) {
	m.VirtualMachine = vm
}

// PatchObject persists the machine spec and status.
func (m *MachineScope) PatchObject() error {
	// always update the readyCondition.
	_ = conditions.SetSummaryCondition(m.ProxmoxMachine, m.ProxmoxMachine, "Ready",
		conditions.ForConditionTypes{infrav1.ProxmoxMachineVirtualMachineProvisionedCondition},
	)

	// Patch the ProxmoxMachine resource.
	return m.patchHelper.Patch(
		context.TODO(),
		m.ProxmoxMachine,
		patch.WithOwnedConditions{Conditions: []string{
			"Ready",
			infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		}})
}

// SetAddresses sets the addresses in the status.
func (m *MachineScope) SetAddresses(addr []clusterv1.MachineAddress) {
	m.ProxmoxMachine.Status.Addresses = addr
}

// Close the MachineScope by updating the machine spec, machine status.
func (m *MachineScope) Close() error {
	return m.PatchObject()
}

// GetBootstrapSecret obtains the bootstrap data secret.
func (m *MachineScope) GetBootstrapSecret(ctx context.Context, secret *corev1.Secret) error {
	secretKey := types.NamespacedName{
		Namespace: m.ProxmoxMachine.GetNamespace(),
		Name:      *m.Machine.Spec.Bootstrap.DataSecretName,
	}

	return m.client.Get(ctx, secretKey, secret)
}

// SkipQemuGuestCheck check whether qemu-agent status check is enabled.
func (m *MachineScope) SkipQemuGuestCheck() bool {
	if m.ProxmoxMachine.Spec.Checks != nil {
		return ptr.Deref(m.ProxmoxMachine.Spec.Checks.SkipQemuGuestAgent, false)
	}

	return false
}

// SkipCloudInitCheck check whether cloud-init status check is enabled.
func (m *MachineScope) SkipCloudInitCheck() bool {
	if m.SkipQemuGuestCheck() {
		return true
	}

	if m.ProxmoxMachine.Spec.Checks != nil {
		return ptr.Deref(m.ProxmoxMachine.Spec.Checks.SkipCloudInitStatus, false)
	}

	return false
}
