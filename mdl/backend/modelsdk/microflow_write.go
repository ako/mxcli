// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"sort"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDT "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func init() {
	// The microflow wrapper always serializes these two action-info slots as null.
	codec.RegisterTypeDefaults("Microflows$Microflow", codec.TypeDefaults{
		NullFields: []string{"MicroflowActionInfo", "WorkflowActionInfo"},
		// StableId is a GUID stored as binary; the gen mistypes it as a string, so
		// emit it via the fresh-GUID default instead (verified vs test7-app).
		FreshGUIDFields: []string{"StableId"},
	})
	// A SequenceFlow's CaseValues list uses typed-array marker 2 (like an index's
	// IndexedAttribute list), not the default 3. Keyed by the case child types.
	for _, t := range []string{"Microflows$NoCase", "Microflows$EnumerationCase", "Microflows$ExpressionCase", "Microflows$InheritanceCase"} {
		codec.RegisterListMarker(t, 2)
	}
	// Create/Change actions store their member-change Items list with marker 2.
	codec.RegisterListMarker("Microflows$ChangeActionItem", 2)
	// A retrieve's SortingsList always serializes its (possibly empty) Sortings
	// list with marker 2 (verified vs real BSON: test7 PopulateUserAttributes).
	codec.RegisterTypeDefaults("Microflows$SortingsList", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Sortings": 2},
	})
	codec.RegisterListMarker("Microflows$RetrieveSorting", 2)
	// Call actions: ParameterMappings list always emitted (marker 2); a MicroflowCall
	// always serializes QueueSettings as null.
	codec.RegisterTypeDefaults("Microflows$MicroflowCall", codec.TypeDefaults{
		NullFields:           []string{"QueueSettings"},
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
	codec.RegisterTypeDefaults("Microflows$NanoflowCall", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ParameterMappings": 2},
	})
	codec.RegisterListMarker("Microflows$MicroflowCallParameterMapping", 2)
	codec.RegisterListMarker("Microflows$NanoflowCallParameterMapping", 2)
}

// majorVersion returns the project's Mendix major version (for version-gated BSON).
func (b *Backend) majorVersion() int {
	if pv := b.ProjectVersion(); pv != nil {
		return pv.MajorVersion
	}
	return 11
}

// CreateMicroflow adds a new microflow document (a top-level unit).
func (b *Backend) CreateMicroflow(mf *microflows.Microflow) error {
	if mf == nil {
		return fmt.Errorf("CreateMicroflow: nil microflow")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateMicroflow: not connected for writing")
	}
	if mf.ID == "" {
		mf.ID = model.ID(mmpr.GenerateID())
	}
	gm := microflowToGen(mf, b.majorVersion())
	gm.SetID(element.ID(mf.ID))
	assignMicroflowIDs(gm)
	contents, err := (&codec.Encoder{}).Encode(gm)
	if err != nil {
		return fmt.Errorf("CreateMicroflow: encode: %w", err)
	}
	return b.writer.InsertUnit(string(mf.ID), string(mf.ContainerID), "Documents", "Microflows$Microflow", contents)
}

// UpdateMicroflow rebuilds a microflow document (the CREATE OR REPLACE path).
func (b *Backend) UpdateMicroflow(mf *microflows.Microflow) error {
	if mf == nil {
		return fmt.Errorf("UpdateMicroflow: nil microflow")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateMicroflow: not connected for writing")
	}
	gm := microflowToGen(mf, b.majorVersion())
	gm.SetID(element.ID(mf.ID))
	assignMicroflowIDs(gm)
	contents, err := (&codec.Encoder{}).Encode(gm)
	if err != nil {
		return fmt.Errorf("UpdateMicroflow: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(mf.ID), contents)
}

// DeleteMicroflow removes the microflow unit.
func (b *Backend) DeleteMicroflow(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteMicroflow: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// microflowToGen builds a gen Microflow wrapper from the model. Mirrors the legacy
// serializer (writer_microflow.go). v10+ adds ReturnVariableName/StableId/Url/
// UrlSearchParameters.
func microflowToGen(mf *microflows.Microflow, major int) *genMf.Microflow {
	out := genMf.NewMicroflow()
	out.SetName(mf.Name)
	out.SetDocumentation(mf.Documentation)
	out.SetExcluded(mf.Excluded)
	out.SetExportLevel("Hidden")
	out.SetAllowConcurrentExecution(mf.AllowConcurrentExecution)
	out.SetApplyEntityAccess(false)
	out.SetMarkAsUsed(mf.MarkAsUsed)
	out.SetConcurrencyErrorMicroflowQualifiedName("")
	out.SetConcurrencyErrorMessage(genTexts.NewText()) // empty Texts$Text (Items=[3] via default)
	out.SetAllowedModuleRolesQualifiedNames(moduleRoleNames(mf.AllowedModuleRoles))
	out.SetMicroflowReturnType(microflowDataTypeToGen(mf.ReturnType))

	// Object collection (parameters merged first, then objects).
	oc := genMf.NewMicroflowObjectCollection()
	for i, p := range mf.Parameters {
		oc.AddObjects(microflowParameterToGen(p, i, major))
	}
	if mf.ObjectCollection != nil {
		for _, obj := range mf.ObjectCollection.Objects {
			if g := microflowObjectToGen(obj); g != nil {
				oc.AddObjects(g)
			}
		}
	}
	out.SetObjectCollection(oc)

	// Flows live on the microflow, not in the object collection.
	if mf.ObjectCollection != nil {
		for _, f := range mf.ObjectCollection.Flows {
			out.AddFlows(sequenceFlowToGen(f, major))
		}
	}

	if major >= 10 {
		out.SetReturnVariableName(mf.ReturnVariableName)
		out.SetUrl("")
		// StableId is emitted as a fresh GUID binary via the registered default
		// (the gen mistypes it as a string), not set here.
		out.SetUrlSearchParametersQualifiedNames(nil) // empty marker-1 list
	}
	return out
}

// moduleRoleNames renders allowed-module-role IDs as the by-name list the codec
// emits. Empty input yields an empty (marker-1) list, matching the legacy writer.
func moduleRoleNames(ids []model.ID) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

// microflowObjectToGen dispatches a flow object to its gen element. Skeleton:
// start/end events; activities are added group by group.
func microflowObjectToGen(obj microflows.MicroflowObject) element.Element {
	switch o := obj.(type) {
	case *microflows.StartEvent:
		g := genMf.NewStartEvent()
		g.SetID(element.ID(o.ID))
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.EndEvent:
		g := genMf.NewEndEvent()
		g.SetID(element.ID(o.ID))
		g.SetDocumentation("")
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetReturnValue(o.ReturnValue)
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.ActionActivity:
		g := genMf.NewActionActivity()
		g.SetID(element.ID(o.ID))
		if a := microflowActionToGen(o.Action); a != nil {
			g.SetAction(a)
		}
		g.SetAutoGenerateCaption(o.AutoGenerateCaption)
		g.SetBackgroundColor(orDefault(o.BackgroundColor, "Default"))
		g.SetCaption(o.Caption)
		g.SetDisabled(o.Disabled)
		g.SetDocumentation(o.Documentation)
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.ExclusiveSplit:
		g := genMf.NewExclusiveSplit()
		g.SetID(element.ID(o.ID))
		g.SetCaption(o.Caption)
		g.SetDocumentation(o.Documentation)
		g.SetErrorHandlingType(string(o.ErrorHandlingType))
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		if sc := splitConditionToGen(o.SplitCondition); sc != nil {
			g.SetSplitCondition(sc)
		}
		return g
	case *microflows.ExclusiveMerge:
		g := genMf.NewExclusiveMerge()
		g.SetID(element.ID(o.ID))
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	case *microflows.LoopedActivity:
		g := genMf.NewLoopedActivity()
		g.SetID(element.ID(o.ID))
		g.SetErrorHandlingType(string(o.ErrorHandlingType))
		if ls := loopSourceToGen(o.LoopSource); ls != nil {
			g.SetLoopSource(ls)
		}
		// Nested loop body: objects only (the gen/legacy ObjectCollection carries
		// no inline Flows — loop-external flows live on the top-level Microflow).
		if o.ObjectCollection != nil {
			noc := genMf.NewMicroflowObjectCollection()
			for _, obj := range o.ObjectCollection.Objects {
				if e := microflowObjectToGen(obj); e != nil {
					noc.AddObjects(e)
				}
			}
			g.SetObjectCollection(noc)
		}
		g.SetRelativeMiddlePoint(pointStr(o.Position))
		g.SetSize(sizeStr(o.Size))
		return g
	default:
		return nil // unsupported object type (added in later activity groups)
	}
}

// loopSourceToGen builds a loop's source (iterate-over-list or while-condition).
func loopSourceToGen(ls microflows.LoopSource) element.Element {
	switch s := ls.(type) {
	case *microflows.IterableList:
		g := genMf.NewIterableList()
		g.SetID(element.ID(s.ID))
		g.SetListVariableName(s.ListVariableName)
		g.SetVariableName(s.VariableName)
		return g
	case *microflows.WhileLoopCondition:
		g := genMf.NewWhileLoopCondition()
		g.SetID(element.ID(s.ID))
		g.SetWhileExpression(s.WhileExpression)
		return g
	default:
		return nil
	}
}

// microflowActionToGen dispatches a microflow action to its gen element. Object
// operations group: create/change/commit/delete/rollback. Uses the storage $Type
// (set by the gen constructors). Returns nil for not-yet-supported actions.
func microflowActionToGen(action microflows.MicroflowAction) element.Element {
	switch a := action.(type) {
	case *microflows.CreateObjectAction:
		g := genMf.NewCreateObjectAction()
		g.SetID(element.ID(a.ID))
		g.SetCommit(string(a.Commit))
		if a.EntityQualifiedName != "" {
			g.SetEntityQualifiedName(a.EntityQualifiedName)
		}
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		for _, m := range a.InitialMembers {
			g.AddItems(memberChangeToGen(m))
		}
		g.SetRefreshInClient(false)
		g.SetOutputVariableName(a.OutputVariable)
		return g
	case *microflows.ChangeObjectAction:
		g := genMf.NewChangeObjectAction()
		g.SetID(element.ID(a.ID))
		g.SetChangeVariableName(a.ChangeVariable)
		g.SetCommit(string(a.Commit))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		for _, m := range a.Changes {
			g.AddItems(memberChangeToGen(m))
		}
		g.SetRefreshInClient(a.RefreshInClient)
		return g
	case *microflows.CommitObjectsAction:
		g := genMf.NewCommitAction()
		g.SetID(element.ID(a.ID))
		g.SetCommitVariableName(a.CommitVariable)
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetRefreshInClient(a.RefreshInClient)
		g.SetWithEvents(a.WithEvents)
		return g
	case *microflows.DeleteObjectAction:
		g := genMf.NewDeleteAction()
		g.SetID(element.ID(a.ID))
		g.SetDeleteVariableName(a.DeleteVariable)
		g.SetRefreshInClient(a.RefreshInClient)
		return g
	case *microflows.RollbackObjectAction:
		g := genMf.NewRollbackAction()
		g.SetID(element.ID(a.ID))
		g.SetRollbackVariableName(a.RollbackVariable)
		g.SetRefreshInClient(a.RefreshInClient)
		return g
	case *microflows.RetrieveAction:
		g := genMf.NewRetrieveAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType("Rollback")
		g.SetOutputVariableName(a.OutputVariable)
		if src := retrieveSourceToGen(a.Source); src != nil {
			g.SetRetrieveSource(src)
		}
		return g
	case *microflows.MicroflowCallAction:
		g := genMf.NewMicroflowCallAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		if a.MicroflowCall != nil {
			mc := genMf.NewMicroflowCall()
			mc.SetID(element.ID(a.MicroflowCall.ID))
			mc.SetMicroflowQualifiedName(a.MicroflowCall.Microflow)
			for _, pm := range a.MicroflowCall.ParameterMappings {
				m := genMf.NewMicroflowCallParameterMapping()
				m.SetID(element.ID(pm.ID))
				m.SetParameterQualifiedName(pm.Parameter)
				m.SetArgument(pm.Argument)
				mc.AddParameterMappings(m)
			}
			g.SetMicroflowCall(mc)
		}
		g.SetOutputVariableName(a.ResultVariableName) // BSON key "ResultVariableName"
		g.SetUseReturnVariable(a.UseReturnVariable)
		return g
	case *microflows.LogMessageAction:
		g := genMf.NewLogMessageAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetIncludeLatestStackTrace(a.IncludeLastStackTrace)
		g.SetLevel(string(a.LogLevel))
		g.SetNode(a.LogNodeName)
		if a.MessageTemplate != nil {
			g.SetMessageTemplate(stringTemplateToGen(a.MessageTemplate, a.TemplateParameters))
		}
		return g
	case *microflows.CreateVariableAction:
		g := genMf.NewCreateVariableAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetVariableName(a.VariableName)
		g.SetInitialValue(a.InitialValue)
		if a.DataType != nil {
			g.SetVariableType(microflowDataTypeToGen(a.DataType))
		}
		return g
	case *microflows.ChangeVariableAction:
		g := genMf.NewChangeVariableAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetChangeVariableName(a.VariableName)
		g.SetValue(a.Value)
		return g
	case *microflows.NanoflowCallAction:
		g := genMf.NewNanoflowCallAction()
		g.SetID(element.ID(a.ID))
		g.SetErrorHandlingType(orDefault(string(a.ErrorHandlingType), "Rollback"))
		g.SetOutputVariableName(a.OutputVariableName)
		g.SetUseReturnVariable(a.UseReturnVariable)
		if a.NanoflowCall != nil {
			nc := genMf.NewNanoflowCall()
			nc.SetID(element.ID(a.NanoflowCall.ID))
			nc.SetNanoflowQualifiedName(a.NanoflowCall.Nanoflow)
			for _, pm := range a.NanoflowCall.ParameterMappings {
				m := genMf.NewNanoflowCallParameterMapping()
				m.SetID(element.ID(pm.ID))
				m.SetParameterQualifiedName(pm.Parameter)
				m.SetArgument(pm.Argument)
				nc.AddParameterMappings(m)
			}
			g.SetNanoflowCall(nc)
		}
		return g
	default:
		return nil // not yet supported (added in later groups)
	}
}

// splitConditionToGen builds an exclusive-split condition. Rule conditions are a
// later slice (RuleCall + parameter mappings).
func splitConditionToGen(sc microflows.SplitCondition) element.Element {
	switch c := sc.(type) {
	case *microflows.ExpressionSplitCondition:
		g := genMf.NewExpressionSplitCondition()
		g.SetID(element.ID(c.ID))
		g.SetExpression(c.Expression)
		return g
	default:
		return nil
	}
}

// caseValueToGen renders a sequence-flow case. ExpressionCase is serialized AS an
// EnumerationCase with Value = the expression ("true"/"false") — Studio Pro has
// never recognised Microflows$ExpressionCase (verified vs legacy). Default NoCase.
func caseValueToGen(cv microflows.CaseValue) element.Element {
	switch c := cv.(type) {
	case *microflows.EnumerationCase:
		g := genMf.NewEnumerationCase()
		g.SetID(element.ID(c.ID))
		g.SetValue(c.Value)
		return g
	case *microflows.ExpressionCase:
		g := genMf.NewEnumerationCase()
		g.SetID(element.ID(c.ID))
		g.SetValue(c.Expression)
		return g
	default:
		return genMf.NewNoCase()
	}
}

// retrieveSourceToGen builds a gen retrieve source. Verified against real BSON
// (test7): a DatabaseRetrieveSource always carries a Range (default ConstantRange,
// SingleObject=false) and a NewSortings SortingsList (empty Sortings=[2]).
func retrieveSourceToGen(src microflows.RetrieveSource) element.Element {
	switch s := src.(type) {
	case *microflows.DatabaseRetrieveSource:
		g := genMf.NewDatabaseRetrieveSource()
		g.SetID(element.ID(s.ID))
		if s.EntityQualifiedName != "" {
			g.SetEntityQualifiedName(s.EntityQualifiedName)
		}
		g.SetRange(rangeToGen(s.Range))
		if s.XPathConstraint != "" {
			g.SetXPathConstraint(s.XPathConstraint)
		}
		g.SetSortItemList(genMf.NewSortItemList()) // empty Sortings=[2] via default; sort columns are a later slice
		return g
	case *microflows.AssociationRetrieveSource:
		g := genMf.NewAssociationRetrieveSource()
		g.SetID(element.ID(s.ID))
		if s.StartVariable != "" {
			g.SetStartVariableName(s.StartVariable)
		}
		if s.AssociationQualifiedName != "" {
			g.SetAssociationQualifiedName(s.AssociationQualifiedName)
		}
		return g
	default:
		return nil
	}
}

// rangeToGen builds a gen retrieve Range; nil → default ConstantRange (retrieve all).
func rangeToGen(r *microflows.Range) element.Element {
	if r == nil {
		g := genMf.NewConstantRange()
		g.SetSingleObject(false)
		return g
	}
	if r.RangeType == microflows.RangeTypeCustom {
		g := genMf.NewCustomRange()
		if r.Limit != "" {
			g.SetLimitExpression(r.Limit)
		}
		if r.Offset != "" {
			g.SetOffsetExpression(r.Offset)
		}
		return g
	}
	g := genMf.NewConstantRange()
	g.SetSingleObject(r.RangeType == microflows.RangeTypeFirst)
	return g
}

// memberChangeToGen builds a gen MemberChange (ChangeActionItem). Mirrors legacy:
// Association is emitted as "" for attribute-targeted changes.
func memberChangeToGen(m *microflows.MemberChange) element.Element {
	g := genMf.NewMemberChange()
	g.SetID(element.ID(m.ID))
	if m.AssociationQualifiedName != "" {
		g.SetAssociationQualifiedName(m.AssociationQualifiedName)
	} else {
		g.SetAssociationQualifiedName("")
		if m.AttributeQualifiedName != "" {
			g.SetAttributeQualifiedName(m.AttributeQualifiedName)
		}
	}
	g.SetType(string(m.Type))
	g.SetValue(m.Value)
	return g
}

// microflowParameterToGen builds a gen MicroflowParameter (position derives from
// index, matching the legacy serializer).
func microflowParameterToGen(p *microflows.MicroflowParameter, idx, major int) element.Element {
	g := genMf.NewMicroflowParameter()
	g.SetID(element.ID(p.ID))
	g.SetDocumentation(p.Documentation)
	g.SetHasVariableNameBeenChanged(false)
	g.SetName(p.Name)
	g.SetRelativeMiddlePoint(fmt.Sprintf("%d;53", 200+idx*100))
	g.SetSize("30;30")
	if major >= 10 {
		g.SetDefaultValue("")
		g.SetIsRequired(true)
	}
	g.SetParameterType(microflowDataTypeToGen(p.Type))
	return g
}

// sequenceFlowToGen builds a gen SequenceFlow. v10+ uses CaseValues + a BezierCurve
// Line; the case defaults to NoCase.
func sequenceFlowToGen(f *microflows.SequenceFlow, major int) element.Element {
	g := genMf.NewSequenceFlow()
	g.SetID(element.ID(f.ID))
	g.SetOriginID(element.ID(f.OriginID))
	g.SetDestinationID(element.ID(f.DestinationID))
	g.SetOriginConnectionIndex(int32(f.OriginConnectionIndex))
	g.SetDestinationConnectionIndex(int32(f.DestinationConnectionIndex))
	g.SetIsErrorHandler(f.IsErrorHandler)
	g.AddCaseValues(caseValueToGen(f.CaseValue))

	originCV := orDefault(f.OriginControlVector, "0;0")
	destCV := orDefault(f.DestinationControlVector, "0;0")
	line := genMf.NewBezierCurve()
	line.SetOriginControlVector(originCV)
	line.SetDestinationControlVector(destCV)
	g.SetLine(line)
	return g
}

// stringTemplateToGen builds a Microflows$StringTemplate (the message body of a
// LogMessageAction): a Text plus one TemplateParameter per {1},{2},… expression.
// The text is taken from the (sorted, for determinism) first translation.
func stringTemplateToGen(text *model.Text, params []string) *genMf.StringTemplate {
	st := genMf.NewStringTemplate()
	assignID(st)
	st.SetText(firstTranslation(text))
	for _, expr := range params {
		tp := genMf.NewTemplateArgument()
		assignID(tp)
		tp.SetExpression(expr)
		st.AddArguments(tp)
	}
	return st
}

// firstTranslation returns a translation value deterministically (lowest language
// code), or "" when there are none.
func firstTranslation(text *model.Text) string {
	if text == nil || len(text.Translations) == 0 {
		return ""
	}
	keys := make([]string, 0, len(text.Translations))
	for k := range text.Translations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return text.Translations[keys[0]]
}

// microflowDataTypeToGen maps a microflow DataType to a gen DataTypes$* element
// (nil → VoidType). Long maps to IntegerType (per the legacy serializer).
func microflowDataTypeToGen(dt microflows.DataType) element.Element {
	if dt == nil {
		return genDT.NewVoidType()
	}
	switch a := dt.(type) {
	case *microflows.BooleanType:
		return genDT.NewBooleanType()
	case *microflows.IntegerType, *microflows.LongType:
		return genDT.NewIntegerType()
	case *microflows.DecimalType:
		return genDT.NewDecimalType()
	case *microflows.StringType:
		return genDT.NewStringType()
	case *microflows.DateTimeType, *microflows.DateType: // Date maps to DateTime in BSON
		return genDT.NewDateTimeType()
	case *microflows.BinaryType:
		return genDT.NewBinaryType()
	case *microflows.ObjectType:
		t := genDT.NewObjectType()
		t.SetEntityQualifiedName(a.EntityQualifiedName)
		return t
	case *microflows.ListType:
		t := genDT.NewListType()
		t.SetEntityQualifiedName(a.EntityQualifiedName)
		return t
	default:
		return genDT.NewVoidType()
	}
}

// assignMicroflowIDs assigns fresh IDs to wrapper sub-elements that lack one
// (return type, object collection + its objects, flows + their cases/lines).
func assignMicroflowIDs(m *genMf.Microflow) {
	assignID(m.MicroflowReturnType())
	assignID(m.ConcurrencyErrorMessage())
	if oc, ok := m.ObjectCollection().(*genMf.MicroflowObjectCollection); ok {
		assignObjectCollectionIDs(oc)
	}
	for _, el := range m.FlowsItems() {
		assignID(el)
		if sf, ok := el.(*genMf.SequenceFlow); ok {
			for _, cv := range sf.CaseValuesItems() {
				assignID(cv)
			}
			assignID(sf.Line())
		}
	}
}

// assignObjectCollectionIDs assigns IDs to a collection's objects and their
// sub-elements, recursing into loop bodies.
func assignObjectCollectionIDs(oc *genMf.MicroflowObjectCollection) {
	assignID(oc)
	for _, el := range oc.ObjectsItems() {
		assignFlowObjectIDs(el)
	}
}

// assignFlowObjectIDs assigns the element's ID and walks its sub-elements
// (parameter type, split condition, action items/sources, loop source + nested body).
func assignFlowObjectIDs(el element.Element) {
	assignID(el)
	switch o := el.(type) {
	case *genMf.MicroflowParameter:
		assignID(o.ParameterType())
	case *genMf.ExclusiveSplit:
		assignID(o.SplitCondition())
	case *genMf.LoopedActivity:
		assignID(o.LoopSource())
		if noc, ok := o.ObjectCollection().(*genMf.MicroflowObjectCollection); ok {
			assignObjectCollectionIDs(noc)
		}
	case *genMf.ActionActivity:
		act := o.Action()
		assignID(act)
		switch a := act.(type) {
		case *genMf.CreateObjectAction:
			for _, it := range a.ItemsItems() {
				assignID(it)
			}
		case *genMf.ChangeObjectAction:
			for _, it := range a.ItemsItems() {
				assignID(it)
			}
		case *genMf.RetrieveAction:
			src := a.RetrieveSource()
			assignID(src)
			if db, ok := src.(*genMf.DatabaseRetrieveSource); ok {
				assignID(db.Range())
				if sl, ok := db.SortItemList().(*genMf.SortItemList); ok {
					assignID(sl)
					for _, it := range sl.ItemsItems() {
						assignID(it)
					}
				}
			}
		case *genMf.MicroflowCallAction:
			if mc, ok := a.MicroflowCall().(*genMf.MicroflowCall); ok {
				assignID(mc)
				for _, pm := range mc.ParameterMappingsItems() {
					assignID(pm)
				}
			}
		case *genMf.NanoflowCallAction:
			if nc, ok := a.NanoflowCall().(*genMf.NanoflowCall); ok {
				assignID(nc)
				for _, pm := range nc.ParameterMappingsItems() {
					assignID(pm)
				}
			}
		}
	}
}

func pointStr(p model.Point) string { return fmt.Sprintf("%d;%d", p.X, p.Y) }
func sizeStr(s model.Size) string {
	if s.Width == 0 && s.Height == 0 {
		return "0;0"
	}
	return fmt.Sprintf("%d;%d", s.Width, s.Height)
}
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
