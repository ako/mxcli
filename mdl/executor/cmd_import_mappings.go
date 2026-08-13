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

// listImportMappings prints a table of all import mapping documents.
func listImportMappings(ctx *ExecContext, inModule string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	all, err := ctx.Backend.ListImportMappings()
	if err != nil {
		return mdlerrors.NewBackend("list import mappings", err)
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

	for _, im := range all {
		modID := h.FindModuleID(im.ContainerID)
		moduleName := h.GetModuleName(modID)
		if inModule != "" && !strings.EqualFold(moduleName, inModule) {
			continue
		}
		qn := moduleName + "." + im.Name
		src := im.JsonStructure
		if src == "" {
			src = im.XmlSchema
		}
		if src == "" {
			src = im.MessageDefinition
		}
		if src == "" {
			src = "(none)"
		}
		rows = append(rows, row{qualifiedName: qn, name: im.Name, schemaSource: src, elementCount: len(im.Elements)})
	}

	if len(rows) == 0 {
		if inModule != "" {
			fmt.Fprintf(ctx.Output, "No import mappings found in module %s\n", inModule)
		} else {
			fmt.Fprintln(ctx.Output, "No import mappings found")
		}
		return nil
	}

	// Sort alphabetically by qualified name
	sort.Slice(rows, func(i, j int) bool { return rows[i].qualifiedName < rows[j].qualifiedName })

	result := &TableResult{
		Columns: []string{"Import Mapping", "Name", "Schema Source", "Elements"},
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.name, r.schemaSource, r.elementCount})
	}
	return writeResult(ctx, result)
}

// describeImportMapping prints the MDL representation of an import mapping.
func describeImportMapping(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	im, err := ctx.Backend.GetImportMappingByQualifiedName(name.Module, name.Name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return mdlerrors.NewNotFound("import mapping", name.String())
		}
		return mdlerrors.NewBackend("get import mapping", err)
	}

	if im.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", strings.ReplaceAll(im.Documentation, "\n", "\n * "))
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(im.ContainerID)
	moduleName := h.GetModuleName(modID)

	fmt.Fprintf(ctx.Output, "create import mapping %s.%s\n", moduleName, im.Name)

	if im.JsonStructure != "" {
		fmt.Fprintf(ctx.Output, "  with json structure %s\n", im.JsonStructure)
	} else if im.XmlSchema != "" {
		fmt.Fprintf(ctx.Output, "  with xml schema %s\n", im.XmlSchema)
	}

	if len(im.Elements) > 0 {
		fmt.Fprintln(ctx.Output, "{")
		for _, elem := range im.Elements {
			printImportMappingElement(ctx.Output, elem, 1, true)
			fmt.Fprintln(ctx.Output)
		}
		fmt.Fprintln(ctx.Output, "};")
	}
	return nil
}

// handlingKeyword returns the MDL keyword for a Mendix ObjectHandling value.
func handlingKeyword(handling string) string {
	switch handling {
	case "Find":
		return "find"
	case "FindOrCreate":
		return "find or create"
	default:
		return "create"
	}
}

func printImportMappingElement(w io.Writer, elem *model.ImportMappingElement, depth int, isRoot bool) {
	indent := strings.Repeat("  ", depth)
	if elem.Kind == "Object" {
		handling := handlingKeyword(elem.ObjectHandling)
		if isRoot {
			// Root: CREATE Module.Entity { — use "." if entity is empty
			entity := elem.Entity
			if entity == "" {
				entity = "."
			}
			fmt.Fprintf(w, "%s%s %s {\n", indent, handling, entity)
		} else {
			// Nested object element:
			//   CREATE Assoc/Entity = jsonKey   — normal association path
			//   CREATE ./Entity = jsonKey       — self-reference (no association)
			//   CREATE . = jsonKey              — structural grouping (no association, no entity)
			assoc := elem.Association
			entity := elem.Entity
			if assoc == "" && entity == "" {
				fmt.Fprintf(w, "%s%s . = %s", indent, handling, elem.ExposedName)
			} else if assoc == "" {
				fmt.Fprintf(w, "%s%s ./%s = %s", indent, handling, entity, elem.ExposedName)
			} else {
				fmt.Fprintf(w, "%s%s %s/%s = %s", indent, handling, assoc, entity, elem.ExposedName)
			}
			if len(elem.Children) > 0 {
				fmt.Fprintln(w, " {")
			}
		}
		if len(elem.Children) > 0 {
			for i, child := range elem.Children {
				printImportMappingElement(w, child, depth+1, false)
				if i < len(elem.Children)-1 {
					fmt.Fprintln(w, ",")
				} else {
					fmt.Fprintln(w)
				}
			}
			fmt.Fprintf(w, "%s}", indent)
		}
	} else {
		// Value mapping: Attr = jsonField KEY
		attrName := elem.Attribute
		// Strip module prefix if present (Module.Entity.Attr → Attr)
		if parts := strings.Split(attrName, "."); len(parts) == 3 {
			attrName = parts[2]
		}
		keyStr := ""
		if elem.IsKey {
			keyStr = " key"
		}
		fmt.Fprintf(w, "%s%s = %s%s", indent, attrName, elem.ExposedName, keyStr)
	}
}

// execCreateImportMapping creates a new import mapping.
func execCreateImportMapping(ctx *ExecContext, s *ast.CreateImportMappingStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	existing, _ := ctx.Backend.GetImportMappingByQualifiedName(s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("import mapping", s.Name.String())
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}
	containerID := module.ID

	im := &model.ImportMapping{
		ContainerID: containerID,
		Name:        s.Name.Name,
		ExportLevel: "Hidden",
	}

	// Set schema source reference
	switch s.SchemaKind {
	case "JSON_STRUCTURE":
		im.JsonStructure = s.SchemaRef.String()
	case "XML_SCHEMA":
		im.XmlSchema = s.SchemaRef.String()
	}

	// Index the JSON structure — mapping elements clone their names, path and
	// occurrence bounds from it.
	idx := newJSONSchemaIndex(nil)
	if s.SchemaKind == "JSON_STRUCTURE" && s.SchemaRef.Module != "" {
		if js, err2 := ctx.Backend.GetJsonStructureByQualifiedName(s.SchemaRef.Module, s.SchemaRef.Name); err2 == nil && js != nil {
			idx = newJSONSchemaIndex(js.Elements)
		}
	}

	// Build element tree from the AST definition, cloning JSON structure properties
	if s.RootElement != nil {
		root, err := buildImportMappingElementModel(s.Name.Module, s.RootElement, "", "(Object)", ctx.Backend, idx, true)
		if err != nil {
			return mdlerrors.NewValidation(fmt.Sprintf("import mapping %s: %v", s.Name.String(), err))
		}
		im.Elements = append(im.Elements, root)
	}

	if existing != nil {
		im.ID = existing.ID
		if err := ctx.Backend.UpdateImportMapping(im); err != nil {
			return mdlerrors.NewBackend("update import mapping", err)
		}
		if !ctx.Quiet {
			fmt.Fprintf(ctx.Output, "Modified import mapping %s.%s\n", s.Name.Module, s.Name.Name)
		}
		return nil
	}

	if err := ctx.Backend.CreateImportMapping(im); err != nil {
		return mdlerrors.NewBackend("create import mapping", err)
	}

	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Created import mapping %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// buildImportMappingElementModel converts an AST element definition to a model element.
// It clones properties from the matching JSON structure element (ExposedName, JsonPath,
// MaxOccurs, ElementType, etc.) and adds mapping-specific bindings (Entity, Attribute,
// Association, ObjectHandling).
func buildImportMappingElementModel(moduleName string, def *ast.ImportMappingElementDef, parentEntity, parentPath string, b backend.FullBackend, idx *jsonSchemaIndex, isRoot bool) (*model.ImportMappingElement, error) {
	elem := &model.ImportMappingElement{
		BaseElement: model.BaseElement{
			ID: model.ID(types.GenerateID()),
		},
	}

	// Resolve the member against the JSON structure. The authored name may be
	// the raw JSON key or the exposed name — DESCRIBE emits the latter, so both
	// have to work or mxcli's own output does not round-trip. (#882)
	var jsElem *types.JsonElement
	lookupPath := parentPath + "|" + def.JsonName
	switch {
	case isRoot:
		lookupPath = "(Object)"
		jsElem = idx.byPath[lookupPath]
	default:
		jsElem = idx.resolve(parentPath, def.JsonName)
	}

	// Clone properties from the matching JSON structure element. A member that
	// resolves to nothing is REFUSED, never given a made-up path: the fabricated
	// path passed `mxcli check` and surfaced only later — in mxbuild as CE5015,
	// or at runtime as an unresolvable mapping. (#882)
	if jsElem == nil && !isRoot {
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
		elem.MinOccurs = jsElem.MinOccurs
		elem.MaxOccurs = jsElem.MaxOccurs
		elem.Nillable = jsElem.Nillable
		elem.OriginalValue = jsElem.OriginalValue
		elem.FractionDigits = jsElem.FractionDigits
		elem.TotalDigits = jsElem.TotalDigits
		elem.MaxLength = jsElem.MaxLength
	} else {
		// Root only, and only when the structure could not be read at all.
		elem.ExposedName = def.JsonName
		elem.JsonPath = lookupPath
		elem.Nillable = true
		elem.FractionDigits = -1
		elem.TotalDigits = -1
	}

	if def.Entity != "" {
		// Object/Array mapping — bind to entity
		elem.Kind = "Object"
		elem.TypeName = "ImportMappings$ObjectMappingElement"

		entity := def.Entity
		if !strings.Contains(entity, ".") {
			entity = moduleName + "." + entity
		}

		assoc := def.Association
		if assoc != "" && !strings.Contains(assoc, ".") {
			assoc = moduleName + "." + assoc
		}

		handling := def.ObjectHandling
		if handling == "" {
			handling = "Create"
		}

		elem.Entity = entity
		elem.Association = assoc
		elem.ObjectHandling = handling

		// For arrays: skip the container, use the item path directly.
		// Studio Pro represents arrays as a single ObjectMappingElement at the |(Object) item path.
		childPath := elem.JsonPath
		if jsElem != nil && jsElem.ElementType == "Array" {
			itemPath := jsElem.Path + "|(Object)"
			if jsItem, ok2 := idx.byPath[itemPath]; ok2 {
				elem.ExposedName = jsItem.ExposedName
				elem.JsonPath = jsItem.Path
				elem.MinOccurs = jsItem.MinOccurs
				elem.MaxOccurs = jsItem.MaxOccurs
				elem.Nillable = jsItem.Nillable
			}
			childPath = itemPath
		}

		for _, child := range def.Children {
			c, err := buildImportMappingElementModel(moduleName, child, entity, childPath, b, idx, false)
			if err != nil {
				return nil, err
			}
			elem.Children = append(elem.Children, c)
		}
	} else {
		// Value mapping — bind to attribute
		elem.Kind = "Value"
		elem.TypeName = "ImportMappings$ValueMappingElement"
		elem.DataType = resolveAttributeType(parentEntity, def.Attribute, b)
		elem.IsKey = def.IsKey
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
	}

	return elem, nil
}

// buildJsonElementPathMap recursively builds a map from JSON path → JsonElement.
func buildJsonElementPathMap(elems []*types.JsonElement, m map[string]*types.JsonElement) {
	for _, e := range elems {
		if e == nil {
			continue
		}
		m[e.Path] = e
		buildJsonElementPathMap(e.Children, m)
	}
}

// jsonSchemaIndex resolves a member named in MDL to its JSON structure element.
//
// A JSON structure element carries TWO names, and they routinely differ:
//
//	Path         "(Object)|uuid"   the raw JSON key — what the RUNTIME resolves by
//	ExposedName  "Uuid"            Mendix's derived name — what Studio Pro DISPLAYS
//
// Mendix derives ExposedName by capitalising the initial (and, for an array's
// item object, by suffixing "Item"), so the two diverge for any lowercase-initial
// key. Both are stored, by Studio Pro too — the blank app's own
// FeedbackModule.JSON_AppInsightsResponse holds ExposedName "Uuid" against Path
// "(Object)|uuid".
//
// DESCRIBE prints ExposedName (it is the name Studio Pro shows), so mxcli's own
// output names members the raw-key lookup could not find. Re-executing a DESCRIBE
// therefore FABRICATED a path from the exposed name — "(Object)|Uuid", and for an
// array the "|(Object)" item marker vanished entirely — producing a mapping that
// resolves against nothing at runtime. Accepting both spellings is what makes
// DESCRIBE round-trip. (issue #882)
type jsonSchemaIndex struct {
	byPath   map[string]*types.JsonElement
	children map[string][]*types.JsonElement // parent path → children, in order
}

func newJSONSchemaIndex(elems []*types.JsonElement) *jsonSchemaIndex {
	idx := &jsonSchemaIndex{
		byPath:   map[string]*types.JsonElement{},
		children: map[string][]*types.JsonElement{},
	}
	idx.add("", elems)
	return idx
}

func (i *jsonSchemaIndex) add(parentPath string, elems []*types.JsonElement) {
	for _, e := range elems {
		if e == nil {
			continue
		}
		i.byPath[e.Path] = e
		i.children[parentPath] = append(i.children[parentPath], e)
		i.add(e.Path, e.Children)
	}
}

// resolve finds the element a member name refers to under parentPath, accepting
// the raw JSON key or the exposed name. For an array addressed by its ITEM's
// exposed name ("__ValueItem"), it returns the ARRAY element, so the caller's
// array branch still takes the "|(Object)" step to the item.
//
// Returns nil when the name matches nothing — the caller must refuse rather than
// invent a path. A fabricated path passes `mxcli check` and only fails later, in
// mxbuild as CE5015 or, worse, at runtime.
func (i *jsonSchemaIndex) resolve(parentPath, name string) *types.JsonElement {
	if e, ok := i.byPath[parentPath+"|"+name]; ok {
		return e
	}
	for _, c := range i.children[parentPath] {
		if c.ExposedName == name {
			return c
		}
	}
	for _, c := range i.children[parentPath] {
		if c.ElementType != "Array" {
			continue
		}
		if item, ok := i.byPath[c.Path+"|(Object)"]; ok && item.ExposedName == name {
			return c
		}
	}
	return nil
}

// memberNames lists the spellings that would have resolved under parentPath, so a
// rejection can name them instead of leaving the author to guess.
func (i *jsonSchemaIndex) memberNames(parentPath string) []string {
	var out []string
	for _, c := range i.children[parentPath] {
		raw := c.Path
		if idx := strings.LastIndex(raw, "|"); idx >= 0 {
			raw = raw[idx+1:]
		}
		if c.ExposedName != "" && c.ExposedName != raw {
			out = append(out, fmt.Sprintf("%s (or %s)", raw, c.ExposedName))
			continue
		}
		out = append(out, raw)
	}
	return out
}

// resolveAttributeType looks up the data type of an entity attribute from the project.
// Returns "String" as default if the attribute cannot be found.
func resolveAttributeType(entityQN, attrName string, b backend.DomainModelBackend) string {
	if b == nil || entityQN == "" {
		return "String"
	}
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		return "String"
	}
	// Follow the generalization chain: an inherited attribute is not in the
	// entity's own list, and defaulting it to String gave a mapping element the
	// wrong DataType (mendixlabs/mxcli#703). Resolving by module name also stops a
	// same-named entity in another module being picked up, which the previous
	// scan-every-domain-model loop did.
	if hb, ok := b.(entityLookupBackend); ok {
		if t := ResolveMemberType(hb, entityQN, attrName); t != "" {
			return t
		}
		return "String"
	}

	dms, err := b.ListDomainModels()
	if err != nil {
		return "String"
	}
	for _, dm := range dms {
		for _, e := range dm.Entities {
			if e.Name == parts[1] {
				for _, a := range e.Attributes {
					if a.Name == attrName && a.Type != nil {
						return a.Type.GetTypeName()
					}
				}
			}
		}
	}
	return "String"
}

// execDropImportMapping deletes an import mapping.
func execDropImportMapping(ctx *ExecContext, s *ast.DropImportMappingStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	im, err := ctx.Backend.GetImportMappingByQualifiedName(s.Name.Module, s.Name.Name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return mdlerrors.NewNotFound("import mapping", s.Name.String())
		}
		return mdlerrors.NewBackend("get import mapping", err)
	}

	if err := ctx.Backend.DeleteImportMapping(im.ID); err != nil {
		return mdlerrors.NewBackend("drop import mapping", err)
	}

	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Dropped import mapping %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}
