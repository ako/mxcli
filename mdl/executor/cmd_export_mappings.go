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

	if em.JsonStructure != "" {
		fmt.Fprintf(ctx.Output, "  with json structure %s\n", em.JsonStructure)
	} else if em.XmlSchema != "" {
		fmt.Fprintf(ctx.Output, "  with xml schema %s\n", em.XmlSchema)
	}

	if em.NullValueOption != "" && em.NullValueOption != "LeaveOutElement" {
		fmt.Fprintf(ctx.Output, "  null values %s\n", em.NullValueOption)
	}

	if len(em.Elements) > 0 {
		fmt.Fprintln(ctx.Output, "{")
		for _, elem := range em.Elements {
			printExportMappingElement(ctx.Output, elem, 1, true)
			fmt.Fprintln(ctx.Output)
		}
		fmt.Fprintln(ctx.Output, "};")
	}
	return nil
}

func printExportMappingElement(w io.Writer, elem *model.ExportMappingElement, depth int, isRoot bool) {
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
				fmt.Fprintf(w, "%s. as %s", indent, mappingMemberName(elem.JsonPath, elem.ExposedName))
			} else if assoc == "" {
				fmt.Fprintf(w, "%s./%s as %s", indent, entity, mappingMemberName(elem.JsonPath, elem.ExposedName))
			} else {
				fmt.Fprintf(w, "%s%s/%s as %s", indent, assoc, entity, mappingMemberName(elem.JsonPath, elem.ExposedName))
			}
			if len(elem.Children) > 0 {
				fmt.Fprintln(w, " {")
			}
		}
		if len(elem.Children) > 0 {
			for i, child := range elem.Children {
				printExportMappingElement(w, child, depth+1, false)
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
		fmt.Fprintf(w, "%s%s = %s", indent, mappingMemberName(elem.JsonPath, elem.ExposedName), attrName)
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
	containerID := module.ID

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

	if existing != nil {
		em.ID = existing.ID
		if err := ctx.Backend.UpdateExportMapping(em); err != nil {
			return mdlerrors.NewBackend("update export mapping", err)
		}
		if !ctx.Quiet {
			fmt.Fprintf(ctx.Output, "Modified export mapping %s.%s\n", s.Name.Module, s.Name.Name)
		}
		return nil
	}

	if err := ctx.Backend.CreateExportMapping(em); err != nil {
		return mdlerrors.NewBackend("create export mapping", err)
	}

	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Created export mapping %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
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
		lookupPath = "(Object)"
		jsElem = idx.byPath[lookupPath]
	} else {
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
