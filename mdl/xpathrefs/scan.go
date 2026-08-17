// SPDX-License-Identifier: Apache-2.0

package xpathrefs

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// Store is the raw-unit surface the rewrite needs. Both engines' backends
// already provide both methods (backend.RawUnitBackend), so this needs no new
// backend interface method — and writes still go through the writer's single
// choke point, so ADR-0008 elision and identity preservation apply unchanged.
type Store interface {
	ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error)
	UpdateRawUnit(unitID string, contents []byte) error
}

// Occurrence is one stored XPath constraint that named the attribute.
type Occurrence struct {
	UnitID       string // the document's unit ID
	UnitType     string // e.g. "Microflows$Microflow"
	Document     string // the document's Name, when it has one
	TargetEntity string // the entity the constraint is evaluated against ("" if unknown)
	Constraint   string // the constraint as stored, before any rewrite
}

// Result reports what the rewrite did and, just as importantly, what it refused
// to do.
type Result struct {
	// Rewritten lists the constraints that changed.
	Rewritten []Occurrence
	// Skipped lists constraints that mention the attribute but could not be
	// shown to mean it — an unreadable predicate group, a path rooted in a
	// variable, an unresolvable association, or one bare name meaning two
	// different entities' attributes. These are reported, never guessed at.
	Skipped []Occurrence
	// Units is how many documents were written.
	Units int
}

// Total returns how many constraints were rewritten.
func (r Result) Total() int { return len(r.Rewritten) }

// RenameAttribute rewrites every stored XPath constraint that names
// entityQN.oldAttr so it names newAttr instead.
//
// domainModelUnitID and entityName identify the renamed entity's own domain
// model unit, which is how an access rule's constraint is attributed to its
// entity: an access rule holds no entity reference of its own, it simply lives
// inside one. Passing the unit ID keeps this package independent of the
// container hierarchy.
func RenameAttribute(s Store, m Model, entityQN, domainModelUnitID, entityName, oldAttr, newAttr string) (Result, error) {
	var res Result
	if oldAttr == "" || oldAttr == newAttr {
		return res, nil
	}

	units, err := s.ListRawUnitsByType("")
	if err != nil {
		return res, fmt.Errorf("listing units: %w", err)
	}

	for _, u := range units {
		if len(u.Contents) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			continue
		}

		sc := &scanner{
			model: m, entityQN: entityQN, oldAttr: oldAttr, newAttr: newAttr,
			unitID: string(u.ID), unitType: u.Type, document: docName(doc),
			ownEntity: entityName,
		}
		sc.inOwnDomainModel = string(u.ID) == domainModelUnitID
		sc.walkDoc(doc, "")

		res.Rewritten = append(res.Rewritten, sc.rewritten...)
		res.Skipped = append(res.Skipped, sc.skipped...)

		if !sc.changed {
			continue
		}
		contents, err := bson.Marshal(doc)
		if err != nil {
			return res, fmt.Errorf("re-encoding %s: %w", u.ID, err)
		}
		if err := s.UpdateRawUnit(string(u.ID), contents); err != nil {
			return res, fmt.Errorf("writing %s: %w", u.ID, err)
		}
		res.Units++
	}

	return res, nil
}

// scanner walks one unit's BSON, rewriting constraint values in place.
type scanner struct {
	model    Model
	entityQN string
	oldAttr  string
	newAttr  string

	unitID   string
	unitType string
	document string

	// ownEntity is the renamed entity's simple name, and inOwnDomainModel says
	// whether this unit is the domain model holding it. Together they attribute
	// an access rule's constraint to the entity that contains it.
	ownEntity        string
	inOwnDomainModel bool

	changed   bool
	rewritten []Occurrence
	skipped   []Occurrence
}

// Mendix spells the key two ways: the microflow retrieve source stores
// "XpathConstraint", everything else "XPathConstraint".
var constraintKeys = []string{"XPathConstraint", "XpathConstraint"}

// walkDoc visits one BSON document. enclosing is the qualified name of the
// entity this node sits inside, or "" — it is what gives an access rule its
// target, since the rule itself names no entity.
func (s *scanner) walkDoc(doc bson.D, enclosing string) {
	if s.inOwnDomainModel && isEntityNode(doc, s.ownEntity) {
		enclosing = s.entityQN
	}

	for i := range doc {
		for _, key := range constraintKeys {
			if doc[i].Key != key {
				continue
			}
			raw, ok := doc[i].Value.(string)
			if !ok || raw == "" {
				continue
			}
			if v, changed := s.rewriteAt(raw, targetEntityOf(doc, enclosing)); changed {
				doc[i].Value = v
				s.changed = true
			}
		}
		s.walkValue(doc[i].Value, enclosing)
	}
}

// rewriteAt applies the rewrite to one constraint and records the outcome.
func (s *scanner) rewriteAt(raw, target string) (string, bool) {
	occ := Occurrence{
		UnitID: s.unitID, UnitType: s.unitType, Document: s.document,
		TargetEntity: target, Constraint: raw,
	}
	if target == "" {
		// Nothing anchors this constraint, so no bare step in it can be
		// resolved. Report it only if it spells the name at all.
		if mentionsIdentifier(raw, s.oldAttr) {
			s.skipped = append(s.skipped, occ)
		}
		return raw, false
	}

	out, understood := rewriteConstraint(raw, target, s.entityQN, s.oldAttr, s.newAttr, s.model)
	if !understood {
		s.skipped = append(s.skipped, occ)
	}
	if out == raw {
		return raw, false
	}
	s.rewritten = append(s.rewritten, occ)
	return out, true
}

func (s *scanner) walkValue(v any, enclosing string) {
	switch val := v.(type) {
	case bson.D:
		s.walkDoc(val, enclosing)
	case bson.A:
		for _, item := range val {
			s.walkValue(item, enclosing)
		}
	case []any:
		for _, item := range val {
			s.walkValue(item, enclosing)
		}
	}
}

// targetEntityOf reads the entity a constraint is evaluated against from the
// node that carries it, falling back to the entity the node sits inside.
//
//   - "Entity" (a qualified name string) — Microflows$DatabaseRetrieveSource
//   - "EntityRef".QualifiedName — the page/widget XPath data sources
//   - enclosing — DomainModels$AccessRule, which carries no entity of its own
func targetEntityOf(doc bson.D, enclosing string) string {
	for _, e := range doc {
		switch e.Key {
		case "Entity":
			if s, ok := e.Value.(string); ok && s != "" {
				return s
			}
		case "EntityRef":
			if ref, ok := e.Value.(bson.D); ok {
				for _, r := range ref {
					if r.Key == "QualifiedName" {
						if s, ok := r.Value.(string); ok && s != "" {
							return s
						}
					}
				}
			}
		}
	}
	return enclosing
}

// entityNodeTypes are the $Type values a domain model entity node is stored
// under.
//
// "DomainModels$EntityImpl" is what is actually on disk — measured on a Mendix
// 11.13.0 project, both engines write it. "DomainModels$Entity" is the name the
// metamodel uses for an entity *reference target* (see the ref registries in
// modelsdk/gen/domainmodels), and checking for it alone silently matched nothing:
// every access rule was then reported as unanchored rather than rewritten, which
// looks like a cautious refusal instead of a bug.
var entityNodeTypes = map[string]bool{
	"DomainModels$EntityImpl": true,
	"DomainModels$Entity":     true,
}

// isEntityNode reports whether doc is the domain model entity called name.
func isEntityNode(doc bson.D, name string) bool {
	if name == "" {
		return false
	}
	typ, _ := lookup(doc, "$Type").(string)
	if !entityNodeTypes[typ] {
		return false
	}
	n, _ := lookup(doc, "Name").(string)
	return n == name
}

func docName(doc bson.D) string {
	n, _ := lookup(doc, "Name").(string)
	return n
}

func lookup(doc bson.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}
