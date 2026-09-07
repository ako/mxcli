// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// navFixture builds a catalog with one user module, four pages, and a
// Responsive profile that routes to three of them: a home page, a role home,
// and a menu item. The fourth page is reachable only from inside another
// screen, which is what CONV019 must stay quiet about.
func navFixture(t *testing.T) *catalog.Catalog {
	t.Helper()

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec %s: %v", q, err)
		}
	}

	exec(`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		"mod-1", "Sales", "default", "s1")

	addPage := func(id, name, url string) {
		exec(`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, Folder, URL, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?)`,
			id, name, "Sales."+name, "Sales", "", url, "default", "s1")
	}
	addPage("pg-home", "Home", "")             // profile home, no URL
	addPage("pg-orders", "Order_Overview", "") // menu target, no URL
	addPage("pg-admin", "Admin_Home", "")      // role home, no URL
	addPage("pg-detail", "Order_Detail", "")   // NOT in navigation

	exec(`INSERT INTO navigation_profiles_data
		(ProfileName, Kind, IsNative, HomePage, HomePageType, LoginPage, NotFoundPage,
		 MenuItemCount, RoleBasedHomeCount, OfflineEntityCount, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		"Responsive", "Responsive", 0, "Sales.Home", "PAGE", "", "", 1, 1, 0, "default", "s1")

	exec(`INSERT INTO navigation_menu_items
		(ProfileName, ItemPath, Depth, Caption, ActionType, TargetPage, TargetMicroflow,
		 SubItemCount, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"Responsive", "0", 0, "Orders", "PAGE", "Sales.Order_Overview", "", 0, "default", "s1")

	exec(`INSERT INTO navigation_role_homes (ProfileName, UserRole, Page, Microflow, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?)`,
		"Responsive", "Administrator", "Sales.Admin_Home", "", "default", "s1")

	return cat
}

func runNavURLRule(t *testing.T, cat *catalog.Catalog) []linter.Violation {
	t.Helper()
	// The rule that ships, loaded from disk.
	rule, err := linter.LoadStarlarkRule("../../.claude/lint-rules/conv019_navigation_page_url.star")
	if err != nil {
		t.Fatalf("loading the shipped CONV019: %v", err)
	}
	return rule.Check(linter.NewLintContext(cat, &minimalReader{}))
}

// The three routes a profile can take, and the page that is not on any of them.
func TestCONV019_FlagsNavigationTargetsWithoutURL(t *testing.T) {
	vs := runNavURLRule(t, navFixture(t))

	if len(vs) != 3 {
		t.Fatalf("got %d violations, want 3:\n%s", len(vs), messages(vs))
	}
	if vs[0].RuleID != "CONV019" {
		t.Errorf("RuleID = %q, want CONV019", vs[0].RuleID)
	}

	joined := messages(vs)
	for _, want := range []string{
		"Sales.Admin_Home",
		"the home page for role 'Administrator'",
		"Sales.Home",
		"the home page of the 'Responsive' profile",
		"Sales.Order_Overview",
		"the 'Orders' menu item",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q:\n%s", want, joined)
		}
	}

	// The whole point of the narrowing: a page reached only from inside another
	// screen is not expected to be addressable.
	if strings.Contains(joined, "Order_Detail") {
		t.Errorf("a page that is not a navigation target was flagged:\n%s", joined)
	}
}

// A page that HAS a URL is the remedy working, so it must go quiet.
func TestCONV019_SilentOnceTheURLIsSet(t *testing.T) {
	cat := navFixture(t)
	if _, err := cat.CatalogDB().Exec(
		`UPDATE pages_data SET URL = 'orders' WHERE Id = ?`, "pg-orders"); err != nil {
		t.Fatalf("update url: %v", err)
	}

	vs := runNavURLRule(t, cat)
	if strings.Contains(messages(vs), "Order_Overview") {
		t.Errorf("a page with a URL was still flagged:\n%s", messages(vs))
	}
	// Control: the other two, still without URLs, must still report — otherwise
	// this passes against a rule that stopped working altogether.
	if len(vs) != 2 {
		t.Fatalf("got %d violations after setting one URL, want 2:\n%s", len(vs), messages(vs))
	}
}

// One page routed to two ways is one finding naming both routes, not two
// findings for the same page.
func TestCONV019_ReportsAPageOnceAcrossRoutes(t *testing.T) {
	cat := navFixture(t)
	if _, err := cat.CatalogDB().Exec(`INSERT INTO navigation_menu_items
		(ProfileName, ItemPath, Depth, Caption, ActionType, TargetPage, TargetMicroflow,
		 SubItemCount, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"Responsive", "1", 0, "Dashboard", "PAGE", "Sales.Home", "", 0, "default", "s1"); err != nil {
		t.Fatalf("insert menu item: %v", err)
	}

	vs := runNavURLRule(t, cat)
	if len(vs) != 3 {
		t.Fatalf("got %d violations, want 3 — Sales.Home must not be reported twice:\n%s",
			len(vs), messages(vs))
	}

	var home string
	for _, v := range vs {
		if strings.Contains(v.Message, "Sales.Home'") {
			home = v.Message
		}
	}
	if home == "" {
		t.Fatalf("Sales.Home was not reported:\n%s", messages(vs))
	}
	for _, want := range []string{"the home page of the 'Responsive' profile", "the 'Dashboard' menu item"} {
		if !strings.Contains(home, want) {
			t.Errorf("the finding does not name the %q route:\n%s", want, home)
		}
	}
}

// Marketplace pages are somebody else's model. They drop out of the join
// against pages() rather than being listed, which is easy to get wrong.
func TestCONV019_IgnoresMarketplaceTargets(t *testing.T) {
	cat := navFixture(t)
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, Source, ProjectId, SnapshotId) VALUES (?,?,?,?,?)`,
		"mod-2", "Administration", "Marketplace", "default", "s1"); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO pages_data
		(Id, Name, QualifiedName, ModuleName, Folder, URL, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?)`,
		"pg-acct", "Account_Overview", "Administration.Account_Overview", "Administration",
		"", "", "default", "s1"); err != nil {
		t.Fatalf("insert page: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO navigation_menu_items
		(ProfileName, ItemPath, Depth, Caption, ActionType, TargetPage, TargetMicroflow,
		 SubItemCount, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"Responsive", "2", 0, "Accounts", "PAGE", "Administration.Account_Overview", "",
		0, "default", "s1"); err != nil {
		t.Fatalf("insert menu item: %v", err)
	}

	vs := runNavURLRule(t, cat)
	if strings.Contains(messages(vs), "Administration") {
		t.Errorf("a Marketplace page was flagged:\n%s", messages(vs))
	}
	// Control: the three user pages still report, so the silence above is the
	// module filter and not the fixture failing to reach the rule.
	if len(vs) != 3 {
		t.Fatalf("got %d violations, want the 3 user pages:\n%s", len(vs), messages(vs))
	}
}

// NavigationTargets is the projection the rule stands on. The login and
// not-found pages must NOT appear: the platform routes to those itself, so a
// URL on them buys nothing and reporting them would be pure noise.
func TestNavigationTargets_ExcludeLoginAndNotFound(t *testing.T) {
	cat := navFixture(t)
	if _, err := cat.CatalogDB().Exec(
		`UPDATE navigation_profiles_data SET LoginPage = ?, NotFoundPage = ? WHERE ProfileName = ?`,
		"Sales.Login", "Sales.NotFound", "Responsive"); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	ctx := linter.NewLintContext(cat, &minimalReader{})
	kinds := map[string]string{}
	for target := range ctx.NavigationTargets() {
		kinds[target.Page] = target.Kind
		if target.Page == "Sales.Login" || target.Page == "Sales.NotFound" {
			t.Errorf("%s is routed by the platform and must not be a navigation target", target.Page)
		}
	}

	// Control: the three real targets ARE there with the right kind, so the
	// absence above is the WHERE clause and not an empty query.
	want := map[string]string{
		"Sales.Home":           "home",
		"Sales.Admin_Home":     "role_home",
		"Sales.Order_Overview": "menu",
	}
	for page, kind := range want {
		if kinds[page] != kind {
			t.Errorf("target %s = kind %q, want %q (targets: %v)", page, kinds[page], kind, kinds)
		}
	}
}

// A microflow-valued home page has no page to be addressable, so it must not
// produce a target at all — otherwise the rule would try to look up a microflow
// name in the page map.
func TestNavigationTargets_SkipMicroflowHomePages(t *testing.T) {
	cat := navFixture(t)
	if _, err := cat.CatalogDB().Exec(
		`UPDATE navigation_profiles_data SET HomePage = ?, HomePageType = ? WHERE ProfileName = ?`,
		"Sales.ACT_Route", "MICROFLOW", "Responsive"); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	ctx := linter.NewLintContext(cat, &minimalReader{})
	n := 0
	for target := range ctx.NavigationTargets() {
		if target.Page == "Sales.ACT_Route" {
			t.Errorf("a microflow home page was yielded as a page target: %+v", target)
		}
		n++
	}
	// Control: the menu item and the role home survive, so the exclusion is the
	// HomePageType filter rather than the whole query going empty.
	if n != 2 {
		t.Errorf("got %d targets, want the 2 that are still pages", n)
	}
}
