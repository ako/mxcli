// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// upstream #872. An association's line anchors (ParentConnection /
// ChildConnection — where the connector attaches to the entity boxes in the
// domain model editor) were hardcoded to mxcli's own "0;50" / "100;50" on every
// write, and never read back. Because assocToGen rebuilds the whole element, any
// association write destroyed them — including a documentation-only
// `alter association … set comment`.
//
// Measured on a blank Mendix 11.13 app, whose Studio-Pro-authored
// Administration.AccountPasswordData_Account stores 0;54 / 100;54:
//
//	build     after `alter association … set comment`
//	pre-fix   0;50 / 100;50   (destroyed)
//	fixed     0;54 / 100;54   (preserved)
//
// This test drives the same read → re-persist → reopen cycle that
// CREATE OR MODIFY ASSOCIATION and ALTER ASSOCIATION both take.
func TestUpdateDomainModel_PreservesConnectionPoints(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	parent := &domainmodel.Entity{Name: "ZzAnchorParent", Persistable: true}
	child := &domainmodel.Entity{Name: "ZzAnchorChild", Persistable: true}
	if err := b.CreateEntity(dm.ID, parent); err != nil {
		t.Fatalf("CreateEntity parent: %v", err)
	}
	if err := b.CreateEntity(dm.ID, child); err != nil {
		t.Fatalf("CreateEntity child: %v", err)
	}
	// Anchors the developer dragged in Studio Pro: bottom-centre of the FROM box
	// to top-centre of the TO box. Deliberately nothing like mxcli's defaults.
	tuned := &domainmodel.Association{
		Name: "ZzAnchorChild_ZzAnchorParent", ParentID: child.ID, ChildID: parent.ID,
		Type: "Reference", Owner: "Default",
		ParentConnection: &model.Point{X: 50, Y: 100},
		ChildConnection:  &model.Point{X: 50, Y: 0},
	}
	if err := b.CreateAssociation(dm.ID, tuned); err != nil {
		t.Fatalf("CreateAssociation: %v", err)
	}

	find := func(t *testing.T, backend *Backend, why string) *domainmodel.Association {
		t.Helper()
		got, err := backend.GetDomainModel(mod.ID)
		if err != nil {
			t.Fatalf("GetDomainModel (%s): %v", why, err)
		}
		for _, a := range got.Associations {
			if a.Name == "ZzAnchorChild_ZzAnchorParent" {
				return a
			}
		}
		t.Fatalf("association missing %s", why)
		return nil
	}
	assertTuned := func(t *testing.T, a *domainmodel.Association, why string) {
		t.Helper()
		if a.ParentConnection == nil || a.ChildConnection == nil {
			t.Fatalf("%s: anchors read back nil (%v, %v) — a field the reader drops is "+
				"a field the next write destroys", why, a.ParentConnection, a.ChildConnection)
		}
		if *a.ParentConnection != (model.Point{X: 50, Y: 100}) || *a.ChildConnection != (model.Point{X: 50, Y: 0}) {
			t.Fatalf("%s: anchors = %+v / %+v, want {50 100} / {50 0} — reset to mxcli's defaults",
				why, *a.ParentConnection, *a.ChildConnection)
		}
	}

	assertTuned(t, find(t, b, "on read"), "on read")

	// Re-persist the whole domain model unchanged — what ALTER ASSOCIATION and
	// CREATE OR MODIFY ASSOCIATION both do.
	dm2, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel(2): %v", err)
	}
	if err := b.UpdateDomainModel(dm2); err != nil {
		t.Fatalf("UpdateDomainModel: %v", err)
	}

	b3 := New()
	if err := b3.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b3.Disconnect() })
	assertTuned(t, find(t, b3, "after UpdateDomainModel"), "after UpdateDomainModel")
}

// An association created without anchors gets mxcli's defaults — the writer must
// not emit an empty or zero pair, which would move every new connector to the
// entity box's top-left corner.
func TestCreateAssociation_DefaultsConnectionPoints(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, _ := b.GetModuleByName("MyFirstModule")
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	parent := &domainmodel.Entity{Name: "ZzPlainParent", Persistable: true}
	child := &domainmodel.Entity{Name: "ZzPlainChild", Persistable: true}
	if err := b.CreateEntity(dm.ID, parent); err != nil {
		t.Fatalf("CreateEntity parent: %v", err)
	}
	if err := b.CreateEntity(dm.ID, child); err != nil {
		t.Fatalf("CreateEntity child: %v", err)
	}
	if err := b.CreateAssociation(dm.ID, &domainmodel.Association{
		Name: "ZzPlainChild_ZzPlainParent", ParentID: child.ID, ChildID: parent.ID,
		Type: "Reference", Owner: "Default",
	}); err != nil {
		t.Fatalf("CreateAssociation: %v", err)
	}

	got, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	for _, a := range got.Associations {
		if a.Name != "ZzPlainChild_ZzPlainParent" {
			continue
		}
		p := domainmodel.FormatConnectionPoint(a.ParentConnection, "")
		c := domainmodel.FormatConnectionPoint(a.ChildConnection, "")
		if p != domainmodel.DefaultParentConnection || c != domainmodel.DefaultChildConnection {
			t.Fatalf("new association anchors = %q / %q, want %q / %q", p, c,
				domainmodel.DefaultParentConnection, domainmodel.DefaultChildConnection)
		}
		return
	}
	t.Fatal("association missing after create")
}
