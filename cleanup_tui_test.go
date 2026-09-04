// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// planFixture builds a cleanup plan by hand: two snapshots cleared for
// destruction and one held back by a safety check.
func planFixture() *cleanupPlan {
	decision := func(name, dataset string, used int64, safe bool, reason string) destroyDecision {
		_, tag, _ := splitSnapshot(name)
		return destroyDecision{
			Orphan: orphanSnapshot{
				snapshotEntry: snapshotEntry{
					Name:     name,
					Dataset:  dataset,
					Tag:      tag,
					Used:     used,
					Creation: time.Date(2026, 8, 13, 23, 47, 0, 0, time.UTC),
				},
				Kind: orphanOutOfScope,
			},
			Safe:       safe,
			SkipReason: reason,
		}
	}

	decisions := []destroyDecision{
		decision("NIXROOT/root@2026-08-13.23h-47-Backup", "NIXROOT/root", 2048, true, ""),
		decision("NIXROOT/root@blank", "NIXROOT/root", 999999, false, "protected snapshot"),
		decision("NIXROOT/nix@2026-08-13.23h-47-Backup", "NIXROOT/nix", 1024, true, ""),
	}

	return &cleanupPlan{
		Scan: &orphanScan{
			Pool:    "NIXROOT",
			InScope: []string{"home"},
		},
		Decisions: decisions,
		Targets:   safeToDestroy(decisions),
	}
}

// The TUI must never be a softer path to destruction than the CLI. It reaches
// ZFS through this one function, so the "one snapshot at a time" rule is
// asserted here rather than in each caller.
func TestDestroyPlannedSnapshotsDestroysOneAtATime(t *testing.T) {
	runner := &fakeRunner{}
	targets := []string{
		"NIXROOT/root@2026-08-13.23h-47-Backup",
		"NIXROOT/nix@2026-08-13.23h-47-Backup",
	}

	outcome := destroyPlannedSnapshots(context.Background(), runner, targets, nil)

	if len(outcome.Destroyed) != 2 || len(outcome.Failures) != 0 {
		t.Fatalf("expected both snapshots destroyed cleanly, got %+v", outcome)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected exactly one destroy per snapshot, got %v", runner.commandLines())
	}
	for i, call := range runner.calls {
		if len(call) != 3 || call[0] != "zfs" || call[1] != "destroy" || call[2] != targets[i] {
			t.Errorf("call %d was %v, want [zfs destroy %s]", i, call, targets[i])
		}
	}
	// A range expression or a recursive destroy would take out snapshots that
	// never appeared in the plan the user approved.
	for _, line := range runner.commandLines() {
		if strings.Contains(line, "%") {
			t.Errorf("range expression must never be used: %q", line)
		}
		if strings.Contains(line, " -r") || strings.Contains(line, " -R") {
			t.Errorf("recursive destroy must never be used: %q", line)
		}
	}
}

func TestDestroyPlannedSnapshotsReportsFailuresAndKeepsGoing(t *testing.T) {
	doomed := "NIXROOT/root@2026-08-13.23h-47-Backup"
	runner := &fakeRunner{
		respond: func(_ string, args []string) (string, error) {
			if len(args) > 1 && args[1] == doomed {
				return "", fmt.Errorf("dataset is busy")
			}
			return "", nil
		},
	}

	outcome := destroyPlannedSnapshots(context.Background(), runner,
		[]string{doomed, "NIXROOT/nix@2026-08-13.23h-47-Backup"}, nil)

	if len(outcome.Destroyed) != 1 {
		t.Errorf("a failure must not abort the remaining snapshots, got %v", outcome.Destroyed)
	}
	if len(outcome.Failures) != 1 || !strings.Contains(outcome.Failures[0], "dataset is busy") {
		t.Errorf("expected the failure reported verbatim, got %v", outcome.Failures)
	}
}

func TestDestroyPlannedSnapshotsReportsProgressOnlyForRealDestroys(t *testing.T) {
	runner := &fakeRunner{
		respond: func(_ string, _ []string) (string, error) {
			return "", fmt.Errorf("nope")
		},
	}
	var reported []string

	destroyPlannedSnapshots(context.Background(), runner,
		[]string{"NIXROOT/root@2026-08-13.23h-47-Backup"},
		func(name string) { reported = append(reported, name) })

	if len(reported) != 0 {
		t.Errorf("a failed destroy must not be reported as progress, got %v", reported)
	}
}

func TestPreviewDestroyNeverDestroys(t *testing.T) {
	runner := &fakeRunner{
		respond: func(_ string, _ []string) (string, error) {
			return "would destroy NIXROOT/root@2026-08-13.23h-47-Backup\n", nil
		},
	}

	lines := previewDestroy(context.Background(), runner,
		[]string{"NIXROOT/root@2026-08-13.23h-47-Backup"})

	if len(lines) != 1 || !strings.Contains(lines[0], "would destroy") {
		t.Errorf("expected the ZFS dry-run output passed through, got %v", lines)
	}
	for _, call := range runner.calls {
		if !contains(call, "-nv") {
			t.Errorf("preview must only ever run a dry run, got %v", call)
		}
	}
}

func TestPreviewDestroyKeepsGoingWhenOneDryRunFails(t *testing.T) {
	runner := &fakeRunner{
		respond: func(_ string, args []string) (string, error) {
			if contains(args, "NIXROOT/root@2026-08-13.23h-47-Backup") {
				return "", fmt.Errorf("no such snapshot")
			}
			return "would destroy NIXROOT/nix@2026-08-13.23h-47-Backup\n", nil
		},
	}

	lines := previewDestroy(context.Background(), runner, []string{
		"NIXROOT/root@2026-08-13.23h-47-Backup",
		"NIXROOT/nix@2026-08-13.23h-47-Backup",
	})

	if len(lines) != 2 {
		t.Fatalf("expected a line per snapshot, got %v", lines)
	}
	if !strings.Contains(lines[0], "dry run failed") {
		t.Errorf("expected the failure surfaced, got %q", lines[0])
	}
}

func TestCleanupPlanUniqueBytesIgnoresSkippedSnapshots(t *testing.T) {
	plan := planFixture()

	// 2048 + 1024. The protected snapshot's 999999 bytes must not be promised
	// to the user, because it is never going to be destroyed.
	if got := plan.uniqueBytes(); got != 3072 {
		t.Errorf("uniqueBytes = %d, want 3072", got)
	}
}

func TestCleanupPlanSkippedListsHeldBackCandidates(t *testing.T) {
	skipped := planFixture().skipped()

	if len(skipped) != 1 || skipped[0].Orphan.Name != "NIXROOT/root@blank" {
		t.Fatalf("expected only the protected snapshot held back, got %+v", skipped)
	}
	if skipped[0].SkipReason != "protected snapshot" {
		t.Errorf("skip reason = %q, want %q", skipped[0].SkipReason, "protected snapshot")
	}
}

func TestRenderCleanupPlanShowsWhatIsSkippedAndWhy(t *testing.T) {
	out := renderCleanupPlan(planFixture())

	for _, want := range []string{
		"NIXROOT/root",
		"NIXROOT/nix",
		"skipping NIXROOT/root@blank",
		"protected snapshot",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan does not mention %q:\n%s", want, out)
		}
	}
}

func TestBuildCleanupPlanViewExplainsTheReclaimCaveat(t *testing.T) {
	view := buildCleanupPlanView(planFixture(), []string{"would destroy NIXROOT/root@..."})

	if !strings.Contains(view, "What would be destroyed") {
		t.Errorf("view should lead with the plan:\n%s", view)
	}
	if !strings.Contains(view, "reclaimed") {
		t.Errorf("view must explain why freed space looks small at first:\n%s", view)
	}
	if !strings.Contains(view, "never touched") {
		t.Errorf("view must state that in-scope datasets are safe:\n%s", view)
	}
}

func TestBuildCleanupPlanViewSaysNothingToDoWhenEverythingIsHeldBack(t *testing.T) {
	plan := planFixture()
	for i := range plan.Decisions {
		plan.Decisions[i].Safe = false
		plan.Decisions[i].SkipReason = "snapshot has a hold"
	}
	plan.Targets = safeToDestroy(plan.Decisions)

	view := buildCleanupPlanView(plan, nil)

	if !strings.Contains(view, "held back by a safety check") {
		t.Errorf("expected the all-skipped message, got:\n%s", view)
	}
}

func TestBuildCleanupPlanViewHandlesAHealthyPool(t *testing.T) {
	plan := &cleanupPlan{Scan: &orphanScan{Pool: "NIXROOT", InScope: []string{"home"}}}

	view := buildCleanupPlanView(plan, nil)

	if !strings.Contains(view, "No orphaned snapshots found") {
		t.Errorf("expected the healthy-pool message, got:\n%s", view)
	}
}

func TestBuildCleanupResultViewReportsFailures(t *testing.T) {
	outcome := cleanupOutcome{
		Destroyed: []string{"NIXROOT/nix@2026-08-13.23h-47-Backup"},
		Failures:  []string{"NIXROOT/root@2026-08-13.23h-47-Backup: dataset is busy"},
	}
	usage := []datasetUsage{{Name: "NIXROOT/root", Used: 1024, UsedBySnapshots: 512}}

	view := buildCleanupResultView("NIXROOT", outcome, usage)

	for _, want := range []string{
		"Cleanup complete on NIXROOT",
		"NIXROOT/nix@2026-08-13.23h-47-Backup",
		"dataset is busy",
		"Space after cleanup",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("result view does not mention %q:\n%s", want, view)
		}
	}
}

// contains reports whether a slice holds an exact string.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// keyRunes builds a key message for a literal string of characters.
func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// cleanupModel builds a model parked on a ready cleanup screen.
func cleanupModel() model {
	return model{
		state:        stateCleanup,
		cleanupPool:  "NIXROOT",
		cleanupPlan:  planFixture(),
		cleanupReady: true,
		cleanupPhase: cleanupPhasePlan,
		input:        textinput.New(),
	}
}

// The whole point of the screen is that no single keystroke destroys anything.
func TestCleanupScreenRequiresTheTypedConfirmation(t *testing.T) {
	m := cleanupModel()

	m, _ = m.updateCleanupScreen(keyRunes("d"))
	if m.cleanupPhase != cleanupPhaseConfirm {
		t.Fatalf("d should open the confirmation, got phase %v", m.cleanupPhase)
	}

	// A plausible-but-wrong answer must not proceed.
	m.input.SetValue("destroy")
	m, cmd := m.updateCleanupScreen(tea.KeyMsg{Type: tea.KeyEnter})
	if m.cleanupPhase != cleanupPhaseConfirm {
		t.Errorf("lowercase confirmation must not proceed, got phase %v", m.cleanupPhase)
	}
	if cmd != nil {
		t.Error("a rejected confirmation must not issue a destroy command")
	}
	if m.cleanupMessage == "" {
		t.Error("a rejected confirmation must say why")
	}
	if m.input.Value() != "" {
		t.Error("the rejected answer should be cleared, not left to be edited into place")
	}

	m.input.SetValue(destroyConfirmationWord)
	m, cmd = m.updateCleanupScreen(tea.KeyMsg{Type: tea.KeyEnter})
	if m.cleanupPhase != cleanupPhaseRunning {
		t.Errorf("the exact word should proceed, got phase %v", m.cleanupPhase)
	}
	if cmd == nil {
		t.Error("proceeding should issue the destroy command")
	}
}

func TestCleanupScreenEscapeAbortsWithoutDestroying(t *testing.T) {
	m := cleanupModel()
	m, _ = m.updateCleanupScreen(keyRunes("d"))
	m.input.SetValue(destroyConfirmationWord)

	m, cmd := m.updateCleanupScreen(tea.KeyMsg{Type: tea.KeyEsc})

	if m.cleanupPhase != cleanupPhasePlan {
		t.Errorf("esc should return to the plan, got phase %v", m.cleanupPhase)
	}
	if cmd != nil {
		t.Error("esc must not issue a destroy command")
	}
	if !strings.Contains(m.cleanupMessage, "Nothing was destroyed") {
		t.Errorf("esc should say nothing was destroyed, got %q", m.cleanupMessage)
	}
	if m.input.Value() != "" {
		t.Error("a typed DESTROY must not survive an abort")
	}
}

func TestCleanupScreenOffersNoDestroyWhenThereIsNothingToDo(t *testing.T) {
	m := cleanupModel()
	m.cleanupPlan.Targets = nil

	m, cmd := m.updateCleanupScreen(keyRunes("d"))

	if m.cleanupPhase != cleanupPhasePlan {
		t.Errorf("d must do nothing when no snapshot is cleared, got phase %v", m.cleanupPhase)
	}
	if cmd != nil {
		t.Error("d must not issue a command when there is nothing to destroy")
	}
	if !strings.Contains(m.cleanupHotkeys(), "esc return") ||
		strings.Contains(m.cleanupHotkeys(), "d destroy") {
		t.Errorf("the destroy key must not be advertised, got %q", m.cleanupHotkeys())
	}
}

func TestCleanupScreenIgnoresKeysWhileDestroying(t *testing.T) {
	m := cleanupModel()
	m.cleanupPhase = cleanupPhaseRunning

	for _, key := range []tea.KeyMsg{keyRunes("d"), keyRunes("q"), keyRunes("r"),
		{Type: tea.KeyEsc}, {Type: tea.KeyEnter}} {
		next, cmd := m.updateCleanupScreen(key)
		if next.cleanupPhase != cleanupPhaseRunning || next.state != stateCleanup || cmd != nil {
			t.Errorf("key %v should be ignored mid-destroy, got phase %v state %v",
				key, next.cleanupPhase, next.state)
		}
	}
}

func TestCleanupScreenCtrlCAlwaysQuits(t *testing.T) {
	for _, phase := range []cleanupPhase{cleanupPhasePlan, cleanupPhaseConfirm, cleanupPhaseDone} {
		m := cleanupModel()
		m.cleanupPhase = phase
		next, _ := m.updateCleanupScreen(tea.KeyMsg{Type: tea.KeyCtrlC})
		if !next.quitting {
			t.Errorf("ctrl+c should quit from phase %v", phase)
		}
	}
}

func TestHealthScreenCarriesThePoolIntoCleanup(t *testing.T) {
	m := model{state: stateDoctor, doctorPool: "NIXROOT", doctorProblems: 3}

	m, cmd := m.updateDoctorScreen(keyRunes("c"))

	if m.state != stateCleanup {
		t.Fatalf("c should open the cleanup screen, got state %v", m.state)
	}
	if m.cleanupPool != "NIXROOT" {
		t.Errorf("the pool should carry over, got %q", m.cleanupPool)
	}
	if m.cleanupPhase != cleanupPhasePlan || m.cleanupReady {
		t.Error("the cleanup screen should start on a freshly loading dry run")
	}
	if cmd == nil {
		t.Error("c should start loading the plan")
	}
}

func TestCleanupPlanViewWarnsWhenNoScopeIsConfigured(t *testing.T) {
	plan := planFixture()
	plan.Scan.ScopeConfigured = false

	view := buildCleanupPlanView(plan, nil)

	if !strings.Contains(view, "no backup scope is set") {
		t.Errorf("the cleanup screen must explain why it may find little:\n%s", view)
	}
}

func TestCleanupPlanViewWarnsEvenWhenItFoundNothing(t *testing.T) {
	plan := &cleanupPlan{Scan: &orphanScan{Pool: "NIXROOT", InScope: []string{"home", "root"}}}

	view := buildCleanupPlanView(plan, nil)

	if !strings.Contains(view, "No orphaned snapshots found") {
		t.Fatalf("expected the nothing-found message:\n%s", view)
	}
	if !strings.Contains(view, "no backup scope is set") {
		t.Errorf("finding nothing is exactly when the reason matters:\n%s", view)
	}
}

func TestCleanupPlanViewIsQuietOnceAScopeIsChosen(t *testing.T) {
	plan := planFixture()
	plan.Scan.ScopeConfigured = true

	if view := buildCleanupPlanView(plan, nil); strings.Contains(view, "no backup scope is set") {
		t.Errorf("a configured pool must not be nagged:\n%s", view)
	}
}

// Blocker from review: while the DESTROY confirmation is open, the list of
// what dies must remain scrollable - arrow keys navigate, they do not type.
func TestCleanupConfirmKeepsThePlanScrollable(t *testing.T) {
	m := cleanupModel()
	m, _ = m.updateCleanupScreen(keyRunes("d"))

	m, _ = m.updateCleanupScreen(tea.KeyMsg{Type: tea.KeyDown})

	if m.input.Value() != "" {
		t.Errorf("arrow keys must scroll, not type; input now %q", m.input.Value())
	}
	if m.cleanupPhase != cleanupPhaseConfirm {
		t.Error("scrolling must not leave the confirmation")
	}
}

// The uninterruptible phase includes ctrl+c - the comment promised it.
func TestCleanupCtrlCIsIgnoredWhileDestroying(t *testing.T) {
	m := cleanupModel()
	m.cleanupPhase = cleanupPhaseRunning

	next, cmd := m.updateCleanupScreen(tea.KeyMsg{Type: tea.KeyCtrlC})

	if next.quitting || cmd != nil {
		t.Error("ctrl+c must not interrupt a destroy in progress")
	}
}

// The confirm phase must fit the terminal AND show the destruction list -
// not the scope advisory - while DESTROY is typed.
func TestCleanupConfirmFitsAndShowsWhatDies(t *testing.T) {
	m := cleanupModel()
	m.width, m.height, m.state = 80, 24, stateCleanup
	m, _ = m.updateCleanupScreen(keyRunes("d"))

	view := m.View()
	lines := strings.Split(view, "\n")

	if len(lines) > 25 {
		t.Errorf("confirm phase emits %d lines on a 24-row terminal", len(lines))
	}
	if !strings.Contains(view, "NIXROOT/root@2026-08-13.23h-47-Backup") {
		t.Errorf("the first target must be visible at the moment of commitment:\n%s", view)
	}
	if !strings.Contains(view, "DESTROYING SNAPSHOTS IS IRREVERSIBLE") {
		t.Error("the banner must be visible")
	}
}

// Backing out of the confirmation restores the full dry-run body.
func TestCleanupEscFromConfirmRestoresThePlan(t *testing.T) {
	m := cleanupModel()
	m.width, m.height = 80, 30
	m.cleanupPlanBody = "FULL PLAN BODY SENTINEL"
	m, _ = m.updateCleanupScreen(keyRunes("d"))
	m, _ = m.updateCleanupScreen(tea.KeyMsg{Type: tea.KeyEsc})

	if !strings.Contains(m.cleanupViewport.View(), "FULL PLAN BODY SENTINEL") {
		t.Error("esc should restore the dry-run body in the viewport")
	}
}

// With more targets than the viewport seats, the confirm phase must say more
// exists - hundreds of orphans is the normal post-1.x case.
func TestCleanupConfirmAdvertisesScrollWhenTheListIsLong(t *testing.T) {
	m := cleanupModel()
	m.width, m.height = 80, 24
	for i := 0; i < 40; i++ {
		m.cleanupPlan.Targets = append(m.cleanupPlan.Targets,
			fmt.Sprintf("NIXROOT/root@2026-01-%02d.00h-00-Backup", i+1))
	}
	m, _ = m.updateCleanupScreen(keyRunes("d"))

	out := m.renderCleanupContent(80)
	if !strings.Contains(out, "▼ more below") {
		t.Errorf("a cut list must say more exists:\n%s", out)
	}
	if !strings.Contains(m.cleanupHotkeys(), "scroll") {
		t.Errorf("the confirm hotkeys must mention scrolling: %q", m.cleanupHotkeys())
	}
}
