// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mustBuild parses MDL for a validator test, failing on a parse error.
func mustBuild(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	return prog
}

func rulesOf(vs []linter.Violation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.RuleID)
	}
	return out
}

// TestFindWithoutKeyIsReported pins CE0250: an object element that searches must
// declare what it searches on. Measured on mxbuild 11.13.0 — the same mapping
// with a key is 0 errors.
func TestFindWithoutKeyIsReported(t *testing.T) {
	prog := mustBuild(t, `create import mapping M.IMM_FindPet with json structure M.JSON_Pet
{ find M.Pet or error { PetId = id, Name = name } };`)

	got := ValidateImportMappingFind(prog, "")
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %v", len(got), rulesOf(got))
	}
	if got[0].RuleID != "MDL-MAP02" {
		t.Errorf("RuleID = %q, want MDL-MAP02", got[0].RuleID)
	}
	if !strings.Contains(got[0].Message, "CE0250") {
		t.Errorf("message should name the build error it prevents, got %q", got[0].Message)
	}
	// The message must quote the handling the author wrote, backup included —
	// `find X or error` stores ObjectHandling "Find" plus a backup, so printing
	// the handling alone would show something they did not write.
	if !strings.Contains(got[0].Message, "find M.Pet or error") {
		t.Errorf("message should quote the statement, got %q", got[0].Message)
	}
	if !strings.Contains(got[0].Suggestion, "PetId") {
		t.Errorf("suggestion should name a member of THIS mapping, got %q", got[0].Suggestion)
	}
}

// TestFindWithKeyIsAccepted is the control for the one above.
func TestFindWithKeyIsAccepted(t *testing.T) {
	prog := mustBuild(t, `create import mapping M.IMM_FindPet with json structure M.JSON_Pet
{ find M.Pet or error { PetId = id key, Name = name } };`)

	if got := ValidateImportMappingFind(prog, ""); len(got) != 0 {
		t.Fatalf("got %v, want none", rulesOf(got))
	}
}

// TestLegacyFindOrCreateWithoutKeyIsReported covers the other spelling. `find or
// create X` and `find X or create` reach the same stored handling by different
// grammar routes, and a rule keyed on only one of them would miss half the
// corpus.
func TestLegacyFindOrCreateWithoutKeyIsReported(t *testing.T) {
	prog := mustBuild(t, `create import mapping M.IMM_UpsertPet with json structure M.JSON_Pet
{ find or create M.Pet { PetId = id, Name = name } };`)

	got := ValidateImportMappingFind(prog, "")
	if len(got) != 1 || got[0].RuleID != "MDL-MAP02" {
		t.Fatalf("got %v, want one MDL-MAP02", rulesOf(got))
	}
}

// TestCreateHandlingNeedsNoKey pins that the rule is about SEARCHING. A `create`
// element has nothing to look up.
func TestCreateHandlingNeedsNoKey(t *testing.T) {
	prog := mustBuild(t, `create import mapping M.IMM_NewPet with json structure M.JSON_Pet
{ create M.Pet { PetId = id, Name = name } };`)

	if got := ValidateImportMappingFind(prog, ""); len(got) != 0 {
		t.Fatalf("got %v, want none", rulesOf(got))
	}
}

// TestCustomHandlerIsExempt pins that `find X by MF(...)` is not a search.
//
// The executor stores ObjectHandling "Custom" for any element carrying a
// handler regardless of the keyword written, so neither requirement applies —
// and 70 of the demo corpus's `find` elements are this shape, so getting it
// wrong would flag most real mappings.
func TestCustomHandlerIsExempt(t *testing.T) {
	prog := mustBuild(t, `create import mapping M.IMM_Self with json structure M.JSON_Pet
{ find M.Pet by M.GetSelf(Pet: parameter) or error { PetId = id, Name = name } };`)

	if got := ValidateImportMappingFind(prog, ""); len(got) != 0 {
		t.Fatalf("got %v, want none — a custom handler IS the find", rulesOf(got))
	}
}

// TestNestedFindWithoutKeyIsReported pins that CE0250 applies below the root.
// Measured: mxbuild reports it "at Object mapping element 'Pet'", so the
// diagnostic names the element rather than only the mapping.
func TestNestedFindWithoutKeyIsReported(t *testing.T) {
	prog := mustBuild(t, `create import mapping M.IMM_Nest with json structure M.JSON_Nest
{ create M.Box { Tag = tag,
    find M.Box_Pet/M.Pet or error = pet { PetId = id, Name = name }
  } };`)

	got := ValidateImportMappingFind(prog, "")
	if len(got) != 1 || got[0].RuleID != "MDL-MAP02" {
		t.Fatalf("got %v, want one MDL-MAP02", rulesOf(got))
	}
	if !strings.Contains(got[0].Message, "element pet") {
		t.Errorf("message should name the element, got %q", got[0].Message)
	}
}

// TestFindOnScriptNonPersistentEntityIsReported pins CE0251 resolved from the
// script alone — the entity is created two statements up, so no project is
// needed and a plain `mxcli check` catches it.
func TestFindOnScriptNonPersistentEntityIsReported(t *testing.T) {
	prog := mustBuild(t, `create non-persistent entity M.Pet ( PetId : integer );
/
create import mapping M.IMM_FindPet with json structure M.JSON_Pet
{ find M.Pet or error { PetId = id key } };`)

	got := ValidateImportMappingFind(prog, "")
	if len(got) != 1 || got[0].RuleID != "MDL-MAP03" {
		t.Fatalf("got %v, want one MDL-MAP03", rulesOf(got))
	}
	if !strings.Contains(got[0].Message, "CE0251") {
		t.Errorf("message should name the build error, got %q", got[0].Message)
	}
}

// TestFindOnScriptPersistentEntityIsAccepted is the control.
func TestFindOnScriptPersistentEntityIsAccepted(t *testing.T) {
	prog := mustBuild(t, `create entity M.Pet ( PetId : integer );
/
create import mapping M.IMM_FindPet with json structure M.JSON_Pet
{ find M.Pet or error { PetId = id key } };`)

	if got := ValidateImportMappingFind(prog, ""); len(got) != 0 {
		t.Fatalf("got %v, want none", rulesOf(got))
	}
}

// TestPersistabilityFollowsTheGeneralizationChain pins the measurement that
// makes this rule non-obvious.
//
// `create entity M.Sub extends M.NpBase` stores Persistable=true — mxcli's own
// catalog reads it as PERSISTENT — and mxbuild still rejects a search on it with
// CE0251. Mendix takes persistability from the parent, so a rule reading the
// entity's own flag would miss exactly this case. The sibling, extending a
// persistable parent, must stay silent.
func TestPersistabilityFollowsTheGeneralizationChain(t *testing.T) {
	prog := mustBuild(t, `create non-persistent entity M.NpBase ( A : string );
/
create entity M.Base ( A : string );
/
create entity M.SubOfNp extends M.NpBase ( B : string );
/
create entity M.SubOfP extends M.Base ( B : string );
/
create import mapping M.IMM_Bad with json structure M.JSON_Pet
{ find M.SubOfNp or error { B = id key } };
create import mapping M.IMM_Good with json structure M.JSON_Pet
{ find M.SubOfP or error { B = id key } };`)

	got := ValidateImportMappingFind(prog, "")
	if len(got) != 1 || got[0].RuleID != "MDL-MAP03" {
		t.Fatalf("got %v, want exactly one MDL-MAP03", rulesOf(got))
	}
	if !strings.Contains(got[0].Message, "M.SubOfNp") {
		t.Errorf("wrong entity flagged: %q", got[0].Message)
	}
}

// TestUnresolvableEntityIsNotFlagged pins the fail-open direction.
//
// A checker that is wrong in the confident direction teaches people to ignore
// it. Without a project and without the entity in the script, persistability is
// simply unknown — and it must NOT be guessed, or the rule fires on every
// mapping over a module the script did not create.
func TestUnresolvableEntityIsNotFlagged(t *testing.T) {
	prog := mustBuild(t, `create import mapping M.IMM_Ghost with json structure M.JSON_Pet
{ find NoSuchModule.Ghost or error { PetId = id key } };`)

	if got := ValidateImportMappingFind(prog, ""); len(got) != 0 {
		t.Fatalf("got %v, want none — persistability is unknown, not false", rulesOf(got))
	}
}

// TestGeneralizationCycleTerminates pins that a cyclic model reports rather than
// hangs. Studio Pro rejects a cycle; an mxcli-written model is not obliged to be
// well-formed, and a checker that loops on bad input is worse than one that says
// nothing.
func TestGeneralizationCycleTerminates(t *testing.T) {
	prog := mustBuild(t, `create entity M.A extends M.B ( X : string );
/
create entity M.B extends M.A ( Y : string );
/
create import mapping M.IMM_Cycle with json structure M.JSON_Pet
{ find M.A or error { X = id key } };`)

	if got := ValidateImportMappingFind(prog, ""); len(got) != 0 {
		t.Fatalf("got %v, want none — an unresolvable chain is unknown", rulesOf(got))
	}
}

// TestValidateProgramRunsTheRule pins the wiring: `mxcli check` and `mxcli exec`
// both go through ValidateProgram, so a rule that is not appended there is a
// rule nothing calls.
func TestValidateProgramRunsTheRule(t *testing.T) {
	prog := mustBuild(t, `create import mapping M.IMM_FindPet with json structure M.JSON_Pet
{ find M.Pet or error { PetId = id, Name = name } };`)

	var found bool
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == "MDL-MAP02" {
			found = true
		}
	}
	if !found {
		t.Error("ValidateProgram does not run ValidateImportMappingFind")
	}
}
