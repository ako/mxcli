// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// PageNavigationSecurityRule checks that pages used in navigation have at least
// one allowed module role. Studio Pro reports this as CE0557 but mx check does not.
//
// It also checks, per navigation profile, that every user role can open the
// home page it lands on: its role-based home page if it has one, otherwise the
// profile's default. A user role none of whose module roles is allowed on that
// page passes `check --references` and reaches mxbuild, which then reports
// CE2729 for every widget on the page ("No read access to attribute … for user
// role 'X' (with no roles defined in module …)") — ten of them for one leftover
// template role in ChipCoV6, and nothing that names the actual cause.
type PageNavigationSecurityRule struct{}

// NewPageNavigationSecurityRule creates a new page navigation security rule.
func NewPageNavigationSecurityRule() *PageNavigationSecurityRule {
	return &PageNavigationSecurityRule{}
}

func (r *PageNavigationSecurityRule) ID() string                       { return "MPR007" }
func (r *PageNavigationSecurityRule) Name() string                     { return "PageNavigationSecurity" }
func (r *PageNavigationSecurityRule) Category() string                 { return "security" }
func (r *PageNavigationSecurityRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *PageNavigationSecurityRule) Description() string {
	return "Checks that pages used in navigation have at least one allowed role (CE0557), and that every user role can open its home page (else CE2729 per widget)"
}

// navUsage describes how a page is used in navigation.
type navUsage struct {
	Profile string
	Context string // "home page", "menu item 'Caption'", "login page", etc.
}

// Check finds pages referenced in navigation profiles and verifies they have allowed roles.
func (r *PageNavigationSecurityRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	nav, err := reader.GetNavigation()
	if err != nil || nav == nil {
		return nil
	}

	// Collect all pages used in navigation, with usage context
	navPages := make(map[string][]navUsage) // qualifiedName → usages

	for _, profile := range nav.Profiles {
		pName := profile.Kind
		if pName == "" {
			pName = profile.Name
		}

		if profile.HomePage != nil && profile.HomePage.Page != "" {
			navPages[profile.HomePage.Page] = append(navPages[profile.HomePage.Page],
				navUsage{Profile: pName, Context: "home page"})
		}

		for _, rbh := range profile.RoleBasedHomePages {
			if rbh.Page != "" {
				navPages[rbh.Page] = append(navPages[rbh.Page],
					navUsage{Profile: pName, Context: fmt.Sprintf("role-based home page for %s", rbh.UserRole)})
			}
		}

		if profile.LoginPage != "" {
			navPages[profile.LoginPage] = append(navPages[profile.LoginPage],
				navUsage{Profile: pName, Context: "login page"})
		}

		if profile.NotFoundPage != "" {
			navPages[profile.NotFoundPage] = append(navPages[profile.NotFoundPage],
				navUsage{Profile: pName, Context: "not-found page"})
		}

		collectMenuPages(profile.MenuItems, pName, navPages)
	}

	// Build map of qualified page name → allowed module roles
	pageRoles := buildPageRoleMap(reader)

	var violations []linter.Violation
	for pageName, usages := range navPages {
		moduleName := moduleFromQualified(pageName)
		if ctx.IsExcluded(moduleName) {
			continue
		}

		roles, found := pageRoles[pageName]
		if !found {
			continue // page not found in project (may be from marketplace module)
		}

		if len(roles) > 0 {
			continue // has at least one allowed role
		}

		usage := usages[0]
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message: fmt.Sprintf("Page '%s' is used as %s in %s navigation but has no allowed roles (CE0557)",
				pageName, usage.Context, usage.Profile),
			Location: linter.Location{
				Module:       moduleName,
				DocumentType: "page",
				DocumentName: docNameFromQualified(pageName),
			},
			Suggestion: fmt.Sprintf("Grant view access: GRANT VIEW ON PAGE %s TO %s.<RoleName>", pageName, moduleName),
		})
	}

	return append(violations, r.checkHomePagePerUserRole(ctx, nav, pageRoles)...)
}

// checkHomePagePerUserRole reports each (profile, user role) whose effective
// home page none of the user role's module roles may open. A page with no
// allowed roles at all is left to the CE0557 check above, which already names
// it; a microflow home page is checked against its allowed module roles.
// Security off disables every role check, so nothing is reported then.
func (r *PageNavigationSecurityRule) checkHomePagePerUserRole(ctx *linter.LintContext, nav *types.NavigationDocument, pageRoles map[string][]string) []linter.Violation {
	reader := ctx.Reader()
	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil || ps.SecurityLevel == security.SecurityLevelOff || len(ps.UserRoles) == 0 {
		return nil
	}

	var mfRoles map[string][]string // loaded only if some home page is a microflow

	var violations []linter.Violation
	for _, profile := range nav.Profiles {
		pName := profile.Kind
		if pName == "" {
			pName = profile.Name
		}
		for _, ur := range ps.UserRoles {
			target, isMicroflow, viaRole := effectiveHomePage(profile, ur.Name)
			if target == "" || ctx.IsExcluded(moduleFromQualified(target)) {
				continue
			}
			var allowed []string
			var found bool
			if isMicroflow {
				if mfRoles == nil {
					mfRoles = buildMicroflowRoleMap(reader)
				}
				allowed, found = mfRoles[target]
			} else {
				allowed, found = pageRoles[target]
			}
			if !found || (!isMicroflow && len(allowed) == 0) {
				continue // not in the project, or already reported as CE0557
			}
			if sharesRole(ur.ModuleRoles, allowed) {
				continue
			}
			violations = append(violations, homePageViolation(r, ur, pName, target, isMicroflow, viaRole))
		}
	}
	return violations
}

// effectiveHomePage returns the home page (or microflow) a user role lands on
// in a profile: its role-based entry if there is one, else the default.
func effectiveHomePage(profile *types.NavigationProfile, userRole string) (target string, isMicroflow, viaRole bool) {
	for _, rbh := range profile.RoleBasedHomePages {
		if rbh.UserRole != userRole {
			continue
		}
		if rbh.Page != "" {
			return rbh.Page, false, true
		}
		if rbh.Microflow != "" {
			return rbh.Microflow, true, true
		}
	}
	if profile.HomePage == nil {
		return "", false, false
	}
	if profile.HomePage.Page != "" {
		return profile.HomePage.Page, false, false
	}
	return profile.HomePage.Microflow, profile.HomePage.Microflow != "", false
}

func homePageViolation(r *PageNavigationSecurityRule, ur *security.UserRole, profile, target string, isMicroflow, viaRole bool) linter.Violation {
	kind, docType, verb, grant := "page", "page", "open", "GRANT VIEW ON PAGE"
	if isMicroflow {
		kind, docType, verb, grant = "microflow", "microflow", "run", "GRANT EXECUTE ON MICROFLOW"
	}
	which := "default home " + kind
	if viaRole {
		which = "role-based home " + kind
	}
	moduleName := moduleFromQualified(target)
	suggestion := fmt.Sprintf("%s %s TO %s.<RoleName> for a module role of user role '%s'", grant, target, moduleName, ur.Name)
	if !viaRole {
		suggestion += fmt.Sprintf(", or give '%s' a role-based home page it can %s in %s navigation", ur.Name, verb, profile)
	}
	return linter.Violation{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message: fmt.Sprintf("User role '%s' cannot %s %s '%s' in %s navigation: none of its module roles is allowed (mxbuild reports CE2729 for each widget on it)",
			ur.Name, verb, which, target, profile),
		Location: linter.Location{
			Module:       moduleName,
			DocumentType: docType,
			DocumentName: docNameFromQualified(target),
		},
		Suggestion: suggestion,
	}
}

// sharesRole reports whether any of a user role's module roles is allowed.
func sharesRole(userModuleRoles, allowed []string) bool {
	for _, a := range allowed {
		for _, m := range userModuleRoles {
			if a == m {
				return true
			}
		}
	}
	return false
}

// collectMenuPages recursively collects pages from navigation menu items.
func collectMenuPages(items []*types.NavMenuItem, profileName string, navPages map[string][]navUsage) {
	for _, item := range items {
		if item.Page != "" {
			navPages[item.Page] = append(navPages[item.Page],
				navUsage{Profile: profileName, Context: fmt.Sprintf("menu item '%s'", item.Caption)})
		}
		collectMenuPages(item.Items, profileName, navPages)
	}
}

// buildPageRoleMap builds a map of qualified page name → allowed module roles.
func buildPageRoleMap(reader linter.LintReader) map[string][]string {
	result := make(map[string][]string)
	pages, err := reader.ListPages()
	if err != nil {
		return result
	}
	resolveModule := moduleResolver(reader)
	for _, pg := range pages {
		moduleName := resolveModule(string(pg.ContainerID))
		if moduleName == "" {
			continue
		}
		result[moduleName+"."+pg.Name] = idsToStrings(pg.AllowedRoles)
	}
	return result
}

// buildMicroflowRoleMap builds a map of qualified microflow name → allowed module roles.
func buildMicroflowRoleMap(reader linter.LintReader) map[string][]string {
	result := make(map[string][]string)
	mfs, err := reader.ListMicroflows()
	if err != nil {
		return result
	}
	resolveModule := moduleResolver(reader)
	for _, mf := range mfs {
		moduleName := resolveModule(string(mf.ContainerID))
		if moduleName == "" {
			continue
		}
		result[moduleName+"."+mf.Name] = idsToStrings(mf.AllowedModuleRoles)
	}
	return result
}

// idsToStrings converts allowed-role references, which the reader fills with
// qualified module role names, to plain strings. Never nil, so a document with
// no allowed roles is distinguishable from one that was not found.
func idsToStrings(ids []model.ID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

// moduleResolver returns a function resolving a container ID (module or
// folder) to its module name, or "" when it cannot.
func moduleResolver(reader linter.LintReader) func(string) string {
	moduleByID := make(map[string]string)   // module ID → module name
	folderParent := make(map[string]string) // folder ID → parent ID
	if modules, err := reader.ListModules(); err == nil {
		for _, m := range modules {
			moduleByID[string(m.ID)] = m.Name
		}
	}
	if folders, err := reader.ListFolders(); err == nil {
		for _, f := range folders {
			folderParent[string(f.ID)] = string(f.ContainerID)
		}
	}
	return func(containerID string) string {
		id := containerID
		for i := 0; i < 20; i++ { // max depth guard
			if name, ok := moduleByID[id]; ok {
				return name
			}
			parent, ok := folderParent[id]
			if !ok {
				return ""
			}
			id = parent
		}
		return ""
	}
}

// moduleFromQualified extracts module name from "Module.Name".
func moduleFromQualified(qualifiedName string) string {
	if idx := strings.Index(qualifiedName, "."); idx > 0 {
		return qualifiedName[:idx]
	}
	return qualifiedName
}
