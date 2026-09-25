// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1185: "two import mappings with the same name, one included
// and one excluded" — the catalog listed both and nothing told them apart.
// import_mappings / export_mappings had no Excluded column and an AUTOINCREMENT
// Id rather than the document's, and CATALOG.SOURCE rendered both twins by name,
// i.e. as the same document.

const twinModuleID = model.ID("mod-integration")

func twinBuilder(t *testing.T, reader *mock.MockBackend, describe DescribeFunc) (*Builder, *Catalog) {
	t.Helper()
	cat, err := New()
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	tx, err := cat.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	b := &Builder{
		catalog:  cat,
		reader:   reader,
		snapshot: &Snapshot{ID: "snap-1"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{twinModuleID: true},
			moduleNames:     map[model.ID]string{twinModuleID: "Integration"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx:           tx,
		sourceMode:   describe != nil,
		describeFunc: describe,
	}
	return b, cat
}

func twinMappingsReader() *mock.MockBackend {
	return &mock.MockBackend{
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) {
			return []*model.ImportMapping{
				{BaseElement: model.BaseElement{ID: "im-excluded"}, ContainerID: twinModuleID, Name: "IMM_Order", Excluded: true},
				{BaseElement: model.BaseElement{ID: "im-live"}, ContainerID: twinModuleID, Name: "IMM_Order"},
			}, nil
		},
		ListExportMappingsFunc: func() ([]*model.ExportMapping, error) {
			return []*model.ExportMapping{
				{BaseElement: model.BaseElement{ID: "em-excluded"}, ContainerID: twinModuleID, Name: "EXM_Order", Excluded: true},
				{BaseElement: model.BaseElement{ID: "em-live"}, ContainerID: twinModuleID, Name: "EXM_Order"},
			}, nil
		},
	}
}

func TestMappingTables_RecordDocumentIdAndExcluded(t *testing.T) {
	b, cat := twinBuilder(t, twinMappingsReader(), nil)
	if err := b.buildImportMappings(); err != nil {
		t.Fatalf("buildImportMappings: %v", err)
	}
	if err := b.buildExportMappings(); err != nil {
		t.Fatalf("buildExportMappings: %v", err)
	}
	if err := b.tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, tc := range []struct{ table, liveID, excludedID string }{
		{"import_mappings", "im-live", "im-excluded"},
		{"export_mappings", "em-live", "em-excluded"},
	} {
		rows, err := cat.db.Query("SELECT Id, Excluded FROM " + tc.table + " ORDER BY Id")
		if err != nil {
			t.Fatalf("%s: %v", tc.table, err)
		}
		got := map[string]bool{}
		for rows.Next() {
			var id string
			var excluded bool
			if err := rows.Scan(&id, &excluded); err != nil {
				t.Fatalf("%s scan: %v", tc.table, err)
			}
			got[id] = excluded
		}
		rows.Close()
		if len(got) != 2 {
			t.Fatalf("%s: got %d distinct ids %v, want the two document ids", tc.table, len(got), got)
		}
		if ex, ok := got[tc.excludedID]; !ok || !ex {
			t.Errorf("%s: the excluded twin must be recorded under its document id with Excluded=1; got %v", tc.table, got)
		}
		if ex, ok := got[tc.liveID]; !ok || ex {
			t.Errorf("%s: the live twin must be recorded under its document id with Excluded=0; got %v", tc.table, got)
		}
	}
}

func TestBuildSource_PinsEachTwinByDocumentId(t *testing.T) {
	var asked []string
	describe := func(objType, qn, id string) (string, error) {
		asked = append(asked, id)
		if id == "im-excluded" || id == "em-excluded" {
			return "@excluded\ncreate or modify " + qn, nil
		}
		return "create or modify " + qn, nil
	}
	b, cat := twinBuilder(t, twinMappingsReader(), describe)
	if err := b.buildSource(); err != nil {
		t.Fatalf("buildSource: %v", err)
	}
	if err := b.tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	rows, err := cat.db.Query(`SELECT ElementId, SourceText FROM source WHERE ObjectType = ? ORDER BY ElementId`, SourceImportMapping)
	if err != nil {
		t.Fatalf("query source: %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var id, text string
		if err := rows.Scan(&id, &text); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[id] = text
	}
	if len(got) != 2 {
		t.Fatalf("got %d source rows keyed by ElementId %v, want one per twin (describes asked for %v)", len(got), got, asked)
	}
	if got["im-excluded"] == got["im-live"] {
		t.Errorf("the two twins' source rows are identical:\n%s", got["im-live"])
	}
}
