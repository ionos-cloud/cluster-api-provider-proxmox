package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetInClusterIPPoolRef will set the reference to the provided InClusterIPPool.
// If nil was provided, the status field will be cleared.
func (c *ProxmoxCluster) SetInClusterIPPoolRef(pool metav1.Object) {
	if pool == nil || pool.GetName() == "" {
		c.Status.InClusterIPPoolRef = nil
		return
	}

	if c.Status.InClusterIPPoolRef == nil {
		c.Status.InClusterIPPoolRef = []corev1.LocalObjectReference{
			{Name: pool.GetName()},
		}
	}

	found := false
	for _, ref := range c.Status.InClusterIPPoolRef {
		if ref.Name == pool.GetName() {
			found = true
		}
	}
	if !found {
		c.Status.InClusterIPPoolRef = append(c.Status.InClusterIPPoolRef, corev1.LocalObjectReference{Name: pool.GetName()})
	}
}

// AddNodeLocation will add a node location to either the control plane or worker
// node locations based on the `isControlPlane` parameter.
func (c *ProxmoxCluster) AddNodeLocation(loc NodeLocation, isControlPlane bool) {
	if c.Status.NodeLocations == nil {
		c.Status.NodeLocations = new(NodeLocations)
	}

	if !c.HasMachine(loc.Machine.Name, isControlPlane) {
		c.addNodeLocation(loc, isControlPlane)
	}
}

// RemoveNodeLocation removes a node location from the status.
func (c *ProxmoxCluster) RemoveNodeLocation(machineName string, isControlPlane bool) {
	nodeLocations := c.Status.NodeLocations

	if nodeLocations == nil {
		return
	}

	if !c.HasMachine(machineName, isControlPlane) {
		return
	}

	if isControlPlane {
		for i, v := range nodeLocations.ControlPlane {
			if v.Machine.Name == machineName {
				nodeLocations.ControlPlane = append(nodeLocations.ControlPlane[:i], nodeLocations.ControlPlane[i+1:]...)
			}
		}
		return
	}

	for i, v := range nodeLocations.Workers {
		if v.Machine.Name == machineName {
			nodeLocations.Workers = append(nodeLocations.Workers[:i], nodeLocations.Workers[i+1:]...)
		}
	}
}

// UpdateNodeLocation will update the node location based on the provided machine name.
// If the node location does not exist, it will be added.
//
// The function returns true if the value was added or updated, otherwise false.
func (c *ProxmoxCluster) UpdateNodeLocation(machineName, node string, isControlPlane bool) bool {
	if !c.HasMachine(machineName, isControlPlane) {
		loc := NodeLocation{
			Node:    node,
			Machine: corev1.LocalObjectReference{Name: machineName},
		}
		c.AddNodeLocation(loc, isControlPlane)
		return true
	}

	locations := c.Status.NodeLocations.Workers
	if isControlPlane {
		locations = c.Status.NodeLocations.ControlPlane
	}

	for i, loc := range locations {
		if loc.Machine.Name == machineName {
			if loc.Node != node {
				locations[i].Node = node
				return true
			}

			return false
		}
	}

	return false
}

// HasMachine returns if true if a machine was found on any node.
func (c *ProxmoxCluster) HasMachine(machineName string, isControlPlane bool) bool {
	return c.GetNode(machineName, isControlPlane) != ""
}

// GetNode tries to return the Proxmox node for the provided machine name.
func (c *ProxmoxCluster) GetNode(machineName string, isControlPlane bool) string {
	if c.Status.NodeLocations == nil {
		return ""
	}

	if isControlPlane {
		for _, cpl := range c.Status.NodeLocations.ControlPlane {
			if cpl.Machine.Name == machineName {
				return cpl.Node
			}
		}
	} else {
		for _, wloc := range c.Status.NodeLocations.Workers {
			if wloc.Machine.Name == machineName {
				return wloc.Node
			}
		}
	}

	return ""
}

func (c *ProxmoxCluster) addNodeLocation(loc NodeLocation, isControlPlane bool) {
	if isControlPlane {
		c.Status.NodeLocations.ControlPlane = append(c.Status.NodeLocations.ControlPlane, loc)
		return
	}

	c.Status.NodeLocations.Workers = append(c.Status.NodeLocations.Workers, loc)
}
