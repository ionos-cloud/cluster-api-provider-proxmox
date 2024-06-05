/*
Copyright 2023-2024 IONOS Cloud.

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
	"fmt"
	"strings"

	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1alpha1 "github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/internal/inject"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/cloudinit"
	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/scope"
)

func reconcileBootstrapData(ctx context.Context, machineScope *scope.MachineScope) (requeue bool, err error) {
	if ptr.Deref(machineScope.ProxmoxMachine.Status.BootstrapDataProvided, false) {
		// skip machine already have the bootstrap data.
		return false, nil
	}

	if !machineHasIPAddress(machineScope.ProxmoxMachine) {
		// skip machine doesn't have an IpAddress yet.
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition, infrav1alpha1.WaitingForStaticIPAllocationReason, clusterv1.ConditionSeverityWarning, "no ip address")
		return true, nil
	}

	// make sure MacAddress is set.
	if !vmHasMacAddresses(machineScope) {
		return true, nil
	}

	machineScope.Logger.V(4).Info("reconciling BootstrapData.")

	// Get the bootstrap data.
	bootstrapData, err := getBootstrapData(ctx, machineScope)
	if err != nil {
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition, infrav1alpha1.CloningFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return false, err
	}

	biosUUID := extractUUID(machineScope.VirtualMachine.VirtualMachineConfig.SMBios1)

	nicData, err := getNetworkConfigData(ctx, machineScope)
	if err != nil {
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition, infrav1alpha1.WaitingForStaticIPAllocationReason, clusterv1.ConditionSeverityWarning, err.Error())
		return false, err
	}

	// create network renderer
	network := cloudinit.NewNetworkConfig(nicData)

	// create metadata renderer
	metadata := cloudinit.NewMetadata(biosUUID, machineScope.Name())

	injector := getISOInjector(machineScope.VirtualMachine, bootstrapData, metadata, network)
	if err = injector.Inject(ctx); err != nil {
		conditions.MarkFalse(machineScope.ProxmoxMachine, infrav1alpha1.VMProvisionedCondition, infrav1alpha1.VMProvisionFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return false, errors.Wrap(err, "cloud-init iso inject failed")
	}

	machineScope.ProxmoxMachine.Status.BootstrapDataProvided = ptr.To(true)

	return false, nil
}

type isoInjector interface {
	Inject(ctx context.Context) error
}

func defaultISOInjector(vm *proxmox.VirtualMachine, bootStrapData []byte, metadata, network cloudinit.Renderer) isoInjector {
	return &inject.ISOInjector{
		VirtualMachine:  vm,
		BootstrapData:   bootStrapData,
		MetaRenderer:    metadata,
		NetworkRenderer: network,
	}
}

var getISOInjector = defaultISOInjector

// getBootstrapData obtains a machine's bootstrap data from the relevant K8s secret and returns the data.
// TODO: Add format return if ignition will be supported.
func getBootstrapData(ctx context.Context, scope *scope.MachineScope) ([]byte, error) {
	if scope.Machine.Spec.Bootstrap.DataSecretName == nil {
		scope.Logger.Info("machine has no bootstrap data.")
		return nil, errors.New("machine has no bootstrap data")
	}

	secret := &corev1.Secret{}
	if err := scope.GetBootstrapSecret(ctx, secret); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve bootstrap data secret")
	}

	value, ok := secret.Data["value"]
	if !ok {
		return nil, errors.New("error retrieving bootstrap data: secret `value` key is missing")
	}

	return value, nil
}

func getNetworkConfigData(ctx context.Context, machineScope *scope.MachineScope) ([]cloudinit.NetworkConfigData, error) {
	// provide a default in case network is not defined
	network := ptr.Deref(machineScope.ProxmoxMachine.Spec.Network, infrav1alpha1.NetworkSpec{})
	networkConfigData := make([]cloudinit.NetworkConfigData, 0, 1+len(network.AdditionalDevices)+len(network.VRFs))

	defaultConfig, err := getDefaultNetworkDevice(ctx, machineScope)
	if err != nil {
		return nil, err
	}
	networkConfigData = append(networkConfigData, defaultConfig...)

	additionalConfig, err := getAdditionalNetworkDevices(ctx, machineScope, network)
	if err != nil {
		return nil, err
	}
	networkConfigData = append(networkConfigData, additionalConfig...)

	virtualConfig, err := getVirtualNetworkDevices(ctx, machineScope, network, networkConfigData)
	if err != nil {
		return nil, err
	}
	networkConfigData = append(networkConfigData, virtualConfig...)

	return networkConfigData, nil
}

func getRoutingData(routes []infrav1alpha1.RouteSpec) *[]cloudinit.RoutingData {
	routingData := make([]cloudinit.RoutingData, 0, len(routes))
	for _, route := range routes {
		routeSpec := cloudinit.RoutingData{}
		routeSpec.To = route.To
		routeSpec.Via = route.Via
		routeSpec.Metric = route.Metric
		routeSpec.Table = route.Table
		routingData = append(routingData, routeSpec)
	}

	return &routingData
}

func getRoutingPolicyData(rules []infrav1alpha1.RoutingPolicySpec) *[]cloudinit.FIBRuleData {
	routingPolicyData := make([]cloudinit.FIBRuleData, 0, len(rules))
	for _, rule := range rules {
		ruleSpec := cloudinit.FIBRuleData{}
		ruleSpec.To = rule.To
		ruleSpec.From = rule.From
		ruleSpec.Priority = rule.Priority
		if rule.Table != nil {
			ruleSpec.Table = *rule.Table
		}
		routingPolicyData = append(routingPolicyData, ruleSpec)
	}

	return &routingPolicyData
}

func getNetworkConfigDataForDevice(ctx context.Context, machineScope *scope.MachineScope, device string) (*cloudinit.NetworkConfigData, error) {
	nets := machineScope.VirtualMachine.VirtualMachineConfig.MergeNets()
	// For nics supporting multiple IP addresses, we need to cut the '-inet' or '-inet6' part,
	// to retrieve the correct MAC address.
	formattedDevice, _, _ := strings.Cut(device, "-")
	macAddress := extractMACAddress(nets[formattedDevice])
	if len(macAddress) == 0 {
		machineScope.Logger.Error(errors.New("unable to extract mac address"), "device has no mac address", "device", device)
		return nil, errors.New("unable to extract mac address")
	}
	// retrieve IPAddress.
	ipAddr, err := findIPAddress(ctx, machineScope, device)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find IPAddress, device=%s", device)
	}

	dns := machineScope.InfraCluster.ProxmoxCluster.Spec.DNSServers
	ip := IPAddressWithPrefix(ipAddr.Spec.Address, ipAddr.Spec.Prefix)
	gw := ipAddr.Spec.Gateway
	metric, err := findIPAddressGatewayMetric(ctx, machineScope, ipAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "error converting metric annotation, kind=%s, name=%s", ipAddr.Spec.PoolRef.Kind, ipAddr.Spec.PoolRef.Name)
	}

	cloudinitNetworkConfigData := &cloudinit.NetworkConfigData{
		MacAddress: macAddress,
		DNSServers: dns,
	}

	// If it's an IPv6 address, we must set Gateway6 and IPV6Address instead
	if strings.Contains(ip, ":") {
		cloudinitNetworkConfigData.Gateway6 = gw
		cloudinitNetworkConfigData.Metric6 = metric
		cloudinitNetworkConfigData.IPV6Address = ip
	} else {
		cloudinitNetworkConfigData.Gateway = gw
		cloudinitNetworkConfigData.Metric = metric
		cloudinitNetworkConfigData.IPAddress = ip
	}

	return cloudinitNetworkConfigData, nil
}

func getDefaultNetworkDevice(ctx context.Context, machineScope *scope.MachineScope) ([]cloudinit.NetworkConfigData, error) {
	var config cloudinit.NetworkConfigData

	// default network device ipv4.
	if machineScope.InfraCluster.ProxmoxCluster.Spec.IPv4Config != nil {
		conf, err := getNetworkConfigDataForDevice(ctx, machineScope, DefaultNetworkDeviceIPV4)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get network config data for device=%s", DefaultNetworkDeviceIPV4)
		}
		config = *conf
	}

	// default network device ipv6.
	if machineScope.InfraCluster.ProxmoxCluster.Spec.IPv6Config != nil {
		conf, err := getNetworkConfigDataForDevice(ctx, machineScope, DefaultNetworkDeviceIPV6)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get network config data for device=%s", DefaultNetworkDeviceIPV6)
		}

		switch {
		case len(config.MacAddress) == 0:
			config = *conf
		case config.MacAddress != conf.MacAddress:
			return nil, errors.New("default network device ipv4 and ipv6 have different mac addresses")
		default:
			config.IPV6Address = conf.IPV6Address
			config.Gateway6 = conf.Gateway6
			config.Metric6 = conf.Metric6
		}
	}

	// Default Network Device lacks a datastructure to transport MTU.
	// We can use the Proxmox Device MTU instead to enable non virtio devices
	// the usage of jumbo frames. This has the minor drawback of coalescing proxmox
	// MTU with interface MTU, which shouldn't matter in almost all cases.
	if network := machineScope.ProxmoxMachine.Spec.Network; network != nil {
		if network.Default != nil {
			if network.Default.MTU != nil && *network.Default.MTU >= 576 {
				config.LinkMTU = network.Default.MTU
			}
		}
	}

	config.Name = "eth0"
	config.Type = "ethernet"
	config.ProxName = "net0"

	return []cloudinit.NetworkConfigData{config}, nil
}

func getCommonInterfaceConfig(ctx context.Context, machineScope *scope.MachineScope, ciconfig *cloudinit.NetworkConfigData, ifconfig infrav1alpha1.InterfaceConfig) error {
	if len(ifconfig.DNSServers) != 0 {
		ciconfig.DNSServers = ifconfig.DNSServers
	}
	ciconfig.Routes = *getRoutingData(ifconfig.Routing.Routes)
	ciconfig.FIBRules = *getRoutingPolicyData(ifconfig.Routing.RoutingPolicy)
	ciconfig.LinkMTU = ifconfig.LinkMTU

	// Only set IPAddresses if they haven't been set yet
	if ippool := ifconfig.IPv4PoolRef; ippool != nil && ciconfig.IPAddress == "" {
		// retrieve IPAddress.
		var ifname = fmt.Sprintf("%s-%s", ciconfig.Name, infrav1alpha1.DefaultSuffix)
		ipAddr, err := findIPAddress(ctx, machineScope, ifname)
		if err != nil {
			return errors.Wrapf(err, "unable to find IPAddress, device=%s", ifname)
		}
		metric, err := findIPAddressGatewayMetric(ctx, machineScope, ipAddr)
		if err != nil {
			return errors.Wrapf(err, "error converting metric annotation, kind=%s, name=%s", ipAddr.Spec.PoolRef.Kind, ipAddr.Spec.PoolRef.Name)
		}

		ciconfig.IPAddress = IPAddressWithPrefix(ipAddr.Spec.Address, ipAddr.Spec.Prefix)
		ciconfig.Gateway = ipAddr.Spec.Gateway
		ciconfig.Metric = metric
	}
	if ifconfig.IPv6PoolRef != nil && ciconfig.IPV6Address == "" {
		var ifname = fmt.Sprintf("%s-%s", ciconfig.Name, infrav1alpha1.DefaultSuffix+"6")
		ipAddr, err := findIPAddress(ctx, machineScope, ifname)
		if err != nil {
			return errors.Wrapf(err, "unable to find IPAddress, device=%s", ifname)
		}
		metric, err := findIPAddressGatewayMetric(ctx, machineScope, ipAddr)
		if err != nil {
			return errors.Wrapf(err, "error converting metric annotation, kind=%s, name=%s", ipAddr.Spec.PoolRef.Kind, ipAddr.Spec.PoolRef.Name)
		}
		ciconfig.IPV6Address = IPAddressWithPrefix(ipAddr.Spec.Address, ipAddr.Spec.Prefix)
		ciconfig.Gateway6 = ipAddr.Spec.Gateway
		ciconfig.Metric6 = metric
	}

	return nil
}

func getVirtualNetworkDevices(_ context.Context, _ *scope.MachineScope, network infrav1alpha1.NetworkSpec, data []cloudinit.NetworkConfigData) ([]cloudinit.NetworkConfigData, error) {
	networkConfigData := make([]cloudinit.NetworkConfigData, 0, len(network.VRFs))

	for _, device := range network.VRFs {
		var config = ptr.To(cloudinit.NetworkConfigData{})
		config.Type = "vrf"
		config.Name = device.Name
		config.Table = device.Table

		for i, child := range device.Interfaces {
			for _, net := range data {
				if (net.Name == child) || (net.ProxName == child) {
					config.Interfaces = append(config.Interfaces, net.Name)
				}
			}
			if len(config.Interfaces)-1 < i {
				return nil, errors.Errorf("unable to find vrf interface=%s child interface %s", config.Name, child)
			}
		}

		config.Routes = *getRoutingData(device.Routing.Routes)
		config.FIBRules = *getRoutingPolicyData(device.Routing.RoutingPolicy)
		networkConfigData = append(networkConfigData, *config)
	}
	return networkConfigData, nil
}

func getAdditionalNetworkDevices(ctx context.Context, machineScope *scope.MachineScope, network infrav1alpha1.NetworkSpec) ([]cloudinit.NetworkConfigData, error) {
	networkConfigData := make([]cloudinit.NetworkConfigData, 0, len(network.AdditionalDevices))

	// additional network devices append after the provisioning interface
	var index = 1
	// additional network devices.
	for _, nic := range network.AdditionalDevices {
		var config = ptr.To(cloudinit.NetworkConfigData{})

		if nic.IPv4PoolRef != nil {
			device := fmt.Sprintf("%s-%s", nic.Name, infrav1alpha1.DefaultSuffix)
			conf, err := getNetworkConfigDataForDevice(ctx, machineScope, device)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to get network config data for device=%s", device)
			}
			if len(nic.DNSServers) != 0 {
				config.DNSServers = nic.DNSServers
			}
			config = conf
		}

		if nic.IPv6PoolRef != nil {
			suffix := infrav1alpha1.DefaultSuffix + "6"
			device := fmt.Sprintf("%s-%s", nic.Name, suffix)
			conf, err := getNetworkConfigDataForDevice(ctx, machineScope, device)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to get network config data for device=%s", device)
			}
			if len(nic.DNSServers) != 0 {
				config.DNSServers = nic.DNSServers
			}

			config.IPV6Address = conf.IPV6Address
			config.Gateway6 = conf.Gateway6
		}

		err := getCommonInterfaceConfig(ctx, machineScope, config, nic.InterfaceConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get network config data for device=%s", nic.Name)
		}

		config.Name = fmt.Sprintf("eth%d", index)
		index++
		config.Type = "ethernet"
		config.ProxName = nic.Name

		if len(config.MacAddress) > 0 {
			networkConfigData = append(networkConfigData, *config)
		}
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
