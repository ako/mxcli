// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"regexp"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// notAutoDescribable lists the catalog `objects` view ObjectTypes that bare
// `DESCRIBE Module.Name` deliberately does not resolve, each with the reason.
// Everything else in the view must have an entry in objectTypeToDescribeKind.
//
// The two lists are maintained in different packages and drifted apart once:
// BUILDING_BLOCK and ICON_COLLECTION had working explicit describe handlers
// (`describe building block M.N`) while bare `describe M.N` reported "no
// describable document named ..." — 43 of the 251 documents in a 7-module
// marketplace project were unreachable that way. This test is the guard.
var notAutoDescribable = map[string]string{
	// Contract-derived rows, read from a cached $metadata / AsyncAPI document
	// rather than from the project — there is no unit in the .mpr to describe.
	"CONTRACT_ENTITY":  "parsed from cached $metadata, not a project document",
	"CONTRACT_ACTION":  "parsed from cached $metadata, not a project document",
	"CONTRACT_MESSAGE": "parsed from cached AsyncAPI, not a project document",
	"EXTERNAL_ACTION":  "exposed by a consumed service, not a project document",
	"BUSINESS_EVENT":   "exposed by a consumed service, not a project document",

	// Project-level, not module-scoped documents.
	"JAR_DEPENDENCY": "a build-time dependency coordinate, not a document",
}

// objectTypesInView extracts the ObjectType literals the `objects` view can
// emit, straight from the view's own SQL, so the test cannot go stale against a
// hand-maintained copy of the list.
func objectTypesInView(t *testing.T) []string {
	t.Helper()

	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	defer cat.Close()

	row := cat.CatalogDB().QueryRow(
		"SELECT sql FROM sqlite_master WHERE type = 'view' AND name = 'objects'")
	var sql string
	if err := row.Scan(&sql); err != nil {
		t.Fatalf("read objects view SQL: %v", err)
	}

	re := regexp.MustCompile(`'([A-Z_]+)' as ObjectType`)
	matches := re.FindAllStringSubmatch(sql, -1)
	if len(matches) == 0 {
		t.Fatal("no ObjectType literals found in the objects view SQL")
	}

	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// TestDescribeAutoCoversCatalogObjectTypes asserts that every ObjectType the
// catalog's objects view can emit is either auto-describable or explicitly
// exempted. A new document type added to the view without a describe kind fails
// here rather than silently becoming unreachable via bare DESCRIBE.
func TestDescribeAutoCoversCatalogObjectTypes(t *testing.T) {
	for _, ot := range objectTypesInView(t) {
		if _, ok := objectTypeToDescribeKind[ot]; ok {
			continue
		}
		if _, exempt := notAutoDescribable[ot]; exempt {
			continue
		}
		t.Errorf("ObjectType %q is emitted by the catalog objects view but has no "+
			"entry in objectTypeToDescribeKind and is not listed in notAutoDescribable; "+
			"bare `DESCRIBE Module.Name` will report it as not found", ot)
	}
}

// TestDescribeAutoExemptionsAreReal guards the other direction: an exemption for
// an ObjectType the view no longer emits is dead weight that hides the next gap.
func TestDescribeAutoExemptionsAreReal(t *testing.T) {
	emitted := map[string]bool{}
	for _, ot := range objectTypesInView(t) {
		emitted[ot] = true
	}
	for ot := range notAutoDescribable {
		if !emitted[ot] {
			t.Errorf("notAutoDescribable lists %q, which the objects view no longer emits; "+
				"remove the exemption", ot)
		}
	}
}

// TestDescribeAutoResolvesReusableUIDocuments pins the specific regression:
// building blocks and icon collections are real, named, module-scoped documents
// (40 and 3 of them respectively in a stock 7-module marketplace project) and
// must resolve without the caller naming the type.
func TestDescribeAutoResolvesReusableUIDocuments(t *testing.T) {
	for _, ot := range []string{"BUILDING_BLOCK", "ICON_COLLECTION"} {
		if _, ok := objectTypeToDescribeKind[ot]; !ok {
			t.Errorf("objectTypeToDescribeKind is missing %q, so bare `DESCRIBE Module.Name` "+
				"cannot resolve it even though an explicit describe handler exists", ot)
		}
	}
}
