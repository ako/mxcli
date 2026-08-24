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

	reportTranslationStats(ctx, s, stats, src, scope)
	return nil
}

// reportTranslationStats says what happened, and says it loudly where a caller
// needs to know: OR REPLACE can delete work somebody did in Studio Pro, and an
// unmatched key means the file has stopped describing the project.
func reportTranslationStats(ctx *ExecContext, s *ast.CreateTranslationsStmt, stats translations.Stats, src string, scope translations.Scope) {
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

	if len(stats.Unmatched) == 0 {
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
		len(stats.Unmatched))
	for _, d := range translations.SuggestDrift(stats.Unmatched, dict, entries, s.Language) {
		fmt.Fprintln(ctx.Output, d.Explain(s.Language))
	}
}

func quoteAll(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, mdlQuote(s))
	}
	sort.Strings(out)
	return out
}
