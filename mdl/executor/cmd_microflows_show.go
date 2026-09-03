// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow SHOW/DESCRIBE commands
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/microflowgraph"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// listMicroflows handles SHOW MICROFLOWS command.
func listMicroflows(ctx *ExecContext, moduleName string) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Validate module exists if specified
	if moduleName != "" {
		if _, err := findModule(ctx, moduleName); err != nil {
			return err
		}
	}

	// Get all microflows
	microflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	// Collect rows and calculate column widths
	type row struct {
		qualifiedName string
		module        string
		name          string
		excluded      bool
		folderPath    string
		params        int
		activities    int
		complexity    int
		returnType    string
	}
	var rows []row

	for _, mf := range microflows {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + mf.Name
			folderPath := h.BuildFolderPath(mf.ContainerID)
			returnType := ""
			if mf.ReturnType != nil {
				returnType = mf.ReturnType.GetTypeName()
			}

			// Count activities (excluding structural elements like Start/End events)
			activityCount := countMicroflowActivities(mf)

			// Calculate McCabe cyclomatic complexity
			complexity := calculateMicroflowComplexity(mf)

			rows = append(rows, row{qualifiedName, modName, mf.Name, mf.Excluded, folderPath, len(mf.Parameters), activityCount, complexity, returnType})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Excluded", "Folder", "Params", "Actions", "McCabe", "Returns"},
		Summary: fmt.Sprintf("(%d microflows)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.excluded, r.folderPath, r.params, r.activities, r.complexity, r.returnType})
	}
	return writeResult(ctx, result)
}

// listNanoflows handles SHOW NANOFLOWS command.
func listNanoflows(ctx *ExecContext, moduleName string) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Validate module exists if specified
	if moduleName != "" {
		if _, err := findModule(ctx, moduleName); err != nil {
			return err
		}
	}

	// Get all nanoflows
	nanoflows, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("list nanoflows", err)
	}

	// Collect rows and calculate column widths
	type row struct {
		qualifiedName string
		module        string
		name          string
		excluded      bool
		folderPath    string
		params        int
		activities    int
		complexity    int
		returnType    string
	}
	var rows []row

	for _, nf := range nanoflows {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName == "" || modName == moduleName {
			qualifiedName := modName + "." + nf.Name
			folderPath := h.BuildFolderPath(nf.ContainerID)
			returnType := ""
			if nf.ReturnType != nil {
				returnType = nf.ReturnType.GetTypeName()
			}

			// Count activities (excluding structural elements like Start/End events)
			activityCount := countNanoflowActivities(nf)

			// Calculate McCabe cyclomatic complexity
			complexity := calculateNanoflowComplexity(nf)

			rows = append(rows, row{qualifiedName, modName, nf.Name, nf.Excluded, folderPath, len(nf.Parameters), activityCount, complexity, returnType})
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Excluded", "Folder", "Params", "Actions", "McCabe", "Returns"},
		Summary: fmt.Sprintf("(%d nanoflows)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.excluded, r.folderPath, r.params, r.activities, r.complexity, r.returnType})
	}
	return writeResult(ctx, result)
}

// countNanoflowActivities counts meaningful activities in a nanoflow.
func countNanoflowActivities(nf *microflows.Nanoflow) int {
	if nf.ObjectCollection == nil {
		return 0
	}
	count := 0
	for _, obj := range nf.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.StartEvent, *microflows.EndEvent, *microflows.ExclusiveMerge:
			// Skip structural elements
		default:
			count++
		}
	}
	return count
}

// calculateNanoflowComplexity calculates McCabe cyclomatic complexity for a nanoflow.
func calculateNanoflowComplexity(nf *microflows.Nanoflow) int {
	if nf.ObjectCollection == nil {
		return 1
	}
	// McCabe = E - N + 2P where E = edges, N = nodes, P = connected components (1 for a single flow)
	// Simplified: 1 + number of decision points (ExclusiveSplit, InheritanceSplit, LoopedActivity)
	complexity := 1
	for _, obj := range nf.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.ExclusiveSplit, *microflows.InheritanceSplit, *microflows.LoopedActivity:
			complexity++
		}
	}
	return complexity
}

// describeMicroflow handles DESCRIBE MICROFLOW command - outputs MDL source code.
func describeMicroflow(ctx *ExecContext, name ast.QualifiedName) error {
	// Get hierarchy for module/folder resolution
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Use pre-warmed cache if available (from PreWarmCache), otherwise build on demand
	entityNames := getEntityNames(ctx, h)
	microflowNames := getMicroflowNames(ctx, h)

	// Find the microflow
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	// Supplement microflow name lookup if not pre-warmed
	if len(microflowNames) == 0 {
		for _, mf := range allMicroflows {
			microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
		}
	}

	// Describe the live microflow: a module may hold an excluded twin of this
	// name, and describing that one shows a body the app does not run (#914).
	targetMf, _ := pickLive(allMicroflows,
		func(mf *microflows.Microflow) bool {
			return h.GetModuleName(h.FindModuleID(mf.ContainerID)) == name.Module && mf.Name == name.Name
		},
		func(mf *microflows.Microflow) bool { return mf.Excluded },
	)

	if targetMf == nil {
		return mdlerrors.NewNotFound("microflow", name.String())
	}

	// Generate MDL output
	var lines []string

	// Documentation
	if targetMf.Documentation != "" {
		lines = append(lines, "/**")
		for docLine := range strings.SplitSeq(targetMf.Documentation, "\n") {
			lines = append(lines, " * "+docLine)
		}
		lines = append(lines, " */")
	}

	// @excluded annotation
	if targetMf.Excluded {
		lines = append(lines, "@excluded")
	}

	// CREATE MICROFLOW header
	qualifiedName := name.Module + "." + name.Name
	if len(targetMf.Parameters) > 0 {
		lines = append(lines, fmt.Sprintf("create or modify microflow %s (", qualifiedName))
		lines = append(lines, describeMicroflowParameters(targetMf.Parameters,
			func(p *microflows.MicroflowParameter) string {
				return formatMicroflowDataType(ctx, p.Type, entityNames)
			})...)
		lines = append(lines, ")")
	} else {
		lines = append(lines, fmt.Sprintf("create or modify microflow %s ()", qualifiedName))
	}

	// Return type
	if targetMf.ReturnType != nil {
		returnType := formatMicroflowDataType(ctx, targetMf.ReturnType, entityNames)
		if returnType != "Void" && returnType != "" {
			returnLine := fmt.Sprintf("returns %s", returnType)
			// Add variable name if specified (AS $VarName)
			if targetMf.ReturnVariableName != "" && targetMf.ReturnVariableName != "Variable" {
				returnLine += fmt.Sprintf(" as $%s", targetMf.ReturnVariableName)
			}
			lines = append(lines, returnLine)
		}
	}

	// Folder
	if folderPath := h.BuildFolderPath(targetMf.ContainerID); folderPath != "" {
		lines = append(lines, fmt.Sprintf("folder %s", mdlQuote(folderPath)))
	}

	lines = append(lines, exposeClauseLines(targetMf)...)

	// BEGIN block
	lines = append(lines, "begin")

	prevDescribingReturnValue := ctx.DescribingMicroflowHasReturnValue
	ctx.DescribingMicroflowHasReturnValue = microflowHasReturnValue(targetMf)
	defer func() {
		ctx.DescribingMicroflowHasReturnValue = prevDescribingReturnValue
	}()

	// Generate activities
	if targetMf.ObjectCollection != nil && len(targetMf.ObjectCollection.Objects) > 0 {
		activityLines := formatMicroflowActivities(ctx, targetMf, entityNames, microflowNames)
		activityLines = prependFreeAnnotationLines(targetMf.ObjectCollection, activityLines)
		for _, line := range activityLines {
			lines = append(lines, "  "+line)
		}
	} else {
		lines = append(lines, "  -- No activities")
	}

	lines = append(lines, "end;")

	// Add GRANT EXECUTE if roles are assigned
	if len(targetMf.AllowedModuleRoles) > 0 {
		roles := make([]string, len(targetMf.AllowedModuleRoles))
		for i, r := range targetMf.AllowedModuleRoles {
			roles[i] = string(r)
		}
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("grant execute on microflow %s.%s to %s;",
			name.Module, name.Name, strings.Join(roles, ", ")))
	}

	lines = append(lines, "/")

	// Output
	fmt.Fprintln(ctx.Output, strings.Join(lines, "\n"))
	return nil
}

// describeNanoflow generates re-executable CREATE OR MODIFY NANOFLOW MDL output
// with activities and control flows listed as comments.
func describeNanoflow(ctx *ExecContext, name ast.QualifiedName) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Build entity name lookup
	entityNames := make(map[model.ID]string)
	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	// Build microflow/nanoflow name lookup (used for call actions)
	microflowNames := make(map[model.ID]string)
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}
	for _, mf := range allMicroflows {
		microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
	}

	// Find the nanoflow
	allNanoflows, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("list nanoflows", err)
	}

	for _, nf := range allNanoflows {
		microflowNames[nf.ID] = h.GetQualifiedName(nf.ContainerID, nf.Name)
	}

	// Describe the live nanoflow, not an excluded twin of the same name (#914).
	targetNf, _ := pickLive(allNanoflows,
		func(nf *microflows.Nanoflow) bool {
			return h.GetModuleName(h.FindModuleID(nf.ContainerID)) == name.Module && nf.Name == name.Name
		},
		func(nf *microflows.Nanoflow) bool { return nf.Excluded },
	)

	if targetNf == nil {
		return mdlerrors.NewNotFound("nanoflow", name.String())
	}

	var lines []string

	// Documentation
	if targetNf.Documentation != "" {
		lines = append(lines, "/**")
		for docLine := range strings.SplitSeq(targetNf.Documentation, "\n") {
			lines = append(lines, " * "+docLine)
		}
		lines = append(lines, " */")
	}

	// @excluded annotation
	if targetNf.Excluded {
		lines = append(lines, "@excluded")
	}

	// CREATE NANOFLOW header
	qualifiedName := name.Module + "." + name.Name
	if len(targetNf.Parameters) > 0 {
		lines = append(lines, fmt.Sprintf("create or modify nanoflow %s (", qualifiedName))
		lines = append(lines, describeMicroflowParameters(targetNf.Parameters,
			func(p *microflows.MicroflowParameter) string {
				return formatMicroflowDataType(ctx, p.Type, entityNames)
			})...)
		lines = append(lines, ")")
	} else {
		lines = append(lines, fmt.Sprintf("create or modify nanoflow %s ()", qualifiedName))
	}

	// Return type
	if targetNf.ReturnType != nil {
		returnType := formatMicroflowDataType(ctx, targetNf.ReturnType, entityNames)
		if returnType != "Void" && returnType != "" {
			lines = append(lines, fmt.Sprintf("returns %s", returnType))
		}
	}

	// Folder
	if folderPath := h.BuildFolderPath(targetNf.ContainerID); folderPath != "" {
		lines = append(lines, fmt.Sprintf("folder %s", mdlQuote(folderPath)))
	}

	// BEGIN block with activities
	lines = append(lines, "begin")

	// Wrap nanoflow in a Microflow to reuse formatMicroflowActivities
	wrapperMf := &microflows.Microflow{
		ReturnType:       targetNf.ReturnType,
		ObjectCollection: targetNf.ObjectCollection,
	}
	prevDescribingReturnValue := ctx.DescribingMicroflowHasReturnValue
	ctx.DescribingMicroflowHasReturnValue = microflowHasReturnValue(wrapperMf)
	defer func() {
		ctx.DescribingMicroflowHasReturnValue = prevDescribingReturnValue
	}()

	if targetNf.ObjectCollection != nil && len(targetNf.ObjectCollection.Objects) > 0 {
		activityLines := formatMicroflowActivities(ctx, wrapperMf, entityNames, microflowNames)
		for _, line := range activityLines {
			lines = append(lines, "  "+line)
		}
	} else {
		lines = append(lines, "  -- No activities")
	}

	lines = append(lines, "end;")
	lines = append(lines, "/")

	fmt.Fprintln(ctx.Output, strings.Join(lines, "\n"))
	return nil
}

// describeMicroflowToString generates MDL source for a microflow and returns it as a string
// along with a source map mapping node IDs to line ranges.
func describeMicroflowToString(ctx *ExecContext, name ast.QualifiedName) (string, map[string]elkSourceRange, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return "", nil, mdlerrors.NewBackend("build hierarchy", err)
	}

	entityNames := make(map[model.ID]string)
	domainModels, _ := ctx.Backend.ListDomainModels()
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	microflowNames := make(map[model.ID]string)
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return "", nil, mdlerrors.NewBackend("list microflows", err)
	}
	for _, mf := range allMicroflows {
		microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
	}

	// Describe the live microflow: a module may hold an excluded twin of this
	// name, and describing that one shows a body the app does not run (#914).
	targetMf, _ := pickLive(allMicroflows,
		func(mf *microflows.Microflow) bool {
			return h.GetModuleName(h.FindModuleID(mf.ContainerID)) == name.Module && mf.Name == name.Name
		},
		func(mf *microflows.Microflow) bool { return mf.Excluded },
	)

	if targetMf == nil {
		return "", nil, mdlerrors.NewNotFound("microflow", name.String())
	}

	sourceMap := make(map[string]elkSourceRange)
	mdl := renderMicroflowMDL(ctx, "microflow", targetMf, name, entityNames, microflowNames, sourceMap)
	return mdl, sourceMap, nil
}

// describeNanoflowToString generates MDL source for a nanoflow and returns it as a string
// along with a source map mapping node IDs to line ranges.
func describeNanoflowToString(ctx *ExecContext, name ast.QualifiedName) (string, map[string]elkSourceRange, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return "", nil, mdlerrors.NewBackend("build hierarchy", err)
	}

	entityNames := make(map[model.ID]string)
	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return "", nil, mdlerrors.NewBackend("list domain models", err)
	}
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	microflowNames := make(map[model.ID]string)
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return "", nil, mdlerrors.NewBackend("list microflows", err)
	}
	for _, mf := range allMicroflows {
		microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
	}

	allNanoflows, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return "", nil, mdlerrors.NewBackend("list nanoflows", err)
	}
	for _, nf := range allNanoflows {
		microflowNames[nf.ID] = h.GetQualifiedName(nf.ContainerID, nf.Name)
	}

	// Describe the live nanoflow, not an excluded twin of the same name (#914).
	targetNf, _ := pickLive(allNanoflows,
		func(nf *microflows.Nanoflow) bool {
			return h.GetModuleName(h.FindModuleID(nf.ContainerID)) == name.Module && nf.Name == name.Name
		},
		func(nf *microflows.Nanoflow) bool { return nf.Excluded },
	)

	if targetNf == nil {
		return "", nil, mdlerrors.NewNotFound("nanoflow", name.String())
	}

	// Wrap nanoflow as a Microflow so renderMicroflowMDL can handle it
	wrapperMf := &microflows.Microflow{
		Documentation:      targetNf.Documentation,
		Excluded:           targetNf.Excluded,
		Parameters:         targetNf.Parameters,
		ReturnType:         targetNf.ReturnType,
		ObjectCollection:   targetNf.ObjectCollection,
		AllowedModuleRoles: targetNf.AllowedModuleRoles,
	}

	sourceMap := make(map[string]elkSourceRange)
	mdl := renderMicroflowMDL(ctx, "nanoflow", wrapperMf, name, entityNames, microflowNames, sourceMap)
	return mdl, sourceMap, nil
}

// renderMicroflowMDL formats a parsed Microflow as MDL text.
//
// Shared by DESCRIBE MICROFLOW and `diff-local`, so both paths produce the
// same output. entityNames/microflowNames provide ID → qualified-name
// resolution; pass empty maps if unavailable (types will fall back to
// "Object"/"List" stubs). If sourceMap is non-nil it will be populated with
// ELK node IDs → line ranges for visualization; pass nil when not needed.
func renderMicroflowMDL(
	ctx *ExecContext,
	flowType string,
	mf *microflows.Microflow,
	name ast.QualifiedName,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	sourceMap map[string]elkSourceRange,
) string {
	prevDescribingReturnValue := ctx.DescribingMicroflowHasReturnValue
	ctx.DescribingMicroflowHasReturnValue = microflowHasReturnValue(mf)
	defer func() {
		ctx.DescribingMicroflowHasReturnValue = prevDescribingReturnValue
	}()

	var lines []string

	if mf.Documentation != "" {
		lines = append(lines, "/**")
		for docLine := range strings.SplitSeq(mf.Documentation, "\n") {
			lines = append(lines, " * "+docLine)
		}
		lines = append(lines, " */")
	}

	if mf.Excluded {
		lines = append(lines, "@excluded")
	}

	qualifiedName := name.Module + "." + name.Name
	if len(mf.Parameters) > 0 {
		lines = append(lines, fmt.Sprintf("create or modify %s %s (", flowType, qualifiedName))
		lines = append(lines, describeMicroflowParameters(mf.Parameters,
			func(p *microflows.MicroflowParameter) string {
				return formatMicroflowDataType(ctx, p.Type, entityNames)
			})...)
		lines = append(lines, ")")
	} else {
		lines = append(lines, fmt.Sprintf("create or modify %s %s ()", flowType, qualifiedName))
	}

	if mf.ReturnType != nil {
		returnType := formatMicroflowDataType(ctx, mf.ReturnType, entityNames)
		if returnType != "Void" && returnType != "" {
			returnLine := fmt.Sprintf("returns %s", returnType)
			if mf.ReturnVariableName != "" && mf.ReturnVariableName != "Variable" {
				returnLine += fmt.Sprintf(" as $%s", mf.ReturnVariableName)
			}
			lines = append(lines, returnLine)
		}
	}

	lines = append(lines, exposeClauseLines(mf)...)

	lines = append(lines, "begin")
	headerLineCount := len(lines)

	if mf.ObjectCollection != nil && len(mf.ObjectCollection.Objects) > 0 {
		var activityLines []string
		if sourceMap != nil {
			activityLines = formatMicroflowActivitiesWithSourceMap(ctx, mf, entityNames, microflowNames, sourceMap, headerLineCount)
		} else {
			activityLines = formatMicroflowActivities(ctx, mf, entityNames, microflowNames)
		}
		activityLines = prependFreeAnnotationLines(mf.ObjectCollection, activityLines)
		for _, line := range activityLines {
			lines = append(lines, "  "+line)
		}
	} else {
		lines = append(lines, "  -- No activities")
	}

	lines = append(lines, "end;")

	if len(mf.AllowedModuleRoles) > 0 {
		roles := make([]string, len(mf.AllowedModuleRoles))
		for i, r := range mf.AllowedModuleRoles {
			roles[i] = string(r)
		}
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("grant execute on %s %s.%s to %s;",
			flowType, name.Module, name.Name, strings.Join(roles, ", ")))
	}

	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

func microflowHasReturnValue(mf *microflows.Microflow) bool {
	if mf == nil || mf.ReturnType == nil {
		return false
	}
	_, isVoid := mf.ReturnType.(*microflows.VoidType)
	return !isVoid
}

// formatMicroflowDataType formats a microflow data type for MDL output.
func formatMicroflowDataType(ctx *ExecContext, dt microflows.DataType, entityNames map[model.ID]string) string {
	if dt == nil {
		return "Unknown"
	}

	switch t := dt.(type) {
	case *microflows.BooleanType:
		return "Boolean"
	case *microflows.IntegerType:
		return "Integer"
	case *microflows.LongType:
		return "Long"
	case *microflows.DecimalType:
		return "Decimal"
	case *microflows.StringType:
		return "String"
	case *microflows.DateTimeType:
		return "DateTime"
	case *microflows.DateType:
		return "Date"
	case *microflows.BinaryType:
		return "Binary"
	case *microflows.VoidType:
		return "Void"
	case *microflows.ObjectType:
		// First try EntityQualifiedName (BY_NAME_REFERENCE), then fall back to EntityID lookup
		if t.EntityQualifiedName != "" {
			return t.EntityQualifiedName
		}
		if name, ok := entityNames[t.EntityID]; ok {
			return name
		}
		return "Object"
	case *microflows.ListType:
		// First try EntityQualifiedName (BY_NAME_REFERENCE), then fall back to EntityID lookup
		if t.EntityQualifiedName != "" {
			return "List of " + t.EntityQualifiedName
		}
		if name, ok := entityNames[t.EntityID]; ok {
			return "List of " + name
		}
		return "List"
	case *microflows.EnumerationType:
		if t.EnumerationQualifiedName != "" {
			return "enum " + t.EnumerationQualifiedName
		}
		return "Enumeration"
	default:
		return dt.GetTypeName()
	}
}

// formatMicroflowActivities generates MDL statements for microflow activities.
func formatMicroflowActivities(
	ctx *ExecContext,
	mf *microflows.Microflow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) []string {
	if mf.ObjectCollection == nil {
		return []string{"-- debug: ObjectCollection is nil"}
	}

	// Build activity map by ID for flow traversal
	activityMap := make(map[model.ID]microflows.MicroflowObject)
	var startID model.ID

	for _, obj := range mf.ObjectCollection.Objects {
		activityMap[obj.GetID()] = obj
		if _, ok := obj.(*microflows.StartEvent); ok {
			startID = obj.GetID()
		}
	}

	// Build flow graph: map from origin ID to flows (sorted by OriginConnectionIndex).
	// Build the inverse destination→flows map for @anchor emission.
	flowsByOrigin := make(map[model.ID][]*microflows.SequenceFlow)
	flowsByDest := make(map[model.ID][]*microflows.SequenceFlow)
	for _, flow := range mf.ObjectCollection.Flows {
		flowsByOrigin[flow.OriginID] = append(flowsByOrigin[flow.OriginID], flow)
		flowsByDest[flow.DestinationID] = append(flowsByDest[flow.DestinationID], flow)
	}

	var lines []string
	lines = append(lines, duplicateOutputVariableWarnings(mf.ObjectCollection)...)
	lines = append(lines, irreducibleGraphWarnings(mf.ObjectCollection)...)

	// Sort flows by OriginConnectionIndex for each origin
	for originID := range flowsByOrigin {
		flows := flowsByOrigin[originID]
		// Simple bubble sort since typically only 2 flows per split
		for i := 0; i < len(flows)-1; i++ {
			for j := i + 1; j < len(flows); j++ {
				if flows[i].OriginConnectionIndex > flows[j].OriginConnectionIndex {
					flows[i], flows[j] = flows[j], flows[i]
				}
			}
		}
	}

	// Find the merge point for each split (where branches converge)
	splitMergeMap := findSplitMergePoints(ctx, mf.ObjectCollection, activityMap)

	// Traverse the flow graph recursively
	visited := make(map[model.ID]bool)

	// Build annotation map for @annotation emission
	annotationsByTarget := buildAnnotationsByTarget(mf.ObjectCollection)

	lines = append(lines, startAnnotationLines(mf.ObjectCollection)...)

	// flowsByOrigin / flowsByDest are threaded into traverseFlow so @anchor
	// emission is per-call — no package-level globals, safe under concurrent
	// describe (e.g. captureDescribeParallel).
	traverseFlow(ctx, startID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, &lines, 0, nil, 0, annotationsByTarget)

	return lines
}

// duplicateOutputVariableWarnings flags output-variable names that are assigned by
// two activities which can BOTH run on a single execution path — i.e. one activity
// reaches the other. Assignments in mutually-exclusive branches (then/else, enum
// cases) are legal and not flagged.
//
// This uses reachability, which is O(V*E). The previous implementation enumerated
// every execution path (cloning the visited set at each branch), which is O(2^b)
// in the number of branch points and made `describe microflow` time out on
// high-complexity flows — 20 sequential if/end-if diamonds already took ~10s
// (issue #710). Microflow control flow is a DAG (loops are nested inside
// LoopedActivity nodes, not back-edges), so reachability is exact and cheap.
func duplicateOutputVariableWarnings(oc *microflows.MicroflowObjectCollection) []string {
	warningPositions := make(map[string]model.Point)
	record := func(name string, pos model.Point) {
		if _, ok := warningPositions[name]; !ok {
			warningPositions[name] = pos
		}
	}

	type assignment struct {
		id  model.ID
		pos model.Point
	}

	var walk func(collection *microflows.MicroflowObjectCollection, inherited map[string]model.Point)
	walk = func(collection *microflows.MicroflowObjectCollection, inherited map[string]model.Point) {
		if collection == nil {
			return
		}
		flowsByOrigin := make(map[model.ID][]*microflows.SequenceFlow)
		for _, flow := range collection.Flows {
			flowsByOrigin[flow.OriginID] = append(flowsByOrigin[flow.OriginID], flow)
		}

		// reachableFrom(id) = node IDs reachable from id via normal flows (excluding
		// id). Memoized; the in-progress guard tolerates any stray cycle.
		cache := make(map[model.ID]map[model.ID]bool)
		inProgress := make(map[model.ID]bool)
		var reachableFrom func(id model.ID) map[model.ID]bool
		reachableFrom = func(id model.ID) map[model.ID]bool {
			if r, ok := cache[id]; ok {
				return r
			}
			if inProgress[id] {
				return nil
			}
			inProgress[id] = true
			r := make(map[model.ID]bool)
			for _, flow := range findNormalFlows(flowsByOrigin[id]) {
				if flow.DestinationID == "" {
					continue
				}
				r[flow.DestinationID] = true
				for k := range reachableFrom(flow.DestinationID) {
					r[k] = true
				}
			}
			delete(inProgress, id)
			cache[id] = r
			return r
		}

		assignments := make(map[string][]assignment)
		var loops []*microflows.LoopedActivity
		for _, obj := range collection.Objects {
			switch o := obj.(type) {
			case *microflows.ActionActivity:
				if name := actionOutputVariableName(o.Action); name != "" {
					assignments[name] = append(assignments[name], assignment{id: o.GetID(), pos: o.GetPosition()})
				}
			case *microflows.LoopedActivity:
				loops = append(loops, o)
			}
		}

		for name, list := range assignments {
			// An outer assignment on the entry path collides with any assignment here.
			if pos, ok := inherited[name]; ok {
				record(name, pos)
				continue
			}
			// Within this collection: a duplicate iff one assignment reaches another.
			for i := range list {
				reach := reachableFrom(list[i].id)
				for j := range list {
					if i != j && reach[list[j].id] {
						record(name, list[i].pos)
					}
				}
			}
		}

		// Recurse into loop bodies. Names visible on entry to a loop body are the
		// inherited names plus names assigned in this collection by an activity that
		// reaches the loop node.
		for _, loop := range loops {
			childInherited := make(map[string]model.Point, len(inherited))
			for n, p := range inherited {
				childInherited[n] = p
			}
			for name, list := range assignments {
				if _, ok := childInherited[name]; ok {
					continue
				}
				for _, a := range list {
					if a.id != loop.GetID() && reachableFrom(a.id)[loop.GetID()] {
						childInherited[name] = a.pos
						break
					}
				}
			}
			walk(loop.ObjectCollection, childInherited)
		}
	}
	walk(oc, nil)

	var names []string
	for name := range warningPositions {
		names = append(names, name)
	}
	sort.Strings(names)

	warnings := make([]string, 0, len(names))
	for _, name := range names {
		pos := warningPositions[name]
		warnings = append(warnings, fmt.Sprintf("-- WARNING: duplicate output variable $%s at position (%d, %d) - model is invalid; open in Studio Pro to fix", name, pos.X, pos.Y))
	}
	return warnings
}

func actionOutputVariableName(action any) string {
	switch a := action.(type) {
	case *microflows.CreateObjectAction:
		return a.OutputVariable
	case *microflows.RetrieveAction:
		return a.OutputVariable
	case *microflows.JavaActionCallAction:
		if a.UseReturnVariable {
			return a.ResultVariableName
		}
	case *microflows.MicroflowCallAction:
		if a.UseReturnVariable {
			return a.ResultVariableName
		}
	case *microflows.NanoflowCallAction:
		if a.UseReturnVariable {
			return a.OutputVariableName
		}
	case *microflows.JavaScriptActionCallAction:
		if a.UseReturnVariable {
			return a.OutputVariableName
		}
	case *microflows.AggregateListAction:
		return a.OutputVariable
	case *microflows.ListOperationAction:
		return a.OutputVariable
	case *microflows.RestCallAction:
		return a.OutputVariable
	case *microflows.ImportMappingCallAction:
		return a.OutputVariable
	case *microflows.ExportMappingCallAction:
		return a.OutputVariable
	case *microflows.CallExternalAction:
		if a.UseReturnVariable {
			return a.ResultVariableName
		}
	}
	return ""
}

// formatMicroflowActivitiesWithSourceMap generates MDL statements and populates a source map
// mapping ELK node IDs ("node-<objectID>") to line ranges (0-indexed) in the full MDL output.
// headerLineCount is the number of lines before the BEGIN body (to compute absolute line numbers).
func formatMicroflowActivitiesWithSourceMap(
	ctx *ExecContext,
	mf *microflows.Microflow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
) []string {
	if mf.ObjectCollection == nil {
		return []string{"-- debug: ObjectCollection is nil"}
	}

	activityMap := make(map[model.ID]microflows.MicroflowObject)
	var startID model.ID

	for _, obj := range mf.ObjectCollection.Objects {
		activityMap[obj.GetID()] = obj
		if _, ok := obj.(*microflows.StartEvent); ok {
			startID = obj.GetID()
		}
	}

	flowsByOrigin := make(map[model.ID][]*microflows.SequenceFlow)
	flowsByDest := make(map[model.ID][]*microflows.SequenceFlow)
	for _, flow := range mf.ObjectCollection.Flows {
		flowsByOrigin[flow.OriginID] = append(flowsByOrigin[flow.OriginID], flow)
		flowsByDest[flow.DestinationID] = append(flowsByDest[flow.DestinationID], flow)
	}

	var lines []string
	lines = append(lines, duplicateOutputVariableWarnings(mf.ObjectCollection)...)
	lines = append(lines, irreducibleGraphWarnings(mf.ObjectCollection)...)

	for originID := range flowsByOrigin {
		flows := flowsByOrigin[originID]
		for i := 0; i < len(flows)-1; i++ {
			for j := i + 1; j < len(flows); j++ {
				if flows[i].OriginConnectionIndex > flows[j].OriginConnectionIndex {
					flows[i], flows[j] = flows[j], flows[i]
				}
			}
		}
	}

	splitMergeMap := findSplitMergePoints(ctx, mf.ObjectCollection, activityMap)
	visited := make(map[model.ID]bool)

	// Build annotation map for @annotation emission
	annotationsByTarget := buildAnnotationsByTarget(mf.ObjectCollection)

	lines = append(lines, startAnnotationLines(mf.ObjectCollection)...)

	traverseFlow(ctx, startID, activityMap, flowsByOrigin, flowsByDest, splitMergeMap, visited, entityNames, microflowNames, &lines, 0, sourceMap, headerLineCount, annotationsByTarget)

	return lines
}

// findSplitMergePoints finds the corresponding merge point for each exclusive split.
func findSplitMergePoints(
	ctx *ExecContext,
	oc *microflows.MicroflowObjectCollection,
	activityMap map[model.ID]microflows.MicroflowObject,
) map[model.ID]model.ID {
	// Build flow graph for forward traversal
	flowsByOrigin := make(map[model.ID][]*microflows.SequenceFlow)
	for _, flow := range oc.Flows {
		flowsByOrigin[flow.OriginID] = append(flowsByOrigin[flow.OriginID], flow)
	}

	return findSplitMergePointsForGraph(ctx, activityMap, flowsByOrigin)
}

// findSplitMergePointsForGraph finds the corresponding merge point for each
// split in an already materialized flow graph. Nested traversals such as loop
// bodies use this because they do not have a top-level object collection.
func findSplitMergePointsForGraph(
	ctx *ExecContext,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
) map[model.ID]model.ID {
	result := make(map[model.ID]model.ID)
	for _, obj := range activityMap {
		switch obj.(type) {
		case *microflows.ExclusiveSplit, *microflows.InheritanceSplit:
			splitID := obj.GetID()
			// Find merge by following both branches until they converge.
			mergeID := findMergeForSplit(ctx, splitID, flowsByOrigin, activityMap)
			if mergeID != "" {
				result[splitID] = mergeID
			}
		}
	}

	return result
}

// findMergeForSplit finds the nearest node where branches from a split converge.
// Studio Pro models often converge directly on the next activity instead of an
// explicit ExclusiveMerge, so the join can be any executable microflow object.
func findMergeForSplit(
	ctx *ExecContext,
	splitID model.ID,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	activityMap map[model.ID]microflows.MicroflowObject,
) model.ID {
	flows := findNormalFlows(flowsByOrigin[splitID])
	if len(flows) < 2 {
		return ""
	}

	branchDistances := make([]map[model.ID]int, 0, len(flows))
	branchStarts := make([]model.ID, 0, len(flows))
	for _, flow := range flows {
		branchStarts = append(branchStarts, flow.DestinationID)
		branchDistances = append(branchDistances, collectReachableDistances(flow.DestinationID, flowsByOrigin))
	}

	return selectNearestCommonJoin(activityMap, flowsByOrigin, branchStarts, branchDistances)
}

// collectReachableDistances collects the shortest normal-flow distance from a
// branch start to every reachable node. Error handler flows are excluded because
// they do not participate in split/merge structural pairing.
func collectReachableDistances(
	startID model.ID,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
) map[model.ID]int {
	distances := map[model.ID]int{}
	type queueItem struct {
		id       model.ID
		distance int
	}
	queue := []queueItem{{id: startID}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if previous, ok := distances[item.id]; ok && previous <= item.distance {
			continue
		}
		distances[item.id] = item.distance

		for _, flow := range findNormalFlows(flowsByOrigin[item.id]) {
			queue = append(queue, queueItem{
				id:       flow.DestinationID,
				distance: item.distance + 1,
			})
		}
	}

	return distances
}

func selectNearestCommonJoin(
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	branchStarts []model.ID,
	branchDistances []map[model.ID]int,
) model.ID {
	if len(branchDistances) < 2 {
		return ""
	}

	type candidate struct {
		id          model.ID
		reachCount  int
		maxDistance int
		sumDistance int
	}
	candidates := []candidate{}

	for nodeID, firstDistance := range branchDistances[0] {
		if !isSplitJoinCandidate(activityMap[nodeID]) {
			continue
		}

		maxDistance := firstDistance
		sumDistance := firstDistance
		common := true
		for _, distances := range branchDistances[1:] {
			distance, ok := distances[nodeID]
			if !ok {
				common = false
				break
			}
			if distance > maxDistance {
				maxDistance = distance
			}
			sumDistance += distance
		}
		if common {
			candidates = append(candidates, candidate{
				id:          nodeID,
				reachCount:  len(branchDistances),
				maxDistance: maxDistance,
				sumDistance: sumDistance,
			})
		}
	}

	if len(candidates) == 0 {
		byNode := map[model.ID]candidate{}
		for _, distances := range branchDistances {
			for nodeID, distance := range distances {
				if !isSplitJoinCandidate(activityMap[nodeID]) {
					continue
				}
				c := byNode[nodeID]
				c.id = nodeID
				c.reachCount++
				if distance > c.maxDistance {
					c.maxDistance = distance
				}
				c.sumDistance += distance
				byNode[nodeID] = c
			}
		}
		for _, c := range byNode {
			if c.reachCount >= 2 {
				candidates = append(candidates, c)
			}
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	filtered := candidates[:0]
	for _, candidate := range candidates {
		if splitJoinCandidateDoesNotHaveDownstreamBypass(candidate.id, activityMap, flowsByOrigin, branchStarts) {
			filtered = append(filtered, candidate)
		}
	}
	candidates = filtered
	if len(candidates) == 0 {
		return ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].reachCount != candidates[j].reachCount {
			return candidates[i].reachCount > candidates[j].reachCount
		}
		if candidates[i].maxDistance != candidates[j].maxDistance {
			return candidates[i].maxDistance < candidates[j].maxDistance
		}
		if candidates[i].sumDistance != candidates[j].sumDistance {
			return candidates[i].sumDistance < candidates[j].sumDistance
		}
		return string(candidates[i].id) < string(candidates[j].id)
	})

	return candidates[0].id
}

func splitJoinCandidateDoesNotHaveDownstreamBypass(
	candidateID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	branchStarts []model.ID,
) bool {
	downstream := collectReachableNonTerminalObjects(candidateID, activityMap, flowsByOrigin)
	if len(downstream) == 0 {
		return true
	}
	for _, startID := range branchStarts {
		if startID == candidateID {
			continue
		}
		if reachesAnyObjectAvoiding(startID, downstream, candidateID, activityMap, flowsByOrigin, map[model.ID]bool{}) {
			return false
		}
	}
	return true
}

func collectReachableNonTerminalObjects(
	startID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
) map[model.ID]bool {
	result := map[model.ID]bool{}
	var walk func(model.ID)
	visited := map[model.ID]bool{startID: true}
	walk = func(currentID model.ID) {
		if visited[currentID] {
			return
		}
		visited[currentID] = true
		if isNonTerminalMicroflowObject(activityMap[currentID]) {
			result[currentID] = true
		}
		for _, flow := range findNormalFlows(flowsByOrigin[currentID]) {
			walk(flow.DestinationID)
		}
	}
	for _, flow := range findNormalFlows(flowsByOrigin[startID]) {
		walk(flow.DestinationID)
	}
	return result
}

func reachesAnyObjectAvoiding(
	currentID model.ID,
	targets map[model.ID]bool,
	avoidID model.ID,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	visited map[model.ID]bool,
) bool {
	if currentID == "" || currentID == avoidID || visited[currentID] {
		return false
	}
	if targets[currentID] {
		return true
	}
	if !isNonTerminalMicroflowObject(activityMap[currentID]) {
		return false
	}
	visited[currentID] = true
	for _, flow := range findNormalFlows(flowsByOrigin[currentID]) {
		if reachesAnyObjectAvoiding(flow.DestinationID, targets, avoidID, activityMap, flowsByOrigin, visited) {
			return true
		}
	}
	return false
}

func isNonTerminalMicroflowObject(obj microflows.MicroflowObject) bool {
	switch obj.(type) {
	case nil, *microflows.StartEvent, *microflows.EndEvent, *microflows.ErrorEvent:
		return false
	default:
		return true
	}
}

func isSplitJoinCandidate(obj microflows.MicroflowObject) bool {
	switch obj.(type) {
	case nil, *microflows.StartEvent, *microflows.EndEvent:
		return false
	default:
		return true
	}
}

// --- Executor method wrappers for callers in unmigrated code ---

// irreducibleGraphWarnings flags a microflow whose branch structure this
// describer cannot render faithfully.
//
// MDL's IF/THEN/ELSE is a single-entry/single-exit block; a Mendix microflow is
// an arbitrary graph. When a branch re-enters a sibling branch's path there is no
// nesting that means the same thing, and the traversal below emits one anyway —
// on the graph in mendixlabs/mxcli#923 the description was the exact inverse of
// the original (it always logged; the description never did).
//
// Until the label form lands (see PROPOSAL_structured_microflow_description.md)
// the honest thing is to say so in the output rather than hand back MDL that
// silently means something else. It follows the `-- WARNING:` convention
// duplicateOutputVariableWarnings established, so it survives copy/paste of the
// description as a comment.
func irreducibleGraphWarnings(oc *microflows.MicroflowObjectCollection) []string {
	if oc == nil {
		return nil
	}
	var out []string
	for _, f := range microflowgraph.Analyze(oc.Objects, oc.Flows) {
		pos := ""
		if f.Split != nil {
			p := f.Split.GetPosition()
			pos = fmt.Sprintf(" at (%d, %d)", p.X, p.Y)
		}
		detail := "the branches rejoin early, so a path that enters the shared part first is not represented"
		if f.Class == microflowgraph.Interleaved {
			detail = fmt.Sprintf("the branches cross at %d separate points", len(f.Entries))
		}
		out = append(out, fmt.Sprintf(
			"-- WARNING: the decision%s has %d branches that do not nest - %s. "+
				"This description is NOT equivalent to the microflow and must not be re-executed over it; "+
				"edit it in Studio Pro instead (mxcli #923, %s)",
			pos, f.BranchCount, detail, f.Class))
	}
	return out
}

// listRules renders SHOW / LIST RULES. A rule is its own doctype, so it has its
// own listing: SHOW MICROFLOWS lists microflows only, as it already does for
// nanoflows and workflows.
//
// The columns mirror the nanoflow listing minus "Excluded"'s neighbours that a
// rule has no concept of — a rule stores no AllowedModuleRoles, so there is
// nothing to grant and nothing to show.
func listRules(ctx *ExecContext, moduleName string) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	if moduleName != "" {
		if _, err := findModule(ctx, moduleName); err != nil {
			return err
		}
	}

	rules, err := ctx.Backend.ListRules()
	if err != nil {
		return mdlerrors.NewBackend("list rules", err)
	}

	type row struct {
		qualifiedName string
		module        string
		name          string
		excluded      bool
		folderPath    string
		params        int
		activities    int
		complexity    int
		returnType    string
	}
	var rows []row

	for _, rule := range rules {
		modID := h.FindModuleID(rule.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}
		returnType := ""
		if rule.ReturnType != nil {
			returnType = rule.ReturnType.GetTypeName()
		}
		rows = append(rows, row{
			qualifiedName: modName + "." + rule.Name,
			module:        modName,
			name:          rule.Name,
			excluded:      rule.Excluded,
			folderPath:    h.BuildFolderPath(rule.ContainerID),
			params:        len(rule.Parameters),
			activities:    countRuleActivities(rule),
			complexity:    calculateRuleComplexity(rule),
			returnType:    returnType,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Excluded", "Folder", "Params", "Actions", "McCabe", "Returns"},
		Summary: fmt.Sprintf("(%d rules)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.module, r.name, r.excluded, r.folderPath, r.params, r.activities, r.complexity, r.returnType})
	}
	return writeResult(ctx, result)
}

// countRuleActivities counts meaningful activities in a rule.
func countRuleActivities(rule *microflows.Rule) int {
	if rule.ObjectCollection == nil {
		return 0
	}
	count := 0
	for _, obj := range rule.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.StartEvent, *microflows.EndEvent, *microflows.ExclusiveMerge:
			// Structural, not activities.
		default:
			count++
		}
	}
	return count
}

// calculateRuleComplexity calculates McCabe cyclomatic complexity for a rule.
func calculateRuleComplexity(rule *microflows.Rule) int {
	if rule.ObjectCollection == nil {
		return 1
	}
	complexity := 1
	for _, obj := range rule.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.ExclusiveSplit, *microflows.InheritanceSplit, *microflows.LoopedActivity:
			complexity++
		}
	}
	return complexity
}

// describeRule renders DESCRIBE RULE as re-executable MDL. It mirrors
// describeNanoflow: a rule shares a microflow's body, so the body is rendered by
// wrapping it in a Microflow and reusing formatMicroflowActivities.
//
// Two rule-specific omissions, both because the document has no such property:
// no `grant execute` line (a rule stores no AllowedModuleRoles) and no
// concurrency or URL options.
func describeRule(ctx *ExecContext, name ast.QualifiedName) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	entityNames := make(map[model.ID]string)
	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	// A rule's body can call microflows, so the call-target lookup is the same
	// one the microflow describer builds.
	microflowNames := make(map[model.ID]string)
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}
	for _, mf := range allMicroflows {
		microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
	}

	allRules, err := ctx.Backend.ListRules()
	if err != nil {
		return mdlerrors.NewBackend("list rules", err)
	}
	for _, r := range allRules {
		microflowNames[r.ID] = h.GetQualifiedName(r.ContainerID, r.Name)
	}

	// Describe the live rule, not an excluded twin of the same name (#914).
	target, _ := pickLive(allRules,
		func(r *microflows.Rule) bool {
			return h.GetModuleName(h.FindModuleID(r.ContainerID)) == name.Module && r.Name == name.Name
		},
		func(r *microflows.Rule) bool { return r.Excluded },
	)
	if target == nil {
		return mdlerrors.NewNotFound("rule", name.String())
	}

	var lines []string

	if target.Documentation != "" {
		lines = append(lines, "/**")
		for docLine := range strings.SplitSeq(target.Documentation, "\n") {
			lines = append(lines, " * "+docLine)
		}
		lines = append(lines, " */")
	}
	if target.Excluded {
		lines = append(lines, "@excluded")
	}

	qualifiedName := name.Module + "." + name.Name
	if len(target.Parameters) > 0 {
		lines = append(lines, fmt.Sprintf("create or modify rule %s (", qualifiedName))
		lines = append(lines, describeMicroflowParameters(target.Parameters,
			func(p *microflows.MicroflowParameter) string {
				return formatMicroflowDataType(ctx, p.Type, entityNames)
			})...)
		lines = append(lines, ")")
	} else {
		lines = append(lines, fmt.Sprintf("create or modify rule %s ()", qualifiedName))
	}

	// A rule always returns Boolean or an enumeration, so unlike a microflow the
	// return type is never legitimately absent — render whatever is stored and
	// let the validator complain about a rule that has none.
	if target.ReturnType != nil {
		returnType := formatMicroflowDataType(ctx, target.ReturnType, entityNames)
		if returnType != "Void" && returnType != "" {
			lines = append(lines, fmt.Sprintf("returns %s", returnType))
		}
	}

	if folderPath := h.BuildFolderPath(target.ContainerID); folderPath != "" {
		lines = append(lines, fmt.Sprintf("folder %s", mdlQuote(folderPath)))
	}

	lines = append(lines, "begin")

	wrapperMf := &microflows.Microflow{
		ReturnType:       target.ReturnType,
		ObjectCollection: target.ObjectCollection,
	}
	prevDescribingReturnValue := ctx.DescribingMicroflowHasReturnValue
	ctx.DescribingMicroflowHasReturnValue = microflowHasReturnValue(wrapperMf)
	defer func() {
		ctx.DescribingMicroflowHasReturnValue = prevDescribingReturnValue
	}()

	if target.ObjectCollection != nil && len(target.ObjectCollection.Objects) > 0 {
		for _, line := range formatMicroflowActivities(ctx, wrapperMf, entityNames, microflowNames) {
			lines = append(lines, "  "+line)
		}
	} else {
		lines = append(lines, "  -- No activities")
	}

	lines = append(lines, "end;")
	lines = append(lines, "/")

	fmt.Fprintln(ctx.Output, strings.Join(lines, "\n"))
	return nil
}

// exposeClauseLines renders a microflow's toolbox entries as EXPOSED AS clauses,
// so a describe → exec round trip keeps it in the toolbox it was dragged from.
//
// The icon and image bitmaps are not rendered — MDL cannot express them — and do
// not need to be: an unwritten clause preserves what is stored rather than
// clearing it.
//
// It is a shared helper because there are two microflow renderers in this file,
// describeMicroflow and renderMicroflowMDL, and patching only one of them is how
// this clause was invisible on the path the CLI actually takes.
func exposeClauseLines(mf *microflows.Microflow) []string {
	var out []string
	for _, e := range []struct {
		kind string
		info *javaactions.MicroflowActionInfo
	}{
		{"microflow", mf.MicroflowActionInfo},
		{"workflow", mf.WorkflowActionInfo},
	} {
		if e.info == nil || e.info.Caption == "" {
			continue
		}
		out = append(out, fmt.Sprintf("exposed as %s action '%s' in '%s'",
			e.kind, escapeMDLString(e.info.Caption), escapeMDLString(e.info.Category)))
		for _, note := range strings.Split(describeBitmapComments(e.info), "\n") {
			if note != "" {
				out = append(out, note)
			}
		}
	}
	return out
}
