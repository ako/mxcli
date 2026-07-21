// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// namedPropValue routes a widget-level property to the right MDL keyword via its
// PropertyKey or a registered alias (item 1b — PieChart/HeatMap bind several
// attribute/texttemplate properties that the single generic `Attribute:` keyword
// can't disambiguate).
func TestNamedPropValue(t *testing.T) {
	m := PropertyMapping{PropertyKey: "seriesValueAttribute", MdlAliases: []string{"ValueAttribute"}}

	// via alias
	if got := namedPropValue(m, &ast.WidgetV3{Properties: map[string]any{"ValueAttribute": "Total"}}); got != "Total" {
		t.Errorf("alias match = %q, want Total", got)
	}
	// via the schema key directly (case-insensitive)
	if got := namedPropValue(m, &ast.WidgetV3{Properties: map[string]any{"seriesvalueattribute": "Amount"}}); got != "Amount" {
		t.Errorf("key match = %q, want Amount", got)
	}
	// unset
	if got := namedPropValue(m, &ast.WidgetV3{Properties: map[string]any{"Other": "x"}}); got != "" {
		t.Errorf("unset = %q, want empty", got)
	}
}

// A named widget-level attribute resolves against the widget's datasource entity
// (entityContext, set by the DataSource mapping that runs first).
func TestResolveMapping_NamedAttribute(t *testing.T) {
	pb := &pageBuilder{
		entityContext:    "ChartExamples.SalesByRegion",
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	engine := &PluggableWidgetEngine{pageBuilder: pb}
	mapping := PropertyMapping{
		PropertyKey: "seriesValueAttribute", Source: "Attribute", Operation: "attribute",
		MdlAliases: []string{"ValueAttribute"},
	}
	w := &ast.WidgetV3{Properties: map[string]any{"ValueAttribute": "Total"}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("resolveMapping: %v", err)
	}
	if ctx.AttributePath != "ChartExamples.SalesByRegion.Total" {
		t.Errorf("AttributePath = %q, want ChartExamples.SalesByRegion.Total", ctx.AttributePath)
	}
}

// A widget-level texttemplate (PieChart seriesName) reads its named MDL value.
func TestResolveMapping_NamedTextTemplate(t *testing.T) {
	engine := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	mapping := PropertyMapping{
		PropertyKey: "seriesName", Source: "TextTemplate", Operation: "texttemplate",
		MdlAliases: []string{"SeriesName"},
	}
	w := &ast.WidgetV3{Properties: map[string]any{"SeriesName": "Sales by Region"}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("resolveMapping: %v", err)
	}
	if ctx.PrimitiveVal != "Sales by Region" {
		t.Errorf("PrimitiveVal = %q, want 'Sales by Region'", ctx.PrimitiveVal)
	}
}

// GenerateDefJSON emits the widget-level aliases; the aliased seriesName carries
// its friendly MDL keyword, and an unaliased texttemplate still gets a mapping
// (authorable by its own property name) — it just has no alias.
func TestGenerateDefJSON_PieChartNamedProperties(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.piechart.PieChart",
		Name: "Pie chart",
		Properties: []mpk.PropertyDef{
			{Key: "seriesDataSource", Type: "datasource"},
			{Key: "seriesValueAttribute", Type: "attribute"},
			{Key: "seriesName", Type: "textTemplate"},
			{Key: "otherTemplate", Type: "textTemplate"}, // no alias → still mapped, just aliasless
		},
	}
	def := GenerateDefJSON(mpkDef, "PIECHART")
	byKey := map[string]PropertyMapping{}
	for _, pm := range def.PropertyMappings {
		byKey[pm.PropertyKey] = pm
	}
	if got := byKey["seriesValueAttribute"].MdlAliases; len(got) != 1 || got[0] != "ValueAttribute" {
		t.Errorf("seriesValueAttribute aliases = %v, want [ValueAttribute]", got)
	}
	sn, ok := byKey["seriesName"]
	if !ok || sn.Operation != "texttemplate" || len(sn.MdlAliases) != 1 || sn.MdlAliases[0] != "SeriesName" {
		t.Errorf("seriesName mapping = %+v, want texttemplate with [SeriesName]", sn)
	}
	// An unaliased texttemplate is now still emitted (authorable by its own name),
	// carrying no MDL alias.
	ot, ok := byKey["otherTemplate"]
	if !ok || ot.Operation != "texttemplate" {
		t.Errorf("otherTemplate mapping = %+v, want a texttemplate mapping", ot)
	}
	if len(ot.MdlAliases) != 0 {
		t.Errorf("otherTemplate aliases = %v, want none", ot.MdlAliases)
	}
}
