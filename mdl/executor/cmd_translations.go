// SPDX-License-Identifier: Apache-2.0

// CREATE / DESCRIBE TRANSLATIONS. See docs/11-proposals/PROPOSAL_translations.md.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/translations"
	"github.com/mendixlabs/mxcli/model"
)

// translationScope restricts the walk to one module's units, or nil for the
// whole project. Resolved through the container hierarchy, which is what knows
// that a document nested in folders still belongs to its module.
func translationScope(ctx *ExecContext, moduleName string) (translations.Scope, error) {
	if moduleName == "" {
		return nil, nil
	}
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return nil, mdlerrors.NewBackend("list modules", err)
	}
	var moduleID model.ID
	for _, m := range modules {
		if strings.EqualFold(m.Name, moduleName) {
			moduleID = m.ID
			break
		}
	}
	if moduleID == "" {
		return nil, mdlerrors.NewNotFound("module", moduleName)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("resolve module hierarchy", err)
	}
	return func(unitID model.ID) bool {
		return h.FindModuleID(unitID) == moduleID || unitID == moduleID
	}, nil
}

// sourceLanguage is the language a translation file's left-hand column is
// written in: the project's default. DESCRIBE already resolves it this way for
// display (issue #702), and using the same value keeps a described file and the
// project talking about the same strings.
func sourceLanguage(ctx *ExecContext) string { return describeDefaultLanguage(ctx) }

func execDescribeTranslations(ctx *ExecContext, s *ast.DescribeTranslationsStmt) error {
	if ctx.Backend == nil {
		return mdlerrors.NewValidation("no project connected")
	}
	scope, err := translationScope(ctx, s.Module)
	if err != nil {
		return err
	}
	src := sourceLanguage(ctx)
	if strings.EqualFold(s.Language, src) {
		return mdlerrors.NewValidationf(
			"%s is the project's source language — there is nothing to translate it into. "+
				"Describe a different language, or change DefaultLanguageCode in project settings.",
			s.Language)
	}

	entries, conflicts, err := translations.Collect(ctx.Backend, src, scope)
	if err != nil {
		return mdlerrors.NewBackend("collect translations", err)
	}

	in := ""
	if s.Module != "" {
		in = " in " + s.Module
	}
	fmt.Fprintf(ctx.Output, "create or modify translations%s for %s (\n", in, s.Language)
	translated := 0
	for _, e := range entries {
		t := e.Targets[s.Language]
		if t != "" {
			translated++
		}
		fmt.Fprintf(ctx.Output, "    %s as %s,\n", mdlQuote(e.Source), mdlQuote(t))
	}
	fmt.Fprintln(ctx.Output, ");")

	fmt.Fprintf(ctx.Output, "\n-- %d source string(s), %d translated, %d to go.\n",
		len(entries), translated, len(entries)-translated)
	if len(conflicts) > 0 {
		fmt.Fprintf(ctx.Output,
			"-- %d source string(s) occur with DIFFERENT translations and cannot be\n"+
				"-- described by one entry; the first is shown. Scope with `in <Module>`\n"+
				"-- to separate them: %s\n",
			len(conflicts), strings.Join(quoteAll(conflicts), ", "))
	}
	return nil
}

func execCreateTranslations(ctx *ExecContext, s *ast.CreateTranslationsStmt) error {
	if ctx.Backend == nil {
		return mdlerrors.NewValidation("no project connected")
	}
	scope, err := translationScope(ctx, s.Module)
	if err != nil {
		return err
	}
	src := sourceLanguage(ctx)
	if strings.EqualFold(s.Language, src) {
		return mdlerrors.NewValidationf(
			"%s is the project's source language — writing translations into it would "+
				"overwrite the strings the rest of the model is keyed on", s.Language)
	}

	mode := translations.ModeMerge
	switch s.Mode {
	case ast.TranslationsReplace:
		mode = translations.ModeReplace
	case ast.TranslationsCreate:
		// The language is the thing that exists, so bare CREATE refuses when it
		// already has translations — the same contract every other CREATE has.
		existing, err := translations.Languages(ctx.Backend, scope)
		if err != nil {
			return mdlerrors.NewBackend("read languages", err)
		}
		for _, l := range existing {
			if strings.EqualFold(l, s.Language) {
				return mdlerrors.NewValidationf(
					"%s already has translations — use `create or modify translations` to "+
						"merge these in, or `create or replace translations` to make this file "+
						"authoritative (which REMOVES translations it does not name)", s.Language)
			}
		}
	}

	dict := make(translations.Dictionary, len(s.Entries))
	for _, e := range s.Entries {
		dict[e.Source] = e.Target
	}

	stats, err := translations.Apply(ctx.Backend, src, s.Language, dict, mode, scope)
	if err != nil {
		return mdlerrors.NewBackend("apply translations", err)
	}

	// Computed before the stats are reported, because it changes what the DRIFT
	// warning may claim: an entry that matched nothing IN SCOPE but does match
	// outside it has not drifted at all, and "the text may have been deleted" is
	// the wrong diagnosis for it.
	outOfScope, err := translations.OutOfScope(ctx.Backend, src, scope, dict)
	if err != nil {
		outOfScope = nil
	}

	reportTranslationStats(ctx, s, stats, src, scope, outOfScope)
	reportOutOfScopeEntries(ctx, s, outOfScope)

	// A translation for a language the project has not enabled is stored, passes
	// every check, and is then discarded by the build. Say so AFTER the stats, so
	// the warning attaches to a run that reports having written something.
	if w := unenabledLanguageWarning(projectLanguageSettings(ctx), s.Language); w != "" {
		fmt.Fprint(ctx.Output, w)
	}
	return nil
}

// reportTranslationStats says what happened, and says it loudly where a caller
// needs to know: OR REPLACE can delete work somebody did in Studio Pro, and an
// unmatched key means the file has stopped describing the project.
//
// outOfScope names the entries an `IN <Module>` run could not reach. They are
// excluded from the drift warning: they matched nothing in scope, but they DID
// match, so "the text may have been deleted" would be false about them and
// reportOutOfScopeEntries says the true thing instead.
func reportTranslationStats(ctx *ExecContext, s *ast.CreateTranslationsStmt, stats translations.Stats, src string, scope translations.Scope, outOfScope []string) {
	switch {
	case stats.Set == 0 && stats.Removed == 0:
		// Nothing to do is the normal outcome of re-running a file, so say so
		// plainly rather than reporting a write that did not happen. Entries that
		// matched nothing are counted separately below, not called "matched".
		fmt.Fprintf(ctx.Output, "Unchanged translations: %s (%d of %d entries already in place)\n",
			s.Language, len(s.Entries)-len(stats.Unmatched), len(s.Entries))
	case stats.Removed == 0:
		fmt.Fprintf(ctx.Output, "Set %d %s translation(s) across %d document(s)\n",
			stats.Set, s.Language, stats.Units)
	default:
		fmt.Fprintf(ctx.Output, "Set %d and removed %d %s translation(s) across %d document(s)\n",
			stats.Set, stats.Removed, s.Language, stats.Units)
	}

	if stats.Removed > 0 {
		// CREATE OR REPLACE can delete a translation somebody made in Studio Pro
		// and never added to the file. Under guard-don't-drop that cannot be a
		// bare count: name what went.
		fmt.Fprintf(ctx.Output,
			"\nRemoved because the file does not name them (create or replace):\n")
		for _, srcStr := range stats.RemovedSources {
			fmt.Fprintf(ctx.Output, "  %s\n", mdlQuote(srcStr))
		}
	}

	drifted := excludeStrings(stats.Unmatched, outOfScope)
	if len(drifted) == 0 {
		return
	}
	entries, _, err := translations.Collect(ctx.Backend, src, scope)
	if err != nil {
		entries = nil
	}
	dict := make(translations.Dictionary, len(s.Entries))
	for _, e := range s.Entries {
		dict[e.Source] = e.Target
	}
	fmt.Fprintf(ctx.Output,
		"\nWarning: %d source string(s) in the file matched nothing in the project.\n"+
			"A source edited after the file was written stops matching, which leaves its\n"+
			"translation attached to a string that no longer exists:\n\n",
		len(drifted))
	for _, d := range translations.SuggestDrift(drifted, dict, entries, s.Language) {
		fmt.Fprintln(ctx.Output, d.Explain(s.Language))
	}
}

// excludeStrings returns ss without any value in drop, preserving order.
func excludeStrings(ss, drop []string) []string {
	if len(drop) == 0 {
		return ss
	}
	skip := make(map[string]bool, len(drop))
	for _, d := range drop {
		skip[d] = true
	}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !skip[s] {
			out = append(out, s)
		}
	}
	return out
}

// reportOutOfScopeEntries says what an `IN <Module>` run did NOT consider.
//
// The NAVIGATION is a project-level document, not a module one, so `in Ledger`
// never reaches it: the app's page text switched to Dutch and the sidebar did
// not, while the run reported "Set 212 nl_NL translation(s) across 20
// document(s)" — a success and a count, and the count is the only thing that
// would have given it away, if you knew what number to expect (ledger #137).
//
// Only the file's OWN entries are reported. "The project has other strings" is
// true of every scoped run and would warn forever, which is exactly the
// per-module workflow the scoping exists to support.
func reportOutOfScopeEntries(ctx *ExecContext, s *ast.CreateTranslationsStmt, missed []string) {
	if s.Module == "" || len(missed) == 0 {
		return
	}
	fmt.Fprintf(ctx.Output,
		"\nNot considered: %d of this file's source string(s) also occur OUTSIDE module %s,\n"+
			"and `in %s` did not reach them. The navigation is a project-level document, so a\n"+
			"scoped run leaves the menu in the source language while the pages switch:\n\n",
		len(missed), s.Module, s.Module)
	for _, srcStr := range quoteAll(missed) {
		fmt.Fprintf(ctx.Output, "  %s\n", srcStr)
	}
	fmt.Fprintf(ctx.Output,
		"\nRe-run the same file without `in %s` to land these as well.\n", s.Module)
}

func quoteAll(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, mdlQuote(s))
	}
	sort.Strings(out)
	return out
}
