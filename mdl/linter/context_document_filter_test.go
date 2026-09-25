// SPDX-License-Identifier: Apache-2.0

// --documents (ako/mxcli#681, upstream mendixlabs/mxcli#1186).
//
// A project-wide lint was the only way to ask a structural question about one
// document: a reporter measured ~13 s per exec and maintained a baseline diff on
// top, so the gate got batched to once per session and three CONV011 violations
// shipped under a clean mxbuild log.
//
// Scoping is in the ITERATOR rather than in each rule because that is the one
// place it covers every rule at once. CONV011 and MPR002 both walk Microflows()
// and both call FullMicroflow() per row — that read is what makes lint scale
// with project size, and narrowing the query skips it without touching a rule.
package linter

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// twoInOneModuleDB adds a SECOND microflow to ModB.
//
// The shared fixture has one microflow per module, which cannot distinguish
// iterator narrowing from the module implication — naming ModB.Flow excludes
// ModA and ModC either way, so a test on that fixture passes against code with
// no document filter at all. Reverting the SQL narrowing proved exactly that,
// and the first version of the test below was green against it.
//
// A sibling in the SAME module is the only thing that isolates the two.
func twoInOneModuleDB(t *testing.T) catalog.CatalogDB {
	t.Helper()
	db := setupModuleFilterDB(t)
	if _, err := db.Exec(`INSERT INTO microflows VALUES (?, ?, ?, ?, '', 'Microflow', '', '', 0, 0, 0)`,
		"ModB_mf2", "ModB_Sibling", "ModB.Sibling", "ModB"); err != nil {
		t.Fatalf("insert sibling microflow: %v", err)
	}
	return db
}

func collectMicroflowNames(ctx *LintContext) []string {
	var out []string
	for mf := range ctx.Microflows() {
		out = append(out, mf.QualifiedName)
	}
	return out
}

func TestDocumentFilter_NarrowsTheIterator(t *testing.T) {
	ctx := NewLintContextFromDB(twoInOneModuleDB(t))
	ctx.SetIncludedDocuments([]string{"ModB.Flow"})

	got := collectMicroflowNames(ctx)
	if len(got) != 1 || got[0] != "ModB.Flow" {
		t.Errorf("Microflows() = %v, want exactly [ModB.Flow] — ModB.Sibling is in the "+
			"SAME module, so only the iterator filter can exclude it, and a rule walking "+
			"this iterator must not pay FullMicroflow() for documents nobody asked about", got)
	}
}

// CONTROL: with no allowlist the iterator is unchanged. Without this the filter
// could be "return nothing unless asked", which every existing lint run would hit.
func TestDocumentFilter_AbsentMeansEverything(t *testing.T) {
	ctx := NewLintContextFromDB(twoInOneModuleDB(t))
	if got := collectMicroflowNames(ctx); len(got) != 4 {
		t.Errorf("Microflows() = %v with no --documents, want all 4", got)
	}
}

// The module implication is what makes a scoped lint faster in the rules this
// package does not narrow: they all guard their per-document read with
// IsExcluded(moduleName), so naming a document must exclude the other modules.
func TestDocumentFilter_ImpliesItsModule(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetIncludedDocuments([]string{"ModB.Flow"})

	if ctx.IsExcluded("ModB") {
		t.Error("ModB excluded, but a document in it was named")
	}
	for _, other := range []string{"ModA", "ModC"} {
		if !ctx.IsExcluded(other) {
			t.Errorf("%s not excluded — a rule that is not iterator-narrowed would "+
				"still read every document in it", other)
		}
	}
}

// --modules and --documents INTERSECT. Replacing would silently widen an
// explicit filter, which is the direction that turns a scoped run into a
// project-wide one without saying so.
func TestDocumentFilter_IntersectsWithModuleFilter(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetIncludedModules([]string{"ModA"})
	ctx.SetIncludedDocuments([]string{"ModB.Flow"})

	for _, m := range []string{"ModA", "ModB", "ModC"} {
		if !ctx.IsExcluded(m) {
			t.Errorf("%s not excluded: --modules ModA with --documents ModB.Flow "+
				"names an empty set, and must lint nothing rather than all of either", m)
		}
	}
}

// An exclude still wins. IsExcluded checks the exclude set first, and
// --documents must not be a way around a config that excludes a module.
func TestDocumentFilter_DoesNotOverrideAnExclude(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	ctx.SetExcludedModules([]string{"ModB"})
	ctx.SetIncludedDocuments([]string{"ModB.Flow"})

	if !ctx.IsExcluded("ModB") {
		t.Error("an explicitly excluded module became lintable by naming a document in it")
	}
}

// A qualified name is data from the command line and reaches a SQL string.
func TestDocumentFilter_QuoteInNameDoesNotBreakTheQuery(t *testing.T) {
	ctx := NewLintContextFromDB(twoInOneModuleDB(t))
	ctx.SetIncludedDocuments([]string{"ModB.O'Brien", "ModB.Flow"})

	got := collectMicroflowNames(ctx)
	if len(got) != 1 || got[0] != "ModB.Flow" {
		t.Errorf("Microflows() = %v, want [ModB.Flow]; an apostrophe in a sibling "+
			"name must not break the query (which would yield nothing and read as clean)", got)
	}
}

func TestIsDocumentExcluded(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))
	if ctx.IsDocumentExcluded("ModA.Flow") {
		t.Error("nothing is document-excluded before an allowlist is set")
	}
	ctx.SetIncludedDocuments([]string{"ModA.Flow"})
	if ctx.IsDocumentExcluded("ModA.Flow") {
		t.Error("the named document reports as excluded")
	}
	if !ctx.IsDocumentExcluded("ModA.Other") {
		t.Error("an unnamed document in the same module reports as included")
	}
}
