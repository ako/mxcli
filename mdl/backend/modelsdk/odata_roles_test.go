// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mxcli-formula1 §41: the §26 role-grant fix landed in sdk/mpr/writer_odata.go
// only. This writer had ZERO references to AllowedModuleRoles, so a
// `create or modify odata service` through the modelsdk engine still revoked the
// service's access however carefully the other writer was fixed:
//
//	grant, then mx check              → 0 errors
//	create-or-modify, no grant clause → CE0307 "At least one allowed role must be selected"
//
// The document is serialized wholesale, so a field the serializer omits is not
// left alone — it is deleted.
func TestPublishedODataServiceToGen_WritesTheRoleGrants(t *testing.T) {
	g := publishedODataServiceToGen(&model.PublishedODataService{
		Name:               "ZzApi",
		AllowedModuleRoles: []string{"MyFirstModule.User", "MyFirstModule.Admin"},
	}, true)
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	val, lookupErr := bson.Raw(raw).LookupErr("AllowedModuleRoles")
	if lookupErr != nil {
		t.Fatal("AllowedModuleRoles is absent; a modify through this engine revokes the grants (CE0307)")
	}
	arr, ok := val.ArrayOK()
	if !ok {
		t.Fatalf("AllowedModuleRoles is not an array: %v", val)
	}
	vals, err := arr.Values()
	if err != nil {
		t.Fatalf("reading the array: %v", err)
	}
	// Marker 1, then the qualified names. The marker is not decoration: the
	// legacy writer documents that a role list without its version prefix is
	// read as stale access control.
	if len(vals) != 3 {
		t.Fatalf("got %d entries, want marker + 2 roles: %v", len(vals), vals)
	}
	if m, ok := vals[0].Int32OK(); !ok || m != 1 {
		t.Errorf("version marker = %v, want int32(1) — the marker AllowedModuleRoles uses", vals[0])
	}
	for i, want := range []string{"MyFirstModule.User", "MyFirstModule.Admin"} {
		if got, ok := vals[i+1].StringValueOK(); !ok || got != want {
			t.Errorf("role %d = %v, want %q", i, vals[i+1], want)
		}
	}
}

// A service with no grants must still write the key. Omitting it entirely is
// what deletes the stored grants on the next modify; an explicit empty list is
// the honest encoding of "none", and matches what the legacy writer emits.
func TestPublishedODataServiceToGen_WritesAnEmptyRoleListNotNothing(t *testing.T) {
	g := publishedODataServiceToGen(&model.PublishedODataService{Name: "ZzApi"}, true)
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	val, lookupErr := bson.Raw(raw).LookupErr("AllowedModuleRoles")
	if lookupErr != nil {
		t.Fatal("the key must be written even with no grants")
	}
	arr, _ := val.ArrayOK()
	vals, _ := arr.Values()
	if len(vals) != 1 {
		t.Fatalf("got %d entries, want just the marker: %v", len(vals), vals)
	}
}

// The engine-parity claim, asserted rather than assumed. The two writers
// serialize the same document; a field one carries and the other drops means
// MXCLI_ENGINE silently changes whether a service keeps its access — and
// nothing in the suite noticed for a whole release.
func TestPublishedODataServiceToGen_RoleEncodingMatchesTheMprEngine(t *testing.T) {
	roles := []string{"MyFirstModule.User"}
	g := publishedODataServiceToGen(&model.PublishedODataService{Name: "ZzApi", AllowedModuleRoles: roles}, true)
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, lookupErr := bson.Raw(raw).LookupErr("AllowedModuleRoles")
	if lookupErr != nil {
		t.Fatal("AllowedModuleRoles is absent")
	}
	// The legacy writer builds exactly bson.A{int32(1), "MyFirstModule.User"}.
	wantDoc, err := bson.Marshal(bson.D{{Key: "AllowedModuleRoles",
		Value: bson.A{int32(1), "MyFirstModule.User"}}})
	if err != nil {
		t.Fatalf("marshalling the legacy shape: %v", err)
	}
	want := bson.Raw(wantDoc).Lookup("AllowedModuleRoles")
	if !got.Equal(want) {
		t.Errorf("modelsdk encodes %v; the mpr engine encodes %v", got, want)
	}
}

// The reproduction from the report, through the backend rather than the
// serializer: grant, then modify without restating the grant. The read path
// already carried the roles; only the write dropped them, so a unit test on the
// serializer alone would not have proven the cycle closes.
func TestPublishedODataService_ModifyKeepsTheGrants(t *testing.T) {
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
	svc := &model.PublishedODataService{
		ContainerID: mod.ID, Name: "RolesApi", Path: "odata/roles/",
		Version: "1.0.0", ODataVersion: "OData4", Namespace: "MyFirstModule.Roles",
		AllowedModuleRoles: []string{"MyFirstModule.User"},
	}
	if err := b.CreatePublishedODataService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Reconnect so the modify reads the stored document rather than reusing the
	// in-memory one — that read-modify-write is where the grants were lost.
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	stored := findServiceByName(t, b2, "RolesApi")
	if len(stored.AllowedModuleRoles) != 1 {
		t.Fatalf("the grant did not survive CREATE: %v", stored.AllowedModuleRoles)
	}
	stored.Summary = "modified" // any edit; the grant clause is NOT restated
	if err := b2.UpdatePublishedODataService(stored); err != nil {
		t.Fatalf("update: %v", err)
	}

	b3 := New()
	if err := b3.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b3.Disconnect() })
	after := findServiceByName(t, b3, "RolesApi")
	if len(after.AllowedModuleRoles) != 1 || after.AllowedModuleRoles[0] != "MyFirstModule.User" {
		t.Errorf("modify revoked the grants: %v — the next build fails CE0307", after.AllowedModuleRoles)
	}
}

func findServiceByName(t *testing.T, b *Backend, name string) *model.PublishedODataService {
	t.Helper()
	svcs, err := b.ListPublishedODataServices()
	if err != nil {
		t.Fatalf("ListPublishedODataServices: %v", err)
	}
	for _, s := range svcs {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("service %q not found", name)
	return nil
}
