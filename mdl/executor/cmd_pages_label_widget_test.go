// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A Studio Pro Forms$Label, as FeedbackModule.ShareFeedback stores it
// (Feedback v4.0.2): a name, a Texts$Text caption, and an appearance.
func storedLabelWidget() map[string]any {
	return map[string]any{
		"$Type": "Forms$Label",
		"Name":  "label4",
		"Appearance": map[string]any{
			"$Type": "Forms$Appearance",
			"Class": "alert alert-warning",
			"Style": "width:100%;",
		},
		"Caption": map[string]any{
			"$Type": "Texts$Text",
			"Items": []any{int32(3), map[string]any{
				"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "Attachment",
			}},
		},
		"TabIndex": int32(0),
	}
}

// DESCRIBE wrote `statictext (Content: 'Attachment')` — no name, so exec of
// the output failed `extraneous input '(' expecting the start of a statement`
// (3 of 17 pages of a stock Administration + Feedback project). It must emit a
// named `label` that parses and keeps the appearance.
func TestDescribeLabelWidget_EmitsNamedLabel(t *testing.T) {
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	raw := parseRawWidget(ctx, storedLabelWidget())
	if len(raw) != 1 {
		t.Fatalf("parseRawWidget returned %d widgets", len(raw))
	}
	outputWidgetMDLV3(ctx, raw[0], 1)
	got := buf.String()
	for _, want := range []string{"label label4", "Content: 'Attachment'", "Class: 'alert alert-warning'", "Style: 'width:100%;'"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output lacks %q:\n%s", want, got)
		}
	}
	src := "create page M.P (Title: 'x', Layout: A.L) {\n" + got + "}\n"
	if _, errs := visitor.Build(src); len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, src)
	}
}

// `label` builds a Forms$Label — not Forms$Text, which Mendix 11 cannot load.
func TestBuildLabelWidget_WritesFormsLabel(t *testing.T) {
	pb := newTestPageBuilderForLabel()
	w := &ast.WidgetV3{Type: "label", Name: "label4", Properties: map[string]any{"Content": "Attachment", "Class": "text-semibold"}}
	built, err := pb.buildWidgetV3(w)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	lbl, ok := built.(*pages.Label)
	if !ok {
		t.Fatalf("built %T, want *pages.Label", built)
	}
	if lbl.TypeName != "Forms$Label" || lbl.Name != "label4" {
		t.Errorf("TypeName/Name = %q/%q", lbl.TypeName, lbl.Name)
	}
	if lbl.Caption == nil || lbl.Caption.Translations["en_US"] != "Attachment" {
		t.Errorf("Caption = %+v", lbl.Caption)
	}
	if lbl.Class != "text-semibold" {
		t.Errorf("Class = %q", lbl.Class)
	}
}

func newTestPageBuilderForLabel() *pageBuilder {
	return &pageBuilder{
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
}
