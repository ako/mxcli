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

	fmt.Fprintf(ctx.Output, "create or modify import mapping %s.%s\n", moduleName, im.Name)
	// Without this the description round-trips to the module root: replaying
	// it in a fresh project would recreate the mapping unfiled (#932).
	if folderPath := h.BuildFolderPath(im.ContainerID); folderPath != "" {
		fmt.Fprintf(ctx.Output, "  folder '%s'\n", folderPath)
	}

	if im.JsonStructure != "" {
		rootClause := ""
		if len(im.Elements) > 0 {
			rootClause = schemaRootClause(im.Elements[0].JsonPath)
		}
		fmt.Fprintf(ctx.Output, "  with json structure %s%s\n", im.JsonStructure, rootClause)
	} else if im.XmlSchema != "" {
		fmt.Fprintf(ctx.Output, "  with xml schema %s\n", im.XmlSchema)
	} else if im.MessageDefinition != "" {
		// Dropped entirely before #263 — and the output still PARSED, so
		// re-executing a DESCRIBE rebuilt the mapping bound to nothing.
		fmt.Fprintf(ctx.Output, "  with message definition %s\n", im.MessageDefinition)
	}
	// The input object (#265). Printing it is what makes a `Param: parameter`
	// handler in the body re-executable at all.
	if im.ParameterEntity != "" {
		fmt.Fprintf(ctx.Output, "  parameter %s\n", im.ParameterEntity)
	}

	if len(im.Elements) > 0 {
		fmt.Fprintln(ctx.Output, "{")
		for _, elem := range im.Elements {
			printImportMappingElement(ctx.Output, elem, 1, true, "")
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
	case "Custom":
		// The `by <microflow>` clause is what makes it Custom, and it reads as
		// "find the object by calling this microflow" — so the word stays `find`
		// and the clause carries the meaning. Printing `create` here would
		// re-execute to Create and lose the handler (#264).
		return "find"
	default:
		return "create"
	}
}

func printImportMappingElement(w io.Writer, elem *model.ImportMappingElement, depth int, isRoot bool, parentPath string) {
	indent := strings.Repeat("  ", depth)
	if elem.Kind == "Object" {
		handling := handlingKeyword(elem.ObjectHandling)
		by := customHandlerText(elem.CustomHandler, elem.JsonPath)
		backup := handlingBackupText(elem)
		if isRoot {
			// Root: CREATE Module.Entity { — use "." if entity is empty
			entity := elem.Entity
			if entity == "" {
				entity = "."
			}
			fmt.Fprintf(w, "%s%s %s%s%s {\n", indent, handling, entity, by, backup)
		} else {
			// Nested object element:
			//   CREATE Assoc/Entity = jsonKey   — normal association path
			//   CREATE ./Entity = jsonKey       — self-reference (no association)
			//   CREATE . = jsonKey              — structural grouping (no association, no entity)
			assoc := elem.Association
			entity := elem.Entity
			if assoc == "" && entity == "" {
				fmt.Fprintf(w, "%s%s . = %s", indent, handling, mappingMemberName(parentPath, elem.JsonPath, elem.ExposedName))
			} else if assoc == "" {
				fmt.Fprintf(w, "%s%s ./%s = %s", indent, handling, entity, mappingMemberName(parentPath, elem.JsonPath, elem.ExposedName))
			} else {
				fmt.Fprintf(w, "%s%s %s/%s%s%s = %s", indent, handling, assoc, entity, by, backup, mappingMemberName(parentPath, elem.JsonPath, elem.ExposedName))
			}
			if len(printableImportChildren(elem)) > 0 {
				fmt.Fprintln(w, " {")
			}
		}
		children := printableImportChildren(elem)
		if len(children) > 0 {
			for i, child := range children {
				printImportMappingElement(w, child, depth+1, false, elem.JsonPath)
				if i < len(children)-1 {
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
		member := mappingMemberName(parentPath, elem.JsonPath, elem.ExposedName)
		// A converter is written as a call around the member it transforms —
		// the stored element has one microflow and no separate parameter, so
		// the member inside the call IS this element's binding (#266).
		if elem.Converter != "" {
			fmt.Fprintf(w, "%s%s = %s(%s)%s", indent, attrName, elem.Converter, member, keyStr)
			return
		}
		fmt.Fprintf(w, "%s%s = %s%s", indent, attrName, member, keyStr)
	}
}

// mappingMemberName is the JSON member DESCRIBE should print for a mapping
// element: the raw key(s) taken from its JsonPath, not the derived ExposedName.
//
// The two differ for any lowercase-initial key, because Mendix derives
// ExposedName by capitalising the initial (and suffixing "Item" for an array's
// item object) — Studio Pro does the same, so the stored ExposedName is correct
// and is deliberately left alone. But printing it made DESCRIBE output that no
// longer matched the script that produced it: `Total = total` came back as
// `Total = Total`, `= item` as `= ItemItem`, `LineId = id` as `LineId = _id`.
// The mapping those re-execute to is identical (jsonSchemaIndex.resolve accepts
// either spelling since #882), but a diff of script vs DESCRIBE was pure noise.
//
// The raw key is also the index's FIRST lookup, so printing it cannot regress
// that resolution. Elements with no JsonPath — an XML-schema or
// message-definition mapping — keep the exposed name, which is all they have.
// (issue #915)
//
// The member is rendered RELATIVE to the enclosing object element rather than as
// its last segment alone. For a direct child the two are the same, but Studio
// Pro can bind a leaf several levels below the object element it belongs to,
// with no entity for the levels in between: a value element at
// "(Object)|customer|name" under an object element at "(Object)". Printing only
// "name" dropped the intermediate levels, so DESCRIBE reported a mapping the
// project did not contain and its own output no longer re-executed —
// `"name" is not a member of the JSON structure at (Object)`. (issue #927)
func mappingMemberName(parentPath, jsonPath, exposedName string) string {
	if jsonPath == "" {
		return exposedName
	}
	// An array's item object is addressed by the ARRAY's key: the mapping element
	// sits at "(Object)|item|(Object)" and the script wrote "item".
	trimmed := strings.TrimSuffix(jsonPath, "|(Object)")
	// An array of PRIMITIVES is addressed by the ARRAY's key too — the mapping
	// element sits at "…|tags|(Wrapper)" and the script wrote "tags" (#268).
	trimmed = strings.TrimSuffix(trimmed, "|(Wrapper)")

	// The parent path is used verbatim: for an array, the object element's own
	// JsonPath is already the ITEM path ("…|items|(Object)"), so trimming the
	// marker off it made a child of that item render as "(Object)/sku".
	if parentPath != "" {
		if rel := strings.TrimPrefix(trimmed, parentPath+"|"); rel != trimmed && rel != "" {
			// "(Value)" is the primitive an array-of-primitives holds — a
			// storage marker, not a member name. Its exposed name is "Value",
			// which is what re-executing resolves against (#268).
			if rel == "(Value)" {
				return exposedName
			}
			return strings.ReplaceAll(rel, "|", "/")
		}
	}

	i := strings.LastIndex(trimmed, "|")
	if i < 0 {
		return exposedName
	}
	if name := trimmed[i+1:]; name != "" && name != "(Object)" {
		return name
	}
	return exposedName
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

	im := &model.ImportMapping{
		ContainerID: containerID,
		Name:        s.Name.Name,
		ExportLevel: "Hidden",
	}
	if existing != nil {
		// Excluded is model state, not script state (#914).
		im.Excluded = existing.Excluded
	}

	// The mapping's input object (#265), which `Param: parameter` binds.
	paramEntity, err := resolveMappingParameter(s.Parameter, s.Name.Module, ctx.Backend)
	if err != nil {
		return mdlerrors.NewValidation(fmt.Sprintf("import mapping %s: %v", s.Name.String(), err))
	}
	im.ParameterEntity = paramEntity

	// Set schema source reference
	switch s.SchemaKind {
	case "JSON_STRUCTURE":
		im.JsonStructure = s.SchemaRef.String()
	case "XML_SCHEMA":
		im.XmlSchema = s.SchemaRef.String()
	case "MESSAGE_DEFINITION":
		im.MessageDefinition = s.SchemaRef.String()
	}

	// A message definition resolves against the domain model, not a payload
	// sample, and the mapping stores both path families — its own builder (#263).
	if s.SchemaKind == "MESSAGE_DEFINITION" {
		md, err := findMessageDefinition(ctx.Backend, im.MessageDefinition)
		if err != nil {
			return mdlerrors.NewValidation(fmt.Sprintf("import mapping %s: %v", s.Name.String(), err))
		}
		if s.RootElement != nil {
			root, err := buildImportMappingFromMessageDefinition(s.Name.Module, s.RootElement,
				md.Root, "", "", true, ctx.Backend)
			if err != nil {
				return mdlerrors.NewValidation(fmt.Sprintf("import mapping %s: %v", s.Name.String(), err))
			}
			im.Elements = append(im.Elements, root)
		}
		return finishImportMapping(ctx, s, im, existing, containerID)
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
	// `root a/b/c` starts the mapping at a nested schema element (#267).
	rootPath := ""
	if s.SchemaRoot != "" {
		if !idx.resolvable() {
			return mdlerrors.NewValidation(fmt.Sprintf("import mapping %s: `root %s` needs a schema "+
				"source that can be read", s.Name.String(), s.SchemaRoot))
		}
		je, err := resolveSchemaRoot(idx, s.SchemaRoot)
		if err != nil {
			return mdlerrors.NewValidation(fmt.Sprintf("import mapping %s: %v", s.Name.String(), err))
		}
		rootPath = je.Path
	}

	if s.RootElement != nil {
		root, err := buildImportMappingElementModel(s.Name.Module, s.RootElement, "", rootPath, ctx.Backend, idx, true)
		if err != nil {
			return mdlerrors.NewValidation(fmt.Sprintf("import mapping %s: %v", s.Name.String(), err))
		}
		im.Elements = append(im.Elements, root)
	}

	return finishImportMapping(ctx, s, im, existing, containerID)
}

// finishImportMapping writes the built mapping, shared by the JSON-structure and
// message-definition paths so the two cannot drift on placement or reporting.
func finishImportMapping(ctx *ExecContext, s *ast.CreateImportMappingStmt,
	im *model.ImportMapping, existing *model.ImportMapping, containerID model.ID,
) error {
	if err := requireDeclaredParameter(im); err != nil {
		return mdlerrors.NewValidation(fmt.Sprintf("import mapping %s: %v", s.Name.String(), err))
	}
	if existing != nil {
		im.ID = existing.ID
		if err := ctx.Backend.UpdateImportMapping(im); err != nil {
			return mdlerrors.NewBackend("update import mapping", err)
		}
		if _, err := applyDocumentFolder(ctx, im.ID, existing.ContainerID, containerID); err != nil {
			return err
		}
		if !ctx.Quiet {
			ctx.ReportMutation("Modified", "import mapping %s.%s", s.Name.Module, s.Name.Name)
		}
		return nil
	}

	if err := ctx.Backend.CreateImportMapping(im); err != nil {
		return mdlerrors.NewBackend("create import mapping", err)
	}
	invalidateHierarchy(ctx)

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
		// The structure decides where its own root is, and Studio Pro does not
		// ask either: an object-rooted structure is built at "(Object)", an
		// array-rooted one at "(Array)". Hardcoding "(Object)" made every
		// array-rooted mapping unauthorable and reported its members missing
		// "at (Object)", a node that structure never had (#248).
		//
		// No extra step is needed for the array itself: the array branch below
		// was already taking "(Array)" -> "(Array)|(Object)" for NESTED arrays,
		// and a root array needs exactly that. The result matches Studio Pro,
		// which stores an array-rooted import mapping as a SINGLE element at
		// "(Array)|(Object)" with no container (measured on
		// Teamcenter.IMM_ItemRevision and FactoryManagement.IMM_ScenarioList,
		// Mendix 11.13).
		// parentPath carries an explicit `root a/b/c` selection when there is
		// one; otherwise the structure's own root is used (#248, #267).
		lookupPath = parentPath
		if lookupPath == "" {
			lookupPath = "(Object)"
			if root := idx.root(); root != nil {
				lookupPath = root.Path
			}
		}
		jsElem = idx.byPath[lookupPath]
	default:
		var arrayLevel *types.JsonElement
		jsElem, arrayLevel = idx.resolvePath(parentPath, def.JsonName)
		if arrayLevel != nil {
			return nil, fmt.Errorf("%q passes through %q, which occurs 0..* — a value cannot be pulled "+
				"through many items, and mxbuild rejects it with CE0256 (\"a schema element with wrong "+
				"occurrence\" between the value mapping and its parent). Give %q its own element with an "+
				"association: create Assoc/Module.Entity = %s { ... }",
				def.JsonName, arrayLevel.ExposedName, arrayLevel.ExposedName, arrayLevel.ExposedName)
		}
	}

	// Clone properties from the matching JSON structure element. A member that
	// resolves to nothing is REFUSED, never given a made-up path: the fabricated
	// path passed `mxcli check` and surfaced only later — in mxbuild as CE5015,
	// or at runtime as an unresolvable mapping. (#882)
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
		elem.MinOccurs = jsElem.MinOccurs
		elem.MaxOccurs = jsElem.MaxOccurs
		elem.Nillable = jsElem.Nillable
		// OriginalValue is deliberately NOT cloned. It is the sample value parsed
		// out of the JSON structure's snippet ("42", "\"Widget\""), and it belongs
		// to the STRUCTURE — Studio Pro leaves it empty on every mapping element.
		// Measured across the two Studio-Pro-authored mappings a blank app ships
		// (FeedbackModule's IMM_PostResponse and EMM_PostFeedback, ~15 value
		// elements between them): all "", while their structures carry 17 non-empty
		// samples. Copying the sample in makes an mxcli-written mapping differ from
		// a Studio-Pro-written one over the same structure. (issue #882)
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
		// `find` on its own does not say what happens when the object is NOT
		// found, and Mendix has three answers. mxcli used to write the handling
		// into the backup — "Find", which is not one of {Create, Error, Ignore}
		// and appears in 0 of the 1,261 object elements in the demo apps. Refuse
		// rather than choose: the corpus has no dominant default (Create 2,
		// Error 6, Ignore 18) and each means something different (#261).
		if handling == "Find" && def.Backup == "" && def.CustomHandler == nil {
			return nil, fmt.Errorf("`find %s` does not say what to do when the object is not "+
				"found — add one of: or create (the old `find or create`), or ignore, or error",
				def.Entity)
		}
		backup := def.Backup

		elem.Entity = entity
		elem.Association = assoc
		elem.ObjectHandling = handling
		elem.ObjectHandlingBackup = backup
		elem.BackupAllowOverride = def.BackupOverridable

		// For arrays: skip the container and bind the item directly, which is
		// how Studio Pro stores an import mapping over an array.
		//
		// The step is to the array's ACTUAL child rather than a hardcoded
		// "|(Object)": an array of OBJECTS has an item at "|(Object)", an array
		// of PRIMITIVES a wrapper at "|(Wrapper)" whose single child is the
		// value (#268). The element takes the child's ElementType too, so a
		// primitive array is stored as a Wrapper element the way Studio Pro
		// writes it (KrogerAPI.IM_ProductList).
		childPath := elem.JsonPath
		if jsElem != nil && jsElem.ElementType == "Array" {
			if jsItem := arrayItemOf(idx, jsElem); jsItem != nil {
				elem.ExposedName = jsItem.ExposedName
				elem.JsonPath = jsItem.Path
				elem.MinOccurs = jsItem.MinOccurs
				elem.MaxOccurs = jsItem.MaxOccurs
				elem.Nillable = jsItem.Nillable
				if jsItem.ElementType == "Wrapper" {
					elem.Kind = "Wrapper"
				}
				childPath = jsItem.Path
			} else {
				childPath = jsElem.Path + "|(Object)"
			}
		}

		// `find X by MF(...)` is stored as ObjectHandling "Custom" — the
		// microflow IS the find (#264). Built HERE, after the array branch has
		// settled JsonPath: a value-path parameter is relative to the element's
		// final path, which for an array is the ITEM's.
		if def.CustomHandler != nil {
			ch, err := buildCustomHandler(def.CustomHandler, moduleName, elem.JsonPath, b)
			if err != nil {
				return nil, err
			}
			elem.CustomHandler = ch
			elem.ObjectHandling = "Custom"
		}

		for _, child := range def.Children {
			c, err := buildImportMappingElementModel(moduleName, child, entity, childPath, b, idx, false)
			if err != nil {
				return nil, err
			}
			elem.Children = append(elem.Children, c)
		}
		// A value-path parameter needs the value it keys on to exist as an
		// element, or mxbuild reports CE0281 (#264). Added after the authored
		// children so one the author already mapped is not duplicated.
		if err := addCustomHandlerValueElements(elem, def.CustomHandler, idx, childPath); err != nil {
			return nil, err
		}
	} else {
		// Value mapping — bind to attribute
		elem.Kind = "Value"
		elem.TypeName = "ImportMappings$ValueMappingElement"
		elem.DataType = resolveAttributeType(parentEntity, def.Attribute, b)
		elem.IsKey = def.IsKey
		// The value may pass through a microflow on its way to the attribute
		// (#266). The stored element carries only the microflow — its input is
		// the member this element already binds, which is why the syntax names
		// the member inside the call.
		if err := setMappingConverter(&elem.Converter, def.Converter, moduleName, b); err != nil {
			return nil, err
		}
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
// resolvePath resolves a `/`-separated member path under parentPath, one segment
// at a time so every step keeps resolve's tolerance for the raw key or the
// exposed name.
//
// A single segment is the ordinary direct-child case. Several segments reach a
// leaf BELOW the enclosing object element with no entity for the levels in
// between — the shape Studio Pro produces when a nested leaf is ticked without
// its parents, stored as one multi-segment JsonPath. Measured on mxbuild 11.13:
// a value element at "(Object)|customer|name" under an object element at
// "(Object)", with nothing mapped for "customer", builds at 0 errors.
//
// An intermediate ARRAY is refused by the caller: measured, mxbuild rejects a
// value element whose path crosses a 0..* element with CE0256, so many items
// genuinely cannot collapse into one value.
func (i *jsonSchemaIndex) resolvePath(parentPath, path string) (*types.JsonElement, *types.JsonElement) {
	segments := strings.Split(path, "/")
	current := parentPath
	var elem *types.JsonElement
	for n, seg := range segments {
		elem = i.resolve(current, seg)
		if elem == nil {
			return nil, nil
		}
		if n < len(segments)-1 && elem.ElementType == "Array" {
			return nil, elem // caller reports which level is the array
		}
		current = elem.Path
	}
	return elem, nil
}

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

// root returns the structure's single top-level element, whatever its path.
// Returns nil for a structure that was not loaded, and for the (impossible in
// practice) multi-root case, so the caller keeps its "(Object)" default rather
// than guessing.
func (i *jsonSchemaIndex) root() *types.JsonElement {
	if len(i.children[""]) != 1 {
		return nil
	}
	return i.children[""][0]
}

// resolvable reports whether a JSON structure was actually loaded.
//
// `create import mapping X { ... }` with no `with json structure` clause is
// legal MDL, and an XML-schema or message-definition mapping resolves no JSON
// elements either. There is nothing to validate a member against in those cases,
// so names must be taken at face value — refusing them broke eight round-trip
// tests that create schema-less mappings. The refusal only applies where a
// schema exists to contradict the name. (issue #882)
func (i *jsonSchemaIndex) resolvable() bool { return len(i.byPath) > 0 }

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

// printableImportChildren drops the attribute-less value elements that exist
// only to feed a custom handler's value-path parameter (#264).
//
// They are DERIVED from the `by (Param: member)` clause — mxcli adds them
// because Mendix requires the value to exist as an element (CE0281) — so
// re-executing the clause rebuilds them. Printing them instead would emit
// ` = idx` with no attribute, which does not parse: exactly the shape that puts
// a mapping in #260's silent-loss set.
func printableImportChildren(elem *model.ImportMappingElement) []*model.ImportMappingElement {
	derived := map[string]bool{}
	if elem.CustomHandler != nil {
		for _, p := range elem.CustomHandler.Parameters {
			if p.Source == "path" && p.ValuePath != "" {
				derived[p.ValuePath] = true
			}
		}
	}
	if len(derived) == 0 {
		return elem.Children
	}
	out := make([]*model.ImportMappingElement, 0, len(elem.Children))
	for _, c := range elem.Children {
		if c.Kind != "Object" && c.Attribute == "" && derived[c.JsonPath] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// handlingBackupText renders the `or create|error|ignore [overridable]`
// continuation for DESCRIBE, or "" when the handling already implies it (#261).
//
// `find or create` prints as itself — the reader maps Find + Create back to
// FindOrCreate — and `create` implies Create, so only the shapes that carry
// information print a clause.
func handlingBackupText(elem *model.ImportMappingElement) string {
	backup := elem.ObjectHandlingBackup
	switch backup {
	case "Create", "Error", "Ignore":
	default:
		return ""
	}
	if elem.ObjectHandling != "Find" && backup == "Create" {
		// create / find-or-create / a custom handler: Create is the default and
		// saying so adds nothing.
		return ""
	}
	out := " or " + strings.ToLower(backup)
	if elem.BackupAllowOverride {
		out += " overridable"
	}
	return out
}
