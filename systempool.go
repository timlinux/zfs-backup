// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
)

// =============================================================================
// System pool vetting - the destination must never be the pool you run on
// =============================================================================
//
// THE DESTINATION VETTING INVARIANT
//
//	A pool holding the running system must never be a local backup
//	destination.
//
// The local backup flow treats its destination as a disposable backup medium:
// the legacy-layout migration renames the destination's flat datasets into
// <destPool>/<hostname>/..., pruning destroys destination snapshots, and the
// run ends by exporting the pool. Pointed at the pool the system boots from,
// those steps relocate or destroy live data - on this very machine a swapped
// selection once renamed NIXROOT/atuin into NIXROOT/abyss/atuin, and only a
// failed rename on a mounted dataset stopped /home following it.
//
// The one legitimate write into a system pool is the pull-from-remote flow,
// which never migrates or prunes and only ever creates datasets underneath
// <pool>/<remote-hostname>/. That flow therefore does not call this vet.

// criticalSystemMounts are mountpoints that identify a pool as hosting the
// running system. A dataset of the pool mounted at any of these means the
// pool must be refused as a local backup destination.
var criticalSystemMounts = map[string]bool{
	"/":     true,
	"/home": true,
	"/nix":  true,
	"/boot": true,
	"/etc":  true,
	"/usr":  true,
	"/var":  true,
}

// systemDatasetsFromList parses `zfs list -H -o name,mountpoint,mounted -r
// <pool>` output and returns a description of every dataset that is mounted
// at a critical system path. Pure, so it can be tested without a pool.
func systemDatasetsFromList(listOutput string) []string {
	var system []string
	for _, line := range strings.Split(strings.TrimSpace(listOutput), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		name, mountpoint, mounted := fields[0], fields[1], fields[2]
		if mounted == "yes" && criticalSystemMounts[mountpoint] {
			system = append(system, fmt.Sprintf("%s (mounted at %s)", name, mountpoint))
		}
	}
	return system
}

// systemPoolReason reports why a pool counts as hosting the running system,
// or "" when it does not. A pool that cannot be listed is not a system pool:
// the running system's pool is by definition imported and listable, so an
// unimported destination (the normal case for an external drive before
// stage 1 imports it) passes the vet.
func systemPoolReason(ctx context.Context, r commandRunner, pool string) string {
	out, err := r.Output(ctx, "zfs", "list", "-H", "-o", "name,mountpoint,mounted", "-r", pool)
	if err != nil {
		return ""
	}
	system := systemDatasetsFromList(out)
	if len(system) == 0 {
		return ""
	}
	return strings.Join(system, ", ")
}

// vetLocalBackupDestination refuses a destination pool that hosts the running
// system. Called before a local backup touches anything.
func vetLocalBackupDestination(ctx context.Context, r commandRunner, destPool string) error {
	reason := systemPoolReason(ctx, r, destPool)
	if reason == "" {
		return nil
	}
	return fmt.Errorf("%s holds the running system - it cannot be a backup destination.\n\n"+
		"The vet found: %s.\n\n"+
		"A local backup would migrate, prune and finally export this pool,\n"+
		"relocating or destroying live data. To back up a REMOTE host into\n"+
		"this pool, use Pull Remote Backup instead - it writes only under\n"+
		"%s/<remote-hostname>/ and never touches anything else",
		destPool, reason, destPool)
}
