// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE MICROFLOW command
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// isBuiltinModuleEntity returns true for modules whose entities are defined
// internally by the Mendix runtime and are therefore not present in the MPR's
// domain models. These types are serialized using the qualified name reference
// ("System.Workflow", "System.User", etc.) and resolved at runtime.
func isBuiltinModuleEntity(moduleName string) bool {
	return moduleName == "System"
}

// execCreateMicroflow handles CREATE MICROFLOW statements.
// loadRestServices returns all consumed REST services, or nil if no backend.
func loadRestServices(ctx *ExecContext) ([]*model.ConsumedRestService, error) {
	if !ctx.Connected() {
		return nil, nil
	}
	svcs, err := ctx.Backend.ListConsumedRestServices()
	return svcs, err
}

func execCreateMicroflow(ctx *ExecContext, s *ast.CreateMicroflowStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	built, err := buildMicroflowFromStmt(ctx, s, buildFlowOpts{AllowCreate: true})
	if err != nil {
		return err
	}
	mf := built.Microflow
	containerID := built.ContainerID
	existingID := built.ExistingID
	existingContainerID := built.ExistingContainerID

	// Create or update the microflow
	if existingID != "" {
		if err := ctx.Backend.UpdateMicroflow(mf); err != nil {
			return mdlerrors.NewBackend("update microflow", err)
		}
		if _, err := applyDocumentFolder(ctx, mf.ID, existingContainerID, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Replaced", "microflow: %s.%s", s.Name.Module, s.Name.Name)
	} else {
		if err := ctx.Backend.CreateMicroflow(mf); err != nil {
			return mdlerrors.NewBackend("create microflow", err)
		}
		fmt.Fprintf(ctx.Output, "Created microflow: %s.%s\n", s.Name.Module, s.Name.Name)
	}

	// Track the created microflow so it can be resolved by subsequent page creations
	returnEntityName := extractEntityFromReturnType(mf.ReturnType)
	ctx.trackCreatedMicroflow(s.Name.Module, s.Name.Name, mf.ID, containerID, returnEntityName)

	// Invalidate hierarchy cache so the new microflow's container is visible
	invalidateHierarchy(ctx)
	return nil
}

// storedStartPosition reads the StartEvent position off the microflow being
// replaced, when a person put it there rather than mxcli's own layout — see
// authoredStartPosition for how the two are told apart, and why carrying the
// position over unconditionally pinned the start of every rewritten flow (#951).
//
// Nil for a fresh CREATE, and nil for a start sitting where the layout would
// have put it anyway; both derive from the new first activity. Best-effort: a
// backend that cannot read the flow yields the derived placement rather than
// failing the statement.
func storedStartPosition(ctx *ExecContext, existingID model.ID) *model.Point {
	if existingID == "" || ctx.Backend == nil {
		return nil
	}
	mf, err := ctx.Backend.GetMicroflow(existingID)
	if err != nil || mf == nil {
		return nil
	}
	return authoredStartPosition(mf.ObjectCollection)
}
