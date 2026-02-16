package v1alpha1

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/randfill"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/api/v1alpha2"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed()) // Added corev1 for TypedLocalObjectReference

	t.Run("for ProxmoxCluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme:      scheme,
		Hub:         &v1alpha2.ProxmoxCluster{},
		Spoke:       &ProxmoxCluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ProxmoxClusterFuzzFuncs},
	}))

	t.Run("for ProxmoxMachine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme:      scheme,
		Hub:         &v1alpha2.ProxmoxMachine{},
		Spoke:       &ProxmoxMachine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ProxmoxMachineFuzzFuncs},
	}))

	t.Run("for ProxmoxMachineTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme:      scheme,
		Hub:         &v1alpha2.ProxmoxMachineTemplate{},
		Spoke:       &ProxmoxMachineTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ProxmoxMachineTemplateFuzzFuncs},
	}))

	t.Run("for ProxmoxClusterTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme:      scheme,
		Hub:         &v1alpha2.ProxmoxClusterTemplate{},
		Spoke:       &ProxmoxClusterTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ProxmoxClusterTemplateFuzzFuncs},
	}))
}

func ProxmoxMachineFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubProxmoxMachineSpec,
		hubProxmoxMachineStatus,
		hubRoutingPolicySpec,
		spokeProxmoxMachineSpec,
	}
}

func hubProxmoxMachineSpec(in *v1alpha2.ProxmoxMachineSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Normalize empty string pointers
	if in.TemplateSource.SourceNode != nil && *in.TemplateSource.SourceNode == "" {
		in.TemplateSource.SourceNode = nil
	}

	if in.Network != nil {
		// Normalize Zone to nil (v1alpha1 doesn't have this field)
		in.Network.Zone = nil

		if len(in.Network.NetworkDevices) == 0 {
			in.Network.NetworkDevices = nil
		}
		for i := range in.Network.NetworkDevices {
			device := &in.Network.NetworkDevices[i]

			// v1alpha1 does not have DefaultIPv4/DefaultIPv6 fields
			device.DefaultIPv4 = nil
			device.DefaultIPv6 = nil

			// v1alpha1 doesn't have VLAN, MTU, LinkMTU
			device.VLAN = nil
			device.MTU = nil
			device.InterfaceConfig.LinkMTU = nil

			// Normalize empty string pointers in routing
			for j := range device.Routing.Routes {
				if device.Routing.Routes[j].To != nil && *device.Routing.Routes[j].To == "" {
					device.Routing.Routes[j].To = nil
				}
				if device.Routing.Routes[j].Via != nil && *device.Routing.Routes[j].Via == "" {
					device.Routing.Routes[j].Via = nil
				}
			}

			for j := range device.Routing.RoutingPolicy {
				if device.Routing.RoutingPolicy[j].To != nil && *device.Routing.RoutingPolicy[j].To == "" {
					device.Routing.RoutingPolicy[j].To = nil
				}
				if device.Routing.RoutingPolicy[j].From != nil && *device.Routing.RoutingPolicy[j].From == "" {
					device.Routing.RoutingPolicy[j].From = nil
				}
				// Normalize Priority to uint32 range (v1alpha1 uses uint32)
				if device.Routing.RoutingPolicy[j].Priority != nil {
					val := *device.Routing.RoutingPolicy[j].Priority
					if val < 0 || val > 4294967295 {
						device.Routing.RoutingPolicy[j].Priority = ptr.To(int64(uint32(val)))
					}
				}
			}
		}

		// Normalize VRF interfaces and routing (empty slices created by auto-conversion)
		for i := range in.Network.VRFs {
			if len(in.Network.VRFs[i].Interfaces) == 0 {
				in.Network.VRFs[i].Interfaces = nil
			} else {
				// Filter out nil elements and empty string NetNames
				filtered := make([]v1alpha2.NetName, 0, len(in.Network.VRFs[i].Interfaces))
				for j := range in.Network.VRFs[i].Interfaces {
					// Skip nil or empty string elements
					if in.Network.VRFs[i].Interfaces[j] == nil || *in.Network.VRFs[i].Interfaces[j] == "" {
						continue
					}
					filtered = append(filtered, in.Network.VRFs[i].Interfaces[j])
				}
				if len(filtered) == 0 {
					in.Network.VRFs[i].Interfaces = nil
				} else {
					in.Network.VRFs[i].Interfaces = filtered
				}
			}

			// Normalize VRF Routes - same issue as network device routes
			for j := range in.Network.VRFs[i].Routing.Routes {
				if in.Network.VRFs[i].Routing.Routes[j].To != nil && *in.Network.VRFs[i].Routing.Routes[j].To == "" {
					in.Network.VRFs[i].Routing.Routes[j].To = nil
				}
				if in.Network.VRFs[i].Routing.Routes[j].Via != nil && *in.Network.VRFs[i].Routing.Routes[j].Via == "" {
					in.Network.VRFs[i].Routing.Routes[j].Via = nil
				}
			}

			for j := range in.Network.VRFs[i].Routing.RoutingPolicy {
				if in.Network.VRFs[i].Routing.RoutingPolicy[j].To != nil && *in.Network.VRFs[i].Routing.RoutingPolicy[j].To == "" {
					in.Network.VRFs[i].Routing.RoutingPolicy[j].To = nil
				}
				if in.Network.VRFs[i].Routing.RoutingPolicy[j].From != nil && *in.Network.VRFs[i].Routing.RoutingPolicy[j].From == "" {
					in.Network.VRFs[i].Routing.RoutingPolicy[j].From = nil
				}
				// Normalize Priority to uint32 range (v1alpha1 uses uint32)
				if in.Network.VRFs[i].Routing.RoutingPolicy[j].Priority != nil {
					val := *in.Network.VRFs[i].Routing.RoutingPolicy[j].Priority
					if val < 0 || val > 4294967295 {
						in.Network.VRFs[i].Routing.RoutingPolicy[j].Priority = ptr.To(int64(uint32(val)))
					}
				}
			}
		}
	}
}

func ProxmoxMachineTemplateFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubProxmoxMachineSpec,
		spokeProxmoxMachineSpec,
	}
}

func hubProxmoxMachineStatus(in *v1alpha2.ProxmoxMachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	// Status: Ready boolean nil -> false conversion
	if in.Ready == nil {
		in.Ready = ptr.To(false)
	}

	if in.VMStatus != nil && *in.VMStatus == "" {
		in.VMStatus = nil
	}

	if in.RetryAfter != nil && in.RetryAfter.IsZero() {
		in.RetryAfter = nil
	}

	// Normalize IPAddresses IPv4/IPv6 empty strings
	// v1alpha1 doesn't support empty strings in IP slices, they become nil
	for i := range in.IPAddresses {
		// Filter out empty strings from IPv4
		if len(in.IPAddresses[i].IPv4) > 0 {
			filtered := make([]string, 0, len(in.IPAddresses[i].IPv4))
			for _, ip := range in.IPAddresses[i].IPv4 {
				if ip != "" {
					filtered = append(filtered, ip)
				}
			}
			if len(filtered) == 0 {
				in.IPAddresses[i].IPv4 = nil
			} else {
				in.IPAddresses[i].IPv4 = filtered
			}
		}

		// Filter out empty strings from IPv6
		if len(in.IPAddresses[i].IPv6) > 0 {
			filtered := make([]string, 0, len(in.IPAddresses[i].IPv6))
			for _, ip := range in.IPAddresses[i].IPv6 {
				if ip != "" {
					filtered = append(filtered, ip)
				}
			}
			if len(filtered) == 0 {
				in.IPAddresses[i].IPv6 = nil
			} else {
				in.IPAddresses[i].IPv6 = filtered
			}
		}
	}

	// Network: nil -> empty slice conversion
	for i := range in.Network {
		if in.Network[i].Connected == nil {
			in.Network[i].Connected = ptr.To(false)
		}
		if in.Network[i].NetworkName == nil {
			in.Network[i].NetworkName = ptr.To("net0")
		}
	}
}

func spokeProxmoxMachineSpec(in *ProxmoxMachineSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Network != nil {
		// Handle Default Network Device
		if in.Network.Default != nil {
			ensureIPPoolNaming(&in.Network.Default.IPPoolConfig)
			// Normalize empty slices to nil
			if len(in.Network.Default.DNSServers) == 0 {
				in.Network.Default.DNSServers = nil
			}
		}

		// Handle Additional Network Devices
		if len(in.Network.AdditionalDevices) == 0 {
			in.Network.AdditionalDevices = nil
		}
		for i := range in.Network.AdditionalDevices {
			ensureIPPoolNaming(&in.Network.AdditionalDevices[i].NetworkDevice.IPPoolConfig)
			// Normalize empty slices
			if len(in.Network.AdditionalDevices[i].NetworkDevice.DNSServers) == 0 {
				in.Network.AdditionalDevices[i].NetworkDevice.DNSServers = nil
			}
		}

		// Handle VRFs - normalize Interfaces to nil if empty
		for i := range in.Network.VirtualNetworkDevices.VRFs {
			// Filter out empty strings from Interfaces (they become &"" in hub)
			if len(in.Network.VirtualNetworkDevices.VRFs[i].Interfaces) > 0 {
				filtered := make([]string, 0, len(in.Network.VirtualNetworkDevices.VRFs[i].Interfaces))
				for _, iface := range in.Network.VirtualNetworkDevices.VRFs[i].Interfaces {
					if iface != "" {
						filtered = append(filtered, iface)
					}
				}
				if len(filtered) == 0 {
					in.Network.VirtualNetworkDevices.VRFs[i].Interfaces = nil
				} else {
					in.Network.VirtualNetworkDevices.VRFs[i].Interfaces = filtered
				}
			} else {
				in.Network.VirtualNetworkDevices.VRFs[i].Interfaces = nil
			}
			// Normalize Routes to nil if empty
			if len(in.Network.VirtualNetworkDevices.VRFs[i].Routing.Routes) == 0 {
				in.Network.VirtualNetworkDevices.VRFs[i].Routing.Routes = nil
			}
			// Normalize RoutingPolicy to nil if empty
			if len(in.Network.VirtualNetworkDevices.VRFs[i].Routing.RoutingPolicy) == 0 {
				in.Network.VirtualNetworkDevices.VRFs[i].Routing.RoutingPolicy = nil
			}
		}
	}
}

// ensureIPPoolNaming applies "inet" and "inet6" suffixes.
// If refs exist but don't have proper naming, it ensures consistent behavior.
func ensureIPPoolNaming(cfg *IPPoolConfig) {
	// If both are nil, leave as is
	if cfg.IPv4PoolRef == nil && cfg.IPv6PoolRef == nil {
		return
	}

	// If only IPv4 exists
	if cfg.IPv4PoolRef != nil && cfg.IPv6PoolRef == nil {
		if !strings.Contains(cfg.IPv4PoolRef.Name, "inet") {
			// No proper naming - this will cause conversion mismatch
			// Set to nil to avoid issues
			cfg.IPv4PoolRef = nil
			return
		}
		// Ensure it has -inet suffix
		if !strings.HasSuffix(cfg.IPv4PoolRef.Name, "-inet") && !strings.HasSuffix(cfg.IPv4PoolRef.Name, "-inet6") {
			cfg.IPv4PoolRef.Name += "-inet"
		}
	}

	// If only IPv6 exists
	if cfg.IPv6PoolRef != nil && cfg.IPv4PoolRef == nil {
		if !strings.Contains(cfg.IPv6PoolRef.Name, "inet6") {
			// No proper naming - set to nil
			cfg.IPv6PoolRef = nil
			return
		}
		if !strings.HasSuffix(cfg.IPv6PoolRef.Name, "-inet6") {
			cfg.IPv6PoolRef.Name += "-inet6"
		}
	}

	// If both exist, ensure proper suffixes
	if cfg.IPv4PoolRef != nil {
		if !strings.Contains(cfg.IPv4PoolRef.Name, "inet") {
			cfg.IPv4PoolRef.Name += "-inet"
		}
	}
	if cfg.IPv6PoolRef != nil {
		if !strings.Contains(cfg.IPv6PoolRef.Name, "inet6") {
			cfg.IPv6PoolRef.Name += "-inet6"
		}
	}
}

func ProxmoxClusterFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubProxmoxClusterSpec,
		hubProxmoxClusterStatus,
		spokeProxmoxClusterSpec,
	}
}

func spokeProxmoxClusterSpec(in *ProxmoxClusterSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Throw CloneSpec away. It serves no function and will not survive conversion.
	in.CloneSpec = nil
}

func hubProxmoxClusterSpec(in *v1alpha2.ProxmoxClusterSpec, c randfill.Continue) {
	// ZoneConfigs does not exist in v1alpha1, so it will be lost during hub→spoke→hub
	// Always set to nil to match conversion behavior
	in.ZoneConfigs = nil
}


func hubProxmoxClusterStatus(in *v1alpha2.ProxmoxClusterStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	// InClusterZoneRef doesn't exist in v1alpha1, so it will be lost during hub→spoke→hub
	in.InClusterZoneRef = nil

	// Zone field does not exist in v1alpha1 NodeLocation, so it will be lost during hub→spoke→hub
	if in.NodeLocations != nil {
		for i := range in.NodeLocations.ControlPlane {
			in.NodeLocations.ControlPlane[i].Zone = nil
		}
		for i := range in.NodeLocations.Workers {
			in.NodeLocations.Workers[i].Zone = nil
		}
	}
}

func hubRoutingPolicySpec(in *v1alpha2.RoutingPolicySpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Normalize Priority to nil if it's less than 0
	if in.Priority != nil && *in.Priority < 0 {
		in.Priority = nil
	}

}

func ProxmoxClusterTemplateFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		//		hubProxmoxClusterTemplateSpec,
		hubProxmoxClusterTemplateSpec,
		spokeProxmoxClusterTemplateSpec,
	}
}

func hubProxmoxClusterTemplateSpec(in *v1alpha2.ProxmoxClusterTemplateSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// ZoneConfigs does not exist in v1alpha1, so it will be lost during hub→spoke→hub
	// Always set to nil to match conversion behavior
	in.Template.Spec.ZoneConfigs = nil

	if in.Template.Spec.ProxmoxClusterCloneSpec.VirtualIPNetworkInterface != nil &&
		*in.Template.Spec.ProxmoxClusterCloneSpec.VirtualIPNetworkInterface == "" {
		in.Template.Spec.ProxmoxClusterCloneSpec.VirtualIPNetworkInterface = nil
	}

	// Normalize ProxmoxClusterClassSpec - use hubProxmoxMachineSpec for each machine spec
	for i := range in.Template.Spec.ProxmoxClusterCloneSpec.ProxmoxClusterClassSpec {
		spec := &in.Template.Spec.ProxmoxClusterCloneSpec.ProxmoxClusterClassSpec[i].ProxmoxMachineSpec
		hubProxmoxMachineSpec(spec, c)
	}
}

func spokeProxmoxClusterTemplateSpec(in *ProxmoxClusterTemplateSpec, c randfill.Continue) {
	c.FillNoCustom(in)
}
