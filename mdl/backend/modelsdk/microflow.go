// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
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

func (b *Backend) ListNanoflows() ([]*microflows.Nanoflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genMf.Nanoflow](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*microflows.Nanoflow, 0, len(units))
	for _, u := range units {
		out = append(out, nanoflowFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

func (b *Backend) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genMf.Nanoflow](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return nanoflowFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

// nanoflowFromGen mirrors microflowFromGen — nanoflows share the same parameter,
// flow-object, and return-type structures, so the same helpers apply.
func nanoflowFromGen(nf *genMf.Nanoflow, containerID model.ID) *microflows.Nanoflow {
	out := &microflows.Nanoflow{
		ContainerID:   containerID,
		Name:          nf.Name(),
		Documentation: nf.Documentation(),
		Excluded:      nf.Excluded(),
		ReturnType:    dataTypeFromGen(nf.MicroflowReturnType()),
	}
	out.ID = model.ID(nf.ID())
	params, objs := splitFlowObjects(nf.ObjectCollection())
	out.Parameters = params
	flows := flowsFromGen(nf.FlowsItems())
	if objs != nil || flows != nil {
		out.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: objs, Flows: flows}
	}
	return out
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
	// Flows live on the gen Microflow, but the model keeps them in the object
	// collection (where the DESCRIBE flow-graph traversal reads them). Without
	// the edges, traversal from the start event has nowhere to go and the body
	// renders empty.
	flows := flowsFromGen(mf.FlowsItems())
	if objs != nil || flows != nil {
		out.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: objs, Flows: flows}
	}
	return out
}

// flowsFromGen reconstructs the sequence-flow edges (origin/destination + branch
// case) from a gen flow list, so DESCRIBE can order activities by the flow graph.
func flowsFromGen(items []element.Element) []*microflows.SequenceFlow {
	var flows []*microflows.SequenceFlow
	for _, el := range items {
		g, ok := el.(*genMf.SequenceFlow)
		if !ok {
			continue
		}
		f := &microflows.SequenceFlow{
			OriginID:                   model.ID(g.OriginRefID()),
			DestinationID:              model.ID(g.DestinationRefID()),
			OriginConnectionIndex:      int(g.OriginConnectionIndex()),
			DestinationConnectionIndex: int(g.DestinationConnectionIndex()),
			IsErrorHandler:             g.IsErrorHandler(),
		}
		f.ID = model.ID(g.ID())
		flows = append(flows, f)
	}
	return flows
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
			p := &microflows.MicroflowParameter{Name: po.Name(), Type: dataTypeFromGen(po.ParameterType())}
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
	// Carry the real object ID: the catalog keys activities_data on the activity Id,
	// so bare objects with an empty ID collide (UNIQUE constraint on
	// activities_data.Id) on the second activity of any microflow (full-mode catalog
	// / REFRESH CATALOG FULL). ID is the promoted public field from BaseElement.
	id := model.ID(el.ID())
	switch el.TypeName() {
	case "Microflows$StartEvent":
		o := &microflows.StartEvent{}
		o.ID = id
		return o
	case "Microflows$EndEvent":
		o := &microflows.EndEvent{}
		o.ID = id
		if g, ok := el.(*genMf.EndEvent); ok {
			o.ReturnValue = g.ReturnValue()
		}
		return o
	case "Microflows$ExclusiveMerge":
		o := &microflows.ExclusiveMerge{}
		o.ID = id
		return o
	case "Microflows$ExclusiveSplit":
		o := &microflows.ExclusiveSplit{}
		o.ID = id
		return o
	case "Microflows$InheritanceSplit":
		o := &microflows.InheritanceSplit{}
		o.ID = id
		return o
	case "Microflows$ErrorEvent":
		o := &microflows.ErrorEvent{}
		o.ID = id
		return o
	case "Microflows$LoopedActivity":
		la := &microflows.LoopedActivity{}
		la.ID = id
		if g, ok := el.(*genMf.LoopedActivity); ok {
			if _, objs := splitFlowObjects(g.ObjectCollection()); objs != nil {
				la.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: objs}
			}
		}
		return la
	default:
		o := &microflows.ActionActivity{}
		o.ID = id
		// Reconstruct the action body so DESCRIBE/SHOW can render it. Unhandled
		// action types leave Action nil (renders empty), so coverage grows
		// incrementally without regressing.
		if g, ok := el.(*genMf.ActionActivity); ok {
			if act := g.Action(); act != nil {
				o.Action = actionFromGen(act)
			}
		}
		return o
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
