// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
)

// fakeRunner is a commandRunner that records every command it is asked to run
// and answers reads from a canned responder. It lets the snapshot, prune and
// cleanup logic be tested without a real ZFS pool - and, crucially, lets a test
// assert which datasets were touched and which were not.
type fakeRunner struct {
	calls   [][]string
	respond func(name string, args []string) (string, error)
}

func (f *fakeRunner) record(name string, args []string) {
	f.calls = append(f.calls, append([]string{name}, args...))
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.record(name, args)
	if f.respond == nil {
		return nil
	}
	_, err := f.respond(name, args)
	return err
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) (string, error) {
	f.record(name, args)
	if f.respond == nil {
		return "", nil
	}
	return f.respond(name, args)
}

// commandLines renders the recorded calls as whitespace-joined strings.
func (f *fakeRunner) commandLines() []string {
	lines := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		lines = append(lines, strings.Join(call, " "))
	}
	return lines
}

// ran reports whether any recorded command contains all the given fragments.
func (f *fakeRunner) ran(fragments ...string) bool {
	for _, line := range f.commandLines() {
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(line, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// mentions reports whether any recorded command mentions the given string.
func (f *fakeRunner) mentions(needle string) bool {
	return f.ran(needle)
}
