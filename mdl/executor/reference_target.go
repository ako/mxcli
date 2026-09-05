// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
)

// resolveReferenceTarget returns the spelling of `name` that CATALOG.REFS
// actually stores, and whether it differs from what the user typed.
//
// SHOW REFERENCES TO and SHOW IMPACT OF match TargetName exactly. Every target
// used to be a module-qualified name, which a user copies verbatim from
// `show entities`, so exact matching was right and nothing needed this.
//
// The `widget` edge (slice 5 of PROPOSAL_def_driven_widget_bodies.md) breaks
// that assumption: its TargetName is the widget's MDL name, which is stored
// SHOUTED (COMBOBOX) because that is how widget_definitions_data holds it,
// while MDL keywords are case-insensitive and every example writes them in
// lower case. So the natural
//
//	show references to combobox
//
// found nothing — and reported "(no references found)", which is a WRONG
// answer rather than a missing one. That is the failure mode worth spending a
// lookup to avoid: a user cannot tell it from a widget genuinely being unused.
//
// The fallback is deliberately second, never first. An exact match is returned
// untouched, so no existing answer can change; only a query that would have
// returned nothing gets a second chance. Callers report the resolved spelling
// so the user can see which name was actually matched.
func resolveReferenceTarget(ctx *ExecContext, name string) (resolved string, matchedLoosely bool) {
	if ctx == nil || ctx.Catalog == nil || name == "" {
		return name, false
	}

	exact, err := ctx.Catalog.Query(fmt.Sprintf(
		`SELECT 1 FROM refs WHERE TargetName = '%s' LIMIT 1`, escapeSQLString(name)))
	if err == nil && exact.Count > 0 {
		return name, false
	}

	// Nothing under that spelling. Try case-insensitively, and only accept the
	// answer when it is unambiguous — two targets differing only in case are a
	// question this cannot answer for the user, so leave the exact (empty)
	// result rather than guessing at one of them.
	loose, err := ctx.Catalog.Query(fmt.Sprintf(
		`SELECT DISTINCT TargetName FROM refs WHERE lower(TargetName) = lower('%s')`,
		escapeSQLString(name)))
	if err != nil || loose.Count != 1 || len(loose.Rows) != 1 || len(loose.Rows[0]) == 0 {
		return name, false
	}
	match, ok := loose.Rows[0][0].(string)
	if !ok || match == "" || match == name {
		return name, false
	}
	return match, true
}

// reportResolvedTarget tells the user which stored spelling was matched, when
// it is not the one they typed. Silent on an exact match.
func reportResolvedTarget(ctx *ExecContext, typed, resolved string, matchedLoosely bool) {
	if !matchedLoosely || ctx == nil || ctx.Output == nil {
		return
	}
	fmt.Fprintf(ctx.Output, "(matched %s)\n", strings.TrimSpace(resolved))
}
