// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#562, reported verbatim:
//
//	Re-running **slice 01 alone** rebuilt the entity from its own statement and
//	dropped the attribute. `exec` reported only `Modified entity:
//	ServiceCore.LithoSystem`; nothing said an attribute had been removed, and
//	`mxcli lint` did not flag it. It surfaced two slices later as a page error:
//
//	  [error] [CE1613] "The selected attribute
//	  'ServiceCore.LithoSystem.OpenRequestCount' no longer exists."
//	   at Text 'dtOpen'
//
// The exec half of the ask shipped in 320a304 and is covered by
// TestCreateOrModifyEntity_WarnsOnDrop. MEASURED on this branch against a real
// 11.6.6 project, re-running the reporter's slice 01: exec now prints
//
//	⚠ create or modify entity MyFirstModule.LithoSystem drops 1 existing
//	  member(s) not listed in this statement: OpenRequestCount
//
// while `mxcli check 01-domain-core.mdl -p app.mpr --references` printed
// "Check passed!" and said nothing. That is what these tests are about: the
// issue's second ask, "a warning at check time would be better still where the
// project is available" — before the attribute is gone rather than as it goes.

// storedLithoSystem is the entity as slice 01 plus slice 03 leave it: two
// attributes from the CREATE, and the calculated one a later ALTER added.
func storedLithoSystem() *domainmodel.Entity {
	return &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "ent-litho", TypeName: "DomainModels$Entity"},
		Name:        "LithoSystem",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{BaseElement: model.BaseElement{ID: "a-name"}, Name: "Name"},
			{BaseElement: model.BaseElement{ID: "a-serial"}, Name: "SerialNumber"},
			{BaseElement: model.BaseElement{ID: "a-open"}, Name: "OpenRequestCount"},
		},
	}
}

// dropCheckCtx wires a context whose project holds exactly the given entities in
// module ServiceCore, reachable through findEntityByQN's module-then-DM path.
func dropCheckCtx(t *testing.T, entities ...*domainmodel.Entity) *ExecContext {
	t.Helper()
	mod := mkModule("ServiceCore")
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    entities,
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "ServiceCore" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

func litho(name string) ast.QualifiedName {
	return ast.QualifiedName{Module: "ServiceCore", Name: name}
}

func dropStrAttr(name string) ast.Attribute {
	return ast.Attribute{Name: name, Type: ast.DataType{Kind: ast.TypeString, Length: 200}}
}

// slice01 is the reporter's 01-domain-core.mdl: the entity's own statement,
// which never mentions OpenRequestCount because the microflow it calculates by
// does not exist until slice 03.
func slice01() *ast.CreateEntityStmt {
	return &ast.CreateEntityStmt{
		Name:           litho("LithoSystem"),
		Kind:           ast.EntityPersistent,
		CreateOrModify: true,
		Attributes:     []ast.Attribute{dropStrAttr("Name"), dropStrAttr("SerialNumber")},
	}
}

func findDropRule(vs []linter.Violation, id string) *linter.Violation {
	for i := range vs {
		if vs[i].RuleID == id {
			return &vs[i]
		}
	}
	return nil
}

// TestMDL087_ReRunningOneSliceAloneIsReported is the issue's case: check the
// reporter's slice 01 against a project that already holds the attribute slice
// 03 added, and the member loss must be named before anything is written.
func TestMDL087_ReRunningOneSliceAloneIsReported(t *testing.T) {
	ctx := dropCheckCtx(t, storedLithoSystem())
	prog := &ast.Program{Statements: []ast.Statement{slice01()}}

	vs := CheckEntityMemberDrops(ctx, prog)
	v := findDropRule(vs, "MDL087")
	if v == nil {
		t.Fatalf("expected MDL087 for the attribute slice 01 does not restate, got %d violation(s): %+v", len(vs), vs)
	}
	if v.Severity != linter.SeverityWarning {
		t.Errorf("MDL087 severity = %v, want warning — 'modify to this shape' is a legitimate intent; the defect was the silence", v.Severity)
	}
	// The reported symptom is the attribute's name arriving too late, in a
	// CE1613 about a page. Both belong in the message.
	for _, want := range []string{"OpenRequestCount", "ServiceCore.LithoSystem", "CE1613"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("MDL087 message does not mention %q: %s", want, v.Message)
		}
	}
	if !strings.Contains(v.Suggestion, "alter entity ServiceCore.LithoSystem add attribute") {
		t.Errorf("MDL087 should point at the incremental spelling, got: %s", v.Suggestion)
	}
	if v.Location.DocumentName != "LithoSystem" || v.Location.Module != "ServiceCore" {
		t.Errorf("MDL087 location = %+v, want ServiceCore/LithoSystem", v.Location)
	}
}

// TestMDL087_FaithfulRestatementIsSilent is the CONTROL for the test above: the
// same pass over a script that restates every member must say nothing. Without
// it, a checker that flagged every `create or modify entity` would pass.
func TestMDL087_FaithfulRestatementIsSilent(t *testing.T) {
	ctx := dropCheckCtx(t, storedLithoSystem())
	full := slice01()
	full.Attributes = append(full.Attributes,
		ast.Attribute{Name: "OpenRequestCount", Type: ast.DataType{Kind: ast.TypeInteger}})

	if vs := CheckEntityMemberDrops(ctx, &ast.Program{Statements: []ast.Statement{full}}); len(vs) != 0 {
		t.Fatalf("a statement restating every member must be silent, got %+v", vs)
	}
}

// TestMDL087_ReAddedLaterInTheSameScriptIsSilent is the whole reason this pass
// is a NET computation over the script rather than a per-statement one. The
// reporter's two slices concatenated — rebuild, then add the attribute back once
// its microflow exists — lose nothing, and warning on that is how a rule gets
// ignored. Note the ADD carries IF NOT EXISTS, which is how the slice is written.
func TestMDL087_ReAddedLaterInTheSameScriptIsSilent(t *testing.T) {
	ctx := dropCheckCtx(t, storedLithoSystem())
	readd := &ast.AlterEntityStmt{
		Name:        litho("LithoSystem"),
		Operation:   ast.AlterEntityAddAttribute,
		IfNotExists: true,
		Attribute: &ast.Attribute{
			Name:                "OpenRequestCount",
			Type:                ast.DataType{Kind: ast.TypeInteger},
			Calculated:          true,
			CalculatedMicroflow: &ast.QualifiedName{Module: "ServiceCore", Name: "CALC_OpenRequestCount"},
		},
	}
	prog := &ast.Program{Statements: []ast.Statement{slice01(), readd}}
	if vs := CheckEntityMemberDrops(ctx, prog); len(vs) != 0 {
		t.Fatalf("an attribute the script adds back is not a loss, got %+v", vs)
	}

	// And the other order is still a loss: adding it and THEN rebuilding
	// without it removes it just the same.
	reversed := &ast.Program{Statements: []ast.Statement{readd, slice01()}}
	if findDropRule(CheckEntityMemberDrops(ctx, reversed), "MDL087") == nil {
		t.Error("a rebuild AFTER the add still drops the attribute — expected MDL087")
	}
}

// TestMDL087_ExplicitRemovalIsSilent covers the three spellings that say what
// they do. A pure before/after diff cannot tell these from an accident, which is
// why intent is tracked rather than inferred from the outcome.
func TestMDL087_ExplicitRemovalIsSilent(t *testing.T) {
	cases := []struct {
		name string
		prog []ast.Statement
	}{
		{"drop attribute", []ast.Statement{
			slice01(),
			&ast.AlterEntityStmt{Name: litho("LithoSystem"), Operation: ast.AlterEntityDropAttribute, AttributeName: "OpenRequestCount"},
		}},
		{"drop attribute before the rebuild", []ast.Statement{
			&ast.AlterEntityStmt{Name: litho("LithoSystem"), Operation: ast.AlterEntityDropAttribute, AttributeName: "OpenRequestCount"},
			slice01(),
		}},
		{"drop entity", []ast.Statement{
			&ast.DropEntityStmt{Name: litho("LithoSystem")},
			slice01(),
		}},
		{"rename attribute", []ast.Statement{
			&ast.AlterEntityStmt{
				Name: litho("LithoSystem"), Operation: ast.AlterEntityRenameAttribute,
				AttributeName: "OpenRequestCount", NewName: "OpenCount",
			},
			func() ast.Statement {
				s := slice01()
				s.Attributes = append(s.Attributes, ast.Attribute{Name: "OpenCount", Type: ast.DataType{Kind: ast.TypeInteger}})
				return s
			}(),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := dropCheckCtx(t, storedLithoSystem())
			if vs := CheckEntityMemberDrops(ctx, &ast.Program{Statements: tc.prog}); len(vs) != 0 {
				t.Errorf("an explicit removal must not warn, got %+v", vs)
			}
		})
	}
}

// TestMDL087_EntityCreatedByThisScriptIsSilent — a rebuild of something the
// project does not hold removes nothing from it.
func TestMDL087_EntityCreatedByThisScriptIsSilent(t *testing.T) {
	ctx := dropCheckCtx(t) // empty project
	prog := &ast.Program{Statements: []ast.Statement{slice01(), slice01()}}
	if vs := CheckEntityMemberDrops(ctx, prog); len(vs) != 0 {
		t.Fatalf("expected silence for an entity this script creates, got %+v", vs)
	}
}

// TestMDL087_NonRebuildingFormsAreSilent — neither CREATE ENTITY IF NOT EXISTS
// (which never touches an existing definition) nor a plain CREATE (refused by
// CheckProjectConflicts) rebuilds, so neither drops.
func TestMDL087_NonRebuildingFormsAreSilent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shape func(*ast.CreateEntityStmt)
	}{
		{"if not exists", func(s *ast.CreateEntityStmt) { s.CreateOrModify = false; s.IfNotExists = true }},
		{"plain create", func(s *ast.CreateEntityStmt) { s.CreateOrModify = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := dropCheckCtx(t, storedLithoSystem())
			s := slice01()
			tc.shape(s)
			if vs := CheckEntityMemberDrops(ctx, &ast.Program{Statements: []ast.Statement{s}}); len(vs) != 0 {
				t.Errorf("expected silence, got %+v", vs)
			}
		})
	}
}

// TestMDL087_AuditFieldsAndGeneralization — the members that are not attributes
// drop the same way, and are reported by the same shared comparison the exec
// warning uses. An audit pseudo-type restated in the statement is NOT a drop,
// which is the control that separates "reported the flag" from "reported every
// entity that has one".
func TestMDL087_AuditFieldsAndGeneralization(t *testing.T) {
	stored := storedLithoSystem()
	stored.HasOwner = true
	stored.HasCreatedDate = true
	stored.GeneralizationRef = "ServiceCore.Asset"

	ctx := dropCheckCtx(t, stored)
	s := slice01()
	s.Attributes = append(s.Attributes,
		ast.Attribute{Name: "OpenRequestCount", Type: ast.DataType{Kind: ast.TypeInteger}},
		// Owner is restated as its pseudo-type — kept, and not counted as an
		// attribute either.
		ast.Attribute{Name: "Owner", Type: ast.DataType{Kind: ast.TypeAutoOwner}},
	)

	v := findDropRule(CheckEntityMemberDrops(ctx, &ast.Program{Statements: []ast.Statement{s}}), "MDL087")
	if v == nil {
		t.Fatal("expected MDL087 for the dropped createdDate and generalization")
	}
	for _, want := range []string{"createdDate (system field)", "extends ServiceCore.Asset (generalization)"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message missing %q: %s", want, v.Message)
		}
	}
	if strings.Contains(v.Message, "owner (system field)") {
		t.Errorf("owner was restated as AutoOwner and must not be reported: %s", v.Message)
	}
	if strings.Contains(v.Message, "Owner,") || strings.Contains(v.Message, ": Owner") {
		t.Errorf("an audit pseudo-type is not an attribute and must not be reported as one: %s", v.Message)
	}
}

// TestMDL087_SharedComparisonWithExecWarning pins the refactor that made the
// exec-time warning and this pass one answer. Two hand-written diffs at two
// layers is how the audit fields came to be covered by one and not the other, so
// the two entry points are asserted to agree on the same pair of entities.
func TestMDL087_SharedComparisonWithExecWarning(t *testing.T) {
	stored := storedLithoSystem()
	stored.HasChangedDate = true
	stored.GeneralizationRef = "ServiceCore.Asset"

	rebuilt := &domainmodel.Entity{
		Name: "LithoSystem",
		Attributes: []*domainmodel.Attribute{
			{Name: "Name"}, {Name: "SerialNumber"},
		},
	}
	execSide := strings.Join(droppedEntityMembers(stored, rebuilt), ", ")

	ctx := dropCheckCtx(t, stored)
	v := findDropRule(CheckEntityMemberDrops(ctx, &ast.Program{Statements: []ast.Statement{slice01()}}), "MDL087")
	if v == nil {
		t.Fatal("expected MDL087")
	}
	if !strings.Contains(v.Message, execSide) {
		t.Errorf("check-time list and exec-time list disagree:\n  exec:  %s\n  check: %s", execSide, v.Message)
	}
}

// TestMDL087_NoProjectIsSilent — the answer is entirely "what does the project
// hold that this script does not mention", so without a project the pass must
// report nothing rather than guess.
func TestMDL087_NoProjectIsSilent(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return false }}))
	if vs := CheckEntityMemberDrops(ctx, &ast.Program{Statements: []ast.Statement{slice01()}}); len(vs) != 0 {
		t.Fatalf("expected silence without a project, got %+v", vs)
	}
}

// TestMDL087_ReportsInStoredAttributeOrder pins the reporting order. It is the
// entity's own attribute order — what `describe entity` prints — not
// alphabetical and not map iteration order, so the list reads against the
// document the user is looking at. This is also what keeps the exec-time
// warning's output unchanged by the refactor onto the shared comparison.
func TestMDL087_ReportsInStoredAttributeOrder(t *testing.T) {
	stored := &domainmodel.Entity{
		Name:        "LithoSystem",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Zeta"}, {Name: "Name"}, {Name: "Alpha"}, {Name: "Mid"},
		},
	}
	// Only Name is restated, so three drop — and Zeta comes first because the
	// entity stores it first.
	keepName := &ast.CreateEntityStmt{
		Name:           litho("LithoSystem"),
		Kind:           ast.EntityPersistent,
		CreateOrModify: true,
		Attributes:     []ast.Attribute{dropStrAttr("Name")},
	}

	ctx := dropCheckCtx(t, stored)
	v := findDropRule(CheckEntityMemberDrops(ctx, &ast.Program{Statements: []ast.Statement{keepName}}), "MDL087")
	if v == nil {
		t.Fatal("expected MDL087")
	}
	if !strings.Contains(v.Message, "Zeta, Alpha, Mid") {
		t.Errorf("expected stored order 'Zeta, Alpha, Mid', got: %s", v.Message)
	}

	// The exec-time helper reports the same list in the same order.
	got := strings.Join(droppedEntityMembers(stored, &domainmodel.Entity{
		Name:       "LithoSystem",
		Attributes: []*domainmodel.Attribute{{Name: "Name"}},
	}), ", ")
	if got != "Zeta, Alpha, Mid" {
		t.Errorf("droppedEntityMembers order = %q, want \"Zeta, Alpha, Mid\"", got)
	}
}
