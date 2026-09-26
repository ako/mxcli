// SPDX-License-Identifier: Apache-2.0

// Tests for Issue #21: ComboBox association Attribute written as pointer; DESCRIBE shows CaptionAttribute.
//
// Root cause: extractCustomWidgetAttribute blindly scans all properties for the first AttributeRef.
// In association mode, attributeAssociation uses EntityRef (not AttributeRef), so the scan
// skips it and finds optionsSourceAssociationCaptionAttribute's AttributeRef ("Name") instead.
//
// Fix: extractCustomWidgetPropertyAssociation — generic property-key-aware association extractor,
// symmetric to the existing extractCustomWidgetPropertyAttributeRef.
package executor

import (
	"context"
	"testing"
)

// buildComboBoxAssocWidget builds a mock CustomWidget map matching the BSON structure
// written by the pluggable widget engine for COMBOBOX in association mode.
// TypePointer IDs are plain strings (extractBinaryID accepts strings directly).
func buildComboBoxAssocWidget(assocPath, captionAttrPath string) map[string]any {
	const (
		idOptionsSourceType = "type-id-001"
		idAttrAssociation   = "type-id-002"
		idAssocDataSource   = "type-id-003"
		idCaptionAttribute  = "type-id-004"
	)

	widgetType := map[string]any{
		"WidgetId": "com.mendix.widget.web.combobox.Combobox",
		"ObjectType": map[string]any{
			"PropertyTypes": []any{
				map[string]any{"$ID": idOptionsSourceType, "PropertyKey": "optionsSourceType"},
				map[string]any{"$ID": idAttrAssociation, "PropertyKey": "attributeAssociation"},
				map[string]any{"$ID": idAssocDataSource, "PropertyKey": "optionsSourceAssociationDataSource"},
				map[string]any{"$ID": idCaptionAttribute, "PropertyKey": "optionsSourceAssociationCaptionAttribute"},
			},
		},
	}

	// Properties mirror what setAssociationRef and setAttributeRef produce in the engine.
	properties := []any{
		// optionsSourceType = "association"
		map[string]any{
			"TypePointer": idOptionsSourceType,
			"Value":       map[string]any{"PrimitiveValue": "association"},
		},
		// attributeAssociation — uses EntityRef (written by opAssociation / setAssociationRef)
		map[string]any{
			"TypePointer": idAttrAssociation,
			"Value": map[string]any{
				"EntityRef": map[string]any{
					"$Type": "DomainModels$IndirectEntityRef",
					"Steps": []any{
						int32(2), // version marker
						map[string]any{
							"$Type":             "DomainModels$EntityRefStep",
							"Association":       assocPath,
							"DestinationEntity": "MyFirstModule.Category",
						},
					},
				},
			},
		},
		// optionsSourceAssociationDataSource
		map[string]any{
			"TypePointer": idAssocDataSource,
			"Value": map[string]any{
				"DataSource": map[string]any{
					"$Type":     "CustomWidgets$CustomWidgetXPathSource",
					"EntityRef": map[string]any{"Entity": "MyFirstModule.Category"},
				},
			},
		},
		// optionsSourceAssociationCaptionAttribute — uses AttributeRef (written by opAttribute)
		map[string]any{
			"TypePointer": idCaptionAttribute,
			"Value": map[string]any{
				"AttributeRef": map[string]any{
					"$Type":     "DomainModels$AttributeRef",
					"Attribute": captionAttrPath,
				},
			},
		},
	}

	return map[string]any{
		"Type":   widgetType,
		"Object": map[string]any{"Properties": properties},
	}
}

// TestExtractCustomWidgetPropertyAssociation_ReturnsAssociationName verifies that
// the generic association extractor returns the short association name from the
// named property's EntityRef.Steps[1].Association.
// Regression test for Issue #21.
func TestExtractCustomWidgetPropertyAssociation_ReturnsAssociationName(t *testing.T) {
	e := &Executor{}
	w := buildComboBoxAssocWidget("MyFirstModule.Task_Category", "MyFirstModule.Category.Name")

	got := e.extractCustomWidgetPropertyAssociation(w, "attributeAssociation")

	if got != "Task_Category" {
		t.Errorf("extractCustomWidgetPropertyAssociation(w, \"attributeAssociation\") = %q, want \"Task_Category\"", got)
	}
}

// TestExtractCustomWidgetPropertyAssociation_WrongKey returns empty for a non-matching key.
func TestExtractCustomWidgetPropertyAssociation_WrongKey(t *testing.T) {
	e := &Executor{}
	w := buildComboBoxAssocWidget("MyFirstModule.Task_Category", "MyFirstModule.Category.Name")

	got := e.extractCustomWidgetPropertyAssociation(w, "nonExistentProperty")

	if got != "" {
		t.Errorf("extractCustomWidgetPropertyAssociation with wrong key = %q, want empty", got)
	}
}

// TestExtractCustomWidgetPropertyAssociation_NilEntityRef returns empty when EntityRef is nil.
func TestExtractCustomWidgetPropertyAssociation_NilEntityRef(t *testing.T) {
	e := &Executor{}
	w := map[string]any{
		"Type": map[string]any{
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{"$ID": "id-1", "PropertyKey": "attributeAssociation"},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{
					"TypePointer": "id-1",
					"Value":       map[string]any{"EntityRef": nil},
				},
			},
		},
	}

	got := e.extractCustomWidgetPropertyAssociation(w, "attributeAssociation")

	if got != "" {
		t.Errorf("extractCustomWidgetPropertyAssociation with nil EntityRef = %q, want empty", got)
	}
}

// TestExtractCustomWidgetPropertyAssociation_DoesNotReturnCaptionAttribute confirms that
// the fixer does not accidentally return the CaptionAttribute value.
// This documents the exact bug: the old generic scan returned "Name" (CaptionAttribute)
// because it found the first AttributeRef, which belonged to CaptionAttribute, not Attribute.
func TestExtractCustomWidgetPropertyAssociation_DoesNotReturnCaptionAttribute(t *testing.T) {
	e := &Executor{}
	w := buildComboBoxAssocWidget("MyFirstModule.Task_Category", "MyFirstModule.Category.Name")

	got := e.extractCustomWidgetPropertyAssociation(w, "attributeAssociation")

	if got == "Name" {
		t.Errorf("extractCustomWidgetPropertyAssociation returned CaptionAttribute value %q; this is the original bug (Issue #21)", got)
	}
}

// buildDataGridOnClickWidget builds a mock DataGrid2 CustomWidget map with an
// onClick property carrying the given action ($Type). Mirrors the BSON the
// pluggable engine writes (property resolved via TypePointer → PropertyKey).
func buildDataGridOnClickWidget(actionType, microflow string) map[string]any {
	const idOnClick = "type-id-onclick"
	widgetType := map[string]any{
		"WidgetId": "com.mendix.widget.web.datagrid.Datagrid",
		"ObjectType": map[string]any{
			"PropertyTypes": []any{
				map[string]any{"$ID": idOnClick, "PropertyKey": "onClick"},
			},
		},
	}
	action := map[string]any{"$Type": actionType}
	if microflow != "" {
		action["MicroflowSettings"] = map[string]any{
			"$Type":     "Forms$MicroflowSettings",
			"Microflow": microflow,
		}
	}
	return map[string]any{
		"Type": widgetType,
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{"TypePointer": idOnClick, "Value": map[string]any{"Action": action}},
			},
		},
	}
}

// TestCustomWidgetPropertyActionMap covers the ledger #67 DESCRIBE read gap: a
// pluggable widget's onClick action is read back so a describe round-trip
// re-emits it. A NoAction (the default) reads as unset.
func TestCustomWidgetPropertyActionMap(t *testing.T) {
	ex := &Executor{}
	ctx := ex.newExecContext(context.Background())

	// Microflow action → returned, with the microflow reachable for rendering.
	w := buildDataGridOnClickWidget("Forms$MicroflowClientAction", "L67.OnRowClick")
	got := customWidgetPropertyActionMap(ctx, w, "onClick")
	if got == nil {
		t.Fatal("onClick microflow action not read back — DESCRIBE would drop it (#67)")
	}
	if extractString(got["$Type"]) != "Forms$MicroflowClientAction" {
		t.Errorf("action $Type = %q, want Forms$MicroflowClientAction", extractString(got["$Type"]))
	}

	// A default NoAction reads as unset (nil) so DESCRIBE stays clean.
	wNo := buildDataGridOnClickWidget("Forms$NoAction", "")
	if customWidgetPropertyActionMap(ctx, wNo, "onClick") != nil {
		t.Error("NoAction should read as unset (nil)")
	}
}

// ako/mxcli#662: DESCRIBE emitted an association bound through a
// generalization as a bare name — `Attribute: UserRoles` on a page over
// Administration.Account (extends System.User) — and exec qualified it with the
// page entity's module, writing `Administration.UserRoles` → CE1613. An
// association declared outside the context entity's module keeps its module.
func TestParseRawWidget_ComboBoxAssociation_QualifiedOutsideContextModule(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	tests := []struct {
		name, assoc, entityCtx, want string
	}{
		{"inherited from another module (#662)", "System.UserRoles", "Administration.Account", "System.UserRoles"},
		{"declared in the context's module stays bare", "MyFirstModule.Task_Category", "MyFirstModule.Task", "Task_Category"},
		{"unknown context keeps the qualifier", "MyFirstModule.Task_Category", "", "MyFirstModule.Task_Category"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := buildComboBoxAssocWidget(tc.assoc, "System.UserRole.Name")
			w["$Type"] = "CustomWidgets$CustomWidget"
			w["Name"] = "comboBox1"
			got := parseRawWidget(ctx, w, tc.entityCtx)
			if len(got) != 1 {
				t.Fatalf("parseRawWidget returned %d widgets, want 1", len(got))
			}
			if got[0].Content != tc.want {
				t.Errorf("Content = %q, want %q", got[0].Content, tc.want)
			}
		})
	}
}
