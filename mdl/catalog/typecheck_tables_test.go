// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// The three lookups expression type checking needs, and which the catalog could
// not answer before schema 10. Each is tested at the schema level — that the
// column or table exists and holds what a reader will select — because the
// failure mode is not an error but an empty answer, which a checker reads as
// "cannot tell" and silently skips.

func TestAttributesCarryTheEnumerationQualifiedName(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()

	if _, err := cat.CatalogDB().Exec(
		`INSERT INTO attributes_data (Id, Name, EntityQualifiedName, DataType, EnumerationQualifiedName)
		 VALUES ('a1', 'Status', 'Shop.Order', 'Enumeration', 'Shop.OrderStatus')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var got string
	if err := cat.CatalogDB().QueryRow(
		`SELECT EnumerationQualifiedName FROM attributes WHERE Id = 'a1'`,
	).Scan(&got); err != nil {
		t.Fatalf("select: %v", err)
	}
	if got != "Shop.OrderStatus" {
		t.Errorf("got %q, want the enum's qualified name", got)
	}
}

func TestEnumerationValuesTableIsQueryable(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()

	for _, v := range []struct {
		name    string
		ordinal int
	}{{"Open", 0}, {"Closed", 1}} {
		if _, err := cat.CatalogDB().Exec(
			`INSERT INTO enumeration_values_data (Id, EnumerationQualifiedName, Name, Caption, Ordinal)
			 VALUES (?, 'Shop.OrderStatus', ?, ?, ?)`,
			"Shop.OrderStatus/"+v.name, v.name, v.name, v.ordinal,
		); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	rows, err := cat.CatalogDB().Query(
		`SELECT Name FROM enumeration_values WHERE EnumerationQualifiedName = 'Shop.OrderStatus' ORDER BY Ordinal`)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, n)
	}
	if len(got) != 2 || got[0] != "Open" || got[1] != "Closed" {
		t.Errorf("got %v, want [Open Closed] in ordinal order", got)
	}
}

func TestMicroflowParametersTableIsQueryable(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()

	if _, err := cat.CatalogDB().Exec(
		`INSERT INTO microflow_parameters_data (Id, MicroflowQualifiedName, Name, ParameterType, Ordinal)
		 VALUES ('p1', 'Shop.ACT_Place', 'Order', 'Object:Shop.Order', 0)`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var name, ptype string
	if err := cat.CatalogDB().QueryRow(
		`SELECT Name, ParameterType FROM microflow_parameters WHERE MicroflowQualifiedName = 'Shop.ACT_Place'`,
	).Scan(&name, &ptype); err != nil {
		t.Fatalf("select: %v", err)
	}
	if name != "Order" || ptype != "Object:Shop.Order" {
		t.Errorf("got (%q, %q), want the parameter name and its encoded type", name, ptype)
	}
}

// TestNewTablesAreListed pins that the additions show up in SHOW CATALOG TABLES.
// A table nothing lists is a table nobody discovers.
func TestNewTablesAreListed(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()

	listed := map[string]bool{}
	for _, name := range cat.Tables() {
		listed[name] = true
	}
	for _, want := range []string{"CATALOG.ENUMERATION_VALUES", "CATALOG.MICROFLOW_PARAMETERS"} {
		if !listed[want] {
			t.Errorf("%s is not in Tables()", want)
		}
	}
}
