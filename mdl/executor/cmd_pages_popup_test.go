// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

func newPopupPageBuilder() *pageBuilder {
	return &pageBuilder{
		moduleID:         model.ID("mod1"),
		moduleName:       "M",
		widgetScope:      map[string]model.ID{},
		paramScope:       map[string]model.ID{},
		paramEntityNames: map[string]string{},
	}
}

// Issue #661 — buildPageV3 carries pop-up dimensions from the header onto the
// page struct (which both writers serialize).
func TestBuildPageV3_PopupDimensions(t *testing.T) {
	w, h, r := 800, 480, true
	s := &ast.CreatePageStmtV3{
		Name:           ast.QualifiedName{Module: "M", Name: "P"},
		PopupWidth:     &w,
		PopupHeight:    &h,
		PopupResizable: &r,
	}
	page, err := newPopupPageBuilder().buildPageV3(s)
	if err != nil {
		t.Fatalf("buildPageV3: %v", err)
	}
	if page.PopupWidth != 800 || page.PopupHeight != 480 || !page.PopupResizable {
		t.Errorf("got %d/%d/%v, want 800/480/true", page.PopupWidth, page.PopupHeight, page.PopupResizable)
	}
}

// When the header omits pop-up properties the Mendix defaults apply.
func TestBuildPageV3_PopupDefaults(t *testing.T) {
	s := &ast.CreatePageStmtV3{Name: ast.QualifiedName{Module: "M", Name: "P"}}
	page, err := newPopupPageBuilder().buildPageV3(s)
	if err != nil {
		t.Fatalf("buildPageV3: %v", err)
	}
	if page.PopupWidth != 600 || page.PopupHeight != 600 || page.PopupResizable {
		t.Errorf("got %d/%d/%v, want 600/600/false", page.PopupWidth, page.PopupHeight, page.PopupResizable)
	}
}
