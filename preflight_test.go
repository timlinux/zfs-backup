// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"
)

// preflightPool answers preflightSyncTarget's queries for a configurable
// target state. Unknown datasets fail to list, like real zfs.
type preflightPool struct {
	existing  map[string]bool
	snapshots string // returned for the snapshot listing of the target
	token     string // receive_resume_token value
	children  string // returned for the depth-1 name listing of the target
	used      string // parseable byte count for the used property
}

func (p preflightPool) respond(line string) (string, error) {
	switch {
	case strings.Contains(line, "-t snapshot"):
		return p.snapshots, nil
	case strings.Contains(line, "receive_resume_token"):
		return p.token, nil
	case strings.Contains(line, "-o value used"):
		return p.used, nil
	case strings.Contains(line, "-d 1"):
		return p.children, nil
	case strings.HasPrefix(line, "zfs list -H "):
		ds := line[strings.LastIndex(line, " ")+1:]
		if p.existing[ds] {
			return ds, nil
		}
		return "", listingError{}
	}
	return "", nil
}

func TestPreflightCreatesOnlyTheParentForAMissingTarget(t *testing.T) {
	r := scriptedRunner(preflightPool{existing: map[string]bool{}}.respond)
	var out strings.Builder

	if err := preflightSyncTarget(context.Background(), r, "NIXBACKUPS/abyss/home", &out); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if !r.ran("zfs create -p NIXBACKUPS/abyss") {
		t.Error("parent NIXBACKUPS/abyss was not created")
	}
	if r.ran("create", "NIXBACKUPS/abyss/home") {
		t.Error("the leaf target was pre-created - syncoid will refuse to replicate into it")
	}
}

func TestPreflightSkipsCreateWhenParentExists(t *testing.T) {
	r := scriptedRunner(preflightPool{
		existing: map[string]bool{"NIXBACKUPS/abyss": true},
	}.respond)
	var out strings.Builder

	if err := preflightSyncTarget(context.Background(), r, "NIXBACKUPS/abyss/home", &out); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if r.mentions("create") {
		t.Error("nothing needed creating, but a create was issued")
	}
}

func TestPreflightLeavesAnEstablishedTargetAlone(t *testing.T) {
	r := scriptedRunner(preflightPool{
		existing:  map[string]bool{"NIXBACKUPS/abyss/home": true},
		snapshots: "NIXBACKUPS/abyss/home@2026-08-01.10h-00-Backup\n",
	}.respond)
	var out strings.Builder

	if err := preflightSyncTarget(context.Background(), r, "NIXBACKUPS/abyss/home", &out); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if r.mentions("destroy") || r.mentions("create") {
		t.Error("an established target must not be touched")
	}
}

func TestPreflightDestroysEmptyPreCreatedDebris(t *testing.T) {
	// The state an older version left behind: the target exists because the
	// tool ran `zfs create` on it, with no snapshots, no children and only
	// metadata-sized usage. It blocks replication and holds nothing.
	r := scriptedRunner(preflightPool{
		existing: map[string]bool{"NIXBACKUPS/abyss/home": true},
		token:    "-",
		children: "NIXBACKUPS/abyss/home\n",
		used:     "98304",
	}.respond)
	var out strings.Builder

	if err := preflightSyncTarget(context.Background(), r, "NIXBACKUPS/abyss/home", &out); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if !r.ran("zfs destroy NIXBACKUPS/abyss/home") {
		t.Error("empty pre-created debris was not removed, so replication will fail again")
	}
	if !strings.Contains(out.String(), "Removing empty pre-created target") {
		t.Error("the repair was not noted in the run log")
	}
}

func TestPreflightSparesAResumablePartialReceive(t *testing.T) {
	r := scriptedRunner(preflightPool{
		existing: map[string]bool{"NIXBACKUPS/abyss/home": true},
		token:    "1-abc123-receive-token",
	}.respond)
	var out strings.Builder

	if err := preflightSyncTarget(context.Background(), r, "NIXBACKUPS/abyss/home", &out); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if r.mentions("destroy") {
		t.Error("a resumable partial receive was destroyed")
	}
}

func TestPreflightSparesATargetWithChildren(t *testing.T) {
	r := scriptedRunner(preflightPool{
		existing: map[string]bool{"NIXBACKUPS/abyss": true},
		token:    "-",
		children: "NIXBACKUPS/abyss\nNIXBACKUPS/abyss/home\n",
		used:     "98304",
	}.respond)
	var out strings.Builder

	if err := preflightSyncTarget(context.Background(), r, "NIXBACKUPS/abyss", &out); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if r.mentions("destroy") {
		t.Error("a target with children was destroyed")
	}
}

func TestPreflightSparesATargetWithRealData(t *testing.T) {
	r := scriptedRunner(preflightPool{
		existing: map[string]bool{"NIXBACKUPS/abyss/home": true},
		token:    "-",
		children: "NIXBACKUPS/abyss/home\n",
		used:     "52428800", // 50M - more than metadata
	}.respond)
	var out strings.Builder

	if err := preflightSyncTarget(context.Background(), r, "NIXBACKUPS/abyss/home", &out); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if r.mentions("destroy") {
		t.Error("a target holding real data was destroyed")
	}
}

func TestParentDataset(t *testing.T) {
	cases := map[string]string{
		"NIXBACKUPS/abyss/home": "NIXBACKUPS/abyss",
		"NIXBACKUPS/home":       "NIXBACKUPS",
		"NIXBACKUPS":            "",
	}
	for dataset, want := range cases {
		if got := parentDataset(dataset); got != want {
			t.Errorf("parentDataset(%q) = %q, want %q", dataset, got, want)
		}
	}
}
