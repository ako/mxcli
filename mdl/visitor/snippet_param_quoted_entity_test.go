// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A quoted entity name in a SNIPPET parameter failed at execution:
//
//	create or modify snippet M.S (params: { $T: Pd."Thing" })
//	-> failed to resolve entity Pd."Thing": entity not found
//
// while the identical quoted form in a PAGE parameter resolved fine
// (ako/CapTrackV4 019). The project convention is to quote every identifier, so
// this was reached by following the house style — and the asymmetry gives no
// clue which of the two spellings is the odd one.
//
// The cause was one line. buildSnippetParameterListAsPage re-split the parse
// node's TEXT (`parseQualifiedName(dt.GetText())`), and GetText() returns the
// source verbatim, quotes included. The page path has always walked the parse
// tree instead, where buildQualifiedName unquotes each part.
//
// A correct implementation already existed beside it — buildSnippetParameters,
// which nothing called. Two copies of one conversion, one of them dead, is how
// they drifted; the dead one is gone.

func snippetParamEntity(t *testing.T, src string) ast.QualifiedName {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateSnippetStmtV3)
		if !ok {
			continue
		}
		if len(s.Parameters) != 1 {
			t.Fatalf("got %d parameters, want 1", len(s.Parameters))
		}
		return s.Parameters[0].EntityType
	}
	t.Fatal("no snippet statement built")
	return ast.QualifiedName{}
}

func TestSnippetParameter_QuotedEntityNameIsUnquoted(t *testing.T) {
	got := snippetParamEntity(t, `CREATE OR MODIFY SNIPPET M.S (Params: { $T: Mod."Thing" })
{ CONTAINER r { DYNAMICTEXT t (Content: 'x') } }`)

	if got.Module != "Mod" || got.Name != "Thing" {
		t.Errorf(`snippet param type = %+v, want {Module:Mod Name:Thing} — the quotes `+
			`reached the resolver and exec failed with "entity not found: Mod.\"Thing\""`, got)
	}
}

// CONTROL: the unquoted form must be unchanged, and a quoted MODULE name has to
// work too — the convention quotes both halves.
func TestSnippetParameter_UnquotedAndFullyQuotedAgree(t *testing.T) {
	for _, src := range []string{
		`CREATE OR MODIFY SNIPPET M.S (Params: { $T: Mod.Thing })
{ CONTAINER r { DYNAMICTEXT t (Content: 'x') } }`,
		`CREATE OR MODIFY SNIPPET M.S (Params: { $T: "Mod"."Thing" })
{ CONTAINER r { DYNAMICTEXT t (Content: 'x') } }`,
	} {
		got := snippetParamEntity(t, src)
		if got.Module != "Mod" || got.Name != "Thing" {
			t.Errorf("snippet param type = %+v, want {Module:Mod Name:Thing}", got)
		}
	}
}

// CONTROL: the page parameter this was compared against still resolves the same
// way, so the fix is "make the snippet agree with the page", not a change to
// both.
func TestPageParameter_QuotedEntityNameStillUnquoted(t *testing.T) {
	prog, errs := Build(`CREATE OR REPLACE PAGE M.P (Title: 'P', Params: { $T: Mod."Thing" })
{ CONTAINER r { DYNAMICTEXT t (Content: 'x') } }`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	for _, stmt := range prog.Statements {
		p, ok := stmt.(*ast.CreatePageStmtV3)
		if !ok {
			continue
		}
		if len(p.Parameters) != 1 {
			t.Fatalf("got %d parameters, want 1", len(p.Parameters))
		}
		if got := p.Parameters[0].EntityType; got.Module != "Mod" || got.Name != "Thing" {
			t.Errorf("page param type = %+v, want {Module:Mod Name:Thing}", got)
		}
		return
	}
	t.Fatal("no page statement built")
}
