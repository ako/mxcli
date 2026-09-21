// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

func parseOneAlterFlow(t *testing.T, src string) *ast.AlterFlowActivitiesStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	if prog == nil || len(prog.Statements) != 1 {
		t.Fatalf("parse %q: want 1 statement", src)
	}
	stmt, ok := prog.Statements[0].(*ast.AlterFlowActivitiesStmt)
	if !ok {
		t.Fatalf("parse %q: got %T, want *ast.AlterFlowActivitiesStmt", src, prog.Statements[0])
	}
	return stmt
}

// The scope half: which documents a statement reaches.
func TestAlterFlowActivitiesScope(t *testing.T) {
	for _, tc := range []struct {
		src     string
		flavour ast.FlowFlavour
		bulk    bool
		module  string
		qname   string
		disable bool
	}{
		{"alter microflow Sales.ACT_Do disable activities where action = log;",
			ast.FlowMicroflow, false, "", "Sales.ACT_Do", true},
		{"alter microflow Sales.ACT_Do enable activities where action = log;",
			ast.FlowMicroflow, false, "", "Sales.ACT_Do", false},
		{"alter nanoflow Sales.NF disable activities where action = log;",
			ast.FlowNanoflow, false, "", "Sales.NF", true},
		{"alter rule Sales.R disable activities where action = log;",
			ast.FlowRule, false, "", "Sales.R", true},
		{"alter microflows in Sales disable activities where action = log;",
			ast.FlowMicroflow, true, "Sales", "", true},
		// No IN — the whole project. Same shape as ALTER PAGES … SET LAYOUT.
		{"alter microflows disable activities where action = log;",
			ast.FlowMicroflow, true, "", "", true},
		{"alter nanoflows in Sales enable activities where action = log;",
			ast.FlowNanoflow, true, "Sales", "", false},
		{"alter rules disable activities where action = log;",
			ast.FlowRule, true, "", "", true},
	} {
		t.Run(tc.src, func(t *testing.T) {
			s := parseOneAlterFlow(t, tc.src)
			if s.Flavour != tc.flavour {
				t.Errorf("Flavour = %q, want %q", s.Flavour, tc.flavour)
			}
			if s.Bulk != tc.bulk {
				t.Errorf("Bulk = %v, want %v", s.Bulk, tc.bulk)
			}
			if s.Module != tc.module {
				t.Errorf("Module = %q, want %q", s.Module, tc.module)
			}
			if got := s.Name.String(); tc.qname != "" && got != tc.qname {
				t.Errorf("Name = %q, want %q", got, tc.qname)
			}
			if s.Disable != tc.disable {
				t.Errorf("Disable = %v, want %v", s.Disable, tc.disable)
			}
		})
	}
}

// The filter half: nothing may be dropped between the parse tree and the AST,
// because a dropped condition WIDENS what the statement acts on. A dropped
// `level = debug` turns "disable the debug logs" into "disable every log".
func TestAlterFlowActivitiesFilter(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want []types.ActivityCondition
	}{
		{
			"the reported case",
			"alter microflows in Sales disable activities where action = log and level = debug;",
			[]types.ActivityCondition{
				{Column: "action", Values: []string{"log"}},
				{Column: "level", Values: []string{"debug"}},
			},
		},
		{
			"IN, which is how an OR is written",
			"alter microflows disable activities where level in (debug, trace);",
			[]types.ActivityCondition{{Column: "level", Values: []string{"debug", "trace"}}},
		},
		{
			"NOT IN",
			"alter microflows disable activities where level not in (error, critical);",
			[]types.ActivityCondition{{Column: "level", Negate: true, Values: []string{"error", "critical"}}},
		},
		{
			"!= negates",
			"alter microflows disable activities where action != log;",
			[]types.ActivityCondition{{Column: "action", Negate: true, Values: []string{"log"}}},
		},
		{
			"a quoted multi-word action keeps its spaces",
			"alter nanoflows disable activities where action = 'call javascript action';",
			[]types.ActivityCondition{{Column: "action", Values: []string{"call javascript action"}}},
		},
		{
			"LIKE keeps the pattern, quotes stripped",
			"alter microflows disable activities where caption like '%TODO%';",
			[]types.ActivityCondition{{Column: "caption", Like: true, Values: []string{"%TODO%"}}},
		},
		{
			"the current state is a column",
			"alter microflows enable activities where disabled = true;",
			[]types.ActivityCondition{{Column: "disabled", Values: []string{"true"}}},
		},
		{
			"three conditions, all carried",
			"alter microflows in Sales disable activities where action = log and level = debug and disabled = false;",
			[]types.ActivityCondition{
				{Column: "action", Values: []string{"log"}},
				{Column: "level", Values: []string{"debug"}},
				{Column: "disabled", Values: []string{"false"}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseOneAlterFlow(t, tc.src).Filter.Conditions
			if len(got) != len(tc.want) {
				t.Fatalf("got %d conditions, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				w := tc.want[i]
				if got[i].Column != w.Column || got[i].Negate != w.Negate || got[i].Like != w.Like {
					t.Errorf("condition %d = %+v, want %+v", i, got[i], w)
				}
				if len(got[i].Values) != len(w.Values) {
					t.Errorf("condition %d values = %v, want %v", i, got[i].Values, w.Values)
					continue
				}
				for j := range got[i].Values {
					if got[i].Values[j] != w.Values[j] {
						t.Errorf("condition %d value %d = %q, want %q", i, j, got[i].Values[j], w.Values[j])
					}
				}
			}
		})
	}
}

// `disabled` became a lexer keyword for the WHERE column. Without an arm in
// annotationName that stops `@disabled` matching IDENTIFIER and the annotation
// becomes a parse error — a keyword collision introduced by adding a token
// rather than by naming something.
func TestDisabledAnnotationStillParsesAfterTheKeyword(t *testing.T) {
	src := "create microflow M.F () returns Boolean begin @disabled log info 'x'; return true; end;"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("`@disabled` no longer parses: %v", errs)
	}
	create, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	ann := ast.StatementAnnotations(create.Body[0])
	if ann == nil || !ann.Disabled {
		t.Error("`@disabled` parsed but did not set the flag")
	}
}
