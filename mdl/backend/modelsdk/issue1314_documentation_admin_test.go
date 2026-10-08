// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// storedString reads a top-level string property from a unit as stored.
func storedString(t *testing.T, b *Backend, id model.ID, key string) string {
	t.Helper()
	raw, err := b.GetRawUnitBytes(id)
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, e := range d {
		if e.Key == key {
			s, _ := e.Value.(string)
			return s
		}
	}
	t.Fatalf("unit %s has no %s property", id, key)
	return ""
}

// mendixlabs/mxcli#1314: the domain model's Documentation is written on the
// DomainModels$DomainModel unit — the property #1269 reads — and survives a
// reconnect.
func TestSetDomainModelDocumentation(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	dms, err := b.ListDomainModels()
	if err != nil || len(dms) == 0 {
		t.Fatalf("ListDomainModels: %v (%d)", err, len(dms))
	}
	var dmID model.ID
	for _, dm := range dms {
		if dm.Documentation == "" && dm.ContainerID != "" {
			if _, err := b.loadDomainModelGen(dm.ID); err == nil {
				dmID = dm.ID
				break
			}
		}
	}
	if dmID == "" {
		t.Fatal("fixture has no stored, undocumented domain model")
	}
	// Control: the fixture starts undocumented.
	if got := storedString(t, b, dmID, "Documentation"); got != "" {
		t.Fatalf("fixture domain model already documented: %q", got)
	}

	if err := b.SetDomainModelDocumentation(dmID, "Sales data"); err != nil {
		t.Fatalf("SetDomainModelDocumentation: %v", err)
	}
	_ = b.Disconnect()

	b = New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	if got := storedString(t, b, dmID, "Documentation"); got != "Sales data" {
		t.Errorf("stored Documentation = %q, want %q", got, "Sales data")
	}
	dm, err := b.GetDomainModelByID(dmID)
	if err != nil || dm.Documentation != "Sales data" {
		t.Errorf("read back %+v (%v), want Documentation %q", dm, err, "Sales data")
	}
}

// mendixlabs/mxcli#1314: AdminUserName is written on Security$ProjectSecurity
// and nothing else about the administrator changes.
func TestSetProjectAdminUserName(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ps, err := b.GetProjectSecurity()
	if err != nil {
		t.Fatalf("GetProjectSecurity: %v", err)
	}
	// Control: the fixture has the default administrator name.
	before := storedString(t, b, ps.ID, "AdminUserName")
	if before == "" || before == "appadmin" {
		t.Fatalf("fixture AdminUserName = %q; the control does not hold", before)
	}
	pwBefore := storedString(t, b, ps.ID, "AdminPassword")

	if err := b.SetProjectAdminUserName(ps.ID, "appadmin"); err != nil {
		t.Fatalf("SetProjectAdminUserName: %v", err)
	}
	_ = b.Disconnect()

	b = New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	if got := storedString(t, b, ps.ID, "AdminUserName"); got != "appadmin" {
		t.Errorf("stored AdminUserName = %q, want appadmin", got)
	}
	if got := storedString(t, b, ps.ID, "AdminPassword"); got != pwBefore {
		t.Errorf("AdminPassword changed: %q -> %q", pwBefore, got)
	}
	got, err := b.GetProjectSecurity()
	if err != nil || got.AdminUserName != "appadmin" {
		t.Errorf("read back AdminUserName %q (%v), want appadmin", got.AdminUserName, err)
	}
}
