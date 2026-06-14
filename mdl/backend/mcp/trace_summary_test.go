// SPDX-License-Identifier: Apache-2.0

package mcp

import "testing"

func TestSummarizeToolCall(t *testing.T) {
	cases := []struct {
		name       string
		tool       string
		args       any
		wantTarget string
		wantDetail string
	}{
		{
			name:       "create module → name is the target",
			tool:       "ped_create_module",
			args:       map[string]any{"name": "Sales"},
			wantTarget: "Sales",
			wantDetail: "",
		},
		{
			name:       "get schema → element types",
			tool:       "ped_get_schema",
			args:       map[string]any{"elementTypes": []string{"DomainModels$Entity", "DomainModels$Attribute"}},
			wantTarget: "",
			wantDetail: "DomainModels$Entity, DomainModels$Attribute",
		},
		{
			name: "update document → operations (typed struct round-trips)",
			tool: "ped_update_document",
			args: map[string]any{
				"documentName": "Sales",
				"operations": []pedOpEntry{
					{Path: "/entities", Operation: pedOperation{Type: "add"}},
				},
			},
			wantTarget: "Sales",
			wantDetail: "add /entities",
		},
		{
			name: "update document → identical ops collapse to ×N",
			tool: "ped_update_document",
			args: map[string]any{
				"documentName": "Sales",
				"operations": []pedOpEntry{
					{Path: "/entities/0/validationRules", Operation: pedOperation{Type: "add"}},
					{Path: "/entities/0/validationRules", Operation: pedOperation{Type: "add"}},
				},
			},
			wantTarget: "Sales",
			wantDetail: "add /entities/0/validationRules ×2",
		},
		{
			name:       "check errors → first document name is the target",
			tool:       "ped_check_errors",
			args:       map[string]any{"documents": []map[string]any{{"documentName": "Sales"}}},
			wantTarget: "Sales",
			wantDetail: "",
		},
		{
			name:       "create document → document type is the detail",
			tool:       "ped_create_document",
			args:       map[string]any{"documentName": "Sales.MyEnum", "documentType": "Enumerations$Enumeration"},
			wantTarget: "Sales.MyEnum",
			wantDetail: "Enumerations$Enumeration",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target, detail := summarizeToolCall(c.tool, c.args)
			if target != c.wantTarget {
				t.Errorf("target = %q, want %q", target, c.wantTarget)
			}
			if detail != c.wantDetail {
				t.Errorf("detail = %q, want %q", detail, c.wantDetail)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("short string altered: %q", got)
	}
	long := truncate(string(make([]byte, 0))+"abcdefghij", 5)
	if len([]rune(long)) != 5 || long[len(long)-len("…"):] != "…" {
		t.Errorf("truncate did not cap with ellipsis: %q", long)
	}
}
