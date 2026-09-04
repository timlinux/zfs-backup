// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// =============================================================================
// Pool recovery
// =============================================================================
//
// When ZFS suspends I/O to a pool - almost always because its devices went
// away underneath it - every later operation fails until the pool is brought
// back. The remedies are a short, well-known escalation, so zfs-backup walks
// them rather than printing commands for the user to retype.

// poolState is the condition of a pool as far as recovery is concerned.
type poolState string

const (
	poolOnline       poolState = "ONLINE"
	poolDegraded     poolState = "DEGRADED"
	poolSuspended    poolState = "SUSPENDED"
	poolUnavailable  poolState = "UNAVAIL"
	poolNotImported  poolState = "NOT IMPORTED"
	poolStateUnknown poolState = "UNKNOWN"
)

// poolCommandTimeout bounds every recovery command.
//
// This is not belt and braces: commands issued against a suspended pool can
// block in the kernel indefinitely. Without a deadline the UI would hang with
// no way back, which is precisely the experience recovery exists to avoid.
const poolCommandTimeout = 45 * time.Second

// poolHealth is what a status check found.
type poolHealth struct {
	Pool  string
	State poolState
	Raw   string
}

// Usable reports whether a backup could proceed against this pool. A degraded
// pool still serves I/O - it has lost redundancy, not the data.
func (h poolHealth) Usable() bool {
	return h.State == poolOnline || h.State == poolDegraded
}

// Summary is a one-line description of the pool's condition.
func (h poolHealth) Summary() string {
	switch h.State {
	case poolOnline:
		return fmt.Sprintf("%s is online and healthy.", h.Pool)
	case poolDegraded:
		return fmt.Sprintf("%s is DEGRADED - it works, but has lost redundancy. Check the disks.", h.Pool)
	case poolSuspended:
		return fmt.Sprintf("%s has SUSPENDED I/O - ZFS lost its devices and stopped talking to it.", h.Pool)
	case poolUnavailable:
		return fmt.Sprintf("%s is UNAVAILABLE - its devices cannot be opened.", h.Pool)
	case poolNotImported:
		return fmt.Sprintf("%s is not imported on this system.", h.Pool)
	default:
		return fmt.Sprintf("%s is in an unrecognised state.", h.Pool)
	}
}

// parsePoolHealth classifies the output of `zpool status POOL`. Kept pure so
// every state can be tested without a pool in that state.
func parsePoolHealth(pool, output string, cmdErr error) poolHealth {
	health := poolHealth{Pool: pool, State: poolStateUnknown, Raw: strings.TrimSpace(output)}

	lower := strings.ToLower(output)

	// A suspended pool is reported either as a state line or as a refusal to
	// open it, depending on which command noticed first.
	if suspendedPoolPattern.MatchString(output) {
		health.State = poolSuspended
		return health
	}

	if strings.Contains(lower, "no such pool") || strings.Contains(lower, "cannot open") {
		health.State = poolNotImported
		return health
	}

	if cmdErr != nil && strings.TrimSpace(output) == "" {
		health.State = poolNotImported
		return health
	}

	switch {
	case strings.Contains(lower, "state: online"):
		health.State = poolOnline
	case strings.Contains(lower, "state: degraded"):
		health.State = poolDegraded
	case strings.Contains(lower, "state: unavail"), strings.Contains(lower, "state: faulted"):
		health.State = poolUnavailable
	}

	return health
}

// checkPoolHealth asks ZFS about a pool, under a deadline.
func checkPoolHealth(ctx context.Context, r commandRunner, pool string) poolHealth {
	ctx, cancel := context.WithTimeout(ctx, poolCommandTimeout)
	defer cancel()

	output, err := r.Output(ctx, "zpool", "status", pool)
	if ctx.Err() != nil {
		return poolHealth{
			Pool:  pool,
			State: poolSuspended,
			Raw:   "zpool status did not return - the pool is wedged in the kernel.",
		}
	}
	if err != nil {
		// The error text carries what ZFS said; classify on that.
		return parsePoolHealth(pool, err.Error(), err)
	}
	return parsePoolHealth(pool, output, nil)
}

// =============================================================================
// The escalation ladder
// =============================================================================

// poolRemedy is one step of recovery: what it does, what it costs, and how.
type poolRemedy struct {
	Title       string
	Explanation string
	// NeedsConfirm marks a remedy forceful enough to ask first. None of these
	// destroy data, but a forced export interrupts in-flight I/O.
	NeedsConfirm bool
	// Commands are run in order and stop at the first failure.
	Commands [][]string
}

// poolRemedies is the ordered escalation for a pool that has stopped
// responding: gentlest first, and nothing here destroys data.
func poolRemedies(pool string) []poolRemedy {
	return []poolRemedy{
		{
			Title: "Ask ZFS to retry the failed I/O",
			Explanation: "zpool clear tells ZFS the devices are back and to resume. If the\n" +
				"drive has been reconnected this is usually all that is needed.",
			Commands: [][]string{{"zpool", "clear", pool}},
		},
		{
			Title: "Force the pool out and back in",
			Explanation: "Exports the pool and re-imports it, which rebuilds ZFS's view of the\n" +
				"devices. The forced export interrupts any I/O still queued against\n" +
				"the pool. No data is destroyed - a pool carries its own transaction\n" +
				"history and comes back consistent.",
			NeedsConfirm: true,
			Commands: [][]string{
				{"zpool", "export", "-f", pool},
				{"zpool", "import", pool},
			},
		},
	}
}

// remedyResult records what one remedy did.
type remedyResult struct {
	Title    string
	Log      []string
	TimedOut bool
	Err      error
	Health   poolHealth
}

// applyRemedy runs a remedy's commands under a deadline and re-checks the
// pool afterwards, so the caller always learns whether it actually helped.
func applyRemedy(ctx context.Context, r commandRunner, pool string, remedy poolRemedy) remedyResult {
	result := remedyResult{Title: remedy.Title}

	for _, command := range remedy.Commands {
		cmdCtx, cancel := context.WithTimeout(ctx, poolCommandTimeout)
		err := r.Run(cmdCtx, command[0], command[1:]...)
		timedOut := cmdCtx.Err() != nil
		cancel()

		line := strings.Join(command, " ")
		switch {
		case timedOut:
			result.Log = append(result.Log, fmt.Sprintf("  %s - timed out after %s", line, poolCommandTimeout))
			result.TimedOut = true
			result.Health = checkPoolHealth(ctx, r, pool)
			return result
		case err != nil:
			result.Log = append(result.Log, fmt.Sprintf("  %s - failed: %v", line, err))
			result.Err = err
			result.Health = checkPoolHealth(ctx, r, pool)
			return result
		default:
			result.Log = append(result.Log, fmt.Sprintf("  %s - ok", line))
		}
	}

	result.Health = checkPoolHealth(ctx, r, pool)
	return result
}

// exhaustedGuidance is what is left when every remedy has been tried.
const exhaustedGuidance = "Everything zfs-backup can safely try has been tried and the pool is still\n" +
	"not responding. That points at the hardware rather than at ZFS:\n\n" +
	"  - Check the drive is powered and its cable is firmly seated.\n" +
	"  - Try a different cable, port, or USB enclosure - failing enclosures\n" +
	"    are a far more common cause of this than failing disks.\n" +
	"  - A reboot clears a suspension the kernel will not let go of.\n\n" +
	"The data is not lost by a suspension. Once the devices come back, the\n" +
	"pool imports normally."
