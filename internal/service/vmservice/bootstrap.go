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

package vmservice

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/types"
)

func reconcileBootstrapData(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if conditions.GetReason(machineScope.ProxmoxMachine, infrav1.ProxmoxMachineVirtualMachineProvisionedCondition) != infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapDataReconciliationReason {
		// Machine is in the wrong state to reconcile, we only reconcile VMs Waiting for Bootstrap Data reconciliation
		return false, nil
	}

	// TODO: remove.
	// make sure MacAddress is set.
	if !vmHasMacAddresses(machineScope) {
		return true, nil
	}

	machineScope.Logger.V(4).Info("reconciling BootstrapData.")

	// Get the bootstrap data.
	bootstrapData, format, err := getBootstrapData(ctx, machineScope)
	if err != nil {
		conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
			Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.ProxmoxMachineVirtualMachineProvisionedCloningFailedReason,
			Message: fmt.Sprintf("%s", err),
		})
		return false, err
	}

	biosUUID := extractUUID(machineScope.VirtualMachine.VirtualMachineConfig.SMBios1)

	nicData, err := getNetworkConfigData(ctx, machineScope)
	if err != nil {
		conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
			Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForBootstrapDataReconciliationReason,
			Message: fmt.Sprintf("%s", err),
		})
		return false, err
	}

	kubernetesVersion := ptr.Deref(machineScope.Machine.Spec.Version, "")

	machineScope.Logger.V(4).Info("reconciling BootstrapData.", "format", format)

	machineScope.Logger.V(4).Info("nicData", "json", func() string { ret, _ := json.Marshal(nicData); return string(ret) }())
	// Inject userdata based on the format
	if ptr.Deref(format, "") == ignition.FormatIgnition {
		err = injectIgnition(ctx, machineScope, bootstrapData, biosUUID, nicData, kubernetesVersion)
	} else if ptr.Deref(format, "") == cloudinit.FormatCloudConfig {
		err = injectCloudInit(ctx, machineScope, bootstrapData, biosUUID, nicData, kubernetesVersion)
	}
	if err != nil {
		// Todo: test this (colliding default gateways for example)
		conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
			Type:    infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.ProxmoxMachineVirtualMachineProvisionedVMProvisionFailedReason,
			Message: fmt.Sprintf("%s", err),
		})
		machineScope.Logger.V(0).Error(err, "nicData", "json", func() string { ret, _ := json.Marshal(nicData); return string(ret) }())
		return false, errors.Wrap(err, "failed to inject bootstrap data")
	}

	// Todo: This status field is now superfluous
	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)
	conditions.Set(machineScope.ProxmoxMachine, metav1.Condition{
		Type:   infrav1.ProxmoxMachineVirtualMachineProvisionedCondition,
		Status: metav1.ConditionFalse,
		Reason: infrav1.ProxmoxMachineVirtualMachineProvisionedWaitingForVMPowerUpReason,
	})

	return false, nil
}

func injectCloudInit(ctx context.Context, machineScope *scope.MachineScope, bootstrapData []byte, biosUUID string, nicData []types.NetworkConfigData, kubernetesVersion string) error {
	// create network renderer
	network := cloudinit.NewNetworkConfig(nicData)

	// create metadata renderer
	metadata := cloudinit.NewMetadata(biosUUID, machineScope.Name(), kubernetesVersion, *ptr.Deref(machineScope.ProxmoxMachine.Spec.MetadataSettings, infrav1.MetadataSettings{ProviderIDInjection: ptr.To(false)}).ProviderIDInjection)

	injector := getISOInjector(machineScope.VirtualMachine, bootstrapData, metadata, network)
	return injector.Inject(ctx, inject.CloudConfigFormat)
}

func injectIgnition(ctx context.Context, machineScope *scope.MachineScope, bootstrapData []byte, biosUUID string, nicData []types.NetworkConfigData, kubernetesVersion string) error {
	// create metadata renderer
	metadata := cloudinit.NewMetadata(biosUUID, machineScope.Name(), kubernetesVersion, *ptr.Deref(machineScope.ProxmoxMachine.Spec.MetadataSettings, infrav1.MetadataSettings{ProviderIDInjection: ptr.To(false)}).ProviderIDInjection)

	// create an enricher
	enricher := &ignition.Enricher{
		BootstrapData: bootstrapData,
		Hostname:      machineScope.Name(),
		InstanceID:    biosUUID,
		ProviderID:    fmt.Sprintf("proxmox://%s", biosUUID),
		Network:       nicData,
	}

	injector := getIgnitionISOInjector(machineScope.VirtualMachine, metadata, enricher)
	return injector.Inject(ctx, inject.IgnitionFormat)
}

type isoInjector interface {
	Inject(ctx context.Context, format inject.BootstrapDataFormat) error
}

func defaultISOInjector(vm *proxmox.VirtualMachine, bootStrapData []byte, metadata, network cloudinit.Renderer) isoInjector {
	return &inject.ISOInjector{
		VirtualMachine:  vm,
		BootstrapData:   bootStrapData,
		MetaRenderer:    metadata,
		NetworkRenderer: network,
	}
}

func defaultIgnitionISOInjector(vm *proxmox.VirtualMachine, metadata cloudinit.Renderer, enricher *ignition.Enricher) isoInjector {
	return &inject.ISOInjector{
		VirtualMachine:   vm,
		IgnitionEnricher: enricher,
		MetaRenderer:     metadata,
	}
}

var (
	getISOInjector         = defaultISOInjector
	getIgnitionISOInjector = defaultIgnitionISOInjector
)

// getBootstrapData obtains a machine's bootstrap data from the relevant K8s secret and returns the data.
// TODO: Add format return if ignition will be supported.
func getBootstrapData(ctx context.Context, scope *scope.MachineScope) ([]byte, *string, error) {
	if scope.Machine.Spec.Bootstrap.DataSecretName == nil {
		scope.Logger.Info("machine has no bootstrap data.")
		return nil, nil, errors.New("machine has no bootstrap data")
	}

	secret := &corev1.Secret{}
	if err := scope.GetBootstrapSecret(ctx, secret); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to retrieve bootstrap data secret")
	}

	format := cloudinit.FormatCloudConfig
	f, ok := secret.Data["format"]
	if ok {
		format = string(f)
	}

	value, ok := secret.Data["value"]
	if !ok {
		return nil, nil, errors.New("error retrieving bootstrap data: secret `value` key is missing")
	}

	return value, &format, nil
}

func getNetworkConfigData(ctx context.Context, machineScope *scope.MachineScope) ([]types.NetworkConfigData, error) {
	// provide a default in case network is not defined
	network := ptr.Deref(machineScope.ProxmoxMachine.Spec.Network, infrav1.NetworkSpec{})
	networkConfigData := make([]types.NetworkConfigData, 0, len(network.NetworkDevices)+len(network.VRFs))

	networkConfig, err := getNetworkDevices(ctx, machineScope, network)
	if err != nil {
		return nil, err
	}
	networkConfigData = append(networkConfigData, networkConfig...)

	virtualConfig, err := getVirtualNetworkDevices(ctx, machineScope, network, networkConfigData)
	if err != nil {
		return nil, err
	}
	networkConfigData = append(networkConfigData, virtualConfig...)

	return networkConfigData, nil
}

func getNetworkConfigDataForDevice(ctx context.Context, machineScope *scope.MachineScope, device string, ipPoolRefs map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress) (*types.NetworkConfigData, error) {
	if device == "" {
		// this should never happen outwith tests
		return nil, errors.New("empty device name")
	}

	nets := machineScope.VirtualMachine.VirtualMachineConfig.MergeNets()
	// For nics supporting multiple IP addresses, we need to cut the '-inet' or '-inet6' part,
	// to retrieve the correct MAC address.
	formattedDevice, _, _ := strings.Cut(device, "-")
	macAddress := extractMACAddress(nets[formattedDevice])
	if len(macAddress) == 0 {
		machineScope.Logger.Error(errors.New("unable to extract mac address"), "device has no mac address", "device", device)
		return nil, errors.New("unable to extract mac address")
	}

	// Keys need to be sorted as golang doesn't guarantee stable map iteration
	ipAddresses := slices.Concat(slices.Collect(maps.Values(ipPoolRefs))...)
	slices.SortFunc(ipAddresses, func(a, b ipamv1.IPAddress) int {
		return strings.Compare(a.Name, b.Name)
	})

	ipConfigs := make([]types.IPConfig, 0, len(ipAddresses))
	for _, ipAddr := range ipAddresses {
		ipConfig := types.IPConfig{}
		ip, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", ipAddr.Spec.Address, ipAddr.Spec.Prefix))
		if err != nil {
			return nil, errors.Wrapf(err, "error converting ip address spec to netip prefix: %+v", ipAddr.Spec)
		}
		ipConfig.IPAddress = ip
		ipConfig.Gateway = ipAddr.Spec.Gateway

		// TODO: IPConfigs is stupid. No need to gather metrics here.
		metric, err := findIPAddressGatewayMetric(ctx, machineScope, &ipAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "error converting metric annotation, kind=%s, name=%s", ipAddr.Spec.PoolRef.Kind, ipAddr.Spec.PoolRef.Name)
		}
		ipConfig.Metric = metric

		isDefaultGateway := ipAddr.GetAnnotations()[infrav1.ProxmoxDefaultGatewayAnnotation]
		if b, _ := strconv.ParseBool(isDefaultGateway); b {
			ipConfig.Default = true
		}

		ipConfigs = append(ipConfigs, ipConfig)
	}

	dns := machineScope.InfraCluster.ProxmoxCluster.Spec.DNSServers

	cloudinitNetworkConfigData := &types.NetworkConfigData{
		IPConfigs:  ipConfigs,
		MacAddress: macAddress,
		DNSServers: dns,
	}

	return cloudinitNetworkConfigData, nil
}

// getCommonInterfaceConfig sets data which is common to all types of network interfaces.
func getCommonInterfaceConfig(_ context.Context, _ *scope.MachineScope, ciconfig *types.NetworkConfigData, ifconfig infrav1.InterfaceConfig) {
	if len(ifconfig.DNSServers) != 0 {
		ciconfig.DNSServers = ifconfig.DNSServers
	}
	ciconfig.Routes = ifconfig.Routing.Routes
	ciconfig.FIBRules = ifconfig.Routing.RoutingPolicy
	ciconfig.LinkMTU = ifconfig.LinkMTU
}

func getNetworkDevices(ctx context.Context, machineScope *scope.MachineScope, network infrav1.NetworkSpec) ([]types.NetworkConfigData, error) {
	networkConfigData := make([]types.NetworkConfigData, 0, len(network.NetworkDevices))
	ipAddressMap := make(map[string]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)

	requeue, err := handleDevices(ctx, machineScope, ipAddressMap)
	if requeue || err != nil {
		// invalid state. Machine should've had all IPs assigned
		return nil, errors.Wrapf(err, "unable to get IPs for network config data")
	}

	// network devices.
	for i, nic := range network.NetworkDevices {
		var config = ptr.To(types.NetworkConfigData{})

		// TODO: Default device IPPool api change
		ipPoolRefs := ipAddressMap[*nic.Name]

		conf, err := getNetworkConfigDataForDevice(ctx, machineScope, *nic.Name, ipPoolRefs)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get network config data for device=%s", *nic.Name)
		}
		if len(nic.DNSServers) != 0 {
			config.DNSServers = nic.DNSServers
		}
		config = conf

		getCommonInterfaceConfig(ctx, machineScope, config, nic.InterfaceConfig)

		config.Name = fmt.Sprintf("eth%d", i)
		config.Type = "ethernet"
		config.ProxName = nic.Name

		// TODO: Figure device names for eth0
		if i == 0 {
			config.ProxName = ptr.To("net0")
		}

		if len(config.MacAddress) > 0 {
			networkConfigData = append(networkConfigData, *config)
		}
	}
	return networkConfigData, nil
}

func getVirtualNetworkDevices(_ context.Context, _ *scope.MachineScope, network infrav1.NetworkSpec, data []types.NetworkConfigData) ([]types.NetworkConfigData, error) {
	networkConfigData := make([]types.NetworkConfigData, 0, len(network.VRFs))

	for _, device := range network.VRFs {
		var config = ptr.To(types.NetworkConfigData{})
		config.Type = "vrf"
		config.Name = device.Name
		config.Table = device.Table

		for i, child := range device.Interfaces {
			for _, net := range data {
				if (net.Name == *child) || (ptr.Deref(net.ProxName, "") == *child) {
					config.Interfaces = append(config.Interfaces, net.Name)
				}
			}
			if len(config.Interfaces)-1 < i {
				return nil, errors.Errorf("unable to find vrf interface=%s child interface %s", config.Name, *child)
			}
		}

		config.Routes = device.Routing.Routes
		config.FIBRules = device.Routing.RoutingPolicy
		networkConfigData = append(networkConfigData, *config)
	}
	return networkConfigData, nil
}

func vmHasMacAddresses(machineScope *scope.MachineScope) bool {
	nets := machineScope.VirtualMachine.VirtualMachineConfig.MergeNets()
	if len(nets) == 0 {
		return false
	}
	for d := range nets {
		if macAddress := extractMACAddress(nets[d]); macAddress == "" {
			return false
		}
	}
	return true
}
