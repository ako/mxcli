// SPDX-License-Identifier: Apache-2.0

// Package xpathrefs rewrites attribute names inside stored XPath constraints.
//
// An attribute is named two ways in a Mendix project. Most places store the
// fully qualified name as a string ("Mod.Entity.Attr"), which a rename can find
// by scanning for that string. XPath constraints instead name it as a bare step
// — `[Status = 'Open']` — where the same three letters mean different attributes
// on different entities, and mean nothing at all inside a string literal. A
// rename that scans for "Status" would corrupt every one of those.
//
// This package resolves each bare step to the entity it actually belongs to
// before touching anything. It needs no type inference to do it: a constraint's
// target entity is known structurally (a retrieve names its entity, a widget
// data source names its entity, an access rule belongs to one), and every
// further hop is an association or entity named in the path itself.
//
// The rewrite is deliberately **textual, gated by the parse** rather than a
// re-render of the parsed tree. Re-rendering would rewrite whitespace and
// spelling in constraints that are none of the rename's business, and any
// disagreement between mxcli's parser and its renderer would silently corrupt a
// working constraint. Here the parse only decides *whether* and *how many*; the
// edit itself replaces one identifier token and leaves every other byte alone.
package xpathrefs

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Model answers the two questions walking an XPath path asks. It is deliberately
// smaller than the ModelResolver in PROPOSAL_expression_type_checking: XPath
// needs no attribute types, no enum cases and no return types, which is why this
// half does not wait for that proposal.
type Model interface {
	// IsEntity reports whether qn ("Module.Name") names an entity.
	IsEntity(qn string) bool
	// AssociationTarget returns the entity at the other end of association qn
	// when it is traversed from entity from, and whether qn is an association at
	// all. Both directions resolve: Mendix XPath traverses an association from
	// either end.
	AssociationTarget(qn, from string) (string, bool)
}

// rewriteConstraint returns the constraint with every bare step that resolves to
// entityQN.oldAttr replaced by newAttr.
//
// The second result is false when some part of the constraint names oldAttr but
// could not be shown to mean this entity's attribute — an unparseable predicate
// group, a path rooted in a variable, an association mxcli cannot resolve, or
// the same bare name belonging to two different entities in one group. The
// caller reports those; it must never rewrite them. A wrong rewrite corrupts a
// constraint that was working, which is strictly worse than the honest partial
// rename this package exists to finish.
func rewriteConstraint(constraint, targetEntity, entityQN, oldAttr, newAttr string, m Model) (string, bool) {
	groups := visitor.SplitXPathPredicateGroups(constraint)
	if len(groups) == 0 {
		// Not a bracket-group constraint at all. If it mentions the name we
		// cannot say anything about it, so report rather than guess.
		return constraint, !mentionsIdentifier(constraint, oldAttr)
	}

	out := make([]string, 0, len(groups))
	understood := true
	for _, g := range groups {
		rewritten, ok := rewriteGroup(g, targetEntity, entityQN, oldAttr, newAttr, m)
		if !ok {
			understood = false
		}
		out = append(out, rewritten)
	}
	return strings.Join(out, ""), understood
}

// rewriteGroup rewrites one bracket group.
func rewriteGroup(group, targetEntity, entityQN, oldAttr, newAttr string, m Model) (string, bool) {
	// Cheap exit: a group that never spells the name lexically cannot contain a
	// reference to it, whatever it parses to.
	rewritten, lexical := replaceIdentifier(group, oldAttr, newAttr)
	if lexical == 0 {
		return group, true
	}

	expr, ok := visitor.ParseXPathConstraint(group)
	if !ok || expr == nil {
		// mxcli could not read this group. It mentions the name, so say so —
		// passing it through silently is how a half-rename looks finished.
		return group, false
	}

	w := &walker{model: m, entityQN: entityQN, attr: oldAttr}
	w.walk(expr, targetEntity)

	// The count invariant, and the load-bearing safety check.
	//
	// visitor.ParseXPathConstraint runs with ANTLR's error listeners removed, so
	// it recovers from almost anything and can hand back a tree that quietly
	// omits part of its input — `[LastName = 'L' FirstName]` parses, and the
	// trailing step is simply not in the tree. Resolution then never sees that
	// occurrence, while a lexical edit would rewrite it regardless. Requiring the
	// parser and the editor to agree on how many times the name occurs is what
	// makes a lenient parse safe to lean on: any disagreement means the two are
	// looking at different text, so neither is trusted.
	if lexical != w.hits+w.others+w.unresolved {
		return group, false
	}

	switch {
	case w.unresolved > 0:
		// Somewhere the name is spelled and the walk could not say whose
		// attribute it is. Never rewrite on a guess — and never stay quiet
		// either, because silence is how a half-rename looks finished.
		return group, false
	case w.hits == 0:
		// The name occurs, but always as something else: a string literal, or
		// another entity's attribute of the same name. That is a definite answer,
		// so it is left alone and not reported.
		return group, true
	case w.others > 0:
		// One bare name means this entity's attribute in one place and a
		// different entity's in another. A token-level edit cannot tell them
		// apart, and re-rendering to fix that is the thing this package refuses
		// to do.
		return group, false
	}
	return rewritten, true
}

// walker counts occurrences of one attribute name, split three ways by what the
// walk could establish about each.
type walker struct {
	model    Model
	entityQN string
	attr     string
	// hits are occurrences shown to be the renamed entity's attribute.
	hits int
	// others are occurrences shown to be a *different* entity's attribute — a
	// definite answer, and a reason to leave them alone rather than to complain.
	others int
	// unresolved are occurrences whose owning entity could not be established.
	unresolved int
}

// note records one bare identifier seen in the context of entity cur. An empty
// cur means the walk lost track of the entity, which is unresolved rather than
// "someone else's": "we do not know" must block the rewrite, not permit it.
func (w *walker) note(name, cur string) {
	if name != w.attr {
		return
	}
	switch {
	case cur == "":
		w.unresolved++
	case cur == w.entityQN:
		w.hits++
	default:
		w.others++
	}
}

// walk visits expr with cur as the entity its bare steps are evaluated against.
func (w *walker) walk(expr ast.Expression, cur string) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.IdentifierExpr:
		w.note(e.Name, cur)
	case *ast.XPathPathExpr:
		w.walkPath(e.Steps, cur)
	case *ast.BinaryExpr:
		w.walk(e.Left, cur)
		w.walk(e.Right, cur)
	case *ast.UnaryExpr:
		w.walk(e.Operand, cur)
	case *ast.ParenExpr:
		w.walk(e.Inner, cur)
	case *ast.FunctionCallExpr:
		for _, a := range e.Arguments {
			w.walk(a, cur)
		}
	case *ast.IfThenElseExpr:
		w.walk(e.Condition, cur)
		w.walk(e.ThenExpr, cur)
		w.walk(e.ElseExpr, cur)
	case *ast.SourceExpr:
		w.walk(e.Expression, cur)
	}
	// Literals, variables, qualified names, tokens and constant refs name no
	// bare attribute, so they need no visit.
}

// walkPath follows a path step by step, carrying the entity each step lands on.
//
// The entity is never inferred: an association hop resolves through the model,
// an explicit entity step names itself, and anything else (a variable root, an
// unknown qualified name) sets the entity to unknown so every later step counts
// as a conflict.
func (w *walker) walkPath(steps []ast.XPathStep, cur string) {
	for i, st := range steps {
		next := ""
		switch e := st.Expr.(type) {
		case *ast.IdentifierExpr:
			if i == len(steps)-1 {
				// The terminal bare step is an attribute of the current entity.
				w.note(e.Name, cur)
			} else if e.Name == w.attr {
				// A bare non-terminal step is not something this package models
				// (Mendix spells the intermediate entity out). Only complain when
				// it is the name we are renaming.
				w.unresolved++
			}
		case *ast.QualifiedNameExpr:
			qn := e.QualifiedName.String()
			if t, ok := w.model.AssociationTarget(qn, cur); ok {
				next = t
			} else if w.model.IsEntity(qn) {
				next = qn
			}
		}
		if st.Predicate != nil {
			// A predicate constrains what the step reached, so it is evaluated
			// against that entity — not the one the step started from.
			w.walk(st.Predicate, next)
		}
		cur = next
	}
}

// replaceIdentifier replaces whole-token occurrences of old with new, returning
// the result and how many it replaced.
//
// Single-quoted string literals are skipped: `[Name = 'Name']` has exactly one
// identifier in it. A run that is part of a qualified name (touching a dot on
// either side) is not a bare step and is skipped too.
func replaceIdentifier(s, old, new string) (string, int) {
	var b strings.Builder
	b.Grow(len(s))
	count := 0

	inString := false
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\'' {
			inString = !inString
			b.WriteByte(c)
			i++
			continue
		}
		if inString || !isIdentStart(c) {
			b.WriteByte(c)
			i++
			continue
		}
		j := i
		for j < len(s) && isIdentChar(s[j]) {
			j++
		}
		run := s[i:j]
		if run == old && !touchesDot(s, i, j) {
			b.WriteString(new)
			count++
		} else {
			b.WriteString(run)
		}
		i = j
	}
	return b.String(), count
}

// mentionsIdentifier reports whether s contains name as a bare token outside a
// string literal.
func mentionsIdentifier(s, name string) bool {
	_, n := replaceIdentifier(s, name, name)
	return n > 0
}

// touchesDot reports whether the run s[i:j] is glued to a dot on either side,
// which makes it part of a qualified name rather than a bare step.
func touchesDot(s string, i, j int) bool {
	if i > 0 && s[i-1] == '.' {
		return true
	}
	if j < len(s) && s[j] == '.' {
		return true
	}
	return false
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}
