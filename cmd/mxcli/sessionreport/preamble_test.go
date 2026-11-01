// SPDX-License-Identifier: Apache-2.0

package sessionreport

import (
	"bytes"
	"strings"
	"testing"
)

// The preamble — system prompt, tool definitions, the skill index, CLAUDE.md —
// is re-read in full by EVERY model call and never amortised, so its cost is
// floor × calls rather than floor. Nothing in the report measured it, which
// left two questions open: how much of a session is fixed overhead rather than
// work, and what a phase split really costs (each phase re-pays the floor).
//
// These tests pin the estimator: the first model call's context, per
// conversation, times that conversation's calls.

// modelUsage writes one model call with explicit usage, so a fixture can make
// the first call's context differ from the later ones. f.model fixes the
// input and cache-write numbers, which cannot express a floor.
func (f *fixture) modelUsage(input, cacheRead, cacheWrite int, uses ...map[string]any) {
	f.modelIn("", input, cacheRead, cacheWrite, uses...)
}

// modelIn is modelUsage into a named sidechain conversation ("" is main).
func (f *fixture) modelIn(agent string, input, cacheRead, cacheWrite int, uses ...map[string]any) {
	f.n++
	id := "msg_" + itoa(f.n)
	usage := map[string]any{
		"input_tokens": input, "cache_read_input_tokens": cacheRead,
		"cache_creation_input_tokens": cacheWrite, "output_tokens": 10,
	}
	blocks := append([]map[string]any{{"type": "text", "text": "x"}}, uses...)
	for _, b := range blocks {
		rec := map[string]any{"type": "assistant", "message": map[string]any{
			"id": id, "role": "assistant", "content": []any{b}, "usage": usage,
		}}
		if agent != "" {
			rec["isSidechain"] = true
			rec["agentId"] = agent
		}
		f.add(rec)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// The floor is the first call's context, and every later call pays it again.
func TestPreambleIsTheFirstCallsContextPaidByEveryCall(t *testing.T) {
	f := newFixture()
	f.modelUsage(0, 0, 50_000, bash("t1", "ls")) // fresh start: the preamble
	f.result("t1", "a", false)
	f.modelUsage(0, 50_000, 400, bash("t2", "pwd")) // work accumulates on top
	f.result("t2", "b", false)
	f.modelUsage(0, 60_000, 400)

	r := analyze(t, f)
	if r.ModelCalls != 3 {
		t.Fatalf("model calls = %d, want 3", r.ModelCalls)
	}
	if r.PreambleTokens != 50_000 {
		t.Errorf("preamble floor = %d, want 50000 (the first call's context)", r.PreambleTokens)
	}
	if want := int64(150_000); r.PreamblePaid != want {
		t.Errorf("preamble paid = %d, want %d (floor × 3 calls, never amortised)", r.PreamblePaid, want)
	}
}

// The control that separates this from "total context": the floor is a strict
// part of the bill, so a session that does real work pays more than the floor.
// Without it, a field that simply returned the total would pass the test above
// whenever the context never grew.
func TestPreamblePaidIsAPartOfTheBillNotTheWholeBill(t *testing.T) {
	f := newFixture()
	f.modelUsage(0, 0, 30_000, bash("t1", "ls"))
	f.result("t1", strings.Repeat("x", 40_000), false)
	f.modelUsage(0, 30_000, 10_000, bash("t2", "pwd"))
	f.result("t2", "b", false)
	f.modelUsage(0, 40_000, 500)

	r := analyze(t, f)
	total := r.Tokens.Input + r.Tokens.CacheRead + r.Tokens.CacheWrite
	if r.PreamblePaid >= total {
		t.Errorf("preamble paid %d >= total context %d: the field is reporting the whole bill", r.PreamblePaid, total)
	}
	if r.PreamblePaid != 90_000 {
		t.Errorf("preamble paid = %d, want 90000", r.PreamblePaid)
	}
}

// Each conversation pays its OWN preamble: a subagent's context starts from a
// different prompt and a different tool set, so charging it the main
// conversation's floor would misprice every delegation — and pricing
// delegation is what this field is for.
func TestEachConversationPaysItsOwnPreamble(t *testing.T) {
	f := newFixture()
	f.modelUsage(0, 0, 50_000, bash("t1", "ls"))
	f.result("t1", "a", false)
	f.modelUsage(0, 50_000, 400)
	f.modelIn("agent-a", 0, 0, 10_000, bash("t2", "pwd"))
	f.result("t2", "b", false)
	f.modelIn("agent-a", 0, 10_000, 200)
	f.modelIn("agent-a", 0, 11_000, 200)

	r := analyze(t, f)
	if r.Subagents != 1 {
		t.Fatalf("subagents = %d, want 1", r.Subagents)
	}
	// main 50k × 2 + agent 10k × 3
	if want := int64(130_000); r.PreamblePaid != want {
		t.Errorf("preamble paid = %d, want %d (main 50k×2 + subagent 10k×3)", r.PreamblePaid, want)
	}
	// The reported floor is the main conversation's, not the subagent's and
	// not a blend: it is the number a project can act on.
	if r.PreambleTokens != 50_000 {
		t.Errorf("preamble floor = %d, want the main conversation's 50000", r.PreambleTokens)
	}
}

// A transcript with no model call must not divide by zero or invent a floor.
func TestPreambleIsZeroWithoutModelCalls(t *testing.T) {
	f := newFixture()
	r := analyze(t, f)
	if r.PreambleTokens != 0 || r.PreamblePaid != 0 {
		t.Errorf("empty transcript: floor %d paid %d, want 0 0", r.PreambleTokens, r.PreamblePaid)
	}
}

// The number is useless if it is not shown: the whole point is that a reader
// can see what share of the session was fixed overhead rather than work.
func TestRenderShowsThePreambleFloorAndItsShare(t *testing.T) {
	f := newFixture()
	f.modelUsage(0, 0, 50_000, bash("t1", "ls"))
	f.result("t1", "a", false)
	f.modelUsage(0, 50_000, 400)

	var buf bytes.Buffer
	Render(&buf, analyze(t, f))
	out := buf.String()
	if !strings.Contains(out, "preamble") {
		t.Errorf("rendered report does not mention the preamble:\n%s", out)
	}
	// 100,000 of 100,800 read is 99%: the share has to be rendered, since the
	// absolute number alone does not say whether it is worth acting on.
	if !strings.Contains(out, "%") {
		t.Errorf("rendered report does not carry the preamble share:\n%s", out)
	}
}

// A resumed transcript's first record is already past earlier work, so the
// floor is an overstatement and floor × calls can exceed the context actually
// read. Rendering "122% of context read" with no explanation would discredit a
// field whose whole job is to be believed, so the overstatement is named.
func TestRenderNamesAnOverstatedFloorInsteadOfPrintingOver100Percent(t *testing.T) {
	f := newFixture()
	// Resumed: the first call already carries 90k of prior conversation, which
	// a compaction then throws away, so later calls read far LESS than the
	// supposed floor and floor × calls comes out above the whole bill.
	f.modelUsage(0, 90_000, 0, bash("t1", "ls"))
	f.result("t1", "a", false)
	f.compact()
	f.modelUsage(0, 20_000, 100)
	f.modelUsage(0, 20_100, 100)

	r := analyze(t, f)
	total := r.Tokens.Input + r.Tokens.CacheRead + r.Tokens.CacheWrite
	if r.PreamblePaid <= total {
		t.Fatalf("fixture does not overstate: paid %d, total %d", r.PreamblePaid, total)
	}
	var buf bytes.Buffer
	Render(&buf, r)
	out := buf.String()
	if strings.Contains(out, "100% of context read") || !strings.Contains(out, "resumed") {
		t.Errorf("an overstated floor is not named as one:\n%s", out)
	}
}
