// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// ReconcileMemberAccesses used to SKIP any rule whose MemberAccesses list held
// only the storage marker:
//
//	// If empty (just the storage marker), skip
//	if len(maArr) <= 1 { break }
//
// A rule with no member entries on an entity that HAS members is precisely the
// out-of-date state CE0066 names, so the skip left behind the one thing this
// function exists to prevent. It also made the legacy engine disagree with the
// codec engine, which fills such a rule in.
//
// Nothing reached that state until `create or modify entity` started PRESERVING
// access rules instead of deleting them: a rewrite that drops every attribute a
// rule covered empties the list, and the entity's new attributes then never got
// an entry. Measured on the BusinessEvents 3.12.0 marketplace module against
// mxbuild 11.14.0 — `create or modify persistent entity
// BusinessEvents.PublishedBusinessEvent ( EventId: long )` over the real module
// took its Administrator rule from 5 members to 0 on legacy and 1 on modelsdk,
// and mx check reported CE0066 at "Domain model of module 'BusinessEvents'".

// seedRuleWithEmptyMembers inserts a domain model with one entity, two
// attributes, and one access rule whose MemberAccesses list is bare.
func seedRuleWithEmptyMembers(t *testing.T, db *sql.DB, attrNames ...string) model.ID {
	t.Helper()

	const (
		unitIDStr      = "11111111-1111-1111-1111-111111111111"
		containerIDStr = "22222222-2222-2222-2222-222222222222"
		entityIDStr    = "33333333-3333-3333-3333-333333333333"
	)

	attrs := bson.A{int32(3)}
	for i, name := range attrNames {
		attrs = append(attrs, bson.D{
			{Key: "$Type", Value: "DomainModels$StoredValue"},
			{Key: "$ID", Value: idToBsonBinary(attrIDForIndex(i))},
			{Key: "Name", Value: name},
		})
	}

	dmBSON := bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: idToBsonBinary(unitIDStr)},
		{Key: "Entities", Value: bson.A{
			int32(3),
			bson.D{
				{Key: "$Type", Value: "DomainModels$Entity"},
				{Key: "$ID", Value: idToBsonBinary(entityIDStr)},
				{Key: "Name", Value: "Order"},
				{Key: "Attributes", Value: attrs},
				{Key: "AccessRules", Value: bson.A{
					int32(3),
					bson.D{
						{Key: "$Type", Value: "DomainModels$AccessRule"},
						{Key: "$ID", Value: idToBsonBinary("44444444-4444-4444-4444-444444444444")},
						{Key: "AllowedModuleRoles", Value: bson.A{int32(1), "MyModule.Administrator"}},
						{Key: "DefaultMemberAccessRights", Value: "ReadOnly"},
						// The state a preserving rewrite leaves behind: the rule
						// survives, every member it named is gone.
						{Key: "MemberAccesses", Value: bson.A{int32(3)}},
					},
				}},
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

func attrIDForIndex(i int) string {
	return string(rune('a'+i)) + "aaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
}

// entityAttrNames reads back the seeded entity's attribute names.
func entityAttrNames(t *testing.T, db *sql.DB, unitID model.ID) []string {
	t.Helper()
	entity := readSeededEntity(t, db, unitID)
	var names []string
	for _, a := range extractBsonArray(entity["Attributes"]) {
		if m, ok := a.(map[string]any); ok {
			names = append(names, extractString(m["Name"]))
		}
	}
	return names
}

// memberAttrRefs reads back the rule's member entries.
func memberAttrRefs(t *testing.T, db *sql.DB, unitID model.ID) []string {
	t.Helper()
	entity := readSeededEntity(t, db, unitID)
	rules := extractBsonArray(entity["AccessRules"])
	if len(rules) == 0 {
		t.Fatal("no access rules")
	}
	rule := rules[0].(map[string]any)

	var refs []string
	for _, ma := range extractBsonArray(rule["MemberAccesses"]) {
		m, ok := ma.(map[string]any)
		if !ok {
			continue
		}
		refs = append(refs, extractString(m["Attribute"]))
	}
	return refs
}

// readSeededEntity returns the single entity of the seeded domain model unit.
func readSeededEntity(t *testing.T, db *sql.DB, unitID model.ID) map[string]any {
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
	entities := extractBsonArray(raw["Entities"])
	if len(entities) == 0 {
		t.Fatal("no entities")
	}
	entity, ok := entities[0].(map[string]any)
	if !ok {
		t.Fatalf("entity is %T, want map[string]any", entities[0])
	}
	return entity
}

func TestReconcileMemberAccesses_FillsARuleWhoseMembersAreAllGone(t *testing.T) {
	w, db := newTestWriterSecurity(t)
	unitID := seedRuleWithEmptyMembers(t, db, "EventId")

	// Read the seed back before asking anything of it. Without this, a fixture
	// that failed to round-trip is indistinguishable from the reconcile
	// declining to act, and the failure message would blame the wrong code.
	if attrs := entityAttrNames(t, db, unitID); len(attrs) != 1 || attrs[0] != "EventId" {
		t.Fatalf("fixture did not round-trip: entity attributes = %v, want [EventId]", attrs)
	}

	count, err := w.ReconcileMemberAccesses(unitID, "MyModule")
	if err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}
	if count == 0 {
		t.Fatal("reconcile reported no change — a rule with zero member entries on " +
			"an entity that has attributes is CE0066 \"Entity access is out of date\", " +
			"and it is exactly the state a preserving entity rewrite leaves behind")
	}

	refs := memberAttrRefs(t, db, unitID)
	if len(refs) != 1 || refs[0] != "MyModule.Order.EventId" {
		t.Errorf("member entries = %v, want [MyModule.Order.EventId]", refs)
	}
}

// CONTROL: an entity with NO members must still come out unchanged. The old
// early break covered this case by accident; removing it must not turn every
// member-less entity into a write.
func TestReconcileMemberAccesses_LeavesAMemberlessEntityAlone(t *testing.T) {
	w, db := newTestWriterSecurity(t)
	unitID := seedRuleWithEmptyMembers(t, db) // no attributes

	count, err := w.ReconcileMemberAccesses(unitID, "MyModule")
	if err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}
	if count != 0 {
		t.Errorf("reconcile rewrote %d rule(s) on an entity with no members", count)
	}
	if refs := memberAttrRefs(t, db, unitID); len(refs) != 0 {
		t.Errorf("member entries = %v, want none", refs)
	}
}

// CONTROL: the ordinary case — some members covered, one not — must keep working.
// This is the path the early break never reached, so a fix that broke it would
// otherwise go unnoticed by the test above.
func TestReconcileMemberAccesses_StillTopsUpAPartiallyCoveredRule(t *testing.T) {
	w, db := newTestWriterSecurity(t)
	unitID := seedRuleWithEmptyMembers(t, db, "Ref", "Amount")

	// Give the rule an entry for Ref only.
	if err := w.AddEntityAccessRule(unitID, "Order",
		[]string{"MyModule.Administrator"}, false, false, "ReadOnly", "",
		[]EntityMemberAccess{{AttributeRef: "MyModule.Order.Ref", AccessRights: "ReadOnly"}},
	); err != nil {
		t.Fatalf("AddEntityAccessRule: %v", err)
	}

	if _, err := w.ReconcileMemberAccesses(unitID, "MyModule"); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	got := map[string]bool{}
	for _, r := range memberAttrRefs(t, db, unitID) {
		got[r] = true
	}
	for _, want := range []string{"MyModule.Order.Ref", "MyModule.Order.Amount"} {
		if !got[want] {
			t.Errorf("missing member entry %s (got %v)", want, got)
		}
	}
}
