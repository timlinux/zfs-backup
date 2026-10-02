// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A stalled resume can sit for hours while sanoid rotates the exact source
// snapshot the destination's resume token points at. zfs then refuses to
// resume ("cannot resume send: '...' ... no longer exists") and nothing about
// that failure changes on a bare retry - it must be recognised so the caller
// knows to clear the token instead.
func TestIsStaleResumeTokenError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "stale token - snapshot rotated away",
			err:  errors.New("syncoid failed: exit status 2\nOutput: INFO: Resuming interrupted zfs send/receive from NIXROOT/home to NIXBACKUPS/abyss/home (~ UNKNOWN remaining): cannot resume send: 'NIXROOT/home@autosnap_2026-09-01_00:00:00_hourly' used in the initial send no longer exists"),
			want: true,
		},
		{
			name: "unrelated syncoid failure",
			err:  errors.New("syncoid failed: exit status 1\nOutput: CRITICAL ERROR: Target does not exist"),
			want: false,
		},
		{
			name: "resume failure without the no-longer-exists detail",
			err:  errors.New("cannot resume send: some other reason"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isStaleResumeTokenError(c.err); got != c.want {
				t.Errorf("isStaleResumeTokenError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// writeFakeSyncoid puts an executable "syncoid" on PATH for the duration of
// the test that fails with a stale-resume-token error on its first call and
// succeeds on every call after, recording how many times it ran via a marker
// file. This exercises runSyncoidWithResumeRecovery's actual retry path
// end-to-end rather than mocking runSyncoidWithTimeout away.
func writeFakeSyncoid(t *testing.T, failFirst bool) (callCountFile string) {
	t.Helper()
	dir := t.TempDir()
	callCountFile = filepath.Join(dir, "calls")

	script := `#!/usr/bin/env bash
n=0
if [ -f "` + callCountFile + `" ]; then
  n=$(cat "` + callCountFile + `")
fi
n=$((n + 1))
echo "$n" > "` + callCountFile + `"
`
	if failFirst {
		script += `if [ "$n" -eq 1 ]; then
  echo "INFO: Resuming interrupted zfs send/receive from a to b (~ UNKNOWN remaining): cannot resume send: 'a@snap' used in the initial send no longer exists"
  exit 2
fi
`
	}
	script += "exit 0\n"

	path := filepath.Join(dir, "syncoid")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake syncoid: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return callCountFile
}

func readCallCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &n)
	return n
}

func TestRunSyncoidWithResumeRecoveryRetriesAfterClearingStaleToken(t *testing.T) {
	callCountFile := writeFakeSyncoid(t, true)

	var aborted bool
	var output strings.Builder
	err := runSyncoidWithResumeRecovery(context.Background(), time.Minute, []string{"src", "dest"},
		func(context.Context) error {
			aborted = true
			return nil
		},
		&output,
	)

	if err != nil {
		t.Fatalf("expected the retry to succeed, got: %v", err)
	}
	if !aborted {
		t.Error("expected the stale resume token to be aborted before retrying")
	}
	if n := readCallCount(t, callCountFile); n != 2 {
		t.Errorf("expected syncoid to run twice (fail then retry), ran %d times", n)
	}
	if !strings.Contains(output.String(), "clearing it and retrying") {
		t.Errorf("expected the recovery to be narrated in the run log, got: %q", output.String())
	}
}

func TestRunSyncoidWithResumeRecoveryDoesNotRetryUnrelatedFailures(t *testing.T) {
	dir := t.TempDir()
	script := "#!/usr/bin/env bash\necho 'CRITICAL ERROR: Target does not exist' >&2\nexit 1\n"
	path := filepath.Join(dir, "syncoid")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake syncoid: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var aborted bool
	var output strings.Builder
	err := runSyncoidWithResumeRecovery(context.Background(), time.Minute, []string{"src", "dest"},
		func(context.Context) error {
			aborted = true
			return nil
		},
		&output,
	)

	if err == nil {
		t.Fatal("expected the unrelated failure to be returned, not swallowed")
	}
	if aborted {
		t.Error("must not abort a resume token for a failure that has nothing to do with a stale token")
	}
}

func TestRunSyncoidWithResumeRecoveryReportsAbortFailure(t *testing.T) {
	writeFakeSyncoid(t, true)

	abortErr := errors.New("zfs receive -A: dataset is busy")
	var output strings.Builder
	err := runSyncoidWithResumeRecovery(context.Background(), time.Minute, []string{"src", "dest"},
		func(context.Context) error {
			return abortErr
		},
		&output,
	)

	if err == nil {
		t.Fatal("expected an error when clearing the stale token itself fails")
	}
	if !strings.Contains(err.Error(), "dataset is busy") {
		t.Errorf("expected the abort failure to be surfaced, got: %v", err)
	}
}

func TestParseZfsSendSize(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want int64
	}{
		{
			name: "typical dry-run output",
			out:  "incremental\ttank/home@a\ttank/home@b\nsize\t123456789\n",
			want: 123456789,
		},
		{
			name: "no size line",
			out:  "full\ttank/home@a\n",
			want: 0,
		},
		{
			name: "empty output",
			out:  "",
			want: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseZfsSendSize(c.out); got != c.want {
				t.Errorf("parseZfsSendSize(%q) = %d, want %d", c.out, got, c.want)
			}
		})
	}
}

func TestDatasetUsedBytes(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, error) {
		return "1048576\n", nil
	}}
	n, ok := datasetUsedBytes(context.Background(), r, "tank/home")
	if !ok || n != 1048576 {
		t.Errorf("datasetUsedBytes() = (%d, %v), want (1048576, true)", n, ok)
	}

	failing := &fakeRunner{respond: func(name string, args []string) (string, error) {
		return "", errors.New("dataset does not exist")
	}}
	if _, ok := datasetUsedBytes(context.Background(), failing, "tank/gone"); ok {
		t.Error("expected ok=false when the dataset lookup fails")
	}
}

func TestEstimateSendBytes(t *testing.T) {
	t.Run("nothing missing returns zero", func(t *testing.T) {
		dots := []SnapshotDot{{Tag: "a", Status: SnapDone}}
		r := &fakeRunner{}
		if got := estimateSendBytes(context.Background(), r, "tank/home", dots, 999); got != 0 {
			t.Errorf("expected 0 when there is nothing left to send, got %d", got)
		}
	})

	t.Run("incremental dry run uses -I between base and newest missing", func(t *testing.T) {
		dots := []SnapshotDot{
			{Tag: "a", Status: SnapDone},
			{Tag: "b", Status: SnapPending},
			{Tag: "c", Status: SnapPending},
		}
		r := &fakeRunner{respond: func(name string, args []string) (string, error) {
			return "size\t555\n", nil
		}}
		got := estimateSendBytes(context.Background(), r, "tank/home", dots, 999)
		if got != 555 {
			t.Errorf("expected the dry-run size 555, got %d", got)
		}
		if !r.ran("-I", "tank/home@a", "tank/home@c") {
			t.Errorf("expected an -I dry run from the base to the newest missing snapshot, got calls: %v", r.commandLines())
		}
	})

	t.Run("first-ever send with no base uses a plain dry run", func(t *testing.T) {
		dots := []SnapshotDot{{Tag: "a", Status: SnapPending}}
		r := &fakeRunner{respond: func(name string, args []string) (string, error) {
			return "size\t42\n", nil
		}}
		got := estimateSendBytes(context.Background(), r, "tank/home", dots, 999)
		if got != 42 {
			t.Errorf("expected 42, got %d", got)
		}
		if r.ran("-I") {
			t.Errorf("a first-ever send has no base snapshot to diff from, so -I must not be used: %v", r.commandLines())
		}
	})

	t.Run("falls back when the dry run fails", func(t *testing.T) {
		dots := []SnapshotDot{{Tag: "a", Status: SnapPending}}
		r := &fakeRunner{respond: func(name string, args []string) (string, error) {
			return "", errors.New("dataset does not exist")
		}}
		if got := estimateSendBytes(context.Background(), r, "tank/home", dots, 777); got != 777 {
			t.Errorf("expected the fallback 777 when the dry run errors, got %d", got)
		}
	})

	t.Run("falls back when the dry run output has no size", func(t *testing.T) {
		dots := []SnapshotDot{{Tag: "a", Status: SnapPending}}
		r := &fakeRunner{respond: func(name string, args []string) (string, error) {
			return "unparseable\n", nil
		}}
		if got := estimateSendBytes(context.Background(), r, "tank/home", dots, 321); got != 321 {
			t.Errorf("expected the fallback 321, got %d", got)
		}
	})
}

func TestUpdateByteProgress(t *testing.T) {
	ds := &DatasetProgress{EstBytes: 1000}
	start := time.Now().Add(-10 * time.Second)

	updateByteProgress(ds, 500, 800, start)

	if ds.SentBytes != 300 {
		t.Errorf("SentBytes = %d, want 300", ds.SentBytes)
	}
	if ds.Rate <= 0 {
		t.Errorf("expected a positive rate, got %f", ds.Rate)
	}
	if ds.ETA <= 0 {
		t.Errorf("expected a positive ETA with 700 bytes remaining, got %v", ds.ETA)
	}
}

func TestUpdateByteProgressNeverGoesNegative(t *testing.T) {
	ds := &DatasetProgress{}
	start := time.Now().Add(-5 * time.Second)

	// A destination's `used` can legitimately dip (e.g. background pruning)
	// mid-poll; progress shown to the user must not go backwards past zero.
	updateByteProgress(ds, 1000, 900, start)

	if ds.SentBytes != 0 {
		t.Errorf("SentBytes = %d, want 0 when the destination shrank", ds.SentBytes)
	}
}

func TestTrackSyncProgressSuccessForcesFullByteProgress(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, error) {
		if len(args) > 0 && args[0] == "send" {
			return "size\t1000\n", nil
		}
		return "0\n", nil // datasetUsedBytes fallback - irrelevant, the dry run wins
	}}

	ds := &DatasetProgress{Snapshots: []SnapshotDot{{Tag: "a", Status: SnapPending}}}

	destBytes := int64(100) // the value getUsedBytes will report on each call
	reportCount := 0
	err := trackSyncProgress(
		context.Background(),
		r,
		"tank/home",
		ds,
		func() map[string]bool { return map[string]bool{} }, // nothing has arrived yet at estimate time
		func() (int64, bool) { destBytes += 200; return destBytes, true },
		func() { reportCount++ },
		func() error { return nil },
	)

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if ds.EstBytes != 1000 {
		t.Errorf("EstBytes = %d, want 1000 from the dry-run estimate", ds.EstBytes)
	}
	if ds.SentBytes != ds.EstBytes {
		t.Errorf("SentBytes = %d, want it forced up to EstBytes (%d) on success, same as the snapshot dots are forced to Done", ds.SentBytes, ds.EstBytes)
	}
	if ds.ETA != 0 {
		t.Errorf("ETA = %v, want 0 once the sync has finished", ds.ETA)
	}
	if ds.Snapshots[0].Status != SnapDone {
		t.Errorf("snapshot status = %v, want SnapDone", ds.Snapshots[0].Status)
	}
	if reportCount < 2 {
		t.Errorf("expected report() to fire at least at the start and the end, got %d calls", reportCount)
	}
}

func TestTrackSyncProgressFailureMarksFirstMissingSnapshotAsError(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, error) {
		if len(args) > 0 && args[0] == "send" {
			return "", errors.New("dataset does not exist")
		}
		return "0\n", nil
	}}

	ds := &DatasetProgress{Snapshots: []SnapshotDot{
		{Tag: "a", Status: SnapPending},
		{Tag: "b", Status: SnapPending},
	}}

	syncErr := errors.New("syncoid failed: exit status 1")
	err := trackSyncProgress(
		context.Background(),
		r,
		"tank/home",
		ds,
		func() map[string]bool { return map[string]bool{} }, // nothing ever arrives
		func() (int64, bool) { return 0, false },
		func() {},
		func() error { return syncErr },
	)

	if !errors.Is(err, syncErr) {
		t.Fatalf("expected the sync error to propagate, got: %v", err)
	}
	if ds.Snapshots[0].Status != SnapError {
		t.Errorf("first missing snapshot status = %v, want SnapError", ds.Snapshots[0].Status)
	}
	if ds.Snapshots[1].Status != SnapPending {
		t.Errorf("second missing snapshot status = %v, want SnapPending (chain broke at the first)", ds.Snapshots[1].Status)
	}
}

func TestStreamHeadlessProgressThrottlesByteProgressLines(t *testing.T) {
	updates := make(chan progressUpdate, 2)
	updates <- progressUpdate{stageNum: 4, totalStages: 7, stage: "Syncing data", datasets: []DatasetProgress{
		{Name: "home", Status: DatasetSyncing, Size: "120G"},
	}}
	// Same status, with byte progress now available - but this arrives
	// immediately after the first update, well inside the throttle window,
	// so it must not print a second line yet.
	updates <- progressUpdate{stageNum: 4, totalStages: 7, stage: "Syncing data", datasets: []DatasetProgress{
		{Name: "home", Status: DatasetSyncing, Size: "120G", SentBytes: 500 * 1024 * 1024, EstBytes: 1000 * 1024 * 1024, Rate: 10 * 1024 * 1024, ETA: 50 * time.Second},
	}}
	close(updates)

	var out strings.Builder
	streamHeadlessProgress(&out, updates)
	text := out.String()

	if strings.Count(text, "syncing home") != 1 {
		t.Errorf("expected exactly one initial 'syncing' line, got: %q", text)
	}
	if strings.Contains(text, "500.0 MB") {
		t.Errorf("byte progress line printed inside the throttle window: %q", text)
	}
}

func TestDatasetProgressSummary(t *testing.T) {
	cases := []struct {
		name string
		d    DatasetProgress
		want string
	}{
		{
			name: "nothing sent yet",
			d:    DatasetProgress{SentBytes: 0},
			want: "",
		},
		{
			name: "known total, with rate and eta",
			d:    DatasetProgress{SentBytes: 500 * 1024 * 1024, EstBytes: 1000 * 1024 * 1024, Rate: 10 * 1024 * 1024, ETA: 50 * time.Second},
			want: "500.0 MB / 1000.0 MB (50%) - 10.0 MB/s - ETA 50s",
		},
		{
			name: "unknown total falls back to a raw counter",
			d:    DatasetProgress{SentBytes: 42 * 1024 * 1024},
			want: "42.0 MB transferred",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := datasetProgressSummary(c.d); got != c.want {
				t.Errorf("datasetProgressSummary() = %q, want %q", got, c.want)
			}
		})
	}
}
