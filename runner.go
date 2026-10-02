// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
)

// commandRunner abstracts external command execution so that the snapshot,
// prune and orphan-detection logic can be exercised in tests without a real
// ZFS pool. Production code uses execRunner; tests substitute a fake that
// records the commands it was asked to run.
type commandRunner interface {
	// Run executes a command and discards its output.
	Run(ctx context.Context, name string, args ...string) error
	// Output executes a command and returns its combined output.
	Output(ctx context.Context, name string, args ...string) (string, error)
}

// execRunner is the production commandRunner, backed by os/exec.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) error {
	timeout := actionTimeout
	if _, has := ctx.Deadline(); has {
		timeout = 0 // the caller's deadline governs
	}
	_, err := execWithDeadline(ctx, timeout, name, args...)
	return err
}

func (execRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	timeout := queryTimeout
	if _, has := ctx.Deadline(); has {
		timeout = 0
	}
	return execWithDeadline(ctx, timeout, name, args...)
}

// defaultRunner is the commandRunner used by the application entry points.
var defaultRunner commandRunner = execRunner{}

// sshRunner is a commandRunner that runs every command on a remote host over
// SSH, so code written against commandRunner (e.g. byte-progress estimation)
// works unchanged whether the dataset it is querying is local or remote.
type sshRunner struct {
	host string
}

func (s sshRunner) Run(ctx context.Context, name string, args ...string) error {
	return execRunner{}.Run(ctx, "ssh", append([]string{s.host, name}, args...)...)
}

func (s sshRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	return execRunner{}.Output(ctx, "ssh", append([]string{s.host, name}, args...)...)
}
