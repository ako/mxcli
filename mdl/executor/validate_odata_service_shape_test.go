// SPDX-License-Identifier: Apache-2.0

// Ledger finding #113: a published OData service failed to build with CE7375
// and the reported cause was a missing mxcli property. It is not — the property
// exists, and the value the script gave it is the one that cannot build.
//
// Measured on Mendix 11.13 against a blank app, both arms:
//
//	PublishAssociations: No  -> CE7375, even publishing no associations at all
//	PublishAssociations: Yes -> 0 errors
//
// The same probe turned up a second shape mxcli writes happily: a Path with no
// slash makes mxbuild throw out of its own validator, with no error code and no
// element name, which reads as a corrupt project rather than a typo.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

func serviceStmt(path string, assoc, assocSet bool) *ast.Program {
	return &ast.Program{Statements: []ast.Statement{
		&ast.CreateODataServiceStmt{
			Name:                   ast.QualifiedName{Module: "M", Name: "Svc"},
			Path:                   path,
			PublishAssociations:    assoc,
			PublishAssociationsSet: assocSet,
			Entities:               []*ast.PublishedEntityDef{{}},
		},
	}}
}

func shapeRuleIDs(vs []linter.Violation) string {
	var ids []string
	for _, v := range vs {
		ids = append(ids, v.RuleID)
	}
	return strings.Join(ids, ",")
}

// TestODataPathSlashRules pins mxbuild's two rules, and the third case that is
// the reason for the check: no slash at all is not an error message, it is a
// .NET stack trace.
func TestODataPathSlashRules(t *testing.T) {
	cases := []struct {
		path      string
		wantRule  bool
		wantPhras string
	}{
		{"odata/cat/", false, ""},                        // the buildable shape
		{"cat/", false, ""},                              // a single trailing slash is enough
		{"/cat/", true, "starts with a slash"},           // CE6550
		{"odata/cat", true, "does not end with a slash"}, // CE6552
		{"cat", true, "contains no slash at all"},        // mxbuild crashes
		{"", false, ""},                                  // unset — not this check's business
	}
	for _, tc := range cases {
		got := ValidateODataServiceShape(serviceStmt(tc.path, true, true))
		var pathViolations []linter.Violation
		for _, v := range got {
			if v.RuleID == "MDL-ODATA05" {
				pathViolations = append(pathViolations, v)
			}
		}
		if tc.wantRule && len(pathViolations) == 0 {
			t.Errorf("Path %q: no MDL-ODATA05, want one", tc.path)
			continue
		}
		if !tc.wantRule && len(pathViolations) > 0 {
			t.Errorf("Path %q: unexpected %s", tc.path, shapeRuleIDs(pathViolations))
			continue
		}
		if tc.wantRule && !strings.Contains(pathViolations[0].Message, tc.wantPhras) {
			t.Errorf("Path %q: message %q, want it to mention %q",
				tc.path, pathViolations[0].Message, tc.wantPhras)
		}
	}
}

// TestPublishAssociationsNoIsExplained is the ledger's case. The suggestion has
// to carry the error code, because CE7375 names "associated object id" — a
// phrase that appears nowhere in the script that caused it.
func TestPublishAssociationsNoIsExplained(t *testing.T) {
	got := ValidateODataServiceShape(serviceStmt("odata/cat/", false, true))
	if len(got) != 1 || got[0].RuleID != "MDL-ODATA06" {
		t.Fatalf("got %v, want one MDL-ODATA06", shapeRuleIDs(got))
	}
	if got[0].Severity != linter.SeverityWarning {
		t.Errorf("severity = %v, want warning: false is a legitimate Mendix mode, just not the one it sounds like", got[0].Severity)
	}
	for _, want := range []string{"CE7375", "Yes"} {
		if !strings.Contains(got[0].Suggestion, want) {
			t.Errorf("suggestion does not mention %q: %s", want, got[0].Suggestion)
		}
	}
}

// TestPublishAssociationsYesAndOmittedAreQuiet — the executor defaults an
// unspecified value to true, which is the buildable one, so neither the default
// nor an explicit Yes has anything to warn about. A rule that fired on the
// correct spelling would be noise on every service in a project.
func TestPublishAssociationsYesAndOmittedAreQuiet(t *testing.T) {
	if got := ValidateODataServiceShape(serviceStmt("odata/cat/", true, true)); len(got) > 0 {
		t.Errorf("explicit Yes produced %s", shapeRuleIDs(got))
	}
	if got := ValidateODataServiceShape(serviceStmt("odata/cat/", false, false)); len(got) > 0 {
		t.Errorf("omitted (defaults to Yes) produced %s", shapeRuleIDs(got))
	}
}

// TestAlterIsCheckedToo — the property is settable both ways, and a script that
// creates a good service then alters it to No has the same build failure.
func TestAlterODataServiceShape(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.AlterODataServiceStmt{
			Name:    ast.QualifiedName{Module: "M", Name: "Svc"},
			Changes: map[string]any{"PublishAssociations": false, "Path": "cat"},
		},
	}}
	got := ValidateODataServiceShape(prog)
	if !strings.Contains(shapeRuleIDs(got), "MDL-ODATA05") || !strings.Contains(shapeRuleIDs(got), "MDL-ODATA06") {
		t.Errorf("alter produced %s, want both rules", shapeRuleIDs(got))
	}
}
