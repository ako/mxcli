// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// dsBSON builds a datasource map in the shape page BSON stores.
func dsBSON(dsType string, kv ...any) map[string]any {
	m := map[string]any{"$Type": dsType}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

// TestParseDataSourceCoversEveryStoredType is the read half of #941. The
// pluggable copy of this switch knew microflows and nanoflows but not
// associations, selection targets or context sources — so a Gallery bound over
// an association described back with no datasource, while a ListView bound the
// same way kept it.
func TestParseDataSourceCoversEveryStoredType(t *testing.T) {
	cases := []struct {
		name    string
		ds      map[string]any
		wantTyp string
		wantRef string
	}{
		{
			name:    "microflow",
			ds:      dsBSON(dsTypeMicroflow, "MicroflowSettings", map[string]any{"Microflow": "Mod.DS_GetItems"}),
			wantTyp: "microflow", wantRef: "Mod.DS_GetItems",
		},
		{
			name:    "association — dropped entirely by the pluggable reader",
			ds:      dsBSON(dsTypeAssociation, "EntityRef", map[string]any{"Steps": []any{map[string]any{"Association": "Mod.Item_Bucket"}}}),
			wantTyp: "association", wantRef: "Mod.Item_Bucket",
		},
		{
			name:    "selection — dropped entirely by the pluggable reader",
			ds:      dsBSON(dsTypeListenTarget, "ListenTarget", "gallery1"),
			wantTyp: "selection", wantRef: "gallery1",
		},
		{
			name:    "page parameter",
			ds:      dsBSON(dsTypeDataView, "SourceVariable", map[string]any{"PageParameter": "Bucket"}),
			wantTyp: "parameter", wantRef: "Bucket",
		},
		{
			name:    "database via EntityRef",
			ds:      dsBSON(dsTypeDatabase, "EntityRef", map[string]any{"Entity": "Mod.Item"}),
			wantTyp: "database", wantRef: "Mod.Item",
		},
		{
			name:    "list view xpath source",
			ds:      dsBSON(dsTypeListViewXPath, "EntityRef", map[string]any{"Entity": "Mod.Item"}),
			wantTyp: "database", wantRef: "Mod.Item",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDataSource(tc.ds)
			if got == nil {
				t.Fatalf("datasource was dropped")
			}
			if got.Unsupported != "" {
				t.Fatalf("reported unsupported (%s)", got.Unsupported)
			}
			if got.Type != tc.wantTyp || got.Reference != tc.wantRef {
				t.Errorf("got Type=%q Reference=%q, want %q / %q", got.Type, got.Reference, tc.wantTyp, tc.wantRef)
			}
		})
	}
}

// TestParseDataSourceReadsAContextScopedXPathSource is symptom 1 of #941. Studio
// Pro stores a pluggable list bound over an association as an XPath source whose
// EntityRef is empty; reading only EntityRef yielded a database source with no
// entity, which the emitter rendered as `database from ` — and `mxcli check`
// then read `from` as the entity name.
func TestParseDataSourceReadsAContextScopedXPathSource(t *testing.T) {
	ds := dsBSON(dsTypeCustomWidgetXPath,
		"EntityRef", map[string]any{"Steps": []any{map[string]any{"Association": "Mod.Item_Bucket"}}},
	)
	got := parseDataSource(ds)
	if got == nil {
		t.Fatal("datasource was dropped")
	}
	if got.Type == "database" && got.Reference == "" {
		t.Fatalf("read as a database source with no entity — emits %q", "database from "+got.Reference)
	}
	if got.Type != "association" || got.Reference != "Mod.Item_Bucket" {
		t.Errorf("got Type=%q Reference=%q, want association / Mod.Item_Bucket", got.Type, got.Reference)
	}
}

// TestDataSourceExprNeverEmitsAnEmptyEntitySlot is the guard for the exact
// string in the report. Whatever a datasource turns out to be, describe must not
// produce `database from ` with nothing after it: that is not a near-miss, it is
// a statement the parser reads as an entity called "from".
func TestDataSourceExprNeverEmitsAnEmptyEntitySlot(t *testing.T) {
	for _, ds := range []*rawDataSource{
		{Type: "database", Reference: ""},
		{Type: "microflow", Reference: ""},
		{Type: "parameter", Reference: ""},
		{Type: "selection", Reference: ""},
		{Type: "association", Reference: ""},
		{Unsupported: "Forms$SomethingNew"},
	} {
		if expr := dataSourceExpr(ds); expr != "" {
			t.Errorf("%+v rendered as %q, want nothing", ds, expr)
		}
	}
}

// TestDataSourceExprKeepsTheType is symptom 2 of #941: the object-list emitter
// had no type switch at all, so a chart series bound to a microflow described as
// `database from Module.TheMicroflow` and re-executing it reported
// "entity not found".
func TestDataSourceExprKeepsTheType(t *testing.T) {
	cases := map[string]struct {
		ds   *rawDataSource
		want string
	}{
		"microflow": {&rawDataSource{Type: "microflow", Reference: "Mod.DS_GetItems"}, "microflow Mod.DS_GetItems"},
		"nanoflow":  {&rawDataSource{Type: "nanoflow", Reference: "Mod.NF_GetItems"}, "nanoflow Mod.NF_GetItems"},
		"database":  {&rawDataSource{Type: "database", Reference: "Mod.Item"}, "database from Mod.Item"},
		"selection": {&rawDataSource{Type: "selection", Reference: "gallery1"}, "selection gallery1"},
		"parameter": {&rawDataSource{Type: "parameter", Reference: "Bucket"}, "$Bucket"},
		"parameter already an expression": {
			&rawDataSource{Type: "parameter", Reference: "$Account/Mod.Assoc"}, "$Account/Mod.Assoc",
		},
		"association": {
			&rawDataSource{Type: "association", Reference: "Mod.Item_Bucket"}, "$currentObject/Mod.Item_Bucket",
		},
		// Bracketed, not bare: the emitter always uses the grammar's
		// xpathConstraint production, because it cannot tell which constraints
		// also happen to be valid MDL expressions (mxcli-formula1 §57.1).
		"database with where and sort": {
			&rawDataSource{
				Type: "database", Reference: "Mod.Item",
				XPathConstraint: "[IsActive = true]",
				SortColumns:     []rawSortColumn{{Attribute: "Name", Order: "asc"}},
			},
			"database from Mod.Item where [IsActive = true] sort by Name asc",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := dataSourceExpr(tc.ds); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseDataSourceReportsWhatItCannotSpell pins the third option. A
// datasource mxcli has no MDL word for is neither dropped (the binding
// disappears with no trace) nor rendered as a guess (a statement that will not
// re-execute) — it comes back as a comment naming the stored type.
func TestParseDataSourceReportsWhatItCannotSpell(t *testing.T) {
	got := parseDataSource(dsBSON("Forms$SomeFutureSource", "Whatever", "x"))
	if got == nil {
		t.Fatal("an unknown datasource was dropped silently")
	}
	if got.Unsupported != "Forms$SomeFutureSource" {
		t.Errorf("Unsupported = %q, want the stored $Type", got.Unsupported)
	}
	comment := dataSourceComment(got)
	if !strings.Contains(comment, "Forms$SomeFutureSource") {
		t.Errorf("comment %q does not name the stored type", comment)
	}
	if dataSourceExpr(got) != "" {
		t.Error("an unspellable datasource still rendered an expression")
	}
}

// TestParseSortColumnsAcceptsBothStoredShapes pins that sorting is read
// wherever it lives. Grid-like datasources carry a GridSortBar and list views a
// Sort.Paths; a reader that knew only one silently dropped the other's sort.
func TestParseSortColumnsAcceptsBothStoredShapes(t *testing.T) {
	sortItem := func(attr, dir string) map[string]any {
		return map[string]any{
			"AttributeRef":  map[string]any{"Attribute": attr},
			"SortDirection": dir,
		}
	}
	sortBar := parseSortColumns(map[string]any{
		"SortBar": map[string]any{"SortItems": []any{sortItem("Mod.Item.Name", "Ascending")}},
	})
	paths := parseSortColumns(map[string]any{
		"Sort": map[string]any{"Paths": []any{sortItem("Mod.Item.Name", "Descending")}},
	})
	if len(sortBar) != 1 || sortBar[0].Attribute != "Name" || sortBar[0].Order != "asc" {
		t.Errorf("SortBar shape gave %+v", sortBar)
	}
	if len(paths) != 1 || paths[0].Attribute != "Name" || paths[0].Order != "desc" {
		t.Errorf("Sort.Paths shape gave %+v", paths)
	}
}

// TestXPathConstraintClauseParses covers the regression in mxcli-formula1 §57.1.
//
// Restoring the WHERE clause (defect 4 of #941) collided with an older gap:
// the constraint was emitted raw, so a stored XPath containing a predicate —
// `System.grantableRoles[reversed()]/…`, which stock Administration.Account_New
// carries — produced MDL that no longer parsed. The page describing clean
// BEFORE the fix and failing after is the worst shape a fix can have.
//
// The emitted form is always the grammar's own `xpathConstraint` production
// (bracketed), never a bare expression. The bare form parses only for simple
// comparisons, and "does this particular XPath happen to be a valid MDL
// expression" is not a question the emitter can answer — while a bracketed
// group is accepted for every constraint.
func TestXPathConstraintClauseParses(t *testing.T) {
	cases := map[string]string{
		"simple comparison":  "Qty > 5",
		"already bracketed":  "[Qty > 5]",
		"inner predicate":    "[System.grantableRoles[reversed()]/System.UserRole/System.UserRoles = '[%CurrentUser%]']",
		"two groups":         "[Active = true][Qty > 5]",
		"bracket in literal": "[Name = 'a]b']",
	}
	for name, stored := range cases {
		t.Run(name, func(t *testing.T) {
			got := xpathConstraintClause(stored)
			if got == "" {
				t.Fatalf("rendered nothing for %q", stored)
			}
			if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
				t.Errorf("rendered %q — want bracketed groups the page grammar accepts", got)
			}
		})
	}
}

// TestXPathConstraintClauseKeepsGroupsIntact pins the trap the old code fell
// into: it stripped the outer brackets by looking at the first and last byte,
// so `[a][b]` became `a][b`. That is the same mangling #772 fixed for the
// microflow emitter, which is why this reuses that fix's splitter rather than
// keeping a second, simpler copy of the logic.
func TestXPathConstraintClauseKeepsGroupsIntact(t *testing.T) {
	got := xpathConstraintClause("[Active = true][Qty > 5]")
	if strings.Contains(got, "a][b") || strings.Count(got, "[") != 2 {
		t.Errorf("rendered %q — want both predicate groups preserved", got)
	}
}

// TestXPathConstraintClauseIgnoresBlank guards mxcli-formula1 §57.3: the old
// guard was `strings.Trim(s, "")`, whose empty cutset trims nothing, so a
// whitespace-only constraint passed the != "" test and emitted a bare `where`.
func TestXPathConstraintClauseIgnoresBlank(t *testing.T) {
	for _, blank := range []string{"", "   ", "\t\n"} {
		if got := xpathConstraintClause(blank); got != "" {
			t.Errorf("xpathConstraintClause(%q) = %q, want empty", blank, got)
		}
	}
}
