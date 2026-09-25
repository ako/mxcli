// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#664: a ComboBox in association mode whose caption is an
// EXPRESSION — Administration.Account_New's comboBox2 (Administration v4.3.2,
// Mendix 11.13.0) stores
//
//	optionsSourceAssociationCaptionType       = "expression"
//	optionsSourceAssociationCaptionExpression = "$currentObject/Description"
//
// DESCRIBE read only the attribute caption, so the widget came back with no
// caption at all and exec produced
//
//	[CE0642] "Property 'Caption' is required." at Combo box 'comboBox2'
func comboBoxWithExpressionCaption() map[string]any {
	w := buildComboBoxAssocWidget("System.User_TimeZone", "")
	pts := w["Type"].(map[string]any)["ObjectType"].(map[string]any)
	pts["PropertyTypes"] = append(pts["PropertyTypes"].([]any),
		map[string]any{"$ID": "type-id-005", "PropertyKey": "optionsSourceAssociationCaptionType"},
		map[string]any{"$ID": "type-id-006", "PropertyKey": "optionsSourceAssociationCaptionExpression"},
	)
	obj := w["Object"].(map[string]any)
	props := obj["Properties"].([]any)
	// Drop the attribute caption: an expression-caption widget stores none.
	kept := props[:0]
	for _, p := range props {
		if p.(map[string]any)["TypePointer"] != "type-id-004" {
			kept = append(kept, p)
		}
	}
	obj["Properties"] = append(kept,
		map[string]any{"TypePointer": "type-id-005", "Value": map[string]any{"PrimitiveValue": "expression"}},
		map[string]any{"TypePointer": "type-id-006", "Value": map[string]any{"Expression": "$currentObject/Description"}},
	)
	w["$Type"] = "CustomWidgets$CustomWidget"
	w["Name"] = "comboBox2"
	return w
}

func TestDescribeComboBox_ExpressionCaption(t *testing.T) {
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	raw := parseRawWidget(ctx, comboBoxWithExpressionCaption(), "Administration.Account")
	outputWidgetMDLV3(ctx, raw[0], 1)
	got := buf.String()
	for _, want := range []string{
		"optionsSourceAssociationCaptionType: expression",
		"optionsSourceAssociationCaptionExpression: '$currentObject/Description'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output lacks %q — the caption is dropped and exec fails CE0642:\n%s", want, got)
		}
	}
	if strings.Contains(got, "CaptionAttribute:") {
		t.Errorf("an expression caption must not also emit CaptionAttribute:\n%s", got)
	}
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + got + "}\n"); len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, got)
	}
}

// The attribute caption is unchanged.
func TestDescribeComboBox_AttributeCaptionUnchanged(t *testing.T) {
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	w := buildComboBoxAssocWidget("MyFirstModule.Task_Category", "MyFirstModule.Category.Name")
	w["$Type"] = "CustomWidgets$CustomWidget"
	w["Name"] = "cb"
	raw := parseRawWidget(ctx, w, "MyFirstModule.Task")
	outputWidgetMDLV3(ctx, raw[0], 1)
	got := buf.String()
	if !strings.Contains(got, "CaptionAttribute: Name") || strings.Contains(got, "CaptionExpression") {
		t.Errorf("attribute caption changed:\n%s", got)
	}
}
