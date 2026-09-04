// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A failed run leaves the earlier stages marked complete. Importing the pool
// and loading its key are not facts that stay true: the drive can be
// unplugged, the pool exported, the machine rebooted. Skipping them on resume
// left the next stage talking to a pool that was no longer there, which
// surfaced as "failed to check key status: zfs failed: exit status 1".
func TestResumeRechecksStagesTheWorldCanUndo(t *testing.T) {
	state := NewBackupState("backup")
	for _, stage := range []BackupStage{
		StageImportPool, StageLoadKey, StageCreateSnapshot, StageSyncData,
	} {
		state.MarkStageCompleted(stage, time.Second)
	}

	for _, stage := range []BackupStage{StageImportPool, StageLoadKey} {
		if state.ShouldSkipStage(stage) {
			t.Errorf("%s must be re-checked on resume, not skipped", stage)
		}
	}
}

// Re-checking must not throw away real progress: the expensive stages still
// resume where they left off.
func TestResumeStillSkipsWorkAlreadyDone(t *testing.T) {
	state := NewBackupState("backup")
	state.MarkStageCompleted(StageCreateSnapshot, time.Second)
	state.MarkStageCompleted(StageSyncData, time.Minute)

	for _, stage := range []BackupStage{StageCreateSnapshot, StageSyncData} {
		if !state.ShouldSkipStage(stage) {
			t.Errorf("%s was completed and is not revocable, so it should be skipped", stage)
		}
	}
}

func TestShouldSkipStageIsFalseForWorkNeverDone(t *testing.T) {
	state := NewBackupState("backup")

	for _, stage := range []BackupStage{
		StageImportPool, StageLoadKey, StageCreateSnapshot, StageSyncData,
		StagePruneLocal, StagePruneBackup, StageExportPool,
	} {
		if state.ShouldSkipStage(stage) {
			t.Errorf("%s has not run, so it must not be skipped", stage)
		}
	}
}

// Every revocable stage must be one whose body is safe to run again.
func TestRevocableStagesAreOnlyThePreconditionStages(t *testing.T) {
	for stage := range revocableStages {
		if stage != StageImportPool && stage != StageLoadKey {
			t.Errorf("%s is marked revocable but re-running it may not be idempotent", stage)
		}
	}
}

// "zfs failed: exit status 1" told the user nothing. ZFS explains itself on
// stderr, and that explanation must survive into the error.
func TestFormatCommandOutputKeepsWhatTheCommandSaid(t *testing.T) {
	got := formatCommandOutput([]byte("cannot open 'NIXBACKUPS': dataset does not exist\n"))

	want := ": cannot open 'NIXBACKUPS': dataset does not exist"
	if got != want {
		t.Errorf("formatCommandOutput = %q, want %q", got, want)
	}
}

func TestFormatCommandOutputBreaksMultiLineOutputOntoItsOwnLines(t *testing.T) {
	got := formatCommandOutput([]byte("first problem\nsecond problem\n"))

	if !strings.HasPrefix(got, "\nOutput: ") {
		t.Errorf("multi-line output should start on its own line, got %q", got)
	}
	for _, want := range []string{"first problem", "second problem"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lost %q: %q", want, got)
		}
	}
}

func TestFormatCommandOutputAddsNothingWhenTheCommandWasSilent(t *testing.T) {
	for _, quiet := range [][]byte{nil, {}, []byte("   \n\t ")} {
		if got := formatCommandOutput(quiet); got != "" {
			t.Errorf("silent command should add nothing to the error, got %q", got)
		}
	}
}

// The whole point: the message a user sees names the real problem.
func TestCommandErrorsNameTheRealProblem(t *testing.T) {
	err := fmt.Errorf("%s failed: %w%s", "zfs", errors.New("exit status 1"),
		formatCommandOutput([]byte("cannot open 'NIXBACKUPS': dataset does not exist")))

	if !strings.Contains(err.Error(), "dataset does not exist") {
		t.Errorf("the actionable part must reach the user, got %q", err)
	}
}

// isPoolImported used to substring-match the whole `zpool list` table, so a
// pool whose name merely appeared somewhere in the output counted as
// imported. The import was then skipped and the next stage failed against a
// pool that was not there.
func TestPoolListContainsMatchesWholeNamesOnly(t *testing.T) {
	// What `zpool list -H -o name` actually returns.
	output := "NIXROOT\nNIXBACKUPS2\n"

	if poolListContains(output, "NIXBACKUPS") {
		t.Error("NIXBACKUPS is not imported - NIXBACKUPS2 is a different pool")
	}
	if !poolListContains(output, "NIXBACKUPS2") {
		t.Error("NIXBACKUPS2 is imported and should be found")
	}
	if !poolListContains(output, "NIXROOT") {
		t.Error("NIXROOT is imported and should be found")
	}
}

func TestPoolListContainsHandlesNoPoolsAndBlankNames(t *testing.T) {
	if poolListContains("", "NIXBACKUPS") {
		t.Error("no pools are imported, so nothing should match")
	}
	if poolListContains("NIXROOT\n", "") {
		t.Error("an empty pool name must never match")
	}
	if poolListContains("\n\n", "") {
		t.Error("an empty pool name must not match a blank line")
	}
}

func TestPoolListContainsToleratesTrailingWhitespace(t *testing.T) {
	if !poolListContains("NIXROOT  \n  NIXBACKUPS\n", "NIXBACKUPS") {
		t.Error("padding around a name should not stop it matching")
	}
}
