// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Pages use a SEPARATE write protocol (pg_write_page), not PED — the PED tools
// are forbidden for pages. pg_write_page takes a high-level widget tree; this
// file maps the executor's pages.Page (shell + LayoutCall slots + pages.Widget
// tree) onto that tree. Widget/action coverage grows one type at a time, like
// the microflow activities; unmapped widgets/actions are rejected with a clear
// error. See docs/03-development/PED_MCP_CAPABILITIES.md.

// CreatePage creates a page via pg_write_page.
func (b *Backend) CreatePage(page *pages.Page) error {
	mod, err := b.GetModule(page.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for page %q: %w", page.Name, err)
	}

	layout := "Atlas_Core.Atlas_Default"
	if page.LayoutCall != nil && page.LayoutCall.LayoutName != "" {
		layout = page.LayoutCall.LayoutName
	}

	// Build the layout-slot content widgets. Each LayoutCall argument fills one
	// layout slot; pg_write_page wraps each as a Pages$Content with a slot name.
	slotWidgets := make([]any, 0)
	if page.LayoutCall != nil {
		for i, arg := range page.LayoutCall.Arguments {
			if arg.Widget == nil {
				continue
			}
			w, err := b.mapPageWidget(arg.Widget)
			if err != nil {
				return fmt.Errorf("page %q: %w", page.Name, err)
			}
			slotWidgets = append(slotWidgets, map[string]any{
				"$Type":   "Pages$Content",
				"slot":    slotName(arg, i),
				"widgets": []any{w},
			})
		}
	}
	if len(slotWidgets) == 0 {
		return fmt.Errorf("page %q: no content widgets — an empty page is not supported by the MCP backend", page.Name)
	}

	content := map[string]any{
		"title":      textValue(page.Title),
		"layout":     layout,
		"parameters": pageParameters(page.Parameters),
		"variables":  []any{},
		"widgets":    slotWidgets,
	}

	if err := b.pgWritePage(mod.Name, page.Name, content); err != nil {
		return err
	}
	if page.ID == "" {
		page.ID = model.ID("mcp~page~" + mod.Name + "~" + page.Name)
	}
	b.sessionPages = append(b.sessionPages, page)
	return nil
}

// ListPages returns pages from the local reader merged with those created over
// MCP this session (for the executor's duplicate/role checks).
func (b *Backend) ListPages() ([]*pages.Page, error) {
	local, err := b.reader.ListPages()
	if err != nil {
		return nil, err
	}
	if len(b.sessionPages) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionPages))
	out := make([]*pages.Page, 0, len(local)+len(b.sessionPages))
	for _, p := range b.sessionPages {
		seen[string(p.ContainerID)+"."+p.Name] = true
		out = append(out, p)
	}
	for _, p := range local {
		if !seen[string(p.ContainerID)+"."+p.Name] {
			out = append(out, p)
		}
	}
	return out, nil
}

// DeletePage drops a page via Concord's delete_document (PED has no delete tool).
// Requires --mcp-concord; errors clearly otherwise.
func (b *Backend) DeletePage(id model.ID) error {
	page, err := b.GetPage(id)
	if err != nil {
		return fmt.Errorf("resolve page %s for DROP: %w", id, err)
	}
	modName, err := b.moduleNameForContainer(page.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for page %q: %w", page.Name, err)
	}
	return b.concordDeleteDocument(modName, page.Name)
}

// Page-related reads delegate to the local reader (the executor's page builder
// resolves layouts/snippets through these; without them the layout won't
// resolve and the widget tree is dropped).
func (b *Backend) GetPage(id model.ID) (*pages.Page, error)     { return b.reader.GetPage(id) }
func (b *Backend) ListLayouts() ([]*pages.Layout, error)        { return b.reader.ListLayouts() }
func (b *Backend) GetLayout(id model.ID) (*pages.Layout, error) { return b.reader.GetLayout(id) }
func (b *Backend) ListSnippets() ([]*pages.Snippet, error)      { return b.reader.ListSnippets() }
func (b *Backend) ListBuildingBlocks() ([]*pages.BuildingBlock, error) {
	return b.reader.ListBuildingBlocks()
}
func (b *Backend) ListPageTemplates() ([]*pages.PageTemplate, error) {
	return b.reader.ListPageTemplates()
}

// ListFolders delegates to the local reader (the container hierarchy needs it to
// map folder-contained documents — e.g. Atlas layouts — to their module).
func (b *Backend) ListFolders() ([]*types.FolderInfo, error) {
	in, err := b.reader.ListFolders()
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.FolderInfo, len(in))
	for i, f := range in {
		out[i] = &types.FolderInfo{ID: f.ID, ContainerID: f.ContainerID, Name: f.Name}
	}
	return out, nil
}

// slotName resolves the layout-slot name for a layout-call argument. The
// executor carries only the placeholder ID; for the common single-slot Atlas
// layouts the main slot is "Main". (Multi-slot resolution from the layout's
// placeholders is future work.)
func slotName(_ *pages.LayoutCallArgument, index int) string {
	if index == 0 {
		return "Main"
	}
	return fmt.Sprintf("Slot%d", index)
}

// pageParameters maps page parameters onto pg PageParameter objects.
func pageParameters(params []*pages.PageParameter) []any {
	out := make([]any, 0, len(params))
	for _, p := range params {
		po := map[string]any{"$Type": "Pages$PageParameter", "name": p.Name}
		if p.EntityName != "" {
			po["entity"] = p.EntityName
		}
		out = append(out, po)
	}
	return out
}

// pgReadPage reads a page's current high-level content tree via pg_read_page.
// The result is the same content shape pg_write_page accepts, so it round-trips
// for read-modify-write (ALTER PAGE).
func (b *Backend) pgReadPage(moduleName, pageName string) (map[string]any, error) {
	res, err := b.client.CallTool("pg_read_page", map[string]any{
		"moduleName": moduleName,
		"pageName":   pageName,
	})
	if err != nil {
		return nil, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return nil, fmt.Errorf("pg_read_page %s.%s: %s", moduleName, pageName, text)
	}
	var content map[string]any
	if err := json.Unmarshal([]byte(text), &content); err != nil {
		return nil, fmt.Errorf("pg_read_page %s.%s: parsing content: %w", moduleName, pageName, err)
	}
	return content, nil
}

// pgWritePage calls the pg_write_page tool and surfaces failures (which, unlike
// ped_*, are reported as a non-"success" result text).
func (b *Backend) pgWritePage(moduleName, pageName string, content any) error {
	res, err := b.client.CallTool("pg_write_page", map[string]any{
		"moduleName": moduleName,
		"pageName":   pageName,
		"content":    content,
	})
	if err != nil {
		return err
	}
	text := pedStripReminder(res.Text)
	if res.IsError || !strings.Contains(strings.ToLower(text), "success") {
		return fmt.Errorf("pg_write_page %s.%s: %s", moduleName, pageName, text)
	}
	b.markDirty(moduleName)
	return nil
}
