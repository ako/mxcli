// SPDX-License-Identifier: Apache-2.0

package mpr

import "testing"

// TestParseQueueSettings covers the legacy engine's half of the queued-call
// round trip (FINDINGS #25).
//
// The legacy writer stored the binding correctly, but the legacy PARSER never
// read it back — so on `--engine legacy` a queued call described as an ordinary
// one, and a describe → exec cycle dropped the binding. The two engines
// disagreed about the same stored document, which is the shape of bug that
// survives longest: each looks self-consistent.
func TestParseQueueSettings(t *testing.T) {
	call := map[string]any{
		"$Type":     "Microflows$MicroflowCall",
		"Microflow": "Q.Target",
		"QueueSettings": map[string]any{
			"$Type": "Queues$QueueSettings",
			"Queue": "Q.MyQueue",
			"Retry": nil,
		},
	}

	qs := parseQueueSettings(call)
	if qs == nil {
		t.Fatal("QueueSettings not read back — a describe→exec round trip drops the binding")
	}
	if qs.Queue != "Q.MyQueue" {
		t.Errorf("Queue = %q, want Q.MyQueue", qs.Queue)
	}
	if qs.Retry != nil {
		t.Errorf("Retry = %v, want nil for an explicit BSON null", qs.Retry)
	}

	// An unqueued call must stay unqueued — the common case by far.
	if got := parseQueueSettings(map[string]any{"QueueSettings": nil}); got != nil {
		t.Errorf("unqueued call produced %+v, want nil", got)
	}
	if got := parseQueueSettings(map[string]any{}); got != nil {
		t.Errorf("absent QueueSettings produced %+v, want nil", got)
	}

	// A stored retry must survive the read, because checkNoQueuedCalls refuses
	// the rewrite on its presence — losing it here would re-enable the reset.
	withRetry := parseQueueSettings(map[string]any{"QueueSettings": map[string]any{
		"Queue": "Q.MyQueue",
		"Retry": map[string]any{"$Type": "Queues$QueueFixedRetry"},
	}})
	if withRetry == nil || withRetry.Retry == nil {
		t.Fatal("a stored Retry must be carried, or the guard that refuses resetting it goes blind")
	}
}
