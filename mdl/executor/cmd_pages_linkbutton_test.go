// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// TestBuildButtonV3_LinkButton covers the papercut where `linkbutton` failed
// with "unsupported widget type". A link button is an action button rendered as
// a link (Forms$ActionButton with RenderType "Link"), so it must build like an
// action button but with RenderMode = Link.
func TestBuildButtonV3_LinkButton(t *testing.T) {
	cases := []struct {
		widgetType string
		wantRender pages.ButtonRenderMode
	}{
		{"linkbutton", pages.ButtonRenderModeLink},
		{"actionbutton", pages.ButtonRenderModeButton},
		{"button", pages.ButtonRenderModeButton},
	}
	for _, tc := range cases {
		t.Run(tc.widgetType, func(t *testing.T) {
			pb := &pageBuilder{widgetScope: map[string]model.ID{}}
			w := &ast.WidgetV3{
				Type:       tc.widgetType,
				Name:       "btn_" + tc.widgetType,
				Properties: map[string]any{"Caption": "Go"},
			}
			widget, err := pb.buildWidgetV3(w)
			if err != nil {
				t.Fatalf("buildWidgetV3(%q) errored: %v", tc.widgetType, err)
			}
			btn, ok := widget.(*pages.ActionButton)
			if !ok {
				t.Fatalf("expected *pages.ActionButton, got %T", widget)
			}
			if btn.RenderMode != tc.wantRender {
				t.Errorf("RenderMode = %q, want %q", btn.RenderMode, tc.wantRender)
			}
			if btn.TypeName != "Forms$ActionButton" {
				t.Errorf("TypeName = %q, want Forms$ActionButton", btn.TypeName)
			}
		})
	}
}

// TestBuildButtonV3_Icon covers issue #602: a button `icon:` property builds an
// icon-collection icon (Forms$IconCollectionIcon) referencing the given
// qualified name. Quotes around the value are stripped.
func TestBuildButtonV3_Icon(t *testing.T) {
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	w := &ast.WidgetV3{
		Type:       "linkbutton",
		Name:       "btnEdit",
		Properties: map[string]any{"Caption": "Edit", "icon": "Atlas_Core.Atlas_Filled.pencil"},
	}
	widget, err := pb.buildWidgetV3(w)
	if err != nil {
		t.Fatalf("buildWidgetV3 errored: %v", err)
	}
	btn := widget.(*pages.ActionButton)
	if btn.Icon == nil {
		t.Fatal("button Icon is nil — the `icon` property was dropped (#602)")
	}
	if btn.Icon.Type != pages.IconTypeIconCollection {
		t.Errorf("Icon.Type = %q, want IconCollection", btn.Icon.Type)
	}
	if btn.Icon.Image != "Atlas_Core.Atlas_Filled.pencil" {
		t.Errorf("Icon.Image = %q, want Atlas_Core.Atlas_Filled.pencil", btn.Icon.Image)
	}
	if btn.Icon.TypeName != "Forms$IconCollectionIcon" {
		t.Errorf("Icon.TypeName = %q, want Forms$IconCollectionIcon", btn.Icon.TypeName)
	}
}

// TestOutputWidgetMDLV3_ButtonIcon verifies DESCRIBE emits `Icon: '<qn>'` for a
// button that carries an icon-collection reference, so it round-trips (#602).
func TestOutputWidgetMDLV3_ButtonIcon(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	w := rawWidget{Type: "Forms$ActionButton", Name: "btn", Icon: "Atlas_Core.Atlas_Filled.pencil"}
	outputWidgetMDLV3(ctx, w, 0)
	if got := buf.String(); !strings.Contains(got, "Icon: 'Atlas_Core.Atlas_Filled.pencil'") {
		t.Errorf("output %q does not contain the Icon clause", got)
	}
}

// TestOutputWidgetMDLV3_LinkButtonKeyword verifies DESCRIBE emits `linkbutton`
// for an action button whose RenderType is "Link", and `actionbutton` otherwise,
// so a linkbutton round-trips through describe → exec.
func TestOutputWidgetMDLV3_LinkButtonKeyword(t *testing.T) {
	cases := []struct {
		render  string
		keyword string
	}{
		{"Link", "linkbutton lnk"},
		{"Button", "actionbutton lnk"},
		{"", "actionbutton lnk"},
	}
	for _, tc := range cases {
		t.Run(tc.render, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := &ExecContext{Output: &buf}
			w := rawWidget{Type: "Forms$ActionButton", Name: "lnk", RenderMode: tc.render}
			outputWidgetMDLV3(ctx, w, 0)
			if got := buf.String(); !strings.Contains(got, tc.keyword) {
				t.Errorf("output %q does not contain %q", got, tc.keyword)
			}
		})
	}
}
