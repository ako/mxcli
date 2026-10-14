// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// Bug 4 — DESCRIBE must reconstruct a Forms$AssociationSource (EntityRef =
// IndirectEntityRef of steps) back into `$currentObject/Module.Assoc` (or
// `$Param/…`) so the round-trip re-parses.
func TestAssociationSourcePath(t *testing.T) {
	step := func(assoc string) map[string]any {
		return map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": assoc, "DestinationEntity": "M.Dest"}
	}
	src := func(sourceVar string, steps ...map[string]any) map[string]any {
		items := make([]any, len(steps))
		for i, s := range steps {
			items[i] = s
		}
		ds := map[string]any{
			"$Type":     "Forms$AssociationSource",
			"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": items},
		}
		if sourceVar != "" {
			ds["SourceVariable"] = map[string]any{"$Type": "Forms$PageVariable", "PageParameter": sourceVar}
		}
		return ds
	}

	tests := []struct {
		name     string
		ds       map[string]any
		wantPath string
		wantCtx  string
	}{
		{"currentObject single hop", src("", step("M.Order_Customer")), "M.Order_Customer", "currentObject"},
		{"page parameter", src("Order", step("M.Order_Lines")), "M.Order_Lines", "Order"},
		{"two hops print each destination", src("", step("M.A_B"), step("M.B_C")), "M.A_B/M.Dest/M.B_C/M.Dest", "currentObject"},
		{"no EntityRef → empty", map[string]any{"$Type": "Forms$AssociationSource"}, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, ctx := associationSourcePath(tc.ds)
			if path != tc.wantPath || ctx != tc.wantCtx {
				t.Errorf("associationSourcePath = (%q, %q), want (%q, %q)", path, ctx, tc.wantPath, tc.wantCtx)
			}
		})
	}
}

// ako/mxcli#721 L3. Inside a snippet the context variable is a snippet
// parameter, stored in the SnippetParameter slot. Describe read PageParameter
// alone and printed `$currentObject/…`, so executing its output dropped the
// binding (TestApp's WorkflowCommons.Snip_UserTask_TaskTimeline).
func TestAssociationSourcePath_SnippetParameter(t *testing.T) {
	ds := map[string]any{
		"$Type": "Forms$DataViewSource",
		"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": []any{
			map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": "System.WorkflowUserTask_Workflow", "DestinationEntity": "System.Workflow"},
		}},
		"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "", "SnippetParameter": "WorkflowUserTask"},
	}
	path, ctx := associationSourcePath(ds)
	if path != "System.WorkflowUserTask_Workflow" || ctx != "WorkflowUserTask" {
		t.Errorf("associationSourcePath = (%q, %q), want (System.WorkflowUserTask_Workflow, WorkflowUserTask)", path, ctx)
	}
}
