// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL087 — `@disabled` on a statement that cannot carry it.
//
// Refused rather than ignored for the reason the annotation exists: the author
// is asking for a step to be inert, and an ignored request ships a LIVE step
// they believe is off. Same argument as MDL059 (#884).
func TestMDL087RefusesDisabledOnANonActivity(t *testing.T) {
	disabled := &ast.ActivityAnnotations{Disabled: true}

	for _, tc := range []struct {
		stmt ast.MicroflowStatement
		want string // a fragment of the expected message
	}{
		{&ast.IfStmt{Annotations: disabled}, "an IF"},
		{&ast.EnumSplitStmt{Annotations: disabled}, "a CASE"},
		{&ast.InheritanceSplitStmt{Annotations: disabled}, "a SPLIT TYPE"},
		{&ast.LoopStmt{Annotations: disabled}, "a LOOP"},
		{&ast.WhileStmt{Annotations: disabled}, "a WHILE"},
		{&ast.MergeStmt{Annotations: disabled}, "a MERGE"},
		{&ast.JoinStmt{Annotations: disabled}, "a JOIN"},
		{&ast.ReturnStmt{Annotations: disabled}, "a RETURN"},
		{&ast.RaiseErrorStmt{Annotations: disabled}, "a RAISE ERROR"},
		{&ast.BreakStmt{Annotations: disabled}, "a BREAK"},
		{&ast.ContinueStmt{Annotations: disabled}, "a CONTINUE"},
	} {
		v := &microflowValidator{mfName: "M.Flow"}
		v.checkDisabledAnnotationTarget(tc.stmt)
		if len(v.violations) != 1 {
			t.Errorf("%T with @disabled: %d violations, want 1", tc.stmt, len(v.violations))
			continue
		}
		got := v.violations[0]
		if got.RuleID != "MDL087" || got.Severity != linter.SeverityError {
			t.Errorf("%T: rule = %s/%v, want MDL087/error", tc.stmt, got.RuleID, got.Severity)
		}
		// The message has to name the statement the author wrote, or it cannot
		// be acted on in a body with several of them.
		if !strings.Contains(got.Message, tc.want) {
			t.Errorf("%T: message %q does not name %q", tc.stmt, got.Message, tc.want)
		}
	}
}

// Two controls. Without them the check above passes against a version that
// reports every statement, or one that reports none.
func TestMDL087LeavesLegitimateUsesAlone(t *testing.T) {
	// An action activity carrying the flag — the whole point of the annotation.
	v := &microflowValidator{mfName: "M.Flow"}
	v.checkDisabledAnnotationTarget(&ast.LogStmt{
		Annotations: &ast.ActivityAnnotations{Disabled: true},
	})
	if len(v.violations) != 0 {
		t.Errorf("@disabled on a LOG was refused: %+v", v.violations)
	}

	// A listed statement WITHOUT the flag: the refusal is about the annotation,
	// not about the statement.
	v = &microflowValidator{mfName: "M.Flow"}
	v.checkDisabledAnnotationTarget(&ast.IfStmt{
		Annotations: &ast.ActivityAnnotations{Caption: "Right format?"},
	})
	if len(v.violations) != 0 {
		t.Errorf("a plain IF was refused: %+v", v.violations)
	}
}
