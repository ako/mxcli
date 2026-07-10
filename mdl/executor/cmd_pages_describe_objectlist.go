// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// rawObjectList is one object-list property of a pluggable widget reconstructed
// for DESCRIBE — e.g. a chart's `series` / `line` / `scalecolor` blocks. Keyword
// is the MDL child keyword (lowercase); Items are the list entries.
type rawObjectList struct {
	Keyword string
	Items   []rawObjectListItem
}

// rawObjectListItem is one entry of an object-list (a CustomWidgets$WidgetObject).
// Props are its scalar/attribute sub-properties (MDL-cased keys); DataSource is
// its per-item datasource, if any (chart series bind their own datasource).
type rawObjectListItem struct {
	Props      []rawExplicitProp
	DataSource *rawDataSource
}

// extractObjectLists reconstructs every object-list property of a pluggable
// widget (chart series/lines/scaleColors, …) from its Object BSON. Mirrors the
// DataGrid2 column reconstruction (extractDataGrid2Columns) but generic: any
// property whose Value carries an `Objects` array and whose nested schema is
// resolvable becomes an object-list. Used only by the generic `pluggablewidget`
// DESCRIBE branch — DataGrid2/Gallery have their own specialized output, so this
// does not double-handle their columns.
func extractObjectLists(ctx *ExecContext, w map[string]any) []rawObjectList {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}
	topKeyMap := buildPropertyTypeKeyMap(w, true)
	if len(topKeyMap) == 0 {
		return nil
	}

	var out []rawObjectList
	for _, prop := range getBsonArrayElements(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		objects := getBsonArrayElements(value["Objects"])
		if len(objects) == 0 {
			continue
		}
		listKey := topKeyMap[extractBinaryID(propMap["TypePointer"])]
		if listKey == "" {
			continue
		}
		nestedMap := buildObjectListNestedKeyMap(w, listKey)
		if len(nestedMap) == 0 {
			continue
		}

		var items []rawObjectListItem
		for _, o := range objects {
			om, ok := o.(map[string]any)
			if !ok {
				continue
			}
			item := extractObjectListItem(ctx, om, nestedMap)
			if len(item.Props) > 0 || item.DataSource != nil {
				items = append(items, item)
			}
		}
		if len(items) > 0 {
			out = append(out, rawObjectList{
				Keyword: strings.ToLower(deriveObjectListKeyword(listKey)),
				Items:   items,
			})
		}
	}
	return out
}

// buildObjectListNestedKeyMap builds the TypePointer→sub-property-key map for one
// object-list property (identified by listKey) from the widget Type. Generalizes
// buildColumnPropertyKeyMap (which hard-codes "columns") to any list property.
func buildObjectListNestedKeyMap(w map[string]any, listKey string) map[string]string {
	result := make(map[string]string)
	widgetType, ok := w["Type"].(map[string]any)
	if !ok {
		return result
	}
	objType, ok := widgetType["ObjectType"].(map[string]any)
	if !ok {
		return result
	}
	for _, pt := range getBsonArrayElements(objType["PropertyTypes"]) {
		ptMap, ok := pt.(map[string]any)
		if !ok || extractString(ptMap["PropertyKey"]) != listKey {
			continue
		}
		valueType, ok := ptMap["ValueType"].(map[string]any)
		if !ok {
			break
		}
		itemObjType, ok := valueType["ObjectType"].(map[string]any)
		if !ok {
			break
		}
		for _, ipt := range getBsonArrayElements(itemObjType["PropertyTypes"]) {
			iptMap, ok := ipt.(map[string]any)
			if !ok {
				continue
			}
			key := extractString(iptMap["PropertyKey"])
			id := extractBinaryID(iptMap["$ID"])
			if key != "" && id != "" {
				result[id] = key
			}
		}
		break
	}
	return result
}

// extractObjectListItem reconstructs one object-list entry's sub-properties.
// Datasource sub-properties (staticDataSource/dynamicDataSource) become the
// item's DataSource; attribute refs, expressions and primitives become Props
// with MDL-cased keys.
func extractObjectListItem(ctx *ExecContext, itemObj map[string]any, nestedMap map[string]string) rawObjectListItem {
	var item rawObjectListItem
	for _, prop := range getBsonArrayElements(itemObj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		key := nestedMap[extractBinaryID(propMap["TypePointer"])]
		if key == "" {
			continue
		}

		// Per-item datasource (e.g. chart series `staticDataSource`).
		if ds, ok := value["DataSource"].(map[string]any); ok && ds != nil {
			if rds := parseCustomWidgetDataSource(ctx, ds); rds != nil && rds.Reference != "" {
				item.DataSource = rds
			}
			continue
		}
		// Attribute binding (staticXAttribute, staticYAttribute, …).
		if attrRef, ok := value["AttributeRef"].(map[string]any); ok && attrRef != nil {
			if a := extractString(attrRef["Attribute"]); a != "" {
				item.Props = append(item.Props, rawExplicitProp{Key: objectListMDLKey(key), Value: shortAttributeName(a), IsRef: true})
			}
			continue
		}
		// Expression sub-property.
		if expr := extractString(value["Expression"]); expr != "" {
			item.Props = append(item.Props, rawExplicitProp{Key: objectListMDLKey(key), Value: expr})
			continue
		}
		// TextTemplate sub-property (e.g. chart series `staticName`).
		if text, _ := extractTextTemplateText(value); text != "" {
			item.Props = append(item.Props, rawExplicitProp{Key: objectListMDLKey(key), Value: text})
			continue
		}
		// Primitive value (dataSet, interpolation, colorValue, …). Skip
		// whitespace-only defaults (e.g. customSeriesOptions: " ") as noise.
		if pv := extractString(value["PrimitiveValue"]); strings.TrimSpace(pv) != "" {
			item.Props = append(item.Props, rawExplicitProp{Key: objectListMDLKey(key), Value: pv})
		}
	}
	return item
}

// objectListMDLKey maps a widget schema sub-property key to the MDL property name
// the DESCRIBE output uses. MDL property names are case-insensitive, so the
// canonical PascalCase form (first letter upper) round-trips to the same schema
// key on re-exec (staticXAttribute→StaticXAttribute, staticName→StaticName).
func objectListMDLKey(schemaKey string) string {
	if schemaKey == "" {
		return schemaKey
	}
	return strings.ToUpper(schemaKey[:1]) + schemaKey[1:]
}
