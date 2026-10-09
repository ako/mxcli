// SPDX-License-Identifier: Apache-2.0

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func TestChooseDescribeType(t *testing.T) {
	cases := []struct {
		name      string
		matches   []string
		wantType  string
		wantCands []string
		wantErr   bool
	}{
		{"single", []string{"microflow"}, "microflow", nil, false},
		{"dedup to single", []string{"entity", "entity"}, "entity", nil, false},
		{"empties filtered", []string{"", "page", ""}, "page", nil, false},
		{"ambiguous", []string{"entity", "microflow"}, "", []string{"entity", "microflow"}, false},
		{"ambiguous with dup", []string{"entity", "microflow", "entity"}, "", []string{"entity", "microflow"}, false},
		{"order preserved", []string{"microflow", "entity"}, "", []string{"microflow", "entity"}, false},
		{"none", nil, "", nil, true},
		{"all empty is none", []string{"", ""}, "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotCands, err := chooseDescribeType("Mod.X", tc.matches)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if gotType != tc.wantType {
				t.Errorf("type = %q, want %q", gotType, tc.wantType)
			}
			if !slices.Equal(gotCands, tc.wantCands) {
				t.Errorf("candidates = %v, want %v", gotCands, tc.wantCands)
			}
		})
	}
}

// TestTypeMaps_KnownEntries pins the mappings the auto-detect dispatch depends
// on: every mapped value must be a describe keyword the command actually handles,
// and the common types must be present.
func TestTypeMaps_KnownEntries(t *testing.T) {
	wantObject := map[string]string{
		"MICROFLOW": "microflow", "ENTITY": "entity", "PAGE": "page",
		"ENUMERATION": "enumeration", "MODULE": "module", "EXTERNAL_ENTITY": "entity",
	}
	for k, v := range wantObject {
		if objectTypeToDescribe[k] != v {
			t.Errorf("objectTypeToDescribe[%q] = %q, want %q", k, objectTypeToDescribe[k], v)
		}
	}
	wantUnit := map[string]string{
		"Microflows$Microflow": "microflow", "Forms$Page": "page",
		"Enumerations$Enumeration": "enumeration", "JavaActions$JavaAction": "javaaction",
	}
	for k, v := range wantUnit {
		if unitTypeToDescribe[k] != v {
			t.Errorf("unitTypeToDescribe[%q] = %q, want %q", k, unitTypeToDescribe[k], v)
		}
	}
	// Every mapped describe keyword must be a single bare word (no spaces) so it
	// slots into the dispatch as args[0].
	for _, m := range []map[string]string{objectTypeToDescribe, unitTypeToDescribe} {
		for k, v := range m {
			if v == "" {
				t.Errorf("empty describe keyword for %q", k)
			}
		}
	}
}

// TestDescribeMDLCommand_AutoDetectKeywordsAccepted pins that every keyword the
// auto-detect maps can produce is one the dispatch accepts and that the MDL it
// builds parses. A keyword missing from the dispatch reaches the user as
// "Unknown type" for a document the catalog lists (#1347).
func TestDescribeMDLCommand_AutoDetectKeywordsAccepted(t *testing.T) {
	for _, m := range []map[string]string{objectTypeToDescribe, unitTypeToDescribe} {
		for k, kw := range m {
			name := "Mod.X"
			if kw == "module" {
				name = "Mod" // a module's name is not qualified
			}
			mdl, ok := describeMDLCommand(strings.ToUpper(kw), name)
			if !ok {
				t.Errorf("auto-detect keyword %q (from %q) is not accepted by describe", kw, k)
				continue
			}
			if _, errs := visitor.Build(mdl); len(errs) > 0 {
				t.Errorf("keyword %q builds %q, which does not parse: %v", kw, mdl, errs)
			}
		}
	}
}

// TestDescribeMDLCommand_PublishedRestService is the #1347 symptom: `mxcli
// describe published rest service M.Api` printed "Unknown type" although the
// MDL statement works under `-c`, and auto-detect could not resolve the name.
func TestDescribeMDLCommand_PublishedRestService(t *testing.T) {
	const want = "DESCRIBE PUBLISHED REST SERVICE M.Api"
	for _, typ := range []string{"PUBLISHED REST SERVICE", "PUBLISHEDRESTSERVICE", "REST SERVICE", "RESTSERVICE"} {
		got, ok := describeMDLCommand(typ, "M.Api")
		if !ok {
			t.Errorf("describe %q: Unknown type", strings.ToLower(typ))
			continue
		}
		if got != want {
			t.Errorf("describe %q = %q, want %q", strings.ToLower(typ), got, want)
		}
	}
	if _, errs := visitor.Build(want); len(errs) > 0 {
		t.Fatalf("%q does not parse: %v", want, errs)
	}
	if got := objectTypeToDescribe["PUBLISHED_REST_SERVICE"]; got == "" {
		t.Error("catalog ObjectType PUBLISHED_REST_SERVICE has no auto-detect keyword")
	}
	if got := unitTypeToDescribe["Rest$PublishedRestService"]; got == "" {
		t.Error("unit type Rest$PublishedRestService has no auto-detect keyword")
	}
}
