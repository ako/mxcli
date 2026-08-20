// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#936: re-granting access to an entity SHRANK the rule instead
// of widening it. `GRANT Viewer ON Customer (READ (Name, Email))` followed by
// `GRANT Viewer ON Customer (READ (Phone))` left the rule reading `(Phone)`
// alone — Name and Email silently revoked, with no error, warning or diff.
//
// Two independent defects, both reproduced here:
//
//  1. AddEntityAccessRule cleared the matched rule's MemberAccesses and rebuilt
//     them from the current statement alone. The executor emits an entry for
//     EVERY member of the entity (unnamed ones at the rule's default, normally
//     "None"), so anything absent from the latest GRANT was written back as
//     None. Legacy has merged additively since it shipped
//     (sdk/mpr.mergeAccessRule); the codec engine — the default — never did, so
//     this is an engine regression, not a longstanding gap. The blast radius is
//     wider than the report says: AllowCreate/AllowDelete and the rule's default
//     rights were lost the same way, so `(CREATE, DELETE, READ *, WRITE *)`
//     followed by `(READ (Name))` left create, delete and every write right off.
//
//  2. The upsert matched a stored rule on its module-role set ALONE, ignoring
//     XPathConstraint. Mendix's own reference guide is explicit that several
//     rules may name the same module role and that "all access rights of those
//     rules are combined", which is how row-level security is normally written.
//     Matching without the constraint therefore collapses two legitimate rules
//     into one and destroys the first.
//
// The rights lattice is None < ReadOnly < ReadWrite, so a merge never narrows —
// which is exactly the documented contract for GRANT ("additive ... never
// removes permissions"). Narrowing is REVOKE's job.
package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// grantFixture creates ZzCust(Name, Email, Phone) and returns the connected
// backend plus the module and domain-model handles.
func grantFixture(t *testing.T) (*Backend, *model.Module, *domainmodel.DomainModel) {
	t.Helper()
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	if err := b.CreateEntity(dm.ID, &domainmodel.Entity{
		Name: "ZzCust", Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Name", Type: &domainmodel.StringAttributeType{}},
			{Name: "Email", Type: &domainmodel.StringAttributeType{}},
			{Name: "Phone", Type: &domainmodel.StringAttributeType{}},
		},
	}); err != nil {
		t.Fatalf("CreateEntity ZzCust: %v", err)
	}
	return b, mod, dm
}

// grantParams mirrors what the executor builds for
// `GRANT MyFirstModule.User ON ZzCust (<rights>) [WHERE xpath]`: one entry per
// member of the entity, the named ones at `rights` and the rest at `def`.
func grantParams(dmID model.ID, def, xpath string, named map[string]string) backend.EntityAccessRuleParams {
	p := backend.EntityAccessRuleParams{
		UnitID: dmID, EntityName: "ZzCust",
		RoleNames:           []string{"MyFirstModule.User"},
		DefaultMemberAccess: def,
		XPathConstraint:     xpath,
	}
	for _, attr := range []string{"Name", "Email", "Phone"} {
		rights := def
		if r, ok := named[attr]; ok {
			rights = r
		}
		p.MemberAccesses = append(p.MemberAccesses, types.EntityMemberAccess{
			AttributeRef: "MyFirstModule.ZzCust." + attr, AccessRights: rights,
		})
	}
	return p
}

// rulesOf returns the stored access rules of ZzCust.
func rulesOf(t *testing.T, b *Backend, modID model.ID) []*domainmodel.AccessRule {
	t.Helper()
	dm, err := b.GetDomainModel(modID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	for _, e := range dm.Entities {
		if e.Name == "ZzCust" {
			return e.AccessRules
		}
	}
	t.Fatalf("entity ZzCust not found")
	return nil
}

// rightsOf maps attribute name -> access rights for a single rule.
func rightsOf(rule *domainmodel.AccessRule) map[string]string {
	out := map[string]string{}
	for _, ma := range rule.MemberAccesses {
		if ma.AttributeName == "" {
			continue
		}
		// stored as Module.Entity.Attr — key on the bare member name
		name := ma.AttributeName
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		out[name] = string(ma.AccessRights)
	}
	return out
}

func onlyRule(t *testing.T, b *Backend, modID model.ID) *domainmodel.AccessRule {
	t.Helper()
	rules := rulesOf(t, b, modID)
	if len(rules) != 1 {
		t.Fatalf("want exactly 1 access rule, got %d", len(rules))
	}
	return rules[0]
}

// Defect 1, the reported symptom: the second GRANT must widen the rule, not
// replace it. Before the fix this stored Name=None, Email=None, Phone=ReadOnly.
func TestGrant_WidensMemberAccessInsteadOfReplacingIt(t *testing.T) {
	b, mod, dm := grantFixture(t)

	first := grantParams(dm.ID, "None", "", map[string]string{"Name": "ReadOnly", "Email": "ReadOnly"})
	if err := b.AddEntityAccessRule(first); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	second := grantParams(dm.ID, "None", "", map[string]string{"Phone": "ReadOnly"})
	if err := b.AddEntityAccessRule(second); err != nil {
		t.Fatalf("second grant: %v", err)
	}

	got := rightsOf(onlyRule(t, b, mod.ID))
	for _, attr := range []string{"Name", "Email", "Phone"} {
		if got[attr] != "ReadOnly" {
			t.Errorf("%s = %q, want ReadOnly — the re-grant revoked a previously granted attribute (#936); full rule: %v",
				attr, got[attr], got)
		}
	}
}

// The same defect with a WHERE clause, which is the form the issue was filed
// against. It is NOT the trigger — the case above fails identically without one
// — but a fix scoped to the constrained path would pass one and not the other.
func TestGrant_WidensMemberAccessUnderAnXPathConstraint(t *testing.T) {
	b, mod, dm := grantFixture(t)

	const xpath = "[Name != $currentUser/Name]"
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", xpath,
		map[string]string{"Name": "ReadOnly", "Email": "ReadOnly"})); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", xpath,
		map[string]string{"Phone": "ReadOnly"})); err != nil {
		t.Fatalf("second grant: %v", err)
	}

	rule := onlyRule(t, b, mod.ID)
	if rule.XPathConstraint != xpath {
		t.Errorf("XPathConstraint = %q, want %q", rule.XPathConstraint, xpath)
	}
	got := rightsOf(rule)
	for _, attr := range []string{"Name", "Email", "Phone"} {
		if got[attr] != "ReadOnly" {
			t.Errorf("%s = %q, want ReadOnly; full rule: %v", attr, got[attr], got)
		}
	}
}

// A member already at ReadWrite must not be downgraded by a later GRANT that
// only asks for read — the lattice takes the higher of the two.
func TestGrant_KeepsTheHigherRightsPerMember(t *testing.T) {
	b, mod, dm := grantFixture(t)

	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", "",
		map[string]string{"Name": "ReadWrite"})); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", "",
		map[string]string{"Name": "ReadOnly", "Email": "ReadOnly"})); err != nil {
		t.Fatalf("second grant: %v", err)
	}

	got := rightsOf(onlyRule(t, b, mod.ID))
	if got["Name"] != "ReadWrite" {
		t.Errorf("Name = %q, want ReadWrite — a read grant downgraded an existing write right", got["Name"])
	}
	if got["Email"] != "ReadOnly" {
		t.Errorf("Email = %q, want ReadOnly", got["Email"])
	}
}

// Defect 1's wider blast radius: create/delete and the rule's default rights are
// structural, and a narrow re-grant used to drop them too. Before the fix this
// turned (CREATE, DELETE, READ *, WRITE *) into a read-only rule on one member.
func TestGrant_KeepsStructuralPermissions(t *testing.T) {
	b, mod, dm := grantFixture(t)

	full := grantParams(dm.ID, "ReadWrite", "", nil)
	full.AllowCreate, full.AllowDelete = true, true
	if err := b.AddEntityAccessRule(full); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", "",
		map[string]string{"Name": "ReadOnly"})); err != nil {
		t.Fatalf("second grant: %v", err)
	}

	rule := onlyRule(t, b, mod.ID)
	if !rule.AllowCreate {
		t.Error("AllowCreate was revoked by a narrower re-grant (#936)")
	}
	if !rule.AllowDelete {
		t.Error("AllowDelete was revoked by a narrower re-grant (#936)")
	}
	if rule.DefaultMemberAccessRights != domainmodel.MemberAccessRightsReadWrite {
		t.Errorf("DefaultMemberAccessRights = %q, want ReadWrite — the rule's default was narrowed",
			rule.DefaultMemberAccessRights)
	}
}

// Defect 2: two constraints for one role are two rules in Mendix, and their
// rights are combined. Matching on the role set alone collapsed them and
// destroyed the first.
func TestGrant_DistinctXPathConstraintsStayDistinctRules(t *testing.T) {
	b, mod, dm := grantFixture(t)

	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", "[Name = 'a']",
		map[string]string{"Name": "ReadOnly"})); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", "[Name = 'b']",
		map[string]string{"Email": "ReadOnly"})); err != nil {
		t.Fatalf("second grant: %v", err)
	}

	rules := rulesOf(t, b, mod.ID)
	if len(rules) != 2 {
		t.Fatalf("want 2 access rules (one per constraint), got %d — the second grant overwrote the first", len(rules))
	}
	byXPath := map[string]*domainmodel.AccessRule{}
	for _, r := range rules {
		byXPath[r.XPathConstraint] = r
	}
	a, okA := byXPath["[Name = 'a']"]
	bRule, okB := byXPath["[Name = 'b']"]
	if !okA || !okB {
		t.Fatalf("constraints not preserved: %v", byXPath)
	}
	if got := rightsOf(a)["Name"]; got != "ReadOnly" {
		t.Errorf("rule a: Name = %q, want ReadOnly", got)
	}
	if got := rightsOf(bRule)["Email"]; got != "ReadOnly" {
		t.Errorf("rule b: Email = %q, want ReadOnly", got)
	}
}

// An unconstrained rule and a constrained one are likewise distinct: the empty
// constraint is a value in the key, not a wildcard that matches anything.
func TestGrant_ConstrainedAndUnconstrainedRulesCoexist(t *testing.T) {
	b, mod, dm := grantFixture(t)

	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", "",
		map[string]string{"Name": "ReadOnly"})); err != nil {
		t.Fatalf("unconstrained grant: %v", err)
	}
	if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", "[Name = 'a']",
		map[string]string{"Email": "ReadOnly"})); err != nil {
		t.Fatalf("constrained grant: %v", err)
	}

	if rules := rulesOf(t, b, mod.ID); len(rules) != 2 {
		t.Fatalf("want 2 access rules, got %d", len(rules))
	}
}

// Re-running the same GRANT must not accumulate rules — the constraint is part
// of the key, so a repeat matches its own rule. Idempotence is ADR-0008's
// contract and the reason `mxcli exec` can be re-run against a project.
func TestGrant_RepeatingTheSameStatementIsIdempotent(t *testing.T) {
	b, mod, dm := grantFixture(t)

	for i := 0; i < 3; i++ {
		if err := b.AddEntityAccessRule(grantParams(dm.ID, "None", "[Name = 'a']",
			map[string]string{"Name": "ReadOnly"})); err != nil {
			t.Fatalf("grant %d: %v", i, err)
		}
	}

	rules := rulesOf(t, b, mod.ID)
	if len(rules) != 1 {
		t.Fatalf("want 1 access rule after 3 identical grants, got %d", len(rules))
	}
	if got := rightsOf(rules[0])["Name"]; got != "ReadOnly" {
		t.Errorf("Name = %q, want ReadOnly", got)
	}
}
