// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestFlowArgsToParameterMappings guards issue #835.
//
// A microflow used as a widget datasource needs an argument for every parameter,
// exactly as a call action does. The grammar parsed `microflow Mod.MF(Name: $x)`
// into DataSourceV3.Args, but the builder never read them and MicroflowSource had
// nowhere to put them, so the binding was silently dropped and mxbuild reported
//
//	[error] [CE1571] "No argument has been selected for parameter 'Name' and no
//	default is available." at Data grid 2 'dg1'
//
// with mxcli check and exec both reporting success.
func TestFlowArgsToParameterMappings(t *testing.T) {
	// An empty builder knows no parameters, so every $-name is unclassifiable
	// and the pre-#1140 behaviour applies unchanged — which is the point.
	pb := &pageBuilder{}
	got := pb.flowArgsToParameterMappings([]ast.FlowArgV3{
		{Name: "Name", Value: "$Filter"},
		{Name: "Limit", Value: "10"},
		{Name: "Ctx", Value: "$currentObject"},
	})
	if len(got) != 3 {
		t.Fatalf("got %d mappings, want 3", len(got))
	}

	// A leading $ is a variable reference; anything else is an expression. The
	// distinction matters: Mendix binds the two through different BSON fields.
	if got[0].ParameterName != "Name" || got[0].Variable != "$Filter" || got[0].Expression != "" {
		t.Errorf("$-value mapping = %+v, want Variable=$Filter", got[0])
	}
	if got[1].ParameterName != "Limit" || got[1].Expression != "10" || got[1].Variable != "" {
		t.Errorf("literal mapping = %+v, want Expression=10", got[1])
	}
	if got[2].Variable != "$currentObject" {
		t.Errorf("$currentObject mapping = %+v, want Variable=$currentObject", got[2])
	}
	for i, m := range got {
		if m.ID == "" {
			t.Errorf("mapping %d has no ID — every model element needs one", i)
		}
	}
}

// No arguments must stay nil rather than an empty slice, so a datasource without
// parameters serializes exactly as it did before this change.
func TestFlowArgsToParameterMappings_Empty(t *testing.T) {
	pb := &pageBuilder{}
	if got := pb.flowArgsToParameterMappings(nil); got != nil {
		t.Errorf("no args should yield nil, got %+v", got)
	}
}
