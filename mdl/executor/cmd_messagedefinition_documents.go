// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// Message definitions as documents (ako/mxcli#987).
//
// Mendix 11.15 replaced the MessageDefinitionCollection with one
// MessageDefinitions$MessageDefinition2 document per definition. Measured with
// `mx convert` 11.15.0: a collection becomes a folder of the collection's name
// (same unit ID) holding one document per entry, the exposed tree is unchanged,
// and a mapping's source moves from MessageDefinition (Module.Collection.Name)
// to MessageDefinition2 (Module.Name).
//
// The statements are the collection's `definition` entry promoted to its own
// document, so they share its builder and describer:
//
//	create [or modify] message definition Module.Name [folder '…']
//	  for Module.Entity [as '…'] { members };
//	describe message definition Module.Name;
//	drop message definition Module.Name;
//	list message definitions [in Module];
//	alter message definition Module.Name add|drop|set member …;
//
// The two storages never coexist in a real project, so the version decides:
// the document form below 11.15 and the collection form from 11.15 on are each
// refused with a hint naming the other, rather than written in a shape the
// project's Mendix version does not have.

// messageCollectionsRemovedError refuses a collection statement on a project
// that stores message definitions as documents.
func messageCollectionsRemovedError(ctx *ExecContext, statement string) error {
	version := "11.15+"
	if pv := ctx.Backend.ProjectVersion(); pv != nil && pv.ProductVersion != "" {
		version = pv.ProductVersion
	}
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"%s: Mendix 11.15 removed message definition collections (project is %s) — each "+
			"definition is its own document now\n"+
			"  hint: create message definition Module.Name for Module.Entity { … }; — Studio Pro's "+
			"converter turns a collection into a folder of that name holding one such document per "+
			"definition, and a mapping names one as Module.Name", statement, version))
}

// requireMessageDefinitionDocuments refuses the per-document form below 11.15.
func requireMessageDefinitionDocuments(ctx *ExecContext, statement string) error {
	return checkFeature(ctx, "integration", "message_definition_document", statement,
		"below Mendix 11.15 a message definition is an entry of a collection: "+
			"create message definition collection Module.Name { definition Name for Module.Entity { … } };")
}

// projectStoresMessageDocuments reports whether the connected project stores
// message definitions as MessageDefinition2 documents.
func projectStoresMessageDocuments(ctx *ExecContext) bool {
	return ctx.Connected() && messageDefinitionsAreDocuments(ctx.Backend.ProjectVersion())
}

// findMessageDefinitionDocument looks a document up by module and name,
// preferring a live one over an excluded twin.
func findMessageDefinitionDocument(ctx *ExecContext, moduleName, name string) *model.MessageDefinitionDocument {
	all, err := ctx.Backend.ListMessageDefinitionDocuments()
	if err != nil {
		return nil
	}
	h, herr := getHierarchy(ctx)
	var excluded *model.MessageDefinitionDocument
	for _, d := range all {
		if d == nil || !strings.EqualFold(d.Name, name) {
			continue
		}
		if herr == nil && !strings.EqualFold(h.GetModuleName(h.FindModuleID(d.ContainerID)), moduleName) {
			continue
		}
		if !d.Excluded {
			return d
		}
		if excluded == nil {
			excluded = d
		}
	}
	return excluded
}

// findConvertedMessageDefinitionDocument resolves a three-part
// Module.Collection.Definition reference on an 11.15 project to the document
// `mx convert` made of it: the definition's document, in a folder named after
// the collection. It is what keeps a pre-11.15 script's references working on
// a converted project.
func findConvertedMessageDefinitionDocument(ctx *ExecContext, moduleName, collection, name string) *model.MessageDefinitionDocument {
	d := findMessageDefinitionDocument(ctx, moduleName, name)
	if d == nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil || !strings.EqualFold(h.folderNames[d.ContainerID], collection) {
		return nil
	}
	return d
}

// messageDocumentQN returns a document's Module.Name, the reference a mapping
// stores in MessageDefinition2.
func messageDocumentQN(ctx *ExecContext, d *model.MessageDefinitionDocument) string {
	if h, err := getHierarchy(ctx); err == nil {
		if m := h.GetModuleName(h.FindModuleID(d.ContainerID)); m != "" {
			return m + "." + d.Name
		}
	}
	return d.Name
}

// execCreateMessageDefinition handles CREATE [OR MODIFY] MESSAGE DEFINITION.
func execCreateMessageDefinition(ctx *ExecContext, s *ast.CreateMessageDefinitionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if err := requireMessageDefinitionDocuments(ctx, "create message definition"); err != nil {
		return err
	}
	existing := findMessageDefinitionDocument(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("message definition", s.Name.String())
	}
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}
	containerID, err := resolveRequestedFolder(ctx, module.ID, s.Folder)
	if err != nil {
		return err
	}
	if containerID == "" {
		containerID = module.ID
		if existing != nil {
			// Without a folder clause an existing document stays where it is.
			containerID = existing.ContainerID
		}
	}

	def := *s.Definition
	def.Name = s.Name.Name
	built, err := buildMessageDefinitionAs(ctx, &def, s.Name.Module, "message definition "+s.Name.String())
	if err != nil {
		return err
	}
	d := &model.MessageDefinitionDocument{
		ContainerID: containerID,
		Name:        s.Name.Name,
		ExportLevel: "Hidden",
		Root:        built.Root,
	}
	if existing != nil {
		// Carry what the statement does not restate.
		d.ID = existing.ID
		d.Name = existing.Name
		d.Documentation = existing.Documentation
		d.Excluded = existing.Excluded
		d.ExportLevel = existing.ExportLevel
		if err := ctx.Backend.UpdateMessageDefinitionDocument(d); err != nil {
			return mdlerrors.NewBackend("update message definition", err)
		}
		if _, err := applyDocumentFolder(ctx, d.ID, existing.ContainerID, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "message definition: %s", s.Name.String())
		return nil
	}
	if err := ctx.Backend.CreateMessageDefinitionDocument(d); err != nil {
		return mdlerrors.NewBackend("create message definition", err)
	}
	// Not in the cached hierarchy yet: a later statement looking it up by module
	// would not find it and would create a duplicate.
	invalidateHierarchy(ctx)
	ctx.ReportMutation("Created", "message definition: %s", s.Name.String())
	return nil
}

// execDropMessageDefinition deletes a document. A mapping bound to it would be
// left dangling, so one still referenced is refused, naming the mappings.
func execDropMessageDefinition(ctx *ExecContext, s *ast.DropMessageDefinitionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	d := findMessageDefinitionDocument(ctx, s.Name.Module, s.Name.Name)
	if d == nil {
		if err := requireMessageDefinitionDocuments(ctx, "drop message definition"); err != nil {
			return err
		}
		return mdlerrors.NewNotFound("message definition", s.Name.String())
	}
	qn := messageDocumentQN(ctx, d)
	if users := mappingsUsingDefinition(ctx, qn); len(users) > 0 {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"message definition %s is still used by %s — dropping it would leave them "+
				"bound to nothing (CE1613)", qn, strings.Join(users, ", ")))
	}
	if err := ctx.Backend.DeleteMessageDefinitionDocument(string(d.ID)); err != nil {
		return mdlerrors.NewBackend("drop message definition", err)
	}
	invalidateHierarchy(ctx)
	ctx.ReportMutation("Dropped", "message definition: %s", s.Name.String())
	return nil
}

// execDescribeMessageDefinition prints the re-executable per-document form.
func execDescribeMessageDefinition(ctx *ExecContext, name ast.QualifiedName) error {
	d := findMessageDefinitionDocument(ctx, name.Module, name.Name)
	if d == nil {
		if ctx.Connected() && !projectStoresMessageDocuments(ctx) {
			return mdlerrors.NewNotFound("message definition", name.String()+
				" (below Mendix 11.15 a message definition is inside a collection: "+
				"describe message definition collection Module.Collection)")
		}
		return mdlerrors.NewNotFound("message definition", name.String())
	}
	describeMessageDefinitionDocument(ctx, d)
	return nil
}

func describeMessageDefinitionDocument(ctx *ExecContext, d *model.MessageDefinitionDocument) {
	fmt.Fprintf(ctx.Output, "create or modify message definition %s\n", messageDocumentQN(ctx, d))
	if h, err := getHierarchy(ctx); err == nil {
		if folder := h.BuildFolderPath(d.ContainerID); folder != "" {
			fmt.Fprintf(ctx.Output, "  folder %s\n", mdlQuote(ctx, folder))
		}
	}
	if d.Root == nil {
		// A document whose root this reader does not model. Say so rather than
		// print a statement that would rebuild it empty.
		fmt.Fprintln(ctx.Output, "  -- ROOT NOT REPRESENTABLE")
		fmt.Fprintln(ctx.Output, ";")
		return
	}
	fmt.Fprintf(ctx.Output, "  for %s%s {\n", d.Root.Entity,
		exposedClause(d.Root, shortEntityName(d.Root.Entity)))
	describeMessageMembers(ctx, d.Root.Children, "    ")
	fmt.Fprintln(ctx.Output, "  };")
}

// listMessageDefinitions handles LIST MESSAGE DEFINITIONS [IN module].
func listMessageDefinitions(ctx *ExecContext, inModule string) error {
	all, err := ctx.Backend.ListMessageDefinitionDocuments()
	if err != nil {
		return mdlerrors.NewBackend("list message definitions", err)
	}
	h, herr := getHierarchy(ctx)

	type row struct{ module, name, entity, folder string }
	var rows []row
	for _, d := range all {
		if d == nil {
			continue
		}
		r := row{name: d.Name}
		if herr == nil {
			r.module = h.GetModuleName(h.FindModuleID(d.ContainerID))
			r.folder = h.BuildFolderPath(d.ContainerID)
		}
		if inModule != "" && !strings.EqualFold(r.module, inModule) {
			continue
		}
		if d.Root != nil {
			r.entity = d.Root.Entity
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].module != rows[j].module {
			return rows[i].module < rows[j].module
		}
		return rows[i].name < rows[j].name
	})

	if len(rows) == 0 {
		if ctx.Connected() && !projectStoresMessageDocuments(ctx) {
			fmt.Fprintln(ctx.Output, "No message definitions found (below Mendix 11.15 they are "+
				"inside collections: list message definition collections)")
			return nil
		}
		fmt.Fprintln(ctx.Output, "No message definitions found")
		return nil
	}
	fmt.Fprintln(ctx.Output, "| Module | Name | Entity | Folder |")
	fmt.Fprintln(ctx.Output, "|--------|------|--------|--------|")
	for _, r := range rows {
		fmt.Fprintf(ctx.Output, "| %s | %s | %s | %s |\n", r.module, r.name, r.entity, r.folder)
	}
	fmt.Fprintf(ctx.Output, "\n(%d message definition(s))\n", len(rows))
	return nil
}
