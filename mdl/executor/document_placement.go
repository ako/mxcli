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
