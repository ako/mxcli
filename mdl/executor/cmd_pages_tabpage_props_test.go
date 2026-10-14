// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1465: on a `tabpage`, `Visible:` and `CaptionParams` passed
// check and exec and were then not written — "the caption shows `Due · {1}`
// literally. `tpB` is always visible, and `ShowTabB` appears 0 times in the
// page unit". Tab pages are built by buildTabPageV3, outside buildWidgetV3, so
// the common tail that applies visibility to every other widget never ran.

// tabPagesOf parses an MDL page whose body is one tabcontainer inside a
// dataview over M.Job, and returns the tabcontainer's AST.
func tabContainerOf(t *testing.T, header, tabs string) (*ast.Program, *ast.WidgetV3) {
	t.Helper()
	src := header + "create page M.P (Title: 'x', Layout: A.L, Params: ($Job: M.Job)) {\n" +
		"  dataview dv (DataSource: $Job) {\n    tabcontainer tc {\n" + tabs + "    }\n  }\n};\n"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	tc := page.Widgets[0].Children[0]
	if !strings.EqualFold(tc.Type, "tabcontainer") {
		t.Fatalf("got %s, want the tabcontainer", tc.Type)
	}
	return prog, tc
}

func buildTabs(t *testing.T, pb *pageBuilder, tc *ast.WidgetV3) *pages.TabContainer {
	t.Helper()
	built, err := pb.buildWidgetV3(tc)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return built.(*pages.TabContainer)
}

func TestTabPage_VisibleIsWritten(t *testing.T) {
	_, tc := tabContainerOf(t, "", `
      tabpage tpA (Caption: 'A') { dynamictext t1 (Content: 'A') }
      tabpage tpB (Caption: 'B', Visible: $currentObject/IsLocal) { dynamictext t2 (Content: 'B') }
      tabpage tpC (Caption: 'C', Visible: false) { dynamictext t3 (Content: 'C') }
      tabpage tpD (Caption: 'D', Visible: IsLocal in (true)) { dynamictext t4 (Content: 'D') }
`)
	got := buildTabs(t, visibleWhenPB("M.Job"), tc)
	if len(got.TabPages) != 4 {
		t.Fatalf("got %d tab pages, want 4", len(got.TabPages))
	}
	// Control: a tab page without Visible: stays without a setting.
	if cv := got.TabPages[0].ConditionalVisibility; cv != nil {
		t.Errorf("tpA: a tab page without Visible: got a visibility setting %+v", cv)
	}
	if cv := got.TabPages[1].ConditionalVisibility; cv == nil || cv.Expression != "$currentObject/IsLocal" {
		t.Errorf("tpB: ConditionalVisibility = %+v, want Expression $currentObject/IsLocal — the condition was dropped (tab always visible)", cv)
	}
	if cv := got.TabPages[2].ConditionalVisibility; cv == nil || cv.Expression != "false" {
		t.Errorf("tpC: ConditionalVisibility = %+v, want Expression false", cv)
	}
	if cv := got.TabPages[3].ConditionalVisibility; cv == nil || cv.Attribute != "M.Job.IsLocal" || len(cv.Conditions) != 2 {
		t.Errorf("tpD: ConditionalVisibility = %+v, want the attribute condition on M.Job.IsLocal", cv)
	}
}

// DESCRIBE reads a tab page's visibility back, so describe -> exec keeps it.
func TestDescribe_TabPageVisible(t *testing.T) {
	stored := map[string]any{
		"$Type": "Forms$TabControl",
		"Name":  "tc",
		"TabPages": []any{int32(3),
			map[string]any{
				"$Type": "Forms$TabPage",
				"Name":  "tpB",
				"ConditionalVisibilitySettings": map[string]any{
					"$Type":      "Forms$ConditionalVisibilitySettings",
					"Expression": "$currentObject/ShowTabB",
				},
				"Widgets": []any{int32(2)},
			},
			map[string]any{"$Type": "Forms$TabPage", "Name": "tpA", "Widgets": []any{int32(2)}},
		},
	}
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	raw := parseRawWidget(ctx, stored)
	outputWidgetMDLV3(ctx, raw[0], 1)
	got := buf.String()
	if !strings.Contains(got, "tabpage tpB (Visible: $currentObject/ShowTabB)") {
		t.Errorf("describe output lacks the tab page's Visible:\n%s", got)
	}
	if strings.Count(got, "Visible:") != 1 {
		t.Errorf("only tpB has a visibility setting:\n%s", got)
	}
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + got + "}\n"); len(errs) > 0 {
		t.Errorf("describe output does not parse: %v\n%s", errs, got)
	}
}

func tabPageViolations(prog *ast.Program) []linter.Violation {
	reg, _ := NewWidgetRegistry()
	var out []linter.Violation
	for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], reg) {
		if v.RuleID == tabPagePropDropped.Code || v.RuleID == "MDL-WIDGET44" {
			out = append(out, v)
		}
	}
	return GateWidgetViolations(out, prog.LanguageVersion)
}

// Forms$TabPage has no Appearance and a Texts$Text caption with no parameters,
// so CaptionParams / Class / Style cannot be stored. check says so: a warning
// under mdl 0, a refusal under mdl 1 (ADR-0011). Control: Caption and Visible
// — what a tab page does store — draw nothing.
func TestCheck_TabPageUnstorableProps(t *testing.T) {
	tabs := `
      tabpage tpA (Caption: 'Due {1}', CaptionParams: ({1} = Title)) { dynamictext t1 (Content: 'A') }
      tabpage tpB (Caption: 'B', Visible: $currentObject/IsLocal) { dynamictext t2 (Content: 'B') }
      tabpage tpC (Caption: 'C', Class: 'foo', Style: 'color: red') { dynamictext t3 (Content: 'C') }
`
	prog, _ := tabContainerOf(t, "", tabs)
	vs := tabPageViolations(prog)
	var msgs []string
	for _, v := range vs {
		msgs = append(msgs, v.Message)
		if v.Severity != linter.SeverityWarning || v.RuleID != tabPagePropDropped.Code {
			t.Errorf("mdl 0: %s/%v, want a %s warning", v.RuleID, v.Severity, tabPagePropDropped.Code)
		}
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{"tpA", "CaptionParams", "tpC", "Class", "Style"} {
		if !strings.Contains(all, want) {
			t.Errorf("mdl 0: no violation mentions %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "tpB") {
		t.Errorf("tpB's Visible: is written — it must not be reported:\n%s", all)
	}
	if len(vs) != 2 {
		t.Errorf("got %d violations, want one per tab page (tpA, tpC):\n%s", len(vs), all)
	}

	prog1, _ := tabContainerOf(t, "mdl 1;\n", tabs)
	for _, v := range tabPageViolations(prog1) {
		if v.RuleID != "MDL-WIDGET44" || v.Severity != linter.SeverityError {
			t.Errorf("mdl 1: %s/%v, want an MDL-WIDGET44 error", v.RuleID, v.Severity)
		}
	}
}

// exec applies what check applies, so `exec --no-check` cannot drop them in
// silence either.
func TestBuild_TabPageUnstorableProps(t *testing.T) {
	_, tc := tabContainerOf(t, "", `
      tabpage tpA (Caption: 'Due {1}', CaptionParams: ({1} = Title)) { dynamictext t1 (Content: 'A') }
`)
	var out bytes.Buffer
	pb := visibleWhenPB("M.Job")
	pb.ctx = &ExecContext{Output: &out, LanguageVersion: langver.V1}
	if _, err := pb.buildWidgetV3(tc); err == nil || !strings.Contains(err.Error(), "CaptionParams") {
		t.Errorf("mdl 1: err = %v, want a refusal naming CaptionParams", err)
	}

	out.Reset()
	pb = visibleWhenPB("M.Job")
	pb.ctx = &ExecContext{Output: &out, LanguageVersion: langver.V0}
	if _, err := pb.buildWidgetV3(tc); err != nil {
		t.Errorf("mdl 0: refused (%v) — a new refusal applies only under the header", err)
	}
	if !strings.Contains(out.String(), tabPagePropDropped.Code) {
		t.Errorf("mdl 0: the drop was silent, want the %s warning; output:\n%s", tabPagePropDropped.Code, out.String())
	}
}
