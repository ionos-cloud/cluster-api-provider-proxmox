/*
Copyright 2023 IONOS Cloud.

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

package proxmox

import "github.com/luthermonson/go-proxmox"

// VMCloneRequest Is the object used to clone a VM.
type VMCloneRequest struct {
	Node        string `json:"node"`
	NewID       int    `json:"newID"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Format      string `json:"format,omitempty"`
	Full        uint8  `json:"full,omitempty"`
	Pool        string `json:"pool,omitempty"`
	SnapName    string `json:"snapname,omitempty"`
	Storage     string `json:"storage,omitempty"`
	Target      string `json:"target,omitempty"`
}

// VMCloneResponse response returned when cloning a VM.
type VMCloneResponse struct {
	NewID int64         `json:"newId,omitempty"`
	Task  *proxmox.Task `json:"task,omitempty"`
}

// VirtualMachineOption is an alias for VirtualMachineOption to prevent import conflicts.
type VirtualMachineOption = proxmox.VirtualMachineOption
