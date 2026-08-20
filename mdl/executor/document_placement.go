// SPDX-License-Identifier: Apache-2.0

// document_placement.go — applying a FOLDER clause to a document that already
// exists.
//
// A document's folder is its unit's container, which lives in the Unit row and
// not in the document's contents. Every Update* on both engines rewrites
// contents only, so a handler that resolved a folder, stored it on the model
// object and called Update had its placement silently dropped: the statement
// reported success, `resolveFolder` had already created the folder as a side
// effect, and the document stayed where it was. That was true of every doctype
// with a FOLDER clause, not of any one of them (#932).
//
// The fix is one call per CREATE OR MODIFY handler, not a per-doctype Move*
// method: placement is the same row update whatever the document is.
package executor

import (
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// resolveRequestedFolder resolves a statement's FOLDER clause to a container,
// returning "" when the statement did not name one.
//
// The empty return is the point. resolveFolder answers an empty path with the
// module root, which is the right answer for a CREATE and exactly the wrong one
// for a MODIFY: it would file every foldered document back into the module root
// the moment anyone re-ran a script that says nothing about folders. Pairing
// this with applyDocumentFolder — which does nothing for an empty target —
// makes "no folder clause" mean "leave it alone" at every call site, instead of
// leaving each handler to remember the distinction.
func resolveRequestedFolder(ctx *ExecContext, moduleID model.ID, folder string) (model.ID, error) {
	if folder == "" {
		return "", nil
	}
	id, err := resolveFolder(ctx, moduleID, folder)
	if err != nil {
		return "", mdlerrors.NewBackend("resolve folder "+folder, err)
	}
	return id, nil
}

// describeFolderClause renders a document's placement as the MDL clause that
// would reproduce it, or "" when the document sits at the module root.
//
// DESCRIBE output is advertised as re-executable, so a description that omits
// the folder does not round-trip: replaying it in a fresh project recreates the
// document unfiled. Returned with a leading space so a caller can append it to
// the name without deciding whether one is needed.
//
// Best-effort in the same way the rest of DESCRIBE is: a hierarchy that cannot
// resolve the container yields no clause rather than a wrong one.
func describeFolderClause(ctx *ExecContext, containerID model.ID) string {
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return ""
	}
	path := h.BuildFolderPath(containerID)
	if path == "" {
		return ""
	}
	return " folder '" + strings.ReplaceAll(path, "'", "''") + "'"
}

// containerForDocument picks the container a CREATE OR MODIFY should use, in
// the order that makes both a create and a modify behave sensibly: the folder
// the statement named, else where the document already sits, else the module
// root.
//
// The middle term is the one worth stating out loud. It means a statement that
// says nothing about folders is silent about placement rather than asserting
// the module root — which matters for the doctypes rewritten as delete+create,
// where the container really is re-applied on every statement.
func containerForDocument(ctx *ExecContext, moduleID model.ID, folder string, existing model.ID) (model.ID, error) {
	target, err := resolveRequestedFolder(ctx, moduleID, folder)
	if err != nil {
		return "", err
	}
	if target != "" {
		return target, nil
	}
	if existing != "" {
		return existing, nil
	}
	return moduleID, nil
}

// applyDocumentFolder moves an existing document to the container a CREATE OR
// MODIFY resolved for it, and reports whether anything moved.
//
// Called only when the placement actually differs. That is not just an
// optimisation: it keeps a statement that says nothing about folders from
// touching the containment row at all, so the common path stays exactly as it
// was and a backend that cannot move documents only ever hears about it when a
// user really asked for a folder.
func applyDocumentFolder(ctx *ExecContext, unitID, from, to model.ID) (bool, error) {
	if unitID == "" || to == "" || from == to {
		return false, nil
	}
	if err := ctx.Backend.MoveDocument(unitID, to); err != nil {
		return false, mdlerrors.NewBackend("apply folder", err)
	}
	// The hierarchy cache indexes documents by container, so it is stale the
	// moment one moves — a DESCRIBE later in the same script would otherwise
	// still render the old folder and look like the bug this fixes.
	invalidateHierarchy(ctx)
	return true, nil
}
