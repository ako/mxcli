// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"
)

// ako/mxcli#1789: GetMicroflow(id) listed every microflow of the project and
// kept the one with that $ID. The catalog's source build reads each microflow
// it describes by ID, so the build read every microflow once per microflow.
// A unit's ID is its document's $ID, so the lookup reads that unit alone.

func TestGetMicroflow_ReadsOneUnitFile(t *testing.T) {
	b := New()
	if err := b.Connect(copyPedApp(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	all, err := b.ListMicroflows()
	if err != nil {
		t.Fatalf("list microflows: %v", err)
	}
	if len(all) < 4 {
		t.Fatalf("control: the fixture has %d microflows; the bound below would prove nothing", len(all))
	}
	before := b.UnitFileReads()
	if _, err := b.GetMicroflow(all[len(all)-1].ID); err != nil {
		t.Fatalf("GetMicroflow: %v", err)
	}
	if reads := b.UnitFileReads() - before; reads != 1 {
		t.Errorf("GetMicroflow of one microflow read %d unit files; the project has %d microflows, and it needs one", reads, len(all))
	}
}

// The narrowing must not change the answer: every microflow is what the
// listing says it is, and an ID that names no microflow — another document's,
// or none at all — finds nothing. No MPR v1 fixture holds a microflow; v1's
// by-ID read is pinned in modelsdk/mpr (TestUnitByID_AgreesWithTheListing).
func TestGetMicroflow_AgreesWithTheListing(t *testing.T) {
	for _, fixture := range []struct {
		name string
		path func(*testing.T) string
	}{
		{"pedapp", copyPedApp},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			b := New()
			if err := b.Connect(fixture.path(t)); err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(func() { _ = b.Disconnect() })
			all, err := b.ListMicroflows()
			if err != nil {
				t.Fatalf("list microflows: %v", err)
			}
			if len(all) == 0 {
				t.Fatalf("control: the fixture has no microflow")
			}
			for _, want := range all {
				got, err := b.GetMicroflow(want.ID)
				if err != nil {
					t.Fatalf("GetMicroflow(%s): %v", want.ID, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("GetMicroflow(%s) differs from the listing's %s", want.ID, want.Name)
				}
			}
			mods, err := b.ListModules()
			if err != nil || len(mods) == 0 {
				t.Fatalf("list modules: %v", err)
			}
			if got, err := b.GetMicroflow(mods[0].ID); err != nil || got != nil {
				t.Errorf("GetMicroflow(a module's ID) = %v, %v; want nil, nil", got, err)
			}
			if got, err := b.GetMicroflow("00000000-0000-0000-0000-000000000000"); err != nil || got != nil {
				t.Errorf("GetMicroflow(unknown ID) = %v, %v; want nil, nil", got, err)
			}
		})
	}
}
