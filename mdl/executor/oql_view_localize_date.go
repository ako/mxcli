// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// oqlSourceAttrRefRe matches an `alias.Attr` or `alias/Attr` reference in a
// select expression as written, either half possibly quoted. oqlColumnRefRe
// works on the lower-cased form and knows neither the path nor the quotes.
var oqlSourceAttrRefRe = regexp.MustCompile(`(` + oqlIdent + `)\s*[./]\s*(` + oqlIdent + `)`)

// viewDateTimeLocalize derives, per declared DateTime view attribute, the
// LocalizeDate its OQL column carries from the source attribute(s) it reads.
//
// A view attribute needs no `localized` / `not localized` clause (#1373) — a
// stated one wins over this derivation, and describe never prints one on a
// view: the column's localization is a property of the query, and Mendix requires the
// attribute to match it — written localized over a non-localized source, mx
// check reports CE6770 "View Entity is out of sync with the OQL Query"
// (mendixlabs/mxcli#1297). Measured on mxbuild 11.12.5, the source's flag
// carries through a pass-through `s.D`, a `s/D` path, MIN, MAX and a CASE over
// the column alike, so every DateTime source attribute the expression references
// is collected. Only a column whose sources all agree is derived; one with no
// DateTime source, or with sources that disagree, is left out of the result and
// keeps Mendix's default, since neither case has been measured.
func viewDateTimeLocalize(ctx *ExecContext, oql string, attrs []ast.ViewAttribute) map[string]bool {
	oql = stripOQLComments(oql)
	clause := extractSelectClause(oql)
	if clause == "" {
		return nil
	}
	aliasMap := extractAliasMap(oql)

	dateTimeAttrs := make(map[string]string, len(attrs)) // lower-cased → declared name
	for _, a := range attrs {
		if a.Type.Kind == ast.TypeDateTime {
			dateTimeAttrs[strings.ToLower(a.Name)] = a.Name
		}
	}
	if len(dateTimeAttrs) == 0 {
		return nil
	}

	entities := map[string]*domainmodel.Entity{}
	sourceLocalize := func(qualified, attrName string) (bool, bool) {
		ent, seen := entities[qualified]
		if !seen {
			if mod, name, ok := strings.Cut(qualified, "."); ok {
				ent, _ = findEntity(ctx, mod, name)
			}
			entities[qualified] = ent
		}
		if ent == nil {
			return false, false
		}
		for _, a := range ent.Attributes {
			if a.Name == attrName {
				if dt, ok := a.Type.(*domainmodel.DateTimeAttributeType); ok {
					return dt.LocalizeDate, true
				}
				return false, false
			}
		}
		return false, false
	}

	result := map[string]bool{}
	for i, expr := range attributeSelectColumns(oql, parseSelectColumns(clause)) {
		name := ""
		if i < len(attrs) {
			name = attrs[i].Name
		}
		if m := oqlAliasSuffixRe.FindStringSubmatch(expr); m != nil {
			name = unquoteOQLIdent(m[1])
			expr = strings.TrimSuffix(expr, m[0])
		}
		declared, ok := dateTimeAttrs[strings.ToLower(name)]
		if !ok {
			continue
		}

		var localize, found, conflict bool
		for _, ref := range oqlSourceAttrRefRe.FindAllStringSubmatch(oqlStringLiteralRe.ReplaceAllString(expr, "''"), -1) {
			entity, ok := aliasMap[unquoteOQLIdent(ref[1])]
			if !ok {
				continue
			}
			l, ok := sourceLocalize(entity, unquoteOQLIdent(ref[2]))
			if !ok {
				continue
			}
			if found && l != localize {
				conflict = true
			}
			localize, found = l, true
		}
		if found && !conflict {
			result[declared] = localize
		}
	}
	return result
}
