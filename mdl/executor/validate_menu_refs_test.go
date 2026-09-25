// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#573 gave the menu widgets `Menu: Module.Menu`, a new qualified name
// to get wrong. Measured on a blank 11.14.0 project, before this check:
//
//	simplemenubar bottomBar (menu: Bug573.No_Such_Menu)
//
//	mxcli check --references -> Check passed!
//	mx check                 -> [CE1613] "The selected menu 'Bug573.No_Such_Menu'
//	                            no longer exists." at Simple menu bar 'bottomBar'
//
// Two gaps: nothing collected a menu reference, and no layout was
// reference-checked at all — while layouts are where menu widgets live.

func menuDocumentCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("Atlas_Core")
	md := &types.MenuDocument{ID: nextID("md"), ContainerID: mod.ID, Name: "Phone_Menu"}
	h := mkHierarchy(mod)
	withContainer(h, md.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:       func() bool { return true },
		ListMenuDocumentsFunc: func() ([]*types.MenuDocument, error) { return []*types.MenuDocument{md}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func layoutWith(t *testing.T, widget string) *ast.CreateLayoutStmt {
	t.Helper()
	src := "create layout App.Phone (layouttype: 'Phone') {\n" +
		"  scrollcontainer sc1 {\n" +
		"    region bottom { " + widget + " }\n" +
		"    region center { placeholder Main }\n" +
		"  }\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("does not parse: %v", errs)
	}
	return prog.Statements[0].(*ast.CreateLayoutStmt)
}

func TestLayoutRefs_UnknownMenuDocumentIsReported(t *testing.T) {
	ctx := menuDocumentCtx(t)
	err := validateWithContext(ctx, layoutWith(t, "simplemenubar b (Menu: Atlas_Core.Phone_Mneu)"), newScriptContext())
	if err == nil {
		t.Fatal("a simple menu bar naming a menu document that does not exist passed --references")
	}
	for _, want := range []string{"menu document not found: Atlas_Core.Phone_Mneu", "Atlas_Core.Phone_Menu"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

// CONTROL: the right name resolves, on each of the three menu widgets.
func TestLayoutRefs_KnownMenuDocumentResolves(t *testing.T) {
	ctx := menuDocumentCtx(t)
	for _, w := range []string{
		"simplemenubar b (Menu: Atlas_Core.Phone_Menu)",
		"menubar b (Menu: Atlas_Core.Phone_Menu)",
		"navigationtree b (Menu: Atlas_Core.Phone_Menu)",
	} {
		if err := validateWithContext(ctx, layoutWith(t, w), newScriptContext()); err != nil {
			t.Errorf("%s: %v", w, err)
		}
	}
}

// The ordinary shape is one script that creates the menu and the layout.
func TestLayoutRefs_MenuCreatedEarlierInTheScriptResolves(t *testing.T) {
	ctx := menuDocumentCtx(t)
	sc := newScriptContext()
	sc.collectSingle(&ast.CreateMenuStmt{Name: ast.QualifiedName{Module: "App", Name: "Bottom_Menu"}})
	if err := validateWithContext(ctx, layoutWith(t, "simplemenubar b (Menu: App.Bottom_Menu)"), sc); err != nil {
		t.Errorf("a menu created earlier in the same script must be exempt: %v", err)
	}
}

// `Menu` is only a menu reference on a menu widget.
func TestWidgetRefs_MenuOnlyCollectedFromMenuWidgets(t *testing.T) {
	refs := &widgetRefCollector{}
	refs.collectFromWidgets([]*ast.WidgetV3{{
		Name: "c", Type: "container", Properties: map[string]any{"Menu": "X.Y"},
	}})
	if len(refs.menus) != 0 {
		t.Errorf("collected %v from a container", refs.menus)
	}
}
