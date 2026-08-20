// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os/exec"
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
	return runCommandWithContext(ctx, name, args...)
}

func (execRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("operation cancelled")
		}
		return "", fmt.Errorf("%s failed: %w\nOutput: %s", name, err, string(output))
	}
	return string(output), nil
}

// defaultRunner is the commandRunner used by the application entry points.
var defaultRunner commandRunner = execRunner{}
