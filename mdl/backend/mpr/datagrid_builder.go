// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/widgetobj"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets"
)

// BuildFilterWidget builds a filter widget for DataGrid2.
func (b *MprBackend) BuildFilterWidget(spec backend.FilterWidgetSpec, projectPath string) (pages.Widget, error) {
	bsonD := b.buildFilterWidgetBSON(spec.WidgetID, spec.FilterName, projectPath)

	// Wrap the BSON in a CustomWidget
	w := &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: spec.FilterName,
		},
		Editable:  "Always",
		RawObject: getBsonField(bsonD, "Object"),
		RawType:   getBsonField(bsonD, "Type"),
	}
	return w, nil
}

// ===========================================================================
// DataGrid2 column BSON construction — used by the ALTER PAGE column
// insert/replace path (page_mutator.go). The full-page build path routes
// through the pluggable widget engine (v0.12.0 B2); migrating ALTER PAGE
// columns to the engine too is tracked as follow-up.
// ===========================================================================

func (b *MprBackend) buildDataGrid2ColumnObject(col *backend.DataGridColumnSpec, columnObjectTypeID string, columnPropertyIDs map[string]pages.PropertyTypeIDEntry) bson.D {
	attrPath := col.Attribute

	// Serialize child widgets to BSON
	var contentWidgets []bson.D
	for _, child := range col.ChildWidgets {
		contentWidgets = append(contentWidgets, mpr.SerializeWidget(child))
	}
	hasCustomContent := len(contentWidgets) > 0

	properties := bson.A{int32(2)}

	// Sort keys for deterministic BSON output.
	colKeys := make([]string, 0, len(columnPropertyIDs))
	for k := range columnPropertyIDs {
		colKeys = append(colKeys, k)
	}
	sort.Strings(colKeys)

	hasDynamicText := !hasCustomContent && strings.EqualFold(col.ShowContentAs, "dynamicText")

	for _, key := range colKeys {
		entry := columnPropertyIDs[key]
		switch key {
		case "showContentAs":
			switch {
			case hasCustomContent:
				properties = append(properties, buildColumnPrimitiveProperty(entry, "customContent"))
			case hasDynamicText:
				properties = append(properties, buildColumnPrimitiveProperty(entry, "dynamicText"))
			default:
				properties = append(properties, buildColumnPrimitiveProperty(entry, "attribute"))
			}

		case "attribute":
			if attrPath != "" {
				properties = append(properties, buildColumnAttributeProperty(entry, attrPath))
			} else {
				properties = append(properties, buildColumnDefaultProperty(entry))
			}

		case "header":
			caption := col.Caption
			if caption == "" {
				caption = col.Attribute
			}
			properties = append(properties, buildColumnHeaderPropertyWithParams(entry, caption, col.CaptionParams))

		case "dynamicText":
			// Studio Pro stores the cell template here when ShowContentAs is
			// dynamicText. Otherwise the field is null (handled in the default
			// branch via buildColumnDefaultProperty).
			if hasDynamicText {
				properties = append(properties, buildColumnHeaderPropertyWithParams(entry, col.Content, col.ContentParams))
			} else {
				properties = append(properties, buildColumnDefaultProperty(entry))
			}

		case "content":
			if hasCustomContent {
				properties = append(properties, buildColumnContentProperty(entry, contentWidgets))
			} else {
				properties = append(properties, buildColumnContentProperty(entry, nil))
			}

		case "filter":
			if col.FilterWidget != nil {
				properties = append(properties, buildColumnContentProperty(entry, mpr.SerializeWidget(col.FilterWidget)))
			} else {
				properties = append(properties, buildColumnContentProperty(entry, nil))
			}

		case "visible":
			visExpr := "true"
			if col.Properties != nil {
				if v, ok := col.Properties["Visible"]; ok {
					if sv, isStr := v.(string); isStr && sv != "" {
						visExpr = sv
					}
				}
			}
			properties = append(properties, buildColumnExpressionProperty(entry, visExpr))

		case "columnClass":
			classExpr := ""
			if col.Properties != nil {
				if v, ok := col.Properties["DynamicCellClass"]; ok {
					if sv, isStr := v.(string); isStr {
						classExpr = sv
					}
				}
			}
			properties = append(properties, buildColumnExpressionProperty(entry, classExpr))

		case "sortable":
			defaultSortable := "false"
			if attrPath != "" {
				defaultSortable = "true"
			}
			sortVal := colPropBool(col.Properties, "Sortable", defaultSortable)
			properties = append(properties, buildColumnPrimitiveProperty(entry, sortVal))

		case "resizable":
			resVal := colPropBool(col.Properties, "Resizable", "true")
			properties = append(properties, buildColumnPrimitiveProperty(entry, resVal))

		case "draggable":
			dragVal := colPropBool(col.Properties, "Draggable", "true")
			properties = append(properties, buildColumnPrimitiveProperty(entry, dragVal))

		case "wrapText":
			wrapVal := colPropBool(col.Properties, "WrapText", "false")
			properties = append(properties, buildColumnPrimitiveProperty(entry, wrapVal))

		case "alignment":
			alignVal := colPropString(col.Properties, "Alignment", "left")
			properties = append(properties, buildColumnPrimitiveProperty(entry, alignVal))

		case "width":
			widthVal := colPropString(col.Properties, "ColumnWidth", "autoFill")
			properties = append(properties, buildColumnPrimitiveProperty(entry, widthVal))

		case "minWidth":
			properties = append(properties, buildColumnPrimitiveProperty(entry, "auto"))

		case "size":
			sizeVal := colPropInt(col.Properties, "Size", "1")
			properties = append(properties, buildColumnPrimitiveProperty(entry, sizeVal))

		case "hidable":
			hidVal := colPropString(col.Properties, "Hidable", "yes")
			properties = append(properties, buildColumnPrimitiveProperty(entry, hidVal))

		case "tooltip":
			// Studio Pro convention (verified against Cars_Overview):
			//   attribute column → empty ClientTemplate (no Translation entries)
			//   custom-content column → null
			if hasCustomContent {
				properties = append(properties, buildColumnDefaultProperty(entry))
			} else {
				tooltipText := ""
				if col.Properties != nil {
					if v, ok := col.Properties["Tooltip"]; ok {
						if sv, isStr := v.(string); isStr {
							tooltipText = sv
						}
					}
				}
				if tooltipText != "" {
					properties = append(properties, buildColumnHeaderProperty(entry, tooltipText))
				} else {
					properties = append(properties, buildColumnEmptyTextTemplateProperty(entry))
				}
			}

		case "exportValue":
			// Studio Pro convention (verified against Cars_Overview):
			//   attribute column → null
			//   custom-content column → empty ClientTemplate (no Translation entries)
			if hasCustomContent {
				properties = append(properties, buildColumnEmptyTextTemplateProperty(entry))
			} else {
				properties = append(properties, buildColumnDefaultProperty(entry))
			}

		default:
			switch entry.ValueType {
			case "Expression":
				properties = append(properties, buildColumnExpressionProperty(entry, ""))
			default:
				properties = append(properties, buildColumnDefaultProperty(entry))
			}
		}
	}

	var typePointer any
	if columnObjectTypeID != "" {
		typePointer = bsonutil.IDToBsonBinary(columnObjectTypeID)
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: properties},
		{Key: "TypePointer", Value: typePointer},
	}
}

// ===========================================================================
// BSON property builders (package-level, no receiver needed)
// ===========================================================================

func buildColumnPrimitiveProperty(entry pages.PropertyTypeIDEntry, value string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, nil, nil, value, nil, nil)},
	}
}

func buildColumnExpressionProperty(entry pages.PropertyTypeIDEntry, expression string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
			{Key: "Action", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NoAction"},
				{Key: "DisabledDuringExecution", Value: true},
			}},
			{Key: "AttributeRef", Value: nil},
			{Key: "DataSource", Value: nil},
			{Key: "EntityRef", Value: nil},
			{Key: "Expression", Value: expression},
			{Key: "Form", Value: ""},
			{Key: "Icon", Value: nil},
			{Key: "Image", Value: ""},
			{Key: "Microflow", Value: ""},
			{Key: "Nanoflow", Value: ""},
			{Key: "Objects", Value: bson.A{int32(2)}},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "Selection", Value: "None"},
			{Key: "SourceVariable", Value: nil},
			{Key: "TextTemplate", Value: nil},
			{Key: "TranslatableValue", Value: nil},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
			{Key: "Widgets", Value: bson.A{int32(2)}},
			{Key: "XPathConstraint", Value: ""},
		}},
	}
}

func buildColumnAttributeProperty(entry pages.PropertyTypeIDEntry, attrPath string) bson.D {
	var attributeRef any
	if strings.Count(attrPath, ".") >= 2 {
		attributeRef = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: attrPath},
			{Key: "EntityRef", Value: nil},
		}
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, nil, attributeRef, "", nil, nil)},
	}
}

func buildColumnHeaderProperty(entry pages.PropertyTypeIDEntry, caption string) bson.D {
	return buildColumnHeaderPropertyWithParams(entry, caption, nil)
}

// buildColumnHeaderPropertyWithParams emits a TextTemplate column property
// (header / dynamicText / tooltip) whose ClientTemplate carries the supplied
// ClientTemplateParameters. Pass nil params for plain-string templates.
func buildColumnHeaderPropertyWithParams(entry pages.PropertyTypeIDEntry, caption string, params []*pages.ClientTemplateParameter) bson.D {
	textTemplate := widgetobj.BuildClientTemplateWithTextAndParams(caption, params)

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, nil, nil, "", textTemplate, nil)},
	}
}

func buildColumnContentProperty(entry pages.PropertyTypeIDEntry, widgetsList any) bson.D {
	widgetsArray := bson.A{int32(2)}
	switch w := widgetsList.(type) {
	case bson.D:
		if w != nil {
			widgetsArray = append(widgetsArray, w)
		}
	case []bson.D:
		for _, widget := range w {
			widgetsArray = append(widgetsArray, widget)
		}
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, nil, nil, "", nil, widgetsArray)},
	}
}

func buildColumnDefaultProperty(entry pages.PropertyTypeIDEntry) bson.D {
	// For unset TextTemplate-typed column properties (dynamicText and
	// hasCustomContent-conditional ones) Studio Pro stores TextTemplate:
	// null, not an empty Forms$ClientTemplate. Emitting an empty
	// ClientTemplate triggers CE0463 "widget definition changed" when the
	// project is opened. For properties that DO want an empty ClientTemplate
	// (tooltip on attribute columns, exportValue on custom-content columns)
	// use buildColumnEmptyTextTemplateProperty instead.
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, nil, nil, entry.DefaultValue, nil, nil)},
	}
}

// buildColumnEmptyTextTemplateProperty emits a column property whose
// TextTemplate is an empty Forms$ClientTemplate (Items array with no
// Texts$Translation entries). This is what Studio Pro stores for unset
// `tooltip` on attribute-bound columns and unset `exportValue` on
// custom-content columns.
func buildColumnEmptyTextTemplateProperty(entry pages.PropertyTypeIDEntry) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, nil, nil, "", widgetobj.BuildEmptyClientTemplate(), nil)},
	}
}

// buildDefaultWidgetValueBSON builds a WidgetValue BSON with the given overrides.
// nil values use defaults.
func buildDefaultWidgetValueBSON(entry pages.PropertyTypeIDEntry, datasourceBSON any, attrRefBSON any, primitiveValue string, textTemplate any, widgetsArray bson.A) bson.D {
	if widgetsArray == nil {
		widgetsArray = bson.A{int32(2)}
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "Action", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$NoAction"},
			{Key: "DisabledDuringExecution", Value: true},
		}},
		{Key: "AttributeRef", Value: attrRefBSON},
		{Key: "DataSource", Value: datasourceBSON},
		{Key: "EntityRef", Value: nil},
		{Key: "Expression", Value: ""},
		{Key: "Form", Value: ""},
		{Key: "Icon", Value: nil},
		{Key: "Image", Value: ""},
		{Key: "Microflow", Value: ""},
		{Key: "Nanoflow", Value: ""},
		{Key: "Objects", Value: bson.A{int32(2)}},
		{Key: "PrimitiveValue", Value: primitiveValue},
		{Key: "Selection", Value: "None"},
		{Key: "SourceVariable", Value: nil},
		{Key: "TextTemplate", Value: textTemplate},
		{Key: "TranslatableValue", Value: nil},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.ValueTypeID)},
		{Key: "Widgets", Value: widgetsArray},
		{Key: "XPathConstraint", Value: ""},
	}
}

// ===========================================================================
// Cloning helpers (package-level)
// ===========================================================================

// ===========================================================================
// Deep cloning
// ===========================================================================

// ===========================================================================
// Column property helpers (domain logic — moved from executor)
// ===========================================================================

func colPropBool(props map[string]any, key string, defaultVal string) string {
	if props == nil {
		return defaultVal
	}
	v, ok := props[key]
	if !ok {
		return defaultVal
	}
	switch bv := v.(type) {
	case bool:
		if bv {
			return "true"
		}
		return "false"
	case string:
		lower := strings.ToLower(bv)
		if lower == "true" || lower == "false" {
			return lower
		}
		return defaultVal
	default:
		return defaultVal
	}
}

func colPropString(props map[string]any, key string, defaultVal string) string {
	if props == nil {
		return defaultVal
	}
	v, ok := props[key]
	if !ok {
		return defaultVal
	}
	if sv, isStr := v.(string); isStr && sv != "" {
		return strings.ToLower(sv)
	}
	return defaultVal
}

func colPropInt(props map[string]any, key string, defaultVal string) string {
	if props == nil {
		return defaultVal
	}
	v, ok := props[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return fmt.Sprintf("%d", n)
	case int64:
		return fmt.Sprintf("%d", n)
	case float64:
		return fmt.Sprintf("%d", int(n))
	case string:
		if n != "" {
			return n
		}
		return defaultVal
	default:
		return defaultVal
	}
}

// ===========================================================================
// Filter widget BSON construction
// ===========================================================================

func (b *MprBackend) buildFilterWidgetBSON(widgetID, filterName string, projectPath string) bson.D {
	rawType, rawObject, _, _, err := widgets.GetTemplateFullBSON(widgetID, types.GenerateID, projectPath)
	if err != nil || rawType == nil {
		if err != nil {
			log.Printf("warning: failed to load template for widget %s: %v; using minimal fallback", widgetID, err)
		}
		return b.buildMinimalFilterWidgetBSON(widgetID, filterName)
	}

	// A complete CustomWidget BSON requires Appearance, ConditionalEditability/
	// VisibilitySettings, LabelTemplate, and TabIndex alongside Type/Object.
	// Omitting Appearance triggers CE0463 ("definition of this widget has
	// changed") because Studio Pro requires every CustomWidget to carry the
	// full Forms$Page widget envelope, not just the inner widget-specific
	// payload. See docs/mpr-bson-shapes.md for reference.
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Appearance", Value: defaultEmptyAppearance()},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: "Always"},
		{Key: "LabelTemplate", Value: nil},
		{Key: "Name", Value: filterName},
		{Key: "Object", Value: rawObject},
		{Key: "TabIndex", Value: int32(0)},
		{Key: "Type", Value: rawType},
	}
}

// defaultEmptyAppearance returns the Forms$Appearance BSON for a widget that
// has no class, style, or design properties — matches what Studio Pro emits.
func defaultEmptyAppearance() bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$Appearance"},
		{Key: "Class", Value: ""},
		{Key: "DesignProperties", Value: bson.A{int32(3)}},
		{Key: "DynamicClasses", Value: ""},
		{Key: "Style", Value: ""},
	}
}

func (b *MprBackend) buildMinimalFilterWidgetBSON(widgetID, filterName string) bson.D {
	typeID := types.GenerateID()
	objectTypeID := types.GenerateID()
	objectID := types.GenerateID()

	var widgetTypeName string
	switch widgetID {
	case pages.WidgetIDDataGridTextFilter:
		widgetTypeName = "Text filter"
	case pages.WidgetIDDataGridNumberFilter:
		widgetTypeName = "Number filter"
	case pages.WidgetIDDataGridDateFilter:
		widgetTypeName = "Date filter"
	case pages.WidgetIDDataGridDropdownFilter:
		widgetTypeName = "Drop-down filter"
	default:
		widgetTypeName = "Text filter"
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Appearance", Value: defaultEmptyAppearance()},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: "Always"},
		{Key: "LabelTemplate", Value: nil},
		{Key: "Name", Value: filterName},
		{Key: "Object", Value: bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(objectID)},
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "Properties", Value: bson.A{int32(2)}},
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(objectTypeID)},
		}},
		{Key: "TabIndex", Value: int32(0)},
		{Key: "Type", Value: bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(typeID)},
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "HelpUrl", Value: ""},
			{Key: "ObjectType", Value: bson.D{
				{Key: "$ID", Value: bsonutil.IDToBsonBinary(objectTypeID)},
				{Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
				{Key: "PropertyTypes", Value: bson.A{int32(2)}},
			}},
			{Key: "OfflineCapable", Value: true},
			{Key: "StudioCategory", Value: "Data Controls"},
			{Key: "StudioProCategory", Value: "Data controls"},
			{Key: "SupportedPlatform", Value: "Web"},
			{Key: "WidgetDescription", Value: ""},
			{Key: "WidgetId", Value: widgetID},
			{Key: "WidgetName", Value: widgetTypeName},
			{Key: "WidgetNeedsEntityContext", Value: false},
			{Key: "WidgetPluginWidget", Value: true},
		}},
	}
}

// ===========================================================================
// BSON field helpers
// ===========================================================================

func getBsonField(d bson.D, key string) bson.D {
	for _, elem := range d {
		if elem.Key == key {
			if nested, ok := elem.Value.(bson.D); ok {
				return nested
			}
		}
	}
	return nil
}
