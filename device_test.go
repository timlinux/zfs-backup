// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"
)

// abyssLsblk mirrors a typical workstation: an NVMe system disk carrying the
// source pool, an internal data disk, and a freshly attached USB drive.
const abyssLsblk = `{
  "blockdevices": [
    {"name": "nvme0n1", "type": "disk", "size": 1024209543168, "model": "Samsung SSD 990", "rm": false, "mountpoint": null,
     "children": [
       {"name": "nvme0n1p1", "type": "part", "size": 536870912, "model": null, "rm": false, "mountpoint": "/boot"},
       {"name": "nvme0n1p2", "type": "part", "size": 1023672672256, "model": null, "rm": false, "mountpoint": null}
     ]},
    {"name": "sda", "type": "disk", "size": 4000787030016, "model": "WDC WD40EFRX", "rm": false, "mountpoint": null,
     "children": [
       {"name": "sda1", "type": "part", "size": 4000786030016, "model": null, "rm": false, "mountpoint": "/mnt/data"}
     ]},
    {"name": "sdb", "type": "disk", "size": 2000398934016, "model": "Seagate Expansion", "rm": true, "mountpoint": null},
    {"name": "loop0", "type": "loop", "size": 4096, "model": null, "rm": false, "mountpoint": "/snap"}
  ]
}`

// zpool status with the source pool on the NVMe partition.
const abyssZpoolStatus = `  pool: NIXROOT
 state: ONLINE
config:

	NAME         STATE     READ WRITE CKSUM
	NIXROOT      ONLINE       0     0     0
	  nvme0n1p2  ONLINE       0     0     0

errors: No known data errors
`

func abyssCandidates(t *testing.T) []deviceCandidate {
	t.Helper()
	runner := &fakeRunner{
		respond: func(name string, _ []string) (string, error) {
			if name == "lsblk" {
				return abyssLsblk, nil
			}
			return abyssZpoolStatus, nil
		},
	}
	candidates, err := collectDeviceCandidates(context.Background(), runner)
	if err != nil {
		t.Fatalf("collectDeviceCandidates: %v", err)
	}
	return candidates
}

// The disk carrying the running system must never be offered for wiping. This
// is the scenario the old free-text flow made one typo away: its placeholder
// was literally /dev/sda.
func TestSystemDiskIsVetoed(t *testing.T) {
	candidates := abyssCandidates(t)

	nvme, found := findCandidate(candidates, "/dev/nvme0n1")
	if !found {
		t.Fatal("the system disk should appear in the list, refused")
	}
	if nvme.Selectable() {
		t.Fatal("the system disk must not be selectable")
	}

	var reasons []string
	for _, v := range nvme.Vetoes {
		reasons = append(reasons, v.Reason)
	}
	joined := strings.Join(reasons, "; ")
	if !strings.Contains(joined, "running system") {
		t.Errorf("the veto should say the system lives here: %s", joined)
	}
	if !strings.Contains(joined, "NIXROOT") {
		t.Errorf("the veto should say the source pool lives here: %s", joined)
	}
}

// A disk with a mounted partition is in use, even if the disk node itself
// carries no mountpoint.
func TestMountedDataDiskIsVetoed(t *testing.T) {
	candidates := abyssCandidates(t)

	sda, _ := findCandidate(candidates, "/dev/sda")
	if sda.Selectable() {
		t.Fatal("/dev/sda has a mounted partition and must be refused")
	}
	if !strings.Contains(sda.Vetoes[0].Reason, "/mnt/data") {
		t.Errorf("the veto should name the mount point: %s", sda.Vetoes[0].Reason)
	}
}

// The unattached USB drive is the one legitimate target.
func TestFreshUSBDriveIsSelectable(t *testing.T) {
	candidates := abyssCandidates(t)

	sdb, found := findCandidate(candidates, "/dev/sdb")
	if !found {
		t.Fatal("/dev/sdb should be offered")
	}
	if !sdb.Selectable() {
		t.Fatalf("a clean removable disk should be selectable, vetoed: %+v", sdb.Vetoes)
	}
	if !strings.Contains(sdb.Describe(), "removable") {
		t.Errorf("the picker line should say it is removable: %q", sdb.Describe())
	}
}

// Loop devices, partitions and other non-disks never reach the picker.
func TestOnlyWholeDisksAreOffered(t *testing.T) {
	for _, c := range abyssCandidates(t) {
		if c.Device.Type != "disk" {
			t.Errorf("non-disk %s reached the picker", c.Device.Path())
		}
	}
}

// If ZFS membership cannot be established, the picker must refuse to run
// rather than offer disks it cannot vouch for.
func TestUnknownZFSStateRefusesToOfferAnything(t *testing.T) {
	runner := &fakeRunner{
		respond: func(name string, _ []string) (string, error) {
			if name == "lsblk" {
				return abyssLsblk, nil
			}
			return "", context.DeadlineExceeded
		},
	}

	if _, err := collectDeviceCandidates(context.Background(), runner); err == nil {
		t.Fatal("not knowing what ZFS is using must be an error, not an empty veto list")
	}
}

func TestZfsMemberDevicesParsesMultiplePools(t *testing.T) {
	status := abyssZpoolStatus + `
  pool: NIXBACKUPS
 state: ONLINE
config:

	NAME        STATE     READ WRITE CKSUM
	NIXBACKUPS  ONLINE       0     0     0
	  sdc1      ONLINE       0     0     0
`
	members := zfsMemberDevices(status)

	if members["nvme0n1p2"] != "NIXROOT" {
		t.Errorf("nvme0n1p2 should map to NIXROOT, got %q", members["nvme0n1p2"])
	}
	if members["sdc1"] != "NIXBACKUPS" {
		t.Errorf("sdc1 should map to NIXBACKUPS, got %q", members["sdc1"])
	}
}

// Typing the disk's own name proves the user knows which disk dies; a shared
// word like DESTROY proves only that they can read a prompt.
func TestWipeConfirmationIsTheDeviceName(t *testing.T) {
	if got := wipeConfirmationWord("/dev/sdb"); got != "sdb" {
		t.Errorf("wipeConfirmationWord = %q, want sdb", got)
	}
	if got := wipeConfirmationWord("/dev/nvme0n1"); got != "nvme0n1" {
		t.Errorf("wipeConfirmationWord = %q, want nvme0n1", got)
	}
}
