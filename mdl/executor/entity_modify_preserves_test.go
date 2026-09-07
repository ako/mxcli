// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `create or modify persistent entity` on an existing entity DELETED every
// access rule on it (ako/mxcli-rest FINDINGS). The only output was
// `Modified entity: …`, and `mxcli check` passed.
//
// MEASURED on a real project, both engines, mxbuild 11.14.0. Re-running one
// domain-model script over eight entities:
//
//	access rules   174 -> 132
//	member entries 1352 -> 1071
//	mx check       0 errors -> 288 errors, every one CE2729
//
// Two things about that measurement are worth keeping.
//
// The 288 is the correction to the report, which said `mx check` stays clean.
// It does — on an entity nothing is bound to yet. The loss is silent EXACTLY
// where nothing depends on the access, and reports itself only once pages exist,
// which is the worst possible ordering for someone building an app.
//
// Idempotence does not help. A byte-identical re-run strips the rules just as
// completely, because the rebuilt document genuinely differs from the stored one
// and so is not elided (ADR-0008).
//
// The cause was structural, not a missing case: the modify branch built a fresh
// entity from the AST and swapped it in, so every field MDL has no words for went
// with it. Documentation and indexes had each been added back to a carry list
// after their own defect report; access rules would have been the third, and the
// external-source and OData fields the next. The fix inverts it — start from what
// is stored, overwrite only what the statement declares — which has no such tail.

func storedEntityWithARule() *domainmodel.Entity {
	return &domainmodel.Entity{
		BaseElement:   model.BaseElement{ID: "stored-id", TypeName: "DomainModels$Entity"},
		Name:          "Probe",
		Documentation: "stored docs",
		Persistable:   true,
		Attributes: []*domainmodel.Attribute{
			{BaseElement: model.BaseElement{ID: "attr-name"}, Name: "Name"},
			{BaseElement: model.BaseElement{ID: "attr-code"}, Name: "Code"},
		},
		AccessRules: []*domainmodel.AccessRule{{
			ModuleRoleNames: []string{"Fx.Reader"},
			AllowRead:       true,
			MemberAccesses: []*domainmodel.MemberAccess{
				{AttributeID: "attr-name", AttributeName: "Name", AccessRights: domainmodel.MemberAccessRightsReadWrite},
				{AttributeID: "attr-code", AttributeName: "Code", AccessRights: domainmodel.MemberAccessRightsReadOnly},
			},
		}},
	}
}

func declaredEntity(attrs ...string) *domainmodel.Entity {
	e := &domainmodel.Entity{Name: "Probe", Persistable: true}
	for _, a := range attrs {
		e.Attributes = append(e.Attributes, &domainmodel.Attribute{Name: a})
	}
	return e
}

// The reported bug, at the layer it lives in.
func TestMergeDeclaredOntoStoredEntity_KeepsAccessRules(t *testing.T) {
	stored := storedEntityWithARule()
	merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name", "Code"), &ast.CreateEntityStmt{})

	if len(merged.AccessRules) != 1 {
		t.Fatalf("the rewrite kept %d access rules, want 1 — every grant on the entity "+
			"was deleted, and only mxbuild says so, as CE2729 on whatever consumes it",
			len(merged.AccessRules))
	}
	if got := len(merged.AccessRules[0].MemberAccesses); got != 2 {
		t.Errorf("the surviving rule kept %d of 2 member entries", got)
	}
}

// An attribute the statement omits IS removed — that half of the contract is
// unchanged, and the member entry must go with it or the rule dangles.
func TestMergeDeclaredOntoStoredEntity_PrunesTheDroppedAttributesMember(t *testing.T) {
	stored := storedEntityWithARule()
	merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name"), &ast.CreateEntityStmt{})
	pruneMemberAccessesForDroppedAttributes(merged, stored)

	if len(merged.Attributes) != 1 || merged.Attributes[0].Name != "Name" {
		t.Fatalf("the declared attribute set did not win: %+v", merged.Attributes)
	}
	var names []string
	for _, ma := range merged.AccessRules[0].MemberAccesses {
		names = append(names, ma.AttributeName)
	}
	if len(names) != 1 || names[0] != "Name" {
		t.Errorf("member entries after dropping Code = %v, want [Name]", names)
	}
}

// THE CONTROL that a whole day went into. The obvious predicate for "which
// members to prune" is "not among the rebuilt entity's attributes", and it is
// wrong in the direction that breaks security: an entity's rules also govern its
// INHERITED members, which never appear in its own Attributes list.
//
// CapTrack.ExportDocument extends System.FileDocument and owns no attributes at
// all. That predicate emptied all five of its rules, and mxbuild reported CE0066
// "Entity access is out of date" — a regression the first version of this fix
// shipped, and one only a specialisation exposes. Nothing else in this file
// catches it, because every other entity here owns the members its rules name.
func TestPruneMemberAccesses_KeepsInheritedMembersOfAnEntityThatOwnsNoAttributes(t *testing.T) {
	stored := &domainmodel.Entity{
		Name:              "ExportDocument",
		GeneralizationRef: "System.FileDocument",
		AccessRules: []*domainmodel.AccessRule{{
			ModuleRoleNames: []string{"CapTrack.Developer"},
			MemberAccesses: []*domainmodel.MemberAccess{
				{AttributeName: "System.FileDocument.Name"},
				{AttributeName: "System.FileDocument.Contents"},
				{AttributeName: "System.FileDocument.FileID"},
			},
		}},
	}
	declared := &domainmodel.Entity{Name: "ExportDocument", GeneralizationRef: "System.FileDocument"}

	merged := mergeDeclaredOntoStoredEntity(stored, declared, &ast.CreateEntityStmt{})
	pruneMemberAccessesForDroppedAttributes(merged, stored)

	if got := len(merged.AccessRules[0].MemberAccesses); got != 3 {
		t.Errorf("kept %d of 3 inherited member entries — pruning on absence from the "+
			"entity's own attributes deletes every inherited grant (CE0066)", got)
	}
}

// The prune is by name AND by id because the two engines fill different fields:
// the modelsdk reader populates only AttributeName, the legacy one an
// AttributeID. A prune that consulted one would be a no-op on the other engine.
func TestPruneMemberAccesses_MatchesOnEitherEngineSpelling(t *testing.T) {
	for _, tc := range []struct {
		name string
		ma   *domainmodel.MemberAccess
	}{
		{"legacy: id only", &domainmodel.MemberAccess{AttributeID: "attr-code"}},
		{"modelsdk: bare name only", &domainmodel.MemberAccess{AttributeName: "Code"}},
		{"modelsdk: qualified name", &domainmodel.MemberAccess{AttributeName: "Fx.Probe.Code"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := storedEntityWithARule()
			stored.AccessRules[0].MemberAccesses = []*domainmodel.MemberAccess{tc.ma}
			merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name"), &ast.CreateEntityStmt{})
			pruneMemberAccessesForDroppedAttributes(merged, stored)
			if got := len(merged.AccessRules[0].MemberAccesses); got != 0 {
				t.Errorf("the member for the dropped Code survived (%d entries)", got)
			}
		})
	}
}

// CONTROL for that: a rewrite that drops NOTHING must not touch a single member
// entry. Without this, a prune broken into "delete everything" would still pass
// the two tests above.
func TestPruneMemberAccesses_TouchesNothingWhenNoAttributeWasDropped(t *testing.T) {
	stored := storedEntityWithARule()
	merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name", "Code", "Extra"), &ast.CreateEntityStmt{})
	pruneMemberAccessesForDroppedAttributes(merged, stored)
	if got := len(merged.AccessRules[0].MemberAccesses); got != 2 {
		t.Errorf("an add-only rewrite left %d of 2 member entries", got)
	}
}

// Documentation keeps the behaviour that was already there and already
// regression-tested (mendixlabs/mxcli#1018): silence preserves, `/** */` clears.
func TestMergeDeclaredOntoStoredEntity_DocumentationFollowsItsAstFlag(t *testing.T) {
	stored := storedEntityWithARule()

	merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name"), &ast.CreateEntityStmt{})
	if merged.Documentation != "stored docs" {
		t.Errorf("a rewrite that said nothing about documentation cleared it: %q", merged.Documentation)
	}

	d := declaredEntity("Name")
	d.Documentation = ""
	merged = mergeDeclaredOntoStoredEntity(stored, d, &ast.CreateEntityStmt{DocumentationSet: true})
	if merged.Documentation != "" {
		t.Errorf("an explicit empty `/** */` did not clear documentation: %q", merged.Documentation)
	}
}

// An omitted EXTENDS still un-inherits, because there is no "extends nothing"
// spelling — preserving it would make an inheritance impossible to remove. It is
// REPORTED instead, which is the treatment an omitted attribute already gets and
// the only part of this that was silent before.
func TestDroppedEntityMembers_ReportsARemovedGeneralization(t *testing.T) {
	stored := &domainmodel.Entity{Name: "ExportDocument", GeneralizationRef: "System.FileDocument"}
	dropped := droppedEntityMembers(stored, &domainmodel.Entity{Name: "ExportDocument"})
	if len(dropped) != 1 {
		t.Fatalf("un-inheriting an entity reported %v — it happened in silence", dropped)
	}
	if !strings.Contains(dropped[0], "System.FileDocument") {
		t.Errorf("the warning does not name the parent being removed: %q", dropped[0])
	}

	// CONTROL: restating the same EXTENDS reports nothing, or every rewrite of a
	// specialisation would warn.
	same := droppedEntityMembers(stored, &domainmodel.Entity{
		Name: "ExportDocument", GeneralizationRef: "System.FileDocument",
	})
	if len(same) != 0 {
		t.Errorf("restating the generalization was reported as a drop: %v", same)
	}
}

// ---------------------------------------------------------------------------
// The drift guard
// ---------------------------------------------------------------------------

// The defect was a carry list that fell behind the struct, so the fix is only
// durable if the struct cannot grow past it. Every domainmodel.Entity field must
// appear in entityFieldsDeclaredByStatement or be preserved from the stored
// entity, and this fails on any field that is neither — which is what forces the
// next person adding one to decide rather than inherit whichever branch is
// convenient.
//
// The check is by VALUE, not by reading the source: both entities are filled
// with distinct non-zero values through reflection, so a field the merge forgot
// to assign shows up as the stored value and a field it should not have assigned
// shows up as the declared one.
func TestMergeDeclaredOntoStoredEntity_EveryFieldHasADecision(t *testing.T) {
	stored := &domainmodel.Entity{}
	declared := &domainmodel.Entity{}
	fillDistinctly(reflect.ValueOf(stored).Elem(), 1)
	fillDistinctly(reflect.ValueOf(declared).Elem(), 2)

	merged := mergeDeclaredOntoStoredEntity(stored, declared, &ast.CreateEntityStmt{DocumentationSet: true})

	et := reflect.TypeOf(domainmodel.Entity{})
	mv, sv, dv := reflect.ValueOf(*merged), reflect.ValueOf(*stored), reflect.ValueOf(*declared)
	for i := 0; i < et.NumField(); i++ {
		name := et.Field(i).Name
		if name == "BaseElement" || name == "ContainerID" {
			continue // element identity; preserved, and re-asserted at the call site
		}
		got, fromStored, fromDeclared := mv.Field(i), sv.Field(i), dv.Field(i)
		wantDeclared := entityFieldsDeclaredByStatement[name]

		if wantDeclared && !reflect.DeepEqual(got.Interface(), fromDeclared.Interface()) {
			t.Errorf("Entity.%s is listed as declared by the statement but the merge did "+
				"not take it from the rebuilt entity", name)
		}
		if !wantDeclared && !reflect.DeepEqual(got.Interface(), fromStored.Interface()) {
			t.Errorf("Entity.%s is not in entityFieldsDeclaredByStatement, so it must be "+
				"preserved from what is stored — the merge overwrote it. MDL has no "+
				"spelling for it, so an omission is not a request to clear it. This is "+
				"exactly how the access rules were lost.", name)
		}
	}
}

// fillDistinctly writes a value derived from seed into every settable field, so
// stored and declared differ in all of them.
func fillDistinctly(v reflect.Value, seed int) {
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString(map[int]string{1: "stored", 2: "declared"}[seed])
		case reflect.Bool:
			f.SetBool(seed == 2) // stored false, declared true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			f.SetInt(int64(seed))
		case reflect.Float32, reflect.Float64:
			f.SetFloat(float64(seed))
		case reflect.Slice:
			f.Set(reflect.MakeSlice(f.Type(), seed, seed))
		case reflect.Struct:
			fillDistinctly(f, seed)
		case reflect.Interface:
			// domainmodel.Generalization; a distinct concrete value per seed.
			if seed == 1 {
				if g := reflect.ValueOf(&domainmodel.NoGeneralization{}); g.Type().Implements(f.Type()) {
					f.Set(g)
				}
			} else if g := reflect.ValueOf(&domainmodel.GeneralizationBase{}); g.Type().Implements(f.Type()) {
				f.Set(g)
			}
		}
	}
}
