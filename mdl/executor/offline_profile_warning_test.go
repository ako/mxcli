// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// twoHopPageUnit is a stored page binding an attribute across two associations —
// exactly what CE6206 rejects once an offline profile exists.
func twoHopPageUnit(t *testing.T) []byte {
	t.Helper()
	raw, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Pages$Page"},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "Forms$TextBox"},
			{Key: "AttributeRef", Value: bson.D{
				{Key: "$Type", Value: "DomainModels$AttributeRef"},
				{Key: "Attribute", Value: "Maintenance.Site.SiteName"},
				{Key: "EntityRef", Value: bson.D{
					{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
					{Key: "Steps", Value: bson.A{int32(2),
						bson.D{{Key: "$Type", Value: "DomainModels$EntityRefStep"}, {Key: "Association", Value: "Maintenance.Req_Asset"}},
						bson.D{{Key: "$Type", Value: "DomainModels$EntityRefStep"}, {Key: "Association", Value: "Maintenance.Asset_Site"}},
					}},
				}},
			}},
		}}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// navBackendWithTwoHopPage is a project holding exactly one page, which binds an
// attribute across two associations, and a navigation document with no profiles
// at all — so any profile the statement names has to be created.
func navBackendWithTwoHopPage(t *testing.T) *mock.MockBackend {
	t.Helper()
	page := &pages.Page{Name: "Request_Overview"}
	page.ID = model.ID("page-1")
	unit := twoHopPageUnit(t)

	return &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		GetNavigationFunc:        func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil },
		AddNavigationProfileFunc: func(model.ID, string) error { return nil },
		UpdateNavigationProfileFunc: func(model.ID, string, types.NavigationProfileSpec) error {
			return nil
		},
		ListPagesFunc:       func() ([]*pages.Page, error) { return []*pages.Page{page}, nil },
		ListSnippetsFunc:    func() ([]*pages.Snippet, error) { return nil, nil },
		GetRawUnitBytesFunc: func(model.ID) ([]byte, error) { return unit, nil },
	}
}

func TestCreatingAnOfflineProfileWarnsAboutMultiStepPaths(t *testing.T) {
	// ako/mxcli-maintenance: the statement succeeds and the project acquires build
	// errors in pages it never mentioned. The pages were valid; adding the offline
	// profile is what invalidated them, and mxcli used to say nothing at all.
	mb := navBackendWithTwoHopPage(t)
	ctx, buf := newMockCtx(t, withBackend(mb))

	stmt := &ast.AlterNavigationStmt{ProfileName: "TabletOffline"}
	assertNoError(t, execAlterNavigation(ctx, stmt))

	out := buf.String()
	assertContainsStr(t, out, "created")
	assertContainsStr(t, out, "CE6206")
	assertContainsStr(t, out, "Maintenance.Site.SiteName")
}

func TestCreatingAnOnlineProfileSaysNothingAboutOfflinePaths(t *testing.T) {
	// The control, and the one that matters most: the SAME project, the SAME
	// two-hop page, a Tablet profile instead of a TabletOffline one. A multi-step
	// path is perfectly legal under an online profile, so a warning here would be
	// noise on every navigation statement anyone writes.
	mb := navBackendWithTwoHopPage(t)
	ctx, buf := newMockCtx(t, withBackend(mb))

	stmt := &ast.AlterNavigationStmt{ProfileName: "Tablet"}
	assertNoError(t, execAlterNavigation(ctx, stmt))

	out := buf.String()
	assertContainsStr(t, out, "created")
	if strings.Contains(out, "CE6206") {
		t.Errorf("an online profile warned about offline paths:\n%s", out)
	}
}

func TestReplacingAnExistingOfflineProfileDoesNotWarn(t *testing.T) {
	// The warning is about the moment the constraint ARRIVES. Re-running a script
	// against a project that already has the profile changes nothing about which
	// pages are legal, so repeating the warning on every run would train people to
	// scroll past it.
	mb := navBackendWithTwoHopPage(t)
	mb.GetNavigationFunc = func() (*types.NavigationDocument, error) {
		return &types.NavigationDocument{
			Profiles: []*types.NavigationProfile{{Name: "TabletOffline", Kind: "TabletOffline"}},
		}, nil
	}
	mb.AddNavigationProfileFunc = func(model.ID, string) error {
		t.Error("AddNavigationProfile called for a profile that already exists")
		return nil
	}
	ctx, buf := newMockCtx(t, withBackend(mb))

	assertNoError(t, execAlterNavigation(ctx, &ast.AlterNavigationStmt{ProfileName: "TabletOffline"}))

	out := buf.String()
	assertContainsStr(t, out, "updated")
	if strings.Contains(out, "CE6206") {
		t.Errorf("a replace warned as if it had created the profile:\n%s", out)
	}
}

func TestOfflineProfileWarningIsSilentOnACleanProject(t *testing.T) {
	// A project with no multi-step binding gets no warning — otherwise the message
	// would be advice attached to every offline profile ever created rather than a
	// report of something found.
	mb := navBackendWithTwoHopPage(t)
	clean, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Pages$Page"},
		{Key: "Widgets", Value: bson.A{int32(2)}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	mb.GetRawUnitBytesFunc = func(model.ID) ([]byte, error) { return clean, nil }
	ctx, buf := newMockCtx(t, withBackend(mb))

	assertNoError(t, execAlterNavigation(ctx, &ast.AlterNavigationStmt{ProfileName: "PhoneOffline"}))
	if strings.Contains(buf.String(), "CE6206") {
		t.Errorf("warned about a project with no multi-step path:\n%s", buf.String())
	}
}
