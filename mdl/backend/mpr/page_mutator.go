// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Compile-time check.
var _ backend.PageMutator = (*mprPageMutator)(nil)

// mprPageMutator implements backend.PageMutator for the MPR backend.
type mprPageMutator struct {
	rawData       bson.D
	containerType backend.ContainerKind // "page", "snippet", or "layout"
	unitID        model.ID
	backend       *MprBackend
	widgetFinder  widgetFinder
}

// ---------------------------------------------------------------------------
// OpenPageForMutation
// ---------------------------------------------------------------------------

// OpenPageForMutation loads a page/snippet/layout unit and returns a PageMutator.
func (b *MprBackend) OpenPageForMutation(unitID model.ID) (backend.PageMutator, error) {
	rawBytes, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil {
		return nil, fmt.Errorf("load raw unit bytes: %w", err)
	}
	var rawData bson.D
	if err := bson.Unmarshal(rawBytes, &rawData); err != nil {
		return nil, fmt.Errorf("unmarshal unit BSON: %w", err)
	}

	// Determine container type from $Type field.
	typeName := dGetString(rawData, "$Type")
	containerType := backend.ContainerPage
	switch {
	case strings.Contains(typeName, "Snippet"):
		containerType = backend.ContainerSnippet
	case strings.Contains(typeName, "Layout"):
		containerType = backend.ContainerLayout
	}

	finder := findBsonWidget
	if containerType == backend.ContainerSnippet {
		finder = findBsonWidgetInSnippet
	}

	return &mprPageMutator{
		rawData:       rawData,
		containerType: containerType,
		unitID:        unitID,
		backend:       b,
		widgetFinder:  finder,
	}, nil
}

// ---------------------------------------------------------------------------
// PageMutator interface implementation
// ---------------------------------------------------------------------------

func (m *mprPageMutator) ContainerType() backend.ContainerKind { return m.containerType }

func (m *mprPageMutator) SetWidgetProperty(widgetRef string, prop string, value any) error {
	if widgetRef == "" {
		// Page-level property
		return applyPageLevelSetMut(m.rawData, prop, value)
	}
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	// DataGrid2 columns are WidgetObjects, not form widgets — use the column setter.
	if len(result.colPropKeys) > 0 {
		return setColumnPropertyMut(result.widget, result.colPropKeys, prop, value)
	}
	return setRawWidgetPropertyMut(result.widget, prop, value)
}

func (m *mprPageMutator) SetWidgetDataSource(widgetRef string, ds pages.DataSource) error {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	serialized := serializeDataSourceBson(ds)
	if serialized == nil {
		return fmt.Errorf("unsupported DataSource type %T", ds)
	}
	dSet(result.widget, "DataSource", serialized)
	return nil
}

func (m *mprPageMutator) SetColumnProperty(gridRef string, columnRef string, prop string, value any) error {
	result := findBsonColumn(m.rawData, gridRef, columnRef, m.widgetFinder)
	if result == nil {
		return fmt.Errorf("column %q on grid %q not found", columnRef, gridRef)
	}
	return setColumnPropertyMut(result.widget, result.colPropKeys, prop, value)
}

func (m *mprPageMutator) InsertWidget(widgetRef string, columnRef string, position backend.InsertPosition, widgets []pages.Widget) error {
	var result *bsonWidgetResult
	if columnRef != "" {
		result = findBsonColumn(m.rawData, widgetRef, columnRef, m.widgetFinder)
	} else {
		result = m.widgetFinder(m.rawData, widgetRef)
	}
	if result == nil {
		if columnRef != "" {
			return fmt.Errorf("column %q on widget %q not found", columnRef, widgetRef)
		}
		return fmt.Errorf("widget %q not found", widgetRef)
	}

	// Serialize widgets
	newBsonWidgets, err := serializeWidgets(widgets)
	if err != nil {
		return fmt.Errorf("serialize widgets: %w", err)
	}

	insertIdx := result.index
	if strings.EqualFold(string(position), "after") {
		insertIdx = result.index + 1
	}

	newArr := make([]any, 0, len(result.parentArr)+len(newBsonWidgets))
	newArr = append(newArr, result.parentArr[:insertIdx]...)
	newArr = append(newArr, newBsonWidgets...)
	newArr = append(newArr, result.parentArr[insertIdx:]...)

	dSetArray(result.parentDoc, result.parentKey, newArr)
	return nil
}

func (m *mprPageMutator) DropWidget(refs []backend.WidgetRef) error {
	for _, ref := range refs {
		// Re-find widget each iteration because previous drops mutate the tree.
		var result *bsonWidgetResult
		if ref.IsColumn() {
			result = findBsonColumn(m.rawData, ref.Widget, ref.Column, m.widgetFinder)
		} else {
			result = m.widgetFinder(m.rawData, ref.Widget)
		}
		if result == nil {
			return fmt.Errorf("widget %q not found", ref.Name())
		}
		newArr := make([]any, 0, len(result.parentArr)-1)
		newArr = append(newArr, result.parentArr[:result.index]...)
		newArr = append(newArr, result.parentArr[result.index+1:]...)
		dSetArray(result.parentDoc, result.parentKey, newArr)
	}
	return nil
}

func (m *mprPageMutator) ReplaceWidget(widgetRef string, columnRef string, widgets []pages.Widget) error {
	var result *bsonWidgetResult
	if columnRef != "" {
		result = findBsonColumn(m.rawData, widgetRef, columnRef, m.widgetFinder)
	} else {
		result = m.widgetFinder(m.rawData, widgetRef)
	}
	if result == nil {
		if columnRef != "" {
			return fmt.Errorf("column %q on widget %q not found", columnRef, widgetRef)
		}
		return fmt.Errorf("widget %q not found", widgetRef)
	}

	newBsonWidgets, err := serializeWidgets(widgets)
	if err != nil {
		return fmt.Errorf("serialize widgets: %w", err)
	}

	newArr := make([]any, 0, len(result.parentArr)-1+len(newBsonWidgets))
	newArr = append(newArr, result.parentArr[:result.index]...)
	newArr = append(newArr, newBsonWidgets...)
	newArr = append(newArr, result.parentArr[result.index+1:]...)

	dSetArray(result.parentDoc, result.parentKey, newArr)
	return nil
}

// InsertColumns inserts new DataGrid2 columns before/after an existing column.
// Columns are serialized as CustomWidgets$WidgetObject (not as form widgets).
func (m *mprPageMutator) InsertColumns(gridRef, afterColumnRef string, position backend.InsertPosition, columns []*backend.DataGridColumnSpec) error {
	if afterColumnRef == "" {
		return fmt.Errorf("InsertColumns requires a column reference")
	}
	result := findBsonColumn(m.rawData, gridRef, afterColumnRef, m.widgetFinder)
	if result == nil {
		return fmt.Errorf("column %q on widget %q not found", afterColumnRef, gridRef)
	}
	gridResult := m.widgetFinder(m.rawData, gridRef)
	if gridResult == nil {
		return fmt.Errorf("widget %q not found", gridRef)
	}
	columnsTypePointerID := findColumnsPropertyTypePointer(gridResult.widget)
	if columnsTypePointerID == "" {
		return fmt.Errorf("widget %q is not a DataGrid2 (no columns property)", gridRef)
	}
	columnObjectTypeID, columnPropertyIDs := extractColumnPropertyIDs(gridResult.widget, columnsTypePointerID)
	if columnObjectTypeID == "" {
		return fmt.Errorf("could not extract column type schema from %q", gridRef)
	}
	var newBsonColumns []any
	for _, col := range columns {
		colBson := m.backend.buildDataGrid2ColumnObject(col, columnObjectTypeID, columnPropertyIDs)
		newBsonColumns = append(newBsonColumns, colBson)
	}
	insertIdx := result.index
	if strings.EqualFold(string(position), "after") {
		insertIdx = result.index + 1
	}
	newArr := make([]any, 0, len(result.parentArr)+len(newBsonColumns))
	newArr = append(newArr, result.parentArr[:insertIdx]...)
	newArr = append(newArr, newBsonColumns...)
	newArr = append(newArr, result.parentArr[insertIdx:]...)
	dSetArray(result.parentDoc, result.parentKey, newArr)
	return nil
}

// ReplaceColumn replaces a single DataGrid2 column with new columns.
// Columns are serialized as CustomWidgets$WidgetObject (not as form widgets).
func (m *mprPageMutator) ReplaceColumn(gridRef, columnRef string, columns []*backend.DataGridColumnSpec) error {
	if columnRef == "" {
		return fmt.Errorf("ReplaceColumn requires a column reference")
	}
	result := findBsonColumn(m.rawData, gridRef, columnRef, m.widgetFinder)
	if result == nil {
		return fmt.Errorf("column %q on widget %q not found", columnRef, gridRef)
	}
	gridResult := m.widgetFinder(m.rawData, gridRef)
	if gridResult == nil {
		return fmt.Errorf("widget %q not found", gridRef)
	}
	columnsTypePointerID := findColumnsPropertyTypePointer(gridResult.widget)
	if columnsTypePointerID == "" {
		return fmt.Errorf("widget %q is not a DataGrid2 (no columns property)", gridRef)
	}
	columnObjectTypeID, columnPropertyIDs := extractColumnPropertyIDs(gridResult.widget, columnsTypePointerID)
	if columnObjectTypeID == "" {
		return fmt.Errorf("could not extract column type schema from %q", gridRef)
	}
	var newBsonColumns []any
	for _, col := range columns {
		colBson := m.backend.buildDataGrid2ColumnObject(col, columnObjectTypeID, columnPropertyIDs)
		newBsonColumns = append(newBsonColumns, colBson)
	}
	newArr := make([]any, 0, len(result.parentArr)-1+len(newBsonColumns))
	newArr = append(newArr, result.parentArr[:result.index]...)
	newArr = append(newArr, newBsonColumns...)
	newArr = append(newArr, result.parentArr[result.index+1:]...)
	dSetArray(result.parentDoc, result.parentKey, newArr)
	return nil
}

// findColumnsPropertyTypePointer locates the "columns" property's $ID in the
// widget's Type.ObjectType.PropertyTypes array. Returns "" if not found.
func findColumnsPropertyTypePointer(widgetDoc bson.D) string {
	widgetType := dGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return ""
	}
	objType := dGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return ""
	}
	for _, pt := range dGetArrayElements(dGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		if dGetString(ptDoc, "PropertyKey") == "columns" {
			return extractBinaryIDFromDoc(dGet(ptDoc, "$ID"))
		}
	}
	return ""
}

// extractColumnPropertyIDs walks an existing CustomWidget's Type tree and
// builds the pages.PropertyTypeIDEntry map for the column object type.
// Returns the columnObjectTypeID and the per-column-property map (forward
// direction; the reverse of buildColumnPropKeyMap).
func extractColumnPropertyIDs(widgetDoc bson.D, columnsTypePointerID string) (objectTypeID string, propIDs map[string]pages.PropertyTypeIDEntry) {
	propIDs = make(map[string]pages.PropertyTypeIDEntry)
	widgetType := dGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return
	}
	objType := dGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return
	}
	for _, pt := range dGetArrayElements(dGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok || extractBinaryIDFromDoc(dGet(ptDoc, "$ID")) != columnsTypePointerID {
			continue
		}
		valType := dGetDoc(ptDoc, "ValueType")
		if valType == nil {
			return
		}
		colObjType := dGetDoc(valType, "ObjectType")
		if colObjType == nil {
			return
		}
		objectTypeID = extractBinaryIDFromDoc(dGet(colObjType, "$ID"))
		for _, cpt := range dGetArrayElements(dGet(colObjType, "PropertyTypes")) {
			cptDoc, ok := cpt.(bson.D)
			if !ok {
				continue
			}
			key := dGetString(cptDoc, "PropertyKey")
			cid := extractBinaryIDFromDoc(dGet(cptDoc, "$ID"))
			cvt := dGetDoc(cptDoc, "ValueType")
			var vid, vtype, defVal string
			if cvt != nil {
				vid = extractBinaryIDFromDoc(dGet(cvt, "$ID"))
				// The discriminator is the inner "Type" field on the
				// CustomWidgets$WidgetValueType document, e.g. "Expression",
				// "TextTemplate", "Widgets", "Enumeration", "Boolean".
				vtype = dGetString(cvt, "Type")
				defVal = dGetString(cvt, "DefaultValue")
			}
			if key == "" || cid == "" {
				continue
			}
			propIDs[key] = pages.PropertyTypeIDEntry{
				PropertyTypeID: cid,
				ValueTypeID:    vid,
				ValueType:      vtype,
				DefaultValue:   defVal,
			}
		}
		return
	}
	return
}

func (m *mprPageMutator) AddVariable(name, dataType, defaultValue string) error {
	// Check for duplicate variable name
	existingVars := dGetArrayElements(dGet(m.rawData, "Variables"))
	for _, ev := range existingVars {
		if evDoc, ok := ev.(bson.D); ok {
			if dGetString(evDoc, "Name") == name {
				return fmt.Errorf("variable $%s already exists", name)
			}
		}
	}

	varTypeID := types.GenerateID()
	bsonTypeName := mdlTypeToBsonType(dataType)
	varType := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(varTypeID)},
		{Key: "$Type", Value: bsonTypeName},
	}
	if bsonTypeName == "DataTypes$ObjectType" {
		varType = append(varType, bson.E{Key: "Entity", Value: dataType})
	}

	varID := types.GenerateID()
	varDoc := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(varID)},
		{Key: "$Type", Value: "Forms$LocalVariable"},
		{Key: "DefaultValue", Value: defaultValue},
		{Key: "Name", Value: name},
		{Key: "VariableType", Value: varType},
	}

	existing := toBsonA(dGet(m.rawData, "Variables"))
	if existing != nil {
		elements := dGetArrayElements(dGet(m.rawData, "Variables"))
		elements = append(elements, varDoc)
		dSetArray(m.rawData, "Variables", elements)
	} else {
		m.rawData = append(m.rawData, bson.E{Key: "Variables", Value: bson.A{int32(3), varDoc}})
	}
	return nil
}

func (m *mprPageMutator) DropVariable(name string) error {
	elements := dGetArrayElements(dGet(m.rawData, "Variables"))
	if elements == nil {
		return fmt.Errorf("variable $%s not found", name)
	}

	found := false
	var kept []any
	for _, elem := range elements {
		if doc, ok := elem.(bson.D); ok {
			if dGetString(doc, "Name") == name {
				found = true
				continue
			}
		}
		kept = append(kept, elem)
	}
	if !found {
		return fmt.Errorf("variable $%s not found", name)
	}
	dSetArray(m.rawData, "Variables", kept)
	return nil
}

func (m *mprPageMutator) SetLayout(newLayout string, paramMappings map[string]string) error {
	if m.containerType == backend.ContainerSnippet {
		return fmt.Errorf("set Layout is not supported for snippets")
	}

	formCall := dGetDoc(m.rawData, "FormCall")
	if formCall == nil {
		return fmt.Errorf("page has no FormCall (layout reference)")
	}

	// Detect old layout name
	oldLayoutQN := ""
	for _, elem := range formCall {
		if elem.Key == "Form" {
			if s, ok := elem.Value.(string); ok && s != "" {
				oldLayoutQN = s
			}
		}
		if elem.Key == "Arguments" {
			if arr, ok := elem.Value.(bson.A); ok {
				for _, item := range arr {
					if doc, ok := item.(bson.D); ok {
						for _, field := range doc {
							if field.Key == "Parameter" {
								if s, ok := field.Value.(string); ok && oldLayoutQN == "" {
									if lastDot := strings.LastIndex(s, "."); lastDot > 0 {
										oldLayoutQN = s[:lastDot]
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if oldLayoutQN == "" {
		return fmt.Errorf("cannot determine current layout from FormCall")
	}
	if oldLayoutQN == newLayout {
		return nil
	}

	// Update Form field
	for i, elem := range formCall {
		if elem.Key == "Form" {
			formCall[i].Value = newLayout
		}
	}

	// Remap Parameter strings
	for _, elem := range formCall {
		if elem.Key != "Arguments" {
			continue
		}
		arr, ok := elem.Value.(bson.A)
		if !ok {
			continue
		}
		for _, item := range arr {
			doc, ok := item.(bson.D)
			if !ok {
				continue
			}
			for j, field := range doc {
				if field.Key != "Parameter" {
					continue
				}
				paramStr, ok := field.Value.(string)
				if !ok {
					continue
				}
				placeholder := paramStr
				if strings.HasPrefix(paramStr, oldLayoutQN+".") {
					placeholder = paramStr[len(oldLayoutQN)+1:]
				}
				if paramMappings != nil {
					if mapped, ok := paramMappings[placeholder]; ok {
						placeholder = mapped
					}
				}
				doc[j].Value = newLayout + "." + placeholder
			}
		}
	}

	// Write FormCall back
	for i, elem := range m.rawData {
		if elem.Key == "FormCall" {
			m.rawData[i].Value = formCall
			break
		}
	}
	return nil
}

func (m *mprPageMutator) SetPluggableProperty(widgetRef string, propKey string, opName backend.PluggablePropertyOp, ctx backend.PluggablePropertyContext) error {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return fmt.Errorf("widget %q not found", widgetRef)
	}

	obj := dGetDoc(result.widget, "Object")
	if obj == nil {
		return fmt.Errorf("widget %q has no pluggable Object", widgetRef)
	}

	propTypeKeyMap := buildPropKeyMap(result.widget)

	props := dGetArrayElements(dGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		resolvedKey := propTypeKeyMap[typePointerID]
		if resolvedKey != propKey {
			continue
		}
		valDoc := dGetDoc(propDoc, "Value")
		if valDoc == nil {
			return fmt.Errorf("property %q has no Value", propKey)
		}

		switch opName {
		case "primitive":
			dSet(valDoc, "PrimitiveValue", ctx.PrimitiveVal)
		case "attribute":
			if attrDoc := dGetDoc(valDoc, "AttributeRef"); attrDoc != nil {
				dSet(attrDoc, "Attribute", ctx.AttributePath)
			} else {
				dSet(valDoc, "AttributeRef", bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "DomainModels$AttributeRef"},
					{Key: "Attribute", Value: ctx.AttributePath},
					{Key: "EntityRef", Value: nil},
				})
			}
		case "association":
			dSet(valDoc, "AssociationRef", ctx.AssocPath)
			if ctx.EntityName != "" {
				dSet(valDoc, "EntityRef", bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
					{Key: "Entity", Value: ctx.EntityName},
				})
			}
		case "datasource":
			serialized := mpr.SerializeCustomWidgetDataSource(ctx.DataSource)
			dSet(valDoc, "DataSource", serialized)
		case "widgets":
			serialized, err := serializeWidgets(ctx.ChildWidgets)
			if err != nil {
				return fmt.Errorf("serialize child widgets: %w", err)
			}
			var bsonArr bson.A
			bsonArr = append(bsonArr, int32(2))
			for _, w := range serialized {
				bsonArr = append(bsonArr, w)
			}
			dSet(valDoc, "Widgets", bsonArr)
		case "texttemplate":
			if tmpl := dGetDoc(valDoc, "TextTemplate"); tmpl != nil {
				items := dGetArrayElements(dGet(tmpl, "Items"))
				if len(items) > 0 {
					if itemDoc, ok := items[0].(bson.D); ok {
						dSet(itemDoc, "Text", ctx.TextTemplate)
					}
				}
			}
		case "action":
			serialized := mpr.SerializeClientAction(ctx.Action)
			dSet(valDoc, "Action", serialized)
		case "selection":
			dSet(valDoc, "PrimitiveValue", ctx.Selection)
		case "attributeObjects":
			// Set multiple attribute paths on sub-objects
			objects := dGetArrayElements(dGet(valDoc, "Objects"))
			for i, attrPath := range ctx.AttributePaths {
				if i >= len(objects) {
					break
				}
				if objDoc, ok := objects[i].(bson.D); ok {
					objProps := dGetArrayElements(dGet(objDoc, "Properties"))
					for _, op := range objProps {
						opDoc, ok := op.(bson.D)
						if !ok {
							continue
						}
						if opVal := dGetDoc(opDoc, "Value"); opVal != nil {
							if attrRef := dGetDoc(opVal, "AttributeRef"); attrRef != nil {
								dSet(attrRef, "Attribute", attrPath)
							}
						}
					}
				}
			}
		default:
			return fmt.Errorf("unsupported pluggable property operation: %s", opName)
		}
		return nil
	}
	return fmt.Errorf("pluggable property %q not found on widget %q", propKey, widgetRef)
}

func (m *mprPageMutator) EnclosingEntity(widgetRef string) string {
	return findEnclosingEntityContext(m.rawData, widgetRef)
}

// EnclosingEntityForChildren returns the entity context that applies to
// children of the named widget. For widgets with their own data source
// (DataView, DataGrid, ListView, DataGrid2), this is the data source entity.
// Used for ALTER PAGE column inserts/replaces, where new columns inherit the
// grid's data source as their entity context.
func (m *mprPageMutator) EnclosingEntityForChildren(widgetRef string) string {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return ""
	}
	if ent := extractEntityFromDataSource(result.widget); ent != "" {
		return ent
	}
	if ent := extractPluggableDataSourceEntity(result.widget); ent != "" {
		return ent
	}
	return findEnclosingEntityContext(m.rawData, widgetRef)
}

// extractPluggableDataSourceEntity walks a CustomWidget's Object.Properties[]
// looking for a "datasource" property and returns the EntityRef.Entity if any.
func extractPluggableDataSourceEntity(widgetDoc bson.D) string {
	obj := dGetDoc(widgetDoc, "Object")
	if obj == nil {
		return ""
	}
	propKeyMap := buildPropKeyMap(widgetDoc)
	if len(propKeyMap) == 0 {
		return ""
	}
	for _, prop := range dGetArrayElements(dGet(obj, "Properties")) {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		if propKeyMap[typePointerID] != "datasource" {
			continue
		}
		valDoc := dGetDoc(propDoc, "Value")
		if valDoc == nil {
			continue
		}
		dsDoc := dGetDoc(valDoc, "DataSource")
		if dsDoc == nil {
			continue
		}
		if entityRef := dGetDoc(dsDoc, "EntityRef"); entityRef != nil {
			if entity := dGetString(entityRef, "Entity"); entity != "" {
				return entity
			}
		}
	}
	return ""
}

func (m *mprPageMutator) WidgetScope() map[string]model.ID {
	return extractWidgetScopeFromBSON(m.rawData)
}

func (m *mprPageMutator) ParamScope() (map[string]model.ID, map[string]string) {
	return extractPageParamsFromBSON(m.rawData)
}

func (m *mprPageMutator) FindWidget(name string) bool {
	return m.widgetFinder(m.rawData, name) != nil
}

func (m *mprPageMutator) Save() error {
	outBytes, err := bson.Marshal(m.rawData)
	if err != nil {
		return fmt.Errorf("marshal modified %s: %w", m.containerType, err)
	}
	return m.backend.writer.UpdateRawUnit(string(m.unitID), outBytes)
}

// ---------------------------------------------------------------------------
// BSON helpers (moved from executor/cmd_alter_page.go)
// ---------------------------------------------------------------------------

// dGet returns the value for a key in a bson.D, or nil if not found.
func dGet(doc bson.D, key string) any {
	for _, elem := range doc {
		if elem.Key == key {
			return elem.Value
		}
	}
	return nil
}

// dGetDoc returns a nested bson.D field value, or nil.
func dGetDoc(doc bson.D, key string) bson.D {
	v := dGet(doc, key)
	if d, ok := v.(bson.D); ok {
		return d
	}
	return nil
}

// dGetString returns a string field value, or "".
func dGetString(doc bson.D, key string) string {
	v := dGet(doc, key)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// dSet sets a field value in a bson.D in place. Returns true if found.
// NOTE: callers generally do not check the return value because the keys
// are structurally guaranteed by the widgetFinder traversal. If a key
// is absent, the mutation is silently skipped — this is intentional for
// optional fields (e.g. Appearance, DataSource) that may not be present
// on every widget type.
func dSet(doc bson.D, key string, value any) bool {
	for i := range doc {
		if doc[i].Key == key {
			doc[i].Value = value
			return true
		}
	}
	return false
}

// dGetArrayElements extracts Mendix array elements from a bson.D field value.
// Strips the int32 type marker at index 0.
func dGetArrayElements(val any) []any {
	arr := toBsonA(val)
	if len(arr) == 0 {
		return nil
	}
	if _, ok := arr[0].(int32); ok {
		return arr[1:]
	}
	if _, ok := arr[0].(int); ok {
		return arr[1:]
	}
	return arr
}

// toBsonA converts various BSON array types to []any.
func toBsonA(v any) []any {
	switch arr := v.(type) {
	case bson.A:
		return []any(arr)
	case []any:
		return arr
	default:
		return nil
	}
}

// dSetArray sets a Mendix-style BSON array field, preserving the int32 marker.
func dSetArray(doc bson.D, key string, elements []any) {
	existing := toBsonA(dGet(doc, key))
	var marker any
	if len(existing) > 0 {
		if _, ok := existing[0].(int32); ok {
			marker = existing[0]
		} else if _, ok := existing[0].(int); ok {
			marker = existing[0]
		}
	}
	var result bson.A
	if marker != nil {
		result = make(bson.A, 0, len(elements)+1)
		result = append(result, marker)
		result = append(result, elements...)
	} else {
		result = make(bson.A, len(elements))
		copy(result, elements)
	}
	dSet(doc, key, result)
}

// extractBinaryIDFromDoc extracts a binary ID string from a bson.D field.
func extractBinaryIDFromDoc(val any) string {
	switch bin := val.(type) {
	case primitive.Binary:
		return types.BlobToUUID(bin.Data)
	case []byte:
		return types.BlobToUUID(bin)
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// BSON widget tree walking
// ---------------------------------------------------------------------------

// bsonWidgetResult holds a found widget and its parent context.
type bsonWidgetResult struct {
	widget      bson.D
	parentArr   []any
	parentKey   string
	parentDoc   bson.D
	index       int
	colPropKeys map[string]string
}

// widgetFinder is a function type for locating widgets in a raw BSON tree.
type widgetFinder func(rawData bson.D, widgetName string) *bsonWidgetResult

// findBsonWidget searches the raw BSON page tree for a widget by name.
func findBsonWidget(rawData bson.D, widgetName string) *bsonWidgetResult {
	formCall := dGetDoc(rawData, "FormCall")
	if formCall == nil {
		return nil
	}
	args := dGetArrayElements(dGet(formCall, "Arguments"))
	for _, arg := range args {
		argDoc, ok := arg.(bson.D)
		if !ok {
			continue
		}
		if result := findInWidgetArray(argDoc, "Widgets", widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findBsonWidgetInSnippet searches the raw BSON snippet tree for a widget by name.
func findBsonWidgetInSnippet(rawData bson.D, widgetName string) *bsonWidgetResult {
	if result := findInWidgetArray(rawData, "Widgets", widgetName); result != nil {
		return result
	}
	if widgetContainer := dGetDoc(rawData, "Widget"); widgetContainer != nil {
		if result := findInWidgetArray(widgetContainer, "Widgets", widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findInWidgetArray searches a widget array for a named widget.
func findInWidgetArray(parentDoc bson.D, key string, widgetName string) *bsonWidgetResult {
	elements := dGetArrayElements(dGet(parentDoc, key))
	for i, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if dGetString(wDoc, "Name") == widgetName {
			return &bsonWidgetResult{
				widget:    wDoc,
				parentArr: elements,
				parentKey: key,
				parentDoc: parentDoc,
				index:     i,
			}
		}
		if result := findInWidgetChildren(wDoc, widgetName); result != nil {
			return result
		}
	}
	return nil
}

// findInWidgetChildren recursively searches widget children for a named widget.
func findInWidgetChildren(wDoc bson.D, widgetName string) *bsonWidgetResult {
	typeName := dGetString(wDoc, "$Type")

	if result := findInWidgetArray(wDoc, "Widgets", widgetName); result != nil {
		return result
	}
	if result := findInWidgetArray(wDoc, "FooterWidgets", widgetName); result != nil {
		return result
	}

	// LayoutGrid: Rows[].Columns[].Widgets[]
	if strings.Contains(typeName, "LayoutGrid") {
		rows := dGetArrayElements(dGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := dGetArrayElements(dGet(rowDoc, "Columns"))
			for _, col := range cols {
				colDoc, ok := col.(bson.D)
				if !ok {
					continue
				}
				if result := findInWidgetArray(colDoc, "Widgets", widgetName); result != nil {
					return result
				}
			}
		}
	}

	// TabContainer: TabPages[].Widgets[]
	tabPages := dGetArrayElements(dGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		if result := findInWidgetArray(tpDoc, "Widgets", widgetName); result != nil {
			return result
		}
	}

	// ControlBar
	if controlBar := dGetDoc(wDoc, "ControlBar"); controlBar != nil {
		if result := findInWidgetArray(controlBar, "Items", widgetName); result != nil {
			return result
		}
	}

	// CustomWidget (pluggable): Object.Properties[].Value.Widgets[]
	if strings.Contains(typeName, "CustomWidget") {
		if obj := dGetDoc(wDoc, "Object"); obj != nil {
			props := dGetArrayElements(dGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
					if result := findInWidgetArray(valDoc, "Widgets", widgetName); result != nil {
						return result
					}
				}
			}
			// DataGrid2: search columns by derived name (stored in Objects, not Widgets)
			propKeyMap := buildPropKeyMap(wDoc)
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
				if propKeyMap[typePointerID] != "columns" {
					continue
				}
				valDoc := dGetDoc(propDoc, "Value")
				if valDoc == nil {
					break
				}
				colPropKeyMap := buildColumnPropKeyMap(wDoc, typePointerID)
				columns := dGetArrayElements(dGet(valDoc, "Objects"))
				for i, colItem := range columns {
					colDoc, ok := colItem.(bson.D)
					if !ok {
						continue
					}
					if deriveColumnNameBson(colDoc, colPropKeyMap, i) == widgetName {
						return &bsonWidgetResult{
							widget:      colDoc,
							parentArr:   columns,
							parentKey:   "Objects",
							parentDoc:   valDoc,
							index:       i,
							colPropKeys: colPropKeyMap,
						}
					}
				}
				break // only one "columns" property per widget
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// DataGrid2 column finder
// ---------------------------------------------------------------------------

// findBsonColumn finds a column inside a DataGrid2 widget by derived name.
func findBsonColumn(rawData bson.D, gridName, columnName string, find widgetFinder) *bsonWidgetResult {
	gridResult := find(rawData, gridName)
	if gridResult == nil {
		return nil
	}

	gridPropKeyMap := buildPropKeyMap(gridResult.widget)

	obj := dGetDoc(gridResult.widget, "Object")
	if obj == nil {
		return nil
	}

	props := dGetArrayElements(dGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		propKey := gridPropKeyMap[typePointerID]
		if propKey != "columns" {
			continue
		}

		valDoc := dGetDoc(propDoc, "Value")
		if valDoc == nil {
			return nil
		}

		colPropKeyMap := buildColumnPropKeyMap(gridResult.widget, typePointerID)

		columns := dGetArrayElements(dGet(valDoc, "Objects"))
		for i, colItem := range columns {
			colDoc, ok := colItem.(bson.D)
			if !ok {
				continue
			}
			derived := deriveColumnNameBson(colDoc, colPropKeyMap, i)
			if derived == columnName {
				return &bsonWidgetResult{
					widget:      colDoc,
					parentArr:   columns,
					parentKey:   "Objects",
					parentDoc:   valDoc,
					index:       i,
					colPropKeys: colPropKeyMap,
				}
			}
		}
		return nil
	}
	return nil
}

// buildPropKeyMap builds a TypePointer ID -> PropertyKey map.
func buildPropKeyMap(widgetDoc bson.D) map[string]string {
	m := make(map[string]string)
	widgetType := dGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return m
	}
	objType := dGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return m
	}
	for _, pt := range dGetArrayElements(dGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		key := dGetString(ptDoc, "PropertyKey")
		id := extractBinaryIDFromDoc(dGet(ptDoc, "$ID"))
		if key != "" && id != "" {
			m[id] = key
		}
	}
	return m
}

// buildColumnPropKeyMap builds a TypePointer ID -> PropertyKey map for column properties.
func buildColumnPropKeyMap(widgetDoc bson.D, columnsTypePointerID string) map[string]string {
	m := make(map[string]string)
	widgetType := dGetDoc(widgetDoc, "Type")
	if widgetType == nil {
		return m
	}
	objType := dGetDoc(widgetType, "ObjectType")
	if objType == nil {
		return m
	}
	for _, pt := range dGetArrayElements(dGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		id := extractBinaryIDFromDoc(dGet(ptDoc, "$ID"))
		if id != columnsTypePointerID {
			continue
		}
		valType := dGetDoc(ptDoc, "ValueType")
		if valType == nil {
			return m
		}
		colObjType := dGetDoc(valType, "ObjectType")
		if colObjType == nil {
			return m
		}
		for _, cpt := range dGetArrayElements(dGet(colObjType, "PropertyTypes")) {
			cptDoc, ok := cpt.(bson.D)
			if !ok {
				continue
			}
			key := dGetString(cptDoc, "PropertyKey")
			cid := extractBinaryIDFromDoc(dGet(cptDoc, "$ID"))
			if key != "" && cid != "" {
				m[cid] = key
			}
		}
		return m
	}
	return m
}

// deriveColumnNameBson derives a column name from its BSON WidgetObject.
func deriveColumnNameBson(colDoc bson.D, propKeyMap map[string]string, index int) string {
	var attribute, caption string

	props := dGetArrayElements(dGet(colDoc, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		propKey := propKeyMap[typePointerID]

		valDoc := dGetDoc(propDoc, "Value")
		if valDoc == nil {
			continue
		}

		switch propKey {
		case "attribute":
			if attrRef := dGetString(valDoc, "AttributeRef"); attrRef != "" {
				attribute = attrRef
			} else if attrDoc := dGetDoc(valDoc, "AttributeRef"); attrDoc != nil {
				attribute = dGetString(attrDoc, "Attribute")
			}
		case "header":
			// TextTemplate → Template (Forms$Text) → Items[] → Translation{Text}.
			// Must traverse the intermediate Template document — same path as
			// deriveColumnName on the DESCRIBE side.
			if tmpl := dGetDoc(valDoc, "TextTemplate"); tmpl != nil {
				if template := dGetDoc(tmpl, "Template"); template != nil {
					items := dGetArrayElements(dGet(template, "Items"))
					for _, item := range items {
						if itemDoc, ok := item.(bson.D); ok {
							if text := dGetString(itemDoc, "Text"); text != "" {
								caption = text
							}
						}
					}
				}
			}
		}
	}

	if attribute != "" {
		parts := strings.Split(attribute, ".")
		return parts[len(parts)-1]
	}
	if caption != "" {
		if name := sanitizeColumnName(caption); name != "" {
			return name
		}
	}
	return fmt.Sprintf("col%d", index+1)
}

// sanitizeColumnName converts a caption string into a valid column identifier,
// matching deriveColumnName() in cmd_pages_describe_output.go exactly.
// Returns "" when the result would be all underscores so the caller falls
// through to the col{N} index fallback.
func sanitizeColumnName(caption string) string {
	sanitized := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, caption)
	return strings.TrimFunc(sanitized, func(r rune) bool { return r == '_' })
}

// ---------------------------------------------------------------------------
// Entity context extraction
// ---------------------------------------------------------------------------

// findEnclosingEntityContext walks the raw BSON tree to find the entity context.
func findEnclosingEntityContext(rawData bson.D, widgetName string) string {
	if formCall := dGetDoc(rawData, "FormCall"); formCall != nil {
		args := dGetArrayElements(dGet(formCall, "Arguments"))
		for _, arg := range args {
			argDoc, ok := arg.(bson.D)
			if !ok {
				continue
			}
			if ctx := findEntityContextInWidgets(argDoc, "Widgets", widgetName, ""); ctx != "" {
				return ctx
			}
		}
	}
	if ctx := findEntityContextInWidgets(rawData, "Widgets", widgetName, ""); ctx != "" {
		return ctx
	}
	if widgetContainer := dGetDoc(rawData, "Widget"); widgetContainer != nil {
		if ctx := findEntityContextInWidgets(widgetContainer, "Widgets", widgetName, ""); ctx != "" {
			return ctx
		}
	}
	return ""
}

func findEntityContextInWidgets(parentDoc bson.D, key string, widgetName string, currentEntity string) string {
	elements := dGetArrayElements(dGet(parentDoc, key))
	for _, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		if dGetString(wDoc, "Name") == widgetName {
			return currentEntity
		}
		entityCtx := currentEntity
		if ent := extractEntityFromDataSource(wDoc); ent != "" {
			entityCtx = ent
		}
		if ctx := findEntityContextInChildren(wDoc, widgetName, entityCtx); ctx != "" {
			return ctx
		}
	}
	return ""
}

func findEntityContextInChildren(wDoc bson.D, widgetName string, currentEntity string) string {
	typeName := dGetString(wDoc, "$Type")

	if ctx := findEntityContextInWidgets(wDoc, "Widgets", widgetName, currentEntity); ctx != "" {
		return ctx
	}
	if ctx := findEntityContextInWidgets(wDoc, "FooterWidgets", widgetName, currentEntity); ctx != "" {
		return ctx
	}
	if strings.Contains(typeName, "LayoutGrid") {
		rows := dGetArrayElements(dGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := dGetArrayElements(dGet(rowDoc, "Columns"))
			for _, col := range cols {
				colDoc, ok := col.(bson.D)
				if !ok {
					continue
				}
				if ctx := findEntityContextInWidgets(colDoc, "Widgets", widgetName, currentEntity); ctx != "" {
					return ctx
				}
			}
		}
	}
	tabPages := dGetArrayElements(dGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		if ctx := findEntityContextInWidgets(tpDoc, "Widgets", widgetName, currentEntity); ctx != "" {
			return ctx
		}
	}
	if controlBar := dGetDoc(wDoc, "ControlBar"); controlBar != nil {
		if ctx := findEntityContextInWidgets(controlBar, "Items", widgetName, currentEntity); ctx != "" {
			return ctx
		}
	}
	if strings.Contains(typeName, "CustomWidget") {
		if obj := dGetDoc(wDoc, "Object"); obj != nil {
			props := dGetArrayElements(dGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
					if ctx := findEntityContextInWidgets(valDoc, "Widgets", widgetName, currentEntity); ctx != "" {
						return ctx
					}
				}
			}
		}
	}
	return ""
}

func extractEntityFromDataSource(wDoc bson.D) string {
	ds := dGetDoc(wDoc, "DataSource")
	if ds == nil {
		return ""
	}
	if entityRef := dGetDoc(ds, "EntityRef"); entityRef != nil {
		if entity := dGetString(entityRef, "Entity"); entity != "" {
			return entity
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Widget scope extraction
// ---------------------------------------------------------------------------

func extractWidgetScopeFromBSON(rawData bson.D) map[string]model.ID {
	scope := make(map[string]model.ID)
	if rawData == nil {
		return scope
	}
	if formCall := dGetDoc(rawData, "FormCall"); formCall != nil {
		args := dGetArrayElements(dGet(formCall, "Arguments"))
		for _, arg := range args {
			argDoc, ok := arg.(bson.D)
			if !ok {
				continue
			}
			collectWidgetScope(argDoc, "Widgets", scope)
		}
	}
	collectWidgetScope(rawData, "Widgets", scope)
	if widgetContainer := dGetDoc(rawData, "Widget"); widgetContainer != nil {
		collectWidgetScope(widgetContainer, "Widgets", scope)
	}
	return scope
}

// extractPageParamsFromBSON extracts page/snippet parameter names and entity
// IDs from the raw BSON document.
func extractPageParamsFromBSON(rawData bson.D) (map[string]model.ID, map[string]string) {
	paramScope := make(map[string]model.ID)
	paramEntityNames := make(map[string]string)
	if rawData == nil {
		return paramScope, paramEntityNames
	}

	params := dGetArrayElements(dGet(rawData, "Parameters"))
	for _, p := range params {
		pDoc, ok := p.(bson.D)
		if !ok {
			continue
		}
		name := dGetString(pDoc, "Name")
		if name == "" {
			continue
		}
		paramType := dGetDoc(pDoc, "ParameterType")
		if paramType == nil {
			continue
		}
		typeName := dGetString(paramType, "$Type")
		if typeName != "DataTypes$ObjectType" {
			continue
		}
		entityName := dGetString(paramType, "Entity")
		if entityName == "" {
			continue
		}
		idVal := dGet(pDoc, "$ID")
		paramID := model.ID(extractBinaryIDFromDoc(idVal))
		paramScope[name] = paramID
		paramEntityNames[name] = entityName
	}
	return paramScope, paramEntityNames
}

func collectWidgetScope(parentDoc bson.D, key string, scope map[string]model.ID) {
	elements := dGetArrayElements(dGet(parentDoc, key))
	for _, elem := range elements {
		wDoc, ok := elem.(bson.D)
		if !ok {
			continue
		}
		name := dGetString(wDoc, "Name")
		if name != "" {
			idVal := dGet(wDoc, "$ID")
			if wID := extractBinaryIDFromDoc(idVal); wID != "" {
				scope[name] = model.ID(wID)
			}
		}
		collectWidgetScopeInChildren(wDoc, scope)
	}
}

func collectWidgetScopeInChildren(wDoc bson.D, scope map[string]model.ID) {
	typeName := dGetString(wDoc, "$Type")

	collectWidgetScope(wDoc, "Widgets", scope)
	collectWidgetScope(wDoc, "FooterWidgets", scope)

	if strings.Contains(typeName, "LayoutGrid") {
		rows := dGetArrayElements(dGet(wDoc, "Rows"))
		for _, row := range rows {
			rowDoc, ok := row.(bson.D)
			if !ok {
				continue
			}
			cols := dGetArrayElements(dGet(rowDoc, "Columns"))
			for _, col := range cols {
				colDoc, ok := col.(bson.D)
				if !ok {
					continue
				}
				collectWidgetScope(colDoc, "Widgets", scope)
			}
		}
	}
	tabPages := dGetArrayElements(dGet(wDoc, "TabPages"))
	for _, tp := range tabPages {
		tpDoc, ok := tp.(bson.D)
		if !ok {
			continue
		}
		collectWidgetScope(tpDoc, "Widgets", scope)
	}
	if controlBar := dGetDoc(wDoc, "ControlBar"); controlBar != nil {
		collectWidgetScope(controlBar, "Items", scope)
	}
	if strings.Contains(typeName, "CustomWidget") {
		if obj := dGetDoc(wDoc, "Object"); obj != nil {
			props := dGetArrayElements(dGet(obj, "Properties"))
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
					collectWidgetScope(valDoc, "Widgets", scope)
				}
			}
			// DataGrid2: add column derived names to scope for duplicate-name detection
			propKeyMap := buildPropKeyMap(wDoc)
			for _, prop := range props {
				propDoc, ok := prop.(bson.D)
				if !ok {
					continue
				}
				typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
				if propKeyMap[typePointerID] != "columns" {
					continue
				}
				valDoc := dGetDoc(propDoc, "Value")
				if valDoc == nil {
					break
				}
				colPropKeyMap := buildColumnPropKeyMap(wDoc, typePointerID)
				columns := dGetArrayElements(dGet(valDoc, "Objects"))
				for i, colItem := range columns {
					colDoc, ok := colItem.(bson.D)
					if !ok {
						continue
					}
					derived := deriveColumnNameBson(colDoc, colPropKeyMap, i)
					if derived != "" {
						idVal := dGet(colDoc, "$ID")
						if wID := extractBinaryIDFromDoc(idVal); wID != "" {
							scope[derived] = model.ID(wID)
						}
					}
				}
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Property setting helpers
// ---------------------------------------------------------------------------

// columnPropertyAliases maps user-facing property names to internal column property keys.
var columnPropertyAliases = map[string]string{
	"Caption":       "header",
	"Attribute":     "attribute",
	"Visible":       "visible",
	"Alignment":     "alignment",
	"WrapText":      "wrapText",
	"Sortable":      "sortable",
	"Resizable":     "resizable",
	"Draggable":     "draggable",
	"Hidable":       "hidable",
	"ColumnWidth":   "width",
	"Size":          "size",
	"ShowContentAs": "showContentAs",
	"ColumnClass":   "columnClass",
	"Tooltip":       "tooltip",
}

func setColumnPropertyMut(colDoc bson.D, propKeyMap map[string]string, propName string, value any) error {
	internalKey := columnPropertyAliases[propName]
	if internalKey == "" {
		internalKey = propName
	}

	props := dGetArrayElements(dGet(colDoc, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		propKey := propKeyMap[typePointerID]
		if propKey != internalKey {
			continue
		}
		if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
			strVal := fmt.Sprintf("%v", value)
			dSet(valDoc, "PrimitiveValue", strVal)
			return nil
		}
		return fmt.Errorf("column property %q has no Value", propName)
	}
	return fmt.Errorf("column property %q not found", propName)
}

func applyPageLevelSetMut(rawData bson.D, prop string, value any) error {
	switch prop {
	case "Title":
		if formCall := dGetDoc(rawData, "FormCall"); formCall != nil {
			setTranslatableText(formCall, "Title", value)
		} else {
			setTranslatableText(rawData, "Title", value)
		}
	case "Url":
		strVal, _ := value.(string)
		dSet(rawData, "Url", strVal)
	default:
		return fmt.Errorf("unsupported page-level property: %s", prop)
	}
	return nil
}

func setRawWidgetPropertyMut(widget bson.D, propName string, value any) error {
	switch propName {
	case "Caption":
		return setWidgetCaptionMut(widget, value)
	case "Content":
		return setWidgetContentMut(widget, value)
	case "Label":
		return setWidgetLabelMut(widget, value)
	case "ButtonStyle":
		if s, ok := value.(string); ok {
			dSet(widget, "ButtonStyle", s)
		}
		return nil
	case "Class":
		if appearance := dGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				dSet(appearance, "Class", s)
			}
		}
		return nil
	case "Style":
		if appearance := dGetDoc(widget, "Appearance"); appearance != nil {
			if s, ok := value.(string); ok {
				dSet(appearance, "Style", s)
			}
		}
		return nil
	case "Editable":
		if s, ok := value.(string); ok {
			dSet(widget, "Editable", s)
		}
		return nil
	case "Visible":
		if s, ok := value.(string); ok {
			dSet(widget, "Visible", s)
		} else if b, ok := value.(bool); ok {
			if b {
				dSet(widget, "Visible", "True")
			} else {
				dSet(widget, "Visible", "False")
			}
		}
		return nil
	case "Name":
		if s, ok := value.(string); ok {
			dSet(widget, "Name", s)
		}
		return nil
	case "Attribute":
		return setWidgetAttributeRefMut(widget, value)
	default:
		// Try as pluggable widget property
		return setPluggableWidgetPropertyMut(widget, propName, value)
	}
}

func setWidgetCaptionMut(widget bson.D, value any) error {
	caption := dGetDoc(widget, "Caption")
	if caption == nil {
		return mdlerrors.NewValidation("widget has no Caption property")
	}
	setTranslatableText(caption, "", value)
	return nil
}

func setWidgetContentMut(widget bson.D, value any) error {
	strVal, ok := value.(string)
	if !ok {
		return fmt.Errorf("Content value must be a string")
	}
	content := dGetDoc(widget, "Content")
	if content == nil {
		return fmt.Errorf("widget has no Content property")
	}
	template := dGetDoc(content, "Template")
	if template == nil {
		return fmt.Errorf("Content has no Template")
	}
	items := dGetArrayElements(dGet(template, "Items"))
	if len(items) > 0 {
		if itemDoc, ok := items[0].(bson.D); ok {
			dSet(itemDoc, "Text", strVal)
			return nil
		}
	}
	return fmt.Errorf("Content.Template has no Items with Text")
}

// setWidgetLabelMut sets the widget's Label caption. Returns nil without error
// if the widget has no Label field — not all widget types support labels.
func setWidgetLabelMut(widget bson.D, value any) error {
	label := dGetDoc(widget, "Label")
	if label == nil {
		return nil
	}
	setTranslatableText(label, "Caption", value)
	return nil
}

func setWidgetAttributeRefMut(widget bson.D, value any) error {
	attrPath, ok := value.(string)
	if !ok {
		return fmt.Errorf("Attribute value must be a string")
	}

	var attrRefValue any
	if strings.Count(attrPath, ".") >= 2 {
		attrRefValue = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: attrPath},
			{Key: "EntityRef", Value: nil},
		}
	} else {
		attrRefValue = nil
	}

	for i, elem := range widget {
		if elem.Key == "AttributeRef" {
			widget[i].Value = attrRefValue
			return nil
		}
	}
	return fmt.Errorf("widget does not have an AttributeRef property")
}

func setPluggableWidgetPropertyMut(widget bson.D, propName string, value any) error {
	obj := dGetDoc(widget, "Object")
	if obj == nil {
		return fmt.Errorf("property %q not found (widget has no pluggable Object)", propName)
	}

	propTypeKeyMap := make(map[string]string)
	if widgetType := dGetDoc(widget, "Type"); widgetType != nil {
		if objType := dGetDoc(widgetType, "ObjectType"); objType != nil {
			propTypes := dGetArrayElements(dGet(objType, "PropertyTypes"))
			for _, pt := range propTypes {
				ptDoc, ok := pt.(bson.D)
				if !ok {
					continue
				}
				key := dGetString(ptDoc, "PropertyKey")
				if key == "" {
					continue
				}
				id := extractBinaryIDFromDoc(dGet(ptDoc, "$ID"))
				if id != "" {
					propTypeKeyMap[id] = key
				}
			}
		}
	}

	props := dGetArrayElements(dGet(obj, "Properties"))
	for _, prop := range props {
		propDoc, ok := prop.(bson.D)
		if !ok {
			continue
		}
		typePointerID := extractBinaryIDFromDoc(dGet(propDoc, "TypePointer"))
		propKey := propTypeKeyMap[typePointerID]
		if propKey != propName {
			continue
		}
		if valDoc := dGetDoc(propDoc, "Value"); valDoc != nil {
			switch v := value.(type) {
			case string:
				dSet(valDoc, "PrimitiveValue", v)
			case bool:
				if v {
					dSet(valDoc, "PrimitiveValue", "yes")
				} else {
					dSet(valDoc, "PrimitiveValue", "no")
				}
			case int:
				dSet(valDoc, "PrimitiveValue", fmt.Sprintf("%d", v))
			case float64:
				dSet(valDoc, "PrimitiveValue", fmt.Sprintf("%g", v))
			default:
				dSet(valDoc, "PrimitiveValue", fmt.Sprintf("%v", v))
			}
			return nil
		}
		return fmt.Errorf("property %q has no Value map", propName)
	}
	return fmt.Errorf("pluggable property %q not found", propName)
}

// setTranslatableText sets a translatable text value in BSON.
func setTranslatableText(parent bson.D, key string, value any) {
	strVal, ok := value.(string)
	if !ok {
		return
	}

	target := parent
	if key != "" {
		if nested := dGetDoc(parent, key); nested != nil {
			target = nested
		} else {
			dSet(parent, key, strVal)
			return
		}
	}

	translations := dGetArrayElements(dGet(target, "Translations"))
	if len(translations) > 0 {
		if tDoc, ok := translations[0].(bson.D); ok {
			dSet(tDoc, "Text", strVal)
			return
		}
	}
	dSet(target, "Text", strVal)
}

// ---------------------------------------------------------------------------
// Widget serialization helpers
// ---------------------------------------------------------------------------

func serializeWidgets(widgets []pages.Widget) ([]any, error) {
	var result []any
	for _, w := range widgets {
		bsonDoc := mpr.SerializeWidget(w)
		if bsonDoc == nil {
			continue
		}
		result = append(result, bsonDoc)
	}
	return result, nil
}

// serializeDataSourceBson converts a pages.DataSource to a BSON document for widget-level DataSource fields.
func serializeDataSourceBson(ds pages.DataSource) bson.D {
	switch d := ds.(type) {
	case *pages.ListenToWidgetSource:
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$ListenTargetSource"},
			{Key: "ListenTarget", Value: d.WidgetName},
		}
	case *pages.DatabaseSource:
		var entityRef any
		if d.EntityName != "" {
			entityRef = bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: d.EntityName},
			}
		}
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: entityRef},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SourceVariable", Value: nil},
		}
	case *pages.MicroflowSource:
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$MicroflowSource"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Asynchronous", Value: false},
				{Key: "ConfirmationInfo", Value: nil},
				{Key: "FormValidations", Value: "All"},
				{Key: "Microflow", Value: d.Microflow},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
				{Key: "ProgressBar", Value: "None"},
				{Key: "ProgressMessage", Value: nil},
			}},
		}
	case *pages.NanoflowSource:
		return bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$NanoflowSource"},
			{Key: "NanoflowSettings", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$NanoflowSettings"},
				{Key: "Nanoflow", Value: d.Nanoflow},
				{Key: "ParameterMappings", Value: bson.A{int32(3)}},
			}},
		}
	default:
		return nil
	}
}

// mdlTypeToBsonType converts an MDL type name to a BSON DataTypes$* type string.
func mdlTypeToBsonType(mdlType string) string {
	switch strings.ToLower(mdlType) {
	case "boolean":
		return "DataTypes$BooleanType"
	case "string":
		return "DataTypes$StringType"
	case "integer":
		return "DataTypes$IntegerType"
	case "long":
		return "DataTypes$LongType"
	case "decimal":
		return "DataTypes$DecimalType"
	case "datetime", "date":
		return "DataTypes$DateTimeType"
	default:
		return "DataTypes$ObjectType"
	}
}
