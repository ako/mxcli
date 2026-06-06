// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

const microflowDocType = "Microflows$Microflow"

// CreateMicroflow creates a microflow via ped_create_document.
//
// The executor's object graph (Start/End/activities + sequence flows) is mapped
// onto the PED constructor: each object becomes a /objects entry and each
// SequenceFlow a /flows entry referencing objects by $id(/objects/N). Adding a
// new activity is one case in mapMicroflowAction. Object and action types that
// are not mapped yet are rejected with a clear error (the 130+ Microflows$*
// types are an iterative follow-on). See docs/03-development/PED_MCP_CAPABILITIES.md.
func (b *Backend) CreateMicroflow(mf *microflows.Microflow) error {
	mod, err := b.reader.GetModule(mf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for microflow %q: %w", mf.Name, err)
	}

	// Parameters occupy the first object slots (they are canvas objects but take
	// part in no flow); the executor's flow objects follow.
	objects := make([]any, 0, len(mf.Parameters)+2)
	for i, p := range mf.Parameters {
		typeName, entity, enumeration, err := mfDataType(p.Type)
		if err != nil {
			return fmt.Errorf("microflow %q parameter %q: %w", mf.Name, p.Name, err)
		}
		po := map[string]any{
			"$Type":               "Microflows$MicroflowParameterObject",
			"name":                p.Name,
			"type":                typeName,
			"relativeMiddlePoint": canvasPoint(i, 280),
		}
		if entity != "" {
			po["entity"] = entity
		}
		if enumeration != "" {
			po["enumeration"] = enumeration
		}
		objects = append(objects, po)
	}
	paramCount := len(objects)

	// Map the flow objects (Start/End/activities) and remember each object's
	// index so SequenceFlows can reference it by $id(/objects/N).
	idIndex := map[model.ID]int{}
	if mf.ObjectCollection != nil {
		for i, o := range mf.ObjectCollection.Objects {
			pedObj, err := b.mapMicroflowObject(o, paramCount+i)
			if err != nil {
				return fmt.Errorf("microflow %q: %w", mf.Name, err)
			}
			idIndex[o.GetID()] = paramCount + i
			objects = append(objects, pedObj)
		}
	}

	flows := make([]any, 0)
	if mf.ObjectCollection != nil {
		for _, f := range mf.ObjectCollection.Flows {
			if f.CaseValue != nil {
				return fmt.Errorf("microflow %q: conditional flows (splits) are not yet supported by the MCP backend", mf.Name)
			}
			oi, ok1 := idIndex[f.OriginID]
			di, ok2 := idIndex[f.DestinationID]
			if !ok1 || !ok2 {
				return fmt.Errorf("microflow %q: a sequence flow references an object that is not supported yet", mf.Name)
			}
			flows = append(flows, map[string]any{
				"originId":      fmt.Sprintf("$id(/objects/%d)", oi),
				"destinationId": fmt.Sprintf("$id(/objects/%d)", di),
			})
		}
	}

	returnTypeName, rtEntity, rtEnum, err := mfDataType(mf.ReturnType)
	if err != nil {
		return fmt.Errorf("microflow %q return type: %w", mf.Name, err)
	}
	returnType := map[string]any{"type": returnTypeName}
	if rtEntity != "" {
		returnType["entity"] = rtEntity
	}
	if rtEnum != "" {
		returnType["enumeration"] = rtEnum
	}

	content := map[string]any{
		"name":               mf.Name,
		"objects":            objects,
		"flows":              flows,
		"returnType":         returnType,
		"returnVariableName": mf.ReturnVariableName,
	}

	if err := b.ensureSchema(microflowDocType); err != nil {
		return err
	}
	if err := b.pedCreateDocument(mod.Name, microflowDocType, mf.Name, content); err != nil {
		return err
	}
	if mf.ID == "" {
		mf.ID = model.ID("mcp~mf~" + mod.Name + "~" + mf.Name)
	}
	b.sessionMicroflows = append(b.sessionMicroflows, mf)
	return b.pedCheckDocument(microflowDocType, mod.Name+"."+mf.Name)
}

// ListMicroflows returns microflows from the local reader merged with those
// created over MCP this session (session entries take precedence by
// module+name) — for duplicate detection and create-then-reference in one run.
func (b *Backend) ListMicroflows() ([]*microflows.Microflow, error) {
	local, err := b.reader.ListMicroflows()
	if err != nil {
		return nil, err
	}
	if len(b.sessionMicroflows) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionMicroflows))
	out := make([]*microflows.Microflow, 0, len(local)+len(b.sessionMicroflows))
	for _, m := range b.sessionMicroflows {
		seen[mfKey(m)] = true
		out = append(out, m)
	}
	for _, m := range local {
		if !seen[mfKey(m)] {
			out = append(out, m)
		}
	}
	return out, nil
}

// GetMicroflow resolves by ID, preferring session-created microflows.
func (b *Backend) GetMicroflow(id model.ID) (*microflows.Microflow, error) {
	for _, m := range b.sessionMicroflows {
		if m.ID == id {
			return m, nil
		}
	}
	return b.reader.GetMicroflow(id)
}

func mfKey(m *microflows.Microflow) string {
	return string(m.ContainerID) + "." + m.Name
}

// Nanoflow + rule reads delegate to the local reader (read-only); their writes
// remain unsupported via the generated base.
func (b *Backend) ListNanoflows() ([]*microflows.Nanoflow, error) { return b.reader.ListNanoflows() }
func (b *Backend) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	return b.reader.GetNanoflow(id)
}
func (b *Backend) IsRule(qualifiedName string) (bool, error) { return b.reader.IsRule(qualifiedName) }

// canvasPoint lays objects out left-to-right by index. Positions are cosmetic
// (they do not affect validity); a clean linear layout is enough for the slice.
func canvasPoint(index, y int) map[string]int {
	return map[string]int{"x": 120 + index*160, "y": y}
}

// mapMicroflowObject maps one executor microflow object onto a PED /objects
// entry. Unmapped object types are rejected.
func (b *Backend) mapMicroflowObject(o microflows.MicroflowObject, index int) (map[string]any, error) {
	switch obj := o.(type) {
	case *microflows.StartEvent:
		return map[string]any{
			"$Type":               "Microflows$StartEvent",
			"relativeMiddlePoint": canvasPoint(index, 120),
		}, nil
	case *microflows.EndEvent:
		m := map[string]any{
			"$Type":               "Microflows$EndEvent",
			"relativeMiddlePoint": canvasPoint(index, 120),
		}
		if obj.ReturnValue != "" {
			m["returnValue"] = obj.ReturnValue
		}
		return m, nil
	case *microflows.ActionActivity:
		action, err := mapMicroflowAction(obj.Action)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":               "Microflows$ActionActivity",
			"relativeMiddlePoint": canvasPoint(index, 120),
			"action":              action,
		}, nil
	default:
		return nil, fmt.Errorf("microflow object type %T is not yet supported by the MCP backend", o)
	}
}

// mapMicroflowAction maps one microflow action onto its PED action element.
// Add a case here to support a new activity type.
func mapMicroflowAction(a microflows.MicroflowAction) (map[string]any, error) {
	switch act := a.(type) {
	case *microflows.CreateVariableAction:
		varType, err := mfVariableType(act.DataType)
		if err != nil {
			return nil, fmt.Errorf("create variable %q: %w", act.VariableName, err)
		}
		m := map[string]any{
			"$Type":        "Microflows$CreateVariableAction",
			"variableName": act.VariableName,
			"variableType": varType,
		}
		if act.InitialValue != "" {
			m["initialValue"] = act.InitialValue
		}
		return m, nil
	case *microflows.ChangeVariableAction:
		return map[string]any{
			"$Type":              "Microflows$ChangeVariableAction",
			"changeVariableName": act.VariableName,
			"value":              act.Value,
		}, nil
	case *microflows.ShowMessageAction:
		messageType := string(act.Type)
		if messageType == "" {
			messageType = "Information"
		}
		return map[string]any{
			"$Type": "Microflows$ShowMessageAction",
			"type":  messageType,
			// ShowMessage's template is an inline object (no $Type).
			"template": mfStringTemplate("", act.Template, act.TemplateParameters),
		}, nil
	case *microflows.LogMessageAction:
		level := string(act.LogLevel)
		if level == "" {
			level = "Info"
		}
		m := map[string]any{
			"$Type": "Microflows$LogMessageAction",
			"level": level,
			// LogMessage's messageTemplate is a StringTemplate element ($Type required).
			"messageTemplate": mfStringTemplate("Microflows$StringTemplate", act.MessageTemplate, act.TemplateParameters),
		}
		if act.LogNodeName != "" {
			m["node"] = act.LogNodeName
		}
		return m, nil
	default:
		return nil, fmt.Errorf("microflow action %T is not yet supported by the MCP backend", a)
	}
}

// mfStringTemplate builds a PED StringTemplate ({text, arguments}) from a
// localized text and its placeholder argument expressions. Shared by
// ShowMessage (template) and LogMessage (messageTemplate).
func mfStringTemplate(elementType string, text *model.Text, args []string) map[string]any {
	tmpl := map[string]any{"text": ""}
	if elementType != "" {
		tmpl["$Type"] = elementType
	}
	if text != nil {
		tmpl["text"] = text.Translations["en_US"]
	}
	if len(args) > 0 {
		tmpl["arguments"] = args
	}
	return tmpl
}

// mfVariableType maps a variable's DataType onto the PED CreateVariableAction
// variableType enum (primitives only).
func mfVariableType(dt microflows.DataType) (string, error) {
	if dt == nil {
		return "", fmt.Errorf("missing variable type")
	}
	switch dt.GetTypeName() {
	case "Boolean", "Decimal", "Integer", "String", "DateTime":
		return dt.GetTypeName(), nil
	case "Date":
		return "DateTime", nil
	default:
		return "", fmt.Errorf("variable type %q is not supported by the MCP backend (primitives only)", dt.GetTypeName())
	}
}

// mfDataType maps a microflow DataType onto the PED parameter/return type enum,
// returning (typeName, entityQualifiedName, enumerationQualifiedName). A nil
// DataType is Void (a microflow with no return value).
func mfDataType(dt microflows.DataType) (typeName, entity, enumeration string, err error) {
	if dt == nil {
		return "Void", "", "", nil
	}
	switch dt.GetTypeName() {
	case "Boolean", "Integer", "Decimal", "String", "DateTime":
		return dt.GetTypeName(), "", "", nil
	case "Date":
		return "DateTime", "", "", nil
	case "Object":
		return "Object", mfEntityName(dt), "", nil
	case "List":
		return "List", mfEntityName(dt), "", nil
	case "Enumeration":
		return "Enumeration", "", mfEnumName(dt), nil
	default:
		return "", "", "", fmt.Errorf("data type %q is not yet supported by the MCP backend", dt.GetTypeName())
	}
}

func mfEntityName(dt microflows.DataType) string {
	switch t := dt.(type) {
	case *microflows.ObjectType:
		return t.EntityQualifiedName
	case microflows.ObjectType:
		return t.EntityQualifiedName
	case *microflows.ListType:
		return t.EntityQualifiedName
	case microflows.ListType:
		return t.EntityQualifiedName
	}
	return ""
}

func mfEnumName(dt microflows.DataType) string {
	switch t := dt.(type) {
	case *microflows.EnumerationType:
		return t.EnumerationQualifiedName
	case microflows.EnumerationType:
		return t.EnumerationQualifiedName
	}
	return ""
}
