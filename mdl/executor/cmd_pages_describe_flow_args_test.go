// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// The read half of mendixlabs/mxcli#1140.
//
// Studio Pro binds an object-typed flow argument through the mapping's Variable —
// a Forms$PageVariable naming a page parameter, snippet parameter or page
// variable — not through Expression. The three action readers and the data-source
// reader all looked for a `Name` key on that sub-document, which
// Forms$PageVariable does not have, so every argument in Studio Pro-authored
// content was described as absent.
//
// Measured before the fix, on Workflow Commons 4.11.0 (Studio Pro-authored):
//
//	Action: microflow WorkflowCommons.ACT_ConflictedWorkflowHelper_ApplyJumpTo
//
// for a button whose stored mappings bind $ConflictedWorkflowHelper and
// $ConflictedWorkflowDefinitionView. Replaying that description drops both
// arguments — the #835 loss in a different storage form.
func TestPageVariableArgValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  any
		want string
	}{
		{
			"page parameter",
			map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "ConflictedWorkflowHelper"},
			"$ConflictedWorkflowHelper",
		},
		{
			"snippet parameter",
			map[string]any{"$Type": "Forms$PageVariable", "SnippetParameter": "AuditTrailViewer"},
			"$AuditTrailViewer",
		},
		{
			"page variable",
			map[string]any{"$Type": "Forms$PageVariable", "LocalVariable": "showStock"},
			"$showStock",
		},
		{
			// Studio Pro writes every slot, the unused ones empty — so an empty
			// string must not be mistaken for a binding.
			"all slots present, one filled",
			map[string]any{
				"$Type": "Forms$PageVariable", "LocalVariable": "", "PageParameter": "Workflow",
				"SnippetParameter": "", "SubKey": "", "UseAllPages": false, "Widget": "",
			},
			"$Workflow",
		},
		{
			// A grid's selection. MDL has no syntax for it, and rendering it as
			// $dataGrid23 would re-execute into a different binding.
			"widget selection is not rendered",
			map[string]any{"$Type": "Forms$PageVariable", "Widget": "dataGrid23"},
			"",
		},
		{"no variable", nil, ""},
		{"empty variable", map[string]any{"$Type": "Forms$PageVariable"}, ""},
		{"not a document", "$Workflow", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pageVariableArgValue(tc.raw); got != tc.want {
				t.Errorf("pageVariableArgValue = %q, want %q", got, tc.want)
			}
		})
	}
}

// A data source stores its arguments the same way, so the same loss applied
// there. The Expression form must keep working — a pre-#1140 document, and every
// literal argument, is written that way.
func TestDataSourceArgsReadPageVariableBinding(t *testing.T) {
	for _, tc := range []struct {
		name string
		arg  map[string]any
		want string
	}{
		{
			"variable binding",
			map[string]any{
				"Parameter": "Mod.DS.Order",
				"Variable":  map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "Order"},
			},
			"microflow Mod.DS(Order: $Order)",
		},
		{
			"expression binding still read",
			map[string]any{"Parameter": "Mod.DS.Order", "Expression": "$Order"},
			"microflow Mod.DS(Order: $Order)",
		},
		{
			"bare string in Variable still read",
			map[string]any{"Parameter": "Mod.DS.Order", "Variable": "$Order"},
			"microflow Mod.DS(Order: $Order)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := map[string]any{
				"$Type": "Forms$MicroflowSource",
				"MicroflowSettings": map[string]any{
					"Microflow":         "Mod.DS",
					"ParameterMappings": []any{int32(3), tc.arg},
				},
			}
			got := parseDataSource(ds)
			if got == nil {
				t.Fatal("datasource not read")
			}
			if expr := dataSourceExpr(got); expr != tc.want {
				t.Errorf("rendered %q, want %q", expr, tc.want)
			}
		})
	}
}
