// SPDX-License-Identifier: Apache-2.0

// Package executor - building a flow from a CREATE statement, separately from
// writing it.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// buildFlowOpts controls how far a build is allowed to go.
//
// AllowCreate is what separates `exec` from `diff`. exec sets it: the module
// and folders a script names are created on demand, a session-level DROP is
// consumed, and the guard-don't-drop refusals (a queued call, an unwritable
// REST body) run before anything is written. diff clears it: rendering a
// proposed flow must not touch the project, and a refusal that aborted the
// build would leave the user with no diff at all rather than a diff plus the
// warning exec will give them anyway.
type buildFlowOpts struct {
	AllowCreate bool
}

// builtFlow is a Microflow assembled from a statement, plus what the write
// phase needs to place it.
type builtFlow struct {
	Microflow *microflows.Microflow
	// ContainerID is where the flow should live: the resolved folder, or the
	// module when no folder was named.
	ContainerID model.ID
	// ExistingID and ExistingContainerID are empty for a flow that is not in
	// the project yet — which is how diff decides a statement is an addition.
	ExistingID          model.ID
	ExistingContainerID model.ID
}

// builtNanoflow is builtFlow for the distinct Nanoflow document type.
type builtNanoflow struct {
	Nanoflow            *microflows.Nanoflow
	ContainerID         model.ID
	ExistingID          model.ID
	ExistingContainerID model.ID
}

// buildMicroflowFromStmt assembles a Microflow from a CREATE MICROFLOW
// statement without writing it.
//
// It exists so that `diff` can render its script side through the SAME
// describer the project side goes through. Before this, diff kept a second
// AST-to-MDL renderer whose statement switch covered 18 of 43 activity types
// and had no default case, so every unhandled activity — every java-action
// call, every `download file`, every canvas annotation — silently rendered as
// nothing and appeared in the diff as a deletion. mxcli then reported that a
// script would gut a microflow that exec proved was a no-op (#997).
//
// The lesson is structural rather than a list of missing cases: two renderers
// for one language drift, and the drift surfaces as a confident false report.
// Adding the 25 missing cases would have fixed the symptom and left the
// mechanism in place.
func buildMicroflowFromStmt(ctx *ExecContext, s *ast.CreateMicroflowStmt, opts buildFlowOpts) (*builtFlow, error) {
	// Validate name is not empty
	if strings.TrimSpace(s.Name.Name) == "" {
		return nil, mdlerrors.NewValidation("microflow name must not be empty")
	}

	// Refuse the XPath constraints Mendix rejects, before writing anything.
	// `mxcli check` already reported these, but exec ran a different validator
	// and wrote them anyway, so a script that skipped check produced a project
	// the build fails on (issue #833). Same placement as the entity handler's
	// ValidateEntity call.
	if opts.AllowCreate {
		if err := validateMicroflowRules(s); err != nil {
			return nil, err
		}
	}

	// Find the module, and the folder, WITHOUT creating either on a dry run:
	// findOrCreateModule and resolveFolder both write, and `diff` must render a
	// proposed flow against an unmodified project.
	var module *model.Module
	if opts.AllowCreate {
		var err error
		module, err = findOrCreateModule(ctx, s.Name.Module)
		if err != nil {
			return nil, err
		}
	} else if m, err := findModule(ctx, s.Name.Module); err == nil {
		module = m
	}
	var moduleID model.ID
	if module != nil {
		moduleID = module.ID
	}

	containerID := moduleID
	if s.Folder != "" {
		if opts.AllowCreate {
			folderID, err := resolveFolder(ctx, moduleID, s.Folder)
			if err != nil {
				return nil, mdlerrors.NewBackend("resolve folder "+s.Folder, err)
			}
			containerID = folderID
		} else if folderID, ok := lookupFolder(ctx, moduleID, s.Folder); ok {
			containerID = folderID
		}
	}

	// Check if microflow with same name already exists in this module
	var existingID model.ID
	var existingContainerID model.ID
	var existingAllowedRoles []model.ID
	preserveAllowedRoles := false
	// Excluded is model state, not script state: an absent @excluded must not
	// clear a stored exclusion (#914).
	existingExcluded := false
	var existingActionInfo, existingWorkflowInfo *types.MicroflowActionInfo
	existingMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil, mdlerrors.NewBackend("check existing microflows", err)
	}
	// A module may hold several microflows with this name as long as all but one
	// are excluded, so target the live one rather than whichever comes first
	// (#914).
	if existing, ok := pickLive(existingMicroflows,
		func(m *microflows.Microflow) bool {
			return m.Name == s.Name.Name && getModuleID(ctx, m.ContainerID) == moduleID
		},
		func(m *microflows.Microflow) bool { return m.Excluded },
	); ok {
		if !s.CreateOrModify && opts.AllowCreate {
			return nil, mdlerrors.NewAlreadyExistsMsg("microflow", s.Name.Module+"."+s.Name.Name, "microflow '"+s.Name.Module+"."+s.Name.Name+"' already exists (use create or modify to overwrite)")
		}
		existingID = existing.ID
		existingContainerID = existing.ContainerID
		existingAllowedRoles = cloneRoleIDs(existing.AllowedModuleRoles)
		preserveAllowedRoles = true
		existingExcluded = existing.Excluded
		// The toolbox entries hold four PNG bitmaps MDL cannot name, so a
		// rewrite carries them rather than rebuilding from the clause.
		existingActionInfo = existing.MicroflowActionInfo
		existingWorkflowInfo = existing.WorkflowActionInfo
	}

	// For CREATE OR REPLACE/MODIFY, reuse the existing ID to preserve references
	qualifiedName := s.Name.Module + "." + s.Name.Name

	// Refuse before writing if the stored microflow has a call bound to a task
	// queue: the rebuild would null it out and nothing downstream would notice.
	if existingID != "" && opts.AllowCreate {
		if err := checkNoQueuedCalls(ctx, existingID, qualifiedName, s); err != nil {
			return nil, err
		}
		// Same reasoning for a REST body the writer cannot express: the rebuild
		// would drop it, DESCRIBE would not show it missing, and the app would
		// still build.
		if err := checkNoUnwritableRestBody(ctx, existingID, qualifiedName); err != nil {
			return nil, err
		}
	}
	microflowID := model.ID(types.GenerateID())
	if existingID != "" {
		microflowID = existingID
		// Keep the original folder unless a new folder is explicitly specified
		if s.Folder == "" {
			containerID = existingContainerID
		}
	} else if dropped := consumeDroppedMicroflow(ctx, qualifiedName); opts.AllowCreate && dropped != nil {
		// A prior DROP MICROFLOW in the same session removed the unit. Reuse
		// its original UnitID and (unless a new folder is specified)
		// ContainerID so that Studio Pro sees the rewrite as an in-place
		// update rather than a delete+insert pair, which produces
		// ".mpr does not look like a Mendix Studio Pro project file" errors.
		microflowID = dropped.ID
		if s.Folder == "" && dropped.ContainerID != "" {
			containerID = dropped.ContainerID
		}
		// consumeDroppedMicroflow removed the cache entry, so we own this
		// slice — no need to clone it again.
		existingAllowedRoles = dropped.AllowedRoles
		preserveAllowedRoles = true
	}

	// Build the microflow
	mf := &microflows.Microflow{
		BaseElement: model.BaseElement{
			ID: microflowID,
		},
		ContainerID:              containerID,
		Name:                     s.Name.Name,
		Documentation:            s.Documentation,
		AllowConcurrentExecution: true, // Default: allow concurrent execution
		MarkAsUsed:               false,
		Excluded:                 s.Excluded || existingExcluded,
	}
	if preserveAllowedRoles {
		mf.AllowedModuleRoles = existingAllowedRoles
	} else {
		if module != nil {
			mf.AllowedModuleRoles = defaultDocumentAccessRoles(ctx, module)
		}
	}
	if mf.MicroflowActionInfo, mf.WorkflowActionInfo, err = applyExposeClauses(ctx,
		s.Expose, existingActionInfo, existingWorkflowInfo, exposeWarner(ctx)); err != nil {
		return nil, err
	}

	// Build entity resolver function for parameter/return types
	entityResolver := func(qn ast.QualifiedName) model.ID {
		// Get all domain models and build module name map
		dms, err := ctx.Backend.ListDomainModels()
		if err != nil {
			return ""
		}
		modules, _ := ctx.Backend.ListModules()
		moduleNames := make(map[model.ID]string)
		for _, m := range modules {
			moduleNames[m.ID] = m.Name
		}
		// Search for entity in all domain models
		for _, dm := range dms {
			modName := moduleNames[dm.ContainerID]
			if modName != qn.Module {
				continue
			}
			for _, ent := range dm.Entities {
				if ent.Name == qn.Name {
					return ent.ID
				}
			}
		}
		return ""
	}

	// Validate and add parameters
	for i, p := range s.Parameters {
		// Validate entity references for List and Entity types.
		// Built-in modules (e.g. System) are not stored in the MPR domain models;
		// their types are serialized by qualified name and resolved at runtime.
		if p.Type.EntityRef != nil && !isBuiltinModuleEntity(p.Type.EntityRef.Module) {
			entityID := entityResolver(*p.Type.EntityRef)
			if entityID == "" {
				// Bare qualified name in microflow context is treated as TypeEntity by the
				// visitor, but it may actually be an enumeration. Try enum lookup before failing.
				if found := findEnumeration(ctx, p.Type.EntityRef.Module, p.Type.EntityRef.Name); found != nil {
					s.Parameters[i].Type = ast.DataType{Kind: ast.TypeEnumeration, EnumRef: p.Type.EntityRef}
					p = s.Parameters[i]
				} else {
					return nil, mdlerrors.NewNotFoundMsg("entity", p.Type.EntityRef.Module+"."+p.Type.EntityRef.Name,
						fmt.Sprintf("entity '%s.%s' not found for parameter '%s'", p.Type.EntityRef.Module, p.Type.EntityRef.Name, p.Name))
				}
			}
		}
		// Validate enumeration references for Enumeration types
		if p.Type.Kind == ast.TypeEnumeration && p.Type.EnumRef != nil {
			if found := findEnumeration(ctx, p.Type.EnumRef.Module, p.Type.EnumRef.Name); found == nil {
				return nil, mdlerrors.NewNotFoundMsg("enumeration", p.Type.EnumRef.Module+"."+p.Type.EnumRef.Name,
					fmt.Sprintf("enumeration '%s.%s' not found for parameter '%s'", p.Type.EnumRef.Module, p.Type.EnumRef.Name, p.Name))
			}
		}
		param := &microflows.MicroflowParameter{
			BaseElement: model.BaseElement{
				ID: model.ID(types.GenerateID()),
			},
			ContainerID: mf.ID,
			Name:        p.Name,
			Type:        convertASTToMicroflowDataType(p.Type, entityResolver),
			Position:    positionFromAST(p.Position),
		}
		mf.Parameters = append(mf.Parameters, param)
	}

	// Validate and set return type
	if s.ReturnType != nil {
		// Validate entity references for return type.
		// Built-in modules (e.g. System) are not stored in the MPR domain models;
		// their types are serialized by qualified name and resolved at runtime.
		if s.ReturnType.Type.EntityRef != nil && !isBuiltinModuleEntity(s.ReturnType.Type.EntityRef.Module) {
			entityID := entityResolver(*s.ReturnType.Type.EntityRef)
			if entityID == "" {
				return nil, mdlerrors.NewNotFoundMsg("entity", s.ReturnType.Type.EntityRef.Module+"."+s.ReturnType.Type.EntityRef.Name,
					fmt.Sprintf("entity '%s.%s' not found for return type", s.ReturnType.Type.EntityRef.Module, s.ReturnType.Type.EntityRef.Name))
			}
		}
		// Validate enumeration references for return type
		if s.ReturnType.Type.Kind == ast.TypeEnumeration && s.ReturnType.Type.EnumRef != nil {
			if found := findEnumeration(ctx, s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name); found == nil {
				return nil, mdlerrors.NewNotFoundMsg("enumeration", s.ReturnType.Type.EnumRef.Module+"."+s.ReturnType.Type.EnumRef.Name,
					fmt.Sprintf("enumeration '%s.%s' not found for return type", s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name))
			}
		}
		mf.ReturnType = convertASTToMicroflowDataType(s.ReturnType.Type, entityResolver)
		// Set return variable name if provided (AS $VarName)
		if s.ReturnType.Variable != "" {
			mf.ReturnVariableName = s.ReturnType.Variable
		}
	} else {
		mf.ReturnType = &microflows.VoidType{}
	}

	// Build flow graph from body statements
	// Initialize variable types from parameters
	varTypes := make(map[string]string)
	declaredVars := make(map[string]string)

	for _, p := range s.Parameters {
		if p.Type.EntityRef != nil {
			entityQN := p.Type.EntityRef.Module + "." + p.Type.EntityRef.Name
			if p.Type.Kind == ast.TypeListOf {
				// Store "List of Module.Entity" for list parameters
				varTypes[p.Name] = "List of " + entityQN
			} else {
				// Store "Module.Entity" for single entity parameters
				varTypes[p.Name] = entityQN
			}
		} else {
			// Primitive type parameters are also considered declared
			declaredVars[p.Name] = p.Type.Kind.String()
		}
	}
	// Get hierarchy for resolving page/microflow references
	hierarchy, _ := getHierarchy(ctx)

	restServices, _ := loadRestServices(ctx)

	builder := &flowBuilder{
		textLang: authoringLanguage(ctx),
		// Carry over a HAND-PLACED StartEvent position from the microflow being
		// replaced, the way the folder and allowed roles already are: a Studio
		// Pro flow's 145;200 became 100;200 on a describe→exec round-trip, the
		// only coordinate in it that did not survive (#884). A start sitting
		// where mxcli's own layout would have put it is not carried over — that
		// pinned the start of every rewritten flow, stranding it across the
		// canvas from activities the same script had just moved (#951). An
		// explicit @start(x, y) on the first statement overrides both.
		startPosition: storedStartPosition(ctx, existingID),
		posX:          200,
		posY:          200,
		baseY:         200, // Base Y for happy path
		spacing:       HorizontalSpacing,
		varTypes:      varTypes,
		declaredVars:  declaredVars,
		measurer:      &layoutMeasurer{varTypes: varTypes},
		backend:       ctx.Backend,
		hierarchy:     hierarchy,
		restServices:  restServices,
	}

	mf.ObjectCollection = builder.buildFlowGraph(s.Body, s.ReturnType)

	// Check for validation errors
	if errors := builder.GetErrors(); len(errors) > 0 {
		// Report all errors to the user
		var errMsg strings.Builder
		errMsg.WriteString(fmt.Sprintf("microflow '%s.%s' has validation errors:\n", s.Name.Module, s.Name.Name))
		for _, err := range errors {
			errMsg.WriteString(fmt.Sprintf("  - %s\n", err))
		}
		return nil, fmt.Errorf("%s", errMsg.String())
	}
	return &builtFlow{
		Microflow:           mf,
		ContainerID:         containerID,
		ExistingID:          existingID,
		ExistingContainerID: existingContainerID,
	}, nil
}

// buildNanoflowFromStmt is buildMicroflowFromStmt for nanoflows — same split,
// same reason (#997).
func buildNanoflowFromStmt(ctx *ExecContext, s *ast.CreateNanoflowStmt, opts buildFlowOpts) (*builtNanoflow, error) {
	// Validate name is not empty
	if strings.TrimSpace(s.Name.Name) == "" {
		return nil, mdlerrors.NewValidation("nanoflow name must not be empty")
	}

	if err := refuseExposeOnFlavour(s.Expose, "nanoflow", s.Name.Module+"."+s.Name.Name); err != nil {
		return nil, err
	}

	// Find the module, and the folder, WITHOUT creating either on a dry run:
	// findOrCreateModule and resolveFolder both write, and `diff` must render a
	// proposed flow against an unmodified project.
	var module *model.Module
	if opts.AllowCreate {
		var err error
		module, err = findOrCreateModule(ctx, s.Name.Module)
		if err != nil {
			return nil, err
		}
	} else if m, err := findModule(ctx, s.Name.Module); err == nil {
		module = m
	}
	var moduleID model.ID
	if module != nil {
		moduleID = module.ID
	}

	containerID := moduleID
	if s.Folder != "" {
		if opts.AllowCreate {
			folderID, err := resolveFolder(ctx, moduleID, s.Folder)
			if err != nil {
				return nil, mdlerrors.NewBackend("resolve folder "+s.Folder, err)
			}
			containerID = folderID
		} else if folderID, ok := lookupFolder(ctx, moduleID, s.Folder); ok {
			containerID = folderID
		}
	}

	// Check if nanoflow with same name already exists in this module.
	// NOTE: O(n) scan over all nanoflows — consistent with microflow handler pattern.
	// Consider catalog-based lookup if this becomes a bottleneck for large projects.
	var existingID model.ID
	var existingContainerID model.ID
	var existingAllowedRoles []model.ID
	preserveAllowedRoles := false
	// Excluded is model state, not script state: an absent @excluded must not
	// clear a stored exclusion (#914).
	existingExcluded := false
	existingNanoflows, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return nil, mdlerrors.NewBackend("check existing nanoflows", err)
	}
	// A module may hold several nanoflows with this name as long as all but one
	// are excluded, so target the live one rather than whichever comes first.
	if existing, ok := pickLive(existingNanoflows,
		func(n *microflows.Nanoflow) bool {
			return n.Name == s.Name.Name && getModuleID(ctx, n.ContainerID) == moduleID
		},
		func(n *microflows.Nanoflow) bool { return n.Excluded },
	); ok {
		if !s.CreateOrModify && opts.AllowCreate {
			return nil, mdlerrors.NewAlreadyExistsMsg("nanoflow", s.Name.Module+"."+s.Name.Name, "nanoflow '"+s.Name.Module+"."+s.Name.Name+"' already exists (use create or modify to overwrite)")
		}
		existingID = existing.ID
		existingContainerID = existing.ContainerID
		existingAllowedRoles = cloneRoleIDs(existing.AllowedModuleRoles)
		preserveAllowedRoles = true
		existingExcluded = existing.Excluded
	}

	// For CREATE OR REPLACE/MODIFY, reuse the existing ID to preserve references
	qualifiedName := s.Name.Module + "." + s.Name.Name
	nanoflowID := model.ID(types.GenerateID())
	if existingID != "" {
		nanoflowID = existingID
		if s.Folder == "" {
			containerID = existingContainerID
		}
	} else if dropped := consumeDroppedNanoflow(ctx, qualifiedName); opts.AllowCreate && dropped != nil {
		nanoflowID = dropped.ID
		if s.Folder == "" && dropped.ContainerID != "" {
			containerID = dropped.ContainerID
		}
		if len(dropped.AllowedRoles) > 0 {
			existingAllowedRoles = dropped.AllowedRoles
			preserveAllowedRoles = true
		}
	}

	// Build the nanoflow
	nf := &microflows.Nanoflow{
		BaseElement: model.BaseElement{
			ID: nanoflowID,
		},
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		MarkAsUsed:    false,
		Excluded:      s.Excluded || existingExcluded,
	}
	if preserveAllowedRoles {
		nf.AllowedModuleRoles = existingAllowedRoles
	} else {
		if module != nil {
			nf.AllowedModuleRoles = defaultDocumentAccessRoles(ctx, module)
		}
	}

	// Load metadata needed by the entity resolver up front so backend read
	// failures are returned as actionable errors instead of being treated as
	// "entity not found".
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return nil, mdlerrors.NewBackend("list domain models", err)
	}
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return nil, mdlerrors.NewBackend("list modules", err)
	}
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}

	// Build entity resolver function for parameter/return types
	entityResolver := func(qn ast.QualifiedName) model.ID {
		for _, dm := range dms {
			modName := moduleNames[dm.ContainerID]
			if modName != qn.Module {
				continue
			}
			for _, ent := range dm.Entities {
				if ent.Name == qn.Name {
					return ent.ID
				}
			}
		}
		return ""
	}

	// Validate and add parameters
	for i, p := range s.Parameters {
		if p.Type.EntityRef != nil && !isBuiltinModuleEntity(p.Type.EntityRef.Module) {
			entityID := entityResolver(*p.Type.EntityRef)
			if entityID == "" {
				// Bare qualified name in microflow context is treated as TypeEntity by the
				// visitor, but it may actually be an enumeration. Try enum lookup before failing.
				if found := findEnumeration(ctx, p.Type.EntityRef.Module, p.Type.EntityRef.Name); found != nil {
					s.Parameters[i].Type = ast.DataType{Kind: ast.TypeEnumeration, EnumRef: p.Type.EntityRef}
					p = s.Parameters[i]
				} else {
					return nil, mdlerrors.NewNotFoundMsg("entity", p.Type.EntityRef.Module+"."+p.Type.EntityRef.Name,
						fmt.Sprintf("entity '%s.%s' not found for parameter '%s'", p.Type.EntityRef.Module, p.Type.EntityRef.Name, p.Name))
				}
			}
		}
		if p.Type.Kind == ast.TypeEnumeration && p.Type.EnumRef != nil {
			if found := findEnumeration(ctx, p.Type.EnumRef.Module, p.Type.EnumRef.Name); found == nil {
				return nil, mdlerrors.NewNotFoundMsg("enumeration", p.Type.EnumRef.Module+"."+p.Type.EnumRef.Name,
					fmt.Sprintf("enumeration '%s.%s' not found for parameter '%s'", p.Type.EnumRef.Module, p.Type.EnumRef.Name, p.Name))
			}
		}
		param := &microflows.MicroflowParameter{
			BaseElement: model.BaseElement{
				ID: model.ID(types.GenerateID()),
			},
			ContainerID: nf.ID,
			Name:        p.Name,
			Type:        convertASTToMicroflowDataType(p.Type, entityResolver),
			Position:    positionFromAST(p.Position),
		}
		nf.Parameters = append(nf.Parameters, param)
	}

	// Validate and set return type
	if s.ReturnType != nil {
		if s.ReturnType.Type.EntityRef != nil && !isBuiltinModuleEntity(s.ReturnType.Type.EntityRef.Module) {
			entityID := entityResolver(*s.ReturnType.Type.EntityRef)
			if entityID == "" {
				return nil, mdlerrors.NewNotFoundMsg("entity", s.ReturnType.Type.EntityRef.Module+"."+s.ReturnType.Type.EntityRef.Name,
					fmt.Sprintf("entity '%s.%s' not found for return type", s.ReturnType.Type.EntityRef.Module, s.ReturnType.Type.EntityRef.Name))
			}
		}
		if s.ReturnType.Type.Kind == ast.TypeEnumeration && s.ReturnType.Type.EnumRef != nil {
			if found := findEnumeration(ctx, s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name); found == nil {
				return nil, mdlerrors.NewNotFoundMsg("enumeration", s.ReturnType.Type.EnumRef.Module+"."+s.ReturnType.Type.EnumRef.Name,
					fmt.Sprintf("enumeration '%s.%s' not found for return type", s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name))
			}
		}
		nf.ReturnType = convertASTToMicroflowDataType(s.ReturnType.Type, entityResolver)
	} else {
		nf.ReturnType = &microflows.VoidType{}
	}

	// Validate nanoflow-specific constraints before building the flow graph
	if errMsg := validateNanoflow(qualifiedName, s.Body, s.ReturnType); errMsg != "" {
		return nil, fmt.Errorf("%s", errMsg)
	}

	// SYNCHRONIZE UNSYNCHRONIZED is the one mode with a floor inside Mendix 9:
	// SynchronizationType.Unsynchronized was introduced in 9.4.0, while the
	// activity and its other two modes go back further. Gate the mode, not the
	// statement, so a 9.0-9.3 project keeps `synchronize all`.
	if bodyUsesUnsynchronized(s.Body) {
		if err := checkFeature(ctx, "microflows", "synchronize_unsynchronized",
			"synchronize unsynchronized",
			"use `synchronize all` or `synchronize $Var`, or upgrade the project to 9.4+"); err != nil {
			return nil, err
		}
	}

	// Build flow graph from body statements
	varTypes := make(map[string]string)
	declaredVars := make(map[string]string)

	for _, p := range s.Parameters {
		if p.Type.EntityRef != nil {
			entityQN := p.Type.EntityRef.Module + "." + p.Type.EntityRef.Name
			if p.Type.Kind == ast.TypeListOf {
				varTypes[p.Name] = "List of " + entityQN
			} else {
				varTypes[p.Name] = entityQN
			}
		} else {
			declaredVars[p.Name] = p.Type.Kind.String()
		}
	}

	hierarchy, _ := getHierarchy(ctx)        // best-effort: builder works without hierarchy
	restServices, _ := loadRestServices(ctx) // best-effort: builder works without REST services

	builder := &flowBuilder{
		textLang:     authoringLanguage(ctx),
		posX:         200,
		posY:         200,
		baseY:        200,
		spacing:      HorizontalSpacing,
		varTypes:     varTypes,
		declaredVars: declaredVars,
		measurer:     &layoutMeasurer{varTypes: varTypes},
		backend:      ctx.Backend,
		hierarchy:    hierarchy,
		restServices: restServices,
		isNanoflow:   true,
	}

	nf.ObjectCollection = builder.buildFlowGraph(s.Body, s.ReturnType)

	// Check for validation errors
	if errors := builder.GetErrors(); len(errors) > 0 {
		var errMsg strings.Builder
		errMsg.WriteString(fmt.Sprintf("nanoflow '%s.%s' has validation errors:\n", s.Name.Module, s.Name.Name))
		for _, err := range errors {
			errMsg.WriteString(fmt.Sprintf("  - %s\n", err))
		}
		return nil, fmt.Errorf("%s", errMsg.String())
	}
	return &builtNanoflow{
		Nanoflow:            nf,
		ContainerID:         containerID,
		ExistingID:          existingID,
		ExistingContainerID: existingContainerID,
	}, nil
}

// lookupFolder resolves a folder path to its ID without creating anything —
// the read-only counterpart of resolveFolder, which creates missing folders.
func lookupFolder(ctx *ExecContext, moduleID model.ID, folderPath string) (model.ID, bool) {
	if folderPath == "" {
		return moduleID, true
	}
	folders, err := ctx.Backend.ListFolders()
	if err != nil {
		return "", false
	}
	current := moduleID
	for _, part := range strings.Split(folderPath, "/") {
		if part == "" {
			continue
		}
		found := false
		for _, f := range folders {
			if f.ContainerID == current && f.Name == part {
				current = f.ID
				found = true
				break
			}
		}
		if !found {
			return "", false
		}
	}
	return current, true
}
