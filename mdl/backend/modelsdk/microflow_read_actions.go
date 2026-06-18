// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// actionFromGen reconstructs the semantic microflow action from an ActionActivity's
// gen Action child, so DESCRIBE/SHOW can render the activity body (the inverse of
// microflowActionToGen). Returns nil for action types not yet reconstructed — the
// activity then renders as an empty action, which is the prior behaviour, so this
// grows incrementally batch-by-batch without regressing already-handled types.
//
// Actions written via gen setters (this batch) read back through the gen
// accessors; the raw-built actions (list-ops, REST, …) will read their explicit
// keys in a later batch.
func actionFromGen(el element.Element) microflows.MicroflowAction {
	switch a := el.(type) {
	case *genMf.LogMessageAction:
		out := &microflows.LogMessageAction{
			ErrorHandlingType:     microflows.ErrorHandlingType(a.ErrorHandlingType()),
			LogLevel:              microflows.LogLevel(a.Level()),
			LogNodeName:           a.Node(),
			IncludeLastStackTrace: a.IncludeLatestStackTrace(),
		}
		out.ID = model.ID(a.ID())
		// MessageTemplate is a Microflows$StringTemplate (scalar Text + Arguments).
		if st, ok := a.MessageTemplate().(*genMf.StringTemplate); ok && st != nil {
			out.MessageTemplate = &model.Text{Translations: map[string]string{"en_US": st.Text()}}
			for _, argEl := range st.ArgumentsItems() {
				if arg, ok := argEl.(*genMf.TemplateArgument); ok {
					out.TemplateParameters = append(out.TemplateParameters, arg.Expression())
				}
			}
		}
		return out

	case *genMf.CreateVariableAction:
		out := &microflows.CreateVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			VariableName:      a.VariableName(),
			InitialValue:      a.InitialValue(),
			DataType:          dataTypeFromGen(a.VariableType()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ChangeVariableAction:
		out := &microflows.ChangeVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			VariableName:      a.ChangeVariableName(),
			Value:             a.Value(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CreateObjectAction:
		out := &microflows.CreateObjectAction{
			ErrorHandlingType:   microflows.ErrorHandlingType(a.ErrorHandlingType()),
			EntityQualifiedName: a.EntityQualifiedName(),
			OutputVariable:      a.OutputVariableName(),
			Commit:              microflows.CommitType(a.Commit()),
			InitialMembers:      memberChangesFromGen(a.ItemsItems()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ChangeObjectAction:
		out := &microflows.ChangeObjectAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			ChangeVariable:    a.ChangeVariableName(),
			Commit:            microflows.CommitType(a.Commit()),
			RefreshInClient:   a.RefreshInClient(),
			Changes:           memberChangesFromGen(a.ItemsItems()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CommitAction:
		out := &microflows.CommitObjectsAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			CommitVariable:    a.CommitVariableName(),
			WithEvents:        a.WithEvents(),
			RefreshInClient:   a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.DeleteAction:
		out := &microflows.DeleteObjectAction{
			DeleteVariable:  a.DeleteVariableName(),
			RefreshInClient: a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.RollbackAction:
		out := &microflows.RollbackObjectAction{
			RollbackVariable: a.RollbackVariableName(),
			RefreshInClient:  a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.RetrieveAction:
		out := &microflows.RetrieveAction{
			OutputVariable: a.OutputVariableName(),
			Source:         retrieveSourceFromGen(a.RetrieveSource()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.MicroflowCallAction:
		out := &microflows.MicroflowCallAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(a.ErrorHandlingType()),
			ResultVariableName: a.OutputVariableName(),
			UseReturnVariable:  a.UseReturnVariable(),
		}
		out.ID = model.ID(a.ID())
		if mc, ok := a.MicroflowCall().(*genMf.MicroflowCall); ok && mc != nil {
			call := &microflows.MicroflowCall{Microflow: mc.MicroflowQualifiedName()}
			call.ID = model.ID(mc.ID())
			for _, pmEl := range mc.ParameterMappingsItems() {
				if pm, ok := pmEl.(*genMf.MicroflowCallParameterMapping); ok {
					m := &microflows.MicroflowCallParameterMapping{Parameter: pm.ParameterQualifiedName(), Argument: pm.Argument()}
					m.ID = model.ID(pm.ID())
					call.ParameterMappings = append(call.ParameterMappings, m)
				}
			}
			out.MicroflowCall = call
		}
		return out

	case *genMf.NanoflowCallAction:
		out := &microflows.NanoflowCallAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(a.ErrorHandlingType()),
			OutputVariableName: a.OutputVariableName(),
			UseReturnVariable:  a.UseReturnVariable(),
		}
		out.ID = model.ID(a.ID())
		if nc, ok := a.NanoflowCall().(*genMf.NanoflowCall); ok && nc != nil {
			call := &microflows.NanoflowCall{Nanoflow: nc.NanoflowQualifiedName()}
			call.ID = model.ID(nc.ID())
			for _, pmEl := range nc.ParameterMappingsItems() {
				if pm, ok := pmEl.(*genMf.NanoflowCallParameterMapping); ok {
					m := &microflows.NanoflowCallParameterMapping{Parameter: pm.ParameterQualifiedName(), Argument: pm.Argument()}
					m.ID = model.ID(pm.ID())
					call.ParameterMappings = append(call.ParameterMappings, m)
				}
			}
			out.NanoflowCall = call
		}
		return out

	default:
		return nil
	}
}

// memberChangesFromGen reconstructs the attribute/association assignments of a
// create/change-object action (the inverse of memberChangeToGen).
func memberChangesFromGen(items []element.Element) []*microflows.MemberChange {
	var out []*microflows.MemberChange
	for _, el := range items {
		g, ok := el.(*genMf.MemberChange)
		if !ok {
			continue
		}
		m := &microflows.MemberChange{
			AttributeQualifiedName:   g.AttributeQualifiedName(),
			AssociationQualifiedName: g.AssociationQualifiedName(),
			Type:                     microflows.MemberChangeType(g.Type()),
			Value:                    g.Value(),
		}
		m.ID = model.ID(g.ID())
		out = append(out, m)
	}
	return out
}

// retrieveSourceFromGen reconstructs a retrieve's source (database with XPath/
// range, or association navigation). Inverse of retrieveSourceToGen.
func retrieveSourceFromGen(el element.Element) microflows.RetrieveSource {
	switch g := el.(type) {
	case *genMf.DatabaseRetrieveSource:
		s := &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: g.EntityQualifiedName(),
			XPathConstraint:     g.XPathConstraint(),
			Range:               rangeFromGen(g.Range()),
		}
		s.ID = model.ID(g.ID())
		return s
	case *genMf.AssociationRetrieveSource:
		s := &microflows.AssociationRetrieveSource{
			StartVariable:            g.StartVariableName(),
			AssociationQualifiedName: g.AssociationQualifiedName(),
		}
		s.ID = model.ID(g.ID())
		return s
	default:
		return nil
	}
}

// rangeFromGen maps a gen range element to the model Range. A ConstantRange with
// SingleObject means "first" (limit 1); SingleObject=false has no range. A
// CustomRange carries limit/offset expressions.
func rangeFromGen(el element.Element) *microflows.Range {
	switch g := el.(type) {
	case *genMf.ConstantRange:
		if g.SingleObject() {
			return &microflows.Range{RangeType: microflows.RangeTypeFirst}
		}
		return nil
	case *genMf.CustomRange:
		return &microflows.Range{RangeType: microflows.RangeTypeCustom, Limit: g.LimitExpression(), Offset: g.OffsetExpression()}
	default:
		return nil
	}
}
