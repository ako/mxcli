// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// Which entity a pluggable widget's attribute property binds against — the ONE
// rule `exec` writes by and `check` judges by (ako/mxcli#647).
//
// Studio Pro's rule is the widget's own. widget.xml's `dataSource="…"` on a
// property names the datasource whose ITEMS it binds to; a property WITHOUT one
// binds to the widget's context object, which is the nearest enclosing data
// container's entity. The engine used to answer "whichever datasource mapping
// ran last" for the second kind — and, in its explicit-key pass, for the first
// kind as well — so on mxcli-demo-2's VoxelViewer (two datasources, inside a
// data view) every unqualified attribute, the data view's own included, was
// written against the LAST datasource's entity: check clean, ten CE1613 in
// mx check.
//
// Keeping the rule here, as a function of plain data, is what lets the check
// share it: the executor feeds it the engine's live state, the validator the
// statically-known one, and neither re-derives it (see
// docs-wiki/bug-patterns/duplicate-resolver-drift.md).

// bindingScopeKind says where a binding's entity came from.
type bindingScopeKind int

const (
	// scopeShared: nothing decided it — the template does not declare the
	// property, the linked datasource did not resolve, or there is no enclosing
	// context. The caller's shared context is used, which is the behaviour
	// before #647; nothing is refused on it.
	scopeShared bindingScopeKind = iota
	// scopeDataSource: the property is linked to one of the widget's own
	// datasources (widget.xml `dataSource="…"`) and binds to its items.
	scopeDataSource
	// scopeEnclosing: the property is linked to no datasource and binds to the
	// context object — the enclosing data container's entity.
	scopeEnclosing
)

// bindingScope is the entity a property binds against, and why.
type bindingScope struct {
	Entity     string
	Kind       bindingScopeKind
	DataSource string // the datasource property key, for scopeDataSource
}

// resolveBindingScope applies the rule.
//
//	link       the property's DataSourceProperty ("" when linked to none)
//	declared   whether the widget's template declares the property at all
//	outer      the enclosing data container's entity ("" when none, or unknown)
//	dsEntities the entity each of the widget's datasources resolved to, by key
//	shared     the fallback context
func resolveBindingScope(link string, declared bool, outer string, dsEntities map[string]string, shared string) bindingScope {
	if declared {
		if link != "" {
			if entity := dsEntities[link]; entity != "" {
				return bindingScope{Entity: entity, Kind: scopeDataSource, DataSource: link}
			}
		} else if outer != "" {
			return bindingScope{Entity: outer, Kind: scopeEnclosing}
		}
	}
	return bindingScope{Entity: shared, Kind: scopeShared}
}

// describe renders where the scope came from, for a message.
func (s bindingScope) describe() string {
	switch s.Kind {
	case scopeDataSource:
		return fmt.Sprintf("the widget links it to its datasource `%s`", s.DataSource)
	case scopeEnclosing:
		return "the widget links it to no datasource, so it binds to the enclosing data container"
	}
	return ""
}

// attributeIndex answers "does this entity have this attribute" over a domain
// model: each entity's own attribute names (lower-cased) and its parent.
type attributeIndex struct {
	owners  map[string]map[string]bool
	parents map[string]string
}

// declares reports whether entityQN — or an entity it specializes — declares
// attr. Case-insensitive, as the page builder's qualification is. known is false
// when the question could not be answered: an entity the index does not hold,
// anywhere on the chain, before a match.
func (ix attributeIndex) declares(entityQN, attr string) (found, known bool) {
	if ix.owners == nil || entityQN == "" || attr == "" {
		return false, false
	}
	lower := strings.ToLower(attr)
	seen := map[string]bool{}
	for cur := entityQN; cur != ""; cur = ix.parents[cur] {
		if seen[cur] {
			break
		}
		seen[cur] = true
		attrs, ok := ix.owners[cur]
		if !ok {
			return false, false
		}
		if attrs[lower] {
			return true, true
		}
	}
	return false, true
}

// misboundAttributeError refuses an unqualified attribute name that is NOT an
// attribute of the entity its property binds to, while it IS one of another
// entity in the widget's scope — the enclosing data container's, or one of the
// widget's own datasources'.
//
// That is the author meaning a different scope from the one the widget gives
// the property, and writing it can only produce CE1613 "The selected attribute
// … no longer exists". The candidates are named because they are the fix: the
// author either picks an attribute of the bound entity, or binds the value to
// the property the widget links to the other datasource.
//
// A name no entity in scope has is refused too, without candidates. It was let
// through as "a plain typo, which other checks own", and no check owned it: exec
// wrote Entity.Missing and the first word of it was mx check's CE1613
// (mendixlabs/mxcli#1591).
//
// A qualified name is judged by foreignQualifiedAttributeError.
//
// Nothing is refused where the question cannot be answered: a path, a scope the
// rule did not decide (scopeShared), or a model the index cannot read.
func misboundAttributeError(where, attr string, scope bindingScope, outer string, dsEntities map[string]string, ix attributeIndex) error {
	if attr == "" || strings.ContainsAny(attr, "/$") || scope.Kind == scopeShared || scope.Entity == "" {
		return nil
	}
	if strings.Contains(attr, ".") {
		return foreignQualifiedAttributeError(where, attr, scope, ix)
	}
	if found, known := ix.declares(scope.Entity, attr); found || !known {
		return nil
	}
	var candidates []string
	seen := map[string]bool{scope.Entity: true}
	add := func(entity, label string) {
		if entity == "" || seen[entity] {
			return
		}
		if found, _ := ix.declares(entity, attr); found {
			seen[entity] = true
			candidates = append(candidates, fmt.Sprintf("%s (%s)", entity, label))
		}
	}
	add(outer, "the enclosing data container")
	keys := make([]string, 0, len(dsEntities))
	for k := range dsEntities {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add(dsEntities[k], fmt.Sprintf("datasource `%s`", k))
	}
	if len(candidates) == 0 {
		return mdlerrors.NewValidationf(
			"%s: `%s` is not an attribute of %s, the entity this property binds to (%s), "+
				"and writing it is CE1613 \"The selected attribute no longer exists\". Bind an attribute of %s here",
			where, attr, scope.Entity, scope.describe(), scope.Entity)
	}
	return mdlerrors.NewValidationf(
		"%s: `%s` is not an attribute of %s, the entity this property binds to (%s); "+
			"it is an attribute of %s. Bind an attribute of %s here, or give the value to the property "+
			"the widget links to that entity",
		where, attr, scope.Entity, scope.describe(), strings.Join(candidates, " and "), scope.Entity)
}

// foreignQualifiedAttributeError refuses a qualified attribute whose entity is
// neither the one the property binds to nor a generalization of it. The writer
// stores a qualified name as written, with no SourceVariable, and Mendix reads
// it against the bound object: mx check CE7247 "Attribute
// 'MyFirstModule.ChartContext.EventData' is not an attribute of entity
// 'MyFirstModule.ChartEntry'" (mendixlabs/mxcli#1591, 11.12.5). An attribute of
// another object is read through a parameter, `$Param.Attr` — what Studio Pro
// stores for that binding.
//
// An inherited attribute is qualified with the entity that declares it, which is
// on the bound entity's generalization chain, so it passes. A chain the index
// cannot follow, or an entity it does not hold, is not judged.
func foreignQualifiedAttributeError(where, attr string, scope bindingScope, ix attributeIndex) error {
	owner := attributeEntity(attr)
	if strings.Count(attr, ".") != 2 || ix.owners == nil {
		return nil
	}
	if _, held := ix.owners[owner]; !held {
		return nil
	}
	seen := map[string]bool{}
	for cur := scope.Entity; cur != "" && !seen[cur]; cur = ix.parents[cur] {
		if strings.EqualFold(cur, owner) {
			return nil
		}
		if _, held := ix.owners[cur]; !held {
			return nil
		}
		seen[cur] = true
	}
	return mdlerrors.NewValidationf(
		"%s: `%s` is an attribute of %s, not of %s, the entity this property binds to (%s), "+
			"and writing it is CE7247 \"… is not an attribute of entity %s\". Bind an attribute of %s here, "+
			"or read the other object through a page or snippet parameter: `$Param.%s`",
		where, attr, owner, scope.Entity, scope.describe(), scope.Entity, scope.Entity, shortAttributeName(attr))
}
