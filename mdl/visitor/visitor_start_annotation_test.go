// SPDX-License-Identifier: Apache-2.0

// Tests for @start(x, y) — the StartEvent's position, written on the first
// statement because the start has no statement of its own. Same shape and same
// reason as @merge, which positions the other implicit node.
//
// The builder-side tests construct ast.ActivityAnnotations directly, so they
// pass whether or not @start survives the grammar and the visitor. This is the
// half that proves an author can actually write it. (upstream #951)
package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestStartAnnotation_ReachesTheAST(t *testing.T) {
	stmt := firstStatement(t, "@start(145, 200)\n@position(260, 200)\nlog info node 'App' 'hi';")

	log, ok := stmt.(*ast.LogStmt)
	if !ok {
		t.Fatalf("expected LogStmt, got %T", stmt)
	}
	if log.Annotations == nil || log.Annotations.Start == nil {
		t.Fatal("@start did not reach the AST — it parses as a bare annotation name, " +
			"so a missing visitor arm loses it in silence")
	}
	if got := *log.Annotations.Start; got.X != 145 || got.Y != 200 {
		t.Errorf("@start parsed as %d;%d, want 145;200", got.X, got.Y)
	}
	// @start must not consume the statement's own @position.
	if log.Annotations.Position == nil {
		t.Fatal("@position was lost")
	}
	if got := *log.Annotations.Position; got.X != 260 || got.Y != 200 {
		t.Errorf("@position parsed as %d;%d, want 260;200", got.X, got.Y)
	}
}

// Negative coordinates: Studio Pro's canvas origin is not a corner, so a start
// dragged up and left of the first activity is ordinary, not malformed.
func TestStartAnnotation_AcceptsNegativeCoordinates(t *testing.T) {
	stmt := firstStatement(t, "@start(-40, -120)\nlog info node 'App' 'hi';")
	log := stmt.(*ast.LogStmt)
	if log.Annotations == nil || log.Annotations.Start == nil {
		t.Fatal("@start did not reach the AST")
	}
	if got := *log.Annotations.Start; got.X != -40 || got.Y != -120 {
		t.Errorf("@start parsed as %d;%d, want -40;-120", got.X, got.Y)
	}
}

// A known annotation name must not land in UnknownNames, or MDL059 rejects the
// very statement DESCRIBE just wrote.
func TestStartAnnotation_IsNotReportedUnknown(t *testing.T) {
	stmt := firstStatement(t, "@start(145, 200)\nlog info node 'App' 'hi';")
	log := stmt.(*ast.LogStmt)
	if n := log.Annotations.UnknownNames; len(n) > 0 {
		t.Errorf("@start recorded as unknown %v — MDL059 would reject it", n)
	}
}
