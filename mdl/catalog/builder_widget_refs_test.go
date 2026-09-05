// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"
)

// seedWidgetRefFixture builds a small project in an in-memory catalog:
//
//	Sales.OrderList   3 comboboxes, 1 datagrid, 2 dynamic texts (built-in)
//	Sales.OrderForm   1 combobox
//	Sales.AddressSnip 1 combobox   (a SNIPPET, not a page)
//
// The three comboboxes on one page are what proves DISTINCT; the dynamic texts
// are what proves a built-in gets no edge.
func seedWidgetRefFixture(t *testing.T) *Catalog {
	t.Helper()
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()

	defs := []struct{ id, mdl string }{
		{"com.mendix.widget.web.combobox.Combobox", "COMBOBOX"},
		{"com.mendix.widget.web.datagrid.Datagrid", "DATAGRID"},
		{"com.acme.widget.Unused.Unused", "UNUSED"},
	}
	for _, d := range defs {
		if _, err := db.Exec(
			`INSERT INTO widget_definitions_data (WidgetId, MdlName, WidgetKind, ProjectId, SnapshotId)
			 VALUES (?, ?, 'pluggable', 'p', 's')`, d.id, d.mdl); err != nil {
			t.Fatalf("seed definition %s: %v", d.id, err)
		}
	}

	widgets := []struct{ id, wtype, container, ctype string }{
		{"w1", "com.mendix.widget.web.combobox.Combobox", "Sales.OrderList", "PAGE"},
		{"w2", "com.mendix.widget.web.combobox.Combobox", "Sales.OrderList", "PAGE"},
		{"w3", "com.mendix.widget.web.combobox.Combobox", "Sales.OrderList", "PAGE"},
		{"w4", "com.mendix.widget.web.datagrid.Datagrid", "Sales.OrderList", "PAGE"},
		{"w5", "Forms$DynamicText", "Sales.OrderList", "PAGE"},
		{"w6", "Forms$DynamicText", "Sales.OrderList", "PAGE"},
		{"w7", "com.mendix.widget.web.combobox.Combobox", "Sales.OrderForm", "PAGE"},
		{"w8", "com.mendix.widget.web.combobox.Combobox", "Sales.AddressSnip", "SNIPPET"},
	}
	for _, w := range widgets {
		if _, err := db.Exec(
			`INSERT INTO widgets_data (Id, Name, WidgetType, ContainerQualifiedName, ContainerType, ModuleName, ProjectId, SnapshotId)
			 VALUES (?, ?, ?, ?, ?, 'Sales', 'p', 's')`,
			w.id, w.id, w.wtype, w.container, w.ctype); err != nil {
			t.Fatalf("seed widget %s: %v", w.id, err)
		}
	}
	return cat
}

func runInsertWidgetRefs(t *testing.T, cat *Catalog) int {
	t.Helper()
	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	n, err := insertWidgetRefs(tx, "p", "s")
	if err != nil {
		tx.Rollback()
		t.Fatalf("insertWidgetRefs: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return n
}

// The edge itself: one row per container x widget definition, named by the MDL
// name, carrying the widget ID, and covering snippets as well as pages.
func TestInsertWidgetRefs_EmitsOneEdgePerContainer(t *testing.T) {
	cat := seedWidgetRefFixture(t)
	if n := runInsertWidgetRefs(t, cat); n != 4 {
		t.Fatalf("inserted %d edges, want 4: OrderList x COMBOBOX, OrderList x DATAGRID, OrderForm x COMBOBOX, AddressSnip x COMBOBOX", n)
	}
}

func TestInsertWidgetRefs_Rows(t *testing.T) {
	cat := seedWidgetRefFixture(t)
	runInsertWidgetRefs(t, cat)

	rows, err := cat.CatalogDB().Query(
		`SELECT SourceType, SourceName, TargetType, TargetName, TargetId, RefKind
		 FROM refs ORDER BY SourceName, TargetName`)
	if err != nil {
		t.Fatalf("query refs: %v", err)
	}
	defer rows.Close()

	type row struct{ srcType, src, tgtType, tgt, tgtID, kind string }
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.srcType, &r.src, &r.tgtType, &r.tgt, &r.tgtID, &r.kind); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}

	want := []row{
		{"SNIPPET", "Sales.AddressSnip", "WIDGET", "COMBOBOX", "com.mendix.widget.web.combobox.Combobox", RefKindWidget},
		{"PAGE", "Sales.OrderForm", "WIDGET", "COMBOBOX", "com.mendix.widget.web.combobox.Combobox", RefKindWidget},
		{"PAGE", "Sales.OrderList", "WIDGET", "COMBOBOX", "com.mendix.widget.web.combobox.Combobox", RefKindWidget},
		{"PAGE", "Sales.OrderList", "WIDGET", "DATAGRID", "com.mendix.widget.web.datagrid.Datagrid", RefKindWidget},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d:\n got: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

// Three comboboxes on one page are one edge, not three. Without DISTINCT this
// test sees 6 rows.
func TestInsertWidgetRefs_CollapsesInstances(t *testing.T) {
	cat := seedWidgetRefFixture(t)
	runInsertWidgetRefs(t, cat)

	var n int
	if err := cat.CatalogDB().QueryRow(
		`SELECT COUNT(*) FROM refs WHERE SourceName = 'Sales.OrderList' AND TargetName = 'COMBOBOX'`,
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("Sales.OrderList -> COMBOBOX = %d edges, want 1 — the page has three combobox instances", n)
	}
}

// A built-in widget stores its BSON $Type in the same column and has no
// definition. It must not produce an edge to a target nothing can resolve. The
// control is in the same fixture: the pluggable widgets on that same page DO
// get edges, so "no built-in edge" cannot pass by emitting nothing at all.
func TestInsertWidgetRefs_SkipsBuiltins(t *testing.T) {
	cat := seedWidgetRefFixture(t)
	runInsertWidgetRefs(t, cat)

	var builtin, pluggable int
	if err := cat.CatalogDB().QueryRow(
		`SELECT COUNT(*) FROM refs WHERE TargetName LIKE 'Forms$%' OR TargetName = 'DynamicText'`,
	).Scan(&builtin); err != nil {
		t.Fatalf("count builtin: %v", err)
	}
	if builtin != 0 {
		t.Errorf("built-in widgets produced %d edges, want 0", builtin)
	}
	if err := cat.CatalogDB().QueryRow(
		`SELECT COUNT(*) FROM refs WHERE SourceName = 'Sales.OrderList'`,
	).Scan(&pluggable); err != nil {
		t.Fatalf("count pluggable: %v", err)
	}
	if pluggable != 2 {
		t.Errorf("control: Sales.OrderList has %d edges, want 2 — if this is 0 the test above proves nothing", pluggable)
	}
}

// An installed .mpk no page uses gets no edge, which is what makes "unused
// widget package" answerable.
func TestInsertWidgetRefs_UnusedDefinitionHasNoEdge(t *testing.T) {
	cat := seedWidgetRefFixture(t)
	runInsertWidgetRefs(t, cat)

	var n int
	if err := cat.CatalogDB().QueryRow(
		`SELECT COUNT(*) FROM refs WHERE TargetName = 'UNUSED'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("UNUSED has %d inbound edges, want 0", n)
	}
}

// Why TargetName is the MDL name and not the widget ID.
//
// graph_module_coupling and graph_module_cohesion derive a module by taking
// everything before the FIRST dot. That is sound for a qualified name and
// nonsense for a dotted widget ID: measured on testdata/expr-checker, using the
// widget ID invented a module called "com" carrying 14 edges from three real
// modules. A non-dotted MDL name is skipped by those views' own
// instr(TargetName, '.') > 0 guard.
//
// The control is the second half: with the widget ID written into the same
// fixture, the fake module DOES appear — so this asserts a property of the
// choice, not of the fixture.
func TestInsertWidgetRefs_MdlNameKeepsModuleViewsClean(t *testing.T) {
	cat := seedWidgetRefFixture(t)
	runInsertWidgetRefs(t, cat)

	countCoupling := func(target string) int {
		var n int
		if err := cat.CatalogDB().QueryRow(
			`SELECT COUNT(*) FROM graph_module_coupling WHERE TargetModule = ?`, target,
		).Scan(&n); err != nil {
			t.Fatalf("query graph_module_coupling: %v", err)
		}
		return n
	}

	if n := countCoupling("com"); n != 0 {
		t.Errorf("graph_module_coupling has %d rows for a module 'com', want 0", n)
	}

	// Control: the widget ID as TargetName does invent that module.
	if _, err := cat.CatalogDB().Exec(
		`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		 VALUES ('PAGE', '', 'Sales.OrderList', 'WIDGET', '', 'com.mendix.widget.web.combobox.Combobox', ?, 'Sales', 'p', 's')`,
		RefKindWidget); err != nil {
		t.Fatalf("seed control row: %v", err)
	}
	if n := countCoupling("com"); n == 0 {
		t.Error("control: a dotted widget ID as TargetName should invent a module 'com' — if it does not, this test cannot detect the problem it exists for")
	}
}

// A widget definition belongs to no Mendix module, so it must not be listed as
// an asset in graph_god_nodes, where every other row is a module-qualified
// document. The page's out-degree still counts it.
func TestWidgetRefsStayOffTheGodNodeAssetList(t *testing.T) {
	cat := seedWidgetRefFixture(t)
	runInsertWidgetRefs(t, cat)

	var asAsset int
	if err := cat.CatalogDB().QueryRow(
		`SELECT COUNT(*) FROM graph_god_nodes WHERE Asset IN ('COMBOBOX', 'DATAGRID')`,
	).Scan(&asAsset); err != nil {
		t.Fatalf("query graph_god_nodes: %v", err)
	}
	if asAsset != 0 {
		t.Errorf("graph_god_nodes lists %d widget definitions as assets, want 0", asAsset)
	}

	var outDeg int
	if err := cat.CatalogDB().QueryRow(
		`SELECT OutDegree FROM graph_god_nodes WHERE Asset = 'Sales.OrderList'`,
	).Scan(&outDeg); err != nil {
		t.Fatalf("query OutDegree: %v", err)
	}
	if outDeg != 2 {
		t.Errorf("Sales.OrderList OutDegree = %d, want 2 — the page's dependency on the widgets it uses is real and must survive", outDeg)
	}
}
