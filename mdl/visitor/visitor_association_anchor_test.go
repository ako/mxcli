// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// upstream #872, third slice: authoring an association's line anchors.
//
// The syntax reuses the EXISTING `@anchor` annotation and its `from:`/`to:`
// parameter names — the same annotation the microflow sequence-flow anchor uses,
// asking the same question (where does the connector attach), differing only in
// the value type. An association endpoint is a continuous point on the entity
// box (a percentage, 0..100), not one of four sides, so it takes a coordinate
// pair. No grammar rule was needed: `annotation*` is generic across every CREATE
// statement, `annotationParamName` already admits FROM and TO, and `(x, y)` is
// the existing `annotationParenValue` with two positional values.
func TestCreateAssociation_AnchorAnnotation(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantFrom *ast.Position
		wantTo   *ast.Position
	}{
		{
			name: "both ends",
			src: `@anchor(from: (0, 54), to: (100, 54))
create association M.Child_Parent from M.Child to M.Parent;`,
			wantFrom: &ast.Position{X: 0, Y: 54},
			wantTo:   &ast.Position{X: 100, Y: 54},
		},
		{
			// Real Studio Pro values from Workflow Commons — neither coordinate
			// is on an edge, which is exactly what a named-anchor vocabulary
			// could not have expressed.
			name: "an off-edge pair",
			src: `@anchor(from: (11, 99), to: (9, 0))
create association M.Child_Parent from M.Child to M.Parent;`,
			wantFrom: &ast.Position{X: 11, Y: 99},
			wantTo:   &ast.Position{X: 9, Y: 0},
		},
		{
			// (0,0) is the box's top-left, a real anchor. It must survive as a
			// value rather than being read as "nothing was said".
			name: "the zero point is authorable",
			src: `@anchor(from: (0, 0), to: (0, 0))
create association M.Child_Parent from M.Child to M.Parent;`,
			wantFrom: &ast.Position{X: 0, Y: 0},
			wantTo:   &ast.Position{X: 0, Y: 0},
		},
		{
			name:     "no annotation leaves both nil, which preserves what is stored",
			src:      `create association M.Child_Parent from M.Child to M.Parent;`,
			wantFrom: nil,
			wantTo:   nil,
		},
		{
			name: "one end only",
			src: `@anchor(from: (25, 100))
create association M.Child_Parent from M.Child to M.Parent;`,
			wantFrom: &ast.Position{X: 25, Y: 100},
			wantTo:   nil,
		},
		{
			// The microflow-flow spelling of @anchor names its inner params, so
			// it must not be misread as a coordinate pair on an association.
			name:     "the microflow side-named form is not a coordinate pair",
			src:      "@anchor(from: right, to: left)\ncreate association M.Child_Parent from M.Child to M.Parent;",
			wantFrom: nil,
			wantTo:   nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			stmt, ok := prog.Statements[0].(*ast.CreateAssociationStmt)
			if !ok {
				t.Fatalf("statement is %T, want *ast.CreateAssociationStmt", prog.Statements[0])
			}
			assertAnchor(t, "from", stmt.FromAnchor, tc.wantFrom)
			assertAnchor(t, "to", stmt.ToAnchor, tc.wantTo)
		})
	}
}

func TestAlterAssociation_SetAnchor(t *testing.T) {
	prog, errs := Build(`alter association M.Child_Parent set anchor from (50, 100) to (50, 0);`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterAssociationStmt)
	if !ok {
		t.Fatalf("statement is %T, want *ast.AlterAssociationStmt", prog.Statements[0])
	}
	if stmt.Operation != ast.AlterAssociationSetAnchor {
		t.Errorf("Operation = %v, want AlterAssociationSetAnchor", stmt.Operation)
	}
	assertAnchor(t, "from", stmt.FromAnchor, &ast.Position{X: 50, Y: 100})
	assertAnchor(t, "to", stmt.ToAnchor, &ast.Position{X: 50, Y: 0})
}

// A non-integer coordinate must be REPORTED, not silently truncated. Mendix
// stores the pair as two integers and its loader refuses to open a project whose
// anchor is anything else (StorageLoadException, before validation runs) — so
// accepting "0.5" and writing 0 would produce a value the author never asked for
// in a file that at least still loads, which is the worse of the two failures.
func TestAssociationAnchor_NonIntegerIsRejected(t *testing.T) {
	for _, src := range []string{
		"@anchor(from: (0.5, 54), to: (100, 54))\ncreate association M.Child_Parent from M.Child to M.Parent;",
		"alter association M.Child_Parent set anchor from (0.5, 54) to (100, 54);",
	} {
		_, errs := Build(src)
		if len(errs) == 0 {
			t.Fatalf("expected an error for a fractional anchor coordinate: %s", src)
		}
		joined := ""
		for _, e := range errs {
			joined += e.Error() + "\n"
		}
		if !strings.Contains(joined, "whole number") {
			t.Errorf("error should explain the integer requirement, got: %s", joined)
		}
	}
}

func assertAnchor(t *testing.T, side string, got, want *ast.Position) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s anchor = %+v, want nil (nothing said ⇒ preserve what is stored)", side, *got)
	case want != nil && got == nil:
		t.Errorf("%s anchor = nil, want %+v", side, *want)
	case want != nil && *got != *want:
		t.Errorf("%s anchor = %+v, want %+v", side, *got, *want)
	}
}
