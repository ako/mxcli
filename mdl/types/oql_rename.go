// SPDX-License-Identifier: Apache-2.0

package types

import (
	"regexp"
	"strings"
)

// RewriteOQLQualifiedName rewrites every reference to oldQN in a view entity's
// OQL, returning the new query and how many references were replaced.
//
// It exists because a view's OQL is the one place a qualified name is stored
// EMBEDDED IN A SENTENCE rather than as a property of its own. The rename
// walkers in both engines match a string that either equals the qualified name
// or begins with it — right for a BY_NAME property, and blind to
//
//	select p.Label as Label, sum(p.Amount) as Total
//	from MyFirstModule.Period as p
//	group by p.Label
//
// where `MyFirstModule.Period` sits in the middle. So `rename entity
// MyFirstModule.Period to Timeframe` renamed the entity, reported no references
// updated, and left the view pointing at a name that no longer exists:
//
//	CE0174 "Cannot resolve object name 'MyFirstModule.Period'."
//
// The rename is deliberately NOT a plain substring replace. Two things it must
// not do, both measured as real spellings a view can contain:
//
//  1. Rewrite a LONGER name that starts with the old one — renaming `M.Period`
//     must leave `M.PeriodDetail` alone. Hence the trailing word boundary.
//  2. Lose the QUOTING of a name that carried it. `from M."Year" as y` is how a
//     view refers to an entity whose name is an OQL reserved word (quoting is
//     valid and required there), so the replacement re-emits whichever spelling
//     it found rather than normalising to bare.
//
// Scope is the OQL string only. A blanket substring rewrite across every string
// in the document would reach documentation text and expressions, where a name
// that merely looks similar is not a reference — a much larger blast radius than
// the bug being fixed.
func RewriteOQLQualifiedName(oql, oldQN, newQN string) (string, int) {
	// A MODULE rename arrives as a prefix pair ("Old." -> "New."), not as a
	// qualified name — execRenameModule calls RenameReferences that way so a
	// whole-string match can rewrite any name beginning with the module. The
	// prefix form has no entity half to split, so it needs its own path; without
	// it a module rename left every view's OQL pointing at the old module and
	// mxbuild reported CE0174 "Cannot resolve object name".
	if strings.HasSuffix(oldQN, ".") && strings.HasSuffix(newQN, ".") {
		return rewriteOQLModulePrefix(oql,
			strings.TrimSuffix(oldQN, "."), strings.TrimSuffix(newQN, "."))
	}

	oldModule, oldName, ok := splitQualifiedName(oldQN)
	if !ok {
		return oql, 0
	}
	newModule, newName, ok := splitQualifiedName(newQN)
	if !ok {
		return oql, 0
	}

	// Either half may be written bare, double-quoted or backtick-quoted, and the
	// two halves are quoted independently (`M."Year"` is the common one).
	re := regexp.MustCompile(
		`(` + identAlternation(oldModule) + `)\s*\.\s*(` + identAlternation(oldName) + `)`)

	count := 0
	out := re.ReplaceAllStringFunc(oql, func(m string) string {
		sub := re.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		count++
		return requote(sub[1], newModule) + "." + requote(sub[2], newName)
	})
	return out, count
}

// identAlternation matches one identifier in any of its three spellings. The
// quoted forms come first so the regex prefers them over the bare one, and the
// bare form ends in \b so `Period` does not match inside `PeriodDetail`.
func identAlternation(name string) string {
	q := regexp.QuoteMeta(name)
	return `"` + q + `"|` + "`" + q + "`" + `|` + q + `\b`
}

// requote gives replacement the same quoting the matched text carried, so a
// name that had to be quoted (an OQL reserved word) stays quoted.
func requote(matched, replacement string) string {
	if len(matched) >= 2 {
		switch matched[0] {
		case '"':
			return `"` + replacement + `"`
		case '`':
			return "`" + replacement + "`"
		}
	}
	return replacement
}

func splitQualifiedName(qn string) (module, name string, ok bool) {
	i := strings.Index(qn, ".")
	if i <= 0 || i == len(qn)-1 {
		return "", "", false
	}
	return qn[:i], qn[i+1:], true
}

// rewriteOQLModulePrefix rewrites the MODULE half of every qualified name in a
// query, for a module rename.
//
// Unlike the entity case this matches on the module alone, so it has one way to
// be wrong: a query-local ALIAS spelled exactly like the module makes
// `Reporting.Label` a column reference, not a qualified name, and rewriting it
// would break the query. Rather than rewrite it and hope, the function detects
// that the query binds such an alias and declines — returning 0 leaves the
// rename reporting nothing for that document, which is a visible non-event
// rather than a silently corrupted query.
func rewriteOQLModulePrefix(oql, oldModule, newModule string) (string, int) {
	if aliasBindingRe(oldModule).MatchString(oql) {
		return oql, 0
	}
	// The trailing group is the entity half: required, so a bare mention of the
	// module word on its own is not treated as a qualified name.
	re := regexp.MustCompile(
		`(` + identAlternation(oldModule) + `)\s*\.\s*(` + anyIdent + `)`)
	count := 0
	out := re.ReplaceAllStringFunc(oql, func(m string) string {
		sub := re.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		count++
		return requote(sub[1], newModule) + "." + sub[2]
	})
	return out, count
}

// anyIdent matches an identifier in any spelling, without pinning its text.
const anyIdent = `"[^"]+"|` + "`[^`]+`" + `|[A-Za-z_][A-Za-z0-9_]*`

// aliasBindingRe matches `AS <name>` binding the given name as a query alias.
func aliasBindingRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\bas\s+` + regexp.QuoteMeta(name) + `\b`)
}
