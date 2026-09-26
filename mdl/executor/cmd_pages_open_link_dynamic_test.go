// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `open_link $currentObject/Attr` — an address read from an attribute — builds
// a LinkClientAction bound to that attribute, qualified against the enclosing
// data container's entity.
func TestBuildClientActionV3_OpenLinkDynamicAddress(t *testing.T) {
	pb := &pageBuilder{entityContext: "FeedbackModule.ResponseHelper"}
	got, err := pb.buildClientActionV3(&ast.ActionV3{Type: "openLink", LinkVariable: "$currentObject", LinkAttribute: "URL"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	link, ok := got.(*pages.LinkClientAction)
	if !ok {
		t.Fatalf("got %T, want *pages.LinkClientAction", got)
	}
	if link.AddressAttribute != "FeedbackModule.ResponseHelper.URL" {
		t.Errorf("AddressAttribute = %q, want FeedbackModule.ResponseHelper.URL", link.AddressAttribute)
	}
	if link.Address != "" {
		t.Errorf("Address = %q, want empty", link.Address)
	}
}

// Outside a data container there is no object to read the address from, and
// only $currentObject is supported — refuse rather than write a dangling ref.
func TestBuildClientActionV3_OpenLinkDynamicAddressRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		pb   *pageBuilder
		act  *ast.ActionV3
		want string
	}{
		{"no entity context", &pageBuilder{}, &ast.ActionV3{Type: "openLink", LinkVariable: "$currentObject", LinkAttribute: "URL"}, "data container"},
		{"other variable", &pageBuilder{entityContext: "M.E"}, &ast.ActionV3{Type: "openLink", LinkVariable: "$Param", LinkAttribute: "URL"}, "$currentObject"},
		{"association path", &pageBuilder{entityContext: "M.E"}, &ast.ActionV3{Type: "openLink", LinkVariable: "$currentObject", LinkAttribute: "E_Other/URL"}, "association"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.pb.buildClientActionV3(tc.act)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// DESCRIBE renders the stored dynamic address as the syntax above instead of
// an inline `--` note, which left `Action:` without a value and made the
// output unparseable (5 of 17 pages of a stock Feedback-module project).
func TestRenderClientActionMDL_OpenLinkDynamicAddress(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	action := map[string]any{
		"$Type": "Forms$OpenLinkClientAction",
		"Address": map[string]any{
			"$Type":     "Forms$StaticOrDynamicString",
			"IsDynamic": true,
			"Value":     "",
			"AttributeRef": map[string]any{
				"$Type":     "DomainModels$AttributeRef",
				"Attribute": "FeedbackModule.ResponseHelper.URL",
				"EntityRef": nil,
			},
		},
		"LinkType": "Web",
	}
	if got, want := renderClientActionMDL(ctx, action), "open_link $currentObject/URL"; got != want {
		t.Errorf("renderClientActionMDL = %q, want %q", got, want)
	}

	// Over an association MDL still has no spelling: a standalone note,
	// never an inline `Action: -- …`.
	action["Address"].(map[string]any)["AttributeRef"].(map[string]any)["EntityRef"] = map[string]any{
		"$Type": "DomainModels$IndirectEntityRef",
		"Steps": []any{int32(2), map[string]any{"Association": "M.E_Other", "DestinationEntity": "M.Other"}},
	}
	got := renderClientActionMDL(ctx, action)
	if !strings.HasPrefix(got, "-- ") || actionProp("Action", got) != got {
		t.Errorf("association-path address rendered %q, want a standalone -- note", got)
	}
}
