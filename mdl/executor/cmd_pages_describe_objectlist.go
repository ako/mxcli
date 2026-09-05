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
	// Children are the widgets nested in a Widgets-typed sub-property of the
	// item — an Accordion group's `content` / `headerContent` slot. Without
	// these an accordion described as an empty group, and re-executing that
	// description deleted whatever was inside it (#891).
	Children []rawWidget
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
			if len(item.Props) > 0 || item.DataSource != nil || len(item.Children) > 0 {
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
		// These branches must consume the property only when they actually
		// EXTRACTED something. A widget value carries every field it could
		// possibly have — Action, AttributeRef, DataSource, Expression,
		// TextTemplate, PrimitiveValue — most of them empty, so a branch that
		// `continue`s merely because its key EXISTS swallows the property and
		// the branches below it never run.
		//
		// The measured culprit is the ACTION branch below: `value["Action"]` is
		// present on every sub-property as a Forms$NoAction, and it continued
		// unconditionally. On an HTML Element authored by mxcli, that consumed
		// all six sub-properties of an `attribute` item; the item ended with
		// zero Props, the caller's `len(item.Props) > 0` filter dropped it, and
		// the whole `attributes` list vanished from DESCRIBE — while the list
		// itself resolved perfectly (probe: list="attributes" objects=1
		// nested=6). Isolated by reverting that one branch: object lists go
		// 2 -> 0.
		//
		// DataSource and AttributeRef are the same latent shape and are guarded
		// the same way. Neither is load-bearing for the measured case.
		if ds, ok := value["DataSource"].(map[string]any); ok && len(ds) > 0 {
			if rds := parseCustomWidgetDataSource(ctx, ds); rds != nil && rds.Reference != "" {
				item.DataSource = rds
			}
			// Consume it either way. A datasource that is PRESENT but could not
			// be rendered must not fall through to the scalar branches below —
			// they would describe it as its PrimitiveValue, which is a wrong
			// answer rather than a missing one. `len(ds) > 0` is the whole
			// change: an EMPTY map means the field is simply unset.
			continue
		}
		// Child widgets (an Accordion group's `content` slot). A Widgets-typed
		// sub-property holds a widget tree, not a scalar, so it is parsed with the
		// same recursion the rest of DESCRIBE uses rather than stringified (#891).
		if childElems := getBsonArrayElements(value["Widgets"]); len(childElems) > 0 {
			for _, ce := range childElems {
				cm, ok := ce.(map[string]any)
				if !ok {
					continue
				}
				item.Children = append(item.Children, parseRawWidget(ctx, cm)...)
			}
			continue
		}
		// Action sub-property (a chart series' staticOnClickAction, a popupmenu
		// item's action, a maps marker's onClick). Emitted under the schema key,
		// which is how MDL addresses an item action slot — there is no alias
		// (#956). A NoAction is the unset default and is skipped, so an
		// untouched item describes exactly as it did before.
		if action, ok := value["Action"].(map[string]any); ok && len(action) > 0 {
			if t := extractString(action["$Type"]); t != "Forms$NoAction" && t != "Pages$NoAction" {
				if mdl := renderClientActionMDL(ctx, action); mdl != "" {
					item.Props = append(item.Props, rawExplicitProp{
						Key: objectListMDLKey(key), Value: mdl, IsRef: true})
				}
				// A real action, rendered or not, is not a scalar.
				continue
			}
			// A NoAction is the unset default: fall through, since the property
			// may carry its value in one of the fields below.
		}
		// Attribute binding (staticXAttribute, staticYAttribute, …).
		if attrRef, ok := value["AttributeRef"].(map[string]any); ok && len(attrRef) > 0 {
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
		// TextTemplate sub-property (e.g. chart series `staticName`, a File
		// Uploader custom button's `buttonCaption`).
		if text, _ := extractTextTemplateText(value); text != "" {
			item.Props = append(item.Props, rawExplicitProp{Key: objectListMDLKey(key), Value: text})
			// …and its PARAMETERS. A template carrying `{1}` with no parameter is
			// CE0720 ("Place holder index 1 is greater than 0, the number of
			// parameter(s)"), so describing the text alone turns a valid page
			// into one that fails the build — measured on a Studio Pro-authored
			// File Uploader custom button (#956). The companion property is the
			// template's own name + "Params", which is the same convention the
			// engine reads back.
			if params := extractClientTemplateParameters(ctx, value, "TextTemplate"); len(params) > 0 {
				item.Props = append(item.Props, rawExplicitProp{
					Key:   objectListMDLKey(key) + "Params",
					Value: "[" + strings.Join(formatParametersV3(params), ", ") + "]",
					IsRef: true, // a param list is syntax, not a quoted literal
				})
			}
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

// objectListMDLKey is the MDL property name DESCRIBE emits for a widget schema
// sub-property: the schema key, verbatim.
//
// It used to upper-case the first letter. That round-tripped — MDL property
// names are case-insensitive, so `StaticName` and `staticName` both resolve —
// but it made DESCRIBE PAGE the only surface using that spelling:
//
//	mxcli widget describe htmlelement   attributeName      (from the .mpk)
//	what you write in a page            attributeName
//	DESCRIBE PAGE, before               AttributeName
//
// Three surfaces, two spellings, for no benefit. It was invisible while only
// chart series reached this code; slice 3 put it on every widget with an object
// list, which is what made it worth fixing.
//
// Emitting the key verbatim also keeps a real distinction visible that
// PascalCase erased: `DataSource:` stays capitalised because it is MDL's own
// keyword, not a widget schema key, so the two kinds of name no longer look
// alike.
func objectListMDLKey(schemaKey string) string {
	return schemaKey
}
