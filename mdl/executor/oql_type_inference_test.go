// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestExtractSelectClause guards against the case-comparison regression where
// extractSelectClause searched an uppercased query for a lowercase "select"
// needle (and compared uppercased slices to lowercase "from"/"union"), so it
// returned "" for every real query. That made `check --references` both
// false-positive ("could not parse select clause from OQL query") and silently
// skip all OQL type validation. Bug 9b.
func TestExtractSelectClause(t *testing.T) {
	cases := []struct {
		name string
		oql  string
		want string
	}{
		{
			name: "lowercase group-by aggregate",
			oql:  "select e.Status, count(e.ID) as StatusCount from ExpenseApproval.Expense e group by e.Status",
			want: "e.Status, count(e.ID) as StatusCount",
		},
		{
			name: "uppercase keywords",
			oql:  "SELECT e.Status FROM Mod.E e",
			want: "e.Status",
		},
		{
			name: "mixed case",
			oql:  "Select a From B.C",
			want: "a",
		},
		{
			// OQL always requires a FROM; a FROM-less query is malformed, so
			// returning "" (→ "could not parse select clause") is acceptable here.
			name: "no from clause returns empty",
			oql:  "select 1",
			want: "",
		},
		{
			name: "from inside subquery is not the main FROM",
			oql:  "select (select count(x.ID) from Mod.X x) as N from Mod.Y y",
			want: "(select count(x.ID) from Mod.X x) as N",
		},
		{
			name: "union ends the first query term",
			oql:  "select a from Mod.A union select b from Mod.B",
			want: "a",
		},
		{
			name: "no select keyword",
			oql:  "update Mod.E set x = 1",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractSelectClause(tc.oql)
			if got != tc.want {
				t.Errorf("extractSelectClause(%q) = %q, want %q", tc.oql, got, tc.want)
			}
		})
	}
}

// TestValidateOQLTypesNoFalsePositive guards the other half of bug 9b: with
// extractSelectClause working again, ValidateOQLTypes actually runs — it must
// not over-fire on the valid view-entity queries the report cites. The static
// inferrer returns TypeUnknown for anything it can't confidently type
// (attribute refs, division), so those columns are skipped rather than flagged.
func TestValidateOQLTypesNoFalsePositive(t *testing.T) {
	cases := []struct {
		name  string
		oql   string
		attrs []ast.ViewAttribute
	}{
		{
			name: "trivial group-by aggregate",
			oql:  "select e.Status, count(e.ID) as StatusCount from ExpenseApproval.Expense e group by e.Status",
			attrs: []ast.ViewAttribute{
				{Name: "StatusValue", Type: ast.DataType{Kind: ast.TypeString, Length: 200}}, // e.Status → Unknown, skipped
				{Name: "StatusCount", Type: ast.DataType{Kind: ast.TypeInteger}},             // count() → Integer, matches
			},
		},
		{
			name: "case/division aggregate is not flagged",
			oql:  "select e.Category, sum(e.Amount) / count(e.ID) as AvgAmount from ExpenseApproval.Expense e group by e.Category",
			attrs: []ast.ViewAttribute{
				{Name: "Category", Type: ast.DataType{Kind: ast.TypeString, Length: 200}},
				{Name: "AvgAmount", Type: ast.DataType{Kind: ast.TypeDecimal}}, // division → Unknown, skipped
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violations := ValidateOQLTypes(tc.oql, tc.attrs)
			if len(violations) != 0 {
				for _, v := range violations {
					t.Errorf("unexpected violation: %s — %s", v.RuleID, v.Message)
				}
			}
		})
	}
}

// TestValidateOQLTypesDerivedString covers the CE6770 case: a DERIVED string
// column (CAST(x AS string) or a string-returning CASE) is normalized by Mendix
// to the platform default length (200). Declaring any other length builds fine
// in mxcli but makes mxbuild report CE6770 "View Entity is out of sync". The
// checker must flag the mismatch pre-build (MDL031) and accept string(200).
func TestValidateOQLTypesDerivedString(t *testing.T) {
	cases := []struct {
		name      string
		oql       string
		attrs     []ast.ViewAttribute
		wantMDL31 bool
	}{
		{
			name: "CAST enum to string declared short is flagged",
			oql:  "select cast(e.Status as string) as StatusName from Mod.E e",
			attrs: []ast.ViewAttribute{
				{Name: "StatusName", Type: ast.DataType{Kind: ast.TypeString, Length: 30}},
			},
			wantMDL31: true,
		},
		{
			name: "CAST to string declared 200 passes",
			oql:  "select cast(e.Status as string) as StatusName from Mod.E e",
			attrs: []ast.ViewAttribute{
				{Name: "StatusName", Type: ast.DataType{Kind: ast.TypeString, Length: 200}},
			},
			wantMDL31: false,
		},
		{
			name: "CAST to string declared unlimited is flagged",
			oql:  "select cast(e.Status as string) as StatusName from Mod.E e",
			attrs: []ast.ViewAttribute{
				{Name: "StatusName", Type: ast.DataType{Kind: ast.TypeString, Length: 0}},
			},
			wantMDL31: true,
		},
		{
			name: "string-returning CASE declared short is flagged",
			oql:  "select case when e.Amount > 100 then 'High' else 'Low' end as Bucket from Mod.E e",
			attrs: []ast.ViewAttribute{
				{Name: "Bucket", Type: ast.DataType{Kind: ast.TypeString, Length: 10}},
			},
			wantMDL31: true,
		},
		{
			name: "string-returning CASE declared 200 passes",
			oql:  "select case when e.Amount > 100 then 'High' else 'Low' end as Bucket from Mod.E e",
			attrs: []ast.ViewAttribute{
				{Name: "Bucket", Type: ast.DataType{Kind: ast.TypeString, Length: 200}},
			},
			wantMDL31: false,
		},
		{
			name: "CAST to integer declared integer passes",
			oql:  "select cast(e.Code as integer) as CodeNum from Mod.E e",
			attrs: []ast.ViewAttribute{
				{Name: "CodeNum", Type: ast.DataType{Kind: ast.TypeInteger}},
			},
			wantMDL31: false,
		},
		{
			name: "CAST to integer declared decimal is flagged",
			oql:  "select cast(e.Code as integer) as CodeNum from Mod.E e",
			attrs: []ast.ViewAttribute{
				{Name: "CodeNum", Type: ast.DataType{Kind: ast.TypeDecimal}},
			},
			wantMDL31: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violations := ValidateOQLTypes(tc.oql, tc.attrs)
			got := false
			for _, v := range violations {
				if v.RuleID == "MDL031" {
					got = true
				}
			}
			if got != tc.wantMDL31 {
				t.Errorf("ValidateOQLTypes MDL031=%v, want %v (violations=%v)", got, tc.wantMDL31, violations)
			}
		})
	}
}
