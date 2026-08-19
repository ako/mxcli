// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ValidateExportMappingMembers is what a plain `mxcli check` calls. It has to
// fire WITHOUT a project — the answer is in the statement — which is also what
// lets the negative fixture be a .fail.mdl at all: `make check-mdl` runs check
// with no -p, and a guard that only fires at exec reports "negative test
// unexpectedly passed" instead. That is how this rule came to exist (#927).
func TestValidateExportMappingMembers_ReportsNestedMemberWithoutProject(t *testing.T) {
	prog, errs := visitor.Build(`create export mapping M.EMM_Order
  with json structure M.JSON_Order
{
  M.Order {
    orderId       = OrderId,
    customer/name = CustomerName
  }
};`)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}

	got := ValidateExportMappingMembers(prog)
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %#v", len(got), got)
	}
	if got[0].RuleID != "MDL-MAP01" {
		t.Errorf("RuleID = %q, want MDL-MAP01", got[0].RuleID)
	}
	if !strings.Contains(got[0].Message, "CE5015") {
		t.Errorf("message should name the build error it prevents, got %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "customer/name") {
		t.Errorf("message should quote the offending member, got %q", got[0].Message)
	}
	if !strings.Contains(got[0].Suggestion, "customer") {
		t.Errorf("suggestion should name the level needing its own element, got %q", got[0].Suggestion)
	}
}

// The controls. A top-level member is the ordinary case; an object element's
// `Assoc/Entity` is a `/` on the OTHER side of the mapping and must not be
// mistaken for a nested member — that pair is what the rule has to tell apart.
func TestValidateExportMappingMembers_CleanCases(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "top-level members only",
			src: `create export mapping M.EMM_A with json structure M.JS
{ M.Order { orderId = OrderId, total = Total } };`,
		},
		{
			name: "an association element is not a nested member",
			src: `create export mapping M.EMM_B with json structure M.JS
{ M.Order { orderId = OrderId, M.Order_Line/M.Line as lines { sku = Sku } } };`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := visitor.Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			if got := ValidateExportMappingMembers(prog); len(got) != 0 {
				t.Errorf("MDL-MAP01 fired on a valid export mapping: %#v", got)
			}
		})
	}
}

// An IMPORT mapping may collapse levels, so the rule must not touch it. Getting
// this wrong would refuse the very feature #927 adds.
func TestValidateExportMappingMembers_IgnoresImportMappings(t *testing.T) {
	prog, errs := visitor.Build(`create import mapping M.IMM_Order with json structure M.JS
{ create M.Order { OrderId = orderId, CustomerName = customer/name } };`)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	if got := ValidateExportMappingMembers(prog); len(got) != 0 {
		t.Errorf("MDL-MAP01 fired on an IMPORT mapping, where collapsing is supported: %#v", got)
	}
}
