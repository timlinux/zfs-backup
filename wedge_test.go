// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// useFakeProcRoot points the procfs reader at a fixture for one test.
func useFakeProcRoot(t *testing.T, states map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for pool, state := range states {
		if err := os.MkdirAll(filepath.Join(dir, pool), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, pool, "state"), []byte(state+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	previous := zfsProcRoot
	zfsProcRoot = dir
	t.Cleanup(func() { zfsProcRoot = previous })
}

func TestPoolStateFromProcReadsTheKernelState(t *testing.T) {
	useFakeProcRoot(t, map[string]string{"NIXBACKUPS": "SUSPENDED", "NIXROOT": "ONLINE"})

	if state, ok := poolStateFromProc("NIXBACKUPS"); !ok || state != "SUSPENDED" {
		t.Errorf("NIXBACKUPS = %q,%v, want SUSPENDED,true", state, ok)
	}
	if state, ok := poolStateFromProc("NIXROOT"); !ok || state != "ONLINE" {
		t.Errorf("NIXROOT = %q,%v, want ONLINE,true", state, ok)
	}
	if _, ok := poolStateFromProc("NOPE"); ok {
		t.Error("an unknown pool must not report a state")
	}
}

func TestSuspendedPoolsFromProcListsOnlySuspended(t *testing.T) {
	useFakeProcRoot(t, map[string]string{"NIXBACKUPS": "SUSPENDED", "NIXROOT": "ONLINE"})

	pools := suspendedPoolsFromProc()
	if len(pools) != 1 || pools[0] != "NIXBACKUPS" {
		t.Errorf("suspendedPoolsFromProc = %v, want [NIXBACKUPS]", pools)
	}
}

// The defect that hung the TUI: a child wedged in uninterruptible sleep
// ignores SIGKILL, so waiting on it after the kill waits forever. The
// deadline layer must abandon it and return.
func TestExecWithDeadlineAbandonsAWedgedChild(t *testing.T) {
	useFakeProcRoot(t, map[string]string{"NIXBACKUPS": "SUSPENDED"})

	start := time.Now()
	_, err := execWithDeadline(context.Background(), 200*time.Millisecond, "sleep", "30")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a command exceeding its deadline must error")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("the caller was held for %s - the whole point is not to wait", elapsed)
	}
	text := err.Error()
	if !strings.Contains(text, "did not return") || !strings.Contains(text, "wedged in the kernel") {
		t.Errorf("the error should explain the wedge: %q", text)
	}
	if !strings.Contains(text, "NIXBACKUPS") || !strings.Contains(text, "zpool clear NIXBACKUPS") {
		t.Errorf("the error should name the suspended pool and the remedy: %q", text)
	}
	if !strings.Contains(text, "STAY suspended") {
		t.Errorf("the error must not push a reboot as troubleshooting: %q", text)
	}
}

func TestExecWithDeadlineReturnsOutputWithinTheDeadline(t *testing.T) {
	out, err := execWithDeadline(context.Background(), 10*time.Second, "echo", "hello")
	if err != nil || strings.TrimSpace(out) != "hello" {
		t.Errorf("got %q, %v", out, err)
	}
}

func TestExecWithDeadlineHonoursCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()

	start := time.Now()
	_, err := execWithDeadline(ctx, 0, "sleep", "30")

	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("expected cancellation, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("cancellation must not wait for the child")
	}
}

// The recovery screen must not spend its 45-second deadline asking a wedged
// pool how wedged it is when the kernel answers instantly.
func TestCheckPoolHealthUsesTheProcfsFastPath(t *testing.T) {
	useFakeProcRoot(t, map[string]string{"NIXBACKUPS": "SUSPENDED"})
	runner := &fakeRunner{}

	health := checkPoolHealth(context.Background(), runner, "NIXBACKUPS")

	if health.State != poolSuspended {
		t.Errorf("state = %q, want suspended", health.State)
	}
	if len(runner.calls) != 0 {
		t.Errorf("no ZFS command may be issued when procfs already answers: %v", runner.commandLines())
	}
}

// A pool procfs knows nothing about still goes through zpool status.
func TestCheckPoolHealthFallsBackToZpoolStatus(t *testing.T) {
	useFakeProcRoot(t, map[string]string{})
	runner := &fakeRunner{
		respond: func(string, []string) (string, error) {
			return "  pool: TANK\n state: ONLINE\n", nil
		},
	}

	health := checkPoolHealth(context.Background(), runner, "TANK")

	if health.State != poolOnline || !runner.ran("zpool status TANK") {
		t.Errorf("expected the zpool status fallback, got %q, calls %v", health.State, runner.commandLines())
	}
}
