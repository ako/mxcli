// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// An input widget bound to an attribute where nothing supplies an object —
// `textbox t (Attribute: FullName)` at the top of a page — was built with its
// binding resolved to the bare name, which the writer cannot store: the widget
// went out with `AttributeRef: null`. `exec` said "Created page"; mxbuild
// 11.13.0 then reported CE0544 "This widget can only function inside a data
// context" plus CE7005 "No value selection has been made", CE0402 on a dynamic
// text and CE0642 "Property 'Attribute' is required" on a combo box. A
// QUALIFIED attribute there is stored, and is just as wrong: CE0544 / CE2421 /
// CE1365 / CE7247 "Move this widget into a data container".

func testInputPB(entity string, argCtx pageArgContext) *pageBuilder {
	return &pageBuilder{entityContext: entity, argCtx: argCtx, widgetScope: map[string]model.ID{}}
}

func inputWidget(kind, name string, attr any) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: kind, Name: name, Properties: map[string]any{"Attribute": attr}}
}

// The builder is the last gate before the writer, and ALTER PAGE reaches it
// without passing the check-time walk. It must refuse, naming the widget, rather
// than hand the writer a binding it will turn into null.
func TestBuildInputWithoutDataContextIsRefused(t *testing.T) {
	cases := []struct {
		kind, attr string
		want       []string
	}{
		{"textbox", "FullName", []string{"`t`", "FullName", "data container"}},
		{"textarea", "Notes", []string{"`t`", "Notes", "data container"}},
		{"datepicker", "Born", []string{"`t`", "data container"}},
		{"checkbox", "Active", []string{"`t`", "data container"}},
		{"radiobuttons", "Fav", []string{"`t`", "data container"}},
		{"dropdown", "Fav", []string{"`t`", "data container"}},
		{"dynamictext", "FullName", []string{"`t`", "data container"}},
		// Qualified is stored, and fails the build all the same.
		{"textbox", "AN.Person.FullName", []string{"`t`", "AN.Person.FullName", "data container"}},
	}
	for _, c := range cases {
		t.Run(c.kind+"/"+c.attr, func(t *testing.T) {
			pb := testInputPB("", atDocumentRoot())
			_, err := pb.buildWidgetV3(inputWidget(c.kind, "t", c.attr))
			if err == nil {
				t.Fatalf("%s bound to %q at page level was built — a bare name is written AttributeRef: null, a qualified one fails CE0544", c.kind, c.attr)
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("refusal does not mention %q: %v", w, err)
				}
			}
		})
	}
}

// ALTER PAGE builds with an UNKNOWN context (the stored page is never walked).
// A qualified binding may be legitimate there — inside a container whose flow
// the project lacks, it is how DESCRIBE writes it — so only the binding the
// writer provably cannot store, a bare name with no entity, is refused.
func TestBuildInputUnknownContext(t *testing.T) {
	pb := testInputPB("", pageArgContext{})
	if _, err := pb.buildWidgetV3(inputWidget("textbox", "t", "FullName")); err == nil {
		t.Fatal("bare attribute with no entity in scope was built — the writer stores AttributeRef: null")
	} else if !strings.Contains(err.Error(), "Module.Entity.Attribute") {
		t.Errorf("refusal should offer the qualified form: %v", err)
	}
	if _, err := pb.buildWidgetV3(inputWidget("textbox", "t", "AN.Person.FullName")); err != nil {
		t.Errorf("qualified attribute in an unknown context was refused: %v", err)
	}
}

// The control: inside a data container the same widgets build.
func TestBuildInputInsideDataContext(t *testing.T) {
	for _, kind := range []string{"textbox", "textarea", "datepicker", "checkbox", "radiobuttons", "dropdown", "dynamictext"} {
		pb := testInputPB("AN.Person", pageArgContext{known: true, present: true})
		if _, err := pb.buildWidgetV3(inputWidget(kind, "t", "FullName")); err != nil {
			t.Errorf("%s inside a data container was refused: %v", kind, err)
		}
	}
}

// `Attribute: $P/FullName` does not parse as an attribute path at all — it lands
// as a data-source expression, which no input builder reads — so the binding
// was dropped INSIDE a data view as well as outside one.
func TestBuildInputVariableRootedAttributeIsRefused(t *testing.T) {
	ds := &ast.DataSourceV3{Type: "parameter", Reference: "$P"}
	pb := testInputPB("AN.Person", pageArgContext{known: true, present: true})
	_, err := pb.buildWidgetV3(inputWidget("textbox", "t", ds))
	if err == nil {
		t.Fatal("`Attribute: $P/…` was built — no builder reads it, so the binding is dropped")
	}
	if !strings.Contains(err.Error(), "`t`") {
		t.Errorf("refusal does not name the widget: %v", err)
	}
}

// Check time: the same refusal from the widget-tree walk, which runs with no
// project, so `mxcli check script.mdl` reports it.
func TestValidateInputBindingWithoutDataContext(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("LoadWidgetRegistry returned nil")
	}
	hits := func(tree []*ast.WidgetV3, subtree bool) (n int, msg string) {
		vs := validateWidgetTree(tree, registry, "page AN.P")
		if subtree {
			vs = validateWidgetSubtree(tree, registry, "alter AN.P")
		}
		for _, v := range vs {
			if v.RuleID == "MDL-WIDGET34" {
				n++
				msg = v.Message
			}
		}
		return
	}

	for _, kind := range []string{"textbox", "textarea", "datepicker", "checkbox", "radiobuttons", "dropdown", "dynamictext", "combobox"} {
		if n, _ := hits([]*ast.WidgetV3{inputWidget(kind, "t", "FullName")}, false); n != 1 {
			t.Errorf("%s at page level: MDL-WIDGET34 = %d, want 1", kind, n)
		}
	}
	if n, msg := hits([]*ast.WidgetV3{inputWidget("textbox", "t", "AN.Person.FullName")}, false); n != 1 {
		t.Errorf("qualified textbox at page level: MDL-WIDGET34 = %d, want 1", n)
	} else if !strings.Contains(msg, "CE0544") {
		t.Errorf("message should name the build error: %s", msg)
	}
	// A plain container supplies nothing.
	nested := []*ast.WidgetV3{{Type: "container", Name: "c", Children: []*ast.WidgetV3{inputWidget("textbox", "t", "FullName")}}}
	if n, _ := hits(nested, false); n != 1 {
		t.Errorf("textbox in a plain container: MDL-WIDGET34 = %d, want 1", n)
	}
	// Controls: a data view supplies the object; ALTER cannot say.
	dv := []*ast.WidgetV3{{
		Type: "dataview", Name: "dv",
		Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "$P"}},
		Children:   []*ast.WidgetV3{inputWidget("textbox", "t", "FullName")},
	}}
	if n, msg := hits(dv, false); n != 0 {
		t.Errorf("textbox inside a data view was flagged: %s", msg)
	}
	if n, msg := hits([]*ast.WidgetV3{inputWidget("textbox", "t", "FullName")}, true); n != 0 {
		t.Errorf("ALTER PAGE insert was flagged at check time, where the enclosing context is unknown: %s", msg)
	}
	// `$P/Attr` is dropped wherever it is written.
	varRooted := []*ast.WidgetV3{{
		Type: "dataview", Name: "dv",
		Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "parameter", Reference: "$P"}},
		Children:   []*ast.WidgetV3{inputWidget("textbox", "t", &ast.DataSourceV3{Type: "parameter", Reference: "$P"})},
	}}
	if n, _ := hits(varRooted, false); n != 1 {
		t.Errorf("`Attribute: $P/…` inside a data view: MDL-WIDGET34 = %d, want 1", n)
	}
}
