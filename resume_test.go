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

// The failure that finally surfaced once errors stopped being swallowed:
//
//	failed to check key status for NIXBACKUPS: zfs failed: exit status 1:
//	cannot open 'NIXBACKUPS': pool I/O is currently suspended
//
// That is not something zfs-backup can fix, so the error has to say what the
// user must do instead of leaving them to search for it.
func TestSuspendedPoolFailureExplainsTheRecovery(t *testing.T) {
	guidance := diagnoseZFSFailure("cannot open 'NIXBACKUPS': pool I/O is currently suspended")

	if guidance == "" {
		t.Fatal("a suspended pool must be recognised")
	}
	for _, want := range []string{
		"suspended all I/O",
		"cannot fix this",
		"zpool clear NIXBACKUPS",
		"zpool export -f NIXBACKUPS",
		"zpool import NIXBACKUPS",
		"zpool status NIXBACKUPS",
		"loses no data",
	} {
		if !strings.Contains(guidance, want) {
			t.Errorf("guidance is missing %q:\n%s", want, guidance)
		}
	}
}

// The pool name is what the user needs in the commands, so a dataset-level
// message must still yield the pool.
func TestSuspendedPoolGuidanceNamesThePoolNotTheDataset(t *testing.T) {
	guidance := diagnoseZFSFailure("cannot open 'NIXBACKUPS/abyss/home': pool I/O is currently suspended")

	if !strings.Contains(guidance, "zpool clear NIXBACKUPS\n") {
		t.Errorf("expected the pool name in the commands, got:\n%s", guidance)
	}
	if strings.Contains(guidance, "NIXBACKUPS/abyss") {
		t.Errorf("zpool commands take a pool, not a dataset:\n%s", guidance)
	}
}

func TestSuspendedPoolRecognisedFromZpoolStatusWording(t *testing.T) {
	for _, wording := range []string{
		"cannot open 'TANK': pool I/O is currently suspended",
		"  state: SUSPENDED",
		"status: The pool is suspended because it lost connectivity",
	} {
		if diagnoseZFSFailure(wording) == "" {
			t.Errorf("suspension not recognised in %q", wording)
		}
	}
}

// Ordinary failures must not be padded with irrelevant advice.
func TestOrdinaryFailuresGetNoGuidance(t *testing.T) {
	for _, ordinary := range []string{
		"cannot open 'NIXBACKUPS': dataset does not exist",
		"cannot destroy snapshot: dataset is busy",
		"",
	} {
		if got := diagnoseZFSFailure(ordinary); got != "" {
			t.Errorf("no guidance expected for %q, got:\n%s", ordinary, got)
		}
	}
}

// The raw ZFS text must survive alongside the guidance - the guidance explains
// it, it does not replace it.
func TestCommandFailureKeepsBothTheMessageAndTheGuidance(t *testing.T) {
	err := commandFailure("zfs", errors.New("exit status 1"),
		[]byte("cannot open 'NIXBACKUPS': pool I/O is currently suspended"))

	text := err.Error()
	if !strings.Contains(text, "pool I/O is currently suspended") {
		t.Errorf("the original ZFS message must survive: %q", text)
	}
	if !strings.Contains(text, "zpool clear NIXBACKUPS") {
		t.Errorf("the guidance must be attached: %q", text)
	}
}

func TestVersionLabelShowsTheCommitItWasBuiltFrom(t *testing.T) {
	version, commit := appVersion, appCommit
	defer func() { appVersion, appCommit = version, commit }()

	appVersion, appCommit = "2.1.0", "fa247f7"
	if got := versionLabel(); got != "2.1.0 (fa247f7)" {
		t.Errorf("versionLabel() = %q, want %q", got, "2.1.0 (fa247f7)")
	}

	// A build with no SHA available must not show an empty pair of brackets.
	for _, missing := range []string{"", "unknown"} {
		appCommit = missing
		if got := versionLabel(); got != "2.1.0" {
			t.Errorf("with commit %q, versionLabel() = %q, want %q", missing, got, "2.1.0")
		}
	}
}

// The guidance is a sequence of shell commands whose indentation matters.
// Centring it line by line - which is what the result screen did to every
// error - scatters the commands and makes them unusable.
func TestErrorDetailKeepsCommandsLeftAligned(t *testing.T) {
	err := commandFailure("zfs", errors.New("exit status 1"),
		[]byte("cannot open 'NIXBACKUPS': pool I/O is currently suspended"))

	rendered := renderErrorDetail(err, 120)

	// These three differ in length, so centring indents each differently while
	// left-alignment puts them all at the same column.
	wanted := []string{
		"sudo zpool clear NIXBACKUPS",
		"sudo zpool export -f NIXBACKUPS",
		"zpool status NIXBACKUPS",
	}
	var commandLines []int
	for _, line := range strings.Split(rendered, "\n") {
		for _, cmd := range wanted {
			if strings.Contains(line, cmd) {
				column := strings.Index(line, strings.Fields(cmd)[0])
				commandLines = append(commandLines, column)
				break
			}
		}
	}
	if len(commandLines) < len(wanted) {
		t.Fatalf("expected %d command lines, found %d:\n%s", len(wanted), len(commandLines), rendered)
	}

	// Every command must start at the same column. Centring gives each a
	// different indent because the lines differ in length.
	for _, indent := range commandLines[1:] {
		if indent != commandLines[0] {
			t.Errorf("commands are not aligned (indents %v):\n%s", commandLines, rendered)
		}
	}
}

func TestErrorDetailLeavesASingleLineErrorAlone(t *testing.T) {
	rendered := renderErrorDetail(errors.New("source pool not selected"), 80)

	if !strings.Contains(rendered, "source pool not selected") {
		t.Errorf("the message must survive: %q", rendered)
	}
	if strings.Contains(rendered, "─") || strings.Contains(rendered, "│") {
		t.Errorf("a one-line error should not be boxed: %q", rendered)
	}
}
