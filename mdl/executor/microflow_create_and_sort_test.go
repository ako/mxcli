// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Two statements `mxcli check` passed, `exec` wrote without complaint, and only
// mxbuild refused — one of them by refusing to load the project at all.
// ako/CapTrackV3 FINDINGS §9 and §46.

// ---------------------------------------------------------------------------
// §9 — an unassigned CREATE
// ---------------------------------------------------------------------------

// `CREATE Mod.Thing ( … ) COMMIT;` with no `$Var =` produced CE6005 "The
// 'Entity' property is required."
//
// The error names the wrong property. `Entity` IS stored — a field-level diff of
// the assigned and unassigned documents shows ONE substantive difference,
// `Action.VariableName` "R" vs "", everything else being identity (element IDs,
// flow pointers, StableId). A Mendix Create activity always names its output, and
// Studio Pro supplies the name itself when you drop one. Patching that single
// field took the project from 1 error to 0, and restoring it took it back to 1.
//
// Supplied rather than refused, because nothing is guessed: the author said
// create this entity and did not ask for a handle, which is exactly what an
// auto-named output is.

func TestFreshCreateVariable_FollowsStudioProConvention(t *testing.T) {
	fb := &flowBuilder{}
	if got := fb.freshCreateVariable("Region"); got != "NewRegion" {
		t.Errorf("freshCreateVariable(Region) = %q, want %q", got, "NewRegion")
	}
}

// Two unassigned creates of the same entity in one flow must not share an output
// variable: Mendix reports that as CE0119, so a bare "New"+entity would trade one
// silent defect for another.
func TestFreshCreateVariable_DoesNotCollideWithItself(t *testing.T) {
	fb := &flowBuilder{generatedVars: map[string]bool{"NewRegion": true}}
	if got := fb.freshCreateVariable("Region"); got != "NewRegion2" {
		t.Errorf("second create got %q, want %q", got, "NewRegion2")
	}
}

// ...nor with a name the SCRIPT already used.
func TestFreshCreateVariable_DoesNotCollideWithAnAuthoredName(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{"NewRegion": "M.Region"}}
	if got := fb.freshCreateVariable("Region"); got == "NewRegion" {
		t.Error("the generated name took a variable the script had already declared")
	}
}

// CONTROL: an assigned create keeps the AUTHOR's name, and only the author's name
// is registered as referenceable — a generated one is deliberately not, so a
// later `change $NewRegion` stays the error it is.
func TestCreateObject_AssignedKeepsTheAuthorsVariable(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{}}
	fb.addCreateObjectAction(&ast.CreateObjectStmt{
		Variable:   "R",
		EntityType: ast.QualifiedName{Module: "M", Name: "Region"},
	})
	if got := fb.varTypes["R"]; got != "M.Region" {
		t.Errorf("varTypes[R] = %q, want %q", got, "M.Region")
	}
}

func TestCreateObject_GeneratedNameIsNotReferenceable(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{}}
	fb.addCreateObjectAction(&ast.CreateObjectStmt{
		EntityType: ast.QualifiedName{Module: "M", Name: "Region"},
	})
	if _, ok := fb.varTypes["NewRegion"]; ok {
		t.Error("the generated output variable was registered as referenceable — " +
			"`change $NewRegion` would then resolve to a name the script never wrote")
	}
	if !fb.generatedVars["NewRegion"] {
		t.Error("the generated name was not reserved, so a second create would collide")
	}
}

// ---------------------------------------------------------------------------
// §46 — a two-part SORT BY path
// ---------------------------------------------------------------------------

// `SORT BY System.createdDate` passed check, exec reported success, and then
// mxbuild could not LOAD the project:
//
//	System.ArgumentNullException: Value cannot be null. (Parameter 'value')
//	  at Mendix.Modeler.DomainModels.Refs.AttributeRef.set_AttributeId
//
// Not a CE code — an unhandled exception with a stack trace, so Studio Pro cannot
// open it either. That is the severest outcome in this codebase's taxonomy, and
// mxcli produced it from a script that every gate accepted.
//
// The cause is one guard. The qualified branch validated `len(parts) >= 3`, so a
// TWO-part path fell through every check and was written verbatim as the sort's
// AttributeQualifiedName. `System.createdDate` reads as Module.Attribute and names
// no entity, so Mendix resolves it to nothing and stores a null AttributeId.
//
// MEASURED on 11.14.0, and narrower than the report suggests:
//
//	sort by <a real attribute>                     0 errors
//	sort by createdDate   (entity stores it)       0 errors  <- works
//	sort by createdDate   (entity does not)        CE1613    <- Mendix is right
//	sort by CreatedDate   (wrong case)             CE1613    <- Mendix is right
//	sort by System.createdDate                     UNLOADABLE
//
// So a system member is spelled BARE and works; only the two-part form is the
// defect. An earlier reading of this — that mxcli could not sort by a system
// member at all — came from testing against an entity that did not store one.

func sortStmt(attr string) *ast.RetrieveStmt {
	return &ast.RetrieveStmt{
		Variable:    "Rows",
		Source:      ast.QualifiedName{Module: "M", Name: "Item"},
		SortColumns: []ast.SortColumnDef{{Attribute: attr, Order: "asc"}},
	}
}

func TestSortBy_TwoPartPathIsRefused(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{}}
	fb.addRetrieveAction(sortStmt("System.createdDate"))

	errs := strings.Join(fb.GetErrors(), "\n")
	if !strings.Contains(errs, "System.createdDate") {
		t.Fatalf("a two-part sort path was accepted; it stores a null attribute "+
			"reference and the project cannot be opened. errors: %v", fb.GetErrors())
	}
	// The message has to carry the fix, since the author cannot tell from
	// mxbuild's stack trace what went wrong.
	if !strings.Contains(errs, "createdDate`") && !strings.Contains(errs, "sort by createdDate") {
		t.Errorf("the error does not offer the bare spelling that works: %s", errs)
	}
}

// CONTROL: a BARE name is untouched. This is how a system member is written, and
// it builds at 0 errors whenever the entity stores it — so refusing it would
// reject the correct spelling.
func TestSortBy_BareNameIsNotRefused(t *testing.T) {
	for _, attr := range []string{"createdDate", "Code", "changedDate"} {
		fb := &flowBuilder{varTypes: map[string]string{}}
		fb.addRetrieveAction(sortStmt(attr))
		if len(fb.GetErrors()) != 0 {
			t.Errorf("bare `sort by %s` was refused: %v", attr, fb.GetErrors())
		}
	}
}

// CONTROL: a fully qualified THREE-part path is untouched. The existing
// belongs-to-the-entity check owns that case, and with no backend it must stay
// permissive rather than start refusing.
func TestSortBy_ThreePartPathIsNotRefused(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{}}
	fb.addRetrieveAction(sortStmt("M.Item.Code"))
	if len(fb.GetErrors()) != 0 {
		t.Errorf("a fully qualified sort path was refused: %v", fb.GetErrors())
	}
}
