// SPDX-License-Identifier: Apache-2.0

// Package executor - Java Action commands (SHOW/DESCRIBE/CREATE JAVA ACTIONS)
package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// listJavaActions handles SHOW JAVA ACTIONS command.
func listJavaActions(ctx *ExecContext, moduleName string) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Get all Java actions
	javaActions, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}

	// Collect rows
	type row struct {
		qualifiedName string
		module        string
		name          string
		folderPath    string
	}
	var rows []row

	for _, ja := range javaActions {
		modID := h.FindModuleID(ja.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + ja.Name
			folderPath := h.BuildFolderPath(ja.ContainerID)
			rows = append(rows, row{qualifiedName, modName, ja.Name, folderPath})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Folder"},
		Summary: fmt.Sprintf("(%d java actions)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.folderPath})
	}
	return writeResult(ctx, result)
}

// describeJavaAction handles DESCRIBE JAVA ACTION command - outputs MDL-style representation.
func describeJavaAction(ctx *ExecContext, name ast.QualifiedName) error {
	qualifiedName := name.Module + "." + name.Name
	ja, err := ctx.Backend.ReadJavaActionByName(qualifiedName)
	if err != nil {
		return mdlerrors.NewNotFound("java action", qualifiedName)
	}

	// Generate MDL-style output for CREATE JAVA ACTION format
	var sb strings.Builder

	// Documentation comment (JavaDoc style) — normalize \r\n to \n
	doc := strings.ReplaceAll(ja.Documentation, "\r\n", "\n")
	doc = strings.ReplaceAll(doc, "\r", "\n")
	if doc != "" {
		sb.WriteString("/**\n")
		for line := range strings.SplitSeq(doc, "\n") {
			sb.WriteString(" * ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString(" */\n")
	}

	// Build CREATE JAVA ACTION statement
	sb.WriteString("create java action ")
	sb.WriteString(qualifiedName)
	sb.WriteString(describeFolderClause(ctx, ja.ContainerID))
	sb.WriteString("(")

	// Parameters — one per line when descriptions are present
	hasParamDescriptions := false
	for _, p := range ja.Parameters {
		if p.Description != "" {
			hasParamDescriptions = true
			break
		}
	}

	for i, param := range ja.Parameters {
		if i > 0 {
			sb.WriteString(", ")
		}
		if hasParamDescriptions {
			sb.WriteString("\n    ")
		}
		sb.WriteString(param.Name)
		sb.WriteString(": ")
		if param.ParameterType != nil {
			sb.WriteString(formatJavaActionType(param.ParameterType))
		} else {
			sb.WriteString("Object")
		}
		if param.IsRequired {
			sb.WriteString(" not null")
		}
		if param.Description != "" {
			paramDoc := strings.ReplaceAll(param.Description, "\r\n", "\n")
			paramDoc = strings.ReplaceAll(paramDoc, "\r", "\n")
			firstLine, _, _ := strings.Cut(paramDoc, "\n")
			sb.WriteString("  -- ")
			sb.WriteString(firstLine)
		}
	}
	if hasParamDescriptions {
		sb.WriteString("\n")
	}
	sb.WriteString(")")

	// Return type
	if ja.ReturnType != nil {
		sb.WriteString(" returns ")
		sb.WriteString(formatJavaActionReturnType(ja.ReturnType))
	}

	// RETURN NAME metadata
	if ja.ActionDefaultReturnName != "" {
		sb.WriteString("\n-- return NAME: '")
		sb.WriteString(ja.ActionDefaultReturnName)
		sb.WriteString("'")
	}

	// EXPOSED AS clause
	if ja.MicroflowActionInfo != nil && ja.MicroflowActionInfo.Caption != "" {
		sb.WriteString("\nexposed as '")
		sb.WriteString(ja.MicroflowActionInfo.Caption)
		sb.WriteString("' in '")
		sb.WriteString(ja.MicroflowActionInfo.Category)
		sb.WriteString("'")
		sb.WriteString(describeBitmapComments(ja.MicroflowActionInfo))
	}

	// The grammar requires an `as $$ ... $$` body for CREATE JAVA ACTION, so always
	// emit one. When the .java source can't be read (e.g. add-on modules ship without
	// source on disk), emit a placeholder body so DESCRIBE output still re-parses (#637).
	javaCode := readJavaActionUserCode(ctx.MprPath, name.Module, name.Name)
	sb.WriteString("\nas $$\n")
	if javaCode != "" {
		sb.WriteString(javaCode)
	} else {
		sb.WriteString("// Java source not available from this project; body omitted by DESCRIBE.")
	}
	sb.WriteString("\n$$;")

	// Output the complete statement
	fmt.Fprintln(ctx.Output, sb.String())

	// Additional info as comments
	if ja.ExportLevel != "" && ja.ExportLevel != "Hidden" {
		fmt.Fprintf(ctx.Output, "-- export level: %s\n", ja.ExportLevel)
	}
	if ja.Excluded {
		fmt.Fprintln(ctx.Output, "-- EXCLUDED: true")
	}

	return nil
}

// readJavaActionUserCode reads the Java source file and extracts the user code section.
func readJavaActionUserCode(mprPath, moduleName, actionName string) string {
	if mprPath == "" {
		return ""
	}

	// Build path to Java source file
	projectRoot := filepath.Dir(mprPath)
	moduleNameLower := strings.ToLower(moduleName)
	javaPath := filepath.Join(projectRoot, "javasource", moduleNameLower, "actions", actionName+".java")

	// Read the file
	content, err := os.ReadFile(javaPath)
	if err != nil {
		return ""
	}

	// Extract user code between BEGIN USER CODE and END USER CODE markers
	source := string(content)
	beginMarker := "// begin user CODE"
	endMarker := "// end user CODE"

	beginIdx := strings.Index(source, beginMarker)
	endIdx := strings.Index(source, endMarker)

	if beginIdx == -1 || endIdx == -1 || endIdx <= beginIdx {
		// No markers found, return empty (or could return raw code)
		return ""
	}

	// Extract code between markers (skip the marker line itself)
	userCode := source[beginIdx+len(beginMarker) : endIdx]
	userCode = strings.TrimPrefix(userCode, "\n")
	userCode = strings.TrimSuffix(userCode, "\n")
	userCode = strings.TrimRight(userCode, " \t")

	return userCode
}

// formatJavaActionType formats a Java action parameter type for MDL output.
func formatJavaActionType(t javaactions.CodeActionParameterType) string {
	if t == nil {
		return "Object"
	}
	// EntityTypeParameterType → ENTITY <name> syntax
	if etp, ok := t.(*javaactions.EntityTypeParameterType); ok {
		if etp.TypeParameterName != "" {
			return "entity <" + etp.TypeParameterName + ">"
		}
		return "entity <>"
	}
	return t.TypeString()
}

// formatJavaActionReturnType formats a Java action return type.
func formatJavaActionReturnType(t javaactions.CodeActionReturnType) string {
	if t == nil {
		return "Void"
	}
	return t.TypeString()
}

// execDropJavaAction handles DROP JAVA ACTION statements.
func execDropJavaAction(ctx *ExecContext, s *ast.DropJavaActionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find and delete the Java action
	jas, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}

	for _, ja := range jas {
		modID := h.FindModuleID(ja.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && ja.Name == s.Name.Name {
			if err := ctx.Backend.DeleteJavaAction(ja.ID); err != nil {
				return mdlerrors.NewBackend("delete java action", err)
			}
			if err := ctx.Backend.DeleteJavaSourceFile(modName, ja.Name); err != nil {
				return mdlerrors.NewBackend("delete java source file", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped java action: %s.%s\n", s.Name.Module, s.Name.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("java action", s.Name.Module+"."+s.Name.Name)
}

// execCreateJavaAction handles CREATE JAVA ACTION statements.
func execCreateJavaAction(ctx *ExecContext, s *ast.CreateJavaActionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the module
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("get modules", err)
	}

	var containerID model.ID
	var moduleName string
	for _, mod := range modules {
		if mod.Name == s.Name.Module {
			containerID = mod.ID
			moduleName = mod.Name
			break
		}
	}
	if containerID == "" {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}

	// Check if Java action already exists
	jas, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}
	var existingJAID model.ID
	var existingContainer model.ID
	// Target the live action and carry its exclusion forward (#914).
	existingExcluded := false
	var existingActionInfo *javaactions.MicroflowActionInfo
	if existing, ok := pickLive(jas,
		func(ja *types.JavaAction) bool {
			return h.GetModuleName(h.FindModuleID(ja.ContainerID)) == s.Name.Module && ja.Name == s.Name.Name
		},
		func(ja *types.JavaAction) bool { return ja.Excluded },
	); ok {
		if !s.CreateOrModify {
			return mdlerrors.NewAlreadyExists("java action", s.Name.Module+"."+s.Name.Name)
		}
		existingJAID = existing.ID
		existingExcluded = existing.Excluded
		existingContainer = existing.ContainerID
		// The toolbox entry holds four PNG bitmaps MDL cannot name, so the
		// stored one has to be read before the rewrite can carry them.
		if full, err := ctx.Backend.ReadJavaActionByName(s.Name.Module + "." + s.Name.Name); err == nil && full != nil {
			existingActionInfo = full.MicroflowActionInfo
		}
	}

	moduleID := containerID
	containerID, err = containerForDocument(ctx, moduleID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	newID := model.ID(types.GenerateID())
	if existingJAID != "" {
		newID = existingJAID
	}

	// Create the Java action
	ja := &javaactions.JavaAction{
		BaseElement: model.BaseElement{
			ID:       newID,
			TypeName: "JavaActions$JavaAction",
		},
		Excluded:      existingExcluded,
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		ExportLevel:   "Public",
	}

	// Build type parameter definitions (with IDs for BY_ID references)
	typeParamNameToID := make(map[string]model.ID)
	for _, tpName := range s.TypeParameters {
		tpDef := &javaactions.TypeParameterDef{
			BaseElement: model.BaseElement{
				ID: model.ID(types.GenerateID()),
			},
			Name: tpName,
		}
		ja.TypeParameters = append(ja.TypeParameters, tpDef)
		typeParamNameToID[tpName] = tpDef.ID
	}

	// Build a set of type parameter names for quick lookup
	typeParamNames := make(map[string]bool)
	for _, tpName := range s.TypeParameters {
		typeParamNames[tpName] = true
	}

	// Convert parameters:
	// - TypeEntityTypeParam → EntityTypeParameterType (entity type selector)
	// - Bare name matching a type parameter → TypeParameter (ParameterizedEntityType)
	// - Other types → convert normally
	for _, param := range s.Parameters {
		jaParam := &javaactions.JavaActionParameter{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "JavaActions$JavaActionParameter",
			},
			Name:       param.Name,
			IsRequired: param.IsRequired,
		}
		if param.Type.Kind == ast.TypeEntityTypeParam {
			// Explicit ENTITY <pEntity> → EntityTypeParameterType (entity type selector)
			tpName := param.Type.TypeParamName
			jaParam.ParameterType = &javaactions.EntityTypeParameterType{
				BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
				TypeParameterID:   typeParamNameToID[tpName],
				TypeParameterName: tpName,
			}
		} else if isTypeParamRef(param.Type, typeParamNames) {
			// Bare name matching a type parameter → TypeParameter (ParameterizedEntityType)
			tpName := getTypeParamRefName(param.Type)
			jaParam.ParameterType = &javaactions.TypeParameter{
				BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
				TypeParameterID: typeParamNameToID[tpName],
				TypeParameter:   tpName,
			}
		} else {
			jaParam.ParameterType = astDataTypeToJavaActionParamType(param.Type)
		}
		ja.Parameters = append(ja.Parameters, jaParam)
	}

	// Convert return type — check if it references a type parameter
	if isTypeParamRef(s.ReturnType, typeParamNames) {
		tpName := getTypeParamRefName(s.ReturnType)
		ja.ReturnType = &javaactions.TypeParameter{
			BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
			TypeParameterID: typeParamNameToID[tpName],
			TypeParameter:   tpName,
		}
	} else {
		ja.ReturnType = astDataTypeToJavaActionReturnType(s.ReturnType)
	}

	// Fold the EXPOSED AS clause onto the stored toolbox entry rather than
	// rebuilding it: the four bitmaps are not expressible in MDL.
	if ja.MicroflowActionInfo, err = mergeMicroflowActionInfo(ctx,
		existingActionInfo, s.ExposedCaption, s.ExposedCategory, s.NotExposed,
		s.ExposedBitmaps, exposeWarner(ctx)); err != nil {
		return err
	}

	// Create or update in MPR
	if existingJAID != "" {
		if err := ctx.Backend.UpdateJavaAction(ja); err != nil {
			return mdlerrors.NewBackend("update java action", err)
		}
		if _, err := applyDocumentFolder(ctx, ja.ID, existingContainer, containerID); err != nil {
			return err
		}
	} else {
		if err := ctx.Backend.CreateJavaAction(ja); err != nil {
			return mdlerrors.NewBackend("create java action", err)
		}
	}

	// Write Java source file if code is provided
	if s.JavaCode != "" {
		if err := ctx.Backend.WriteJavaSourceFile(moduleName, s.Name.Name, s.JavaCode, ja.Parameters, ja.ReturnType, s.Imports, s.ExtraCode); err != nil {
			return mdlerrors.NewBackend("write java source file", err)
		}
	}

	// Clear cache
	ctx.InvalidateCache()

	if existingJAID != "" {
		ctx.ReportMutation("Modified", "java action: %s.%s", s.Name.Module, s.Name.Name)
	} else {
		fmt.Fprintf(ctx.Output, "Created java action: %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// astDataTypeToJavaActionParamType converts an AST DataType to a Java action parameter type.
func astDataTypeToJavaActionParamType(dt ast.DataType) javaactions.CodeActionParameterType {
	switch dt.Kind {
	case ast.TypeBoolean:
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$BooleanType",
			},
		}
	case ast.TypeInteger:
		return &javaactions.IntegerType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$IntegerType",
			},
		}
	case ast.TypeLong:
		return &javaactions.LongType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$LongType",
			},
		}
	case ast.TypeDecimal:
		return &javaactions.DecimalType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$DecimalType",
			},
		}
	case ast.TypeString:
		return &javaactions.StringType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$StringType",
			},
		}
	case ast.TypeDateTime, ast.TypeDate:
		return &javaactions.DateTimeType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$DateTimeType",
			},
		}
	case ast.TypeEntity, ast.TypeEnumeration:
		// An unambiguous `ENUM`/`Enumeration(...)` type must serialize as an
		// enumeration parameter, not an entity reference (#680). A bare
		// qualified name (ExplicitEnum=false) can't be told apart from an entity
		// by the parser, so it stays an entity type here — callers that have the
		// project loaded resolve it via resolveCodeActionParamType.
		if dt.Kind == ast.TypeEnumeration && dt.ExplicitEnum && dt.EnumRef != nil {
			return &javaactions.EnumerationType{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "CodeActions$EnumerationType",
				},
				Enumeration: dt.EnumRef.Module + "." + dt.EnumRef.Name,
			}
		}
		// `Microflow` / `Nanoflow` are what DESCRIBE prints for a callback
		// parameter (MCPServer.AddTool's ExecutingMicroflow, and every other
		// "register a handler" action). The parser cannot tell a bare name from
		// an entity, so without this the DESCRIBE output round-tripped into an
		// entity type with an empty module — `.Microflow`. Only the unqualified
		// name is treated this way; a real `Module.Microflow` entity is not.
		if bare := bareDataTypeName(dt); bare == "Microflow" || bare == "Nanoflow" {
			id := model.ID(types.GenerateID())
			if bare == "Nanoflow" {
				return &javaactions.NanoflowType{BaseElement: model.BaseElement{ID: id}}
			}
			return &javaactions.MicroflowType{BaseElement: model.BaseElement{ID: id}}
		}
		entityName := ""
		if dt.EntityRef != nil {
			entityName = dt.EntityRef.Module + "." + dt.EntityRef.Name
		} else if dt.EnumRef != nil {
			entityName = dt.EnumRef.Module + "." + dt.EnumRef.Name
		}
		return &javaactions.EntityType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$EntityType",
			},
			Entity: entityName,
		}
	case ast.TypeListOf:
		entityName := ""
		if dt.EntityRef != nil {
			entityName = dt.EntityRef.Module + "." + dt.EntityRef.Name
		}
		return &javaactions.ListType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$ListType",
			},
			Entity: entityName,
		}
	default:
		// Default to String type for unknown kinds
		return &javaactions.StringType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$StringType",
			},
		}
	}
}

// bareDataTypeName returns the name of an unqualified entity/enumeration data
// type (no module part), or "" when the type is qualified or absent.
func bareDataTypeName(dt ast.DataType) string {
	switch {
	case dt.EntityRef != nil && dt.EntityRef.Module == "":
		return dt.EntityRef.Name
	case dt.EnumRef != nil && dt.EnumRef.Module == "":
		return dt.EnumRef.Name
	}
	return ""
}

// astDataTypeToJavaActionReturnType converts an AST DataType to a Java action return type.
func astDataTypeToJavaActionReturnType(dt ast.DataType) javaactions.CodeActionReturnType {
	switch dt.Kind {
	case ast.TypeVoid:
		return &javaactions.VoidType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$VoidType",
			},
		}
	case ast.TypeBoolean:
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$BooleanType",
			},
		}
	case ast.TypeInteger:
		return &javaactions.IntegerType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$IntegerType",
			},
		}
	case ast.TypeLong:
		return &javaactions.LongType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$LongType",
			},
		}
	case ast.TypeDecimal:
		return &javaactions.DecimalType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$DecimalType",
			},
		}
	case ast.TypeString:
		return &javaactions.StringType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$StringType",
			},
		}
	case ast.TypeDateTime, ast.TypeDate:
		return &javaactions.DateTimeType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$DateTimeType",
			},
		}
	case ast.TypeEntity, ast.TypeEnumeration:
		// An unambiguous `ENUM`/`Enumeration(...)` return type serializes as an
		// enumeration, not an entity reference (#680). A bare qualified name
		// can't be told apart from an entity, so it stays an entity type.
		// "void" parses as a bare qualified name; treat it as VoidType.
		if dt.Kind == ast.TypeEnumeration && dt.ExplicitEnum && dt.EnumRef != nil {
			return &javaactions.EnumerationType{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "CodeActions$EnumerationType",
				},
				Enumeration: dt.EnumRef.Module + "." + dt.EnumRef.Name,
			}
		}
		entityName := ""
		if dt.EntityRef != nil {
			entityName = dt.EntityRef.Module + "." + dt.EntityRef.Name
		} else if dt.EnumRef != nil {
			entityName = dt.EnumRef.Module + "." + dt.EnumRef.Name
		}
		if strings.EqualFold(strings.TrimPrefix(entityName, "."), "void") {
			return &javaactions.VoidType{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "CodeActions$VoidType",
				},
			}
		}
		return &javaactions.EntityType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$EntityType",
			},
			Entity: entityName,
		}
	case ast.TypeListOf:
		entityName := ""
		if dt.EntityRef != nil {
			entityName = dt.EntityRef.Module + "." + dt.EntityRef.Name
		}
		return &javaactions.ListType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$ListType",
			},
			Entity: entityName,
		}
	default:
		// Default to Boolean type (most common for Java actions)
		return &javaactions.BooleanType{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CodeActions$BooleanType",
			},
		}
	}
}

// isTypeParamRef checks if an AST DataType is a bare name that matches a type parameter.
// Type parameter names like "pEntity" parse as either TypeEnumeration (with EnumRef)
// or TypeEntity (with EntityRef) with a single-part name (no module prefix).
func isTypeParamRef(dt ast.DataType, typeParamNames map[string]bool) bool {
	name := getTypeParamRefName(dt)
	return name != "" && typeParamNames[name]
}

// getTypeParamRefName extracts the name from a DataType that could be a type parameter reference.
// Returns empty string if the DataType doesn't look like a type parameter reference.
func getTypeParamRefName(dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeEnumeration:
		if dt.EnumRef != nil && dt.EnumRef.Module == "" {
			return dt.EnumRef.Name
		}
		if dt.EnumRef != nil {
			return dt.EnumRef.Module + "." + dt.EnumRef.Name
		}
	case ast.TypeEntity:
		if dt.EntityRef != nil && dt.EntityRef.Module == "" {
			return dt.EntityRef.Name
		}
		if dt.EntityRef != nil {
			return dt.EntityRef.Module + "." + dt.EntityRef.Name
		}
	}
	return ""
}
