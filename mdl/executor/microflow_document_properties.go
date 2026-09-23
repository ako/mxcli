// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// applyMicroflowDocumentProperties overlays the URL / EXPORT LEVEL / concurrency
// clauses onto a microflow whose fields already hold the STORED values.
//
// The order matters and is the point: the caller seeds mf from the stored
// document, and this only overwrites what the statement actually said. That is
// what keeps "absent preserves" true for properties a script never mentions,
// which is the rule these clauses exist to let a script opt out of rather than
// to abolish (mendixlabs/mxcli#1120).
func applyMicroflowDocumentProperties(ctx *ExecContext, mf *microflows.Microflow, s *ast.CreateMicroflowStmt) error {
	// Deep links are Mendix 10.6+. Gate on the clause being STATED, not on the
	// resulting value: a rewrite that carries a stored URL forward on an older
	// project is preserving what is already there, and refusing that would make
	// the guard destroy the very thing it protects.
	if s.URL != nil || s.URLSearchParameters != nil {
		if err := checkFeature(ctx, "microflows", "deep_link_url",
			"a microflow URL (deep link)",
			"upgrade your project to 10.6+, or set the URL in Studio Pro"); err != nil {
			return err
		}
	}
	if s.URL != nil {
		if err := checkURLNotTaken(ctx, mf, *s.URL); err != nil {
			return err
		}
		mf.URL = *s.URL
	}
	if s.URLSearchParameters != nil {
		mf.URLSearchParameters = qualifiedSearchParams(mf.Name, s)
	}
	if s.ExportLevel != nil {
		mf.ExportLevel = *s.ExportLevel
	}
	if s.Concurrency == nil {
		return nil
	}

	// A clause sets what it states and leaves the sibling alone — it does NOT
	// clear the other kind of error handling. Two independent reasons:
	//
	//  1. It would not work. canon.CarryTranslations copies a stored text's
	//     other languages onto the rebuilt document, because a rebuild drops
	//     every language MDL could not state. It cannot distinguish "cleared on
	//     purpose" from "the statement had no way to say it", so an emptied
	//     message comes straight back. Only a targeted patch may claim
	//     ContentsOwnTranslations, and a microflow rebuild is not one.
	//  2. It should not. Studio Pro greys the error fields out when concurrent
	//     execution is allowed rather than erasing them, so re-ticking the box
	//     restores the message. Matching that keeps a message the user may want
	//     back, and an inert stored message breaks nothing: mxbuild reads it
	//     only when execution is disallowed.
	c := s.Concurrency
	mf.AllowConcurrentExecution = c.Allow
	switch {
	case c.ErrorMicroflow != "":
		mf.ConcurrencyErrorMicroflow = c.ErrorMicroflow
	case c.ErrorMessageSet:
		// One language, and the rest survive: CarryTranslations pairs this text
		// with the stored one by containment path and copies the other languages
		// back, so restating an English message does not drop its Dutch
		// translation. That is what makes ERROR MESSAGE safe to author at all —
		// the guard this once had was refusing a round trip the platform already
		// handles.
		mf.ConcurrencyErrorMessage = &model.Text{
			Translations: map[string]string{messageLanguage(ctx, mf.ConcurrencyErrorMessage): c.ErrorMessage},
		}
	}
	return nil
}

// qualifiedSearchParams renders the bare parameter names a script wrote as the
// Module.Microflow.Parameter qualified names Mendix stores.
func qualifiedSearchParams(microflowName string, s *ast.CreateMicroflowStmt) []string {
	names := *s.URLSearchParameters
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		// Use the parameter's DECLARED spelling: the check accepts a
		// case-insensitive match, and a qualified name that disagrees with the
		// parameter it points at does not resolve.
		spelling := n
		for _, p := range s.Parameters {
			if strings.EqualFold(p.Name, n) {
				spelling = p.Name
				break
			}
		}
		out = append(out, fmt.Sprintf("%s.%s.%s", s.Name.Module, microflowName, spelling))
	}
	return out
}

// messageLanguage picks the language a single-string MDL message is written in:
// the stored message's own language when there is exactly one, else the
// PROJECT's default.
//
// Not a hardcoded "en_US". Mendix has no language-neutral text, so a message
// written under the wrong code is invisible to a Dutch app and adds a language
// rather than editing the one that is there — #1113, in the catalog.
func messageLanguage(ctx *ExecContext, stored *model.Text) string {
	if stored != nil && len(stored.Translations) == 1 {
		for lang := range stored.Translations {
			return lang
		}
	}
	return describeDefaultLanguage(ctx)
}

// MicroflowDocumentPropertyProblems returns every rule violation in the URL /
// EXPORT LEVEL / concurrency clauses of one statement.
//
// Exported and statement-shaped because `mxcli check` and the executor MUST
// apply the identical rules: check calls it to report MDL084-MDL086 with source
// positions, exec calls it to refuse the write. The rules themselves live one
// layer down in mdl/types, so neither caller owns them.
func MicroflowDocumentPropertyProblems(s *ast.CreateMicroflowStmt) []string {
	if s == nil {
		return nil
	}
	names := make([]string, 0, len(s.Parameters))
	for _, p := range s.Parameters {
		names = append(names, p.Name)
	}

	var problems []string
	url, search := "", []string(nil)
	if s.URL != nil {
		url = *s.URL
	}
	if s.URLSearchParameters != nil {
		search = *s.URLSearchParameters
	}
	problems = append(problems, types.CheckMicroflowURL(url, search, names)...)

	if s.ExportLevel != nil {
		if p := types.CheckExportLevel(*s.ExportLevel); p != "" {
			problems = append(problems, p)
		}
	}
	if c := s.Concurrency; c != nil && !c.Allow {
		if p := types.CheckMicroflowConcurrency(true, c.ErrorMessageSet, c.ErrorMicroflow); p != "" {
			problems = append(problems, p)
		}
	}
	return problems
}

// checkMicroflowDocumentProperties refuses a write that breaks any of them.
func checkMicroflowDocumentProperties(s *ast.CreateMicroflowStmt) error {
	if problems := MicroflowDocumentPropertyProblems(s); len(problems) > 0 {
		return mdlerrors.NewUnsupported(fmt.Sprintf("microflow %s: %s",
			s.Name.String(), strings.Join(problems, "; ")))
	}
	return nil
}

// checkURLNotTaken refuses a deep link another microflow already owns.
//
// Mendix requires URLs to be unique across the app and reports a collision as
// CE0570 ("The URL 'item/{Key}' of microflow 'A' is conflicting with the URL of
// microflow 'B'"). Like CE5612, it is invisible until a build — and this feature
// makes it reachable in a way it was not before: `describe` now emits the URL,
// so the describe -> rename -> exec COPY that the clauses exist to enable
// produces two microflows with one URL unless the copy is edited. That is the
// first thing anyone will do with this, so it is worth refusing by name.
//
// Needs the project, so it lives here rather than in the statement-local rules:
// a script cannot see the other microflows, and neither can `mxcli check`
// without -p.
func checkURLNotTaken(ctx *ExecContext, mf *microflows.Microflow, url string) error {
	if url == "" || ctx == nil || ctx.Backend == nil {
		return nil
	}
	others, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil // not this guard's business; the write path reports its own errors
	}
	for _, other := range others {
		// The microflow being rewritten owns its own URL, and an excluded
		// document is not part of the app, so neither can collide.
		if other.ID == mf.ID || other.Excluded || !strings.EqualFold(other.URL, url) {
			continue
		}
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"URL %q is already the deep link of microflow %s — Mendix requires them to be "+
				"unique and reports a duplicate as CE0570.\n"+
				"  Give this microflow a different URL, or `drop url` on the other one.",
			url, other.Name))
	}
	return nil
}
