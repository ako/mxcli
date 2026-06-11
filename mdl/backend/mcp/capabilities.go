// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"sort"
	"strings"
)

// CapabilityReport renders a human-readable summary of what the MCP backend can
// author against the connected Studio Pro server — so an agent can check, before
// generating MDL, which operations are possible against this version. It combines
// the live server identity + tools/list probe with the backend's current known
// capability set.
//
// NOTE (ADR-0004, slice 1): the authorable/blocked lists are a curated snapshot of
// the backend's coverage, not yet derived from a version-keyed capability registry.
// Slice 2 replaces them with a Capabilities model so this report and the backend's
// authoring gates share one source of truth (and so a lifted PED limit updates both
// at once). Today they are kept in step with PED_MCP_CAPABILITIES.md by hand.
func (b *Backend) CapabilityReport() string {
	var sb strings.Builder
	sb.WriteString("MCP backend capabilities\n")
	sb.WriteString("========================\n")
	fmt.Fprintf(&sb, "Studio Pro MCP server : %s %s\n", orUnknown(b.server.Name), orUnknown(b.server.Version))
	concord := "not connected — DROP of standalone docs (enum/microflow/page/…) is unavailable"
	if b.concord != nil {
		concord = "connected"
	}
	fmt.Fprintf(&sb, "Concord gap-filler    : %s\n\n", concord)

	sb.WriteString("Authorable over MCP:\n")
	for _, s := range capAuthorable {
		fmt.Fprintf(&sb, "  ✓ %s\n", s)
	}
	sb.WriteString("\nNot authorable (PED limits this version):\n")
	for _, s := range capBlocked {
		fmt.Fprintf(&sb, "  ✗ %s\n", s)
	}
	sb.WriteString("\nReads (SHOW / DESCRIBE of any document type): always available from the local .mpr.\n")

	if b.client != nil {
		if tools, err := b.client.ListTools(); err == nil {
			sort.Strings(tools)
			fmt.Fprintf(&sb, "\nPED tools present (%d): %s\n", len(tools), strings.Join(tools, ", "))
		}
	}
	sb.WriteString("\nDetail & per-version onboarding: docs/03-development/PED_MCP_CAPABILITIES.md\n")
	return sb.String()
}

func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

// capAuthorable / capBlocked are the curated capability snapshot (see the slice-1
// note on CapabilityReport). Keep in step with PED_MCP_CAPABILITIES.md until the
// registry (ADR-0004) backs them.
var capAuthorable = []string{
	"Modules — CREATE",
	"Entities — CREATE/DROP; ALTER add/drop/rename attribute, entity & attribute documentation; generalization (extends)",
	"Associations — CREATE/DROP within a module",
	"Enumerations — CREATE (DROP via Concord)",
	"Constants — CREATE / CREATE OR MODIFY / DROP (String/Integer/Decimal/Boolean/DateTime)",
	"Microflows — CREATE (broad activity + control-flow coverage)",
	"Pages — CREATE + ALTER (widget coverage grows per type)",
	"Workflows — CREATE / CREATE OR REPLACE / DROP / ALTER (full, any nesting depth)",
	"View entities — CREATE",
	"Documents into folders — create <doc> … folder 'A/B' (nested; folder auto-created)",
}

var capBlocked = []string{
	"Nanoflows, Java actions, Business-event services — CREATE (off PED's create whitelist)",
	"Security (roles, access, demo users), Navigation, OData/REST, mappings, settings — no PED write path",
	"ALTER … MODIFY attribute type — PED can't change a type in place (would drop the column)",
	"MOVE / re-parent a document, empty CREATE FOLDER — PED can't re-parent or create empty folders",
	"Pages into folders — pg_write_page has no folderPath (page lands at the module root)",
}
