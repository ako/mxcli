// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#795: DESCRIBE PAGE drops a datagrid's microflow DataSource, so
// re-applying the describe output recreates the grid with no data source at all.
//
// Root cause: a Forms$MicroflowSource stores the microflow name in the nested
// Forms$MicroflowSettings ("MicroflowSettings" → "Microflow"), which is what the
// write path emits and what Studio Pro stores. The DataGrid2 and Gallery readers
// looked up a top-level "Microflow" key instead, got "", and returned no data
// source — while the describe formatter's `case "microflow"` branch was correct all
// along. A database source was unaffected, which is why only microflow sources
// silently lost their binding.
package executor

import "testing"

// flowSourceWidget builds a minimal pluggable-widget map whose datasource is the
// given raw source document, mirroring the on-disk BSON.
func flowSourceWidget(ds map[string]any) map[string]any {
	return map[string]any{
		"Object": map[string]any{
			"Properties": []any{
				int32(2), // BSON non-empty-array version marker
				map[string]any{
					"Value": map[string]any{"DataSource": ds},
				},
			},
		},
	}
}

// nestedMicroflowSource is the shape Studio Pro and the codec engine write.
func nestedMicroflowSource(name string) map[string]any {
	return map[string]any{
		"$Type":            "Forms$MicroflowSource",
		"ForceFullObjects": false,
		"MicroflowSettings": map[string]any{
			"$Type":     "Forms$MicroflowSettings",
			"Microflow": name,
		},
	}
}

// flatMicroflowSource is the legacy shape: the name directly on the source.
func flatMicroflowSource(name string) map[string]any {
	return map[string]any{
		"$Type":     "Forms$MicroflowSource",
		"Microflow": name,
	}
}

func nestedNanoflowSource(name string) map[string]any {
	return map[string]any{
		"$Type": "Forms$NanoflowSource",
		"NanoflowSettings": map[string]any{
			"$Type":    "Forms$NanoflowSettings",
			"Nanoflow": name,
		},
	}
}

// extractors is every reader that turns a raw datasource document into a
// rawDataSource. They historically disagreed about where the microflow name lives;
// the table keeps them honest with each other.
var extractors = []struct {
	name string
	fn   func(*ExecContext, map[string]any) *rawDataSource
}{
	{"datagrid2", extractDataGrid2DataSource},
	{"gallery", extractGalleryDataSource},
}

func TestPluggableDataSource_MicroflowNestedSettings(t *testing.T) {
	const mf = "MyModule.DBG_ListBucketObjects"
	for _, ex := range extractors {
		t.Run(ex.name, func(t *testing.T) {
			ds := ex.fn(nil, flowSourceWidget(nestedMicroflowSource(mf)))
			if ds == nil {
				t.Fatal("microflow datasource dropped: got nil")
			}
			if ds.Type != "microflow" {
				t.Errorf("Type = %q, want microflow", ds.Type)
			}
			if ds.Reference != mf {
				t.Errorf("Reference = %q, want %q", ds.Reference, mf)
			}
		})
	}
}

// TestPluggableDataSource_MicroflowFlatLegacy keeps the legacy top-level shape
// readable so files written before the nested form still round-trip.
func TestPluggableDataSource_MicroflowFlatLegacy(t *testing.T) {
	const mf = "MyModule.Legacy"
	for _, ex := range extractors {
		t.Run(ex.name, func(t *testing.T) {
			ds := ex.fn(nil, flowSourceWidget(flatMicroflowSource(mf)))
			if ds == nil || ds.Type != "microflow" || ds.Reference != mf {
				t.Fatalf("legacy flat microflow source not read: %+v", ds)
			}
		})
	}
}

// TestPluggableDataSource_Nanoflow covers the sibling source type, which the
// DataGrid2 and Gallery readers did not handle at all.
func TestPluggableDataSource_Nanoflow(t *testing.T) {
	const nf = "MyModule.NF_ListBuckets"
	for _, ex := range extractors {
		t.Run(ex.name, func(t *testing.T) {
			ds := ex.fn(nil, flowSourceWidget(nestedNanoflowSource(nf)))
			if ds == nil {
				t.Fatal("nanoflow datasource dropped: got nil")
			}
			if ds.Type != "nanoflow" {
				t.Errorf("Type = %q, want nanoflow", ds.Type)
			}
			if ds.Reference != nf {
				t.Errorf("Reference = %q, want %q", ds.Reference, nf)
			}
		})
	}
}

// TestFlowSourceRef_Helpers pins the shared lookup the readers delegate to.
func TestFlowSourceRef_Helpers(t *testing.T) {
	if got := microflowSourceRef(nestedMicroflowSource("A.B")); got != "A.B" {
		t.Errorf("microflowSourceRef(nested) = %q", got)
	}
	if got := microflowSourceRef(flatMicroflowSource("A.B")); got != "A.B" {
		t.Errorf("microflowSourceRef(flat) = %q", got)
	}
	if got := microflowSourceRef(map[string]any{"$Type": "Forms$MicroflowSource"}); got != "" {
		t.Errorf("microflowSourceRef(empty) = %q, want empty", got)
	}
	// A non-map MicroflowSettings must not panic.
	if got := microflowSourceRef(map[string]any{"MicroflowSettings": "junk"}); got != "" {
		t.Errorf("microflowSourceRef(junk) = %q, want empty", got)
	}
	if got := nanoflowSourceRef(nestedNanoflowSource("A.B")); got != "A.B" {
		t.Errorf("nanoflowSourceRef(nested) = %q", got)
	}
	if got := nanoflowSourceRef(map[string]any{"Nanoflow": "A.B"}); got != "A.B" {
		t.Errorf("nanoflowSourceRef(flat) = %q", got)
	}
	if got := nanoflowSourceRef(map[string]any{"NanoflowSettings": 42}); got != "" {
		t.Errorf("nanoflowSourceRef(junk) = %q, want empty", got)
	}
}
