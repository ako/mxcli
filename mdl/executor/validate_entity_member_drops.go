// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// entityMemberDropRule is the check-time counterpart of the exec-time warning in
// execCreateEntity: a CREATE OR MODIFY ENTITY rebuilds the entity from the
// statement alone, so every member the statement omits is deleted.
const entityMemberDropRule = "MDL087"

// entityMemberSet is the name-level member inventory of an entity — the named
// attributes, the four audit system fields and the generalization.
//
// It exists so the exec-time warning and the check-time one share ONE answer to
// "what counts as a member". The two run at different layers and in different
// currencies (exec holds two *domainmodel.Entity, check holds a stored entity
// and an AST statement), and a second hand-written comparison is how the audit
// fields came to be reported by one and not the other.
type entityMemberSet struct {
	// attrs maps the lower-cased attribute name to the name as written, because
	// Mendix attribute names are matched case-insensitively but reported as
	// spelled.
	attrs map[string]string
	// order is the lower-cased keys in insertion order. droppedMembers reports
	// in it, so the list reads in the entity's own attribute order — the same
	// order `describe entity` prints — rather than alphabetically, and does not
	// depend on map iteration.
	order          []string
	hasOwner       bool
	hasChangedBy   bool
	hasCreatedDate bool
	hasChangedDate bool
	generalization string
}

func newEntityMemberSet() *entityMemberSet {
	return &entityMemberSet{attrs: map[string]string{}}
}

func (m *entityMemberSet) addAttr(name string) {
	key := strings.ToLower(name)
	if _, seen := m.attrs[key]; !seen {
		m.order = append(m.order, key)
	}
	m.attrs[key] = name
}

// removeAttr drops the attribute but LEAVES its key in order, so a name that is
// removed and re-added keeps its original position. order is only ever read
// through attrs, so a stale key is skipped.
func (m *entityMemberSet) removeAttr(name string) {
	delete(m.attrs, strings.ToLower(name))
}

func (m *entityMemberSet) hasAttr(name string) bool {
	_, ok := m.attrs[strings.ToLower(name)]
	return ok
}

// applyAuditPseudoType records an AutoOwner/AutoChangedBy/AutoCreatedDate/
// AutoChangedDate pseudo-type as the entity flag it becomes, and reports whether
// the type was one. A pseudo-type is not an attribute: execCreateEntity `continue`s
// past it, so counting it as one would make a faithful restatement look like a drop.
func (m *entityMemberSet) applyAuditPseudoType(kind ast.DataTypeKind) bool {
	switch kind {
	case ast.TypeAutoOwner:
		m.hasOwner = true
	case ast.TypeAutoChangedBy:
		m.hasChangedBy = true
	case ast.TypeAutoCreatedDate:
		m.hasCreatedDate = true
	case ast.TypeAutoChangedDate:
		m.hasChangedDate = true
	default:
		return false
	}
	return true
}

// memberSetFromEntity reads the inventory off a stored (or rebuilt) entity.
func memberSetFromEntity(e *domainmodel.Entity) *entityMemberSet {
	m := newEntityMemberSet()
	if e == nil {
		return m
	}
	for _, a := range e.Attributes {
		m.addAttr(a.Name)
	}
	m.hasOwner = e.HasOwner
	m.hasChangedBy = e.HasChangedBy
	m.hasCreatedDate = e.HasCreatedDate
	m.hasChangedDate = e.HasChangedDate
	m.generalization = e.GeneralizationRef
	return m
}

// memberSetFromCreateStmt reads the inventory a CREATE [OR MODIFY] ENTITY
// statement declares — i.e. exactly what the rebuilt entity will hold.
func memberSetFromCreateStmt(s *ast.CreateEntityStmt) *entityMemberSet {
	m := newEntityMemberSet()
	if s == nil {
		return m
	}
	for _, a := range s.Attributes {
		if m.applyAuditPseudoType(a.Type.Kind) {
			continue
		}
		m.addAttr(a.Name)
	}
	if s.Generalization != nil {
		m.generalization = s.Generalization.String()
	}
	return m
}

// droppedMembers reports the members present in before and absent from after —
// what a rebuild would delete. Attribute names are compared case-insensitively
// and reported as stored.
func droppedMembers(before, after *entityMemberSet) []string {
	if before == nil {
		return nil
	}
	if after == nil {
		after = newEntityMemberSet()
	}
	var dropped []string
	for _, lower := range before.order {
		name, present := before.attrs[lower]
		if !present {
			continue
		}
		if _, kept := after.attrs[lower]; !kept {
			dropped = append(dropped, name)
		}
	}
	// Audit system fields that were enabled and are no longer requested are also
	// removed by the replace.
	if before.hasOwner && !after.hasOwner {
		dropped = append(dropped, "owner (system field)")
	}
	if before.hasChangedBy && !after.hasChangedBy {
		dropped = append(dropped, "changedBy (system field)")
	}
	if before.hasCreatedDate && !after.hasCreatedDate {
		dropped = append(dropped, "createdDate (system field)")
	}
	if before.hasChangedDate && !after.hasChangedDate {
		dropped = append(dropped, "changedDate (system field)")
	}
	// An omitted EXTENDS un-inherits the entity, which is a bigger change than a
	// dropped attribute and was the only one of these that happened in silence.
	// It is reported rather than preserved because there is no "extends nothing"
	// spelling, so preserving it would make an inheritance impossible to remove.
	if before.generalization != "" && after.generalization == "" {
		dropped = append(dropped, "extends "+before.generalization+" (generalization)")
	}
	return dropped
}

// CheckEntityMemberDrops reports, per entity, the members this script removes
// from the connected project without having been asked to — MDL087.
//
// It is the check-time half of ako/mxcli#562: exec already prints the same list,
// but only as it applies the statement, by which point the attribute is gone and
// the loss surfaces slices later as a CE1613 on whatever still binds it.
//
// Two things make this a NET computation over the whole script rather than a
// per-statement one, unlike the exec-time warning:
//
//  1. Re-adding is idiomatic. A domain script that rebuilds an entity and then
//     `alter entity … add attribute` the members whose microflows did not exist
//     yet loses nothing, and warning on it would train people to ignore the rule.
//  2. An explicit removal is not a surprise. `drop attribute`, `rename
//     attribute` and `drop entity` say what they do, so they are excluded by
//     intent rather than by outcome — a pure before/after diff cannot tell them
//     from an accident.
//
// The warning is therefore about the one case the reporter hit: a member the
// project holds, that this script does not restate and never asks to remove.
//
// Severity is a warning, not an error. "Modify to this shape" is a legitimate
// statement of intent and the user may well mean it; the defect was that it
// happened in silence.
func (e *Executor) CheckEntityMemberDrops(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	return CheckEntityMemberDrops(e.newExecContext(context.Background()), prog)
}

// entityDropState is one tracked entity's simulated member set plus the members
// this script explicitly asked to remove.
type entityDropState struct {
	stored *entityMemberSet
	// current is the member set after applying the statements seen so far.
	current *entityMemberSet
	// intentional holds lower-cased attribute names an explicit DROP or RENAME
	// removed, and the audit/generalization sentinels for the same.
	intentional map[string]bool
	// skip marks an entity this script drops outright; a DROP ENTITY is as
	// explicit as a statement gets and its members are not a loss to report.
	skip bool
}

// CheckEntityMemberDrops is the ExecContext-level entry point. A disconnected
// context returns no violations rather than an error: this check is entirely
// about what the PROJECT holds, so without one there is nothing it can say.
func CheckEntityMemberDrops(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || prog == nil || !ctx.Connected() {
		return nil
	}

	states := map[string]*entityDropState{}
	var order []string

	// track returns the state for an entity, loading the stored member set on
	// first use. An entity the project does not hold is tracked with a nil
	// stored set, so nothing it does can be reported as a loss.
	track := func(qn string) *entityDropState {
		if st, ok := states[qn]; ok {
			return st
		}
		st := &entityDropState{intentional: map[string]bool{}}
		if stored, ok := findEntityByQN(ctx.Backend, qn); ok && stored != nil {
			st.stored = memberSetFromEntity(stored)
			st.current = memberSetFromEntity(stored)
		}
		states[qn] = st
		order = append(order, qn)
		return st
	}

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt:
			st := track(s.Name.String())
			if st.stored == nil {
				// Created by this script: a rebuild of something the project does
				// not hold removes nothing from it.
				continue
			}
			// CREATE ENTITY IF NOT EXISTS never touches an existing definition,
			// and a plain CREATE over an existing entity is refused by
			// CheckProjectConflicts — neither rebuilds, so neither drops.
			if s.IfNotExists || !s.CreateOrModify {
				continue
			}
			st.current = memberSetFromCreateStmt(s)
		case *ast.DropEntityStmt:
			track(s.Name.String()).skip = true
		case *ast.AlterEntityStmt:
			applyAlterToDropState(track(s.Name.String()), s)
		case *ast.AlterEntitiesStmt:
			// The bulk form runs each action through the single-entity path, so
			// its removals are just as explicit. Apply them to every entity this
			// pass is already tracking within the statement's module scope; that
			// cannot invent a drop, only cancel one out.
			for _, qn := range order {
				st := states[qn]
				if s.Module != "" && !strings.EqualFold(splitQualifiedName(qn).Module, s.Module) {
					continue
				}
				for _, action := range s.Actions {
					applyAlterToDropState(st, action)
				}
			}
		}
	}

	var out []linter.Violation
	for _, qn := range order {
		st := states[qn]
		if st.skip || st.stored == nil || st.current == nil {
			continue
		}
		var dropped []string
		for _, name := range droppedMembers(st.stored, st.current) {
			if st.intentional[strings.ToLower(memberKey(name))] {
				continue
			}
			dropped = append(dropped, name)
		}
		if len(dropped) == 0 {
			continue
		}
		target := splitQualifiedName(qn)
		out = append(out, linter.Violation{
			RuleID:   entityMemberDropRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf(
				"applying this script to the project removes %d member(s) from entity %s that it does not restate: %s — anything still bound to them (widgets, microflows) fails the build with CE1613",
				len(dropped), qn, strings.Join(dropped, ", ")),
			Suggestion: fmt.Sprintf(
				"if these members are meant to survive, add them to the create or modify statement, or add them incrementally with 'alter entity %s add attribute <name>: <type>;' in this script; if they are meant to go, say so with 'alter entity %s drop attribute <name>;'",
				qn, qn),
			Location: linter.Location{Module: target.Module, DocumentType: "entity", DocumentName: target.Name},
		})
	}
	return out
}

// applyAlterToDropState folds one ALTER ENTITY action into the simulated member
// set. Only the four operations that change MEMBERSHIP are handled; MODIFY
// ATTRIBUTE, indexes, documentation and the rest cannot add or remove a member.
func applyAlterToDropState(st *entityDropState, s *ast.AlterEntityStmt) {
	if st == nil || st.current == nil || st.stored == nil || s == nil {
		return
	}
	switch s.Operation {
	case ast.AlterEntityAddAttribute:
		if s.Attribute == nil {
			return
		}
		if st.current.applyAuditPseudoType(s.Attribute.Type.Kind) {
			return
		}
		st.current.addAttr(s.Attribute.Name)
		// An ADD after a rebuild dropped the same name restores it, so the
		// earlier removal is no longer a loss.
		delete(st.intentional, strings.ToLower(s.Attribute.Name))
	case ast.AlterEntityDropAttribute:
		st.current.removeAttr(s.AttributeName)
		st.intentional[strings.ToLower(s.AttributeName)] = true
	case ast.AlterEntityRenameAttribute:
		if !st.current.hasAttr(s.AttributeName) && !st.stored.hasAttr(s.AttributeName) {
			return
		}
		st.current.removeAttr(s.AttributeName)
		st.intentional[strings.ToLower(s.AttributeName)] = true
		if s.NewName != "" {
			st.current.addAttr(s.NewName)
			delete(st.intentional, strings.ToLower(s.NewName))
		}
	}
}

// memberKey strips the parenthesised annotation droppedMembers adds to the
// non-attribute members ("owner (system field)"), so an intent lookup keyed on a
// bare attribute name cannot accidentally match one.
func memberKey(reported string) string {
	if i := strings.Index(reported, " ("); i > 0 {
		return reported[:i]
	}
	return reported
}
