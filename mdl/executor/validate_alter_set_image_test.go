// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1464: once `set (Image: …)` is accepted on a static image,
// the name it writes must be resolved by `check --references` the way CREATE's
// `Image:` is — otherwise a typo passes check and fails the build as CE1613.

func storedStaticImagePage() bson.D {
	si := bson.D{
		{Key: "$Type", Value: "Forms$StaticImageViewer"},
		{Key: "Name", Value: "staticImage12"},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}}},
		{Key: "Image", Value: "MyModule.Images.cog"},
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: bson.A{int32(2), si}}},
			}},
		}},
	}
}

func staticImagePageCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "Portal")
	ic := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "Images",
		Images:      []types.Image{{Name: "cog"}, {Name: "gear"}},
	}
	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)
	deps := &countingDeps{}
	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListModulesFunc:          func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:          func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:            func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(storedStaticImagePage(), unitID, deps), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func TestAlterSetImage_KnownImagePasses(t *testing.T) {
	ctx := staticImagePageCtx(t)
	errs := checkAlterSet(t, ctx, `alter page MyModule.Portal { set (Image: 'MyModule.Images.gear') on staticImage12; }`)
	if len(errs) != 0 {
		t.Fatalf("a known image was refused: %v", errs)
	}
}

func TestAlterSetImage_UnknownImageIsReported(t *testing.T) {
	ctx := staticImagePageCtx(t)
	errs := checkAlterSet(t, ctx, `alter page MyModule.Portal { set (Image: 'MyModule.Images.nosuch') on staticImage12; }`)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	for _, want := range []string{"MyModule.Portal", "image not found", "nosuch"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

// The same-script exemption CREATE has (mendixlabs/mxcli#1419): an image the
// script itself creates resolves.
func TestAlterSetImage_ImageTheScriptCreatesPasses(t *testing.T) {
	ctx := staticImagePageCtx(t)
	errs := checkAlterSet(t, ctx, `create image collection MyModule.More;
alter page MyModule.Portal { set (Image: 'MyModule.More.x') on staticImage12; }`)
	for _, e := range errs {
		if strings.Contains(e.Error(), "image collection not found") {
			t.Fatalf("a collection the script creates was reported missing: %v", e)
		}
	}
}
