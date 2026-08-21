// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"
	"time"
)

// pedCheckDocument must not believe the first verdict it gets. ped_check_errors
// reads Studio Pro's background error list, which lags the write it should
// reflect (measured live, 26 samples: 75-128ms in both directions). Asked too
// early it answers "No errors found." for a document that was just broken, and
// ped_update_document is no backstop — it reports op-level failures only.
func TestPedCheckDocument_ReasksWhileVerdictIsClean(t *testing.T) {
	checks := 0
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name != "ped_check_errors" {
			return "SUCCESS", false
		}
		checks++
		if checks == 1 {
			return "No errors found.", false // the stale, pre-write verdict
		}
		return "'M.WF': - Duplicate name 'ReviewOrder'.", false
	})
	b := &Backend{client: f.connectClient(t), settleWindow: 500 * time.Millisecond}

	err := b.pedCheckDocument("Workflows$Workflow", "M.WF")
	if err == nil {
		t.Fatal("a stale clean verdict was accepted; the lagged error was never seen")
	}
	if checks < 2 {
		t.Errorf("ped_check_errors called %d time(s); a clean verdict must be re-asked", checks)
	}
}

// The lag cuts both ways: the error list also still reports an error a
// just-applied op has already cleared. Asking before the delay has elapsed fails
// a perfectly good statement on a leftover verdict.
func TestPedCheckDocument_WaitsBeforeTheFirstAsk(t *testing.T) {
	start := time.Now()
	var firstAsk time.Duration
	checks := 0
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name != "ped_check_errors" {
			return "SUCCESS", false
		}
		checks++
		if checks == 1 {
			firstAsk = time.Since(start)
		}
		return "'M.WF': - Duplicate name 'ReviewOrder'.", false
	})
	const delay = 200 * time.Millisecond
	// No window: past the delay a dirty verdict is current and short-circuits.
	b := &Backend{client: f.connectClient(t), settleDelay: delay}

	if err := b.pedCheckDocument("Workflows$Workflow", "M.WF"); err == nil {
		t.Fatal("the error must be surfaced")
	}
	if firstAsk < delay {
		t.Errorf("first ped_check_errors at %v, before the %v settle delay: a leftover verdict would be believed", firstAsk, delay)
	}
	if checks != 1 {
		t.Errorf("ped_check_errors called %d times; past the delay an error must short-circuit", checks)
	}
}

// Control: a zero-value Backend asks once and returns, so the unit tests that do
// not care about pacing stay fast.
func TestPedCheckDocument_NoSettleConfiguredIsOneImmediateAsk(t *testing.T) {
	checks := 0
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_check_errors" {
			checks++
			return "No errors found.", false
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}

	start := time.Now()
	if err := b.pedCheckDocument("Workflows$Workflow", "M.WF"); err != nil {
		t.Fatalf("pedCheckDocument: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("took %v with no settle configured; it must ask once and return", elapsed)
	}
	if checks != 1 {
		t.Errorf("ped_check_errors called %d times, want 1", checks)
	}
}

// New() is what production uses, so that is where the pacing has to be wired —
// a Backend built any other way silently races.
func TestNewBackend_CarriesTheSettlePacing(t *testing.T) {
	b := New("http://localhost/mcp", "")
	if b.settleDelay != settleDelay || b.settleWindow != settleWindow {
		t.Errorf("New() settle = %v/%v, want %v/%v", b.settleDelay, b.settleWindow, settleDelay, settleWindow)
	}
}
