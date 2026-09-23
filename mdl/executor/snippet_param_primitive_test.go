// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1028. A primitive-typed snippet parameter — the spelling
// `mxcli syntax snippet.create` printed in its own Syntax line —
//
//	CREATE SNIPPET Test.SNIPPET_Label ( Params: { $Label: string } )
//	{ dynamictext dt (content: $Label) };
//
// passed `mxcli check` and then failed at exec with
//
//	Error: failed to build snippet: failed to resolve entity string:
//	       entity not found: string
//
// naming a type nobody spelled. `snippetParameter` was a byte-identical
// duplicate of the `pageParameter` grammar rule with its own, lesser visitor:
// it never called buildDataType, so a primitive type never reached the AST at
// all and the executor took the source text for an entity name. (The same
// duplication had produced the quoted-name bug fixed just before this one —
// see visitor.TestSnippetParameter_QuotedEntityNameIsUnquoted.)
//
// The repair is NOT to write the primitive the way a page parameter writes one.
// Storage would take it — Forms$SnippetParameter's ParameterType is the same
// polymorphic DataTypes$DataType — but mxbuild rejects every primitive snippet
// parameter with CE0046, measured in types.SnippetParameterTypeRule. So the
// statement is refused, by the rule `mxcli check` also applies (MDL087).

// buildSnippetFromMDL parses one CREATE SNIPPET statement and builds it against
// a backend that knows no entities, so an attempt to resolve a primitive as an
// entity surfaces rather than being masked by a lucky name collision.
func buildSnippetFromMDL(t *testing.T, src string) (*pages.Snippet, error) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var stmt *ast.CreateSnippetStmtV3
	for _, s := range prog.Statements {
		if cs, ok := s.(*ast.CreateSnippetStmtV3); ok {
			stmt = cs
			break
		}
	}
	if stmt == nil {
		t.Fatal("no CREATE SNIPPET statement built")
	}

	mod := mkModule("Test")
	h := mkHierarchy(mod)
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return nil, nil },
	}
	return newPageBuilder(mb, h, "Test").buildSnippetV3(stmt)
}

// The reported repro. The refusal must name the CE the author would otherwise
// meet a whole build later, and must not be the old "entity not found" — which
// pointed at a lower-cased word that appears nowhere in the script and offered
// no way to act.
func TestBuildSnippetV3_PrimitiveParameterIsRefusedNotMistakenForAnEntity(t *testing.T) {
	_, err := buildSnippetFromMDL(t, `CREATE SNIPPET Test.SNIPPET_Label (
  Params: { $Label: string }
) {
  DYNAMICTEXT dt (Content: $Label)
}`)
	if err == nil {
		t.Fatal("a primitive snippet parameter was accepted; mxbuild rejects it with CE0046")
	}
	msg := err.Error()
	for _, want := range []string{"$Label", "String", "CE0046", "must be an entity"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "entity not found") {
		t.Errorf("still the #1028 message, which names a type nobody spelled: %s", msg)
	}
}

// Every primitive, in the reporter's own casing and in the documented casing —
// MDL type keywords are case-insensitive and the old visitor passed the source
// text through, which is why the report said "string" and the docs said
// "String". The refusal must not depend on either.
func TestBuildSnippetV3_EveryPrimitiveParameterIsRefused(t *testing.T) {
	for _, spelling := range []string{
		"string", "String", "STRING",
		"Integer", "Long", "Decimal", "Boolean", "DateTime",
	} {
		t.Run(spelling, func(t *testing.T) {
			_, err := buildSnippetFromMDL(t, `CREATE SNIPPET Test.S (
  Params: { $P: `+spelling+` }
) { DYNAMICTEXT dt (Content: 'x') }`)
			if err == nil {
				t.Fatalf("%q accepted as a snippet parameter type", spelling)
			}
			if !strings.Contains(err.Error(), "CE0046") {
				t.Errorf("%q: error = %v, want it to name CE0046", spelling, err)
			}
		})
	}
}

// CONTROL: an entity-typed parameter is still built, still resolved, and still
// fails loudly when the entity does not exist. Without this the refusal above
// is satisfied by "refuse every snippet parameter".
func TestBuildSnippetV3_EntityParameterStillResolves(t *testing.T) {
	_, err := buildSnippetFromMDL(t, `CREATE SNIPPET Test.S (
  Params: { $C: Test.NoSuchEntity }
) { DYNAMICTEXT dt (Content: 'x') }`)
	if err == nil {
		t.Fatal("an unknown entity-typed parameter was accepted; want a resolve failure")
	}
	if !strings.Contains(err.Error(), "Test.NoSuchEntity") {
		t.Errorf("error = %v, want it to name Test.NoSuchEntity", err)
	}
	if strings.Contains(err.Error(), "CE0046") {
		t.Errorf("entity parameter hit the primitive refusal: %v", err)
	}
}

// CONTROL: a PAGE parameter of the same primitive is accepted and typed — the
// restriction is on snippet parameters, not on primitives. Both halves were
// measured in one mxbuild 11.13.0 run: the page below builds at 0 errors while
// the identical clause on a snippet produces one CE0046 per parameter.
func TestBuildPageV3_PrimitiveParameterStillAccepted(t *testing.T) {
	// No widgets and no layout: this asserts what the parameter clause builds,
	// and a layout the mock backend does not have would fail the page for an
	// unrelated reason.
	prog, errs := visitor.Build(`CREATE PAGE Test.P (
  Title: 'P', Params: { $Label: String, $Count: Long }
) { }`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var stmt *ast.CreatePageStmtV3
	for _, s := range prog.Statements {
		if cp, ok := s.(*ast.CreatePageStmtV3); ok {
			stmt = cp
			break
		}
	}
	if stmt == nil {
		t.Fatal("no CREATE PAGE statement built")
	}
	mod := mkModule("Test")
	pb := newPageBuilder(&mock.MockBackend{IsConnectedFunc: func() bool { return true }},
		mkHierarchy(mod), "Test")
	page, err := pb.buildPageV3(stmt)
	if err != nil {
		t.Fatalf("buildPageV3: %v", err)
	}
	want := []string{"DataTypes$StringType", "DataTypes$IntegerType"}
	for i, p := range page.Parameters {
		if p.TypeName != want[i] {
			t.Errorf("page param %s = %q, want %q", p.Name, p.TypeName, want[i])
		}
	}
}
