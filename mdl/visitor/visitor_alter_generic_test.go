// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// The generic ALTER (ADR-0012 decision 2, ako/mxcli#712): one grammar rule
// `alter <type> Module.Name { set / insert / replace / drop }` for every
// document type, with the target written in one address syntax and resolved by
// the document type. These tests pin the CANONICAL spelling and that the old
// page spellings still parse to the same AST, flagged as the alias they are.

func buildAlterPage(t *testing.T, input string) *ast.AlterPageStmt {
	t.Helper()
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("want 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.AlterPageStmt)
	if !ok {
		t.Fatalf("want *ast.AlterPageStmt, got %T", prog.Statements[0])
	}
	return stmt
}

func TestGenericAlter_CanonicalSetIsParenthesisedAndColon(t *testing.T) {
	const src = `alter page Module.Page {
		set (Caption: 'Save', ButtonStyle: Success) on btnSave;
		set (Title: 'Edit order');
	};`
	stmt := buildAlterPage(t, src)
	if got := deprecationCodes(mustBuild(t, src)); len(got) != 0 {
		t.Errorf("canonical set must not be reported as an alias, got %v", got)
	}
	if len(stmt.Operations) != 2 {
		t.Fatalf("want 2 operations, got %d", len(stmt.Operations))
	}
	onWidget := stmt.Operations[0].(*ast.SetPropertyOp)
	if onWidget.Target.Widget != "btnSave" {
		t.Errorf("target: got %q", onWidget.Target.Widget)
	}
	if onWidget.Properties["Caption"] != "Save" || onWidget.Properties["ButtonStyle"] != "Success" {
		t.Errorf("properties: got %v", onWidget.Properties)
	}
	pageLevel := stmt.Operations[1].(*ast.SetPropertyOp)
	if pageLevel.Target.Widget != "" || pageLevel.Properties["Title"] != "Edit order" {
		t.Errorf("page-level set: target %q, properties %v", pageLevel.Target.Widget, pageLevel.Properties)
	}
}

func TestGenericAlter_CanonicalDropNamesTargetsWithoutKeyword(t *testing.T) {
	const src = `alter snippet Module.Snip {
		drop txtOld, dgOrders.Total;
	};`
	stmt := buildAlterPage(t, src)
	if got := deprecationCodes(mustBuild(t, src)); len(got) != 0 {
		t.Errorf("canonical drop reported as an alias: %v", got)
	}
	if stmt.ContainerType != "SNIPPET" {
		t.Errorf("container type: got %q", stmt.ContainerType)
	}
	drop := stmt.Operations[0].(*ast.DropWidgetOp)
	if len(drop.Targets) != 2 || drop.Targets[0].Widget != "txtOld" ||
		drop.Targets[1].Widget != "dgOrders" || drop.Targets[1].Column != "Total" {
		t.Errorf("targets: got %+v", drop.Targets)
	}
}

// A widget may be NAMED like a keyword the old forms use; the canonical drop
// of it must still parse as a drop of that name.
func TestGenericAlter_DropOfWidgetNamedLikeAKeyword(t *testing.T) {
	const src = `alter page Module.Page { drop widget; };`
	stmt := buildAlterPage(t, src)
	drop := stmt.Operations[0].(*ast.DropWidgetOp)
	if got := deprecationCodes(mustBuild(t, src)); len(drop.Targets) != 1 || drop.Targets[0].Widget != "widget" || len(got) != 0 {
		t.Errorf("got %+v, deprecations %v", drop.Targets, got)
	}
}

func TestGenericAlter_TargetAddressForms(t *testing.T) {
	stmt := buildAlterPage(t, `alter layout Module.Lay {
		insert into layoutContainer.top { snippetcall bar (Snippet: Module.Bar) }
		insert after 'Approve order'@2 { textbox t1 (Label: 'x') }
		replace hdr@1 with { container c1 }
	};`)
	if stmt.ContainerType != "LAYOUT" {
		t.Errorf("container type: got %q", stmt.ContainerType)
	}
	into := stmt.Operations[0].(*ast.InsertWidgetOp)
	if into.Position != "INTO" || into.Target.Widget != "layoutContainer" || into.Target.Column != "top" {
		t.Errorf("into: %+v", into)
	}
	byCaption := stmt.Operations[1].(*ast.InsertWidgetOp)
	if byCaption.Target.Caption != "Approve order" || byCaption.Target.Ordinal != 2 || byCaption.Target.Widget != "" {
		t.Errorf("caption target: %+v", byCaption.Target)
	}
	repl := stmt.Operations[2].(*ast.ReplaceWidgetOp)
	if repl.Target.Widget != "hdr" || repl.Target.Ordinal != 1 {
		t.Errorf("ordinal target: %+v", repl.Target)
	}
}

// The old spellings are aliases: they must parse to the same operations as the
// canonical form, and record which alias was used so check/exec can warn.
func TestGenericAlter_OldSpellingsAreFlaggedAliases(t *testing.T) {
	cases := []struct {
		name, op string
		legacy   string
	}{
		{"set without parentheses", `set Caption = 'Save' on btnSave`, deprecation.AlterPageSetEquals},
		{"page-level set without parentheses", `set Title = 'Edit'`, deprecation.AlterPageSetEquals},
		{"parenthesised set with =", `set (Caption = 'Save', ButtonStyle = Success) on btnSave`, deprecation.AlterPageSetEquals},
		{"set without parentheses, with colon", `set Caption: 'Save' on btnSave`, deprecation.AlterPageSetUnparenthesised},
		{"drop widget", `drop widget txtOld, txtUnused`, deprecation.AlterPageDropWidget},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := deprecationCodes(mustBuild(t, "alter page Module.Page { "+c.op+"; };"))
			if len(got) != 1 || got[0] != c.legacy {
				t.Errorf("recorded %v, want [%s]", got, c.legacy)
			}
		})
	}
}

// The document-specific operations with no generic spelling yet keep their own
// form inside the generic block and are NOT aliases.
func TestGenericAlter_DocumentSpecificOperationsStillParse(t *testing.T) {
	stmt := buildAlterPage(t, `alter page Module.Page {
		set layout = Atlas_Core.TopBar map (Main as Content);
		drop template for Module.Special in lvItems;
		add variables $show: Boolean = 'true';
		drop variables $show;
	};`)
	if len(stmt.Operations) != 4 {
		t.Fatalf("want 4 operations, got %d", len(stmt.Operations))
	}
	if _, ok := stmt.Operations[0].(*ast.SetLayoutOp); !ok {
		t.Errorf("op 0: %T", stmt.Operations[0])
	}
	if _, ok := stmt.Operations[1].(*ast.DropListViewTemplateOp); !ok {
		t.Errorf("op 1: %T", stmt.Operations[1])
	}
	if _, ok := stmt.Operations[2].(*ast.AddVariableOp); !ok {
		t.Errorf("op 2: %T", stmt.Operations[2])
	}
	if _, ok := stmt.Operations[3].(*ast.DropVariableOp); !ok {
		t.Errorf("op 3: %T", stmt.Operations[3])
	}
}

// @0 would read as "no ordinal" and address whatever a bare name addresses.
func TestGenericAlter_OrdinalZeroIsRefused(t *testing.T) {
	_, errs := Build(`alter page Module.Page { drop txtName@0; };`)
	if len(errs) == 0 {
		t.Fatal("want an error for @0, got none")
	}
}

// mendixlabs/mxcli#1234: `add parameters` was a parse error — "mismatched input
// 'parameters' expecting VARIABLES_KW" — so adding a parameter to an existing
// page meant CREATE OR REPLACE. The declaration is CREATE's own `pageParameter`,
// so an entity and a primitive both parse, as does a quoted reserved name.
func TestAlterPage_AddAndDropParameters(t *testing.T) {
	stmt := buildAlterPage(t, `alter page Module.Page {
		add parameters $Customer: Module.Customer;
		add parameters $Count: Integer;
		add parameters "List": String;
		drop parameters $Old;
	};`)
	if len(stmt.Operations) != 4 {
		t.Fatalf("want 4 operations, got %d", len(stmt.Operations))
	}
	entity, ok := stmt.Operations[0].(*ast.AddParameterOp)
	if !ok {
		t.Fatalf("op 0: %T", stmt.Operations[0])
	}
	if entity.Parameter.Name != "Customer" || entity.Parameter.EntityType.String() != "Module.Customer" {
		t.Errorf("op 0: got %+v", entity.Parameter)
	}
	prim, ok := stmt.Operations[1].(*ast.AddParameterOp)
	if !ok {
		t.Fatalf("op 1: %T", stmt.Operations[1])
	}
	if prim.Parameter.Name != "Count" || prim.Parameter.Type.Kind != ast.TypeInteger || prim.Parameter.EntityType.Name != "" {
		t.Errorf("op 1: got %+v", prim.Parameter)
	}
	if quoted, ok := stmt.Operations[2].(*ast.AddParameterOp); !ok || quoted.Parameter.Name != "List" {
		t.Errorf("op 2: %T %+v", stmt.Operations[2], stmt.Operations[2])
	}
	drop, ok := stmt.Operations[3].(*ast.DropParameterOp)
	if !ok {
		t.Fatalf("op 3: %T", stmt.Operations[3])
	}
	if drop.ParameterName != "Old" {
		t.Errorf("op 3: got %q, want Old", drop.ParameterName)
	}
	if got := deprecationCodes(mustBuild(t, `alter page Module.Page { add parameters $X: String; };`)); len(got) != 0 {
		t.Errorf("add parameters is not an alias, recorded %v", got)
	}
}
