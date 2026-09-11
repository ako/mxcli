// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// The constraints below are the real ones, from a reproduction on a blank
// Mendix 11.14.0 app. `Ticket_Reporter` is an association from
// MyFirstModule.Ticket to System.User; `Title` is one of Ticket's attributes.
//
//	[Ticket_Reporter = $currentUser]                 check passed  →  build exit 3
//	[MyFirstModule.Ticket_Reporter = $currentUser]   check passed  →  BUILD SUCCEEDED

var (
	probeAttrs  = map[string]bool{"Title": true, "Status": true}
	probeAssocs = map[string]string{
		"Ticket_Reporter":  "MyFirstModule.Ticket_Reporter",
		"Ticket_Equipment": "MyFirstModule.Ticket_Equipment",
	}
)

func TestUnqualifiedAssociationsInConstraint(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		want       []string // the bare names expected, in order
	}{
		{
			name:       "the reproduced failure",
			constraint: "[Ticket_Reporter = $currentUser]",
			want:       []string{"Ticket_Reporter"},
		},
		{
			name:       "the qualified form is the fix, and must not fire",
			constraint: "[MyFirstModule.Ticket_Reporter = $currentUser]",
			want:       nil,
		},
		{
			name:       "bare association at the head of a traversal",
			constraint: "[Ticket_Reporter/System.User/Name = 'ada']",
			want:       []string{"Ticket_Reporter"},
		},
		{
			name:       "a qualified traversal is clean",
			constraint: "[MyFirstModule.Ticket_Reporter/System.User/Name = 'ada']",
			want:       nil,
		},
		{
			name:       "two bare associations are both reported",
			constraint: "[Ticket_Reporter = $currentUser and Ticket_Equipment != empty]",
			want:       []string{"Ticket_Reporter", "Ticket_Equipment"},
		},
		{
			name:       "the same one twice is reported once",
			constraint: "[Ticket_Reporter = $a or Ticket_Reporter = $b]",
			want:       []string{"Ticket_Reporter"},
		},
		{
			name:       "an attribute of this entity is never an association",
			constraint: "[Title = 'x' and Status = 'open']",
			want:       nil,
		},
		{
			name:       "XPath keywords and functions cannot match an association name",
			constraint: "[not(contains(Title, 'x')) and Status != empty]",
			want:       nil,
		},
		{
			name:       "a Mendix token is left alone",
			constraint: "[MyFirstModule.Ticket_Reporter = '[%CurrentUser%]']",
			want:       nil,
		},
		{
			name:       "System members are another rule's business",
			constraint: "[System.owner = '[%CurrentUser%]']",
			want:       nil,
		},
		{
			name:       "empty constraint",
			constraint: "",
			want:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hits := unqualifiedAssociationsInConstraint(tc.constraint, probeAttrs, probeAssocs)
			if len(hits) != len(tc.want) {
				t.Fatalf("got %d hits %v, want %d %v", len(hits), assocHitNames(hits), len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if hits[i].Name != w {
					t.Errorf("hit %d = %q, want %q", i, hits[i].Name, w)
				}
				if !strings.HasSuffix(hits[i].Qualified, "."+w) {
					t.Errorf("hit %d qualified = %q, must end in .%s", i, hits[i].Qualified, w)
				}
			}
		})
	}
}

// A name inside a string literal is not a member reference, and offering to
// qualify it would corrupt the literal. Mendix escapes a quote by doubling it,
// so the scanner has to track that or it loses its place and mis-reads the rest
// of the constraint — which is how a name OUTSIDE a literal gets missed.
func TestUnqualifiedAssociationsIgnoresStringLiterals(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		want       []string
	}{
		{
			name:       "a name inside a literal is not a reference",
			constraint: "[Title = 'Ticket_Reporter']",
			want:       nil,
		},
		{
			name:       "a doubled quote is an escape, not the end of the literal",
			constraint: "[Title = 'it''s Ticket_Reporter' and Ticket_Equipment != empty]",
			want:       []string{"Ticket_Equipment"},
		},
		{
			name:       "a real reference after a literal is still found",
			constraint: "[Title = 'x' and Ticket_Reporter = $currentUser]",
			want:       []string{"Ticket_Reporter"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hits := unqualifiedAssociationsInConstraint(tc.constraint, probeAttrs, probeAssocs)
			if len(hits) != len(tc.want) {
				t.Fatalf("got %v, want %v", assocHitNames(hits), tc.want)
			}
			for i, w := range tc.want {
				if hits[i].Name != w {
					t.Errorf("hit %d = %q, want %q", i, hits[i].Name, w)
				}
			}
		})
	}
}

// blankXPathLiterals must preserve length, or every match index after the first
// literal points at the wrong character and the rule reports the wrong name.
func TestBlankXPathLiteralsPreservesLength(t *testing.T) {
	for _, in := range []string{
		"[Title = 'abc']",
		"[Title = 'it''s here' and X = 'y']",
		"[Title = 'unterminated",
		"",
	} {
		if got := blankXPathLiterals(in); len(got) != len(in) {
			t.Errorf("blankXPathLiterals(%q) length %d, want %d", in, len(got), len(in))
		}
	}
}

// An ambiguous name is left out of the index on purpose, so the rule stays
// silent rather than offering one of two spellings and being wrong half the
// time. This asserts the silence, since a lookup miss is indistinguishable from
// a correct constraint at the call site.
func TestUnqualifiedAssociationsSilentWhenNameIsNotAKnownAssociation(t *testing.T) {
	hits := unqualifiedAssociationsInConstraint(
		"[Some_Ambiguous_Assoc = $currentUser]", probeAttrs, probeAssocs)
	if len(hits) != 0 {
		t.Errorf("must not flag a name that is not a known association, got %v", assocHitNames(hits))
	}
}

func assocHitNames(hits []xpathAssocHit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Name
	}
	return out
}

// scriptContext has TWO parallel collectors over the same statement types —
// collectDefinitions (whole program, up front) and collectSingle (incremental).
// MDL-XPATH01 first shipped with its cases added to collectSingle only, so it
// fired against stored associations and stayed silent on script-declared ones,
// which is the shape the rule exists for. Neither the unit tests nor a
// project-backed run caught that: the rule was correct, it was simply never
// given the data.
//
// This asserts the two agree. It fails if a case is added to one and not the
// other, which is the only way the gap can reappear.
func TestBothCollectorsRecordAssociationsAndAttrs(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateEntityStmt{
			Name:       ast.QualifiedName{Module: "M", Name: "Ticket"},
			Attributes: []ast.Attribute{{Name: "Title"}, {Name: "Status"}},
		},
		&ast.CreateAssociationStmt{
			Name:   ast.QualifiedName{Module: "M", Name: "Ticket_Equipment"},
			Parent: ast.QualifiedName{Module: "M", Name: "Ticket"},
			Child:  ast.QualifiedName{Module: "M", Name: "Equipment"},
		},
	}}

	whole := newScriptContext()
	whole.collectDefinitions(prog)

	incremental := newScriptContext()
	for _, stmt := range prog.Statements {
		incremental.collectSingle(stmt)
	}

	for _, sc := range []struct {
		label string
		ctx   *scriptContext
	}{{"collectDefinitions", whole}, {"collectSingle", incremental}} {
		if got := sc.ctx.associations["Ticket_Equipment"]; got != "M.Ticket_Equipment" {
			t.Errorf("%s: associations[Ticket_Equipment] = %q, want M.Ticket_Equipment", sc.label, got)
		}
		attrs := sc.ctx.entityAttrs["M.Ticket"]
		if !attrs["Title"] || !attrs["Status"] {
			t.Errorf("%s: entityAttrs[M.Ticket] = %v, want Title and Status", sc.label, attrs)
		}
	}
}

// The same association name in two modules is dropped rather than guessed at,
// on both collectors.
func TestAmbiguousAssociationNameIsDropped(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateAssociationStmt{Name: ast.QualifiedName{Module: "A", Name: "Shared_Link"}},
		&ast.CreateAssociationStmt{Name: ast.QualifiedName{Module: "B", Name: "Shared_Link"}},
	}}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	if !sc.ambiguousAssc["Shared_Link"] {
		t.Fatal("a name declared in two modules must be marked ambiguous")
	}
	// And the rule must then stay silent on it — offering one of two spellings
	// would be wrong half the time.
	assocs := map[string]string{}
	for name, q := range sc.associations {
		if !sc.ambiguousAssc[name] {
			assocs[name] = q
		}
	}
	if hits := unqualifiedAssociationsInConstraint("[Shared_Link = $x]", nil, assocs); len(hits) != 0 {
		t.Errorf("must stay silent on an ambiguous name, got %v", assocHitNames(hits))
	}
}
