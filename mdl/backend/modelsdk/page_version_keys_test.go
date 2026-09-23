// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// Forms$Page and Forms$Snippet carry two properties Mendix introduced after the
// oldest version mxcli supports (mendixmodelsdk 4.115.0, Page.versionInfo /
// Snippet.versionInfo):
//
//	Page.Autofocus              11.1.0
//	Page.Variables              10.17.0
//	Snippet.Variables           10.17.0
//
// Both were written unconditionally on every page and snippet, so a document
// mxcli created for a Mendix 10 project carried a key that project's metamodel
// does not declare — the same defect as the page-parameter keys in #1121, in the
// document header rather than its parameters.
//
// mxbuild is not a safety net for this: measured on 10.24.25, `mx check` reports
// 0 errors with an 11.5-only key present. Studio Pro resolves every stored
// property against the type's property list and throws InvalidOperationException
// at MprProperty.cs.

func v(major, minor, patch int) *types.ProjectVersion {
	return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor, PatchVersion: patch}
}

func pageKeys(t *testing.T, pv *types.ProjectVersion) bson.Raw {
	t.Helper()
	p := &pages.Page{Name: "P"}
	p.ID = "1"
	b, err := encodePage(p, pv, nil)
	if err != nil {
		t.Fatalf("encodePage: %v", err)
	}
	return bson.Raw(b)
}

func has(raw bson.Raw, key string) bool {
	_, err := raw.LookupErr(key)
	return err == nil
}

func TestPageOmitsAutofocusBelow11_1(t *testing.T) {
	tests := []struct {
		name string
		pv   *types.ProjectVersion
		want bool
	}{
		{"10.24.25 (the version in #1121)", v(10, 24, 25), false},
		{"9.24.0", v(9, 24, 0), false},
		{"11.0.0", v(11, 0, 0), false},
		{"11.1.0 (the floor)", v(11, 1, 0), true},
		{"11.13.0", v(11, 13, 0), true},
		{"unknown version", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := has(pageKeys(t, tt.pv), "Autofocus"); got != tt.want {
				t.Errorf("Autofocus emitted = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPageOmitsVariablesBelow10_17(t *testing.T) {
	tests := []struct {
		name string
		pv   *types.ProjectVersion
		want bool
	}{
		{"10.16.0", v(10, 16, 0), false},
		{"9.24.0", v(9, 24, 0), false},
		{"10.17.0 (the floor)", v(10, 17, 0), true},
		{"10.24.25", v(10, 24, 25), true},
		{"11.13.0", v(11, 13, 0), true},
		{"unknown version", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := has(pageKeys(t, tt.pv), "Variables"); got != tt.want {
				t.Errorf("Variables emitted = %v, want %v", got, tt.want)
			}
		})
	}
}

// A snippet carries the same Variables floor. It never populates the list, so the
// only thing that ever reaches disk is the empty marker the defaults registry
// adds — which is exactly the key that must not reach a pre-10.17 project.
func TestSnippetOmitsVariablesBelow10_17(t *testing.T) {
	encode := func(pv *types.ProjectVersion) bson.Raw {
		sn := &pages.Snippet{Name: "S"}
		sn.ID = "1"
		b, err := encodeSnippet(sn, pv)
		if err != nil {
			t.Fatalf("encodeSnippet: %v", err)
		}
		return bson.Raw(b)
	}
	if has(encode(v(10, 16, 0)), "Variables") {
		t.Error("10.16: Variables emitted, want omitted")
	}
	if !has(encode(v(10, 17, 0)), "Variables") {
		t.Error("10.17: Variables omitted, want emitted")
	}
	if !has(encode(v(11, 13, 0)), "Variables") {
		t.Error("11.13: Variables omitted, want emitted")
	}
}

// Whatever the version, the document's own identity and structure must survive —
// a suppression that over-reached would pass the tests above and write a page
// Mendix cannot load.
func TestPageKeepsItsStructureAtEveryVersion(t *testing.T) {
	for _, pv := range []*types.ProjectVersion{v(9, 24, 0), v(10, 24, 25), v(11, 13, 0), nil} {
		raw := pageKeys(t, pv)
		for _, key := range []string{"$ID", "$Type", "Name", "Parameters", "Title", "Appearance"} {
			if !has(raw, key) {
				t.Errorf("pv=%v: %s missing", pv, key)
			}
		}
		if got := raw.Lookup("$Type").StringValue(); got != "Forms$Page" {
			t.Errorf("pv=%v: $Type = %q", pv, got)
		}
	}
}

// Suppressing the empty Variables marker is right; suppressing variables the
// script declared is not. Below the floor those are refused, because silently
// dropping them leaves widgets referencing names that are gone (CE1151) from a
// statement that reported success — guard-don't-drop, ADR-0005.
func TestPageWithVariablesIsRefusedBelow10_17(t *testing.T) {
	withVars := func() *pages.Page {
		p := &pages.Page{
			Name:      "P",
			Variables: []*pages.LocalVariable{{Name: "Flag", DefaultValue: "true"}},
		}
		p.ID = "1"
		return p
	}

	_, err := encodePage(withVars(), v(10, 16, 0), nil)
	if err == nil {
		t.Fatal("10.16: a page with variables was encoded, want a refusal")
	}
	for _, want := range []string{"Flag", "10.17", "10.16"} {
		if !strings.Contains(err.Error(), want) {
			// Name the variable count, the floor, and the project's own version —
			// an error missing any of them cannot be acted on.
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// At and above the floor the same page encodes, with its variables.
	b, err := encodePage(withVars(), v(10, 17, 0), nil)
	if err != nil {
		t.Fatalf("10.17: %v", err)
	}
	if !has(bson.Raw(b), "Variables") {
		t.Error("10.17: Variables missing from a page that declares one")
	}
}

// The merge of #541 (carry the stored header) and this fix (do not write a key
// the project's version does not declare) has a case neither had alone: a
// rewrite whose STORED document already carries Autofocus on a project below
// 11.1. A pre-fix mxcli wrote exactly that, so the stored value can itself be
// the defect — carrying it would make the repair a no-op.
//
// The fixture project is 11.6.6, so the version is passed explicitly rather than
// read off the backend: what is under test is the decision, against a real
// stored document that really does carry the key.
func TestStoredAutofocusIsNotCarriedBelow11_1(t *testing.T) {
	b, page := headerFixture(t)
	setStoredHeader(t, b, page.ID, "Off", int64(800), int64(500))

	carried := func(pv *types.ProjectVersion) string {
		t.Helper()
		g := genPg.NewPage()
		b.carryStoredPageHeader(page.ID, g, pv)
		return g.Autofocus()
	}

	// Control first: at and above the floor the stored value IS carried, so a
	// failure below it cannot be "the carry never works".
	if got := carried(v(11, 6, 6)); got != "Off" {
		t.Fatalf("11.6.6: Autofocus = %q, want the stored Off", got)
	}
	if got := carried(v(11, 1, 0)); got != "Off" {
		t.Errorf("11.1.0 (the floor): Autofocus = %q, want the stored Off", got)
	}
	for _, pv := range []*types.ProjectVersion{v(10, 24, 25), v(11, 0, 0), nil} {
		if got := carried(pv); got == "Off" {
			t.Errorf("pv=%v: carried the stored Autofocus into a project that cannot declare it", pv)
		}
	}
}
