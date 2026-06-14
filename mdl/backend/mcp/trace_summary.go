// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// summarizeToolCall renders a compact target + detail for a PED tool call, for
// --mcp-verbose / --mcp-trace. target is the document/module the call acts on;
// detail (shown only at trace level) is the per-tool specifics — the update
// operations, schema element types, read paths, or created document type.
//
// arguments is whatever CallTool was handed (a map or a typed struct); it is
// round-tripped through JSON so one shape handles every call site.
func summarizeToolCall(tool string, arguments any) (target, detail string) {
	raw, err := json.Marshal(arguments)
	if err != nil {
		return "", ""
	}
	var a struct {
		DocumentName string   `json:"documentName"`
		DocumentType string   `json:"documentType"`
		Name         string   `json:"name"` // ped_create_module
		ElementTypes []string `json:"elementTypes"`
		Paths        []string `json:"paths"`
		Operations   []struct {
			Path      string `json:"path"`
			Operation struct {
				Type string `json:"type"`
			} `json:"operation"`
		} `json:"operations"`
		Documents []struct {
			DocumentName string `json:"documentName"`
		} `json:"documents"`
	}
	_ = json.Unmarshal(raw, &a)

	target = a.DocumentName
	if target == "" {
		target = a.Name
	}
	if target == "" && len(a.Documents) > 0 {
		target = a.Documents[0].DocumentName
	}

	switch tool {
	case "ped_update_document":
		// Collapse identical "type path" ops into "… ×N" (e.g. several validation
		// rules added to the same array), preserving first-seen order.
		counts := map[string]int{}
		var order []string
		for _, op := range a.Operations {
			k := op.Operation.Type + " " + op.Path
			if counts[k] == 0 {
				order = append(order, k)
			}
			counts[k]++
		}
		parts := make([]string, 0, len(order))
		for _, k := range order {
			if counts[k] > 1 {
				parts = append(parts, fmt.Sprintf("%s ×%d", k, counts[k]))
			} else {
				parts = append(parts, k)
			}
		}
		detail = strings.Join(parts, ", ")
	case "ped_get_schema":
		detail = strings.Join(a.ElementTypes, ", ")
	case "ped_read_document":
		detail = strings.Join(a.Paths, ", ")
	case "ped_create_document":
		detail = a.DocumentType
	}
	return target, truncate(detail, 100)
}

// truncate caps a detail string so a trace line stays readable.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
