// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
)

func TestCollectMenuPages_Flat(t *testing.T) {
	items := []*types.NavMenuItem{
		{Page: "MyModule.HomePage", Caption: "Home"},
		{Page: "MyModule.About", Caption: "About"},
	}

	navPages := make(map[string][]navUsage)
	collectMenuPages(items, "Responsive", navPages)

	if len(navPages) != 2 {
		t.Errorf("expected 2 pages, got %d", len(navPages))
	}
	if usages, ok := navPages["MyModule.HomePage"]; !ok || len(usages) != 1 {
		t.Errorf("expected 1 usage for HomePage")
	}
}

func TestCollectMenuPages_Nested(t *testing.T) {
	items := []*types.NavMenuItem{
		{
			Caption: "Parent",
			Items: []*types.NavMenuItem{
				{Page: "MyModule.ChildPage", Caption: "Child"},
			},
		},
	}

	navPages := make(map[string][]navUsage)
	collectMenuPages(items, "Responsive", navPages)

	if _, ok := navPages["MyModule.ChildPage"]; !ok {
		t.Error("expected nested page to be collected")
	}
}

func TestCollectMenuPages_EmptyPage(t *testing.T) {
	items := []*types.NavMenuItem{
		{Page: "", Caption: "Separator"},
	}

	navPages := make(map[string][]navUsage)
	collectMenuPages(items, "Responsive", navPages)

	if len(navPages) != 0 {
		t.Errorf("expected 0 pages for empty page ref, got %d", len(navPages))
	}
}

func TestModuleFromQualified(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"MyModule.Page", "MyModule"},
		{"Admin.Login", "Admin"},
		{"NoModule", "NoModule"},
		{"", ""},
	}
	for _, tt := range tests {
		got := moduleFromQualified(tt.input)
		if got != tt.want {
			t.Errorf("moduleFromQualified(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPageNavigationSecurityRule_NilReader(t *testing.T) {
	r := NewPageNavigationSecurityRule()
	ctx := linter.NewLintContextFromDB(nil)
	violations := r.Check(ctx)
	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

func TestPageNavigationSecurityRule_Metadata(t *testing.T) {
	r := NewPageNavigationSecurityRule()
	if r.ID() != "MPR007" {
		t.Errorf("ID = %q, want MPR007", r.ID())
	}
	if r.Category() != "security" {
		t.Errorf("Category = %q, want security", r.Category())
	}
}

// navSecurityReader serves the navigation, security, pages and microflows the
// MPR007 home-page check reads; everything else is empty.
type navSecurityReader struct {
	nav   *types.NavigationDocument
	sec   *security.ProjectSecurity
	pages []*pages.Page
	mfs   []*microflows.Microflow
}

func (r navSecurityReader) GetMicroflow(model.ID) (*microflows.Microflow, error) { return nil, nil }
func (r navSecurityReader) ListMicroflows() ([]*microflows.Microflow, error)     { return r.mfs, nil }
func (r navSecurityReader) GetProjectSecurity() (*security.ProjectSecurity, error) {
	return r.sec, nil
}
func (r navSecurityReader) GetProjectSettings() (*model.ProjectSettings, error) { return nil, nil }
func (r navSecurityReader) GetNavigation() (*types.NavigationDocument, error)   { return r.nav, nil }
func (r navSecurityReader) ListPages() ([]*pages.Page, error)                   { return r.pages, nil }
func (r navSecurityReader) ListModules() ([]*model.Module, error) {
	return []*model.Module{{BaseElement: model.BaseElement{ID: "mod"}, Name: "App"}}, nil
}
func (r navSecurityReader) ListFolders() ([]*types.FolderInfo, error)   { return nil, nil }
func (r navSecurityReader) GetRawUnit(model.ID) (map[string]any, error) { return nil, nil }
func (r navSecurityReader) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	return nil, nil
}

func navPage(name string, roles ...model.ID) *pages.Page {
	return &pages.Page{ContainerID: "mod", Name: name, AllowedRoles: roles}
}

// homePageFixture: App.Home is the default home page, open to App.User only;
// App.AdminHome is open to App.Admin only. The template-style role "Leftover"
// has only System/Administration module roles — the ChipCoV6 case.
func homePageFixture(rbh ...*types.NavRoleBasedHome) navSecurityReader {
	return navSecurityReader{
		nav: &types.NavigationDocument{Profiles: []*types.NavigationProfile{{
			Name: "Responsive", Kind: "Responsive",
			HomePage:           &types.NavHomePage{Page: "App.Home"},
			RoleBasedHomePages: rbh,
		}}},
		sec: &security.ProjectSecurity{
			SecurityLevel: security.SecurityLevelProduction,
			UserRoles: []*security.UserRole{
				{Name: "Member", ModuleRoles: []string{"System.User", "App.User"}},
				{Name: "Leftover", ModuleRoles: []string{"System.User", "Administration.User"}},
			},
		},
		pages: []*pages.Page{navPage("Home", "App.User"), navPage("AdminHome", "App.Admin"), navPage("LeftoverHome", "Administration.User")},
	}
}

func homePageViolations(t *testing.T, reader navSecurityReader) []linter.Violation {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	return NewPageNavigationSecurityRule().Check(linter.NewLintContext(cat, reader))
}

// A user role none of whose module roles may open the default home page is
// reported, naming role, profile and page (ChipCoV6: 10× CE2729 in mxbuild,
// silent in lint and check --references).
func TestPageNavigationSecurity_UserRoleCannotOpenHomePage(t *testing.T) {
	vs := homePageViolations(t, homePageFixture())
	if len(vs) != 1 {
		t.Fatalf("want 1 violation for Leftover, got %d: %+v", len(vs), vs)
	}
	msg := vs[0].Message
	for _, want := range []string{"'Leftover'", "'App.Home'", "Responsive", "CE2729"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %s", msg, want)
		}
	}
	if !strings.Contains(vs[0].Suggestion, "GRANT VIEW ON PAGE App.Home") || !strings.Contains(vs[0].Suggestion, "role-based home page") {
		t.Errorf("suggestion %q should offer both fixes", vs[0].Suggestion)
	}
}

// Control: a role-based home page the role can open replaces the inaccessible default.
func TestPageNavigationSecurity_RoleBasedHomePageItCanOpen(t *testing.T) {
	vs := homePageViolations(t, homePageFixture(&types.NavRoleBasedHome{UserRole: "Leftover", Page: "App.LeftoverHome"}))
	if len(vs) != 0 {
		t.Fatalf("want no violations, got %+v", vs)
	}
}

// A role-based home page the role cannot open is reported as such.
func TestPageNavigationSecurity_RoleBasedHomePageItCannotOpen(t *testing.T) {
	vs := homePageViolations(t, homePageFixture(&types.NavRoleBasedHome{UserRole: "Leftover", Page: "App.AdminHome"}))
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "role-based home page 'App.AdminHome'") {
		t.Fatalf("want 1 role-based violation, got %+v", vs)
	}
}

// Control: when every role has access, nothing is reported.
func TestPageNavigationSecurity_AllRolesCanOpenHomePage(t *testing.T) {
	r := homePageFixture()
	r.sec.UserRoles[1].ModuleRoles = append(r.sec.UserRoles[1].ModuleRoles, "App.User")
	if vs := homePageViolations(t, r); len(vs) != 0 {
		t.Fatalf("want no violations, got %+v", vs)
	}
}

// Security off checks no roles, so neither does the rule.
func TestPageNavigationSecurity_SecurityOffSkipsRoleCheck(t *testing.T) {
	r := homePageFixture()
	r.sec.SecurityLevel = security.SecurityLevelOff
	if vs := homePageViolations(t, r); len(vs) != 0 {
		t.Fatalf("want no violations, got %+v", vs)
	}
}

// A microflow home page is checked against the microflow's allowed roles.
func TestPageNavigationSecurity_MicroflowHomePage(t *testing.T) {
	r := homePageFixture()
	r.nav.Profiles[0].HomePage = &types.NavHomePage{Microflow: "App.ACT_Home"}
	r.mfs = []*microflows.Microflow{{ContainerID: "mod", Name: "ACT_Home", AllowedModuleRoles: []model.ID{"App.User"}}}
	vs := homePageViolations(t, r)
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "cannot run default home microflow 'App.ACT_Home'") || !strings.Contains(vs[0].Message, "'Leftover'") {
		t.Fatalf("want 1 microflow violation for Leftover, got %+v", vs)
	}
}
