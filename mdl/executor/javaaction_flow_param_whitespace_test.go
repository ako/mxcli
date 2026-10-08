// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1282: a Microflow-typed argument followed by a line break
// before the closing parenthesis. The visitor keeps that whitespace on the
// argument (a SourceExpr, so an expression argument round-trips as written),
// and the Microflow-typed path read the wrapper rather than the name inside
// it: 0.24 stored "MyModule.SUB_Target\n" (CE1613 "The selected microflow
// 'MyModule.SUB_Target⏎' no longer exists"), and once #1210's guard landed
// the same wrapper was refused as "not a microflow name". The arguments are
// parsed from MDL, not hand-built, because only the parser produces the
// wrapper — a test that constructs a QualifiedNameExpr cannot see this bug.
func TestBuildJavaAction_MicroflowArgumentIgnoresTrailingLineBreak(t *testing.T) {
	cases := []struct {
		name string
		args string
		want string
	}{
		// Control: the reporter's script with `)` on the same line.
		{"same line", `Flow = MyModule.SUB_Target)`, "MyModule.SUB_Target"},
		{"line break before )", "Flow = MyModule.SUB_Target\n  )", "MyModule.SUB_Target"},
		{"quoted name, line break", "Flow = 'MyModule.SUB_Target'\n  )", "MyModule.SUB_Target"},
		{"space and tab before )", "Flow = MyModule.SUB_Target \t)", "MyModule.SUB_Target"},
		{"empty, line break", "Flow = empty\n  )", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow MyModule.ACT_Caller () returns Nothing\nbegin\n" +
				"  $ok = call java action MyModule.DoSomething(" + tc.args + ";\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs[0])
			}
			stmt := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.CallJavaActionStmt)

			fb := &flowBuilder{
				posX: 100, posY: 100, spacing: HorizontalSpacing,
				backend: &mock.MockBackend{
					ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) {
						return &javaactions.JavaAction{
							Parameters: []*javaactions.JavaActionParameter{{
								Name:          "Flow",
								ParameterType: &javaactions.MicroflowType{BaseElement: model.BaseElement{ID: "t"}},
							}},
						}, nil
					},
				},
			}
			id := fb.addCallJavaActionAction(stmt)
			if len(fb.errors) > 0 {
				t.Fatalf("builder refused the argument: %v", fb.errors)
			}
			var action *microflows.JavaActionCallAction
			for _, obj := range fb.objects {
				if obj.GetID() == id {
					action = obj.(*microflows.ActionActivity).Action.(*microflows.JavaActionCallAction)
				}
			}
			if action == nil {
				t.Fatal("expected Java action activity")
			}
			value, ok := action.ParameterMappings[0].Value.(*microflows.MicroflowParameterValue)
			if !ok {
				t.Fatalf("mapping value = %T, want *MicroflowParameterValue", action.ParameterMappings[0].Value)
			}
			if value.Microflow != tc.want {
				t.Fatalf("stored microflow = %q, want %q", value.Microflow, tc.want)
			}
		})
	}
}
