// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"testing"
)

// ako/mxcli#1789: UnitByID reads one unit where a lookup by ID used to list
// every unit of the type. It must find what the listing finds, on both storage
// formats, and nothing for an ID the project does not hold.
func TestUnitByID_AgreesWithTheListing(t *testing.T) {
	v2, _ := seedProject(t, 6)
	for _, fixture := range []struct {
		name string
		r    *Reader
	}{
		{"v1", newTypedListingFixture(t, []string{"Microflows$Microflow", "Forms$Page", "Projects$Folder", "Microflows$Microflow"}).r},
		{"v2", v2.reader()},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			all, err := fixture.r.listUnitsByType("")
			if err != nil {
				t.Fatal(err)
			}
			if len(all) < 4 {
				t.Fatalf("control: the fixture lists %d units", len(all))
			}
			for _, want := range all {
				got, ok, err := fixture.r.UnitByID(want.ID)
				if err != nil || !ok {
					t.Fatalf("UnitByID(%s) = ok %v, err %v", want.ID, ok, err)
				}
				if got.ID != want.ID || got.ContainerID != want.ContainerID || got.Type != want.Type || !bytes.Equal(got.Contents, want.Contents) {
					t.Errorf("UnitByID(%s) = {%s %s %s %d bytes}, the listing has {%s %s %s %d bytes}",
						want.ID, got.ID, got.ContainerID, got.Type, len(got.Contents),
						want.ID, want.ContainerID, want.Type, len(want.Contents))
				}
			}
			if _, ok, err := fixture.r.UnitByID("99999999-9999-9999-9999-999999999999"); ok || err != nil {
				t.Errorf("UnitByID(unknown) = ok %v, err %v; want not found", ok, err)
			}
		})
	}
}

// On v2 the lookup opens the one file.
func TestUnitByID_V2OpensOneFile(t *testing.T) {
	p, _ := seedProject(t, 40)
	r := p.reader()
	units, err := r.listUnitTypes() // the unit index, as any earlier call builds it
	if err != nil {
		t.Fatal(err)
	}
	before := r.UnitFileReads()
	if _, ok, err := r.UnitByID(units[len(units)-1].ID); !ok || err != nil {
		t.Fatalf("UnitByID = ok %v, err %v", ok, err)
	}
	if reads := r.UnitFileReads() - before; reads != 1 {
		t.Errorf("UnitByID opened %d unit files of %d; it needs one", reads, len(units))
	}
}
