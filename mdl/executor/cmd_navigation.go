// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// execAlterNavigation handles CREATE [OR REPLACE] NAVIGATION <profile> command.
// It fully replaces the profile's home pages, login page, not-found page, and menu tree.
func execAlterNavigation(ctx *ExecContext, s *ast.AlterNavigationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Resolve `FOR <user role>` before anything is written — including the
	// profile this may add below. A module-qualified role here produces a project
	// Mendix cannot load at all, so refusing is the only useful outcome
	// (mendixlabs/mxcli#1001). Same function `check` calls.
	if err := validateNavigationRoleForExec(ctx, s); err != nil {
		return err
	}

	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	// Verify the profile exists
	createdProfile := false
	createdKind := ""
	profileFound := false
	for _, p := range nav.Profiles {
		if strings.EqualFold(p.Name, s.ProfileName) {
			profileFound = true
			break
		}
	}
	if !profileFound {
		// CREATE OR REPLACE could only ever REPLACE: a profile that did not exist
		// was an error, and nothing else in mxcli added one. That put a whole class
		// of app out of reach, because Mendix routes a phone to a Phone PROFILE on
		// User-Agent — not on viewport width — so a device-specific front end could
		// not be built from MDL at all (ako/mxcli-maintenance §7).
		//
		// Mendix's web profiles are a closed set, so an unknown name is still an
		// error: creating "Phone" is adding the profile the platform defines,
		// while creating "Mobile" would be inventing one that can never route.
		kind, ok := types.CanonicalProfileKind(s.ProfileName)
		if !ok {
			return mdlerrors.NewNotFoundMsg("navigation profile", s.ProfileName,
				fmt.Sprintf("navigation profile not found: %s (available: %s; Mendix's web profiles are %s)",
					s.ProfileName, profileNames(nav), strings.Join(types.WebProfileKindNames(), ", ")))
		}
		if err := ctx.Backend.AddNavigationProfile(nav.ID, kind); err != nil {
			return mdlerrors.NewBackend("create navigation profile", err)
		}
		createdProfile, createdKind = true, kind
		// Re-read: the spec below is applied against the stored document, and the
		// profile it targets has only just come into existence.
		if nav, err = ctx.Backend.GetNavigation(); err != nil {
			return mdlerrors.NewBackend("reload navigation", err)
		}
	}

	// Convert AST types to writer spec
	spec := types.NavigationProfileSpec{
		HasMenu: s.HasMenuBlock,
	}

	for _, hp := range s.HomePages {
		hpSpec := types.NavHomePageSpec{
			IsPage: hp.IsPage,
			Target: hp.Target.String(),
		}
		if hp.ForRole != nil {
			hpSpec.ForRole = hp.ForRole.String()
		}
		spec.HomePages = append(spec.HomePages, hpSpec)
	}

	if s.LoginPage != nil {
		spec.LoginPage = s.LoginPage.String()
	}
	if s.NotFoundPage != nil {
		spec.NotFoundPage = s.NotFoundPage.String()
	}

	for _, mi := range s.MenuItems {
		spec.MenuItems = append(spec.MenuItems, convertMenuItemDef(mi))
	}

	if err := ctx.Backend.UpdateNavigationProfile(nav.ID, s.ProfileName, spec); err != nil {
		return mdlerrors.NewBackend("update navigation profile", err)
	}

	// Say which of the two happened. A run that silently reports "updated" after
	// CREATING a profile hides the fact that the project gained a routing target
	// it did not have — and a phone profile changes which pages a phone lands on.
	if createdProfile {
		fmt.Fprintf(ctx.Output, "Navigation profile '%s' created.\n", s.ProfileName)
		// An offline profile changes what the platform demands of pages this
		// statement never mentioned. Say so now, not at the next build.
		warnOfflineIncompatiblePages(ctx, createdKind)
	} else {
		fmt.Fprintf(ctx.Output, "Navigation profile '%s' updated.\n", s.ProfileName)
	}
	return nil
}

// convertMenuItemDef converts an AST NavMenuItemDef to a writer NavMenuItemSpec.
func convertMenuItemDef(def ast.NavMenuItemDef) types.NavMenuItemSpec {
	spec := types.NavMenuItemSpec{
		Caption: def.Caption,
		Icon:    def.Icon,
	}
	if def.Page != nil {
		spec.Page = def.Page.String()
	}
	if def.Microflow != nil {
		spec.Microflow = def.Microflow.String()
	}
	for _, sub := range def.Items {
		spec.Items = append(spec.Items, convertMenuItemDef(sub))
	}
	return spec
}

// profileNames returns a comma-separated list of profile names for error messages.
func profileNames(nav *types.NavigationDocument) string {
	names := make([]string, len(nav.Profiles))
	for i, p := range nav.Profiles {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}

// listNavigation handles SHOW NAVIGATION command.
// Displays an overview of all navigation profiles with their home pages and menu item counts.
func listNavigation(ctx *ExecContext) error {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	if len(nav.Profiles) == 0 {
		fmt.Fprintln(ctx.Output, "No navigation profiles found.")
		return nil
	}

	type row struct {
		name      string
		kind      string
		homePage  string
		loginPage string
		menuItems int
		roleHomes int
	}
	var rows []row

	for _, p := range nav.Profiles {
		homePage := ""
		if p.HomePage != nil {
			if p.HomePage.Page != "" {
				homePage = p.HomePage.Page
			} else if p.HomePage.Microflow != "" {
				homePage = "MF:" + p.HomePage.Microflow
			}
		}

		loginPage := p.LoginPage
		if loginPage == "" {
			loginPage = "-"
		}

		menuCount := countMenuItems(p.MenuItems)

		kind := p.Kind
		if p.IsNative {
			kind += " (native)"
		}

		rows = append(rows, row{p.Name, kind, homePage, loginPage, menuCount, len(p.RoleBasedHomePages)})
	}

	result := &TableResult{
		Columns: []string{"Profile", "Kind", "HomePage", "LoginPage", "MenuItems", "RoleHomes"},
		Summary: fmt.Sprintf("(%d navigation profiles)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.name, r.kind, r.homePage, r.loginPage, r.menuItems, r.roleHomes})
	}
	return writeResult(ctx, result)
}

// listNavigationMenu handles SHOW NAVIGATION MENU [profile] command.
// Displays the menu tree for a specific profile, or all profiles if none specified.
func listNavigationMenu(ctx *ExecContext, profileName *ast.QualifiedName) error {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	for _, p := range nav.Profiles {
		if profileName != nil && !strings.EqualFold(p.Name, profileName.Name) {
			continue
		}

		fmt.Fprintf(ctx.Output, "-- Navigation Menu: %s (%s)\n", p.Name, p.Kind)
		if len(p.MenuItems) == 0 {
			fmt.Fprintln(ctx.Output, "  (no menu items)")
		} else {
			printMenuTree(ctx.Output, p.MenuItems, 0)
		}
		fmt.Fprintln(ctx.Output)
	}

	return nil
}

// listNavigationHomes handles SHOW NAVIGATION HOMES command.
// Displays all home page configurations including role-based overrides.
func listNavigationHomes(ctx *ExecContext) error {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	for _, p := range nav.Profiles {
		fmt.Fprintf(ctx.Output, "-- Profile: %s (%s)\n", p.Name, p.Kind)

		// Default home page
		if p.HomePage != nil {
			if p.HomePage.Page != "" {
				fmt.Fprintf(ctx.Output, "  Default Home: page %s\n", p.HomePage.Page)
			} else if p.HomePage.Microflow != "" {
				fmt.Fprintf(ctx.Output, "  Default Home: microflow %s\n", p.HomePage.Microflow)
			}
		} else {
			fmt.Fprintln(ctx.Output, "  Default Home: (none)")
		}

		// Role-based home pages
		if len(p.RoleBasedHomePages) > 0 {
			fmt.Fprintln(ctx.Output, "  Role-Based Homes:")
			for _, rh := range p.RoleBasedHomePages {
				target := ""
				if rh.Page != "" {
					target = "page " + rh.Page
				} else if rh.Microflow != "" {
					target = "microflow " + rh.Microflow
				}
				fmt.Fprintf(ctx.Output, "    %s -> %s\n", rh.UserRole, target)
			}
		}

		fmt.Fprintln(ctx.Output)
	}

	return nil
}

// describeNavigation handles DESCRIBE NAVIGATION [profile] command.
// Outputs a complete MDL-style description of a navigation profile.
func describeNavigation(ctx *ExecContext, name ast.QualifiedName) error {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	// If no profile name, describe all profiles
	if name.Name == "" {
		for _, p := range nav.Profiles {
			outputNavigationProfile(ctx, p)
		}
		return nil
	}

	// Find specific profile
	for _, p := range nav.Profiles {
		if strings.EqualFold(p.Name, name.Name) {
			outputNavigationProfile(ctx, p)
			return nil
		}
	}

	return mdlerrors.NewNotFound("navigation profile", name.Name)
}

// outputNavigationProfile outputs a single profile in round-trippable CREATE OR REPLACE NAVIGATION format.
func outputNavigationProfile(ctx *ExecContext, p *types.NavigationProfile) {
	fmt.Fprintf(ctx.Output, "-- navigation PROFILE: %s\n", p.Name)
	fmt.Fprintf(ctx.Output, "--   Kind: %s\n", p.Kind)
	if p.IsNative {
		fmt.Fprintf(ctx.Output, "--   Native: Yes\n")
	}

	fmt.Fprintf(ctx.Output, "create or replace navigation %s\n", p.Name)

	// Home page
	if p.HomePage != nil {
		if p.HomePage.Page != "" {
			fmt.Fprintf(ctx.Output, "  home page %s\n", p.HomePage.Page)
		} else if p.HomePage.Microflow != "" {
			fmt.Fprintf(ctx.Output, "  home microflow %s\n", p.HomePage.Microflow)
		}
	}

	// Role-based home pages
	for _, rh := range p.RoleBasedHomePages {
		if rh.Page != "" {
			fmt.Fprintf(ctx.Output, "  home page %s for %s\n", rh.Page, rh.UserRole)
		} else if rh.Microflow != "" {
			fmt.Fprintf(ctx.Output, "  home microflow %s for %s\n", rh.Microflow, rh.UserRole)
		}
	}

	// Login page
	if p.LoginPage != "" {
		fmt.Fprintf(ctx.Output, "  login page %s\n", p.LoginPage)
	}

	// Not-found page
	if p.NotFoundPage != "" {
		fmt.Fprintf(ctx.Output, "  not found page %s\n", p.NotFoundPage)
	}

	// Menu items
	if len(p.MenuItems) > 0 {
		fmt.Fprintln(ctx.Output, "  menu (")
		printMenuMDL(ctx.Output, p.MenuItems, 2, "CREATE NAVIGATION")
		fmt.Fprintln(ctx.Output, "  )")
	}

	// Offline entities (as comments since CREATE NAVIGATION doesn't handle sync yet)
	if len(p.OfflineEntities) > 0 {
		fmt.Fprintln(ctx.Output, "  -- Offline Entities (not yet modifiable):")
		for _, oe := range p.OfflineEntities {
			constraint := ""
			if oe.Constraint != "" {
				constraint = fmt.Sprintf(" where '%s'", oe.Constraint)
			}
			fmt.Fprintf(ctx.Output, "  -- SYNC %s MODE %s%s;\n", oe.Entity, oe.SyncMode, constraint)
		}
	}

	fmt.Fprintln(ctx.Output, ";")
	fmt.Fprintln(ctx.Output)
}

// countMenuItems counts the total number of menu items recursively.
func countMenuItems(items []*types.NavMenuItem) int {
	count := len(items)
	for _, item := range items {
		count += countMenuItems(item.Items)
	}
	return count
}

// printMenuTree prints a menu tree with indentation to an io.Writer.
func printMenuTree(w io.Writer, items []*types.NavMenuItem, depth int) {
	indent := strings.Repeat("  ", depth+1)
	for _, item := range items {
		target := menuItemTarget(item)
		fmt.Fprintf(w, "%s%s%s\n", indent, item.Caption, target)
		if len(item.Items) > 0 {
			printMenuTree(w, item.Items, depth+1)
		}
	}
}

// menuItemTarget returns a display string for a menu item's action target.
func menuItemTarget(item *types.NavMenuItem) string {
	if item.Page != "" {
		return " -> " + item.Page
	}
	if item.Microflow != "" {
		return " -> MF:" + item.Microflow
	}
	return ""
}

// printMenuMDL prints menu items in MDL-style format. reproducer names the
// construct an icon note should point at — navigation menus are authored by
// CREATE NAVIGATION, while a standalone menu document cannot be authored at all.
func printMenuMDL(w io.Writer, items []*types.NavMenuItem, depth int, reproducer string) {
	indent := strings.Repeat("  ", depth)
	for _, item := range items {
		icon := menuItemIconMDL(item)
		if len(item.Items) > 0 {
			// Sub-menu container
			fmt.Fprintf(w, "%smenu '%s'%s (\n", indent, item.Caption, icon)
			printMenuMDL(w, item.Items, depth+1, reproducer)
			fmt.Fprintf(w, "%s);\n", indent)
		} else if item.Page != "" {
			fmt.Fprintf(w, "%smenu item '%s' page %s%s;\n", indent, item.Caption, item.Page, icon)
		} else if item.Microflow != "" {
			fmt.Fprintf(w, "%smenu item '%s' microflow %s%s;\n", indent, item.Caption, item.Microflow, icon)
		} else {
			fmt.Fprintf(w, "%smenu item '%s'%s;\n", indent, item.Caption, icon)
		}
		if note := menuItemIconNote(item, reproducer); note != "" {
			fmt.Fprintf(w, "%s%s\n", indent, note)
		}
	}
}

// menuItemIconMDL renders the ICON clause for a menu item, or "" when there is
// nothing CREATE NAVIGATION can reproduce.
func menuItemIconMDL(item *types.NavMenuItem) string {
	if item.Icon == "" || !strings.HasSuffix(item.IconType, "IconCollectionIcon") {
		return ""
	}
	return " icon " + quoteQualifiedName(item.Icon)
}

// menuItemIconNote flags an icon DESCRIBE cannot round-trip, so re-running the
// output loses it visibly rather than silently. CREATE NAVIGATION writes only
// Forms$IconCollectionIcon; a glyph icon (numeric Code) or an image icon
// (pointing into an image collection, not an icon collection) is a different
// element and would have to be guessed at.
func menuItemIconNote(item *types.NavMenuItem, reproducer string) string {
	if item.IconType == "" || strings.HasSuffix(item.IconType, "IconCollectionIcon") {
		return ""
	}
	target := item.Icon
	if target == "" {
		target = "a numeric glyph code"
	}
	return fmt.Sprintf("-- icon %s (%s) is not reproducible by %s; set it in Studio Pro",
		target, item.IconType, reproducer)
}
