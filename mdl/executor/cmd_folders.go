// SPDX-License-Identifier: Apache-2.0

// Package executor - DROP/MOVE FOLDER commands
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// findFolderByPath walks a folder path under a module and returns the folder ID.
func findFolderByPath(ctx *ExecContext, moduleID model.ID, folderPath string, folders []*types.FolderInfo) (model.ID, error) {
	parts := strings.Split(folderPath, "/")
	currentContainerID := moduleID

	var targetFolderID model.ID
	for i, part := range parts {
		if part == "" {
			continue
		}

		var found bool
		for _, f := range folders {
			if f.ContainerID == currentContainerID && f.Name == part {
				currentContainerID = f.ID
				if i == len(parts)-1 {
					targetFolderID = f.ID
				}
				found = true
				break
			}
		}

		if !found {
			return "", mdlerrors.NewNotFound("folder", folderPath)
		}
	}

	if targetFolderID == "" {
		return "", mdlerrors.NewNotFound("folder", folderPath)
	}

	return targetFolderID, nil
}

// folderContentSummary reports what sits directly inside a folder.
//
// It reads UnitInfo, which is type-agnostic, rather than the per-kind lists
// LIST FOLDERS renders from (documentsByContainer). That distinction is the
// whole fix for #892: documentsByContainer names twelve document kinds and
// JSON structures, mappings, message definitions and others are not among
// them, so the folder holding Mendix's own FeedbackModule mappings rendered as
// `[0]` while holding four documents. A guard built on the same list would
// inherit the same blind spot and still wave the drop through.
//
// Counts are rendered in a stable order so the refusal message does not vary
// between runs.
func folderContentSummary(folderID model.ID, units []*types.UnitInfo) (int, string) {
	counts := map[string]int{}
	total := 0
	for _, u := range units {
		if u == nil || u.ContainerID != folderID {
			continue
		}
		total++
		counts[folderContentKind(u.Type)]++
	}
	if total == 0 {
		return 0, ""
	}

	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], plural(counts[k], k, k+"s")))
	}
	return total, strings.Join(parts, ", ")
}

// folderContentKind turns a storage type into something worth reading in an
// error message: "JsonStructures$JsonStructure" -> "JsonStructure".
func folderContentKind(storageType string) string {
	if storageType == folderStorageType {
		return "sub-folder"
	}
	if _, after, ok := strings.Cut(storageType, "$"); ok && after != "" {
		return after
	}
	if storageType == "" {
		return "document"
	}
	return storageType
}

// folderStorageType is the unit type of a folder itself; folders are units, so
// a sub-folder shows up in the same containment scan as a document.
const folderStorageType = "Projects$Folder"

// execDropFolder handles DROP FOLDER 'path' IN Module statements.
//
// The folder must be empty (no child documents or sub-folders). Before #892
// that contract existed only in this comment: nothing checked, and deleting a
// populated folder left every document inside it pointing at a container that
// no longer existed. They were not deleted — they were orphaned, losing their
// module qualification (`FeedbackModule.IMM_PostResponse` -> `.IMM_PostResponse`)
// so nothing could resolve them and mxbuild reported CE1613 "no longer exists".
func execDropFolder(ctx *ExecContext, s *ast.DropFolderStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnected()
	}

	module, err := findModule(ctx, s.Module)
	if err != nil {
		return mdlerrors.NewNotFound("module", s.Module)
	}

	folders, err := ctx.Backend.ListFolders()
	if err != nil {
		return mdlerrors.NewBackend("list folders", err)
	}

	folderID, err := findFolderByPath(ctx, module.ID, s.FolderPath, folders)
	if err != nil {
		return fmt.Errorf("%w in %s", err, s.Module)
	}

	// Fail closed. For a destructive operation "I could not check" must never
	// mean "go ahead" — that is exactly how #892 destroyed containment.
	units, err := ctx.Backend.ListUnits()
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("check whether folder '%s' is empty", s.FolderPath), err)
	}
	if n, summary := folderContentSummary(folderID, units); n > 0 {
		return mdlerrors.NewValidationf(
			"folder '%s' in %s is not empty: it holds %s. "+
				"Dropping it would leave those documents without a module, so nothing could resolve them "+
				"(mxbuild reports CE1613). Move or drop the contents first.",
			s.FolderPath, s.Module, summary)
	}

	if err := ctx.Backend.DeleteFolder(folderID); err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("delete folder '%s'", s.FolderPath), err)
	}

	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Dropped folder: '%s' in %s\n", s.FolderPath, s.Module)
	return nil
}

// execMoveFolder handles MOVE FOLDER Module.FolderName TO ... statements.
func execMoveFolder(ctx *ExecContext, s *ast.MoveFolderStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnected()
	}

	// Find the source module
	sourceModule, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewNotFound("source module", s.Name.Module)
	}

	// Find the source folder
	folders, err := ctx.Backend.ListFolders()
	if err != nil {
		return mdlerrors.NewBackend("list folders", err)
	}

	folderID, err := findFolderByPath(ctx, sourceModule.ID, s.Name.Name, folders)
	if err != nil {
		return fmt.Errorf("%w in %s", err, s.Name.Module)
	}

	// Determine target module
	var targetModule *model.Module
	if s.TargetModule != "" {
		targetModule, err = findModule(ctx, s.TargetModule)
		if err != nil {
			return mdlerrors.NewNotFound("target module", s.TargetModule)
		}
	} else {
		targetModule = sourceModule
	}

	// Resolve target container
	var targetContainerID model.ID
	if s.TargetFolder != "" {
		targetContainerID, err = resolveFolder(ctx, targetModule.ID, s.TargetFolder)
		if err != nil {
			return mdlerrors.NewBackend("resolve target folder", err)
		}
	} else {
		targetContainerID = targetModule.ID
	}

	// Move the folder
	if err := ctx.Backend.MoveFolder(folderID, targetContainerID); err != nil {
		return mdlerrors.NewBackend("move folder", err)
	}

	invalidateHierarchy(ctx)

	target := targetModule.Name
	if s.TargetFolder != "" {
		target += "/" + s.TargetFolder
	}
	fmt.Fprintf(ctx.Output, "Moved folder %s to %s\n", s.Name.String(), target)
	return nil
}
