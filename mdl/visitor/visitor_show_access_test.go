// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	mdlast "github.com/mendixlabs/mxcli/mdl/ast"
)

// #925: `SHOW ACCESS ON ENTITY Mod.E` was a parse error while the MICROFLOW and
// PAGE spellings parsed, even though the CLAUDE.md `mxcli init` writes into every
// project documents all three. The cause is that `ENTITY` is in the grammar's
// `keyword` rule, so `ACCESS ON qualifiedName` matched the word ENTITY as the
// whole name and the real name became extraneous input:
//
//	line 1:27 extraneous input '.' expecting the start of a statement
//
// The two halves of the fix are covered here: the grammar alternative (without
// it these do not parse at all) and the visitor's ACCESS guard (without it the
// long-preceding ENTITY branch answers with the entity's DEFINITION instead of
// its access rules — a wrong answer rather than an error).
func TestShowAccessOnEntity(t *testing.T) {
	for _, src := range []string{
		"SHOW ACCESS ON ENTITY Mod.Customer",
		"LIST ACCESS ON ENTITY Mod.Customer",
		`SHOW ACCESS ON ENTITY "Mod"."Customer"`,
		"show access on entity Mod.Customer",
	} {
		s := parseOneShow(t, src)
		if s.ObjectType != mdlast.ShowAccessOn {
			t.Errorf("%q: ObjectType = %v, want ShowAccessOn (entity access rules)",
				src, s.ObjectType)
		}
		if s.Name == nil || s.Name.String() != "Mod.Customer" {
			t.Errorf("%q: Name = %v, want Mod.Customer", src, s.Name)
		}
	}
}

// The explicit spelling must be the bare one's synonym, not a near-miss: both
// reach listAccessOnEntity, so a divergence here would be two commands with the
// same name and different answers.
func TestShowAccessOnEntityMatchesBareForm(t *testing.T) {
	bare := parseOneShow(t, "SHOW ACCESS ON Mod.Customer")
	explicit := parseOneShow(t, "SHOW ACCESS ON ENTITY Mod.Customer")

	if bare.ObjectType != explicit.ObjectType {
		t.Errorf("bare = %v, explicit = %v — the two spellings must agree",
			bare.ObjectType, explicit.ObjectType)
	}
	if bare.Name.String() != explicit.Name.String() {
		t.Errorf("bare name = %q, explicit name = %q",
			bare.Name.String(), explicit.Name.String())
	}
}

// False-positive control for the ACCESS guard: the plain SHOW ENTITY branch must
// still produce ShowEntity, and the other ACCESS ON <kind> spellings must keep
// their own object types. Without this the guard could "pass" by routing every
// ENTITY statement to the access path.
func TestShowAccessOnKindsStayDistinct(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want mdlast.ShowObjectType
	}{
		{"SHOW ENTITY Mod.Customer", mdlast.ShowEntity},
		{"SHOW ACCESS ON Mod.Customer", mdlast.ShowAccessOn},
		{"SHOW ACCESS ON ENTITY Mod.Customer", mdlast.ShowAccessOn},
		{"SHOW ACCESS ON MICROFLOW Mod.ACT_Do", mdlast.ShowAccessOnMicroflow},
		{"SHOW ACCESS ON PAGE Mod.Home", mdlast.ShowAccessOnPage},
		{"SHOW ACCESS ON WORKFLOW Mod.Approve", mdlast.ShowAccessOnWorkflow},
		{"SHOW ACCESS ON NANOFLOW Mod.NAV_Go", mdlast.ShowAccessOnNanoflow},
	} {
		if got := parseOneShow(t, tc.src).ObjectType; got != tc.want {
			t.Errorf("%q: ObjectType = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func parseOneShow(t *testing.T, src string) *mdlast.ShowStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	if prog == nil {
		t.Fatalf("%q: no program", src)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("%q: got %d statements, want 1", src, len(prog.Statements))
	}
	s, ok := prog.Statements[0].(*mdlast.ShowStmt)
	if !ok {
		t.Fatalf("%q: got %T, want *ast.ShowStmt", src, prog.Statements[0])
	}
	return s
}
