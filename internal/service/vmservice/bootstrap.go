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
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/ignition"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/network"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
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
			Message: err.Error(),
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
			Message: err.Error(),
		})
		return false, err
	}

	kubernetesVersion := machineScope.Machine.Spec.Version

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
			Message: err.Error(),
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

func injectCloudInit(ctx context.Context, machineScope *scope.MachineScope, bootstrapData []byte, biosUUID string, nicData []network.NetworkConfigData, kubernetesVersion string) error {
	// create network renderer
	network := cloudinit.NewNetworkConfig(nicData)

	// create metadata renderer
	metadata := cloudinit.NewMetadata(biosUUID, machineScope.Name(), kubernetesVersion, *ptr.Deref(machineScope.ProxmoxMachine.Spec.MetadataSettings, infrav1.MetadataSettings{ProviderIDInjection: ptr.To(false)}).ProviderIDInjection)

	injector := getISOInjector(machineScope.VirtualMachine, bootstrapData, metadata, network)
	return injector.Inject(ctx, inject.CloudConfigFormat)
}

func injectIgnition(ctx context.Context, machineScope *scope.MachineScope, bootstrapData []byte, biosUUID string, nicData []network.NetworkConfigData, kubernetesVersion string) error {
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

func getNetworkConfigData(ctx context.Context, machineScope *scope.MachineScope) ([]network.NetworkConfigData, error) {
	// provide a default in case network is not defined
	networkSpec := ptr.Deref(machineScope.ProxmoxMachine.Spec.Network, infrav1.NetworkSpec{})
	networkConfigData := make([]network.NetworkConfigData, 0, len(networkSpec.NetworkDevices)+len(networkSpec.VRFs))

	networkConfig, err := getNetworkDevices(ctx, machineScope, networkSpec)
	if err != nil {
		return nil, err
	}
	networkConfigData = append(networkConfigData, networkConfig...)

	virtualConfig, err := getVirtualNetworkDevices(ctx, machineScope, networkSpec, networkConfigData)
	if err != nil {
		return nil, err
	}
	networkConfigData = append(networkConfigData, virtualConfig...)

	return networkConfigData, nil
}

func getNetworkConfigDataForDevice(ctx context.Context, machineScope *scope.MachineScope, device infrav1.NetName, ipPoolRefs map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress) (*network.NetworkConfigData, error) {
	if device == "" {
		// this should never happen outwith tests
		return nil, errors.New("empty device name")
	}

	nets := machineScope.VirtualMachine.VirtualMachineConfig.MergeNets()

	macAddress := extractMACAddress(nets[string(device)])
	if len(macAddress) == 0 {
		machineScope.Logger.Error(errors.New("unable to extract mac address"), "device has no mac address", "device", device)
		return nil, errors.New("unable to extract mac address")
	}

	dns := machineScope.InfraCluster.ProxmoxCluster.Spec.DNSServers

	cloudinitNetworkConfigData := &network.NetworkConfigData{
		MacAddress: macAddress,
		DNSServers: dns,
	}

	// Keys need to be sorted as golang doesn't guarantee stable map iteration
	ipAddresses := slices.Concat(slices.Collect(maps.Values(ipPoolRefs))...)
	slices.SortFunc(ipAddresses, func(a, b ipamv1.IPAddress) int {
		return strings.Compare(a.Name, b.Name)
	})

	for _, ipAddr := range ipAddresses {
		ipConfig := network.IPConfig{}
		ip, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", ipAddr.Spec.Address, ptr.Deref(ipAddr.Spec.Prefix, 0)))
		if err != nil {
			return nil, errors.Wrapf(err, "error converting ip address spec to netip prefix: %+v", ipAddr.Spec)
		}
		ipConfig.IPAddress = ip

		isDefaultGateway := ipAddr.GetAnnotations()[infrav1.ProxmoxDefaultGatewayAnnotation]
		if b, _ := strconv.ParseBool(isDefaultGateway); b {
			ipConfig.Default = true
		}

		cloudinitNetworkConfigData.IPConfigs = append(cloudinitNetworkConfigData.IPConfigs, ipConfig)

		gateway := ipAddr.Spec.Gateway
		if len(gateway) == 0 {
			continue
		}

		via, err := netip.ParseAddr(gateway)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid gateway %q for ip %s", gateway, ipAddr.Name)
		}

		defaultPrefix := netip.PrefixFrom(netip.IPv4Unspecified(), 0)
		if ip.Addr().Is6() {
			defaultPrefix = netip.PrefixFrom(netip.IPv6Unspecified(), 0)
		}

		metric, err := findIPAddressGatewayMetric(ctx, machineScope, &ipAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "error converting metric annotation, kind=%s, name=%s", ipAddr.Spec.PoolRef.Kind, ipAddr.Spec.PoolRef.Name)
		}

		cloudinitNetworkConfigData.Routes = append(cloudinitNetworkConfigData.Routes, network.RoutingData{
			To:     defaultPrefix,
			Via:    via,
			Metric: metric,
		})
	}

	return cloudinitNetworkConfigData, nil
}

// getCommonInterfaceConfig sets data which is common to all types of network interfaces.
func getCommonInterfaceConfig(_ context.Context, _ *scope.MachineScope, ciconfig *network.NetworkConfigData, ifconfig infrav1.InterfaceConfig) error {
	if len(ifconfig.DNSServers) != 0 {
		ciconfig.DNSServers = ifconfig.DNSServers
	}
	routes, err := ToRoutingData(ifconfig.Routing.Routes)
	if err != nil {
		return err
	}
	rules, err := ToFIBRuleData(ifconfig.Routing.RoutingPolicy)
	if err != nil {
		return err
	}
	ciconfig.Routes = append(ciconfig.Routes, routes...)
	ciconfig.FIBRules = rules
	ciconfig.LinkMTU = ifconfig.LinkMTU
	return nil
}

func getNetworkDevices(ctx context.Context, machineScope *scope.MachineScope, networkSpec infrav1.NetworkSpec) ([]network.NetworkConfigData, error) {
	networkConfigData := make([]network.NetworkConfigData, 0, len(networkSpec.NetworkDevices))
	ipAddressMap := make(map[infrav1.NetName]map[corev1.TypedLocalObjectReference][]ipamv1.IPAddress)

	requeue, err := handleDevices(ctx, machineScope, ipAddressMap)
	if requeue || err != nil {
		// invalid state. Machine should've had all IPs assigned
		return nil, errors.Wrapf(err, "unable to get IPs for network config data")
	}

	// network devices.
	for i, nic := range networkSpec.NetworkDevices {
		ipPoolRefs := ipAddressMap[nic.Name]

		conf, err := getNetworkConfigDataForDevice(ctx, machineScope, nic.Name, ipPoolRefs)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get network config data for device=%s", nic.Name)
		}
		// Per-interface DNS override (nic.DNSServers) is applied by
		// getCommonInterfaceConfig below; conf.DNSServers is already seeded with
		// the cluster default in getNetworkConfigDataForDevice.
		if err := getCommonInterfaceConfig(ctx, machineScope, conf, nic.InterfaceConfig); err != nil {
			return nil, errors.Wrapf(err, "unable to convert routing config for device=%s", nic.Name)
		}

		conf.Name = fmt.Sprintf("eth%d", i)
		conf.Type = "ethernet"
		conf.ProxName = nic.Name

		if i == 0 {
			conf.ProxName = infrav1.DefaultNetworkDevice
		}

		if len(conf.MacAddress) > 0 {
			networkConfigData = append(networkConfigData, *conf)
		}
	}
	return networkConfigData, nil
}

func getVirtualNetworkDevices(_ context.Context, _ *scope.MachineScope, networkSpec infrav1.NetworkSpec, data []network.NetworkConfigData) ([]network.NetworkConfigData, error) {
	networkConfigData := make([]network.NetworkConfigData, 0, len(networkSpec.VRFs))

	for _, device := range networkSpec.VRFs {
		var config = ptr.To(network.NetworkConfigData{})
		config.Type = "vrf"
		config.Name = device.Name
		config.Table = ptr.To(device.Table)

		for i, child := range device.Interfaces {
			for _, net := range data {
				if net.Name == string(child) || net.ProxName == child {
					config.Children = append(config.Children, net.Name)
				}
			}
			if len(config.Children) < i+1 {
				return nil, errors.Errorf("unable to find vrf interface=%s child interface %s", config.Name, child)
			}
		}

		var err error
		config.Routes, err = ToRoutingData(device.Routing.Routes)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to convert routes for vrf=%s", config.Name)
		}
		config.FIBRules, err = ToFIBRuleData(device.Routing.RoutingPolicy)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to convert routing policy for vrf=%s", config.Name)
		}
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
