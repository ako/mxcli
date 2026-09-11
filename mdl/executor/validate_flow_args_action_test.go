// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1082: CE1571 raised on a CONTAINER's action while
// `mxcli check --references` said "Check passed!".
//
// The rule walked GetDataSource() only, so the identical missing-argument fault
// on an ACTION slot was silent. Each test below mirrors one row of the mxbuild
// 11.12.0 measurement in the file header; every "no error" case is paired with a
// control that still reports, so a green run cannot come from the rule having
// been blunted.

// actionWidget is a widget carrying one action slot under the given property.
func slotActionWidget(name, property, flowType, target string, args ...string) *ast.WidgetV3 {
	a := &ast.ActionV3{Type: flowType, Target: target}
	for _, arg := range args {
		a.Args = append(a.Args, ast.FlowArgV3{Name: arg, Value: "$x"})
	}
	return &ast.WidgetV3{
		Name:       name,
		Type:       "container",
		Properties: map[string]any{property: a},
	}
}

// gridWidget is a data grid bound to an entity, with the children given.
func slotGridWidget(name, entity string, children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Name:       name,
		Type:       "datagrid",
		Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "database", Reference: entity}},
		Children:   children,
	}
}

func slotWidget(name, typ string, children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{Name: name, Type: typ, Children: children}
}

// a1082Sigs is one microflow taking one object parameter — the reported shape.
func a1082Sigs() map[string]*flowSignature {
	return map[string]*flowSignature{
		"m.act_unlink": objSig("LogisticWhitelist", "M.LogisticWhitelist"),
		"m.mf_onerow":  {Returns: "M.LogisticWhitelist"},
		"m.mf_other":   {Returns: "M.OtherThing"},
	}
}

// THE REPORTED CASE, reduced: an action with no argument and nothing enclosing
// it. Measured CE1571; the rule was silent.
func TestActionArg_NoContextIsReported(t *testing.T) {
	page := []*ast.WidgetV3{slotActionWidget("cLoose", "Action", "microflow", "M.ACT_UnLink")}
	errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "cLoose") || !strings.Contains(errs[0], "CE1571") {
		t.Errorf("message should name the widget and CE1571: %s", errs[0])
	}
	// The remedy has to be spellable from the message alone — the issue was
	// filed because its author concluded the syntax did not exist.
	if !strings.Contains(errs[0], "Action: microflow M.ACT_UnLink(LogisticWhitelist: $Value)") {
		t.Errorf("message should spell the argument form: %s", errs[0])
	}
}

// An explicit argument clears it — the fix the reporter needed, and proof the
// rule is not simply rejecting every action.
func TestActionArg_ExplicitArgumentIsClean(t *testing.T) {
	page := []*ast.WidgetV3{
		slotActionWidget("cBound", "Action", "microflow", "M.ACT_UnLink", "LogisticWhitelist"),
	}
	if errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName); len(errs) != 0 {
		t.Errorf("an explicitly bound action reported: %v", errs)
	}
}

// Measured: an enclosing data context of the parameter's type supplies an
// action's argument exactly as it supplies a data source's.
func TestActionArg_MatchingEnclosingContextSuppliesIt(t *testing.T) {
	page := []*ast.WidgetV3{
		dvWidget("dvW", mfDS("M.MF_OneRow"),
			slotActionWidget("cM1", "Action", "microflow", "M.ACT_UnLink"),
		),
	}
	if errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName); len(errs) != 0 {
		t.Errorf("mxbuild accepts this page; the check must not reject it: %v", errs)
	}
}

// …and a context of a DIFFERENT type does not. The control for the test above.
func TestActionArg_MismatchedContextIsReported(t *testing.T) {
	page := []*ast.WidgetV3{
		dvWidget("dvO", mfDS("M.MF_Other"),
			slotActionWidget("cM2", "Action", "microflow", "M.ACT_UnLink"),
		),
	}
	errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "M.OtherThing") {
		t.Errorf("message should name the enclosing context type: %s", errs[0])
	}
}

// A page PARAMETER of the exact type is not a data context — the same
// distinction the data-source half rests on, re-measured for the action.
func TestActionArg_PageParameterAloneDoesNotSupplyIt(t *testing.T) {
	page := []*ast.WidgetV3{slotActionWidget("cM3", "Action", "microflow", "M.ACT_UnLink")}
	params := []ast.PageParameter{
		{Name: "W", EntityType: ast.QualifiedName{Module: "M", Name: "LogisticWhitelist"}},
	}
	if errs := validateFlowArgumentsIn(params, page, a1082Sigs(), sameName); len(errs) != 1 {
		t.Fatalf("a page parameter is not a data context (measured: CE1571); got %v", errs)
	}
}

// A data grid COLUMN is row-scoped, so the grid's object supplies the argument.
func TestActionArg_GridColumnIsRowScoped(t *testing.T) {
	page := []*ast.WidgetV3{
		slotGridWidget("dgM4", "M.LogisticWhitelist",
			slotWidget("colM4", "column",
				slotActionWidget("cM4", "Action", "microflow", "M.ACT_UnLink"),
			),
		),
	}
	if errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName); len(errs) != 0 {
		t.Errorf("a column is row-scoped (measured: no error); got %v", errs)
	}
}

// …and a CONTROL BAR is not. This is the reported shape, and the pair with the
// column test above is the whole point: the two differ only in which child of
// the same grid the widget sits in.
func TestActionArg_ControlBarIsNotRowScoped(t *testing.T) {
	page := []*ast.WidgetV3{
		slotGridWidget("dgMaterials", "M.LogisticWhitelist",
			slotWidget("cb1", "controlbar",
				slotActionWidget("cM6", "Action", "microflow", "M.ACT_UnLink"),
			),
		),
	}
	errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName)
	if len(errs) != 1 {
		t.Fatalf("a control bar is not row-scoped (measured: CE1571); got %v", errs)
	}
	// The remedy differs from every other context miss, so the advice must too:
	// nesting does not help here, the grid's selection does.
	if !strings.Contains(errs[0], "$dgMaterials") {
		t.Errorf("message should offer the grid's selection: %s", errs[0])
	}
	if strings.Contains(errs[0], "nest the widget in a data container") {
		t.Errorf("nesting is not the remedy inside a control bar: %s", errs[0])
	}
}

// The control bar drops only the grid's OWN object. Measured: an outer context
// still reaches it (dataview > datagrid > controlbar > action → no error), so
// the walk must not hand the control bar an empty context.
func TestActionArg_OuterContextStillReachesTheControlBar(t *testing.T) {
	page := []*ast.WidgetV3{
		dvWidget("dvM9", mfDS("M.MF_OneRow"),
			slotGridWidget("dgM9", "M.LogisticWhitelist",
				slotWidget("cbM9", "controlbar",
					slotActionWidget("cM9", "Action", "microflow", "M.ACT_UnLink"),
				),
			),
		),
	}
	if errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName); len(errs) != 0 {
		t.Errorf("an outer context reaches into a control bar (measured: no error); got %v", errs)
	}
}

// A data container INSIDE a control bar ends the control bar's reach: its own
// object is in scope for its children like anywhere else.
func TestActionArg_DataContainerInsideAControlBarRestoresScope(t *testing.T) {
	page := []*ast.WidgetV3{
		slotGridWidget("dgInner", "M.LogisticWhitelist",
			slotWidget("cbInner", "controlbar",
				dvWidget("dvInner", mfDS("M.MF_OneRow"),
					slotActionWidget("cInner", "Action", "microflow", "M.ACT_UnLink"),
				),
			),
		),
	}
	if errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName); len(errs) != 0 {
		t.Errorf("a dataview inside a control bar establishes context: %v", errs)
	}
}

// The control-bar rule is in the shared WALK, not in either rule, so a DATA
// SOURCE placed in a control bar is reported too. Measured on the same project:
// the same dataview is CE1571 in the control bar and clean in a column.
func TestDataSourceArg_ControlBarIsNotRowScoped(t *testing.T) {
	sigs := map[string]*flowSignature{"m.mf_needsone": objSig("LogisticWhitelist", "M.LogisticWhitelist")}
	inControlBar := []*ast.WidgetV3{
		slotGridWidget("dgM7", "M.LogisticWhitelist",
			slotWidget("cbM7", "controlbar",
				dvWidget("dvM7", mfDS("M.MF_NeedsOne")),
			),
		),
	}
	if errs := validateFlowArgumentsIn(nil, inControlBar, sigs, sameName); len(errs) != 1 {
		t.Fatalf("a data source in a control bar is CE1571 (measured); got %v", errs)
	}
	// CONTROL: the identical data source in a column is accepted.
	inColumn := []*ast.WidgetV3{
		slotGridWidget("dgM8", "M.LogisticWhitelist",
			slotWidget("colM8", "column",
				dvWidget("dvM8", mfDS("M.MF_NeedsOne")),
			),
		),
	}
	if errs := validateFlowArgumentsIn(nil, inColumn, sigs, sameName); len(errs) != 0 {
		t.Errorf("the control failed: a column is row-scoped; got %v", errs)
	}
}

// Every action slot is covered, not just the reported one. `OnClick:` is stored
// under "Action" by the visitor, so the distinct keys worth proving are
// OnChange and a pluggable widget's named slot.
func TestActionArg_EveryActionSlotIsChecked(t *testing.T) {
	for _, property := range []string{"Action", "OnChange", "createFileAction"} {
		t.Run(property, func(t *testing.T) {
			page := []*ast.WidgetV3{slotActionWidget("w", property, "nanoflow", "M.ACT_UnLink")}
			errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName)
			if len(errs) != 1 {
				t.Fatalf("slot %s not checked; got %v", property, errs)
			}
			if !strings.Contains(errs[0], "("+property+":)") {
				t.Errorf("message should name the slot it is about: %s", errs[0])
			}
		})
	}
}

// A widget with two faulty slots reports both, in a stable order — the
// Properties map is iterated, and a map is not ordered.
func TestActionArg_TwoFaultySlotsReportDeterministically(t *testing.T) {
	w := slotActionWidget("wBoth", "Action", "microflow", "M.ACT_UnLink")
	w.Properties["OnChange"] = &ast.ActionV3{Type: "microflow", Target: "M.ACT_UnLink"}
	page := []*ast.WidgetV3{w}

	first := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName)
	if len(first) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(first), first)
	}
	for i := 0; i < 20; i++ {
		again := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName)
		if strings.Join(again, "|") != strings.Join(first, "|") {
			t.Fatalf("order is not stable across runs:\n%v\n%v", first, again)
		}
	}
	if !strings.Contains(first[0], "(Action:)") {
		t.Errorf("sorted by property key, Action comes first: %v", first)
	}
}

// The unknown-argument half applies to actions too: an argument naming no
// parameter binds nothing and is silent otherwise.
func TestActionArg_UnknownArgumentIsReported(t *testing.T) {
	page := []*ast.WidgetV3{
		slotActionWidget("cTypo", "Action", "microflow", "M.ACT_UnLink", "LogisticWhitelist", "Whitelst"),
	}
	errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName)
	if len(errs) != 1 || !strings.Contains(errs[0], "'Whitelst'") {
		t.Fatalf("the unknown-argument half must cover actions; got %v", errs)
	}
}

// A non-flow action carries no parameters to miss, and an unresolved flow is
// validateWidgetReferences' business — neither may be reported here.
func TestActionArg_NonFlowAndUnknownFlowAreSilent(t *testing.T) {
	page := []*ast.WidgetV3{
		{Name: "cSave", Type: "actionbutton", Properties: map[string]any{
			"Action": &ast.ActionV3{Type: "save", ClosePage: true},
		}},
		{Name: "cGone", Type: "actionbutton", Properties: map[string]any{
			"Action": &ast.ActionV3{Type: "microflow", Target: "M.DoesNotExist"},
		}},
	}
	if errs := validateFlowArgumentsIn(nil, page, a1082Sigs(), sameName); len(errs) != 0 {
		t.Errorf("neither a non-flow action nor an unresolved one belongs here: %v", errs)
	}
}
