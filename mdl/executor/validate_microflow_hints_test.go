// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestValidateMicroflow_DateTimeLiterals covers MDL046 (ledger finding #21):
// dateTime()/dateTimeUTC() accept only literal numeric constants — a variable or
// computed argument fails the build with CE0117.
func TestValidateMicroflow_DateTimeLiterals(t *testing.T) {
	cases := []struct {
		name    string
		params  string
		body    string
		wantMDL bool
	}{
		{"variable arg", "$Month: Integer, $Day: Integer", "set $C = dateTime(2026, $Month, $Day);", true},
		{"computed arg", "$Y: Integer", "set $C = dateTime($Y + 1, 1, 1);", true},
		{"utc variable arg", "$Month: Integer", "set $C = dateTimeUTC(2026, $Month, 1);", true},
		{"all literals", "", "set $C = dateTime(2026, 1, 1);", false},
		{"literals with time", "", "set $C = dateTime(2026, 12, 31, 23, 59, 59);", false},
		{"no datetime call", "$X: Integer", "set $C = $X + 1;", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F (" + tc.params + ")\nreturns DateTime\nbegin\n  " + tc.body + "\n  return $C;\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			var got bool
			for _, vi := range ValidateMicroflow(mf) {
				if vi.RuleID == "MDL046" {
					got = true
				}
			}
			if got != tc.wantMDL {
				t.Errorf("MDL046 fired=%v, want %v (body: %q)", got, tc.wantMDL, tc.body)
			}
		})
	}
}

// TestValidateMicroflow_XPathAssociationEmpty covers MDL047 (ledger finding #25):
// `[Module.Association = empty]` is CE0161 — XPath has no `= empty` for
// associations; the nullability test is `not(Assoc/Target)`. A bare attribute
// (`Name = empty`) and an attribute-over-association (`Assoc/Attr = empty`) are
// valid and must not be flagged.
func TestValidateMicroflow_XPathAssociationEmpty(t *testing.T) {
	cases := []struct {
		name    string
		where   string
		wantMDL bool
	}{
		{"association = empty", "[Ledger.Transaction_Category = empty]", true},
		{"negation form is fine", "[not(Ledger.Transaction_Category/Ledger.Category)]", false},
		{"bare attribute = empty is fine", "[Description = empty]", false},
		{"attribute over association is fine", "[Ledger.Transaction_Category/Ledger.Name = empty]", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ()\nreturns list of Ledger.Transaction\nbegin\n  retrieve $t from Ledger.Transaction where " + tc.where + ";\n  return $t;\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			var got bool
			for _, vi := range ValidateMicroflow(mf) {
				if vi.RuleID == "MDL047" {
					got = true
				}
			}
			if got != tc.wantMDL {
				t.Errorf("MDL047 fired=%v, want %v (where: %q)", got, tc.wantMDL, tc.where)
			}
		})
	}
}

// TestValidateMicroflow_XPathIdConstraint covers MDL048 (ledger #42): a retrieve
// that constrains on the object id (`[id = $x]`) fails the build with CE0161.
func TestValidateMicroflow_XPathIdConstraint(t *testing.T) {
	cases := []struct {
		name    string
		where   string
		wantMDL bool
	}{
		{"id equals a String value var", "[id = $Id]", true},
		{"ID case-insensitive against value var", "[ID = $Id]", true},
		{"id equals a string literal", "[id = '123']", true},
		{"id in boolean clause", "[Active = true and id = $Id]", true},
		// Comparing id to an OBJECT variable is the valid "exclude self" pattern.
		{"id not-equals an object var is fine", "[id != $This]", false},
		// The `[%CurrentUser%]` server token is the standard, build-clean idiom for
		// the signed-in user's id — Mendix resolves it to a GUID (FINDINGS #53).
		{"id equals CurrentUser token is fine", "[id = '[%CurrentUser%]']", false},
		{"attribute containing id is fine", "[Valid = true]", false},
		{"paidstatus is fine", "[PaidStatus = $x]", false},
		{"plain attribute is fine", "[Name = $n]", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ($Id: String, $x: String, $n: String, $This: M.B)\nreturns list of M.B\nbegin\n  retrieve $B from M.B where " + tc.where + ";\n  return $B;\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			var got bool
			for _, vi := range ValidateMicroflow(mf) {
				if vi.RuleID == "MDL048" {
					got = true
				}
			}
			if got != tc.wantMDL {
				t.Errorf("MDL048 fired=%v, want %v (where: %q)", got, tc.wantMDL, tc.where)
			}
		})
	}
}

// TestValidateMicroflow_AssociationObjectArg covers MDL049 (ledger #43/#44): a
// call argument bound to an association-object path (`$obj/Module.Assoc`) fails
// the build with CE0117 — it must be materialized first.
func TestValidateMicroflow_AssociationObjectArg(t *testing.T) {
	cases := []struct {
		name    string
		arg     string
		wantMDL bool
	}{
		{"association object path", "B = $E/M.Edit_Budget", true},
		{"attribute over association is fine", "Name = $E/M.Edit_Budget/Label", false},
		{"plain attribute is fine", "Name = $E/Note", false},
		{"variable is fine", "B = $E", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ($E: M.Edit)\nreturns boolean\nbegin\n  $R = call microflow M.Consume(" + tc.arg + ");\n  return $R;\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			var got bool
			for _, vi := range ValidateMicroflow(mf) {
				if vi.RuleID == "MDL049" {
					got = true
				}
			}
			if got != tc.wantMDL {
				t.Errorf("MDL049 fired=%v, want %v (arg: %q)", got, tc.wantMDL, tc.arg)
			}
		})
	}
}

// TestValidateMicroflow_ConditionalBreakAccepted: MDL051 rejected a `break` or
// `continue` inside a conditional within a loop, because the write path dropped the
// Break/Continue event and left a dangling sequence flow (ledger #52). That was an
// interim guard "until the serialization is fixed" — it now is (mendixlabs/mxcli#791,
// microflowObjectToGen), so the pattern must be accepted again rather than pushing
// users to a guard variable. It also only ever covered `break`, which is why the
// `continue` form reached users as a corrupt project.
func TestValidateMicroflow_ConditionalBreakAccepted(t *testing.T) {
	bodies := []string{
		"loop $R in $L begin if $R/Active then break; end if; end loop",
		"loop $R in $L begin if $R/Active then continue; end if; end loop",
		"loop $R in $L begin if $R/Active then if $R/Active then break; end if; end if; end loop",
		"loop $R in $L begin break; end loop",
	}
	for _, body := range bodies {
		t.Run(body, func(t *testing.T) {
			src := "create microflow M.F ($L: list of M.R)\nreturns boolean\nbegin\n  " + body + "\n  return true;\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			for _, vi := range ValidateMicroflow(mf) {
				if vi.RuleID == "MDL051" {
					t.Errorf("MDL051 still rejects a now-serializable pattern: %s", body)
				}
			}
		})
	}
}

// TestValidateDatasourceXPathAssociationEmpty covers the page/widget-datasource
// arm of MDL047 (ledger #25 verification round): the original check only saw
// microflow retrieves, so `datagrid (datasource: database ... where [Assoc =
// empty])` slipped through.
func TestValidateDatasourceXPathAssociationEmpty(t *testing.T) {
	dg := func(where string) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "datagrid", Name: "dg", Properties: map[string]any{
			"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Ledger.Transaction", Where: where},
		}}
	}
	cases := []struct {
		name   string
		widget *ast.WidgetV3
		want   bool
	}{
		{"association = empty", dg("[Ledger.Transaction_Category = empty]"), true},
		{"negation is fine", dg("[not(Ledger.Transaction_Category/Ledger.Category)]"), false},
		{"attribute = empty is fine", dg("[Description = empty]"), false},
		{"no where clause", dg(""), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := len(validateDatasourceXPathAssociationEmpty(c.widget, "page X")) > 0
			if got != c.want {
				t.Errorf("MDL047 present = %v, want %v", got, c.want)
			}
		})
	}
}

// TestValidateMicroflow_DuplicateLoopVariable covers MDL052 (ledger #64): a loop
// iterator is scoped to the whole microflow, so reusing a name across loops
// builds as CE0111. Distinct names — and a single loop — are fine.
func TestValidateMicroflow_DuplicateLoopVariable(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMDL bool
	}{
		{"two loops same iterator", "loop $R in $L begin set $x = 1; end loop loop $R in $L begin set $y = 1; end loop", true},
		{"nested loop reuses outer iterator", "loop $R in $L begin loop $R in $L begin set $x = 1; end loop end loop", true},
		{"distinct iterators are fine", "loop $R in $L begin set $x = 1; end loop loop $C in $L begin set $y = 1; end loop", false},
		{"single loop is fine", "loop $R in $L begin set $x = 1; end loop", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ($L: list of M.R)\nreturns Boolean\nbegin\n  " + tc.body + "\n  return true;\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			var got bool
			for _, vi := range ValidateMicroflow(mf) {
				if vi.RuleID == "MDL052" {
					got = true
				}
			}
			if got != tc.wantMDL {
				t.Errorf("MDL052 fired=%v, want %v (body: %q)", got, tc.wantMDL, tc.body)
			}
		})
	}
}
