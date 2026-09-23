// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#575: "A pluggable widget's textTemplate property takes only literal
// text, so it renders the same string on every row."
//
// A TreeNode's `headerCaption` is a textTemplate. Written as
//
//	treenode tnCustomer (headerType: 'text', headerCaption: 'Name', ...)
//
// it passes check, exec and mx check — and renders the literal word "Name" on
// every node. The companion an object-list item has took the widget's own
// property name + "Params" (#956), but that convention stopped at the item
// boundary: at the WIDGET level `headerCaptionParams` was an unknown property
// (MDL-WIDGET01) and the engine read parameters only from the single,
// widget-wide `contentparams:`.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func treeNodeDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID: "com.mendix.widget.custom.treenode.TreeNode",
		MDLName:  "TREENODE",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "headerCaption", Source: "TextTemplate", Operation: "texttemplate"},
			{PropertyKey: "headerType", Source: "Primitive", Operation: "primitive"},
		},
	}
}

// A timeline exposes SEVERAL textTemplate properties, which is what the single
// widget-wide `contentparams:` cannot address: each one needs its own binding.
func timelineDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID: "com.mendix.widget.custom.timeline.Timeline",
		MDLName:  "TIMELINE",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "title", Source: "TextTemplate", Operation: "texttemplate"},
			{PropertyKey: "description", Source: "TextTemplate", Operation: "texttemplate"},
		},
	}
}

func TestIssue575_TextTemplateTakesItsOwnParamsCompanion(t *testing.T) {
	def := treeNodeDef()
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{entityContext: "Sales.Customer"}, currentDef: def}
	w := &ast.WidgetV3{Name: "tn", Properties: map[string]any{
		"headerType":          "text",
		"headerCaption":       "{1}",
		"headerCaptionParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Name"}},
	}}

	ctx, err := e.resolveMapping(def.PropertyMappings[0], w)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.PrimitiveVal != "{1}" {
		t.Fatalf("PrimitiveVal = %q, want %q", ctx.PrimitiveVal, "{1}")
	}
	if len(ctx.ClientParams) != 1 {
		t.Fatalf("ClientParams = %d, want 1 — a {1} with no parameter renders nothing "+
			"and is CE0720 at build time", len(ctx.ClientParams))
	}
	if got := ctx.ClientParams[0].AttributeRef; got != "Sales.Customer.Name" {
		t.Errorf("parameter attribute = %q, want Sales.Customer.Name", got)
	}
}

// Each textTemplate property binds its OWN parameters. The widget-wide
// `contentparams:` cannot express this: one list, several templates.
func TestIssue575_EachTextTemplatePropertyBindsSeparately(t *testing.T) {
	def := timelineDef()
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{entityContext: "Sales.Order"}, currentDef: def}
	w := &ast.WidgetV3{Name: "tl", Properties: map[string]any{
		"title":             "{1}",
		"titleParams":       []ast.ParamAssignmentV3{{Index: 1, Value: "OrderNumber"}},
		"description":       "{1}",
		"descriptionParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Remarks"}},
	}}

	want := map[string]string{"title": "Sales.Order.OrderNumber", "description": "Sales.Order.Remarks"}
	for _, m := range def.PropertyMappings {
		ctx, err := e.resolveMapping(m, w)
		if err != nil {
			t.Fatal(err)
		}
		if len(ctx.ClientParams) != 1 {
			t.Fatalf("%s: ClientParams = %d, want 1", m.PropertyKey, len(ctx.ClientParams))
		}
		if got := ctx.ClientParams[0].AttributeRef; got != want[m.PropertyKey] {
			t.Errorf("%s: parameter attribute = %q, want %q", m.PropertyKey, got, want[m.PropertyKey])
		}
	}
}

// The widget-wide `contentparams:` keeps working for single-template widgets.
func TestIssue575_ContentParamsStillTheFallback(t *testing.T) {
	def := treeNodeDef()
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{entityContext: "Sales.Customer"}, currentDef: def}
	w := &ast.WidgetV3{Name: "tn", Properties: map[string]any{
		"headerCaption": "{1}",
		"ContentParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Name"}},
	}}
	ctx, err := e.resolveMapping(def.PropertyMappings[0], w)
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.ClientParams) != 1 {
		t.Fatalf("ClientParams = %d, want 1 — contentparams must remain the fallback", len(ctx.ClientParams))
	}
}

func TestIssue575_ParamsCompanionIsNotAnUnknownProperty(t *testing.T) {
	reg := &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{"TREENODE": treeNodeDef()}}
	w := &ast.WidgetV3{Name: "tn", Type: "treenode", Properties: map[string]any{
		"headerCaption":       "{1}",
		"headerCaptionParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Name"}},
	}}
	got := ruleIDs(validatePluggableWidgetProperties(w, reg, "page P"))
	if msg, ok := got["MDL-WIDGET01"]; ok {
		t.Errorf("headerCaptionParams reported as unknown: %s", msg)
	}
}

// Control: the allowance is per text-template property, not a blanket "*Params".
func TestIssue575_ParamsCompanionForANonTemplatePropertyIsStillUnknown(t *testing.T) {
	reg := &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{"TREENODE": treeNodeDef()}}
	w := &ast.WidgetV3{Name: "tn", Type: "treenode", Properties: map[string]any{
		"headerTypeParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Name"}},
	}}
	got := ruleIDs(validatePluggableWidgetProperties(w, reg, "page P"))
	if _, ok := got["MDL-WIDGET01"]; !ok {
		t.Errorf("a Params companion for a non-template property must stay MDL-WIDGET01; got %v", keysOf(got))
	}
}

// Parameters with no `{N}` placeholder to fill are dropped on write — the same
// silent class MDL-WIDGET21 already reports for the widget-wide contentparams.
func TestIssue575_OrphanedParamsCompanionIsReported(t *testing.T) {
	reg := &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{"TREENODE": treeNodeDef()}}
	w := &ast.WidgetV3{Name: "tn", Type: "treenode", Properties: map[string]any{
		"headerCaption":       "Name",
		"headerCaptionParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Name"}},
	}}
	found := false
	for _, v := range validateWidgetTree([]*ast.WidgetV3{w}, reg, "page P") {
		if v.RuleID == "MDL-WIDGET21" {
			found = true
		}
	}
	if !found {
		t.Fatal("headerCaptionParams with no {1} in headerCaption must be reported (MDL-WIDGET21)")
	}
}

// The built-in pluggable Image is the shape reachable without a Marketplace
// widget: two text-template properties (`imageUrl`, `alternativeText`), which is
// exactly what one widget-wide `contentparams:` cannot address. Its mappings are
// named by SOURCE (`ImageUrl`), so they resolve through a different branch of
// resolveMapping than the TreeNode's — and that branch carried no parameters at
// all, contentparams included.
func TestIssue575_ImageBindsEachTemplateSeparately(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}
	def := reg.byMDLName["IMAGE"]
	if def == nil {
		t.Fatal("no built-in IMAGE definition")
	}

	for _, spelling := range []struct {
		name          string
		url, urlParam string
		alt, altParam string
	}{
		{"source names", "ImageUrl", "ImageUrlParams", "AlternativeText", "AlternativeTextParams"},
		{"schema keys", "imageUrl", "imageUrlParams", "alternativeText", "alternativeTextParams"},
	} {
		t.Run(spelling.name, func(t *testing.T) {
			e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{entityContext: "Sales.Product"}, currentDef: def}
			w := &ast.WidgetV3{Name: "img", Type: "image", Properties: map[string]any{
				spelling.url:      "{1}",
				spelling.urlParam: []ast.ParamAssignmentV3{{Index: 1, Value: "PhotoUrl"}},
				spelling.alt:      "{1}",
				spelling.altParam: []ast.ParamAssignmentV3{{Index: 1, Value: "Name"}},
			}}

			want := map[string]string{"imageUrl": "Sales.Product.PhotoUrl", "alternativeText": "Sales.Product.Name"}
			for _, m := range def.PropertyMappings {
				if m.Operation != "texttemplate" {
					continue
				}
				ctx, err := e.resolveMapping(m, w)
				if err != nil {
					t.Fatal(err)
				}
				if len(ctx.ClientParams) != 1 {
					t.Fatalf("%s: ClientParams = %d, want 1", m.PropertyKey, len(ctx.ClientParams))
				}
				if got := ctx.ClientParams[0].AttributeRef; got != want[m.PropertyKey] {
					t.Errorf("%s: parameter attribute = %q, want %q", m.PropertyKey, got, want[m.PropertyKey])
				}
			}

			if got := ruleIDs(validatePluggableWidgetProperties(w, reg, "page P")); got["MDL-WIDGET01"] != "" {
				t.Errorf("params companion reported as unknown: %s", got["MDL-WIDGET01"])
			}
		})
	}
}

// DESCRIBE must emit the companion, or the round trip unbinds what it copied: a
// bound `ImageUrl: '{1}'` described back without its parameter re-executes into
// CE0720 ("Place holder index 1 is greater than 0, the number of parameter(s)").
func TestIssue575_DescribeEmitsTheParamsCompanion(t *testing.T) {
	w := rawWidget{
		Name:                  "cardImage",
		ImageType:             "imageUrl",
		ImageUrl:              "{1}",
		ImageUrlParams:        []string{"Bug575.Product.PictureUrl"},
		AlternativeText:       "{1}",
		AlternativeTextParams: []string{"Bug575.Product.Name"},
	}
	got := strings.Join(describeImageWidgetProps(w), ", ")
	for _, want := range []string{
		"ImageUrlParams: [{1} = Bug575.Product.PictureUrl]",
		"AlternativeTextParams: [{1} = Bug575.Product.Name]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output missing %q; got: %s", want, got)
		}
	}
}

// CONTROL: an unbound template emits no companion. An empty `[]` would not parse
// back, and a companion on a literal caption is what MDL-WIDGET21 warns about.
func TestIssue575_DescribeOmitsAnAbsentParamsCompanion(t *testing.T) {
	w := rawWidget{Name: "cardImage", ImageType: "imageUrl", ImageUrl: "https://example.com/x.png"}
	for _, p := range describeImageWidgetProps(w) {
		if strings.HasSuffix(strings.SplitN(p, ":", 2)[0], "Params") {
			t.Errorf("emitted %q for a template with no parameters", p)
		}
	}
}

// treeNodeStoredWidget is one stored CustomWidget in the shape a TreeNode takes:
// a text-template property (`headerCaption`) bound to `{1}` = attr, beside a
// primitive (`headerType`). TreeNode and Timeline have no dedicated DESCRIBE
// extractor, so they go through extractExplicitProperties — which read
// AttributeRef and PrimitiveValue only.
func treeNodeStoredWidget(attr string) map[string]any {
	template := map[string]any{
		"$Type": "Forms$ClientTemplate",
		"Template": map[string]any{
			"$Type": "Texts$Text",
			"Items": []any{
				map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "{1}"},
			},
		},
		"Parameters": []any{
			map[string]any{
				"$Type":        "Forms$ClientTemplateParameter",
				"AttributeRef": map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": attr},
				"Expression":   "",
			},
		},
	}
	propType := func(id, key, valueType string) map[string]any {
		return map[string]any{
			"$ID": id, "$Type": "CustomWidgets$WidgetPropertyType",
			"PropertyKey": key, "ValueType": valueType,
		}
	}
	prop := func(ptr string, value map[string]any) map[string]any {
		return map[string]any{"$Type": "CustomWidgets$WidgetProperty", "TypePointer": ptr, "Value": value}
	}
	return map[string]any{
		"Type": map[string]any{"ObjectType": map[string]any{"PropertyTypes": []any{
			propType("pt-1", "headerCaption", "TextTemplate"),
			propType("pt-2", "headerType", "Enumeration"),
		}}},
		"Object": map[string]any{"Properties": []any{
			prop("pt-1", map[string]any{"$Type": "CustomWidgets$WidgetValue", "TextTemplate": template}),
			prop("pt-2", map[string]any{"$Type": "CustomWidgets$WidgetValue", "PrimitiveValue": "text"}),
		}},
	}
}

// The generic extractor must read text templates. Without this every
// text-template property of every widget with no dedicated extractor — a
// TreeNode's headerCaption, a Timeline's title/description/timeIndication — was
// absent from DESCRIBE whether it was bound OR literal, so describe → exec
// dropped the caption entirely and the copy rendered blank.
func TestIssue575_GenericDescribeReadsTextTemplates(t *testing.T) {
	ctx, _ := newMockCtx(t)
	props := extractExplicitProperties(ctx, treeNodeStoredWidget("Sales.Customer.Name"))

	var caption *rawExplicitProp
	for i := range props {
		if props[i].Key == "headerCaption" {
			caption = &props[i]
		}
	}
	if caption == nil {
		t.Fatalf("headerCaption absent from the generic describe; got %+v", props)
	}
	if caption.Value != "{1}" {
		t.Errorf("headerCaption = %q, want {1}", caption.Value)
	}
	if len(caption.Params) != 1 || caption.Params[0] != "Name" {
		t.Errorf("headerCaption params = %v, want [Name] — a `{1}` re-executed with no "+
			"parameter is CE0720", caption.Params)
	}

	// The primitive beside it is untouched: the new branch must not swallow
	// properties the extractor already handled.
	var seenHeaderType bool
	for _, p := range props {
		if p.Key == "headerType" && p.Value == "text" {
			seenHeaderType = true
		}
	}
	if !seenHeaderType {
		t.Errorf("headerType lost from the generic describe; got %+v", props)
	}
}

// CONTROL: an unset or widget-hidden template stores a null / empty
// ClientTemplate and must emit nothing — a bare `headerCaption: ”` would
// re-execute into an empty caption where the widget's own default belongs.
func TestIssue575_GenericDescribeOmitsAnEmptyTextTemplate(t *testing.T) {
	ctx, _ := newMockCtx(t)
	w := treeNodeStoredWidget("Sales.Customer.Name")
	obj := w["Object"].(map[string]any)
	obj["Properties"].([]any)[0].(map[string]any)["Value"] = map[string]any{
		"$Type": "CustomWidgets$WidgetValue", "TextTemplate": nil,
	}
	for _, p := range extractExplicitProperties(ctx, w) {
		if p.Key == "headerCaption" {
			t.Errorf("emitted %q for an unset template", p.Value)
		}
	}
}
