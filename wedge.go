// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// =============================================================================
// Surviving a wedged pool
// =============================================================================
//
// When ZFS suspends a pool's I/O, commands that touch it block in
// UNINTERRUPTIBLE kernel sleep: SIGKILL does not take effect until the
// pool's I/O resumes, so exec.CommandContext cannot unblock a caller -
// Wait() hangs right alongside the child. Every command this application
// runs therefore goes through execWithDeadline, which abandons the wedged
// process instead of waiting for it, and the pool state is read from procfs
// - which never issues pool I/O - to say plainly what happened.

// Command deadlines. Queries are quick or wedged; actions get long enough
// for a slow USB pool export to flush.
var (
	queryTimeout  = 30 * time.Second
	actionTimeout = 5 * time.Minute
)

// zfsProcRoot is where the kernel exposes per-pool state without any pool
// I/O. Variable so tests can point it at a fixture.
var zfsProcRoot = "/proc/spl/kstat/zfs"

// poolStateFromProc reads a pool's state from procfs. Instant and safe on a
// wedged pool, unlike `zpool status`.
func poolStateFromProc(pool string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(zfsProcRoot, pool, "state"))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

// suspendedPoolsFromProc lists every imported pool the kernel reports as
// suspended.
func suspendedPoolsFromProc() []string {
	entries, err := os.ReadDir(zfsProcRoot)
	if err != nil {
		return nil
	}
	var suspended []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if state, ok := poolStateFromProc(entry.Name()); ok && strings.EqualFold(state, "SUSPENDED") {
			suspended = append(suspended, entry.Name())
		}
	}
	return suspended
}

// wedgedCommandError explains a command that never returned, naming the
// suspended pool when the kernel knows it.
func wedgedCommandError(name string, args []string, timeout time.Duration) error {
	msg := fmt.Sprintf("%s %s did not return within %s - the command is wedged in the kernel,\nwhich almost always means a pool has suspended I/O after losing its device",
		name, strings.Join(args, " "), timeout)
	if pools := suspendedPoolsFromProc(); len(pools) > 0 {
		msg += fmt.Sprintf("\n\nThe kernel confirms suspended pool(s): %s.\n\n"+
			"Reconnect the drive (a different port or cable is worth trying), then\n"+
			"run, in a fresh terminal:\n"+
			"    sudo zpool clear %s\n"+
			"Hung commands drain by themselves once the pool resumes. If the drive\n"+
			"will not come back, the pool can safely STAY suspended - it harms\n"+
			"nothing else and loses no data. Watch it without blocking:\n"+
			"    cat /proc/spl/kstat/zfs/%s/state", strings.Join(pools, ", "), pools[0], pools[0])
	}
	return fmt.Errorf("%s", msg)
}

// execResult carries a finished command out of its goroutine.
type execResult struct {
	output []byte
	err    error
}

// execWithDeadline runs a command and abandons it if it exceeds the deadline
// (from timeout, or from a deadline already on ctx when timeout is 0). The
// abandoned goroutine and child linger until the kernel releases them -
// which it will not do for a wedged pool until its I/O resumes - but the
// caller stays alive, which is the entire point.
func execWithDeadline(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	done := make(chan execResult, 1)
	go func() {
		output, err := cmd.CombinedOutput()
		done <- execResult{output: output, err: err}
	}()

	finish := func(r execResult) (string, error) {
		switch {
		case ctx.Err() == context.DeadlineExceeded:
			return "", wedgedCommandError(name, args, timeout)
		case ctx.Err() != nil:
			return "", fmt.Errorf("operation cancelled")
		case r.err != nil:
			return "", commandFailure(name, r.err, r.output)
		default:
			return string(r.output), nil
		}
	}

	select {
	case r := <-done:
		return finish(r)
	case <-ctx.Done():
		// Give the kill a moment to take effect on a healthy child, so the
		// common timeout still collects the real output-so-far semantics.
		select {
		case r := <-done:
			return finish(r)
		case <-time.After(2 * time.Second):
			// The child ignored SIGKILL: uninterruptible sleep. Abandon it.
			if ctx.Err() == context.DeadlineExceeded {
				return "", wedgedCommandError(name, args, timeout)
			}
			return "", fmt.Errorf("operation cancelled")
		}
	}
}
