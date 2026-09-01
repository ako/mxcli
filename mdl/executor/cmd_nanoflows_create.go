// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE NANOFLOW command
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// execCreateNanoflow handles CREATE NANOFLOW statements.
func execCreateNanoflow(ctx *ExecContext, s *ast.CreateNanoflowStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	built, err := buildNanoflowFromStmt(ctx, s, buildFlowOpts{AllowCreate: true})
	if err != nil {
		return err
	}
	nf := built.Nanoflow
	containerID := built.ContainerID
	existingID := built.ExistingID
	existingContainerID := built.ExistingContainerID

	// Create or update the nanoflow
	if existingID != "" {
		if err := ctx.Backend.UpdateNanoflow(nf); err != nil {
			return mdlerrors.NewBackend("update nanoflow", err)
		}
		if _, err := applyDocumentFolder(ctx, nf.ID, existingContainerID, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Replaced", "nanoflow: %s.%s", s.Name.Module, s.Name.Name)
	} else {
		if err := ctx.Backend.CreateNanoflow(nf); err != nil {
			return mdlerrors.NewBackend("create nanoflow", err)
		}
		fmt.Fprintf(ctx.Output, "Created nanoflow: %s.%s\n", s.Name.Module, s.Name.Name)
	}

	// Track the created nanoflow
	returnEntityName := extractEntityFromReturnType(nf.ReturnType)
	ctx.trackCreatedNanoflow(s.Name.Module, s.Name.Name, nf.ID, containerID, returnEntityName)

	invalidateHierarchy(ctx)
	return nil
}

// bodyUsesUnsynchronized reports whether any SYNCHRONIZE in the body selects the
// Unsynchronized mode. Recurses the same nesting validateNanoflowStatements does
// — branches, loops and error-handler bodies — so a gated mode cannot slip
// through by sitting inside an `if`.
func bodyUsesUnsynchronized(stmts []ast.MicroflowStatement) bool {
	for _, stmt := range stmts {
		if s, ok := stmt.(*ast.SynchronizeStmt); ok && s.SyncType == "Unsynchronized" {
			return true
		}
		switch s := stmt.(type) {
		case *ast.IfStmt:
			if bodyUsesUnsynchronized(s.ThenBody) || bodyUsesUnsynchronized(s.ElseBody) {
				return true
			}
		case *ast.LoopStmt:
			if bodyUsesUnsynchronized(s.Body) {
				return true
			}
		case *ast.WhileStmt:
			if bodyUsesUnsynchronized(s.Body) {
				return true
			}
		}
		if eh := getErrorHandling(stmt); eh != nil && bodyUsesUnsynchronized(eh.Body) {
			return true
		}
	}
	return false
}
