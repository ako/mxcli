// SPDX-License-Identifier: Apache-2.0

package xpathrefs

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// memStore is an in-memory Store holding whole BSON documents.
type memStore struct {
	units   []*types.RawUnit
	written map[string][]byte
}

func (s *memStore) ListRawUnitsByType(string) ([]*types.RawUnit, error) { return s.units, nil }

func (s *memStore) UpdateRawUnit(unitID string, contents []byte) error {
	if s.written == nil {
		s.written = map[string][]byte{}
	}
	s.written[unitID] = contents
	return nil
}

// constraintsIn returns every XPath constraint stored anywhere in the document.
func constraintsIn(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch val := v.(type) {
		case bson.D:
			for _, e := range val {
				if e.Key == "XPathConstraint" || e.Key == "XpathConstraint" {
					if s, ok := e.Value.(string); ok {
						out = append(out, s)
					}
				}
				walk(e.Value)
			}
		case bson.A:
			for _, item := range val {
				walk(item)
			}
		}
	}
	walk(doc)
	return out
}

func unit(id, typ string, doc bson.D) *types.RawUnit {
	b, _ := bson.Marshal(doc)
	return &types.RawUnit{ID: model.ID(id), Type: typ, Contents: b}
}

// retrieveUnit builds a microflow holding one database retrieve. The shape
// matches what mxcli writes: the constraint's target entity is the sibling
// "Entity" field, and the key is spelled "XpathConstraint" here and
// "XPathConstraint" everywhere else.
func retrieveUnit(id, name, entity, constraint string) *types.RawUnit {
	return unit(id, "Microflows$Microflow", bson.D{
		{Key: "Name", Value: name},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "Objects", Value: bson.A{
				bson.D{
					{Key: "$Type", Value: "Microflows$ActionActivity"},
					{Key: "Action", Value: bson.D{
						{Key: "$Type", Value: "Microflows$RetrieveAction"},
						{Key: "RetrieveSource", Value: bson.D{
							{Key: "$Type", Value: "Microflows$DatabaseRetrieveSource"},
							{Key: "Entity", Value: entity},
							{Key: "XpathConstraint", Value: constraint},
						}},
					}},
				},
			}},
		}},
	})
}

// widgetUnit builds a page holding one XPath data source, whose target entity is
// a nested EntityRef rather than a plain string.
func widgetUnit(id, name, entity, constraint string) *types.RawUnit {
	return unit(id, "Forms$Page", bson.D{
		{Key: "Name", Value: name},
		{Key: "Widgets", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "CustomWidgets$CustomWidgetXPathSource"},
				{Key: "EntityRef", Value: bson.D{{Key: "QualifiedName", Value: entity}}},
				{Key: "XPathConstraint", Value: constraint},
			},
		}},
	})
}

// domainModelUnit builds a domain model whose entities carry access rules. An
// access rule names no entity of its own — it is attributed to the entity it
// sits inside, which is the case a sibling-field lookup cannot handle.
func domainModelUnit(id string, entities ...bson.D) *types.RawUnit {
	return unit(id, "DomainModels$DomainModel", bson.D{
		{Key: "Entities", Value: bson.A(func() []any {
			out := make([]any, 0, len(entities))
			for _, e := range entities {
				out = append(out, e)
			}
			return out
		}())},
	})
}

// entityWithRule builds a domain model entity node under the $Type actually
// found on disk (measured on Mendix 11.13.0), not the metamodel's name for an
// entity reference target.
func entityWithRule(name, constraint string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "DomainModels$EntityImpl"},
		{Key: "Name", Value: name},
		{Key: "AccessRules", Value: bson.A{
			bson.D{
				{Key: "$Type", Value: "DomainModels$AccessRule"},
				{Key: "XPathConstraint", Value: constraint},
			},
		}},
	}
}

func renameIn(s *memStore) (Result, error) {
	return RenameAttribute(s, testModel{}, "Sales.Person", "dm-1", "Person", "FirstName", "GivenName")
}

func TestScanRewritesRetrieveConstraint(t *testing.T) {
	s := &memStore{units: []*types.RawUnit{
		retrieveUnit("mf-1", "ACT_Find", "Sales.Person", "[FirstName = 'Ada']"),
	}}

	res, err := renameIn(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 1 || res.Units != 1 {
		t.Fatalf("got %d rewrites in %d units, want 1 in 1 (skipped: %+v)", res.Total(), res.Units, res.Skipped)
	}
	if got := constraintsIn(t, s.written["mf-1"]); len(got) != 1 || got[0] != "[GivenName = 'Ada']" {
		t.Errorf("stored constraint is %q", got)
	}
}

func TestScanRewritesWidgetDatasourceConstraint(t *testing.T) {
	s := &memStore{units: []*types.RawUnit{
		widgetUnit("pg-1", "Overview", "Sales.Person", "[FirstName != '']"),
	}}

	res, err := renameIn(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 1 {
		t.Fatalf("got %d rewrites, want 1 (skipped: %+v)", res.Total(), res.Skipped)
	}
	if got := constraintsIn(t, s.written["pg-1"]); len(got) != 1 || got[0] != "[GivenName != '']" {
		t.Errorf("stored constraint is %q", got)
	}
}

// TestScanRewritesAccessRuleInOwnDomainModel pins the case with no entity
// reference to read: the rule is attributed to the entity that contains it.
func TestScanRewritesAccessRuleInOwnDomainModel(t *testing.T) {
	s := &memStore{units: []*types.RawUnit{
		domainModelUnit("dm-1",
			entityWithRule("Person", "[FirstName != '']"),
			entityWithRule("Order", "[FirstName != '']"),
		),
	}}

	res, err := renameIn(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 1 {
		t.Fatalf("got %d rewrites, want exactly the Person rule (skipped: %+v)", res.Total(), res.Skipped)
	}
	got := constraintsIn(t, s.written["dm-1"])
	want := []string{"[GivenName != '']", "[FirstName != '']"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %q, want %q — Order's rule must not move", got, want)
	}
}

// TestScanIgnoresAnotherModulesDomainModel pins that the entity name alone is not
// enough: a Person in another module has its own FirstName.
func TestScanIgnoresAnotherModulesDomainModel(t *testing.T) {
	s := &memStore{units: []*types.RawUnit{
		domainModelUnit("dm-2", entityWithRule("Person", "[FirstName != '']")),
	}}

	res, err := renameIn(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 0 {
		t.Errorf("a same-named entity in another module was rewritten: %+v", res.Rewritten)
	}
	if len(s.written) != 0 {
		t.Errorf("a unit was written with nothing to change: %v", s.written)
	}
}

// TestScanDoesNotWriteUnchangedUnits pins that a document with constraints that
// do not concern this rename is not rewritten at all. Writing it would churn the
// project's version control for no reason.
func TestScanDoesNotWriteUnchangedUnits(t *testing.T) {
	s := &memStore{units: []*types.RawUnit{
		retrieveUnit("mf-1", "ACT_Find", "Sales.Person", "[LastName = 'L']"),
		retrieveUnit("mf-2", "ACT_Other", "Sales.Order", "[FirstName = 'Ada']"),
	}}

	res, err := renameIn(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 0 || res.Units != 0 {
		t.Errorf("got %d rewrites in %d units, want none", res.Total(), res.Units)
	}
	if len(s.written) != 0 {
		t.Errorf("units were written with nothing to change: %v", s.written)
	}
}

// TestScanReportsUnresolvableConstraint pins that a constraint naming the
// attribute which the walk cannot resolve is reported and left alone — the
// project is no worse than before, and the user is told where to look.
func TestScanReportsUnresolvableConstraint(t *testing.T) {
	const in = "[Sales.Mystery/FirstName = 'Ada']"
	s := &memStore{units: []*types.RawUnit{
		retrieveUnit("mf-1", "ACT_Find", "Sales.Person", in),
	}}

	res, err := renameIn(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 0 {
		t.Errorf("an unresolvable constraint was rewritten: %+v", res.Rewritten)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("got %d skipped, want 1: %+v", len(res.Skipped), res.Skipped)
	}
	occ := res.Skipped[0]
	if occ.Document != "ACT_Find" || occ.Constraint != in || occ.TargetEntity != "Sales.Person" {
		t.Errorf("the report does not locate the constraint: %+v", occ)
	}
	if len(s.written) != 0 {
		t.Errorf("the unit was written anyway: %v", s.written)
	}
}

// TestScanReportsConstraintWithNoAnchor pins that a constraint whose target
// entity cannot be determined at all is reported rather than assumed to be ours.
func TestScanReportsConstraintWithNoAnchor(t *testing.T) {
	s := &memStore{units: []*types.RawUnit{
		unit("w-1", "Forms$Page", bson.D{
			{Key: "Name", Value: "Orphan"},
			{Key: "XPathConstraint", Value: "[FirstName = 'Ada']"},
		}),
	}}

	res, err := renameIn(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 0 {
		t.Errorf("an unanchored constraint was rewritten: %+v", res.Rewritten)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("got %d skipped, want 1: %+v", len(res.Skipped), res.Skipped)
	}
}

// TestScanPreservesTheRestOfTheDocument pins that only the constraint changes:
// the round trip through bson.D must not reorder or drop anything else.
func TestScanPreservesTheRestOfTheDocument(t *testing.T) {
	s := &memStore{units: []*types.RawUnit{
		retrieveUnit("mf-1", "ACT_Find", "Sales.Person", "[FirstName = 'Ada']"),
	}}
	before := s.units[0].Contents

	if _, err := renameIn(s); err != nil {
		t.Fatal(err)
	}

	var got, want bson.D
	if err := bson.Unmarshal(s.written["mf-1"], &got); err != nil {
		t.Fatal(err)
	}
	if err := bson.Unmarshal(before, &want); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("the document gained or lost top-level fields: %d vs %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Key != want[i].Key {
			t.Errorf("field %d is %q, want %q — the field order changed", i, got[i].Key, want[i].Key)
		}
	}
	if n, _ := got[0].Value.(string); n != "ACT_Find" {
		t.Errorf("the document name changed to %q", n)
	}
}
