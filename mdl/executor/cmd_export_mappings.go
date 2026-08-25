// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// listExportMappings prints a table of all export mapping documents.
func listExportMappings(ctx *ExecContext, inModule string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	all, err := ctx.Backend.ListExportMappings()
	if err != nil {
		return mdlerrors.NewBackend("list export mappings", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	type row struct {
		qualifiedName, name, schemaSource string
		elementCount                      int
	}
	var rows []row

	for _, em := range all {
		modID := h.FindModuleID(em.ContainerID)
		moduleName := h.GetModuleName(modID)
		if inModule != "" && !strings.EqualFold(moduleName, inModule) {
			continue
		}
		qn := moduleName + "." + em.Name
		src := em.JsonStructure
		if src == "" {
			src = em.XmlSchema
		}
		if src == "" {
			src = em.MessageDefinition
		}
		if src == "" {
			src = "(none)"
		}
		rows = append(rows, row{qualifiedName: qn, name: em.Name, schemaSource: src, elementCount: len(em.Elements)})
	}

	if len(rows) == 0 {
		if inModule != "" {
			fmt.Fprintf(ctx.Output, "No export mappings found in module %s\n", inModule)
		} else {
			fmt.Fprintln(ctx.Output, "No export mappings found")
		}
		return nil
	}

	// Sort alphabetically by qualified name
	sort.Slice(rows, func(i, j int) bool { return rows[i].qualifiedName < rows[j].qualifiedName })

	result := &TableResult{
		Columns: []string{"Export Mapping", "Name", "Schema Source", "Elements"},
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.name, r.schemaSource, r.elementCount})
	}
	return writeResult(ctx, result)
}

// describeExportMapping prints the MDL representation of an export mapping.
func describeExportMapping(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	em, err := ctx.Backend.GetExportMappingByQualifiedName(name.Module, name.Name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return mdlerrors.NewNotFound("export mapping", name.String())
		}
		return mdlerrors.NewBackend("get export mapping", err)
	}

	if em.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", strings.ReplaceAll(em.Documentation, "\n", "\n * "))
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(em.ContainerID)
	moduleName := h.GetModuleName(modID)

	fmt.Fprintf(ctx.Output, "create or modify export mapping %s.%s\n", moduleName, em.Name)
	// Without this the description round-trips to the module root: replaying
	// it in a fresh project would recreate the mapping unfiled (#932).
	if folderPath := h.BuildFolderPath(em.ContainerID); folderPath != "" {
		fmt.Fprintf(ctx.Output, "  folder '%s'\n", folderPath)
	}

	if em.JsonStructure != "" {
		fmt.Fprintf(ctx.Output, "  with json structure %s\n", em.JsonStructure)
	} else if em.XmlSchema != "" {
		fmt.Fprintf(ctx.Output, "  with xml schema %s\n", em.XmlSchema)
	} else if em.MessageDefinition != "" {
		// Dropped entirely before #263 — and the output still PARSED, so
		// re-executing a DESCRIBE rebuilt the mapping bound to nothing.
		fmt.Fprintf(ctx.Output, "  with message definition %s\n", em.MessageDefinition)
	}

	if em.NullValueOption != "" && em.NullValueOption != "LeaveOutElement" {
		fmt.Fprintf(ctx.Output, "  null values %s\n", em.NullValueOption)
	}

	if len(em.Elements) > 0 {
		fmt.Fprintln(ctx.Output, "{")
		for _, elem := range em.Elements {
			printExportMappingElement(ctx.Output, exportRootToPrint(elem), 1, true, "")
			fmt.Fprintln(ctx.Output)
		}
		fmt.Fprintln(ctx.Output, "};")
	}
	return nil
}

// exportRootToPrint unwraps the bare Array container an array-rooted export
// mapping stores (#248) so DESCRIBE prints the MDL that produced it — the root
// entity, not the container.
//
// Without this the container falls into printExportMappingElement's value branch
// (it is Kind "Array", not "Object") and prints "Root = " with its whole subtree
// dropped, which is the #260 defect. Every mapping this change makes authorable
// would have gone straight into the silent-loss set otherwise.
//
// Only the shape mxcli itself writes is unwrapped: a container carrying an
// entity, or one with more than a single object child, is a shape MDL cannot
// author (DigitalTwin.EMM_EntityQuery — see buildExportRootArrayElement), so it
// is left alone for #260/#262 to handle rather than being printed as something
// it is not.
func exportRootToPrint(elem *model.ExportMappingElement) *model.ExportMappingElement {
	// Discriminate on the stored paths, not on Kind: the two engines' readers
	// label the container differently ("Array" vs "Object"), and the paths are
	// what the document actually says.
	if elem == nil || elem.JsonPath != "(Array)" || elem.Entity != "" || len(elem.Children) != 1 {
		return elem
	}
	if item := elem.Children[0]; item.Entity != "" && item.JsonPath == "(Array)|(Object)" {
		return item
	}
	return elem
}

func printExportMappingElement(w io.Writer, elem *model.ExportMappingElement, depth int, isRoot bool, parentPath string) {
	indent := strings.Repeat("  ", depth)
	if elem.Kind == "Object" {
		if isRoot {
			// Root: Module.Entity { — use "." if entity is empty (parameter mapping)
			entity := elem.Entity
			if entity == "" {
				entity = "."
			}
			fmt.Fprintf(w, "%s%s {\n", indent, entity)
		} else {
			// Nested object element. Several cases:
			//   Assoc/Entity AS jsonKey  — normal association path
			//   ./Entity AS jsonKey      — self-reference (no association, entity set)
			//   . AS jsonKey             — structural grouping (no association, no entity)
			assoc := elem.Association
			entity := elem.Entity
			if assoc == "" && entity == "" {
				fmt.Fprintf(w, "%s. as %s", indent, mappingMemberName(parentPath, elem.JsonPath, elem.ExposedName))
			} else if assoc == "" {
				fmt.Fprintf(w, "%s./%s as %s", indent, entity, mappingMemberName(parentPath, elem.JsonPath, elem.ExposedName))
			} else {
				fmt.Fprintf(w, "%s%s/%s as %s", indent, assoc, entity, mappingMemberName(parentPath, elem.JsonPath, elem.ExposedName))
			}
			if len(elem.Children) > 0 {
				fmt.Fprintln(w, " {")
			}
		}
		if len(elem.Children) > 0 {
			for i, child := range elem.Children {
				printExportMappingElement(w, child, depth+1, false, elem.JsonPath)
				if i < len(elem.Children)-1 {
					fmt.Fprintln(w, ",")
				} else {
					fmt.Fprintln(w)
				}
			}
			fmt.Fprintf(w, "%s}", indent)
		}
	} else {
		// Value mapping: jsonField = Attr
		attrName := elem.Attribute
		// Strip module prefix if present (Module.Entity.Attr → Attr)
		if parts := strings.Split(attrName, "."); len(parts) == 3 {
			attrName = parts[2]
		}
		fmt.Fprintf(w, "%s%s = %s", indent, mappingMemberName(parentPath, elem.JsonPath, elem.ExposedName), attrName)
	}
}

// execCreateExportMapping creates a new export mapping.
func execCreateExportMapping(ctx *ExecContext, s *ast.CreateExportMappingStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	existing, _ := ctx.Backend.GetExportMappingByQualifiedName(s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("export mapping", s.Name.String())
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}
	// A folder clause places the mapping; without one a new mapping goes to the
	// module root and an existing one stays where it is (#932).
	containerID, err := resolveRequestedFolder(ctx, module.ID, s.Folder)
	if err != nil {
		return err
	}
	if containerID == "" {
		containerID = module.ID
		if existing != nil {
			containerID = existing.ContainerID
		}
	}

	em := &model.ExportMapping{
		ContainerID:     containerID,
		Name:            s.Name.Name,
		ExportLevel:     "Hidden",
		NullValueOption: s.NullValueOption,
	}
	if existing != nil {
		// Excluded is model state, not script state (#914).
		em.Excluded = existing.Excluded
	}
	if em.NullValueOption == "" {
		em.NullValueOption = "LeaveOutElement"
	}

	// Set schema source reference
	switch s.SchemaKind {
	case "JSON_STRUCTURE":
		em.JsonStructure = s.SchemaRef.String()
	case "XML_SCHEMA":
		em.XmlSchema = s.SchemaRef.String()
	case "MESSAGE_DEFINITION":
		em.MessageDefinition = s.SchemaRef.String()
	}

	// See the import twin: a message definition has its own builder (#263).
	if s.SchemaKind == "MESSAGE_DEFINITION" {
		md, err := findMessageDefinition(ctx.Backend, em.MessageDefinition)
		if err != nil {
			return mdlerrors.NewValidation(fmt.Sprintf("export mapping %s: %v", s.Name.String(), err))
		}
		if s.RootElement != nil {
			root, err := buildExportMappingFromMessageDefinition(s.Name.Module, s.RootElement,
				md.Root, "", "", true, ctx.Backend)
			if err != nil {
				return mdlerrors.NewValidation(fmt.Sprintf("export mapping %s: %v", s.Name.String(), err))
			}
			em.Elements = append(em.Elements, root)
		}
		return finishExportMapping(ctx, s, em, existing, containerID)
	}

	// Index the JSON structure for schema alignment.
	idx := newJSONSchemaIndex(nil)
	if s.SchemaKind == "JSON_STRUCTURE" && s.SchemaRef.Module != "" {
		if js, err2 := ctx.Backend.GetJsonStructureByQualifiedName(s.SchemaRef.Module, s.SchemaRef.Name); err2 == nil && js != nil {
			idx = newJSONSchemaIndex(js.Elements)
		}
	}

	// Build element tree from the AST definition, cloning JSON structure properties
	if s.RootElement != nil {
		root, err := buildExportMappingElementModel(s.Name.Module, s.RootElement, "", "(Object)", idx, ctx.Backend, true)
		if err != nil {
			return mdlerrors.NewValidation(fmt.Sprintf("export mapping %s: %v", s.Name.String(), err))
		}
		em.Elements = append(em.Elements, root)
	}

	return finishExportMapping(ctx, s, em, existing, containerID)
}

// finishExportMapping writes the built mapping, shared by the JSON-structure and
// message-definition paths.
func finishExportMapping(ctx *ExecContext, s *ast.CreateExportMappingStmt,
	em *model.ExportMapping, existing *model.ExportMapping, containerID model.ID,
) error {
	if existing != nil {
		em.ID = existing.ID
		if err := ctx.Backend.UpdateExportMapping(em); err != nil {
			return mdlerrors.NewBackend("update export mapping", err)
		}
		if _, err := applyDocumentFolder(ctx, em.ID, existing.ContainerID, containerID); err != nil {
			return err
		}
		if !ctx.Quiet {
			ctx.ReportMutation("Modified", "export mapping %s.%s", s.Name.Module, s.Name.Name)
		}
		return nil
	}

	if err := ctx.Backend.CreateExportMapping(em); err != nil {
		return mdlerrors.NewBackend("create export mapping", err)
	}
	invalidateHierarchy(ctx)

	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Created export mapping %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// buildExportRootArrayElement builds the two-level shape Studio Pro stores for an
// array-rooted export mapping: a bare Array container at the structure's root
// path, whose single child is the item object carrying the entity.
//
// Measured on SnowflakeIntegration.EXM_SensorData (Evora demo app, Mendix
// 11.13), which is the shape a root entity in MDL means — the mapping's
// parameter is the item and the array is produced from a list of them:
//
//	et=Array   oh=Find      entity=-                          max=1   path=(Array)
//	  et=Object  oh=Parameter entity=SnowflakeIntegration.SensorData max=-1  path=(Array)|(Object)
//
// The other shape in the corpus (DigitalTwin.EMM_EntityQuery) puts an entity on
// the container and Find on the item — the parameter owns the list rather than
// being in it. MDL names one entity at the root and cannot say which, so that
// shape stays unauthorable; it needs the container syntax tracked by #262.
func buildExportRootArrayElement(moduleName string, def *ast.ExportMappingElementDef,
	root *types.JsonElement, idx *jsonSchemaIndex, b backend.FullBackend,
) (*model.ExportMappingElement, error) {
	entity := def.Entity
	if entity != "" && !strings.Contains(entity, ".") {
		entity = moduleName + "." + entity
	}

	itemPath := root.Path + "|(Object)"
	item := &model.ExportMappingElement{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "ExportMappings$ObjectMappingElement",
		},
		Kind:           "Object",
		Entity:         entity,
		ObjectHandling: "Parameter",
		JsonPath:       itemPath,
		MaxOccurs:      -1, // 0..*, mirroring the schema item (#841)
	}
	if jsItem, ok := idx.byPath[itemPath]; ok {
		item.ExposedName = jsItem.ExposedName
		item.MaxOccurs = jsItem.MaxOccurs
	}
	for _, child := range def.Children {
		c, err := buildExportMappingElementModel(moduleName, child, entity, itemPath, idx, b, false)
		if err != nil {
			return nil, err
		}
		item.Children = append(item.Children, c)
	}

	return &model.ExportMappingElement{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "ExportMappings$ObjectMappingElement",
		},
		Kind:           "Array",
		ObjectHandling: "Find",
		ExposedName:    root.ExposedName,
		JsonPath:       root.Path,
		MaxOccurs:      root.MaxOccurs,
		Children:       []*model.ExportMappingElement{item},
	}, nil
}

// buildExportMappingElementModel converts an AST element definition to a model element.
// It clones properties from the matching JSON structure element and adds mapping bindings.
func buildExportMappingElementModel(moduleName string, def *ast.ExportMappingElementDef, parentEntity, parentPath string, idx *jsonSchemaIndex, b backend.FullBackend, isRoot bool) (*model.ExportMappingElement, error) {
	elem := &model.ExportMappingElement{
		BaseElement: model.BaseElement{
			ID: model.ID(types.GenerateID()),
		},
	}

	// Resolve the member against the JSON structure, accepting the raw JSON key
	// or the exposed name. DESCRIBE emits the latter, so both have to work or
	// mxcli's own output does not round-trip — re-executing it rewrote
	// "(Object)|total" as "(Object)|Total". (issue #882)
	var jsElem *types.JsonElement
	lookupPath := parentPath + "|" + def.JsonName
	if isRoot {
		// The structure decides where its own root is (#248) — see the twin
		// comment in cmd_import_mappings.go. The export side needs one thing the
		// import side does not: an array-rooted EXPORT mapping is stored as TWO
		// elements, a bare Array container plus the item that carries the entity,
		// where the import mapping collapses to the item alone.
		lookupPath = "(Object)"
		if root := idx.root(); root != nil {
			lookupPath = root.Path
			if root.ElementType == "Array" {
				return buildExportRootArrayElement(moduleName, def, root, idx, b)
			}
		}
		jsElem = idx.byPath[lookupPath]
	} else {
		// An EXPORT mapping cannot collapse levels the way an import mapping can.
		// Measured on mxbuild 11.13 with a three-way control: an import mapping
		// binding "(Object)|customer|name" under an object element at "(Object)"
		// builds at 0 errors, an export mapping over the same structure mapping
		// only top-level fields builds at 0 errors, and the same export mapping
		// with the collapsed member is CE5015 "There is no child mapping matching
		// schema element". That follows from what the two do: an export mapping
		// has to PRODUCE the customer node, so something must map it.
		//
		// Refused here rather than written, because the model would be valid MDL,
		// pass `mxcli check`, and fail only in the build. (issue #927)
		if strings.Contains(def.JsonName, "/") {
			return nil, nestedExportMemberError(def.JsonName)
		}
		jsElem = idx.resolve(parentPath, def.JsonName)
	}

	if jsElem == nil && !isRoot && idx.resolvable() {
		known := idx.memberNames(parentPath)
		if len(known) == 0 {
			return nil, fmt.Errorf("%q is not a member of the JSON structure at %s, which has no members there",
				def.JsonName, parentPath)
		}
		return nil, fmt.Errorf("%q is not a member of the JSON structure at %s; available: %s",
			def.JsonName, parentPath, strings.Join(known, ", "))
	}
	if jsElem != nil {
		elem.ExposedName = jsElem.ExposedName
		elem.JsonPath = jsElem.Path
		elem.MaxOccurs = jsElem.MaxOccurs
		lookupPath = jsElem.Path
	} else {
		elem.ExposedName = def.JsonName
		elem.JsonPath = lookupPath
	}

	if def.Entity != "" {
		// Object/Array mapping — bind to entity
		elem.Kind = "Object"
		elem.TypeName = "ExportMappings$ObjectMappingElement"

		entity := def.Entity
		if !strings.Contains(entity, ".") {
			entity = moduleName + "." + entity
		}

		assoc := def.Association
		if assoc != "" && !strings.Contains(assoc, ".") {
			assoc = moduleName + "." + assoc
		}

		handling := "Parameter"
		if !isRoot {
			handling = "Find"
		}

		// Check if this is an array element in the JSON structure
		if jsElem != nil && jsElem.ElementType == "Array" {
			// Export arrays have two levels:
			// 1. Array container: Kind=Array, entity=container entity, assoc to parent
			// 2. Item object: Kind=Object, entity=item entity, assoc to container
			//
			// MDL syntax: Assoc/Entity AS items { ItemAssoc/ItemEntity AS ItemsItem { values } }
			// The outer Assoc/Entity is for the container, the nested child provides the item.
			elem.Kind = "Array"
			elem.Association = assoc
			elem.ObjectHandling = handling
			elem.Entity = entity

			itemPath := lookupPath + "|(Object)"

			// The first (and typically only) child of the array in the MDL is the item definition.
			// Its children become the item element's value children.
			if len(def.Children) == 1 && def.Children[0].Entity != "" {
				itemDef := def.Children[0]
				itemEntity := itemDef.Entity
				if !strings.Contains(itemEntity, ".") {
					itemEntity = moduleName + "." + itemEntity
				}
				itemAssoc := itemDef.Association
				if itemAssoc != "" && !strings.Contains(itemAssoc, ".") {
					itemAssoc = moduleName + "." + itemAssoc
				}

				itemElem := &model.ExportMappingElement{
					BaseElement: model.BaseElement{
						ID:       model.ID(types.GenerateID()),
						TypeName: "ExportMappings$ObjectMappingElement",
					},
					Kind:           "Object",
					Entity:         itemEntity,
					Association:    itemAssoc,
					ObjectHandling: "Find",
				}
				if jsItem, ok2 := idx.byPath[itemPath]; ok2 {
					itemElem.ExposedName = jsItem.ExposedName
					itemElem.JsonPath = jsItem.Path
					itemElem.MaxOccurs = jsItem.MaxOccurs
				} else {
					itemElem.ExposedName = elem.ExposedName + "Item"
					itemElem.JsonPath = itemPath
					itemElem.MaxOccurs = -1
				}
				// Item's children are the value elements
				for _, valChild := range itemDef.Children {
					c, err := buildExportMappingElementModel(moduleName, valChild, itemEntity, itemPath, idx, b, false)
					if err != nil {
						return nil, err
					}
					itemElem.Children = append(itemElem.Children, c)
				}
				elem.Children = append(elem.Children, itemElem)
			} else {
				// Fallback: treat children as direct item children (no intermediate entity)
				for _, child := range def.Children {
					c, err := buildExportMappingElementModel(moduleName, child, entity, itemPath, idx, b, false)
					if err != nil {
						return nil, err
					}
					elem.Children = append(elem.Children, c)
				}
			}
		} else {
			// Regular object element
			elem.Entity = entity
			elem.Association = assoc
			elem.ObjectHandling = handling
			for _, child := range def.Children {
				c, err := buildExportMappingElementModel(moduleName, child, entity, lookupPath, idx, b, false)
				if err != nil {
					return nil, err
				}
				elem.Children = append(elem.Children, c)
			}
		}
	} else {
		// Value mapping — bind to attribute
		elem.Kind = "Value"
		elem.TypeName = "ExportMappings$ValueMappingElement"
		elem.DataType = resolveAttributeType(parentEntity, def.Attribute, b)
		// A member reference is qualified against the entity that DECLARES it, so
		// an inherited attribute carries an ancestor's name. Prefixing the entity
		// being mapped produced CE1613 "The selected attribute no longer exists"
		// and left the field unmapped in Studio Pro (mendixlabs/mxcli#703) — the
		// same rule as entity access rules (#758), both under the #765 umbrella.
		attr := def.Attribute
		if parentEntity != "" && !strings.Contains(attr, ".") {
			if ref, ok := ResolveMemberRef(b, parentEntity, attr); ok {
				attr = ref
			} else {
				attr = parentEntity + "." + attr
			}
		}
		elem.Attribute = attr
		// JsonPath already set from JSON structure clone above
	}

	return elem, nil
}

// execDropExportMapping deletes an export mapping.
func execDropExportMapping(ctx *ExecContext, s *ast.DropExportMappingStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	em, err := ctx.Backend.GetExportMappingByQualifiedName(s.Name.Module, s.Name.Name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return mdlerrors.NewNotFound("export mapping", s.Name.String())
		}
		return mdlerrors.NewBackend("get export mapping", err)
	}

	if err := ctx.Backend.DeleteExportMapping(em.ID); err != nil {
		return mdlerrors.NewBackend("drop export mapping", err)
	}

	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Dropped export mapping %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// nestedExportMemberError is shared by the check-time guard and the executor so
// the author sees one message wherever the statement is stopped.
func nestedExportMemberError(member string) error {
	level := strings.Split(member, "/")[0]
	return mdlerrors.NewValidationf("export mapping member %q: an export mapping cannot reach a nested "+
		"member directly — Mendix rejects it with CE5015 because the intermediate object has nothing "+
		"producing it. Give %q its own element: Association/Module.Entity as %s { ... }. "+
		"(Collapsing levels this way works for IMPORT mappings, which only read.)",
		member, level, level)
}
