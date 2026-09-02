// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1018 — a rewrite that does not restate a `/** … */` doc
// comment must not delete the stored one.
//
// Measured before the fix: `create or replace microflow` and `create or modify
// entity` both wrote an empty Documentation over the stored value, with the run
// reporting success and mx check clean (a document with no documentation is
// valid). `ALTER ENTITY … ADD ATTRIBUTE` preserved it, which is what localises
// the defect to the rewrite paths.
//
// Table-driven over every doctype that accepts a doc comment, because the
// carry-forward has to be added per rewrite path and fixing only the two
// doctypes in the report would leave the class open — the shape the wiki calls
// duplicate-resolver-drift.

type docPreserveCase struct {
	name string
	// create carries a doc comment; rewrite deliberately does not.
	create   string
	rewrite  string
	describe string
}

const docMarker = "DOC-MARKER-PRESERVE-ME"

func docPreserveCases() []docPreserveCase {
	doc := "/** " + docMarker + " */\n"
	return []docPreserveCase{
		{
			name:     "entity",
			create:   doc + "create entity TestModule.DocEnt ( Label: String );",
			rewrite:  "create or modify entity TestModule.DocEnt ( Label: String, Extra: Integer );",
			describe: "describe entity TestModule.DocEnt",
		},
		{
			name:     "microflow",
			create:   doc + "create microflow TestModule.DocMf ()\nbegin\nend;",
			rewrite:  "create or replace microflow TestModule.DocMf ()\nbegin\nend;",
			describe: "describe microflow TestModule.DocMf",
		},
		{
			name:     "enumeration",
			create:   doc + "create enumeration TestModule.DocEnum ( A 'A', B 'B' );",
			rewrite:  "create or replace enumeration TestModule.DocEnum ( A 'A', B 'B', C 'C' );",
			describe: "describe enumeration TestModule.DocEnum",
		},
	}
}

// TestDocumentation_SurvivesRewrite is the defect itself.
func TestDocumentation_SurvivesRewrite(t *testing.T) {
	for _, tc := range docPreserveCases() {
		t.Run(tc.name, func(t *testing.T) {
			env := setupTestEnv(t)
			defer env.teardown()

			if err := env.executeMDL(tc.create); err != nil {
				t.Fatalf("create: %v", err)
			}
			before, err := env.describeMDL(tc.describe)
			if err != nil {
				t.Fatalf("describe after create: %v", err)
			}
			if !strings.Contains(before, docMarker) {
				t.Fatalf("precondition failed: the doc comment did not survive CREATE, so this test "+
					"cannot say anything about the rewrite:\n%s", before)
			}

			if err := env.executeMDL(tc.rewrite); err != nil {
				t.Fatalf("rewrite: %v", err)
			}
			after, err := env.describeMDL(tc.describe)
			if err != nil {
				t.Fatalf("describe after rewrite: %v", err)
			}
			if !strings.Contains(after, docMarker) {
				t.Errorf("the rewrite deleted the documentation it never mentioned (#1018):\n%s", after)
			}
		})
	}
}

// TestDocumentation_UntouchedObjectKeepsItsOwn is the control that separates
// "rewrites drop documentation" from "writes drop documentation". Without it a
// green run proves only that nothing was written at all.
func TestDocumentation_UntouchedObjectKeepsItsOwn(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(
		"/** " + docMarker + " */\ncreate entity TestModule.DocBystander ( Label: String );",
	); err != nil {
		t.Fatalf("create bystander: %v", err)
	}
	if err := env.executeMDL(
		"/** unrelated */\ncreate microflow TestModule.DocOther ()\nbegin\nend;",
	); err != nil {
		t.Fatalf("create other: %v", err)
	}
	// Rewrite the OTHER document.
	if err := env.executeMDL(
		"create or replace microflow TestModule.DocOther ()\nbegin\nend;",
	); err != nil {
		t.Fatalf("rewrite other: %v", err)
	}

	out, err := env.describeMDL("describe entity TestModule.DocBystander")
	if err != nil {
		t.Fatalf("describe bystander: %v", err)
	}
	if !strings.Contains(out, docMarker) {
		t.Errorf("a bystander lost its documentation when a different document was rewritten:\n%s", out)
	}
}

// TestDocumentation_EmptyCommentClears pins the other half of the decision:
// an ABSENT doc comment preserves, an explicitly EMPTY one clears. Without this
// the fix makes documentation unclearable — and for a microflow there is no
// `ALTER MICROFLOW … SET DOCUMENTATION` to fall back on, so the empty comment is
// the only spelling available.
func TestDocumentation_EmptyCommentClears(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(
		"/** " + docMarker + " */\ncreate microflow TestModule.DocClear ()\nbegin\nend;",
	); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := env.executeMDL(
		"/** */\ncreate or replace microflow TestModule.DocClear ()\nbegin\nend;",
	); err != nil {
		t.Fatalf("rewrite with empty doc comment: %v", err)
	}
	out, err := env.describeMDL("describe microflow TestModule.DocClear")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if strings.Contains(out, docMarker) {
		t.Errorf("an explicitly empty /** */ did not clear the documentation:\n%s", out)
	}
}
