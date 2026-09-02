// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ============================================================================
// V3 Page Creation
// ============================================================================

// execCreatePageV3 handles CREATE PAGE statement with V3 syntax.
func execCreatePageV3(ctx *ExecContext, s *ast.CreatePageStmtV3) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Version pre-check: page parameters require 11.0+
	if len(s.Parameters) > 0 {
		if err := checkFeature(ctx, "pages", "page_parameters",
			"create page with parameters",
			"pass data via a non-persistent entity or microflow parameter instead"); err != nil {
			return err
		}
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("find module %s", s.Name.Module), err)
	}
	moduleID := module.ID

	// Check if page already exists - collect ALL duplicates
	existingPages, _ := ctx.Backend.ListPages()
	var pagesToDelete []model.ID
	var existingAllowedRoles []model.ID
	preserveAllowedRoles := false
	// Mendix allows several pages in one module to share a name as long as all
	// but one are excluded, so the replace set is the LIVE pages only. An
	// excluded twin is a different document that the app does not render: it is
	// neither rewritten nor deleted here, and its exclusion is carried forward
	// when it is the only match (#914).
	existingExcluded := false
	// Where the page being rewritten currently sits, so a statement that says
	// nothing about folders leaves it there (#932).
	var existingContainerID model.ID
	var existingDocumentation string
	haveExistingPage := false
	var excludedMatches []*pages.Page
	matches := 0
	for _, p := range existingPages {
		modID := getModuleID(ctx, p.ContainerID)
		modName := getModuleName(ctx, modID)
		if modName != s.Name.Module || p.Name != s.Name.Name {
			continue
		}
		matches++
		if !s.IsReplace && !s.IsModify && matches == 1 {
			return mdlerrors.NewAlreadyExists("page", s.Name.String())
		}
		if p.Excluded {
			excludedMatches = append(excludedMatches, p)
			continue
		}
		if len(pagesToDelete) == 0 {
			existingAllowedRoles = cloneRoleIDs(p.AllowedRoles)
			preserveAllowedRoles = true
			existingContainerID = p.ContainerID
			existingDocumentation = p.Documentation
			haveExistingPage = true
		}
		pagesToDelete = append(pagesToDelete, p.ID)
	}
	if len(pagesToDelete) == 0 && len(excludedMatches) > 0 {
		// Every page with this name is excluded: rewrite the first of them in
		// place and keep it excluded, rather than adding an active page the
		// script never asked to un-exclude.
		p := excludedMatches[0]
		existingAllowedRoles = cloneRoleIDs(p.AllowedRoles)
		preserveAllowedRoles = true
		existingExcluded = true
		existingContainerID = p.ContainerID
		existingDocumentation = p.Documentation
		haveExistingPage = true
		pagesToDelete = append(pagesToDelete, p.ID)
	}

	// Build the page BEFORE deleting the old one (atomic: if build fails, old page is preserved)
	pb := &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       s.Name.Module,
		widgetScope:      make(map[string]model.ID),
		paramScope:       make(map[string]model.ID),
		paramEntityNames: make(map[string]string),
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
	}

	page, err := pb.buildPageV3(s)
	if err != nil {
		return mdlerrors.NewBackend("build page", err)
	}
	page.Excluded = page.Excluded || existingExcluded
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if haveExistingPage {
		page.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existingDocumentation)
	}
	if preserveAllowedRoles {
		page.AllowedRoles = existingAllowedRoles
	} else if len(page.AllowedRoles) == 0 {
		page.AllowedRoles = defaultDocumentAccessRoles(ctx, module)
	}

	// Replace or create the page in the MPR
	if len(pagesToDelete) > 0 {
		// Reuse first existing page's UUID to avoid git delete+add (which crashes Studio Pro RevStatusCache)
		page.ID = pagesToDelete[0]
		if s.Folder == "" {
			page.ContainerID = existingContainerID
		}
		if err := ctx.Backend.UpdatePage(page); err != nil {
			return mdlerrors.NewBackend("update page", err)
		}
		if _, err := applyDocumentFolder(ctx, page.ID, existingContainerID, page.ContainerID); err != nil {
			return err
		}
		// Delete any additional duplicates
		for _, id := range pagesToDelete[1:] {
			if err := ctx.Backend.DeletePage(id); err != nil {
				return mdlerrors.NewBackend("delete duplicate page", err)
			}
		}
	} else {
		if err := ctx.Backend.CreatePage(page); err != nil {
			return mdlerrors.NewBackend("create page", err)
		}
	}

	// Track the created page so it can be resolved by subsequent page references
	ctx.trackCreatedPage(s.Name.Module, s.Name.Name, page.ID, moduleID)

	// Invalidate hierarchy cache so the new page's container is visible
	invalidateHierarchy(ctx)

	// "Created" was printed for the replace path too, so re-running a script
	// against an unchanged page claimed to have created it every time. Duplicate
	// pages of the same name are deleted above, and a delete is a real change
	// that unit-write counting does not see — so only a plain one-for-one
	// replacement is eligible to be reported as unchanged.
	switch {
	case len(pagesToDelete) == 1:
		ctx.ReportMutation("Replaced", "page %s", s.Name.String())
	case len(pagesToDelete) > 1:
		fmt.Fprintf(ctx.Output, "Replaced page %s\n", s.Name.String())
	default:
		fmt.Fprintf(ctx.Output, "Created page %s\n", s.Name.String())
	}
	return nil
}

// execCreateSnippetV3 handles CREATE SNIPPET statement with V3 syntax.
func execCreateSnippetV3(ctx *ExecContext, s *ast.CreateSnippetStmtV3) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("find module %s", s.Name.Module), err)
	}
	moduleID := module.ID

	// Check if snippet already exists - collect ALL duplicates
	existingSnippets, _ := ctx.Backend.ListSnippets()
	var snippetsToDelete []model.ID
	// As for pages: an excluded twin of this name is a separate document that
	// Mendix allows, so it is neither replaced nor deleted, and an exclusion is
	// carried forward when every match is excluded (#914).
	existingExcluded := false
	// Where the snippet being replaced currently sits. A snippet is rewritten
	// as delete+create, so unlike the Update* doctypes its container really is
	// re-applied on every statement — leaving this unset filed a foldered
	// snippet back into the module root whenever a script that says nothing
	// about folders was re-run (#932).
	var existingContainerID model.ID
	var existingSnipDoc string
	haveExistingSnip := false
	var excludedSnippets []*pages.Snippet
	matches := 0
	for _, snip := range existingSnippets {
		modID := getModuleID(ctx, snip.ContainerID)
		modName := getModuleName(ctx, modID)
		if modName != s.Name.Module || snip.Name != s.Name.Name {
			continue
		}
		matches++
		if !s.IsReplace && !s.IsModify && matches == 1 {
			return mdlerrors.NewAlreadyExists("snippet", s.Name.String())
		}
		if snip.Excluded {
			excludedSnippets = append(excludedSnippets, snip)
			continue
		}
		if len(snippetsToDelete) == 0 {
			existingContainerID = snip.ContainerID
			existingSnipDoc = snip.Documentation
			haveExistingSnip = true
		}
		snippetsToDelete = append(snippetsToDelete, snip.ID)
	}
	if len(snippetsToDelete) == 0 && len(excludedSnippets) > 0 {
		existingExcluded = true
		existingContainerID = excludedSnippets[0].ContainerID
		existingSnipDoc = excludedSnippets[0].Documentation
		haveExistingSnip = true
		snippetsToDelete = append(snippetsToDelete, excludedSnippets[0].ID)
	}

	// Build the snippet BEFORE deleting the old one (atomic: if build fails, old snippet is preserved)
	pb := &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       s.Name.Module,
		widgetScope:      make(map[string]model.ID),
		paramScope:       make(map[string]model.ID),
		paramEntityNames: make(map[string]string),
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
	}

	snippet, err := pb.buildSnippetV3(s)
	if err != nil {
		return mdlerrors.NewBackend("build snippet", err)
	}

	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if haveExistingSnip {
		snippet.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existingSnipDoc)
	}
	snippet.Excluded = snippet.Excluded || existingExcluded
	if s.Folder == "" && existingContainerID != "" {
		snippet.ContainerID = existingContainerID
	}

	// Replace or create the snippet in the MPR, the same way a page does.
	//
	// This used to be an unconditional delete-then-create, and that destroyed
	// everything the stored document carried that a rebuild cannot express —
	// most visibly its TRANSLATIONS. A `create or replace snippet` reset a
	// translated label to its source language in every language, silently:
	// the run reported "Created snippet", `mx check` reported 0 errors, and the
	// app rendered in one language (ako/mxcli-ledger #143).
	//
	// Delete-then-create is not a write that canon.Reconcile can preserve
	// anything through, because by the time the create runs there is no stored
	// document left to reconcile against — the identity transplant and the
	// unchanged-elision both have nothing to work from. Reusing the stored
	// unit's ID and going through Update puts the snippet back on the same
	// choke point every other document type uses (ADR-0008). It also fixes the
	// second consequence the page path documents: a delete+add churns the file
	// in git and crashes Studio Pro's RevStatusCache.
	if len(snippetsToDelete) > 0 {
		snippet.ID = snippetsToDelete[0]
		if err := ctx.Backend.UpdateSnippet(snippet); err != nil {
			return mdlerrors.NewBackend("update snippet", err)
		}
		// Any further documents of the same name are genuine duplicates.
		for _, id := range snippetsToDelete[1:] {
			if err := ctx.Backend.DeleteSnippet(id); err != nil {
				return mdlerrors.NewBackend("delete duplicate snippet", err)
			}
		}
	} else {
		if err := ctx.Backend.CreateSnippet(snippet); err != nil {
			return mdlerrors.NewBackend("create snippet", err)
		}
	}

	// Track the created snippet so it can be resolved by subsequent snippet references
	ctx.trackCreatedSnippet(s.Name.Module, s.Name.Name, snippet.ID, moduleID)

	// Invalidate hierarchy cache so the new snippet's container is visible
	invalidateHierarchy(ctx)

	// Same rule as pages: "Created" was printed for the replace path too, so
	// re-running a script against an unchanged snippet claimed to create it every
	// time. Only a plain one-for-one replacement is eligible to be reported as
	// unchanged — deleting a duplicate is a real change unit-write counting
	// cannot see.
	switch {
	case len(snippetsToDelete) == 1:
		ctx.ReportMutation("Replaced", "snippet %s", s.Name.String())
	case len(snippetsToDelete) > 1:
		fmt.Fprintf(ctx.Output, "Replaced snippet %s\n", s.Name.String())
	default:
		fmt.Fprintf(ctx.Output, "Created snippet %s\n", s.Name.String())
	}
	return nil
}
