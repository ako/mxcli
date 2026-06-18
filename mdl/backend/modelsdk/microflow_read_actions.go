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

	default:
		return nil
	}
}
