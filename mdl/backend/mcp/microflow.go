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
	// idPath maps each object's ID to its JSON-pointer path. Loop bodies nest, so
	// a body object's path is /objects/N/objects/M; flows (which all live at the
	// microflow top level, even loop-body flows) reference objects by these paths.
	idPath := map[model.ID]string{}
	if mf.ObjectCollection != nil {
		for i, o := range mf.ObjectCollection.Objects {
			path := fmt.Sprintf("/objects/%d", paramCount+i)
			pedObj, err := b.mapObjectTree(o, path, i, idPath)
			if err != nil {
				return fmt.Errorf("microflow %q: %w", mf.Name, err)
			}
			objects = append(objects, pedObj)
		}
	}

	flows := make([]any, 0)
	if mf.ObjectCollection != nil {
		for _, f := range mf.ObjectCollection.Flows {
			op, ok1 := idPath[f.OriginID]
			dp, ok2 := idPath[f.DestinationID]
			if !ok1 || !ok2 {
				return fmt.Errorf("microflow %q: a sequence flow references an object that is not supported yet", mf.Name)
			}
			pf := map[string]any{
				"originId":      fmt.Sprintf("$id(%s)", op),
				"destinationId": fmt.Sprintf("$id(%s)", dp),
			}
			cv, err := mapCaseValue(f.CaseValue)
			if err != nil {
				return fmt.Errorf("microflow %q: %w", mf.Name, err)
			}
			if cv != nil {
				pf["caseValue"] = cv
			}
			flows = append(flows, pf)
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

// mapObjectTree maps one executor microflow object onto a PED /objects entry,
// registering its path in idPath and recursing into loop bodies (whose objects
// nest at <path>/objects/M). localIndex drives the cosmetic left-to-right layout
// within the object's container. Unmapped object types are rejected.
func (b *Backend) mapObjectTree(o microflows.MicroflowObject, path string, localIndex int, idPath map[model.ID]string) (map[string]any, error) {
	idPath[o.GetID()] = path
	switch obj := o.(type) {
	case *microflows.StartEvent:
		return map[string]any{
			"$Type":               "Microflows$StartEvent",
			"relativeMiddlePoint": canvasPoint(localIndex, 120),
		}, nil
	case *microflows.EndEvent:
		m := map[string]any{
			"$Type":               "Microflows$EndEvent",
			"relativeMiddlePoint": canvasPoint(localIndex, 120),
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
			"relativeMiddlePoint": canvasPoint(localIndex, 120),
			"action":              action,
		}, nil
	case *microflows.ExclusiveSplit:
		m := map[string]any{
			"$Type":               "Microflows$ExclusiveSplit",
			"relativeMiddlePoint": canvasPoint(localIndex, 120),
		}
		switch c := obj.SplitCondition.(type) {
		case *microflows.ExpressionSplitCondition:
			m["expressionSplitCondition"] = c.Expression
		case microflows.ExpressionSplitCondition:
			m["expressionSplitCondition"] = c.Expression
		default:
			return nil, fmt.Errorf("exclusive split: only expression conditions are supported yet (got %T)", obj.SplitCondition)
		}
		if obj.Caption != "" {
			m["caption"] = obj.Caption
		}
		return m, nil
	case *microflows.ExclusiveMerge:
		return map[string]any{
			"$Type":               "Microflows$ExclusiveMerge",
			"relativeMiddlePoint": canvasPoint(localIndex, 120),
		}, nil
	case *microflows.BreakEvent:
		return map[string]any{
			"$Type":               "Microflows$BreakEvent",
			"relativeMiddlePoint": canvasPoint(localIndex, 120),
		}, nil
	case *microflows.ContinueEvent:
		return map[string]any{
			"$Type":               "Microflows$ContinueEvent",
			"relativeMiddlePoint": canvasPoint(localIndex, 120),
		}, nil
	case *microflows.LoopedActivity:
		m := map[string]any{
			"$Type":               "Microflows$LoopedActivity",
			"relativeMiddlePoint": canvasPoint(localIndex, 120),
		}
		switch src := obj.LoopSource.(type) {
		case *microflows.IterableList:
			m["iterableListSource"] = map[string]any{
				"listVariableName":     src.ListVariableName,
				"iteratorVariableName": src.VariableName,
			}
		case *microflows.WhileLoopCondition:
			m["whileLoopSource"] = map[string]any{"condition": src.WhileExpression}
		default:
			return nil, fmt.Errorf("loop: unsupported loop source %T", obj.LoopSource)
		}
		// Body objects nest under <path>/objects/M. Their flows are already at
		// the microflow top level (the executor lifts them there), so we only
		// map objects here and register their nested paths.
		body := make([]any, 0)
		if obj.ObjectCollection != nil {
			for j, bo := range obj.ObjectCollection.Objects {
				bp := fmt.Sprintf("%s/objects/%d", path, j)
				pedBody, err := b.mapObjectTree(bo, bp, j, idPath)
				if err != nil {
					return nil, err
				}
				body = append(body, pedBody)
			}
		}
		m["objects"] = body
		return m, nil
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
	case *microflows.CreateObjectAction:
		if act.EntityQualifiedName == "" {
			return nil, fmt.Errorf("create object: missing entity")
		}
		m := map[string]any{
			"$Type":  "Microflows$CreateObjectAction",
			"entity": act.EntityQualifiedName,
			"commit": mfCommitType(act.Commit),
		}
		if act.OutputVariable != "" {
			m["outputVariableName"] = act.OutputVariable
		}
		if len(act.InitialMembers) > 0 {
			m["items"] = mapMemberChanges(act.InitialMembers)
		}
		return m, nil
	case *microflows.ChangeObjectAction:
		m := map[string]any{
			"$Type":              "Microflows$ChangeObjectAction",
			"changeVariableName": act.ChangeVariable,
			"commit":             mfCommitType(act.Commit),
			"refreshInClient":    act.RefreshInClient,
		}
		if len(act.Changes) > 0 {
			m["items"] = mapMemberChanges(act.Changes)
		}
		return m, nil
	case *microflows.CommitObjectsAction:
		return map[string]any{
			"$Type":              "Microflows$CommitAction",
			"commitVariableName": act.CommitVariable,
			"withEvents":         act.WithEvents,
			"refreshInClient":    act.RefreshInClient,
		}, nil
	case *microflows.DeleteObjectAction:
		return map[string]any{
			"$Type":              "Microflows$DeleteAction",
			"deleteVariableName": act.DeleteVariable,
			"refreshInClient":    act.RefreshInClient,
		}, nil
	case *microflows.MicroflowCallAction:
		if act.MicroflowCall == nil || act.MicroflowCall.Microflow == "" {
			return nil, fmt.Errorf("call microflow: missing target microflow")
		}
		mappings := make([]any, 0, len(act.MicroflowCall.ParameterMappings))
		for _, pm := range act.MicroflowCall.ParameterMappings {
			mappings = append(mappings, map[string]any{
				"$Type":     "Microflows$MicroflowCallParameterMapping",
				"parameter": pm.Parameter,
				"argument":  pm.Argument,
			})
		}
		call := map[string]any{
			"$Type":     "Microflows$MicroflowCall",
			"microflow": act.MicroflowCall.Microflow,
		}
		if len(mappings) > 0 {
			call["parameterMappings"] = mappings
		}
		m := map[string]any{
			"$Type":             "Microflows$MicroflowCallAction",
			"microflowCall":     call,
			"useReturnVariable": act.UseReturnVariable,
		}
		if act.ResultVariableName != "" {
			m["outputVariableName"] = act.ResultVariableName
		}
		return m, nil
	case *microflows.RetrieveAction:
		m := map[string]any{"$Type": "Microflows$RetrieveAction"}
		if act.OutputVariable != "" {
			m["outputVariableName"] = act.OutputVariable
		}
		switch src := act.Source.(type) {
		case *microflows.AssociationRetrieveSource:
			m["byAssociation"] = map[string]any{
				"startVariableName": src.StartVariable,
				"association":       src.AssociationQualifiedName,
			}
		case *microflows.DatabaseRetrieveSource:
			if len(src.Sorting) > 0 {
				return nil, fmt.Errorf("retrieve: sorting is not yet supported by the MCP backend")
			}
			if src.Range != nil && src.Range.RangeType == microflows.RangeTypeCustom {
				return nil, fmt.Errorf("retrieve: custom range (offset/limit) is not yet supported by the MCP backend")
			}
			q := map[string]any{"entity": src.EntityQualifiedName}
			if src.XPathConstraint != "" {
				q["xPathConstraint"] = src.XPathConstraint
			}
			if src.Range != nil && src.Range.RangeType == microflows.RangeTypeFirst {
				q["takeOnlyFirst"] = true
			}
			m["byDatabaseQuery"] = q
		default:
			return nil, fmt.Errorf("retrieve: unsupported source %T", act.Source)
		}
		return m, nil
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

// mapCaseValue maps a sequence-flow case value onto the PED flow caseValue
// object ({enumerationCase} for boolean/enum splits, {inheritanceCase} for
// inheritance splits). A nil / NoCase value yields no caseValue (non-split
// flow). Boolean branches arrive as ExpressionCase{"true"|"false"}.
func mapCaseValue(cv microflows.CaseValue) (map[string]any, error) {
	switch c := cv.(type) {
	case nil:
		return nil, nil
	case *microflows.ExpressionCase:
		return map[string]any{"enumerationCase": c.Expression}, nil
	case microflows.ExpressionCase:
		return map[string]any{"enumerationCase": c.Expression}, nil
	case *microflows.EnumerationCase:
		return map[string]any{"enumerationCase": c.Value}, nil
	case microflows.EnumerationCase:
		return map[string]any{"enumerationCase": c.Value}, nil
	case *microflows.BooleanCase:
		return map[string]any{"enumerationCase": boolString(c.Value)}, nil
	case microflows.BooleanCase:
		return map[string]any{"enumerationCase": boolString(c.Value)}, nil
	case *microflows.InheritanceCase:
		return map[string]any{"inheritanceCase": c.EntityQualifiedName}, nil
	case microflows.InheritanceCase:
		return map[string]any{"inheritanceCase": c.EntityQualifiedName}, nil
	case *microflows.NoCase, microflows.NoCase:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported flow case value %T", cv)
	}
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// mapMemberChanges maps a list of attribute/association member changes onto PED
// MemberChange elements ({attribute|association by-name, type, value}).
func mapMemberChanges(changes []*microflows.MemberChange) []any {
	items := make([]any, 0, len(changes))
	for _, c := range changes {
		m := map[string]any{
			"$Type": "Microflows$MemberChange",
			"type":  memberChangeType(c.Type),
			"value": c.Value,
		}
		if c.AttributeQualifiedName != "" {
			m["attribute"] = c.AttributeQualifiedName
		}
		if c.AssociationQualifiedName != "" {
			m["association"] = c.AssociationQualifiedName
		}
		items = append(items, m)
	}
	return items
}

func memberChangeType(t microflows.MemberChangeType) string {
	if t == "" {
		return "Set"
	}
	return string(t)
}

// mfCommitType maps a CommitType onto the PED commit enum (Yes / YesWithoutEvents / No).
func mfCommitType(c microflows.CommitType) string {
	switch c {
	case microflows.CommitTypeYes, microflows.CommitTypeYesWithEvents:
		return "Yes"
	case microflows.CommitTypeNoEvent:
		return "YesWithoutEvents"
	default:
		return "No"
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
	case "Void":
		return "Void", "", "", nil
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
