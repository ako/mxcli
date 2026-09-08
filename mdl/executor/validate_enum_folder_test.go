// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1071: `mxcli check --references` reported every enumeration
// as missing —
//
//	statement 4: attribute 'CriticalPathStation': enumeration not found: Approval.StationKey
//
// while DESCRIBE ENUMERATION returned its 19 values, SHOW ENUMERATIONS listed
// it, and mxbuild built the same declaration at 0 errors. A pure false
// negative: `exec` writes the attribute and the project checks clean.
//
// The discriminator is a FOLDER. enumerationExists compared containers directly
//
//	if enum.ContainerID == module.ID && enum.Name == enumName
//
// and an enumeration inside a folder has the FOLDER as its container, so the
// equality can never hold. Every other command resolves through the container
// hierarchy, which walks folders up to the module.
//
// This is the same defect as upstream #976, which fixed DROP and did not sweep
// for the other callers of this question — see cmd_enumerations_drop_folder_test.go.
// The reference checker was the one left.

// enumRefFixture builds a module holding `RootEnum` at the module root and
// `StationKey` inside a folder, with an entity to hang attributes off.
func enumRefFixture(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("Approval")
	folderID := model.ID("folder-enums")

	root := mkEnumeration(mod.ID, "RootEnum", "A", "B")
	filed := mkEnumeration(folderID, "StationKey", "S1", "S2")

	h := mkHierarchy(mod)
	withContainer(h, root.ContainerID, mod.ID)
	// `filed.ContainerID` IS the folder, so the link to register is the folder's
	// own parent — the same shape as the #976 fixture.
	withContainer(h, folderID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return []*model.Enumeration{root, filed}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

// addAttrProgram is `ALTER ENTITY Approval.ApprovalRun ADD ATTRIBUTE <name>:
// Enumeration(<enumModule>.<enumName>)`.
func addAttrProgram(name, enumModule, enumName string) *ast.Program {
	return &ast.Program{Statements: []ast.Statement{
		&ast.AlterEntityStmt{
			Name:      ast.QualifiedName{Module: "Approval", Name: "ApprovalRun"},
			Operation: ast.AlterEntityAddAttribute,
			Attribute: &ast.Attribute{
				Name: name,
				Type: ast.DataType{
					Kind:    ast.TypeEnumeration,
					EnumRef: &ast.QualifiedName{Module: enumModule, Name: enumName},
				},
			},
		},
	}}
}

func enumErrors(errs []error) string {
	var out []string
	for _, e := range errs {
		out = append(out, e.Error())
	}
	return strings.Join(out, "\n")
}

// The reported statement, verbatim in shape.
func TestReferences_FolderedEnumerationResolves(t *testing.T) {
	errs := validateProgram(enumRefFixture(t), addAttrProgram("CriticalPathStation", "Approval", "StationKey"))

	for _, e := range errs {
		if strings.Contains(e.Error(), "enumeration not found") {
			t.Fatalf("an enumeration in a folder was reported missing: %v\n"+
				"DESCRIBE, SHOW and ALTER all resolve it; this is mendixlabs/mxcli#1071", enumErrors(errs))
		}
	}
}

// The control. This one passed before the fix too, which is what makes the test
// above a container-resolution bug rather than "references are broken".
func TestReferences_RootEnumerationStillResolves(t *testing.T) {
	errs := validateProgram(enumRefFixture(t), addAttrProgram("RootAttr", "Approval", "RootEnum"))

	for _, e := range errs {
		if strings.Contains(e.Error(), "enumeration not found") {
			t.Fatalf("an enumeration at the module root must keep resolving: %v", enumErrors(errs))
		}
	}
}

// Resolving through the hierarchy must not make the check meaningless: an
// enumeration that genuinely is not there is still an error, otherwise the fix
// would be "stop checking" rather than "check correctly".
func TestReferences_MissingEnumerationIsStillReported(t *testing.T) {
	errs := validateProgram(enumRefFixture(t), addAttrProgram("Ghost", "Approval", "NoSuchEnum"))

	if !strings.Contains(enumErrors(errs), "enumeration not found: Approval.NoSuchEnum") {
		t.Errorf("a genuinely missing enumeration was not reported: %v", enumErrors(errs))
	}
}

// A foldered enumeration still belongs to its own module. Naming another
// module's must not find it — the same guard the #976 fix needed.
func TestReferences_FolderedEnumerationDoesNotAnswerForAnotherModule(t *testing.T) {
	errs := validateProgram(enumRefFixture(t), addAttrProgram("Wrong", "SomeOtherModule", "StationKey"))

	if !strings.Contains(enumErrors(errs), "not found") {
		t.Errorf("Approval.StationKey answered for SomeOtherModule.StationKey: %v", enumErrors(errs))
	}
}

// The other call site. The report showed ALTER ENTITY; CREATE ENTITY resolves
// enumerated attributes through the same helper and failed the same way, which
// the report did not mention — measured before fixing.
func TestReferences_FolderedEnumerationResolvesOnCreateEntity(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateEntityStmt{
			Name: ast.QualifiedName{Module: "Approval", Name: "NewThing"},
			Attributes: []ast.Attribute{{
				Name: "Station",
				Type: ast.DataType{
					Kind:    ast.TypeEnumeration,
					EnumRef: &ast.QualifiedName{Module: "Approval", Name: "StationKey"},
				},
			}},
		},
	}}

	for _, e := range validateProgram(enumRefFixture(t), prog) {
		if strings.Contains(e.Error(), "enumeration not found") {
			t.Fatalf("CREATE ENTITY could not resolve a foldered enumeration either: %v", e)
		}
	}
}
