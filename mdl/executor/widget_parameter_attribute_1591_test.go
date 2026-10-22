// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1591, the half its first fix left open. A Custom chart
// inside a data view over $Entry had its eventDataAttribute bound to an
// attribute of the OTHER page parameter, $Ctx. Studio Pro stores that as the
// AttributeRef plus a SourceVariable naming the parameter, and builds clean.
// MDL had no spelling for it:
//
//   - `eventDataAttribute: $Ctx.EventData` was MDL-WIDGET42 "is a attribute
//     property and cannot hold the expression $Ctx.EventData";
//   - `eventDataAttribute: MyFirstModule.ChartContext.EventData` passed check
//     and exec and wrote the AttributeRef alone — mx check CE7247 "Attribute
//     'MyFirstModule.ChartContext.EventData' is not an attribute of entity
//     'MyFirstModule.ChartEntry'" (measured on 11.12.5);
//   - and DESCRIBE printed that stored pair as the qualified name, which
//     re-executes into the CE7247 shape.
//
// The property here is the combo box's staticAttribute: linked to no
// datasource and mapped by no .def.json entry, so — like eventDataAttribute —
// it is written by the explicit-key pass.

// paramDataViewEngine is scopeEngine inside a data view over Sales.Invoice on
// a page that also declares $Ctx: Sales.Customer.
func paramDataViewEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	e := scopeEngine(t)
	e.pageBuilder.paramScope = map[string]model.ID{"Ctx": "e-customer"}
	e.pageBuilder.paramEntityNames["Ctx"] = "Sales.Customer"
	return e
}

func TestBuild_ExplicitAttributeKeyReadsAPageParameter(t *testing.T) {
	e := paramDataViewEngine(t)
	w := twoSourceWidget(map[string]any{"staticAttribute": &ast.WidgetExpressionV3{Text: "$Ctx.Name"}})
	widget, err := e.Build(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	val := widgetValueFor(widget.RawObject, "Sales.Customer.Name")
	if val == nil {
		t.Fatal("no WidgetValue binds Sales.Customer.Name — the binding was dropped")
	}
	sv, ok := dGet(val, "SourceVariable").(bson.D)
	if !ok {
		t.Fatalf("SourceVariable = %#v, want a Forms$PageVariable naming $Ctx", dGet(val, "SourceVariable"))
	}
	for k, want := range map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "Ctx",
		"SnippetParameter": "", "LocalVariable": "", "Widget": ""} {
		if got := dGet(sv, k); got != want {
			t.Errorf("SourceVariable.%s = %#v, want %#v", k, got, want)
		}
	}

	// Control: a `$name` that is no parameter is refused, as on a mapped key.
	w = twoSourceWidget(map[string]any{"staticAttribute": &ast.WidgetExpressionV3{Text: "$Nope.Name"}})
	if _, err := paramDataViewEngine(t).Build(multiSourceDef(), w); err == nil {
		t.Error("staticAttribute: $Nope.Name built; want a refusal")
	}
}

// The qualified name of an attribute the bound entity does not have is the
// CE7247 shape: refused by exec and check alike, pointing at `$Param.Attr`.
func TestCheck_QualifiedAttributeOfAnotherEntityIsRefused(t *testing.T) {
	ctx := scopeCheckCtx(t)
	s := scopeCheckPage(t, `staticAttribute: Sales.Customer.Name`)
	errs := validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, allPageWidgets(s), newScriptContext())
	if len(errs) != 1 {
		t.Fatalf("want one binding error, got %q", errs)
	}
	for _, want := range []string{"viewer", "staticAttribute", "Sales.Customer.Name", "Sales.Invoice", "CE7247", "$Param.Name"} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("error should mention %q, got: %s", want, errs[0])
		}
	}

	_, err := scopeEngine(t).Build(multiSourceDef(), twoSourceWidget(map[string]any{"staticAttribute": "Sales.Customer.Name"}))
	if err == nil || !strings.Contains(err.Error(), errs[0]) {
		t.Errorf("check and exec disagree:\n check: %s\n exec:  %v", errs[0], err)
	}

	// Controls: the bound entity's own attribute, qualified, passes both.
	s = scopeCheckPage(t, `staticAttribute: Sales.Invoice.Reference`)
	if errs := validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, allPageWidgets(s), newScriptContext()); len(errs) > 0 {
		t.Errorf("Sales.Invoice.Reference: unexpected refusal %q", errs)
	}
	if _, err := scopeEngine(t).Build(multiSourceDef(), twoSourceWidget(map[string]any{"staticAttribute": "Sales.Invoice.Reference"})); err != nil {
		t.Errorf("Sales.Invoice.Reference: unexpected exec refusal %v", err)
	}
}

// An inherited attribute stays qualified with the entity that declares it, and
// that entity is a generalization of the bound one — not "another entity".
func TestMisbound_QualifiedInheritedAttributePasses(t *testing.T) {
	ix := attributeIndex{
		owners:  map[string]map[string]bool{"Sales.Invoice": {"reference": true}, "Sales.Document": {"title": true}},
		parents: map[string]string{"Sales.Invoice": "Sales.Document"},
	}
	scope := bindingScope{Entity: "Sales.Invoice", Kind: scopeEnclosing}
	if err := misboundAttributeError("w", "Sales.Document.Title", scope, "Sales.Invoice", nil, ix); err != nil {
		t.Errorf("inherited Sales.Document.Title refused: %v", err)
	}
	if err := misboundAttributeError("w", "Sales.Order.Number", scope, "Sales.Invoice", nil, ix); err != nil {
		t.Errorf("an entity the index does not hold must not be judged: %v", err)
	}
}

// Check takes `$Param.Attr` on an attribute property, and still refuses a real
// expression there.
func TestCheck_ParameterAttributeOnAttributeKeyIsNotAnExpression(t *testing.T) {
	widget42 := func(value string) []string {
		t.Helper()
		src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($Ctx: M.A)) {\n" +
			"  combobox cmb (Attribute: Name, staticAttribute: " + value + ")\n};"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parsing %q: %v", src, errs)
		}
		var out []string
		for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], LoadWidgetRegistry("")) {
			if v.RuleID == "MDL-WIDGET42" {
				out = append(out, v.Message)
			}
		}
		return out
	}
	if got := widget42("$Ctx.Name"); len(got) > 0 {
		t.Errorf("staticAttribute: $Ctx.Name refused: %q", got)
	}
	if got := widget42("$Ctx.Name + 'x'"); len(got) != 1 {
		t.Errorf("staticAttribute: $Ctx.Name + 'x': want MDL-WIDGET42, got %q", got)
	}
}

func TestDescribePluggableAttributeOfAPageParameter(t *testing.T) {
	ctx, _ := newMockCtx(t)
	dv := chartInParameterDataView("MyFirstModule.ChartContext.EventData")
	chart := dv["Widgets"].([]any)[0].(map[string]any)
	props := chart["Object"].(map[string]any)["Properties"].([]any)
	props[1].(map[string]any)["Value"].(map[string]any)["SourceVariable"] = map[string]any{
		"$Type": "Forms$PageVariable", "PageParameter": "Ctx", "SnippetParameter": "", "LocalVariable": "", "Widget": "",
	}
	dvs := parseRawWidget(ctx, dv)
	got := map[string]string{}
	for _, p := range dvs[0].Children[0].ExplicitProperties {
		got[p.Key] = p.Value
	}
	if got["eventDataAttribute"] != "$Ctx.EventData" {
		t.Errorf("eventDataAttribute = %q, want $Ctx.EventData (qualified, re-exec drops the parameter: CE7247)",
			got["eventDataAttribute"])
	}
	if got["dataAttribute"] != "JsonData" {
		t.Errorf("dataAttribute = %q, want the bare JsonData", got["dataAttribute"])
	}
}

// With no project the Custom chart has neither a definition nor a schema, so
// eventDataAttribute's kind is unknown. `$Param.Attr` is no Mendix expression
// (that is `$Param/Attr`), so it is not refused as one; a real expression on
// the same key still is.
func TestCheck_ParameterAttributeOnAnUnknownKeyIsNotAnExpression(t *testing.T) {
	widget42 := func(value string) []string {
		t.Helper()
		src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($Ctx: M.A, $Entry: M.B)) {\n" +
			"  dataview dv (DataSource: $Entry) {\n    customchart chart1 (eventDataAttribute: " + value + ")\n  }\n};"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parsing %q: %v", src, errs)
		}
		var out []string
		for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], LoadWidgetRegistry("")) {
			if v.RuleID == "MDL-WIDGET42" {
				out = append(out, v.Message)
			}
		}
		return out
	}
	if got := widget42("$Ctx.EventData"); len(got) > 0 {
		t.Errorf("eventDataAttribute: $Ctx.EventData refused: %q", got)
	}
	if got := widget42("$Ctx.EventData + 'x'"); len(got) != 1 {
		t.Errorf("eventDataAttribute: $Ctx.EventData + 'x': want MDL-WIDGET42, got %q", got)
	}
}
