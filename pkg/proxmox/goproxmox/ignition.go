package goproxmox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	"github.com/luthermonson/go-proxmox"
)

var (
	blockSize        = 2048
	volumeIdentifier = "config-2"

	TagCloudInit = "cloud-init"
)

// Ignition takes a json doc as a string and make an ISO, upload it to the data store as <vmid>-user-data.iso and will
// mount it as a CD-ROM to be used with nutanix config-drive  'config-2'.
func (c *APIClient) Ignition(ctx context.Context, v *proxmox.VirtualMachine, device, userdata string) error {
	UserDataISOFormat := "user-data-%d.iso"
	isoName := fmt.Sprintf(UserDataISOFormat, v.VMID)
	// create userdata iso file on the local fs
	iso, err := makeCloudInitISO(isoName, userdata)
	if err != nil {
		return err
	}

	defer func() {
		// _ = os.Remove(iso.Name())
	}()

	node, err := c.Node(ctx, v.Node)
	if err != nil {
		return err
	}

	storage, err := node.StorageISO(ctx)
	if err != nil {
		return err
	}

	task, err := storage.Upload("iso", iso.Name())
	if err != nil {
		return err
	}

	// iso should only be < 5mb so wait for it and then mount it
	if err := task.WaitFor(ctx, 5); err != nil {
		return err
	}

	_, err = v.AddTag(ctx, proxmox.MakeTag(TagCloudInit))
	if err != nil && !proxmox.IsErrNoop(err) {
		return err
	}

	task, err = v.Config(ctx, proxmox.VirtualMachineOption{
		Name:  device,
		Value: fmt.Sprintf("%s:iso/%s,media=cdrom", storage.Name, isoName),
	}, proxmox.VirtualMachineOption{
		Name:  "boot",
		Value: fmt.Sprintf("%s;%s", v.VirtualMachineConfig.Boot, device),
	})

	if err != nil {
		return err
	}

	return task.WaitFor(ctx, 2)
}

func makeCloudInitISO(filename, userdata string) (iso *os.File, err error) {
	iso, err = os.Create(filepath.Join(os.TempDir(), filename))
	if err != nil {
		return nil, err
	}

	defer func() {
		err = iso.Close()
	}()

	fs, err := iso9660.Create(iso, 0, 0, int64(blockSize), "")
	if err != nil {
		return nil, err
	}

	if err := fs.Mkdir("/openstack/latest"); err != nil {
		return nil, err
	}

	cifiles := map[string]string{
		"user_data": userdata,
	}

	for filename, content := range cifiles {
		rw, err := fs.OpenFile("/openstack/latest/"+filename, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return nil, err
		}

		if _, err := rw.Write([]byte(content)); err != nil {
			return nil, err
		}
	}

	if err = fs.Finalize(iso9660.FinalizeOptions{
		RockRidge:        true,
		VolumeIdentifier: volumeIdentifier,
	}); err != nil {
		return nil, err
	}

	return
}
