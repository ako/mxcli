// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance-2: `datasource: microflow M.F` on a microflow WITH
// parameters built to CE1571 while `mxcli check --references` passed. The
// microflow resolves fine — the missing argument was nobody's check.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func dsWidget(name, flow string, args ...string) *ast.WidgetV3 {
	ds := &ast.DataSourceV3{Type: "microflow", Reference: flow}
	for _, a := range args {
		ds.Args = append(ds.Args, ast.FlowArgV3{Name: a, Value: "$x"})
	}
	return &ast.WidgetV3{
		Name:       name,
		Properties: map[string]any{"DataSource": ds},
	}
}

func TestDataSourceMissingArgumentIsReported(t *testing.T) {
	params := map[string][]string{"ds.mf": {"Task"}}
	got := dataSourceArgErrors("dgBad", dsWidget("dgBad", "DS.MF").GetDataSource(), params)
	if len(got) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(got), got)
	}
	for _, want := range []string{"dgBad", "'Task'", "CE1571"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("message lacks %q:\n%s", want, got[0])
		}
	}
	// The message must show the fix, not just the fault.
	if !strings.Contains(got[0], "DS.MF(Task:") {
		t.Errorf("message does not show the corrected form:\n%s", got[0])
	}
}

func TestDataSourceWithItsArgumentIsClean(t *testing.T) {
	// The control that makes the rule worth having: the correct form must pass, or
	// the check simply forbids microflow data sources.
	params := map[string][]string{"ds.mf": {"Task"}}
	if got := dataSourceArgErrors("dgGood", dsWidget("dgGood", "DS.MF", "Task").GetDataSource(), params); len(got) != 0 {
		t.Errorf("correct arguments reported: %v", got)
	}
	// Mendix resolves names case-insensitively, and so must this.
	if got := dataSourceArgErrors("dgGood", dsWidget("dgGood", "ds.MF", "task").GetDataSource(), params); len(got) != 0 {
		t.Errorf("case difference reported: %v", got)
	}
}

func TestParameterlessDataSourceNeedsNoParens(t *testing.T) {
	// The most common data source in any app. A rule that flagged this would fire
	// on nearly every page and be turned off.
	params := map[string][]string{"ds.all": {}}
	if got := dataSourceArgErrors("dgA", dsWidget("dgA", "DS.All").GetDataSource(), params); len(got) != 0 {
		t.Errorf("parameterless microflow reported: %v", got)
	}
}

func TestUnknownFlowIsLeftToTheReferenceCheck(t *testing.T) {
	// "not found" is validateWidgetReferences' job. Reporting it here too would
	// double every typo, and worse, a typo'd flow name would be reported as a
	// missing ARGUMENT — which sends the reader to the wrong line.
	params := map[string][]string{"ds.mf": {"Task"}}
	if got := dataSourceArgErrors("dgX", dsWidget("dgX", "DS.Nope").GetDataSource(), params); len(got) != 0 {
		t.Errorf("unknown flow reported by the argument check: %v", got)
	}
}

func TestArgumentNamingNoParameterIsReported(t *testing.T) {
	// A typo in the ARGUMENT name binds nothing and is silent otherwise. Both
	// faults are reported: the parameter left unfilled and the name that matches
	// nothing.
	params := map[string][]string{"ds.mf": {"Task"}}
	got := dataSourceArgErrors("dgT", dsWidget("dgT", "DS.MF", "Tsak").GetDataSource(), params)
	if len(got) != 2 {
		t.Fatalf("got %d errors, want 2 (missing + unknown): %v", len(got), got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "'Tsak'") || !strings.Contains(joined, "no argument for parameter 'Task'") {
		t.Errorf("both faults not reported:\n%s", joined)
	}
}

func TestNonFlowDataSourcesAreIgnored(t *testing.T) {
	// Database, association, selection and variable sources have no parameters to
	// map, and must not be dragged into this.
	params := map[string][]string{"ds.mf": {"Task"}}
	for _, kind := range []string{"database", "association", "selection", "parameter"} {
		ds := &ast.DataSourceV3{Type: kind, Reference: "DS.MF"}
		if got := dataSourceArgErrors("w", ds, params); len(got) != 0 {
			t.Errorf("%s data source reported: %v", kind, got)
		}
	}
}
