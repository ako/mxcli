// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// storedAuth reads the two authentication fields as stored.
func storedAuth(t *testing.T, b *Backend, id model.ID) (bson.A, string) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("raw unit: %v", err)
	}
	var d struct {
		AuthenticationTypes     bson.A `bson:"AuthenticationTypes"`
		AuthenticationMicroflow string `bson:"AuthenticationMicroflow"`
	}
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return d.AuthenticationTypes, d.AuthenticationMicroflow
}

// TestPublishedRestService_WritesAuthentication pins the writer to what Studio
// Pro stores for each dialog state, measured on ako/TestApp's Services module
// (mendixlabs/mxcli#1331): a marker-1 list in the order the methods were
// ticked, the microflow by qualified name, and the empty list with an empty
// microflow for "no authentication". Each update states the setting; the
// carry of an unstated one is the executor's (cmd_published_rest.go).
func TestPublishedRestService_WritesAuthentication(t *testing.T) {
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
	svc := &model.PublishedRestService{
		ContainerID: mod.ID, Name: "ZzAuth", Path: "rest/zza/v1",
		// TestApp's OrdersRestApi_MicroflowSession: Custom ticked first.
		AuthenticationTypes:     []string{"Microflow", "Session"},
		AuthenticationMicroflow: "MyFirstModule.ACT_Auth",
	}
	if err := b.CreatePublishedRestService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}

	steps := []struct {
		name      string
		types     []string
		mf        string
		wantTypes bson.A
	}{
		{"created", svc.AuthenticationTypes, svc.AuthenticationMicroflow, bson.A{int32(1), "Microflow", "Session"}},
		// OrdersRestApi_CustomOff: Custom unticked, microflow cleared.
		{"custom off", []string{"Basic", "Session"}, "", bson.A{int32(1), "Basic", "Session"}},
		// OrdersRestApi_NoAuth.
		{"none", nil, "", bson.A{int32(1)}},
	}
	for i, st := range steps {
		if i > 0 {
			svc.AuthenticationTypes, svc.AuthenticationMicroflow = st.types, st.mf
			if err := b.UpdatePublishedRestService(svc); err != nil {
				t.Fatalf("%s: update: %v", st.name, err)
			}
		}
		types, mf := storedAuth(t, b, svc.ID)
		if !reflect.DeepEqual(types, st.wantTypes) || mf != st.mf {
			t.Errorf("%s: stored %v / %q, want %v / %q", st.name, types, mf, st.wantTypes, st.mf)
		}
		all, err := b.ListPublishedRestServices()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, s := range all {
			if s.Name == "ZzAuth" && (!reflect.DeepEqual(s.AuthenticationTypes, st.types) && len(s.AuthenticationTypes)+len(st.types) > 0 || s.AuthenticationMicroflow != st.mf) {
				t.Errorf("%s: read back %v / %q, want %v / %q", st.name, s.AuthenticationTypes, s.AuthenticationMicroflow, st.types, st.mf)
			}
		}
	}
}
