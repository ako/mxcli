// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// TestCheckMicroflowURL_PathAndSearchAreDisjoint is CE5612, the rule that is
// invisible without a build and that this feature's own first fixture broke.
func TestCheckMicroflowURL_PathAndSearchAreDisjoint(t *testing.T) {
	params := []string{"Key", "Filter"}

	if got := CheckMicroflowURL("item/{Key}", []string{"Filter"}, params); len(got) != 0 {
		t.Errorf("a disjoint path/search split must be accepted, got %v", got)
	}

	got := CheckMicroflowURL("item/{Key}", []string{"Key"}, params)
	if len(got) != 1 {
		t.Fatalf("reusing one parameter for both must be refused, got %v", got)
	}
	if !contains(got[0], "CE5612") {
		t.Errorf("the message must name the build error it prevents: %q", got[0])
	}
}

// TestCheckMicroflowURL_PlaceholderNamesAParameter catches the typo that
// otherwise reaches a build.
func TestCheckMicroflowURL_PlaceholderNamesAParameter(t *testing.T) {
	// An unknown name lists what IS declared rather than guessing at a
	// correction. A case-only mismatch is NOT one of these: it resolves.
	got := CheckMicroflowURL("item/{Nope}", nil, []string{"Key"})
	if len(got) != 1 || !contains(got[0], "declared: $Key") {
		t.Errorf("an unrelated name should list the parameters, got %v", got)
	}
	if got := CheckMicroflowURL("item/{Key}", nil, []string{"Key"}); len(got) != 0 {
		t.Errorf("a correct placeholder must be accepted, got %v", got)
	}
	// Case-insensitive resolution is the rule, not a near-miss report: MDL
	// resolves parameter references that way everywhere else.
	if got := CheckMicroflowURL("item/{KEY}", []string{"filter"}, []string{"Key", "Filter"}); len(got) != 0 {
		t.Errorf("case-insensitive resolution must be accepted, got %v", got)
	}
}

// TestURLPathParameters_AttributePath is the rule the page-side validator
// already knew and this one nearly missed: Mendix allows an attribute path in a
// segment, so `{Customer/Name}` binds the Customer PARAMETER. Matching the whole
// segment would read that as a parameter named "Customer/Name" and flag a URL
// that builds perfectly well.
func TestURLPathParameters_AttributePath(t *testing.T) {
	got := URLPathParameters("order/{Customer/Name}/{Id}")
	if len(got) != 2 || got[0] != "Customer" || got[1] != "Id" {
		t.Fatalf("got %v, want [Customer Id]", got)
	}
	if problems := CheckMicroflowURL("order/{Customer/Name}", nil, []string{"Customer"}); len(problems) != 0 {
		t.Errorf("an attribute path must resolve to its parameter, got %v", problems)
	}
}

// TestCheckMicroflowConcurrency is CE4899: disallowing needs a handler.
func TestCheckMicroflowConcurrency(t *testing.T) {
	if got := CheckMicroflowConcurrency(true, false, ""); !contains(got, "CE4899") {
		t.Errorf("a bare DISALLOW must be refused by name, got %q", got)
	}
	if got := CheckMicroflowConcurrency(true, true, ""); got != "" {
		t.Errorf("a message satisfies it, got %q", got)
	}
	if got := CheckMicroflowConcurrency(true, false, "Mod.OnBusy"); got != "" {
		t.Errorf("an error microflow satisfies it, got %q", got)
	}
	if got := CheckMicroflowConcurrency(false, false, ""); got != "" {
		t.Errorf("ALLOW needs no handler, got %q", got)
	}
}

// TestCheckExportLevel pins the enum. "Public" is the value the image-collection
// grammar's own example comment uses and neither enum declares — the reason this
// clause takes keywords rather than a quoted string.
func TestCheckExportLevel(t *testing.T) {
	for _, ok := range []string{"", ExportLevelAPI, ExportLevelHidden} {
		if got := CheckExportLevel(ok); got != "" {
			t.Errorf("%q must be accepted, got %q", ok, got)
		}
	}
	if got := CheckExportLevel("Public"); got == "" {
		t.Error("Public is not a member of MicroflowsExportLevel and must be refused")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
