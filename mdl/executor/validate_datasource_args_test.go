// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance-2: `datasource: microflow M.F` on a microflow WITH
// parameters built to CE1571 while `mxcli check --references` passed. The
// microflow resolves fine — the missing argument was nobody's check.
//
// These cases all sit OUTSIDE any data context (dataContext{}), which is where
// the rule was measured and where a missing argument is still an error. The
// enclosing-context half of the same rule is in
// validate_datasource_context_test.go.
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

// noContext runs one data source with nothing enclosing it — the shape the rule
// was originally measured on.
func noContext(widgetName string, ds *ast.DataSourceV3, sigs map[string]*flowSignature) []string {
	return dataSourceArgErrors(widgetName, ds, sigs, dataContext{}, sameName)
}

func TestDataSourceMissingArgumentIsReported(t *testing.T) {
	sigs := map[string]*flowSignature{"ds.mf": objSig("Task", "DS.Task")}
	got := noContext("dgBad", dsWidget("dgBad", "DS.MF").GetDataSource(), sigs)
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
	sigs := map[string]*flowSignature{"ds.mf": objSig("Task", "DS.Task")}
	if got := noContext("dgGood", dsWidget("dgGood", "DS.MF", "Task").GetDataSource(), sigs); len(got) != 0 {
		t.Errorf("correct arguments reported: %v", got)
	}
	// Mendix resolves names case-insensitively, and so must this.
	if got := noContext("dgGood", dsWidget("dgGood", "ds.MF", "task").GetDataSource(), sigs); len(got) != 0 {
		t.Errorf("case difference reported: %v", got)
	}
}

func TestParameterlessDataSourceNeedsNoParens(t *testing.T) {
	// The most common data source in any app. A rule that flagged this would fire
	// on nearly every page and be turned off.
	sigs := map[string]*flowSignature{"ds.all": {}}
	if got := noContext("dgA", dsWidget("dgA", "DS.All").GetDataSource(), sigs); len(got) != 0 {
		t.Errorf("parameterless microflow reported: %v", got)
	}
}

func TestUnknownFlowIsLeftToTheReferenceCheck(t *testing.T) {
	// "not found" is validateWidgetReferences' job. Reporting it here too would
	// double every typo, and worse, a typo'd flow name would be reported as a
	// missing ARGUMENT — which sends the reader to the wrong line.
	sigs := map[string]*flowSignature{"ds.mf": objSig("Task", "DS.Task")}
	if got := noContext("dgX", dsWidget("dgX", "DS.Nope").GetDataSource(), sigs); len(got) != 0 {
		t.Errorf("unknown flow reported by the argument check: %v", got)
	}
}

func TestArgumentNamingNoParameterIsReported(t *testing.T) {
	// A typo in the ARGUMENT name binds nothing and is silent otherwise. Both
	// faults are reported: the parameter left unfilled and the name that matches
	// nothing.
	sigs := map[string]*flowSignature{"ds.mf": objSig("Task", "DS.Task")}
	got := noContext("dgT", dsWidget("dgT", "DS.MF", "Tsak").GetDataSource(), sigs)
	if len(got) != 2 {
		t.Fatalf("got %d errors, want 2 (missing + unknown): %v", len(got), got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "'Tsak'") || !strings.Contains(joined, "no argument for parameter 'Task'") {
		t.Errorf("both faults not reported:\n%s", joined)
	}
	if !strings.Contains(joined, "(it declares 'Task')") {
		t.Errorf("the unknown-argument message should list the real parameters:\n%s", joined)
	}
}

func TestNonFlowDataSourcesAreIgnored(t *testing.T) {
	// Database, association, selection and variable sources have no parameters to
	// map, and must not be dragged into this.
	sigs := map[string]*flowSignature{"ds.mf": objSig("Task", "DS.Task")}
	for _, kind := range []string{"database", "association", "selection", "parameter"} {
		ds := &ast.DataSourceV3{Type: kind, Reference: "DS.MF"}
		if got := noContext("w", ds, sigs); len(got) != 0 {
			t.Errorf("%s data source reported: %v", kind, got)
		}
	}
}
