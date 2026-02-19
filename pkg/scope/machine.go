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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	capierrors "sigs.k8s.io/cluster-api/errors"
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
	return &MachineScope{
		Logger:      params.Logger,
		client:      params.Client,
		patchHelper: helper,

		Cluster:        params.Cluster,
		Machine:        params.Machine,
		InfraCluster:   params.InfraCluster,
		ProxmoxMachine: params.ProxmoxMachine,
		IPAMHelper:     params.IPAMHelper,
	}, nil
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
		node = m.ProxmoxMachine.GetNode()
	}

	return node
}

// GetProviderID returns the ProxmoxMachine providerID from the spec.
func (m *MachineScope) GetProviderID() string {
	if m.ProxmoxMachine.Spec.ProviderID != nil {
		return *m.ProxmoxMachine.Spec.ProviderID
	}
	return ""
}

// GetVirtualMachineID returns the ProxmoxMachine vmid from the spec.
func (m *MachineScope) GetVirtualMachineID() int64 {
	return m.ProxmoxMachine.GetVirtualMachineID()
}

// SetProviderID sets the ProxmoxMachine providerID in spec.
func (m *MachineScope) SetProviderID(biosUUID string) {
	providerID := fmt.Sprintf("proxmox://%s", biosUUID)
	m.ProxmoxMachine.Spec.ProviderID = ptr.To(providerID)
}

// SetVirtualMachineID sets the ProxmoxMachine instanceID in spec.
func (m *MachineScope) SetVirtualMachineID(vmID int64) {
	m.ProxmoxMachine.Spec.VirtualMachineID = ptr.To(vmID)
}

// SetReady sets the ProxmoxMachine Ready Status.
func (m *MachineScope) SetReady() {
	m.ProxmoxMachine.Status.Initialization.Provisioned = ptr.To(true)
	m.ensureDeprecatedV1Beta1MachineStatus().Ready = ptr.To(true)
}

// SetNotReady sets the ProxmoxMachine Ready Status to false.
func (m *MachineScope) SetNotReady() {
	m.ProxmoxMachine.Status.Initialization.Provisioned = ptr.To(false)
	m.ensureDeprecatedV1Beta1MachineStatus().Ready = ptr.To(false)
}

// SetFailureMessage sets the ProxmoxMachine status failure message.
func (m *MachineScope) SetFailureMessage(v error) {
	m.ensureDeprecatedV1Beta1MachineStatus().FailureMessage = ptr.To(v.Error())
}

// SetFailureReason sets the ProxmoxMachine status failure reason.
func (m *MachineScope) SetFailureReason(v capierrors.MachineStatusError) {
	m.ensureDeprecatedV1Beta1MachineStatus().FailureReason = &v
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
	if dep := m.ProxmoxMachine.Status.Deprecated; dep != nil && dep.V1Beta1 != nil {
		return dep.V1Beta1.FailureReason != nil || dep.V1Beta1.FailureMessage != nil
	}
	return false
}

// ensureDeprecatedV1Beta1MachineStatus returns the V1Beta1 deprecated status,
// initializing the nested structs if necessary.
func (m *MachineScope) ensureDeprecatedV1Beta1MachineStatus() *infrav1.ProxmoxMachineV1Beta1DeprecatedStatus {
	if m.ProxmoxMachine.Status.Deprecated == nil {
		m.ProxmoxMachine.Status.Deprecated = &infrav1.ProxmoxMachineDeprecatedStatus{}
	}
	if m.ProxmoxMachine.Status.Deprecated.V1Beta1 == nil {
		m.ProxmoxMachine.Status.Deprecated.V1Beta1 = &infrav1.ProxmoxMachineV1Beta1DeprecatedStatus{}
	}
	return m.ProxmoxMachine.Status.Deprecated.V1Beta1
}

// SetVirtualMachine sets the Proxmox VirtualMachine object to the machinescope.
func (m *MachineScope) SetVirtualMachine(vm *proxmox.VirtualMachine) {
	m.VirtualMachine = vm
}

// PatchObject persists the machine spec and status.
func (m *MachineScope) PatchObject() error {
	// always update the readyCondition.
	_ = conditions.SetSummaryCondition(m.ProxmoxMachine, m.ProxmoxMachine, string(clusterv1.ReadyCondition),
		conditions.ForConditionTypes{string(infrav1.VMProvisionedCondition)},
	)

	// Patch the ProxmoxMachine resource.
	return m.patchHelper.Patch(
		context.TODO(),
		m.ProxmoxMachine,
		patch.WithOwnedConditions{Conditions: []string{
			string(clusterv1.ReadyCondition),
			string(infrav1.VMProvisionedCondition),
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
