// SPDX-License-Identifier: Apache-2.0

package executor

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1346. `dataview dv (DataSource: $O, ShowFooter: true)` drew
// MDL-WIDGET07 "not recognized and will be silently dropped on write" although
// the dataview builder reads it and `describe page` emits it. navigationtree /
// menubar's `Menu:`, `Profile:` and `Orientation:` were the same false warning:
// read by the builder, emitted by describe, absent from the allow-list.
func TestStaticWidgetUnknownProps_BuilderConsumedKeysDoNotWarn(t *testing.T) {
	prog := parseMDL(t, `create or modify page Shop.Order_Edit (
  title: 'Order',
  Layout: Atlas_Core.Atlas_Default,
  params: ( $Order: Shop.Order )
)
{
  dataview dvOrder (datasource: $Order, showFooter: true) {
    textbox txtNumber (attribute: Number)
  }
  navigationtree nav1 (Profile: 'Responsive', Orientation: Vertical)
  menubar mb1 (Menu: Shop.MainMenu)
};`)
	var widgets []*ast.WidgetV3
	for _, s := range prog.Statements {
		if p, ok := s.(*ast.CreatePageStmtV3); ok {
			widgets = p.Widgets
		}
	}
	if len(widgets) == 0 {
		t.Fatal("no page widgets parsed")
	}
	for _, v := range validateWidgetTree(widgets, LoadWidgetRegistry(""), "page Shop.Order_Edit") {
		if v.RuleID == "MDL-WIDGET07" {
			t.Errorf("false MDL-WIDGET07: %s", v.Message)
		}
	}
}

// The allow-list and TestStaticWidgetKnownPropsCoverDescribe were both typed by
// hand, so a property added to a builder and to describe (#813's ShowFooter)
// slipped past both. This derives the vocabulary from the source instead: every
// key a static-widget builder reads, and every `Key: …` describe emits, must be
// recognized — otherwise check warns that a written property is dropped, and
// re-executing describe's output warns about itself.
func TestStaticWidgetKnownProps_CoverSourceVocabulary(t *testing.T) {
	// Emitted only for the pluggable `image` widget, whose `<Name>Params`
	// companions the pluggable engine consumes (textTemplateParams); MDL-WIDGET01
	// validates that path, never MDL-WIDGET07.
	pluggableOnly := map[string]bool{"ImageUrlParams": true, "AlternativeTextParams": true}

	builderFiles, err := filepath.Glob("cmd_pages_builder*.go")
	if err != nil {
		t.Fatal(err)
	}
	consumed := map[string]string{}
	fset := token.NewFileSet()
	for _, path := range builderFiles {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		goast.Inspect(f, func(n goast.Node) bool {
			var lit goast.Expr
			switch x := n.(type) {
			case *goast.CallExpr:
				switch fn := x.Fun.(type) {
				case *goast.SelectorExpr: // w.GetStringProp("Menu")
					if strings.HasPrefix(fn.Sel.Name, "Get") && strings.HasSuffix(fn.Sel.Name, "Prop") && len(x.Args) == 1 {
						lit = x.Args[0]
					}
				case *goast.Ident: // lookupPropCI(w, "ShowFooter")
					if fn.Name == "lookupPropCI" && len(x.Args) == 2 {
						lit = x.Args[1]
					}
				}
			case *goast.IndexExpr: // w.Properties["Footer"]
				if sel, ok := x.X.(*goast.SelectorExpr); ok && sel.Sel.Name == "Properties" {
					lit = x.Index
				}
			}
			if bl, ok := lit.(*goast.BasicLit); ok && bl.Kind == token.STRING {
				if key, err := strconv.Unquote(bl.Value); err == nil {
					consumed[key] = fset.Position(bl.Pos()).String()
				}
			}
			return true
		})
	}
	if len(consumed) < 20 {
		t.Fatalf("found only %d builder-consumed keys — the source scan no longer matches how builders read properties", len(consumed))
	}

	emitted := map[string]string{}
	src, err := os.ReadFile("cmd_pages_describe_output.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`"([A-Z][A-Za-z]+): `).FindAllStringSubmatch(string(src), -1) {
		emitted[m[1]] = "cmd_pages_describe_output.go"
	}
	if len(emitted) < 20 {
		t.Fatalf("found only %d describe-emitted keys — the source scan no longer matches how describe prints properties", len(emitted))
	}

	var missing []string
	for _, set := range []map[string]string{consumed, emitted} {
		for key, where := range set {
			if !pluggableOnly[key] && !isKnownStaticWidgetProp(key) {
				missing = append(missing, key+" ("+where+")")
			}
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("%s is read or emitted for a built-in widget but is not in staticWidgetKnownProps — check would false-warn MDL-WIDGET07", m)
	}
}
