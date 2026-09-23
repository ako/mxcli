// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#550 — the builder half. DESCRIBE now emits `Password:`,
// `Validation:` / `ValidationMessage:` and a DataView's `ReadOnlyStyle:`, so the
// builder has to consume them or the round trip still loses the value — just one
// layer further along.
//
// The DataView case is the same shape as ako/mxcli#490's checkbox: the property
// parses, `mxcli check` accepts it (staticWidgetKnownProps is deliberately a
// union across widget types, not per-type, so a DataView carrying ReadOnlyStyle
// draws no MDL-WIDGET07 warning) and every layer below drops it.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

func newPropsBuilder() *pageBuilder {
	return &pageBuilder{widgetScope: map[string]model.ID{}}
}

func TestBuildTextBox_Password(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{"unset stays plaintext", nil, false},
		{"true", true, true},
		{"string true, as describe emits it", "true", true},
		{"false", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pb := newPropsBuilder()
			w := &ast.WidgetV3{Name: "tbSecret", Type: "textbox", Properties: map[string]any{}}
			if tc.value != nil {
				w.Properties["Password"] = tc.value
			}
			tb, err := pb.buildTextBoxV3(w)
			if err != nil {
				t.Fatalf("buildTextBoxV3: %v", err)
			}
			if tb.IsPassword != tc.want {
				t.Errorf("IsPassword = %v, want %v — a password field must not "+
					"round-trip into a plaintext one", tb.IsPassword, tc.want)
			}
		})
	}
}

func TestBuildTextBox_Validation(t *testing.T) {
	pb := newPropsBuilder()
	w := &ast.WidgetV3{Name: "tbSecret", Type: "textbox", Properties: map[string]any{
		"Validation":        "length(toString($value)) > 0",
		"ValidationMessage": "Required",
	}}
	tb, err := pb.buildTextBoxV3(w)
	if err != nil {
		t.Fatalf("buildTextBoxV3: %v", err)
	}
	if tb.ValidationExpression != "length(toString($value)) > 0" {
		t.Errorf("ValidationExpression = %q, want the authored expression", tb.ValidationExpression)
	}
	if tb.ValidationMessage != "Required" {
		t.Errorf("ValidationMessage = %q, want Required", tb.ValidationMessage)
	}
}

// Control: no validation authored means none carried, so the writer keeps
// emitting the empty Forms$WidgetValidation Studio Pro stores on every widget.
func TestBuildTextBox_NoValidation(t *testing.T) {
	pb := newPropsBuilder()
	tb, err := pb.buildTextBoxV3(&ast.WidgetV3{
		Name: "tbPlain", Type: "textbox", Properties: map[string]any{},
	})
	if err != nil {
		t.Fatalf("buildTextBoxV3: %v", err)
	}
	if tb.ValidationExpression != "" || tb.ValidationMessage != "" {
		t.Errorf("an unauthored validation was invented: %q / %q",
			tb.ValidationExpression, tb.ValidationMessage)
	}
}

func TestBuildDataView_ReadOnlyStyle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"unset stays unset (writer keeps the stored default)", nil, ""},
		{"text", "Text", "Text"},
		{"control", "Control", "Control"},
		{"inherit", "Inherit", "Inherit"},
		{"lowercase is canonicalised", "text", "Text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pb := newPropsBuilder()
			w := &ast.WidgetV3{Name: "dvMain", Type: "dataview", Properties: map[string]any{}}
			if tc.value != nil {
				w.Properties["ReadOnlyStyle"] = tc.value
			}
			dv, err := pb.buildDataViewV3(w)
			if err != nil {
				t.Fatalf("buildDataViewV3: %v", err)
			}
			if dv.ReadOnlyStyle != tc.want {
				t.Errorf("ReadOnlyStyle = %q, want %q", dv.ReadOnlyStyle, tc.want)
			}
		})
	}
}

// Same refusal as the checkbox: an unknown member is a property Studio Pro
// cannot resolve, and mxbuild tolerates it — so the build stays green and the
// project will not open.
func TestBuildDataView_ReadOnlyStyleRejectsUnknownValue(t *testing.T) {
	pb := newPropsBuilder()
	_, err := pb.buildDataViewV3(&ast.WidgetV3{
		Name: "dvMain", Type: "dataview",
		Properties: map[string]any{"ReadOnlyStyle": "ReadOnly"},
	})
	if err == nil {
		t.Fatal("an unknown ReadOnlyStyle was accepted — it would be written into the document")
	}
	if !strings.Contains(err.Error(), "Control") {
		t.Errorf("the error should name the accepted values, got: %v", err)
	}
}
