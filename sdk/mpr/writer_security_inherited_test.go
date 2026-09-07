// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// mendixlabs/mxcli#1047: adding an attribute to a GENERALIZATION left the
// project at CE0066 "Entity access is out of date", and `UPDATE SECURITY`
// reported "All entity access rules are up to date" without changing anything.
//
// ReconcileMemberAccesses computed a specialization's expected member set from
// the entity's OWN attributes, so an inherited one was never missing and never
// added. Both engines had it, in the same shape; this is the legacy half.

// seedGeneralizationChain inserts a domain model holding Gen (attribute Name)
// and Spec (extends Gen, attribute Extra), each with one access rule. The rules
// list only what the entity declares itself, which is the state a project
// reaches when the generalization gains an attribute afterwards.
func seedGeneralizationChain(t *testing.T, db *sql.DB) model.ID {
	t.Helper()

	const (
		unitIDStr      = "11111111-1111-1111-1111-111111111111"
		containerIDStr = "22222222-2222-2222-2222-222222222222"
		genIDStr       = "33333333-3333-3333-3333-333333333333"
		specIDStr      = "55555555-5555-5555-5555-555555555555"
	)

	attr := func(id, name string) bson.D {
		return bson.D{
			{Key: "$Type", Value: "DomainModels$StoredValue"},
			{Key: "$ID", Value: idToBsonBinary(id)},
			{Key: "Name", Value: name},
		}
	}
	rule := func(id string, members bson.A) bson.D {
		return bson.D{
			{Key: "$Type", Value: "DomainModels$AccessRule"},
			{Key: "$ID", Value: idToBsonBinary(id)},
			{Key: "AllowedModuleRoles", Value: bson.A{int32(1), "MyModule.Administrator"}},
			{Key: "DefaultMemberAccessRights", Value: "ReadWrite"},
			{Key: "MemberAccesses", Value: members},
		}
	}
	member := func(id, ref string) bson.D {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(id)},
			{Key: "$Type", Value: "DomainModels$MemberAccess"},
			{Key: "AccessRights", Value: "ReadWrite"},
			{Key: "Attribute", Value: ref},
		}
	}

	dmBSON := bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: idToBsonBinary(unitIDStr)},
		{Key: "Entities", Value: bson.A{
			int32(3),
			bson.D{
				{Key: "$Type", Value: "DomainModels$Entity"},
				{Key: "$ID", Value: idToBsonBinary(genIDStr)},
				{Key: "Name", Value: "Gen"},
				{Key: "Attributes", Value: bson.A{int32(3), attr(attrIDForIndex(0), "Name")}},
				{Key: "AccessRules", Value: bson.A{int32(3),
					rule("44444444-4444-4444-4444-444444444444", bson.A{
						int32(3), member("66666666-6666-6666-6666-666666666666", "MyModule.Gen.Name"),
					})}},
			},
			bson.D{
				{Key: "$Type", Value: "DomainModels$Entity"},
				{Key: "$ID", Value: idToBsonBinary(specIDStr)},
				{Key: "Name", Value: "Spec"},
				{Key: "Attributes", Value: bson.A{int32(3), attr(attrIDForIndex(1), "Extra")}},
				{Key: "Generalization", Value: bson.D{
					{Key: "$Type", Value: "DomainModels$Generalization"},
					{Key: "$ID", Value: idToBsonBinary("77777777-7777-7777-7777-777777777777")},
					{Key: "Generalization", Value: "MyModule.Gen"},
				}},
				{Key: "AccessRules", Value: bson.A{int32(3),
					rule("88888888-8888-8888-8888-888888888888", bson.A{
						int32(3), member("99999999-9999-9999-9999-999999999999", "MyModule.Spec.Extra"),
					})}},
			},
		}},
		{Key: "Associations", Value: bson.A{int32(3)}},
	}

	contents, err := bson.Marshal(dmBSON)
	if err != nil {
		t.Fatalf("marshal domain model: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO Unit (UnitID, ContainerID, ContainmentName, TreeConflict, ContentsHash, ContentsConflicts, Contents)
		VALUES (?, ?, 'DomainModel', 0, ?, '', ?)`,
		uuidToBlob(unitIDStr), uuidToBlob(containerIDStr),
		contentHashBase64(contents), contents,
	); err != nil {
		t.Fatalf("insert domain model unit: %v", err)
	}
	return model.ID(unitIDStr)
}

// memberRefsOfEntity reads one named entity's first rule's attribute references.
func memberRefsOfEntity(t *testing.T, db *sql.DB, unitID model.ID, entityName string) []string {
	t.Helper()
	var contents []byte
	if err := db.QueryRow(`SELECT Contents FROM Unit WHERE UnitID = ?`,
		uuidToBlob(string(unitID))).Scan(&contents); err != nil {
		t.Fatalf("read unit: %v", err)
	}
	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, e := range extractBsonArray(raw["Entities"]) {
		ent, ok := e.(map[string]any)
		if !ok || extractString(ent["Name"]) != entityName {
			continue
		}
		rules := extractBsonArray(ent["AccessRules"])
		if len(rules) == 0 {
			t.Fatalf("entity %s has no access rules", entityName)
		}
		var refs []string
		for _, ma := range extractBsonArray(rules[0].(map[string]any)["MemberAccesses"]) {
			if m, ok := ma.(map[string]any); ok {
				refs = append(refs, extractString(m["Attribute"]))
			}
		}
		return refs
	}
	t.Fatalf("entity %s not found", entityName)
	return nil
}

func hasString(vals []string, want string) bool {
	for _, v := range vals {
		if v == want {
			return true
		}
	}
	return false
}

func TestReconcileMemberAccesses_AddsAnInheritedAttribute(t *testing.T) {
	w, db := newTestWriterSecurity(t)
	unitID := seedGeneralizationChain(t, db)

	// Read the seed back before asking anything of it: a fixture that failed to
	// store the chain would make the assertions below meaningless.
	if refs := memberRefsOfEntity(t, db, unitID, "Spec"); len(refs) != 1 || refs[0] != "MyModule.Spec.Extra" {
		t.Fatalf("fixture did not store the specialization's rule as expected: %v", refs)
	}

	modified, err := w.ReconcileMemberAccesses(unitID, "MyModule")
	if err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	refs := memberRefsOfEntity(t, db, unitID, "Spec")
	if !hasString(refs, "MyModule.Gen.Name") {
		t.Fatalf("the inherited attribute was not added: %v\n"+
			"this is the CE0066 in mendixlabs/mxcli#1047 — and the reference must name Gen, "+
			"the entity that DECLARES it, not Spec", refs)
	}
	if !hasString(refs, "MyModule.Spec.Extra") {
		t.Errorf("the specialization's own attribute was dropped: %v", refs)
	}
	// The count is what `update security` turns into its message. Reporting 0
	// while adding a member is how "All entity access rules are up to date" came
	// to be printed over a project mx check rejects.
	if modified == 0 {
		t.Error("a member was added but 0 modified was reported")
	}
}

// Running it twice must not add a second copy. The compare pass keys on the
// full reference; keying on the bare attribute name instead would leave the
// inherited entry looking uncovered on every later run.
func TestReconcileMemberAccesses_InheritedAttributeIsNotDuplicated(t *testing.T) {
	w, db := newTestWriterSecurity(t)
	unitID := seedGeneralizationChain(t, db)

	if _, err := w.ReconcileMemberAccesses(unitID, "MyModule"); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	first := memberRefsOfEntity(t, db, unitID, "Spec")

	modified, err := w.ReconcileMemberAccesses(unitID, "MyModule")
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	second := memberRefsOfEntity(t, db, unitID, "Spec")

	if len(second) != len(first) {
		t.Errorf("a second reconcile changed the member list: %v -> %v", first, second)
	}
	if modified != 0 {
		t.Errorf("a second reconcile reported %d modified; an in-sync rule must be quiet", modified)
	}
}

// The generalization's own rule is already complete, so it must not change —
// otherwise the test above would pass against a fix that rewrites everything.
func TestReconcileMemberAccesses_LeavesTheGeneralizationsRuleAlone(t *testing.T) {
	w, db := newTestWriterSecurity(t)
	unitID := seedGeneralizationChain(t, db)

	before := memberRefsOfEntity(t, db, unitID, "Gen")
	if _, err := w.ReconcileMemberAccesses(unitID, "MyModule"); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}
	after := memberRefsOfEntity(t, db, unitID, "Gen")

	if len(before) != len(after) || !hasString(after, "MyModule.Gen.Name") {
		t.Errorf("the generalization's rule changed: %v -> %v", before, after)
	}
	for _, r := range after {
		if r != "MyModule.Gen.Name" {
			t.Errorf("the generalization gained a member it does not declare: %v", after)
		}
	}
}
