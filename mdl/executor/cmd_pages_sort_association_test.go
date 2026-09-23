// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1152, page half. A page datasource's `sort by` is stored as
// the same DomainModels$AttributeRef a microflow retrieve's sort is, so it has
// the same two halves: the hops must be READ (or DESCRIBE drops them) and
// EMITTED as `Assoc/Attr` (or the replay has to guess which association).

// TestParseSortColumns_ReadsAssociationHops is the read half.
func TestParseSortColumns_ReadsAssociationHops(t *testing.T) {
	ds := map[string]any{
		"SortBar": map[string]any{
			"SortItems": []any{
				map[string]any{
					"SortDirection": "Ascending",
					"AttributeRef": map[string]any{
						"Attribute": "Sales.Address.City",
						"EntityRef": map[string]any{
							"Steps": []any{
								map[string]any{
									"Association":       "Sales.Order_BillTo",
									"DestinationEntity": "Sales.Address",
								},
							},
						},
					},
				},
			},
		},
	}
	cols := parseSortColumns(ds)
	if len(cols) != 1 {
		t.Fatalf("got %d sort columns, want 1", len(cols))
	}
	if got := strings.Join(cols[0].Associations, ","); got != "Sales.Order_BillTo" {
		t.Errorf("Associations = %q, want Sales.Order_BillTo — the hop is invisible to "+
			"DESCRIBE without it", got)
	}
	if got := sortColumnPath(cols[0]); got != "Sales.Order_BillTo/City" {
		t.Errorf("rendered %q, want Sales.Order_BillTo/City", got)
	}
}

// CONTROL: an own-entity sort reads back with no hops and renders as the bare
// attribute — a reader that manufactured an empty hop would emit a stray `/`.
func TestParseSortColumns_PlainSortHasNoHops(t *testing.T) {
	ds := map[string]any{
		"SortBar": map[string]any{
			"SortItems": []any{
				map[string]any{
					"SortDirection": "Descending",
					"AttributeRef":  map[string]any{"Attribute": "Sales.Order.OrderNo"},
				},
			},
		},
	}
	cols := parseSortColumns(ds)
	if len(cols) != 1 {
		t.Fatalf("got %d sort columns, want 1", len(cols))
	}
	if len(cols[0].Associations) != 0 {
		t.Errorf("Associations = %v, want none", cols[0].Associations)
	}
	if got := sortColumnPath(cols[0]); got != "OrderNo" {
		t.Errorf("rendered %q, want OrderNo", got)
	}
}

// TestDataSourceExpr_EmitsSortAssociationPath is the emit half: the rendered
// datasource must carry the hop, because that text is what a person replays.
func TestDataSourceExpr_EmitsSortAssociationPath(t *testing.T) {
	ds := &rawDataSource{
		Type:      "database",
		Reference: "Sales.Order",
		SortColumns: []rawSortColumn{
			{Attribute: "City", Associations: []string{"Sales.Order_BillTo"}, Order: "asc"},
			{Attribute: "OrderNo", Order: "desc"},
		},
	}
	got := dataSourceExpr(ds)
	want := "database from Sales.Order sort by Sales.Order_BillTo/City asc, OrderNo desc"
	if got != want {
		t.Errorf("dataSourceExpr =\n  %q\nwant\n  %q", got, want)
	}
}

// A GridSort carrying hops must reach the writer with them attached — the field
// exists so the page writer can build the EntityRef, and a builder that resolved
// the path but dropped the steps would store an attribute of a far entity with
// nothing to reach it, which is CE7247.
func TestGridSortCarriesAttributeRefSteps(t *testing.T) {
	s := &pages.GridSort{
		AttributePath:     "Sales.Address.City",
		AttributeRefSteps: []pages.AttributeRefStep{{Association: "Sales.Order_BillTo", DestinationEntity: "Sales.Address"}},
		Direction:         pages.SortDirectionAscending,
	}
	if len(s.AttributeRefSteps) != 1 || s.AttributeRefSteps[0].Association != "Sales.Order_BillTo" {
		t.Fatalf("GridSort hops = %+v", s.AttributeRefSteps)
	}
}
