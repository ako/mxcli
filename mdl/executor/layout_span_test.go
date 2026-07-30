// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#790: a loop box was drawn far wider than its contents.
// measureStatements sums each element's full width and adds HorizontalSpacing
// between them, but HorizontalSpacing is a centre-to-centre pitch — the builder
// centres each activity on posX and advances by exactly that — so the sum
// over-counts a run of n simple activities by (n-1)*ActivityWidth.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// simpleStmts returns n statements that each measure to one plain activity box.
func simpleStmts(n int) []ast.MicroflowStatement {
	out := make([]ast.MicroflowStatement, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &ast.MfCommitStmt{})
	}
	return out
}

func TestMeasureStatementsSpan_SimpleRun(t *testing.T) {
	m := &layoutMeasurer{}
	tests := []struct {
		n    int
		want int
	}{
		{0, 0},
		{1, ActivityWidth},                       // 120
		{2, HorizontalSpacing + ActivityWidth},   // 280
		{3, 2*HorizontalSpacing + ActivityWidth}, // 440 — was 680
	}
	for _, tc := range tests {
		got := m.measureStatementsSpan(simpleStmts(tc.n)).Width
		if got != tc.want {
			t.Errorf("span of %d activities = %d, want %d", tc.n, got, tc.want)
		}
		if tc.n > 1 {
			// The old measure is the one that over-sized the loop box.
			if old := m.measureStatements(simpleStmts(tc.n)).Width; old <= got {
				t.Errorf("expected measureStatements (%d) to exceed the true span (%d)", old, got)
			}
		}
	}
}

// TestMeasureStatementsSpan_CompoundFallsBack: a compound element advances posX by
// geometry this measure cannot reproduce. Guessing there under-sizes the box and
// pushes activities outside it, so such runs must keep the conservative measure.
func TestMeasureStatementsSpan_CompoundFallsBack(t *testing.T) {
	m := &layoutMeasurer{}
	body := []ast.MicroflowStatement{
		&ast.MfCommitStmt{},
		&ast.IfStmt{ThenBody: simpleStmts(1)},
		&ast.MfCommitStmt{},
	}
	span := m.measureStatementsSpan(body).Width
	full := m.measureStatements(body).Width
	if span != full {
		t.Errorf("compound run: span = %d, want the conservative %d", span, full)
	}
}

// TestMeasureStatementsSpan_ZeroWidthIgnored: a RETURN produces no box, so it must
// not contribute a pitch step.
func TestMeasureStatementsSpan_ZeroWidthIgnored(t *testing.T) {
	m := &layoutMeasurer{}
	withReturn := []ast.MicroflowStatement{&ast.MfCommitStmt{}, &ast.ReturnStmt{}, &ast.MfCommitStmt{}}
	if got, want := m.measureStatementsSpan(withReturn).Width, m.measureStatementsSpan(simpleStmts(2)).Width; got != want {
		t.Errorf("span with a zero-width statement = %d, want %d", got, want)
	}
}
