// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"
)

func TestCapabilityReport(t *testing.T) {
	// No client/server connected: the report still renders the curated summary
	// (and skips the live tool list rather than panicking on a nil client).
	r := (&Backend{}).CapabilityReport()
	for _, want := range []string{
		"MCP backend capabilities",
		"Studio Pro MCP server : (unknown) (unknown)",
		"Concord gap-filler    : not connected",
		"Workflows",               // authorable
		"Nanoflows, Java actions", // blocked
		"Reads (SHOW / DESCRIBE",
		"PED_MCP_CAPABILITIES.md",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("capability report missing %q in:\n%s", want, r)
		}
	}
	if strings.Contains(r, "PED tools present") {
		t.Error("no live client -> should not print a tool list")
	}
}
