// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

func TestObjectListMDLKey(t *testing.T) {
	cases := map[string]string{
		"staticXAttribute": "StaticXAttribute",
		"staticName":       "StaticName",
		"dataSet":          "DataSet",
		"interpolation":    "Interpolation",
		"":                 "",
	}
	for in, want := range cases {
		if got := objectListMDLKey(in); got != want {
			t.Errorf("objectListMDLKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// extractObjectListItem must reconstruct each sub-property kind of a chart series:
// per-item datasource, attribute bindings, a texttemplate name, and primitives.
func TestExtractObjectListItem_ChartSeries(t *testing.T) {
	const (
		idDataSet = "id-dataset"
		idDS      = "id-datasource"
		idX       = "id-x"
		idName    = "id-name"
	)
	nested := map[string]string{
		idDataSet: "dataSet",
		idDS:      "staticDataSource",
		idX:       "staticXAttribute",
		idName:    "staticName",
	}
	prop := func(typePtr string, value map[string]any) map[string]any {
		return map[string]any{"TypePointer": typePtr, "Value": value}
	}
	itemObj := map[string]any{
		"Properties": []any{
			int32(2), // BSON non-empty-array marker
			prop(idDataSet, map[string]any{"PrimitiveValue": "static"}),
			prop(idDS, map[string]any{"DataSource": map[string]any{
				"$Type":     "CustomWidgets$CustomWidgetXPathSource",
				"EntityRef": map[string]any{"Entity": "Sales.ByRegion"},
			}}),
			prop(idX, map[string]any{"AttributeRef": map[string]any{"Attribute": "Sales.ByRegion.Region"}}),
			prop(idName, map[string]any{"TextTemplate": map[string]any{
				"Template": map[string]any{"Items": []any{int32(2), map[string]any{"Text": "Revenue"}}},
			}}),
		},
	}

	item := extractObjectListItem(nil, itemObj, nested)

	if item.DataSource == nil || item.DataSource.Reference != "Sales.ByRegion" {
		t.Fatalf("DataSource = %#v, want reference Sales.ByRegion", item.DataSource)
	}
	got := map[string]struct {
		val   string
		isRef bool
	}{}
	for _, p := range item.Props {
		got[p.Key] = struct {
			val   string
			isRef bool
		}{p.Value, p.IsRef}
	}
	if p, ok := got["DataSet"]; !ok || p.val != "static" || p.isRef {
		t.Errorf("DataSet prop = %+v, want {static,false}", p)
	}
	if p, ok := got["StaticXAttribute"]; !ok || p.val != "Region" || !p.isRef {
		t.Errorf("StaticXAttribute prop = %+v, want {Region,true}", p)
	}
	if p, ok := got["StaticName"]; !ok || p.val != "Revenue" || p.isRef {
		t.Errorf("StaticName prop = %+v, want {Revenue,false}", p)
	}
	// The datasource sub-property must NOT also appear as a scalar prop.
	if _, ok := got["StaticDataSource"]; ok {
		t.Error("staticDataSource leaked into Props; should be the item DataSource")
	}
}

// Whitespace-only primitive defaults (e.g. customSeriesOptions: " ") are noise
// and must be dropped.
func TestExtractObjectListItem_SkipsWhitespacePrimitive(t *testing.T) {
	nested := map[string]string{"id-opt": "customSeriesOptions"}
	itemObj := map[string]any{"Properties": []any{
		int32(2),
		map[string]any{"TypePointer": "id-opt", "Value": map[string]any{"PrimitiveValue": " "}},
	}}
	item := extractObjectListItem(nil, itemObj, nested)
	if len(item.Props) != 0 {
		t.Errorf("expected whitespace-only primitive dropped, got %+v", item.Props)
	}
}
