// SPDX-License-Identifier: Apache-2.0

// cmd_list_folders.go — LIST FOLDERS, the layout view.
//
// MOVE could place a document in a folder, but nothing could read the placement
// back: SHOW STRUCTURE is flat at every depth and DESCRIBE reports one document
// at a time. So a move could not be confirmed, and an intended layout could not
// be diffed against the real one, without opening the .mpr as SQLite
// (mxcli-formula1 #32).
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// folderEntry is one folder and the documents directly inside it.
type folderEntry struct {
	Module string
	Path   string // "" for the module root
	Docs   []folderDoc
}

// folderDoc is a document placed in a folder, named by kind and name so the
// listing reads the way the model does.
type folderDoc struct {
	Kind string
	Name string
}

// listFolders handles LIST FOLDERS [IN Module].
func listFolders(ctx *ExecContext, s *ast.ShowStmt) error {
	// execShow has already refused an unconnected project.
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}
	folders, err := ctx.Backend.ListFolders()
	if err != nil {
		return mdlerrors.NewBackend("list folders", err)
	}

	docsByContainer := documentsByContainer(ctx, h)

	// Every folder is a row, even an empty one — an empty folder is part of the
	// layout and a diff that hid it would not round-trip.
	entries := make(map[string]*folderEntry)
	key := func(mod, path string) string { return mod + "\x00" + path }
	add := func(mod, path string) *folderEntry {
		k := key(mod, path)
		if e, ok := entries[k]; ok {
			return e
		}
		e := &folderEntry{Module: mod, Path: path}
		entries[k] = e
		return e
	}

	for _, f := range folders {
		if f == nil {
			continue
		}
		mod := h.GetModuleName(h.FindModuleID(f.ID))
		if mod == "" || !moduleMatches(mod, s.InModule) {
			continue
		}
		e := add(mod, h.BuildFolderPath(f.ID))
		e.Docs = append(e.Docs, docsByContainer[f.ID]...)
	}

	// Documents still at the module root, so "what is not filed yet" is visible
	// in the same view rather than by subtraction.
	for _, m := range modulesInScope(ctx, s.InModule) {
		if docs := docsByContainer[m.ID]; len(docs) > 0 {
			add(m.Name, "").Docs = append(add(m.Name, "").Docs, docs...)
		}
	}

	ordered := make([]*folderEntry, 0, len(entries))
	for _, e := range entries {
		sortFolderDocs(e.Docs)
		ordered = append(ordered, e)
	}
	// Module, then path: the root ("") sorts first inside each module, which is
	// where an unfiled document is most worth noticing.
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Module != ordered[j].Module {
			return ordered[i].Module < ordered[j].Module
		}
		return ordered[i].Path < ordered[j].Path
	})

	if ctx.Format == FormatJSON {
		return writeResult(ctx, foldersJSON(ordered))
	}
	return writeFoldersText(ctx, ordered, s.InModule)
}

// writeFoldersText renders the layout as an indented list — a shape that diffs
// cleanly against a checked-in expectation.
func writeFoldersText(ctx *ExecContext, entries []*folderEntry, inModule string) error {
	if len(entries) == 0 {
		if inModule != "" {
			fmt.Fprintf(ctx.Output, "No folders or documents in %s.\n", inModule)
		} else {
			fmt.Fprintln(ctx.Output, "No folders in this project.")
		}
		return nil
	}

	lastModule := ""
	folderCount, docCount := 0, 0
	for _, e := range entries {
		if e.Module != lastModule {
			if lastModule != "" {
				fmt.Fprintln(ctx.Output)
			}
			fmt.Fprintf(ctx.Output, "%s\n", e.Module)
			lastModule = e.Module
		}
		label := e.Path
		if label == "" {
			label = "(module root)"
		} else {
			folderCount++
		}
		fmt.Fprintf(ctx.Output, "  %s  [%d]\n", label, len(e.Docs))
		for _, d := range e.Docs {
			fmt.Fprintf(ctx.Output, "    %s %s\n", d.Kind, d.Name)
			docCount++
		}
	}
	fmt.Fprintf(ctx.Output, "\n(%d folder(s), %d document(s))\n", folderCount, docCount)
	return nil
}

func foldersJSON(entries []*folderEntry) *TableResult {
	result := &TableResult{Columns: []string{"Module", "Folder", "Kind", "Document"}}
	for _, e := range entries {
		path := e.Path
		if len(e.Docs) == 0 {
			result.Rows = append(result.Rows, []any{e.Module, path, "", ""})
			continue
		}
		for _, d := range e.Docs {
			result.Rows = append(result.Rows, []any{e.Module, path, d.Kind, d.Name})
		}
	}
	return result
}

// documentsByContainer indexes every document mxcli can name by the container it
// sits in. Container, not module: that is the whole point — a document's folder
// is its ContainerID, and the module is only what you get by walking up.
//
// Each list is best-effort. A backend that cannot answer one of them yields a
// listing missing that kind rather than no listing at all.
func documentsByContainer(ctx *ExecContext, h *ContainerHierarchy) map[model.ID][]folderDoc {
	out := map[model.ID][]folderDoc{}
	put := func(kind, name string, container model.ID) {
		if name == "" {
			return
		}
		out[container] = append(out[container], folderDoc{Kind: kind, Name: name})
	}

	if v, err := ctx.Backend.ListMicroflows(); err == nil {
		for _, x := range v {
			put("Microflow", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListNanoflows(); err == nil {
		for _, x := range v {
			put("Nanoflow", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListPages(); err == nil {
		for _, x := range v {
			put("Page", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListSnippets(); err == nil {
		for _, x := range v {
			put("Snippet", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListEnumerations(); err == nil {
		for _, x := range v {
			put("Enumeration", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListConstants(); err == nil {
		for _, x := range v {
			put("Constant", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListJavaActionsFull(); err == nil {
		for _, x := range v {
			put("JavaAction", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListDatabaseConnections(); err == nil {
		for _, x := range v {
			put("DatabaseConnection", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListPublishedODataServices(); err == nil {
		for _, x := range v {
			put("ODataService", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListConsumedODataServices(); err == nil {
		for _, x := range v {
			put("ODataClient", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListWorkflows(); err == nil {
		for _, x := range v {
			put("Workflow", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListScheduledEvents(); err == nil {
		for _, x := range v {
			put("ScheduledEvent", x.Name, x.ContainerID)
		}
	}
	// #892: these five were missing, which is why the folder holding Mendix's
	// own FeedbackModule mappings rendered as `[0]` while holding four
	// documents — and an empty count is what made dropping it look safe.
	if v, err := ctx.Backend.ListJsonStructures(); err == nil {
		for _, x := range v {
			put("JsonStructure", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListImportMappings(); err == nil {
		for _, x := range v {
			put("ImportMapping", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListExportMappings(); err == nil {
		for _, x := range v {
			put("ExportMapping", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListRegularExpressions(); err == nil {
		for _, x := range v {
			put("RegularExpression", x.Name, x.ContainerID)
		}
	}
	if v, err := ctx.Backend.ListImageCollections(); err == nil {
		for _, x := range v {
			put("ImageCollection", x.Name, x.ContainerID)
		}
	}

	// Everything above is a hand-maintained list of document kinds, and #892 is
	// what that costs: five kinds were missing, so a folder holding four
	// documents rendered as `[0]` and dropping it looked safe. Adding the
	// missing five did not fix the shape of the problem — layouts, menus,
	// JavaScript actions and building blocks were still absent, and #932 made
	// all of them movable, so a move you could not see was a move you could not
	// check.
	//
	// So the list is now a source of good LABELS, not the source of truth for
	// what exists. This pass walks the unit table, which cannot omit a kind
	// because it never asks what kind anything is, and fills in every document
	// the typed calls did not already account for. A kind added to Mendix
	// tomorrow appears here without anyone editing this function.
	if units, err := ctx.Backend.ListDocumentUnits(); err == nil {
		seen := make(map[model.ID]map[string]bool, len(out))
		for container, docs := range out {
			names := make(map[string]bool, len(docs))
			for _, d := range docs {
				names[d.Name] = true
			}
			seen[container] = names
		}
		for _, u := range units {
			if seen[u.ContainerID][u.Name] {
				continue
			}
			put(documentKindLabel(u.Kind), u.Name, u.ContainerID)
		}
	}
	return out
}

// documentKindLabel renders a derived kind ("json structure") in the CamelCase
// style the typed entries above use ("JsonStructure"), so one listing does not
// mix two spellings of the same idea.
func documentKindLabel(kind string) string {
	var b strings.Builder
	for _, word := range strings.Fields(kind) {
		b.WriteString(strings.ToUpper(word[:1]))
		b.WriteString(word[1:])
	}
	if b.Len() == 0 {
		return "Document"
	}
	return b.String()
}

// modulesInScope returns the modules the listing covers.
func modulesInScope(ctx *ExecContext, inModule string) []*model.Module {
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return nil
	}
	var out []*model.Module
	for _, m := range modules {
		if moduleMatches(m.Name, inModule) {
			out = append(out, m)
		}
	}
	return out
}

// moduleMatches applies the optional IN clause, case-insensitively as MDL is
// elsewhere. An empty filter matches everything.
func moduleMatches(name, filter string) bool {
	return filter == "" || strings.EqualFold(name, filter)
}

// sortFolderDocs orders a folder's contents by kind then name, so the same model
// always renders the same listing and a diff shows only real movement.
func sortFolderDocs(docs []folderDoc) {
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Kind != docs[j].Kind {
			return docs[i].Kind < docs[j].Kind
		}
		return docs[i].Name < docs[j].Name
	})
}
