// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"strings"
	"testing"
)

func TestTracerNilIsNoOp(t *testing.T) {
	var tr *Tracer
	// None of these may panic on a nil receiver.
	if tr.Enabled() {
		t.Error("nil tracer should not be enabled")
	}
	tr.Statement("create entity X")
	tr.Call("ped_update_document", "X", "add /entities")
}

func TestTracerLevels(t *testing.T) {
	// Level 1: calls only, no command headers, no detail.
	var b strings.Builder
	tr := &Tracer{Level: 1, W: &b}
	tr.Statement("create entity X")                      // suppressed at level 1
	tr.Call("ped_update_document", "X", "add /entities") // detail suppressed at level 1
	out := b.String()
	if strings.Contains(out, "▸") {
		t.Errorf("level 1 should not print command headers: %q", out)
	}
	if !strings.Contains(out, "→ ped_update_document X") {
		t.Errorf("level 1 should print the call with target: %q", out)
	}
	if strings.Contains(out, "add /entities") {
		t.Errorf("level 1 should not print call detail: %q", out)
	}

	// Level 2: command headers + call detail.
	b.Reset()
	tr = &Tracer{Level: 2, W: &b}
	tr.Statement("create entity X")
	tr.Call("ped_update_document", "X", "add /entities")
	out = b.String()
	if !strings.Contains(out, "▸ create entity X") {
		t.Errorf("level 2 should print the command header: %q", out)
	}
	if !strings.Contains(out, "→ ped_update_document X: add /entities") {
		t.Errorf("level 2 should print the call with detail: %q", out)
	}
}
