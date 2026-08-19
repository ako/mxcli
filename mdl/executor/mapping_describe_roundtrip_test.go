// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// TestMappingMemberName covers the name DESCRIBE prints for a mapping element.
// The array case is the one that produced "ItemItem": the mapping element sits
// at the item object, but the script addressed the ARRAY.
func TestMappingMemberName(t *testing.T) {
	// parentPath is the enclosing object element's path (#927); these are all
	// direct children, where the member is a single segment either way.
	cases := []struct {
		name       string
		parentPath string
		jsonPath   string
		exposed    string
		want       string
	}{
		{"value under root", "(Object)", "(Object)|total", "Total", "total"},
		{"camelCase preserved", "(Object)", "(Object)|camelCase", "CamelCase", "camelCase"},
		{"array item object uses the array's key", "(Object)", "(Object)|item|(Object)", "ItemItem", "item"},
		{"value inside an array item", "(Object)|item|(Object)", "(Object)|item|(Object)|id", "_id", "id"},
		{"no JsonPath falls back (XML / message mapping)", "(Object)", "", "Total", "Total"},
		{"root has no member name", "", "(Object)", "JsonObject", "JsonObject"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mappingMemberName(c.parentPath, c.jsonPath, c.exposed); got != c.want {
				t.Errorf("mappingMemberName(%q, %q, %q) = %q, want %q", c.parentPath, c.jsonPath, c.exposed, got, c.want)
			}
		})
	}
}

// TestDescribeImportMapping_RoundTripsMemberNames is #915: DESCRIBE has to
// reproduce the script that produced the mapping. Printing ExposedName turned
// `Total = total` into `Total = Total` and `= item` into `= ItemItem`, so a diff
// of script against DESCRIBE was pure noise — and the bare `create` header meant
// the output could not be re-run at all ("import mapping already exists").
func TestDescribeImportMapping_RoundTripsMemberNames(t *testing.T) {
	mod := mkModule("Integration")
	im := &model.ImportMapping{
		BaseElement:   model.BaseElement{ID: nextID("im")},
		ContainerID:   mod.ID,
		Name:          "IMM_Payload",
		JsonStructure: "Integration.JSON_Payload",
		Elements: []*model.ImportMappingElement{{
			Kind:           "Object",
			Entity:         "Integration.Payload",
			ObjectHandling: "Create",
			ExposedName:    "JsonObject",
			JsonPath:       "(Object)",
			Children: []*model.ImportMappingElement{
				{
					Kind:        "Value",
					Attribute:   "Integration.Payload.Total",
					ExposedName: "Total",
					JsonPath:    "(Object)|total",
				},
				{
					Kind:           "Object",
					Entity:         "Integration.Line",
					Association:    "Integration.Line_Payload",
					ObjectHandling: "Create",
					ExposedName:    "ItemItem",
					JsonPath:       "(Object)|item|(Object)",
					Children: []*model.ImportMappingElement{{
						Kind:        "Value",
						Attribute:   "Integration.Line.LineId",
						ExposedName: "_id",
						JsonPath:    "(Object)|item|(Object)|id",
					}},
				},
			},
		}},
	}

	h := mkHierarchy(mod)
	withContainer(h, im.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
			return im, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeImportMapping(ctx, ast.QualifiedName{Module: "Integration", Name: "IMM_Payload"}))
	out := buf.String()

	// Re-runnable against the project it was read from.
	if !strings.Contains(out, "create or modify import mapping") {
		t.Errorf("DESCRIBE must emit a re-runnable header, got:\n%s", out)
	}
	// The raw JSON keys the script wrote, not Mendix's derived display names.
	for _, want := range []string{"= total", "= item", "= id"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in DESCRIBE output:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"= Total", "ItemItem", "= _id"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("DESCRIBE printed the derived name %q instead of the JSON key:\n%s", unwanted, out)
		}
	}
}

// TestDescribeExportMapping_RoundTripsMemberNames — the export side had the
// identical defect, which the issue did not mention.
func TestDescribeExportMapping_RoundTripsMemberNames(t *testing.T) {
	mod := mkModule("Integration")
	em := &model.ExportMapping{
		BaseElement:   model.BaseElement{ID: nextID("em")},
		ContainerID:   mod.ID,
		Name:          "EMM_Payload",
		JsonStructure: "Integration.JSON_Payload",
		Elements: []*model.ExportMappingElement{{
			Kind:        "Object",
			Entity:      "Integration.Payload",
			ExposedName: "JsonObject",
			JsonPath:    "(Object)",
			Children: []*model.ExportMappingElement{{
				Kind:        "Value",
				Attribute:   "Integration.Payload.Total",
				ExposedName: "Total",
				JsonPath:    "(Object)|total",
			}},
		}},
	}

	h := mkHierarchy(mod)
	withContainer(h, em.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetExportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ExportMapping, error) {
			return em, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeExportMapping(ctx, ast.QualifiedName{Module: "Integration", Name: "EMM_Payload"}))
	out := buf.String()

	if !strings.Contains(out, "create or modify export mapping") {
		t.Errorf("DESCRIBE must emit a re-runnable header, got:\n%s", out)
	}
	if !strings.Contains(out, "total =") {
		t.Errorf("expected the raw JSON key %q in:\n%s", "total =", out)
	}
	if strings.Contains(out, "Total =") {
		t.Errorf("DESCRIBE printed the derived ExposedName instead of the JSON key:\n%s", out)
	}
}
