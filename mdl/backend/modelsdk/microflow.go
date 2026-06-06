// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// gen→microflows read adapter. Covers the breadth SHOW MICROFLOWS needs: name,
// module, excluded, return type, parameter count, and a flow-object collection
// faithful enough for the activity count and McCabe complexity (which key off
// the concrete object types: Start/End/Merge are skipped, splits/loops/error
// events drive complexity). Full per-activity detail (DESCRIBE) is a later phase.

func (b *Backend) ListMicroflows() ([]*microflows.Microflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genMf.Microflow](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*microflows.Microflow, 0, len(units))
	for _, u := range units {
		out = append(out, microflowFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

func (b *Backend) GetMicroflow(id model.ID) (*microflows.Microflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genMf.Microflow](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return microflowFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

func microflowFromGen(mf *genMf.Microflow, containerID model.ID) *microflows.Microflow {
	out := &microflows.Microflow{
		ContainerID:        containerID,
		Name:               mf.Name(),
		Documentation:      mf.Documentation(),
		Excluded:           mf.Excluded(),
		ReturnVariableName: mf.ReturnVariableName(),
		ReturnType:         dataTypeFromGen(mf.MicroflowReturnType()),
	}
	out.ID = model.ID(mf.ID())
	params, objs := splitFlowObjects(mf.ObjectCollection())
	out.Parameters = params
	if objs != nil {
		out.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: objs}
	}
	return out
}

// splitFlowObjects separates parameter objects (which our model keeps in
// Microflow.Parameters) from flow activities (kept in ObjectCollection.Objects),
// mirroring the legacy parser so parameter and activity counts both match.
func splitFlowObjects(coll element.Element) ([]*microflows.MicroflowParameter, []microflows.MicroflowObject) {
	mc, ok := coll.(*genMf.MicroflowObjectCollection)
	if !ok || mc == nil {
		return nil, nil
	}
	var params []*microflows.MicroflowParameter
	var objs []microflows.MicroflowObject
	for _, el := range mc.ObjectsItems() {
		if po, ok := el.(*genMf.MicroflowParameter); ok {
			p := &microflows.MicroflowParameter{Name: po.Name()}
			p.ID = model.ID(el.ID())
			params = append(params, p)
			continue
		}
		objs = append(objs, flowObjectFromGen(el))
	}
	return params, objs
}

// flowObjectFromGen maps a gen flow object to the concrete our-model type the
// activity/complexity counters discriminate on. Non-control objects collapse to
// ActionActivity (sufficient for counting); LoopedActivity recurses so decision
// points inside loop bodies are counted.
func flowObjectFromGen(el element.Element) microflows.MicroflowObject {
	switch el.TypeName() {
	case "Microflows$StartEvent":
		return &microflows.StartEvent{}
	case "Microflows$EndEvent":
		return &microflows.EndEvent{}
	case "Microflows$ExclusiveMerge":
		return &microflows.ExclusiveMerge{}
	case "Microflows$ExclusiveSplit":
		return &microflows.ExclusiveSplit{}
	case "Microflows$InheritanceSplit":
		return &microflows.InheritanceSplit{}
	case "Microflows$ErrorEvent":
		return &microflows.ErrorEvent{}
	case "Microflows$LoopedActivity":
		la := &microflows.LoopedActivity{}
		if g, ok := el.(*genMf.LoopedActivity); ok {
			if _, objs := splitFlowObjects(g.ObjectCollection()); objs != nil {
				la.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: objs}
			}
		}
		return la
	default:
		return &microflows.ActionActivity{}
	}
}

// dataTypeFromGen maps a gen DataTypes$* return-type element to our DataType.
// Only GetTypeName() fidelity is needed for SHOW; nil means no declared type.
func dataTypeFromGen(el element.Element) microflows.DataType {
	if el == nil {
		return nil
	}
	switch el.TypeName() {
	case "DataTypes$BooleanType":
		return microflows.BooleanType{}
	case "DataTypes$IntegerType":
		return microflows.IntegerType{}
	case "DataTypes$LongType":
		return microflows.LongType{}
	case "DataTypes$DecimalType":
		return microflows.DecimalType{}
	case "DataTypes$StringType":
		return microflows.StringType{}
	case "DataTypes$DateTimeType":
		return microflows.DateTimeType{}
	case "DataTypes$VoidType":
		return microflows.VoidType{}
	case "DataTypes$ObjectType":
		return microflows.ObjectType{}
	case "DataTypes$ListType":
		return microflows.ListType{}
	case "DataTypes$EnumerationType":
		return microflows.EnumerationType{}
	default:
		return nil
	}
}
