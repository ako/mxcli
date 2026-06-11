// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// modelsdkPageMutator is a minimal ALTER PAGE/SNIPPET mutator for the codec
// engine. It operates on the unit's raw BSON (bson v1, version-agnostic bytes)
// and currently supports the variable + layout operations; widget-tree mutation
// (SET/INSERT/DROP/REPLACE) is deferred to the shared page-mutator extraction.
type modelsdkPageMutator struct {
	b             *Backend
	unitID        model.ID
	rawData       bson.D
	containerType backend.ContainerKind
}

// OpenPageForMutation loads a page/layout/snippet unit and returns a mutator.
func (b *Backend) OpenPageForMutation(unitID model.ID) (backend.PageMutator, error) {
	if b.writer == nil {
		return nil, fmt.Errorf("OpenPageForMutation: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return nil, fmt.Errorf("OpenPageForMutation: load unit: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("OpenPageForMutation: unmarshal: %w", err)
	}
	ct := backend.ContainerPage
	switch ty := mutGetString(d, "$Type"); {
	case strings.Contains(ty, "Snippet"):
		ct = backend.ContainerSnippet
	case strings.Contains(ty, "Layout"):
		ct = backend.ContainerLayout
	}
	return &modelsdkPageMutator{b: b, unitID: unitID, rawData: d, containerType: ct}, nil
}

func (m *modelsdkPageMutator) ContainerType() backend.ContainerKind { return m.containerType }

func (m *modelsdkPageMutator) AddVariable(name, dataType, defaultValue string) error {
	for _, ev := range mutArrayElements(mutGet(m.rawData, "Variables")) {
		if evd, ok := ev.(bson.D); ok && mutGetString(evd, "Name") == name {
			return fmt.Errorf("variable $%s already exists", name)
		}
	}
	bsonTypeName := mutMdlTypeToBsonType(dataType)
	varType := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(mmpr.GenerateID())},
		{Key: "$Type", Value: bsonTypeName},
	}
	if bsonTypeName == "DataTypes$ObjectType" {
		varType = append(varType, bson.E{Key: "Entity", Value: dataType})
	}
	varDoc := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(mmpr.GenerateID())},
		{Key: "$Type", Value: "Forms$LocalVariable"},
		{Key: "DefaultValue", Value: defaultValue},
		{Key: "Name", Value: name},
		{Key: "VariableType", Value: varType},
	}
	elems := append(mutArrayElements(mutGet(m.rawData, "Variables")), varDoc)
	mutSetArray(&m.rawData, "Variables", mutMarker(mutGet(m.rawData, "Variables"), 3), elems)
	return nil
}

func (m *modelsdkPageMutator) DropVariable(name string) error {
	elems := mutArrayElements(mutGet(m.rawData, "Variables"))
	kept := make([]any, 0, len(elems))
	found := false
	for _, e := range elems {
		if d, ok := e.(bson.D); ok && mutGetString(d, "Name") == name {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return fmt.Errorf("variable $%s not found", name)
	}
	mutSetArray(&m.rawData, "Variables", mutMarker(mutGet(m.rawData, "Variables"), 3), kept)
	return nil
}

func (m *modelsdkPageMutator) SetLayout(newLayout string, paramMappings map[string]string) error {
	if m.containerType == backend.ContainerSnippet {
		return fmt.Errorf("SET Layout is not supported for snippets")
	}
	formCall := mutGetDoc(m.rawData, "FormCall")
	if formCall == nil {
		return fmt.Errorf("page has no FormCall (layout reference)")
	}
	oldLayoutQN := ""
	for _, elem := range formCall {
		if elem.Key == "Form" {
			if s, ok := elem.Value.(string); ok && s != "" {
				oldLayoutQN = s
			}
		}
	}
	if oldLayoutQN == "" {
		for _, elem := range formCall {
			if elem.Key != "Arguments" {
				continue
			}
			for _, item := range mutToBsonA(elem.Value) {
				doc, ok := item.(bson.D)
				if !ok {
					continue
				}
				if p := mutGetString(doc, "Parameter"); p != "" {
					if dot := strings.LastIndex(p, "."); dot > 0 && oldLayoutQN == "" {
						oldLayoutQN = p[:dot]
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
	for i, elem := range formCall {
		if elem.Key == "Form" {
			formCall[i].Value = newLayout
		}
	}
	for _, elem := range formCall {
		if elem.Key != "Arguments" {
			continue
		}
		for _, item := range mutToBsonA(elem.Value) {
			doc, ok := item.(bson.D)
			if !ok {
				continue
			}
			for j, field := range doc {
				if field.Key != "Parameter" {
					continue
				}
				p, ok := field.Value.(string)
				if !ok {
					continue
				}
				placeholder := p
				if strings.HasPrefix(p, oldLayoutQN+".") {
					placeholder = p[len(oldLayoutQN)+1:]
				}
				if mapped, ok := paramMappings[placeholder]; ok {
					placeholder = mapped
				}
				doc[j].Value = newLayout + "." + placeholder
			}
		}
	}
	return nil
}

func (m *modelsdkPageMutator) Save() error {
	out, err := bson.Marshal(m.rawData)
	if err != nil {
		return fmt.Errorf("page mutator: marshal: %w", err)
	}
	return m.b.writer.UpdateRawUnit(string(m.unitID), out)
}

// --- not-yet-ported operations (widget-tree mutation) ---

func (m *modelsdkPageMutator) FindWidget(string) bool                     { return false }
func (m *modelsdkPageMutator) EnclosingEntity(string) string              { return "" }
func (m *modelsdkPageMutator) EnclosingEntityForChildren(string) string   { return "" }
func (m *modelsdkPageMutator) WidgetScope() map[string]model.ID           { return nil }
func (m *modelsdkPageMutator) ParamScope() (map[string]model.ID, map[string]string) {
	return nil, nil
}

func errAlterUnsupported(op string) error {
	return fmt.Errorf("modelsdk engine: ALTER PAGE %s not implemented yet — rerun with MXCLI_ENGINE=legacy", op)
}

func (m *modelsdkPageMutator) SetWidgetProperty(string, string, any) error {
	return errAlterUnsupported("SET widget property")
}
func (m *modelsdkPageMutator) SetWidgetDataSource(string, pages.DataSource) error {
	return errAlterUnsupported("SET widget data source")
}
func (m *modelsdkPageMutator) SetColumnProperty(string, string, string, any) error {
	return errAlterUnsupported("SET column property")
}
func (m *modelsdkPageMutator) SetDesignProperty(string, string, string, string) error {
	return errAlterUnsupported("SET design property")
}
func (m *modelsdkPageMutator) RemoveDesignProperty(string, string) error {
	return errAlterUnsupported("REMOVE design property")
}
func (m *modelsdkPageMutator) ClearDesignProperties(string) error {
	return errAlterUnsupported("CLEAR design properties")
}
func (m *modelsdkPageMutator) InsertWidget(string, string, backend.InsertPosition, []pages.Widget) error {
	return errAlterUnsupported("INSERT widget")
}
func (m *modelsdkPageMutator) DropWidget([]backend.WidgetRef) error {
	return errAlterUnsupported("DROP widget")
}
func (m *modelsdkPageMutator) ReplaceWidget(string, string, []pages.Widget) error {
	return errAlterUnsupported("REPLACE widget")
}
func (m *modelsdkPageMutator) InsertColumns(string, string, backend.InsertPosition, []*backend.DataGridColumnSpec) error {
	return errAlterUnsupported("INSERT columns")
}
func (m *modelsdkPageMutator) ReplaceColumn(string, string, []*backend.DataGridColumnSpec) error {
	return errAlterUnsupported("REPLACE column")
}
func (m *modelsdkPageMutator) SetPluggableProperty(string, string, backend.PluggablePropertyOp, backend.PluggablePropertyContext) error {
	return errAlterUnsupported("SET pluggable property")
}

// --- raw-BSON helpers (v1) ---

func mutGet(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func mutGetString(d bson.D, key string) string {
	s, _ := mutGet(d, key).(string)
	return s
}

func mutGetDoc(d bson.D, key string) bson.D {
	x, _ := mutGet(d, key).(bson.D)
	return x
}

func mutToBsonA(v any) []any {
	switch a := v.(type) {
	case bson.A:
		return []any(a)
	case []any:
		return a
	}
	return nil
}

// mutArrayElements strips the leading typed-array marker, returning the items.
func mutArrayElements(v any) []any {
	arr := mutToBsonA(v)
	if len(arr) == 0 {
		return nil
	}
	switch arr[0].(type) {
	case int32, int:
		return arr[1:]
	}
	return arr
}

// mutMarker returns the leading typed-array marker, or def if absent.
func mutMarker(v any, def int32) int32 {
	arr := mutToBsonA(v)
	if len(arr) > 0 {
		if m, ok := arr[0].(int32); ok {
			return m
		}
	}
	return def
}

// mutSetArray sets key to a typed array (marker + elems), in place or appended.
func mutSetArray(d *bson.D, key string, marker int32, elems []any) {
	arr := make(bson.A, 0, len(elems)+1)
	arr = append(arr, marker)
	arr = append(arr, elems...)
	for i := range *d {
		if (*d)[i].Key == key {
			(*d)[i].Value = arr
			return
		}
	}
	*d = append(*d, bson.E{Key: key, Value: arr})
}

func mutMdlTypeToBsonType(mdlType string) string {
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
