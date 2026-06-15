// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"io"
)

// Tracer reports MCP tool-call activity for --mcp-verbose / --mcp-trace, so a
// user can see which PED tool calls each MDL command makes. It is shared by the
// executor (which announces each MDL command via Statement) and the MCP client
// (which reports each PED call via Call), so the two interleave on one writer.
//
// A nil *Tracer is a safe no-op: every method guards its receiver, so callers
// need not nil-check before invoking.
type Tracer struct {
	Level int       // 0 off; 1 verbose (PED calls); 2 trace (MDL command + PED calls)
	W     io.Writer // destination (typically os.Stderr, to keep stdout pipeable)
}

// Enabled reports whether any tracing should be emitted.
func (t *Tracer) Enabled() bool {
	return t != nil && t.Level > 0 && t.W != nil
}

// Statement prints the MDL command that is about to run. Level 2 only — at
// level 1 the per-call lines stand alone without command grouping.
func (t *Tracer) Statement(label string) {
	if t == nil || t.Level < 2 || t.W == nil {
		return
	}
	fmt.Fprintf(t.W, "\n▸ %s\n", label)
}

// Call prints one PED tool call: the tool name, an optional target (usually the
// document/module), and — at level 2 only — a detail string (e.g. the update
// operations or schema element types).
func (t *Tracer) Call(tool, target, detail string) {
	if !t.Enabled() {
		return
	}
	line := "  → " + tool
	if target != "" {
		line += " " + target
	}
	if t.Level >= 2 && detail != "" {
		line += ": " + detail
	}
	fmt.Fprintln(t.W, line)
}
