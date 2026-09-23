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
		if entityFieldsMergedWithStored[name] {
			// Neither side owns the whole field, so there is no value to compare
			// against. The semantics are covered by the named tests above
			// (KeepsRulesTheEntityBodyCannotSpell and its three controls), and
			// listing a field here is the deliberate act of taking it out of this
			// guard's reach.
			continue
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

// ---------------------------------------------------------------------------
// Validation rules the entity body has no words for (ako/mxcli#556)
// ---------------------------------------------------------------------------

// The reported symptom: re-running an idempotent script reports
// `Modified entity: …` + `Created RegEx validation rule on …` on EVERY run, and
// the unit comes back the same size with a handful of 16-byte runs changed.
//
// MEASURED on the v1 fixture, two statements that say nothing new:
//
//	create or modify entity Demo.Widget ( SerialNumber: String(100) );
//	create validation rule for Demo.Widget.SerialNumber regex Demo.SerialPattern ...;
//
//	unit 1368 bytes before and after, 62 bytes differ, at
//	  ValidationRules/1/$ID, .../Message/$ID, .../Message/Items/1/$ID, .../RuleInfo/$ID
//
// Splitting the two statements says which one moves: the entity rewrite alone
// takes the unit from 1368 to 947 bytes — it DELETES the rule — and the
// validation-rule statement then puts an identically-shaped one back under four
// fresh identities. The validation-rule statement on its own is already
// idempotent to the byte (canon transplants the ids back), so the churn is
// entirely the drop.
//
// The drop is the bug, not the re-mint. `create [or modify] entity` can only
// spell Required (`not null`) and Unique (`unique`); RegEx and Range have no
// spelling in the entity body at all, so an omission carries no meaning — the
// same reasoning that already preserves access rules here.
func storedEntityWithARegexRule() *domainmodel.Entity {
	e := storedEntityWithARule()
	e.ValidationRules = []*domainmodel.ValidationRule{{
		BaseElement: model.BaseElement{ID: "vr-regex"},
		AttributeID: "Fx.Probe.Code",
		Type:        "RegEx",
		Rule:        &domainmodel.RegexValidationRuleInfo{RegularExpressionQualifiedName: "Fx.SerialPattern"},
	}}
	return e
}

func TestMergeDeclaredOntoStoredEntity_KeepsRulesTheEntityBodyCannotSpell(t *testing.T) {
	stored := storedEntityWithARegexRule()
	merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name", "Code"), &ast.CreateEntityStmt{})

	if len(merged.ValidationRules) != 1 || merged.ValidationRules[0].Type != "RegEx" {
		t.Fatalf("the rewrite kept %+v, want the stored RegEx rule — `create or modify entity` "+
			"has no spelling for one, so dropping it makes every re-run re-create it under "+
			"fresh identities (ako/mxcli#556)", merged.ValidationRules)
	}
	if merged.ValidationRules[0].ID != "vr-regex" {
		t.Errorf("the carried rule lost its identity: %q", merged.ValidationRules[0].ID)
	}
}

// A MaxLength or EqualsTo rule does not survive the READ (the model carries no
// payload type for it), so carrying it is what lets UpdateEntity REFUSE the
// rewrite. Dropping it here is worse than refusing: the constraint is gone and
// the build still passes.
func TestMergeDeclaredOntoStoredEntity_KeepsAnUnreadableRuleSoTheWriteCanRefuse(t *testing.T) {
	stored := storedEntityWithARule()
	stored.ValidationRules = []*domainmodel.ValidationRule{{
		BaseElement: model.BaseElement{ID: "vr-maxlength"},
		AttributeID: "Fx.Probe.Code",
		Type:        "MaxLength",
	}}
	merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name", "Code"), &ast.CreateEntityStmt{})
	if len(merged.ValidationRules) != 1 {
		t.Fatalf("a MaxLength rule was silently dropped by the rewrite (%d rules kept) — "+
			"UpdateEntity's guard-don't-drop refusal never even sees it", len(merged.ValidationRules))
	}
}

// The other half of the contract, and the control that a merge broken into
// "keep everything stored" would fail: Required and Unique DO have a spelling,
// so the statement stays authoritative about them. Omitting `not null` removes
// the Required rule.
func TestMergeDeclaredOntoStoredEntity_StatementStillOwnsRequiredAndUnique(t *testing.T) {
	stored := storedEntityWithARegexRule()
	stored.ValidationRules = append(stored.ValidationRules, &domainmodel.ValidationRule{
		BaseElement: model.BaseElement{ID: "vr-required"},
		AttributeID: "Fx.Probe.Name",
		Type:        "Required",
	})

	merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name", "Code"), &ast.CreateEntityStmt{})

	for _, vr := range merged.ValidationRules {
		if vr.Type == "Required" {
			t.Fatalf("omitting `not null` left the Required rule in place — the statement is " +
				"authoritative about the rule types it can spell")
		}
	}
	if len(merged.ValidationRules) != 1 {
		t.Fatalf("kept %d rules, want only the RegEx one", len(merged.ValidationRules))
	}
}

// A re-declared Required rule is taken from the STATEMENT, not carried — its
// error message is part of what the statement says.
func TestMergeDeclaredOntoStoredEntity_RedeclaredRequiredComesFromTheStatement(t *testing.T) {
	stored := storedEntityWithARule()
	stored.ValidationRules = []*domainmodel.ValidationRule{{
		BaseElement:  model.BaseElement{ID: "vr-stored"},
		AttributeID:  "Fx.Probe.Name",
		Type:         "Required",
		ErrorMessage: &model.Text{Translations: map[string]string{"en_US": "old message"}},
	}}
	declared := declaredEntity("Name", "Code")
	declared.Attributes[0].ID = "attr-name"
	declared.ValidationRules = []*domainmodel.ValidationRule{{
		BaseElement:  model.BaseElement{ID: "vr-declared"},
		AttributeID:  "attr-name",
		Type:         "Required",
		ErrorMessage: &model.Text{Translations: map[string]string{"en_US": "new message"}},
	}}

	merged := mergeDeclaredOntoStoredEntity(stored, declared, &ast.CreateEntityStmt{})
	if len(merged.ValidationRules) != 1 {
		t.Fatalf("kept %d rules, want 1", len(merged.ValidationRules))
	}
	if got := merged.ValidationRules[0].ErrorMessage.Translations["en_US"]; got != "new message" {
		t.Errorf("the re-declared Required rule kept the stored message %q", got)
	}
}

// A rule whose attribute this rewrite REMOVED must go with it, or it outlives
// the attribute it constrains — CE1613, the same class reconcileDroppedIndexes
// and pruneMemberAccessesForDroppedAttributes exist to prevent.
func TestMergeDeclaredOntoStoredEntity_DropsARuleWhoseAttributeWentAway(t *testing.T) {
	stored := storedEntityWithARegexRule()
	merged := mergeDeclaredOntoStoredEntity(stored, declaredEntity("Name"), &ast.CreateEntityStmt{})
	if len(merged.ValidationRules) != 0 {
		t.Errorf("the rule on the dropped Code attribute survived: %+v", merged.ValidationRules)
	}
}

// CONTROL for that, and the specialisation trap pruneMemberAccessesForDroppedAttributes
// already paid for: a rule naming a member the entity does not own itself
// (inherited, or an attribute the reader spelled differently) is kept unless the
// STORED entity owned an attribute of that name and this rewrite removed it.
func TestMergeDeclaredOntoStoredEntity_KeepsARuleNamingAMemberTheEntityDoesNotOwn(t *testing.T) {
	stored := &domainmodel.Entity{
		Name:              "ExportDocument",
		GeneralizationRef: "System.FileDocument",
		ValidationRules: []*domainmodel.ValidationRule{{
			BaseElement: model.BaseElement{ID: "vr-inherited"},
			AttributeID: "System.FileDocument.Name",
			Type:        "RegEx",
			Rule:        &domainmodel.RegexValidationRuleInfo{RegularExpressionQualifiedName: "Fx.P"},
		}},
	}
	declared := &domainmodel.Entity{Name: "ExportDocument", GeneralizationRef: "System.FileDocument"}
	merged := mergeDeclaredOntoStoredEntity(stored, declared, &ast.CreateEntityStmt{})
	if len(merged.ValidationRules) != 1 {
		t.Errorf("a rule on an inherited member was dropped by an entity that owns no attributes")
	}
}
