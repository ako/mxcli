// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#529 (read half) — DESCRIBE PAGE silently DROPS the association hops
// of an input widget bound to an attribute over an association.
//
// Measured on ako/TestApp, page Rules.RuleAction_NewEdit, whose textBox4 stores:
//
//	DomainModels$AttributeRef
//	  Attribute: "Rules.BusinessRule.Name"
//	  EntityRef: DomainModels$IndirectEntityRef
//	    Steps: [ EntityRefStep{ Association: "Rules.RuleAction_BusinessRule",
//	                            DestinationEntity: "Rules.BusinessRule" } ]
//
// DESCRIBE emitted `Attribute: Name` — and Rules.RuleAction has NO attribute
// called Name, so re-running that output rebinds the widget to something that
// does not exist. A describe → exec round trip over a Studio Pro page therefore
// BREAKS it, with `check` clean, and the damage only shows up at build time as
// CE1613 or in a browser as a blank field.
//
// DataGrid2 columns already got this right (columnAttributeFromRef, bug 7), so
// the same page could round-trip a grid column and destroy a text box.
package executor

import "testing"

func TestExtractAttributeRef_KeepsAssociationHops(t *testing.T) {
	step := func(assoc, dest string) map[string]any {
		return map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": assoc, "DestinationEntity": dest}
	}
	widget := func(attr string, steps ...map[string]any) map[string]any {
		ref := map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": attr}
		if len(steps) > 0 {
			items := make([]any, len(steps))
			for i, s := range steps {
				items[i] = s
			}
			ref["EntityRef"] = map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": items}
		}
		return map[string]any{"AttributeRef": ref}
	}

	tests := []struct {
		name string
		w    map[string]any
		want string
	}{
		{
			// The control: a plain binding must stay bare. The enclosing
			// dataview establishes the entity, so a qualified name here would
			// not re-parse.
			name: "own attribute stays bare",
			w:    widget("Rules.RuleAction.ActionType"),
			want: "ActionType",
		},
		{
			// The reported case, verbatim from TestApp.
			name: "single hop keeps the association",
			w:    widget("Rules.BusinessRule.Name", step("Rules.RuleAction_BusinessRule", "Rules.BusinessRule")),
			want: "RuleAction_BusinessRule/Name",
		},
		{
			name: "two hops",
			w: widget("A.Country.Code",
				step("A.Order_Customer", "A.Customer"),
				step("A.Customer_Country", "A.Country")),
			want: "Order_Customer/Customer_Country/Code",
		},
		{
			// A malformed EntityRef must fall back to the bare attribute rather
			// than emitting a half-built path — broken MDL is worse than a
			// binding the reader can see is incomplete.
			name: "step with no association falls back",
			w: widget("A.Country.Code",
				map[string]any{"$Type": "DomainModels$EntityRefStep", "DestinationEntity": "A.Country"}),
			want: "Code",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractAttributeRef(nil, tc.w); got != tc.want {
				t.Errorf("extractAttributeRef = %q, want %q", got, tc.want)
			}
		})
	}
}
