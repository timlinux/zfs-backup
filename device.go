// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// =============================================================================
// Block device inspection - the safety net under "Prepare Backup Device"
// =============================================================================
//
// Preparing a backup device wipes it. The single most dangerous thing this
// application can do is wipe the wrong disk, so a device is inspected before
// it may be offered, and refused outright when anything on the system is
// still using it. The user picks from a vetted list rather than typing a
// /dev path from memory.

// blockDevice is one node of the lsblk tree.
type blockDevice struct {
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Size       int64         `json:"size"`
	Model      string        `json:"model"`
	Removable  bool          `json:"rm"`
	MountPoint string        `json:"mountpoint"`
	Children   []blockDevice `json:"children"`
}

// Path returns the /dev path of the device.
func (d blockDevice) Path() string {
	return "/dev/" + d.Name
}

// mountPoints collects every mount point on the device or its partitions.
func (d blockDevice) mountPoints() []string {
	var mounts []string
	if d.MountPoint != "" {
		mounts = append(mounts, d.MountPoint)
	}
	for _, child := range d.Children {
		mounts = append(mounts, child.mountPoints()...)
	}
	return mounts
}

// parseLsblk decodes `lsblk -J -b -o NAME,TYPE,SIZE,MODEL,RM,MOUNTPOINT`.
// Kept pure so every refusal rule can be tested without hardware.
func parseLsblk(data []byte) ([]blockDevice, error) {
	var tree struct {
		BlockDevices []blockDevice `json:"blockdevices"`
	}
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("failed to parse lsblk output: %w", err)
	}
	return tree.BlockDevices, nil
}

// listBlockDevices asks lsblk for the disk tree.
func listBlockDevices(ctx context.Context, r commandRunner) ([]blockDevice, error) {
	output, err := r.Output(ctx, "lsblk", "-J", "-b", "-o", "NAME,TYPE,SIZE,MODEL,RM,MOUNTPOINT")
	if err != nil {
		return nil, fmt.Errorf("failed to list block devices: %w", err)
	}
	return parseLsblk([]byte(output))
}

// zfsMemberDevices maps device base names (sda, nvme0n1p2, ...) to the
// imported pool using them, from `zpool status` output.
func zfsMemberDevices(statusOutput string) map[string]string {
	members := map[string]string{}
	pool := ""
	for _, line := range strings.Split(statusOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pool:") {
			pool = strings.TrimSpace(strings.TrimPrefix(trimmed, "pool:"))
			continue
		}
		if pool == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			name := path.Base(fields[0])
			switch fields[1] {
			case "ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED", "AVAIL", "INUSE", "SUSPENDED":
				members[name] = pool
			}
		}
	}
	// The pool line itself matches the vdev pattern ("TANK ONLINE 0 0 0");
	// harmless: it maps a pool name, never a device name.
	return members
}

// deviceVeto is one reason a device must not be wiped.
type deviceVeto struct {
	Reason string
}

// vetDeviceForWipe applies the refusal rules to one device. An empty result
// means the device may be offered for preparation. Pure: the caller supplies
// the lsblk tree and the zpool membership map.
func vetDeviceForWipe(device blockDevice, zfsMembers map[string]string) []deviceVeto {
	var vetoes []deviceVeto

	if device.Type != "disk" {
		vetoes = append(vetoes, deviceVeto{
			Reason: fmt.Sprintf("%s is a %s, not a whole disk", device.Path(), device.Type),
		})
	}

	for _, mount := range device.mountPoints() {
		reason := fmt.Sprintf("mounted at %s", mount)
		switch mount {
		case "/", "/boot", "/home", "/nix", "/var":
			reason = fmt.Sprintf("this disk holds the running system (%s)", mount)
		case "[SWAP]":
			reason = "this disk holds active swap"
		}
		vetoes = append(vetoes, deviceVeto{Reason: reason})
	}

	names := []string{device.Name}
	for _, child := range device.Children {
		names = append(names, child.Name)
	}
	for _, name := range names {
		if pool, used := zfsMembers[name]; used {
			vetoes = append(vetoes, deviceVeto{
				Reason: fmt.Sprintf("%s belongs to the imported ZFS pool %s", name, pool),
			})
		}
	}

	return vetoes
}

// deviceCandidate is a disk with its verdict, ready for the picker.
type deviceCandidate struct {
	Device blockDevice
	Vetoes []deviceVeto
}

// Selectable reports whether the picker may offer this disk for wiping.
func (c deviceCandidate) Selectable() bool {
	return len(c.Vetoes) == 0
}

// Describe renders one line for the picker: path, size, model, and - when the
// disk is refused - why.
func (c deviceCandidate) Describe() string {
	model := strings.TrimSpace(c.Device.Model)
	if model == "" {
		model = "unknown model"
	}
	kind := "internal"
	if c.Device.Removable {
		kind = "removable"
	}
	line := fmt.Sprintf("%-14s %8s  %s (%s)", c.Device.Path(), formatSize(c.Device.Size), model, kind)
	if !c.Selectable() {
		line += "  - " + c.Vetoes[0].Reason
	}
	return line
}

// collectDeviceCandidates inspects every whole disk on the system and vets
// each one. Read-only.
func collectDeviceCandidates(ctx context.Context, r commandRunner) ([]deviceCandidate, error) {
	devices, err := listBlockDevices(ctx, r)
	if err != nil {
		return nil, err
	}

	// Failing to read zpool status must not clear the vetoes: not knowing
	// which disks ZFS is using is a reason to refuse more, not less. With no
	// pools imported the command still succeeds with empty output.
	statusOutput, err := r.Output(ctx, "zpool", "status")
	if err != nil {
		return nil, fmt.Errorf("cannot check which disks ZFS is using: %w", err)
	}
	members := zfsMemberDevices(statusOutput)

	var candidates []deviceCandidate
	for _, device := range devices {
		if device.Type != "disk" {
			continue // loop devices, zram, partitions at top level
		}
		candidates = append(candidates, deviceCandidate{
			Device: device,
			Vetoes: vetDeviceForWipe(device, members),
		})
	}
	return candidates, nil
}

// findCandidate returns the candidate for a /dev path, if the disk exists.
func findCandidate(candidates []deviceCandidate, devPath string) (deviceCandidate, bool) {
	for _, c := range candidates {
		if c.Device.Path() == devPath {
			return c, true
		}
	}
	return deviceCandidate{}, false
}

// wipeConfirmationWord is what the user must type before a disk is wiped: the
// disk's own base name (e.g. "sdb"). Typing the name of the thing being
// destroyed proves the user knows which disk it is, which a generic word like
// DESTROY does not.
func wipeConfirmationWord(devPath string) string {
	return path.Base(devPath)
}
