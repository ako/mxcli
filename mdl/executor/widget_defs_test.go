// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

func TestDeriveMDLName(t *testing.T) {
	tests := []struct {
		widgetID string
		expected string
	}{
		{"com.mendix.widget.web.combobox.Combobox", "COMBOBOX"},
		{"com.mendix.widget.web.gallery.Gallery", "GALLERY"},
		{"com.company.widget.MyCustomWidget", "MYCUSTOMWIDGET"},
		{"SimpleWidget", "SIMPLEWIDGET"},
	}

	for _, tc := range tests {
		t.Run(tc.widgetID, func(t *testing.T) {
			result := DeriveMDLName(tc.widgetID)
			if result != tc.expected {
				t.Errorf("DeriveMDLName(%q) = %q, want %q", tc.widgetID, result, tc.expected)
			}
		})
	}
}

func TestGenerateDefJSON(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.widget.TestWidget",
		Name: "Test Widget",
		Properties: []mpk.PropertyDef{
			{Key: "datasource", Type: "datasource"},
			{Key: "content", Type: "widgets"},
			{Key: "filterBar", Type: "widgets"},
			{Key: "myAttribute", Type: "attribute"},
			{Key: "showHeader", Type: "boolean", DefaultValue: "true"},
			{Key: "itemSelection", Type: "selection", DefaultValue: "Single"},
			{Key: "myAssociation", Type: "association"},
			{Key: "pageSize", Type: "integer", DefaultValue: "10"},
		},
	}

	def := GenerateDefJSON(mpkDef, "TESTWIDGET")

	// Verify basic fields
	if def.WidgetID != "com.example.widget.TestWidget" {
		t.Errorf("WidgetID = %q, want %q", def.WidgetID, "com.example.widget.TestWidget")
	}
	if def.MDLName != "TESTWIDGET" {
		t.Errorf("MDLName = %q, want %q", def.MDLName, "TESTWIDGET")
	}
	if def.TemplateFile != "testwidget.json" {
		t.Errorf("TemplateFile = %q, want %q", def.TemplateFile, "testwidget.json")
	}
	if def.DefaultEditable != "Always" {
		t.Errorf("DefaultEditable = %q, want %q", def.DefaultEditable, "Always")
	}

	// Verify property mappings count (datasource, attribute, boolean, selection, association, integer = 6)
	if len(def.PropertyMappings) != 6 {
		t.Fatalf("PropertyMappings count = %d, want 6", len(def.PropertyMappings))
	}

	// Verify child slots (content → TEMPLATE, filterBar → FILTERBAR)
	if len(def.ChildSlots) != 2 {
		t.Fatalf("ChildSlots count = %d, want 2", len(def.ChildSlots))
	}

	// content → TEMPLATE (special case)
	if def.ChildSlots[0].MDLContainer != "TEMPLATE" {
		t.Errorf("ChildSlots[0].MDLContainer = %q, want %q", def.ChildSlots[0].MDLContainer, "TEMPLATE")
	}
	// filterBar → FILTERBAR
	if def.ChildSlots[1].MDLContainer != "FILTERBAR" {
		t.Errorf("ChildSlots[1].MDLContainer = %q, want %q", def.ChildSlots[1].MDLContainer, "FILTERBAR")
	}

	// Verify datasource mapping
	dsMappings := findMapping(def.PropertyMappings, "datasource")
	if dsMappings == nil {
		t.Fatal("datasource mapping not found")
	}
	if dsMappings.Operation != "datasource" {
		t.Errorf("datasource operation = %q, want %q", dsMappings.Operation, "datasource")
	}

	// Verify attribute mapping
	attrMapping := findMapping(def.PropertyMappings, "myAttribute")
	if attrMapping == nil {
		t.Fatal("myAttribute mapping not found")
	}
	if attrMapping.Operation != "attribute" || attrMapping.Source != "Attribute" {
		t.Errorf("myAttribute: operation=%q source=%q, want operation=attribute source=Attribute",
			attrMapping.Operation, attrMapping.Source)
	}

	// Verify boolean with default value
	boolMapping := findMapping(def.PropertyMappings, "showHeader")
	if boolMapping == nil {
		t.Fatal("showHeader mapping not found")
	}
	if boolMapping.Value != "true" {
		t.Errorf("showHeader value = %q, want %q", boolMapping.Value, "true")
	}

	// Verify selection with default
	selMapping := findMapping(def.PropertyMappings, "itemSelection")
	if selMapping == nil {
		t.Fatal("itemSelection mapping not found")
	}
	if selMapping.Operation != "selection" || selMapping.Default != "Single" {
		t.Errorf("itemSelection: operation=%q default=%q, want operation=selection default=Single",
			selMapping.Operation, selMapping.Default)
	}
}

func TestGenerateDefJSON_SkipsComplexTypes(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.Complex",
		Name: "Complex",
		Properties: []mpk.PropertyDef{
			{Key: "myAction", Type: "action"},
			{Key: "myExpr", Type: "expression"},
			{Key: "myTemplate", Type: "textTemplate"},
			{Key: "myIcon", Type: "icon"},
			{Key: "myObj", Type: "object"},
		},
	}

	def := GenerateDefJSON(mpkDef, "COMPLEX")

	// Complex types should be skipped
	if len(def.PropertyMappings) != 0 {
		t.Errorf("PropertyMappings count = %d, want 0 (complex types should be skipped)", len(def.PropertyMappings))
	}
	if len(def.ChildSlots) != 0 {
		t.Errorf("ChildSlots count = %d, want 0", len(def.ChildSlots))
	}
}

func TestGenerateDefJSON_AssociationAfterDataSource(t *testing.T) {
	// Association mappings require entityContext from a prior DataSource mapping.
	// GenerateDefJSON must order datasource before association regardless of MPK order.
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.AssocFirst",
		Name: "AssocFirst",
		Properties: []mpk.PropertyDef{
			{Key: "myAssoc", Type: "association"}, // association BEFORE datasource in MPK
			{Key: "myLabel", Type: "string"},
			{Key: "myDS", Type: "datasource"},
		},
	}

	def := GenerateDefJSON(mpkDef, "ASSOCFIRST")

	// Should have 3 mappings: datasource, string primitive, association
	if len(def.PropertyMappings) != 3 {
		t.Fatalf("PropertyMappings count = %d, want 3", len(def.PropertyMappings))
	}

	// datasource must appear before association in the mappings slice
	dsIdx, assocIdx := -1, -1
	for i, m := range def.PropertyMappings {
		if m.Source == "DataSource" {
			dsIdx = i
		}
		if m.Source == "Association" {
			assocIdx = i
		}
	}
	if dsIdx < 0 {
		t.Fatal("DataSource mapping not found")
	}
	if assocIdx < 0 {
		t.Fatal("Association mapping not found")
	}
	if dsIdx > assocIdx {
		t.Errorf("DataSource at index %d must come before Association at index %d", dsIdx, assocIdx)
	}

	// Verify the generated definition can be loaded by the registry without validation errors.
	// The registry's validateMappings enforces Association-after-DataSource ordering.
}

func findMapping(mappings []PropertyMapping, key string) *PropertyMapping {
	for i := range mappings {
		if mappings[i].PropertyKey == key {
			return &mappings[i]
		}
	}
	return nil
}

// TestDeriveObjectListKeyword verifies plural→singular keyword derivation
// for object-list properties, including the override map for irregular cases.
func TestDeriveObjectListKeyword(t *testing.T) {
	tests := []struct {
		propertyKey string
		expected    string
	}{
		// Regular plurals (strip trailing 's', uppercase)
		{"groups", "GROUP"},
		{"columns", "COLUMN"},
		{"markers", "MARKER"},
		// Override map (irregular cases)
		{"basicItems", "ITEM"},
		{"customItems", "CUSTOMITEM"},
		{"dynamicMarkers", "DYNAMICMARKER"},
		{"attributesList", "ATTR"},
		{"filterOptions", "OPTION"},
		{"series", "SERIES"}, // Latin singular == plural
	}

	for _, tc := range tests {
		t.Run(tc.propertyKey, func(t *testing.T) {
			got := deriveObjectListKeyword(tc.propertyKey)
			if got != tc.expected {
				t.Errorf("deriveObjectListKeyword(%q) = %q, want %q",
					tc.propertyKey, got, tc.expected)
			}
		})
	}
}

// TestGenerateDefJSON_ObjectList covers extraction of Type:"object"+IsList:true
// properties (e.g. Accordion groups, DataGrid columns, PopupMenu basicItems).
// Each list item's sub-property tree should be split between ItemProperties
// (scalar/datasource/attribute/etc.) and ItemSlots (widgets-typed).
func TestGenerateDefJSON_ObjectList(t *testing.T) {
	// Synthesize an Accordion-style "groups" property with mixed sub-property kinds.
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.widget.Accordion",
		Name: "Accordion",
		Properties: []mpk.PropertyDef{
			{Key: "advancedMode", Type: "boolean", DefaultValue: "false"},
			{
				Key:    "groups",
				Type:   "object",
				IsList: true,
				Children: []mpk.PropertyDef{
					{Key: "headerRenderMode", Type: "enumeration", DefaultValue: "text"},
					{Key: "headerText", Type: "textTemplate"},
					{Key: "visible", Type: "expression"},
					{Key: "collapsed", Type: "attribute"},
					{Key: "onToggleCollapsed", Type: "action"},
					{Key: "headerContent", Type: "widgets"},
					{Key: "content", Type: "widgets"},
				},
			},
		},
	}

	def := GenerateDefJSON(mpkDef, "ACCORDION")

	// Top-level primitive should still land in PropertyMappings.
	if len(def.PropertyMappings) != 1 {
		t.Fatalf("PropertyMappings count = %d, want 1", len(def.PropertyMappings))
	}

	// Object-list goes to ObjectLists, not to ChildSlots or PropertyMappings.
	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists count = %d, want 1", len(def.ObjectLists))
	}
	ol := def.ObjectLists[0]
	if ol.PropertyKey != "groups" {
		t.Errorf("ObjectLists[0].PropertyKey = %q, want %q", ol.PropertyKey, "groups")
	}
	if ol.MDLContainer != "GROUP" {
		t.Errorf("ObjectLists[0].MDLContainer = %q, want %q", ol.MDLContainer, "GROUP")
	}

	// 5 non-widgets items should be ItemProperties; 2 widgets items should be ItemSlots.
	if len(ol.ItemProperties) != 5 {
		t.Errorf("ItemProperties count = %d, want 5", len(ol.ItemProperties))
	}
	if len(ol.ItemSlots) != 2 {
		t.Errorf("ItemSlots count = %d, want 2", len(ol.ItemSlots))
	}

	// Spot-check operation kinds for sub-properties.
	wantOps := map[string]string{
		"headerRenderMode":  "primitive",
		"headerText":        "texttemplate",
		"visible":           "expression",
		"collapsed":         "attribute",
		"onToggleCollapsed": "action",
	}
	for _, ip := range ol.ItemProperties {
		want, ok := wantOps[ip.PropertyKey]
		if !ok {
			t.Errorf("unexpected ItemProperty key %q", ip.PropertyKey)
			continue
		}
		if ip.Operation != want {
			t.Errorf("ItemProperty %q: Operation = %q, want %q",
				ip.PropertyKey, ip.Operation, want)
		}
	}

	// ItemSlots should map widgets-typed sub-properties to their MDLContainer.
	wantSlots := map[string]string{
		"headerContent": "HEADERCONTENT",
		"content":       "CONTENT",
	}
	for _, slot := range ol.ItemSlots {
		want, ok := wantSlots[slot.PropertyKey]
		if !ok {
			t.Errorf("unexpected ItemSlot key %q", slot.PropertyKey)
			continue
		}
		if slot.MDLContainer != want {
			t.Errorf("ItemSlot %q: MDLContainer = %q, want %q",
				slot.PropertyKey, slot.MDLContainer, want)
		}
		if slot.Operation != "widgets" {
			t.Errorf("ItemSlot %q: Operation = %q, want %q",
				slot.PropertyKey, slot.Operation, "widgets")
		}
	}

	// IsList=false on an object property should still skip (not extracted).
	mpkDef2 := &mpk.WidgetDefinition{
		ID:   "com.example.widget.NotAList",
		Name: "NotAList",
		Properties: []mpk.PropertyDef{
			{Key: "myObj", Type: "object", IsList: false},
		},
	}
	def2 := GenerateDefJSON(mpkDef2, "NOTALIST")
	if len(def2.ObjectLists) != 0 {
		t.Errorf("ObjectLists for non-list object property = %d, want 0",
			len(def2.ObjectLists))
	}
}

// TestGenerateDefJSON_ObjectListPrimitiveDefaults verifies that primitive
// item properties carry their MPK default values into the ItemPropertyMapping.
func TestGenerateDefJSON_ObjectListPrimitiveDefaults(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.example.widget.Sized",
		Name: "Sized",
		Properties: []mpk.PropertyDef{
			{
				Key:    "items",
				Type:   "object",
				IsList: true,
				Children: []mpk.PropertyDef{
					{Key: "size", Type: "integer", DefaultValue: "10"},
					{Key: "label", Type: "string", DefaultValue: ""},
					{Key: "kind", Type: "enumeration", DefaultValue: "default"},
				},
			},
		},
	}

	def := GenerateDefJSON(mpkDef, "SIZED")
	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists count = %d, want 1", len(def.ObjectLists))
	}
	props := def.ObjectLists[0].ItemProperties
	if len(props) != 3 {
		t.Fatalf("ItemProperties count = %d, want 3", len(props))
	}

	wantValues := map[string]string{
		"size":  "10",
		"label": "", // empty default → no Value set
		"kind":  "default",
	}
	for _, ip := range props {
		want := wantValues[ip.PropertyKey]
		if ip.Value != want {
			t.Errorf("ItemProperty %q Value = %q, want %q",
				ip.PropertyKey, ip.Value, want)
		}
		if ip.Operation != "primitive" {
			t.Errorf("ItemProperty %q Operation = %q, want primitive",
				ip.PropertyKey, ip.Operation)
		}
	}
}

// TestMdlContainerForWidgetSlot covers the editorial override table that
// maps (widgetID, propertyKey) pairs to the MDL keyword users type to fill
// a widgets-typed property.
//
// The override exists because Studio Pro users (and the historical keyword
// path) think of `controlbar { ... }` for DataGrid and `filter { ... }`
// for Gallery, not `filtersplaceholder { ... }`. Without the override the
// auto-derived keyword would be the uppercase property key, which doesn't
// match the MDL grammar tokens.
func TestMdlContainerForWidgetSlot(t *testing.T) {
	tests := []struct {
		name        string
		widgetID    string
		propertyKey string
		want        string
	}{
		{
			name:        "DataGrid filtersPlaceholder → CONTROLBAR",
			widgetID:    "com.mendix.widget.web.datagrid.Datagrid",
			propertyKey: "filtersPlaceholder",
			want:        "CONTROLBAR",
		},
		{
			name:        "Gallery filtersPlaceholder → FILTER",
			widgetID:    "com.mendix.widget.web.gallery.Gallery",
			propertyKey: "filtersPlaceholder",
			want:        "FILTER",
		},
		{
			name:        "content property → TEMPLATE (global convention)",
			widgetID:    "com.example.AnyWidget",
			propertyKey: "content",
			want:        "TEMPLATE",
		},
		{
			name:        "unmapped property → uppercase key",
			widgetID:    "com.mendix.widget.web.datagrid.Datagrid",
			propertyKey: "emptyPlaceholder",
			want:        "EMPTYPLACEHOLDER",
		},
		{
			name:        "unmapped widget, unmapped property → uppercase key",
			widgetID:    "com.example.UnknownWidget",
			propertyKey: "myslot",
			want:        "MYSLOT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mdlContainerForWidgetSlot(tc.widgetID, tc.propertyKey)
			if got != tc.want {
				t.Errorf("mdlContainerForWidgetSlot(%q, %q) = %q, want %q",
					tc.widgetID, tc.propertyKey, got, tc.want)
			}
		})
	}
}

// TestObjectListItemAliases covers MDL property name aliases on object-list
// items. A DataGrid column's `Caption:` and `Content:` in MDL fill the
// schema's `header` and `dynamicText` properties respectively. Without the
// aliases, the engine would look up `header` / `dynamicText` in the AST
// property bag and find nothing — the caption text would be silently dropped.
func TestObjectListItemAliases(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.datagrid.Datagrid",
		Name: "Data grid 2",
		Properties: []mpk.PropertyDef{
			{
				Key:    "columns",
				Type:   "object",
				IsList: true,
				Children: []mpk.PropertyDef{
					{Key: "header", Type: "textTemplate"},
					{Key: "dynamicText", Type: "textTemplate"},
					{Key: "tooltip", Type: "textTemplate"},
					{Key: "attribute", Type: "attribute"},
				},
			},
		},
	}
	def := GenerateDefJSON(mpkDef, "DATAGRID")

	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists count = %d, want 1", len(def.ObjectLists))
	}
	cols := def.ObjectLists[0]
	if cols.PropertyKey != "columns" {
		t.Fatalf("PropertyKey = %q, want columns", cols.PropertyKey)
	}

	aliases := map[string][]string{}
	for _, ip := range cols.ItemProperties {
		aliases[ip.PropertyKey] = ip.MdlAliases
	}

	if got := aliases["header"]; len(got) != 1 || got[0] != "Caption" {
		t.Errorf("header MdlAliases = %v, want [Caption]", got)
	}
	if got := aliases["dynamicText"]; len(got) != 1 || got[0] != "Content" {
		t.Errorf("dynamicText MdlAliases = %v, want [Content]", got)
	}
	// tooltip and attribute have no aliases — schema name is the MDL keyword.
	if got := aliases["tooltip"]; len(got) != 0 {
		t.Errorf("tooltip MdlAliases = %v, want []", got)
	}
	if got := aliases["attribute"]; len(got) != 0 {
		t.Errorf("attribute MdlAliases = %v, want []", got)
	}
}

// TestItemSlotAcceptedChildTypes covers the routing of widget keywords (e.g.
// textfilter, numberfilter) directly inside an object-list item body. Without
// this routing, the engine would treat the filter widgets as default-slot
// (content) children, putting them in the wrong slot.
func TestItemSlotAcceptedChildTypes(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.datagrid.Datagrid",
		Name: "Data grid 2",
		Properties: []mpk.PropertyDef{
			{
				Key:    "columns",
				Type:   "object",
				IsList: true,
				Children: []mpk.PropertyDef{
					{Key: "content", Type: "widgets"},
					{Key: "filter", Type: "widgets"},
				},
			},
		},
	}
	def := GenerateDefJSON(mpkDef, "DATAGRID")
	if len(def.ObjectLists) != 1 {
		t.Fatalf("ObjectLists count = %d, want 1", len(def.ObjectLists))
	}

	slots := def.ObjectLists[0].ItemSlots
	got := map[string][]string{}
	for _, s := range slots {
		got[s.PropertyKey] = s.AcceptedChildTypes
	}

	if len(got["content"]) != 0 {
		t.Errorf("content slot AcceptedChildTypes = %v, want [] (no special routing)", got["content"])
	}
	wantFilter := []string{"textfilter", "numberfilter", "datefilter", "dropdownfilter"}
	if len(got["filter"]) != len(wantFilter) {
		t.Fatalf("filter slot AcceptedChildTypes = %v, want %v", got["filter"], wantFilter)
	}
	for i, w := range wantFilter {
		if got["filter"][i] != w {
			t.Errorf("filter[%d] = %q, want %q", i, got["filter"][i], w)
		}
	}
}
