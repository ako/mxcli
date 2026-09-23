// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func strp(s string) *string       { return &s }
func slicep(s []string) *[]string { return &s }

// TestAuthoredProperties_AbsentPreserves is the rule the whole clause design
// rests on, and the reason every field is a pointer.
//
// These properties were carried unconditionally before they were authorable
// (mendixlabs/mxcli#1120). Making them authorable must not turn "the script did
// not say" into "set it to the zero value" — that is the bug the carry fixed,
// reintroduced through the front door.
func TestAuthoredProperties_AbsentPreserves(t *testing.T) {
	stored := &microflows.Microflow{
		Name:                     "ACT_Item",
		URL:                      "item/{Key}",
		URLSearchParameters:      []string{"M.ACT_Item.Filter"},
		ExportLevel:              "API",
		AllowConcurrentExecution: false,
		ConcurrencyErrorMessage:  &model.Text{Translations: map[string]string{"en_US": "Busy"}},
	}
	// A statement with no clauses at all.
	if err := applyMicroflowDocumentProperties(nil, stored, &ast.CreateMicroflowStmt{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if stored.URL != "item/{Key}" || len(stored.URLSearchParameters) != 1 {
		t.Errorf("URL not preserved: %q %v", stored.URL, stored.URLSearchParameters)
	}
	if stored.ExportLevel != "API" {
		t.Errorf("export level not preserved: %q", stored.ExportLevel)
	}
	if stored.AllowConcurrentExecution || stored.ConcurrencyErrorMessage == nil {
		t.Error("concurrency not preserved")
	}
}

// TestAuthoredProperties_StatedOverrides is the other half: a clause that IS
// written must take effect, or the feature does nothing.
func TestAuthoredProperties_StatedOverrides(t *testing.T) {
	mf := &microflows.Microflow{Name: "ACT_Item", ExportLevel: "Hidden", AllowConcurrentExecution: true}
	stmt := &ast.CreateMicroflowStmt{
		Name:        ast.QualifiedName{Module: "M", Name: "ACT_Item"},
		Parameters:  []ast.MicroflowParam{{Name: "Key"}, {Name: "Filter"}},
		URL:         strp("item/{Key}"),
		ExportLevel: strp("API"),
		Concurrency: &ast.ConcurrencyClause{ErrorMessage: "Busy", ErrorMessageSet: true},
	}
	stmt.URLSearchParameters = slicep([]string{"Filter"})
	if err := applyMicroflowDocumentProperties(nil, mf, stmt); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if mf.URL != "item/{Key}" || mf.ExportLevel != "API" || mf.AllowConcurrentExecution {
		t.Errorf("clauses not applied: %#v", mf)
	}
	// The stored form is the qualified name, built from the parameter's DECLARED
	// spelling — a qualified name that disagrees with its parameter does not
	// resolve.
	if len(mf.URLSearchParameters) != 1 || mf.URLSearchParameters[0] != "M.ACT_Item.Filter" {
		t.Errorf("search parameter not qualified: %v", mf.URLSearchParameters)
	}
	if mf.ConcurrencyErrorMessage == nil || mf.ConcurrencyErrorMessage.Translations["en_US"] != "Busy" {
		t.Errorf("error message not applied: %#v", mf.ConcurrencyErrorMessage)
	}
}

// TestAuthoredProperties_AllowKeepsTheMessage pins a deliberate non-clearing.
//
// ALLOW CONCURRENT EXECUTION sets the flag and LEAVES a stored error message.
// Two reasons, and the first is that it could not work anyway:
// canon.CarryTranslations copies a stored text's languages back onto a rebuilt
// document, because a rebuild cannot say them — so an emptied message returns on
// the next write. Measured end to end: after `allow concurrent execution` the
// stored en_US text was still on disk, byte-identical, with its original $ID.
// The second is that Studio Pro greys those fields rather than erasing them, so
// keeping the message is what re-ticking the box expects.
func TestAuthoredProperties_AllowKeepsTheMessage(t *testing.T) {
	mf := &microflows.Microflow{
		Name:                     "ACT_Item",
		AllowConcurrentExecution: false,
		ConcurrencyErrorMessage:  &model.Text{Translations: map[string]string{"en_US": "Busy"}},
	}
	stmt := &ast.CreateMicroflowStmt{Concurrency: &ast.ConcurrencyClause{Allow: true}}
	if err := applyMicroflowDocumentProperties(nil, mf, stmt); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !mf.AllowConcurrentExecution {
		t.Error("ALLOW did not set the flag")
	}
	if mf.ConcurrencyErrorMessage == nil {
		t.Error("ALLOW erased the error message; it is inert, not deleted — and " +
			"CarryTranslations would put it back on the next write anyway")
	}
}

// TestAuthoredProperties_DropURLClearsBoth: a search-parameter list without a
// URL is configuration for a deep link that no longer exists.
func TestAuthoredProperties_DropURLClearsBoth(t *testing.T) {
	mf := &microflows.Microflow{
		Name:                "ACT_Item",
		URL:                 "item/{Key}",
		URLSearchParameters: []string{"M.ACT_Item.Filter"},
	}
	stmt := &ast.CreateMicroflowStmt{URL: strp(""), URLSearchParameters: slicep(nil)}
	if err := applyMicroflowDocumentProperties(nil, mf, stmt); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if mf.URL != "" || len(mf.URLSearchParameters) != 0 {
		t.Errorf("DROP URL left %q %v", mf.URL, mf.URLSearchParameters)
	}
}

// TestAuthoredProperties_ChecksRefuseBeforeTheWrite pins that exec applies the
// SAME rules `mxcli check` reports, so a script cannot pass check and fail exec.
func TestAuthoredProperties_ChecksRefuseBeforeTheWrite(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name:       ast.QualifiedName{Module: "M", Name: "ACT_Item"},
		Parameters: []ast.MicroflowParam{{Name: "Key"}},
		URL:        strp("item/{Key}"),
	}
	stmt.URLSearchParameters = slicep([]string{"Key"}) // CE5612: path AND search

	if problems := MicroflowDocumentPropertyProblems(stmt); len(problems) == 0 {
		t.Fatal("check did not report the CE5612 overlap")
	}
	if err := checkMicroflowDocumentProperties(stmt); err == nil {
		t.Error("exec accepted a statement check refuses — the two have drifted")
	}
}
