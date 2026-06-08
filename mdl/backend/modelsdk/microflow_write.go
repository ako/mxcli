// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

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
	default:
		return nil // unsupported object type (added in later activity groups)
	}
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

// caseValueToGen renders a sequence-flow case (NoCase default).
func caseValueToGen(cv microflows.CaseValue) element.Element {
	return genMf.NewNoCase()
}

// microflowDataTypeToGen maps a microflow DataType to a gen DataTypes$* element
// (nil → VoidType). Long maps to IntegerType (per the legacy serializer).
func microflowDataTypeToGen(dt microflows.DataType) element.Element {
	if dt == nil {
		return genDT.NewVoidType()
	}
	switch dt.(type) {
	case *microflows.BooleanType:
		return genDT.NewBooleanType()
	case *microflows.IntegerType, *microflows.LongType:
		return genDT.NewIntegerType()
	case *microflows.DecimalType:
		return genDT.NewDecimalType()
	case *microflows.StringType:
		return genDT.NewStringType()
	case *microflows.DateTimeType:
		return genDT.NewDateTimeType()
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
		assignID(oc)
		for _, el := range oc.ObjectsItems() {
			assignID(el)
			if p, ok := el.(*genMf.MicroflowParameter); ok {
				assignID(p.ParameterType())
			}
		}
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
