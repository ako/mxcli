// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// Concord is a Studio Pro extension whose MCP server provides capabilities the
// built-in PED server lacks (delete, save, validate, run). The MCP backend uses
// PED for authoring by default and reaches for Concord ONLY for these gaps. The
// Concord client is optional (nil unless configured via WithConcord); every
// Concord-backed operation errors clearly when it is not configured.

// concordCall invokes a Concord tool, returning the result text. It errors if
// Concord is not configured or the tool reports failure. Concord tools report
// failure either via the MCP isError flag or a JSON body with "error"/"success":
// false in the result text.
func (b *Backend) concordCall(tool string, args map[string]any) (string, error) {
	if b.concord == nil {
		return "", fmt.Errorf("%s requires the Concord MCP server — pass --mcp-concord (Concord provides capabilities the built-in server lacks)", tool)
	}
	res, err := b.concord.CallTool(tool, args)
	if err != nil {
		return "", err
	}
	text := pedStripReminder(res.Text)
	if res.IsError || concordFailed(text) {
		return "", fmt.Errorf("%s: %s", tool, text)
	}
	return text, nil
}

// concordFailed reports whether a Concord result body signals failure. Concord
// returns JSON like {"error":"..."} or {"success":false,...} on failure and
// {"success":true,...}/{"status":"ok",...} on success.
func concordFailed(text string) bool {
	t := strings.TrimSpace(text)
	return strings.Contains(t, `"error"`) || strings.Contains(t, `"success":false`) || strings.Contains(t, `"success": false`)
}

// SaveAll persists every unsaved change in Studio Pro (Concord save_all, the
// equivalent of Ctrl+S). PED has no save tool, so this is the only way to flush
// PED-authored in-memory writes to disk from mxcli.
func (b *Backend) SaveAll() error {
	_, err := b.concordCall("save_all", map[string]any{})
	return err
}

// concordDeleteDocument removes a standalone document (enumeration, microflow,
// page) via Concord's delete_document. PED has no delete tool, so DROP of these
// document types is only possible through Concord.
func (b *Backend) concordDeleteDocument(moduleName, docName string) error {
	_, err := b.concordCall("delete_document", map[string]any{
		"module_name":   moduleName,
		"document_name": docName,
	})
	return err
}

// moduleNameForContainer resolves a container (module) ID to its module name,
// session-aware so freshly created modules resolve too.
func (b *Backend) moduleNameForContainer(containerID model.ID) (string, error) {
	mod, err := b.GetModule(containerID)
	if err != nil {
		return "", err
	}
	return mod.Name, nil
}
