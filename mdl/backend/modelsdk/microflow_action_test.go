// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// TestMicroflowActionToGen_LogAndVariableActions guards against the CE0008
// ("No action defined") / CE0109 ("Undefined variable") gap: log/declare/set
// actions must convert to a non-nil gen element with the correct storage $Type.
// An unhandled action returns nil → the ActionActivity is written without an
// Action → Studio Pro rejects the microflow.
func TestMicroflowActionToGen_LogAndVariableActions(t *testing.T) {
	cases := []struct {
		name   string
		action microflows.MicroflowAction
		typ    string
	}{
		{"log", &microflows.LogMessageAction{
			LogLevel:        "Info",
			MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "hi"}},
		}, "Microflows$LogMessageAction"},
		{"declare", &microflows.CreateVariableAction{
			VariableName: "X", InitialValue: "0", DataType: &microflows.DecimalType{},
		}, "Microflows$CreateVariableAction"},
		{"set", &microflows.ChangeVariableAction{
			VariableName: "X", Value: "5",
		}, "Microflows$ChangeVariableAction"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := microflowActionToGen(c.action)
			if g == nil {
				t.Fatalf("%s: microflowActionToGen returned nil (would emit an actionless activity → CE0008)", c.name)
			}
			if g.TypeName() != c.typ {
				t.Errorf("%s: $Type = %q, want %q", c.name, g.TypeName(), c.typ)
			}
		})
	}
}
