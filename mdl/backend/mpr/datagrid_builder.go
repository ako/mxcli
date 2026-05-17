// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets"
)

// BuildDataGrid2Widget builds a complete DataGrid2 CustomWidget from domain-typed inputs.
func (b *MprBackend) BuildDataGrid2Widget(id model.ID, name string, spec backend.DataGridSpec, projectPath string) (*pages.CustomWidget, error) {
	// Load embedded template
	embeddedType, embeddedObject, embeddedIDs, embeddedObjectTypeID, err :=
		widgets.GetTemplateFullBSON(pages.WidgetIDDataGrid2, types.GenerateID, projectPath)
	if err != nil {
		return nil, mdlerrors.NewBackend("load DataGrid2 template", err)
	}
	if embeddedType == nil || embeddedObject == nil {
		return nil, mdlerrors.NewNotFound("widget template", "DataGrid2")
	}

	propertyTypeIDs := convertPropertyTypeIDs(embeddedIDs)

	// Build the object
	var updatedObject bson.D
	if len(spec.Columns) > 0 || len(spec.HeaderWidgets) > 0 {
		updatedObject = b.updateDataGrid2Object(embeddedObject, propertyTypeIDs, spec)
	} else {
		updatedObject = b.cloneDataGrid2ObjectWithDatasourceOnly(embeddedObject, propertyTypeIDs, spec.DataSource)
	}

	// Apply paging overrides
	if len(spec.PagingOverrides) > 0 {
		updatedObject = b.applyDataGridPagingProps(updatedObject, propertyTypeIDs, spec.PagingOverrides)
	}

	// Apply selection mode
	if spec.SelectionMode != "" {
		updatedObject = b.applyDataGridSelectionProp(updatedObject, propertyTypeIDs, spec.SelectionMode)
	}

	grid := &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       id,
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: name,
		},
		Editable:          "Always",
		RawType:           embeddedType,
		RawObject:         updatedObject,
		PropertyTypeIDMap: propertyTypeIDs,
		ObjectTypeID:      embeddedObjectTypeID,
	}

	return grid, nil
}

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
// DataGrid2 BSON construction (moved from executor)
// ===========================================================================

func (b *MprBackend) updateDataGrid2Object(templateObject bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, spec backend.DataGridSpec) bson.D {
	result := make(bson.D, 0, len(templateObject))

	for _, elem := range templateObject {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Properties" {
			if propsArr, ok := elem.Value.(bson.A); ok {
				updatedProps := b.updateDataGrid2Properties(propsArr, propertyTypeIDs, spec)
				result = append(result, bson.E{Key: "Properties", Value: updatedProps})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

func (b *MprBackend) updateDataGrid2Properties(props bson.A, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, spec backend.DataGridSpec) bson.A {
	result := bson.A{int32(2)}

	datasourceEntry := propertyTypeIDs["datasource"]
	columnsEntry := propertyTypeIDs["columns"]
	filtersPlaceholderEntry := propertyTypeIDs["filtersPlaceholder"]

	// Serialize header widgets to BSON
	var headerWidgetsBSON []bson.D
	for _, w := range spec.HeaderWidgets {
		headerWidgetsBSON = append(headerWidgetsBSON, mpr.SerializeWidget(w))
	}

	for _, propVal := range props {
		if _, ok := propVal.(int32); ok {
			continue
		}

		propMap, ok := propVal.(bson.D)
		if !ok {
			continue
		}

		typePointer := getTypePointerFromProperty(propMap)

		if typePointer == datasourceEntry.PropertyTypeID {
			result = append(result, buildDataGrid2Property(datasourceEntry, spec.DataSource, "", "", b))
		} else if typePointer == columnsEntry.PropertyTypeID {
			result = append(result, b.cloneAndUpdateColumnsProperty(propMap, columnsEntry, propertyTypeIDs, spec.Columns))
		} else if typePointer == filtersPlaceholderEntry.PropertyTypeID && len(headerWidgetsBSON) > 0 {
			result = append(result, buildFiltersPlaceholderProperty(filtersPlaceholderEntry, headerWidgetsBSON))
		} else {
			result = append(result, clonePropertyWithNewIDs(propMap))
		}
	}

	return result
}

func (b *MprBackend) cloneAndUpdateColumnsProperty(templateProp bson.D, columnsEntry pages.PropertyTypeIDEntry, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, columns []backend.DataGridColumnSpec) bson.D {
	// Extract template column object
	var templateColumnObj bson.D
	for _, elem := range templateProp {
		if elem.Key == "Value" {
			if valMap, ok := elem.Value.(bson.D); ok {
				for _, ve := range valMap {
					if ve.Key == "Objects" {
						if objArr, ok := ve.Value.(bson.A); ok {
							for _, obj := range objArr {
								if colObj, ok := obj.(bson.D); ok {
									templateColumnObj = colObj
									break
								}
							}
						}
					}
				}
			}
		}
	}

	columnObjects := bson.A{int32(2)}
	for i := range columns {
		col := &columns[i]
		if templateColumnObj != nil {
			columnObjects = append(columnObjects, b.cloneAndUpdateColumnObject(templateColumnObj, col, columnsEntry.NestedPropertyIDs))
		} else {
			columnObjects = append(columnObjects, b.buildDataGrid2ColumnObject(col, columnsEntry.ObjectTypeID, columnsEntry.NestedPropertyIDs))
		}
	}

	result := make(bson.D, 0, len(templateProp))
	for _, elem := range templateProp {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Value" {
			if valMap, ok := elem.Value.(bson.D); ok {
				newVal := make(bson.D, 0, len(valMap))
				for _, ve := range valMap {
					if ve.Key == "$ID" {
						newVal = append(newVal, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
					} else if ve.Key == "Objects" {
						newVal = append(newVal, bson.E{Key: "Objects", Value: columnObjects})
					} else if ve.Key == "Action" {
						if actionMap, ok := ve.Value.(bson.D); ok {
							newVal = append(newVal, bson.E{Key: "Action", Value: deepCloneWithNewIDs(actionMap)})
						} else {
							newVal = append(newVal, ve)
						}
					} else {
						newVal = append(newVal, ve)
					}
				}
				result = append(result, bson.E{Key: "Value", Value: newVal})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

func (b *MprBackend) cloneAndUpdateColumnObject(templateCol bson.D, col *backend.DataGridColumnSpec, columnPropertyIDs map[string]pages.PropertyTypeIDEntry) bson.D {
	attrPath := col.Attribute
	caption := col.Caption
	if caption == "" {
		caption = col.Attribute
	}

	// Serialize child widgets to BSON
	var contentWidgets []bson.D
	for _, child := range col.ChildWidgets {
		contentWidgets = append(contentWidgets, mpr.SerializeWidget(child))
	}

	result := make(bson.D, 0, len(templateCol))
	for _, elem := range templateCol {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Properties" {
			if propsArr, ok := elem.Value.(bson.A); ok {
				result = append(result, bson.E{Key: "Properties", Value: b.cloneAndUpdateColumnProperties(propsArr, columnPropertyIDs, col, attrPath, caption, contentWidgets)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func (b *MprBackend) cloneAndUpdateColumnProperties(templateProps bson.A, columnPropertyIDs map[string]pages.PropertyTypeIDEntry, col *backend.DataGridColumnSpec, attrPath, caption string, contentWidgets []bson.D) bson.A {
	result := bson.A{int32(2)}

	addedProps := make(map[string]bool)
	hasCustomContent := len(contentWidgets) > 0

	for _, propVal := range templateProps {
		if _, ok := propVal.(int32); ok {
			continue
		}
		propMap, ok := propVal.(bson.D)
		if !ok {
			continue
		}

		typePointer := getTypePointerFromProperty(propMap)

		propKey := ""
		for key, entry := range columnPropertyIDs {
			if entry.PropertyTypeID == typePointer {
				addedProps[key] = true
				propKey = key
				break
			}
		}

		hasDynamicText := !hasCustomContent && strings.EqualFold(col.ShowContentAs, "dynamicText")

		switch propKey {
		case "showContentAs":
			switch {
			case hasCustomContent:
				result = append(result, clonePropertyWithPrimitiveValue(propMap, "customContent"))
			case hasDynamicText:
				result = append(result, clonePropertyWithPrimitiveValue(propMap, "dynamicText"))
			default:
				result = append(result, clonePropertyWithNewIDs(propMap))
			}
		case "attribute":
			if attrPath != "" {
				entry := columnPropertyIDs["attribute"]
				result = append(result, buildColumnAttributeProperty(entry, attrPath))
			} else {
				result = append(result, clonePropertyWithNewIDs(propMap))
			}
		case "header":
			entry := columnPropertyIDs["header"]
			result = append(result, buildColumnHeaderPropertyWithParams(entry, caption, col.CaptionParams))
		case "dynamicText":
			if hasDynamicText {
				entry := columnPropertyIDs["dynamicText"]
				result = append(result, buildColumnHeaderPropertyWithParams(entry, col.Content, col.ContentParams))
			} else {
				result = append(result, clonePropertyClearingTextTemplate(propMap))
			}
		case "content":
			if hasCustomContent {
				entry := columnPropertyIDs["content"]
				result = append(result, buildColumnContentProperty(entry, contentWidgets))
			} else {
				result = append(result, clonePropertyWithNewIDs(propMap))
			}
		case "filter":
			if col.FilterWidget != nil {
				entry := columnPropertyIDs["filter"]
				result = append(result, buildColumnContentProperty(entry, mpr.SerializeWidget(col.FilterWidget)))
			} else {
				result = append(result, clonePropertyWithNewIDs(propMap))
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
			result = append(result, clonePropertyWithExpression(propMap, visExpr))

		case "columnClass":
			classExpr := ""
			if col.Properties != nil {
				if v, ok := col.Properties["DynamicCellClass"]; ok {
					if sv, isStr := v.(string); isStr {
						classExpr = sv
					}
				}
			}
			result = append(result, clonePropertyWithExpression(propMap, classExpr))

		case "tooltip":
			if hasCustomContent {
				result = append(result, clonePropertyClearingTextTemplate(propMap))
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
					entry := columnPropertyIDs["tooltip"]
					result = append(result, buildColumnHeaderProperty(entry, tooltipText))
				} else {
					result = append(result, clonePropertyWithNewIDs(propMap))
				}
			}
		case "exportValue":
			if hasCustomContent {
				entry := columnPropertyIDs["exportValue"]
				result = append(result, buildColumnHeaderProperty(entry, ""))
			} else {
				result = append(result, clonePropertyWithNewIDs(propMap))
			}
		case "allowEventPropagation":
			result = append(result, clonePropertyWithNewIDs(propMap))

		case "sortable":
			defaultSortable := "false"
			if attrPath != "" {
				defaultSortable = "true"
			}
			sortVal := colPropBool(col.Properties, "Sortable", defaultSortable)
			result = append(result, clonePropertyWithPrimitiveValue(propMap, sortVal))

		case "resizable":
			resVal := colPropBool(col.Properties, "Resizable", "true")
			result = append(result, clonePropertyWithPrimitiveValue(propMap, resVal))

		case "draggable":
			dragVal := colPropBool(col.Properties, "Draggable", "true")
			result = append(result, clonePropertyWithPrimitiveValue(propMap, dragVal))

		case "hidable":
			hidVal := colPropString(col.Properties, "Hidable", "yes")
			result = append(result, clonePropertyWithPrimitiveValue(propMap, hidVal))

		case "width":
			widthVal := colPropString(col.Properties, "ColumnWidth", "autoFill")
			result = append(result, clonePropertyWithPrimitiveValue(propMap, widthVal))

		case "size":
			sizeVal := colPropInt(col.Properties, "Size", "1")
			result = append(result, clonePropertyWithPrimitiveValue(propMap, sizeVal))

		case "wrapText":
			wrapVal := "false"
			if col.Properties != nil {
				if v, ok := col.Properties["WrapText"]; ok {
					if bv, isBool := v.(bool); isBool && bv {
						wrapVal = "true"
					} else if sv, isStr := v.(string); isStr {
						wrapVal = strings.ToLower(sv)
					}
				}
			}
			result = append(result, clonePropertyWithPrimitiveValue(propMap, wrapVal))

		case "alignment":
			alignVal := "left"
			if col.Properties != nil {
				if v, ok := col.Properties["Alignment"]; ok {
					if sv, isStr := v.(string); isStr && sv != "" {
						alignVal = strings.ToLower(sv)
					}
				}
			}
			result = append(result, clonePropertyWithPrimitiveValue(propMap, alignVal))

		default:
			result = append(result, clonePropertyWithNewIDs(propMap))
		}
	}

	// Add required properties that were missing from template
	if !addedProps["visible"] {
		if visibleEntry, ok := columnPropertyIDs["visible"]; ok {
			visExpr := "true"
			if col.Properties != nil {
				if v, ok := col.Properties["Visible"]; ok {
					if sv, isStr := v.(string); isStr && sv != "" {
						visExpr = sv
					}
				}
			}
			result = append(result, buildColumnExpressionProperty(visibleEntry, visExpr))
		}
	}

	return result
}

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

func (b *MprBackend) cloneDataGrid2ObjectWithDatasourceOnly(templateObject bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, datasource pages.DataSource) bson.D {
	result := make(bson.D, 0, len(templateObject))

	for _, elem := range templateObject {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Properties" {
			if propsArr, ok := elem.Value.(bson.A); ok {
				updatedProps := b.updateOnlyDatasource(propsArr, propertyTypeIDs, datasource)
				result = append(result, bson.E{Key: "Properties", Value: updatedProps})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

func (b *MprBackend) updateOnlyDatasource(props bson.A, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, datasource pages.DataSource) bson.A {
	result := bson.A{int32(2)}
	datasourceEntry := propertyTypeIDs["datasource"]

	for _, propVal := range props {
		if _, ok := propVal.(int32); ok {
			continue
		}
		propMap, ok := propVal.(bson.D)
		if !ok {
			continue
		}

		typePointer := getTypePointerFromProperty(propMap)
		if typePointer == datasourceEntry.PropertyTypeID {
			result = append(result, buildDataGrid2Property(datasourceEntry, datasource, "", "", b))
		} else {
			result = append(result, clonePropertyWithNewIDs(propMap))
		}
	}

	return result
}

// applyDataGridPagingProps applies paging property overrides to a DataGrid2 BSON object.
func (b *MprBackend) applyDataGridPagingProps(obj bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, overrides map[string]string) bson.D {
	if len(overrides) == 0 {
		return obj
	}

	typePointerToKey := make(map[string]string)
	for widgetKey, entry := range propertyTypeIDs {
		typePointerToKey[entry.PropertyTypeID] = widgetKey
	}

	result := make(bson.D, 0, len(obj))
	for _, elem := range obj {
		if elem.Key == "Properties" {
			if propsArr, ok := elem.Value.(bson.A); ok && len(propsArr) > 0 {
				updatedProps := bson.A{propsArr[0]}
				for _, propVal := range propsArr[1:] {
					propMap, ok := propVal.(bson.D)
					if !ok {
						updatedProps = append(updatedProps, propVal)
						continue
					}
					tp := getTypePointerFromProperty(propMap)
					widgetKey := typePointerToKey[tp]
					if newVal, hasOverride := overrides[widgetKey]; hasOverride {
						updatedProps = append(updatedProps, clonePropertyWithPrimitiveValue(propMap, newVal))
					} else {
						updatedProps = append(updatedProps, propMap)
					}
				}
				result = append(result, bson.E{Key: "Properties", Value: updatedProps})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// applyDataGridSelectionProp applies the Selection mode to a DataGrid2 object.
func (b *MprBackend) applyDataGridSelectionProp(obj bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, selectionMode string) bson.D {
	itemSelectionEntry, ok := propertyTypeIDs["itemSelection"]
	if !ok {
		return obj
	}

	result := make(bson.D, 0, len(obj))
	for _, elem := range obj {
		if elem.Key == "Properties" {
			if propsArr, ok := elem.Value.(bson.A); ok {
				updatedProps := bson.A{propsArr[0]}
				for _, propVal := range propsArr[1:] {
					propMap, ok := propVal.(bson.D)
					if !ok {
						updatedProps = append(updatedProps, propVal)
						continue
					}
					tp := getTypePointerFromProperty(propMap)
					if tp == itemSelectionEntry.PropertyTypeID {
						updatedProps = append(updatedProps, buildGallerySelectionProperty(propMap, selectionMode))
					} else {
						updatedProps = append(updatedProps, propMap)
					}
				}
				result = append(result, bson.E{Key: "Properties", Value: updatedProps})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// ===========================================================================
// BSON property builders (package-level, no receiver needed)
// ===========================================================================

// buildDataGrid2Property builds a single property BSON document for a DataGrid2 column.
// attrRef and primitiveValue are reserved for future column types that require direct
// attribute references or primitive default values; current callers pass empty strings.
func buildDataGrid2Property(entry pages.PropertyTypeIDEntry, datasource pages.DataSource, attrRef string, primitiveValue string, _ *MprBackend) bson.D {
	var datasourceBSON any
	if datasource != nil {
		datasourceBSON = mpr.SerializeCustomWidgetDataSource(datasource)
	}

	var attrRefBSON any
	if attrRef != "" {
		attrRefBSON = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: attrRef},
			{Key: "EntityRef", Value: nil},
		}
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, datasourceBSON, attrRefBSON, primitiveValue, nil, nil)},
	}
}

func buildFiltersPlaceholderProperty(entry pages.PropertyTypeIDEntry, widgetsBSON []bson.D) bson.D {
	widgetsArray := bson.A{int32(2)}
	for _, w := range widgetsBSON {
		widgetsArray = append(widgetsArray, w)
	}

	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(entry.PropertyTypeID)},
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, nil, nil, "", nil, widgetsArray)},
	}
}

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
	textTemplate := buildClientTemplateWithTextAndParams(caption, params)

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
		{Key: "Value", Value: buildDefaultWidgetValueBSON(entry, nil, nil, "", buildEmptyClientTemplate(), nil)},
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

// buildClientTemplateWithTextAndParams builds a Forms$ClientTemplate with the
// given Template text and an optional list of ClientTemplateParameters.
// Mirrors sdk/mpr/writer_widgets.go:serializeClientTemplate for the templated
// column header / dynamicText paths.
func buildClientTemplateWithTextAndParams(text string, params []*pages.ClientTemplateParameter) bson.D {
	parametersArr := bson.A{int32(2)} // empty array marker; populated below if params exist
	if len(params) > 0 {
		for _, p := range params {
			parametersArr = append(parametersArr, serializeColumnClientTemplateParameter(p))
		}
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "Parameters", Value: parametersArr},
		{Key: "Template", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{
				int32(3),
				bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: "en_US"},
					{Key: "Text", Value: text},
				},
			}},
		}},
	}
}

// serializeColumnClientTemplateParameter serializes a ClientTemplateParameter
// for embedding inside a column TextTemplate. Mirrors the structure used by
// sdk/mpr/writer_widgets.go:serializeClientTemplateParameter (Forms$FormattingInfo
// schema-aligned to avoid CE0463).
func serializeColumnClientTemplateParameter(param *pages.ClientTemplateParameter) bson.D {
	paramID := bsonutil.NewIDBsonBinary()
	if param.ID != "" {
		paramID = bsonutil.IDToBsonBinary(string(param.ID))
	}

	var attrRefBSON any
	if param.AttributeRef != "" {
		attrRefBSON = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: param.AttributeRef},
			{Key: "EntityRef", Value: nil},
		}
	}

	formattingInfo := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$FormattingInfo"},
		{Key: "CustomDateFormat", Value: ""},
		{Key: "DateFormat", Value: "Date"},
		{Key: "DecimalPrecision", Value: int64(2)},
		{Key: "EnumFormat", Value: "Text"},
		{Key: "GroupDigits", Value: false},
	}

	var sourceVariable any
	if param.SourceVariable != "" {
		sourceVariable = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$PageVariable"},
			{Key: "PageParameter", Value: param.SourceVariable},
			{Key: "UseAllPages", Value: false},
			{Key: "Widget", Value: ""},
		}
	}

	return bson.D{
		{Key: "$ID", Value: paramID},
		{Key: "$Type", Value: "Forms$ClientTemplateParameter"},
		{Key: "AttributeRef", Value: attrRefBSON},
		{Key: "Expression", Value: param.Expression},
		{Key: "FormattingInfo", Value: formattingInfo},
		{Key: "SourceVariable", Value: sourceVariable},
	}
}

func buildEmptyClientTemplate() bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Template", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
	}
}

// ===========================================================================
// Cloning helpers (package-level)
// ===========================================================================

func getTypePointerFromProperty(prop bson.D) string {
	for _, elem := range prop {
		if elem.Key == "TypePointer" {
			switch v := elem.Value.(type) {
			case primitive.Binary:
				return bsonutil.BsonBinaryToID(v)
			case []byte:
				return types.BlobToUUID(v)
			}
		}
	}
	return ""
}

func clonePropertyWithNewIDs(prop bson.D) bson.D {
	result := make(bson.D, 0, len(prop))
	for _, elem := range prop {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Value" {
			if valMap, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Value", Value: deepCloneWithNewIDs(valMap)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func clonePropertyWithPrimitiveValue(prop bson.D, newValue string) bson.D {
	result := make(bson.D, 0, len(prop))
	for _, elem := range prop {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Value" {
			if valMap, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Value", Value: cloneValueWithUpdatedPrimitive(valMap, newValue)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func cloneValueWithUpdatedPrimitive(val bson.D, newValue string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "PrimitiveValue" {
			result = append(result, bson.E{Key: "PrimitiveValue", Value: newValue})
		} else {
			result = append(result, bson.E{Key: elem.Key, Value: deepCloneValue(elem.Value)})
		}
	}
	return result
}

func clonePropertyWithExpression(prop bson.D, newExpr string) bson.D {
	result := make(bson.D, 0, len(prop))
	for _, elem := range prop {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Value" {
			if valMap, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Value", Value: cloneValueWithUpdatedExpression(valMap, newExpr)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func cloneValueWithUpdatedExpression(val bson.D, newExpr string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Expression" {
			result = append(result, bson.E{Key: "Expression", Value: newExpr})
		} else {
			result = append(result, bson.E{Key: elem.Key, Value: deepCloneValue(elem.Value)})
		}
	}
	return result
}

func clonePropertyClearingTextTemplate(prop bson.D) bson.D {
	result := make(bson.D, 0, len(prop))
	for _, elem := range prop {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "Value" {
			if valMap, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Value", Value: cloneValueClearingTextTemplate(valMap)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func cloneValueClearingTextTemplate(val bson.D) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else if elem.Key == "TextTemplate" {
			result = append(result, bson.E{Key: "TextTemplate", Value: nil})
		} else {
			result = append(result, bson.E{Key: elem.Key, Value: deepCloneValue(elem.Value)})
		}
	}
	return result
}

// ===========================================================================
// Deep cloning
// ===========================================================================

func deepCloneWithNewIDs(doc bson.D) bson.D {
	result := make(bson.D, 0, len(doc))
	for _, elem := range doc {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else {
			result = append(result, bson.E{Key: elem.Key, Value: deepCloneValue(elem.Value)})
		}
	}
	return result
}

func deepCloneValue(v any) any {
	switch val := v.(type) {
	case bson.D:
		return deepCloneWithNewIDs(val)
	case bson.A:
		return deepCloneArray(val)
	case []any:
		return deepCloneSlice(val)
	default:
		return v
	}
}

func deepCloneArray(arr bson.A) bson.A {
	result := make(bson.A, len(arr))
	for i, elem := range arr {
		result[i] = deepCloneValue(elem)
	}
	return result
}

func deepCloneSlice(arr []any) []any {
	result := make([]any, len(arr))
	for i, elem := range arr {
		result[i] = deepCloneValue(elem)
	}
	return result
}

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
