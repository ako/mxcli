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
		{"two hops", src("", step("M.A_B"), step("M.B_C")), "M.A_B/M.B_C", "currentObject"},
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
