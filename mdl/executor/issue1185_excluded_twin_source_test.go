// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1185, reported against an app with "two import mappings with
// the same name, one included and one excluded": the catalog showed both but
// nothing told them apart, and "the excluded mapping source doesn't follow the
// established @excluded prefix convention".
//
// Two defects, both pinned here. DESCRIBE of an import/export mapping never
// printed @excluded. And the source build describes every DOCUMENT but looked
// each one up by NAME, so both twins were rendered as whichever one the lookup
// preferred — the same text twice.

// twinImportMappings returns a live and an excluded import mapping sharing one
// qualified name. The excluded one comes FIRST, which is the order a by-name
// lookup that takes the first match gets wrong.
func twinImportMappings(t *testing.T) (*ExecContext, *model.ImportMapping, *model.ImportMapping) {
	t.Helper()
	mod := mkModule("Integration")
	excluded := &model.ImportMapping{
		BaseElement:   model.BaseElement{ID: nextID("im")},
		ContainerID:   mod.ID,
		Name:          "IMM_Order",
		Excluded:      true,
		JsonStructure: "Integration.JSON_OrderOld",
	}
	live := &model.ImportMapping{
		BaseElement:   model.BaseElement{ID: nextID("im")},
		ContainerID:   mod.ID,
		Name:          "IMM_Order",
		JsonStructure: "Integration.JSON_Order",
	}
	all := []*model.ImportMapping{excluded, live}

	h := mkHierarchy(mod)
	withContainer(h, mod.ID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) { return all, nil },
		// The backend's by-name lookup prefers the live twin.
		GetImportMappingByQualifiedNameFunc: func(_, _ string) (*model.ImportMapping, error) { return live, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, excluded, live
}

func TestDescribeImportMapping_ExcludedPrintsAnnotation(t *testing.T) {
	ctx, excluded, _ := twinImportMappings(t)

	text, err := captureDescribeParallel(ctx, catalog.SourceImportMapping, "Integration.IMM_Order", excluded.ID)
	assertNoError(t, err)
	if !strings.HasPrefix(text, "@excluded\ncreate or modify import mapping Integration.IMM_Order") {
		t.Errorf("an excluded mapping must describe with the @excluded prefix a microflow carries; got:\n%s", text)
	}
}

func TestSourceDescribe_ImportMappingTwinsAreDistinct(t *testing.T) {
	ctx, excluded, live := twinImportMappings(t)

	exText, err := captureDescribeParallel(ctx, catalog.SourceImportMapping, "Integration.IMM_Order", excluded.ID)
	assertNoError(t, err)
	liveText, err := captureDescribeParallel(ctx, catalog.SourceImportMapping, "Integration.IMM_Order", live.ID)
	assertNoError(t, err)

	if exText == liveText {
		t.Fatalf("both twins describe identically — the source table cannot tell them apart:\n%s", exText)
	}
	assertContainsStr(t, exText, "JSON_OrderOld")
	if strings.Contains(liveText, "@excluded") || !strings.Contains(liveText, "Integration.JSON_Order\n") {
		t.Errorf("the live twin must describe as itself, unannotated; got:\n%s", liveText)
	}

	// An interactive DESCRIBE pins nothing and shows the live one.
	byName, err := captureDescribeParallel(ctx, catalog.SourceImportMapping, "Integration.IMM_Order", "")
	assertNoError(t, err)
	if byName != liveText {
		t.Errorf("an unpinned describe must show the live twin; got:\n%s", byName)
	}
}

func TestSourceDescribe_ExportMappingTwinsAreDistinct(t *testing.T) {
	mod := mkModule("Integration")
	excluded := &model.ExportMapping{
		BaseElement:   model.BaseElement{ID: nextID("em")},
		ContainerID:   mod.ID,
		Name:          "EXM_Order",
		Excluded:      true,
		JsonStructure: "Integration.JSON_OrderOld",
	}
	live := &model.ExportMapping{
		BaseElement:   model.BaseElement{ID: nextID("em")},
		ContainerID:   mod.ID,
		Name:          "EXM_Order",
		JsonStructure: "Integration.JSON_Order",
	}
	h := mkHierarchy(mod)
	withContainer(h, mod.ID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListExportMappingsFunc: func() ([]*model.ExportMapping, error) {
			return []*model.ExportMapping{excluded, live}, nil
		},
		GetExportMappingByQualifiedNameFunc: func(_, _ string) (*model.ExportMapping, error) { return live, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	exText, err := captureDescribeParallel(ctx, catalog.SourceExportMapping, "Integration.EXM_Order", excluded.ID)
	assertNoError(t, err)
	liveText, err := captureDescribeParallel(ctx, catalog.SourceExportMapping, "Integration.EXM_Order", live.ID)
	assertNoError(t, err)

	if !strings.HasPrefix(exText, "@excluded\n") {
		t.Errorf("excluded export mapping must carry @excluded; got:\n%s", exText)
	}
	if strings.Contains(liveText, "@excluded") {
		t.Errorf("live export mapping must not carry @excluded; got:\n%s", liveText)
	}
}

// Microflows had the same source defect: describeMicroflow always picked the
// live twin, so the excluded one's row was a copy of the live one's.
func TestSourceDescribe_MicroflowTwinsAreDistinct(t *testing.T) {
	mod := mkModule("MyModule")
	excluded := &microflows.Microflow{
		BaseElement: model.BaseElement{ID: nextID("mf")},
		ContainerID: mod.ID,
		Name:        "ACT_Calc",
		Excluded:    true,
	}
	live := &microflows.Microflow{
		BaseElement: model.BaseElement{ID: nextID("mf")},
		ContainerID: mod.ID,
		Name:        "ACT_Calc",
	}
	h := mkHierarchy(mod)
	withContainer(h, mod.ID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{excluded, live}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	exText, err := captureDescribeParallel(ctx, catalog.SourceMicroflow, "MyModule.ACT_Calc", excluded.ID)
	assertNoError(t, err)
	liveText, err := captureDescribeParallel(ctx, catalog.SourceMicroflow, "MyModule.ACT_Calc", live.ID)
	assertNoError(t, err)

	if !strings.HasPrefix(exText, "@excluded\n") {
		t.Errorf("the excluded twin's source must carry @excluded; got:\n%s", exText)
	}
	if strings.Contains(liveText, "@excluded") {
		t.Errorf("the live twin's source must not carry @excluded; got:\n%s", liveText)
	}
}
