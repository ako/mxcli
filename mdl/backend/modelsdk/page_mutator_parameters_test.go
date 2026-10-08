// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1234: `alter page … { add parameters … }` was a parse error,
// so a parameter could only be added by CREATE OR REPLACE. These go through the
// real mutator and the real writer, and read the page back from storage.

func openPageMutator(t *testing.T, name string) (*Backend, backend.PageMutator, model.ID) {
	t.Helper()
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	ps, err := b.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	var id model.ID
	for _, p := range ps {
		if p.Name == name {
			id = p.ID
		}
	}
	if id == "" {
		t.Fatalf("fixture has no %s page", name)
	}
	m, err := b.OpenPageForMutation(id)
	if err != nil {
		t.Fatalf("OpenPageForMutation: %v", err)
	}
	return b, m, id
}

func storedParameters(t *testing.T, b *Backend, id model.ID) []bson.D {
	t.Helper()
	raw, err := b.GetRawUnitBytes(id)
	if err != nil {
		t.Fatalf("read stored page: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var out []bson.D
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(d, "Parameters")) {
		out = append(out, p.(bson.D))
	}
	return out
}

func sortedParamKeys(d bson.D) []string {
	var keys []string
	for _, e := range d {
		keys = append(keys, e.Key)
	}
	slices.Sort(keys)
	return keys
}

func TestAlterPageAddParameters_Persisted(t *testing.T) {
	b, m, id := openPageMutator(t, "Account_Overview")
	if got := storedParameters(t, b, id); len(got) != 0 {
		t.Fatalf("fixture changed: Account_Overview already has %d parameters", len(got))
	}

	if err := m.AddParameter(backend.PageParameterSpec{Name: "Customer", EntityName: "Administration.Account"}); err != nil {
		t.Fatalf("AddParameter entity: %v", err)
	}
	if err := m.AddParameter(backend.PageParameterSpec{Name: "Count", PrimitiveType: "DataTypes$IntegerType"}); err != nil {
		t.Fatalf("AddParameter primitive: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	params := storedParameters(t, b, id)
	if len(params) != 2 {
		t.Fatalf("stored %d parameters, want 2", len(params))
	}
	// The key set Studio Pro writes for a page parameter on this (11.6) fixture,
	// read off ChangePasswordForm's own: no key more, none fewer.
	want := []string{"$ID", "$Type", "DefaultValue", "IsRequired", "Name", "ParameterType"}
	for i, p := range params {
		if got := sortedParamKeys(p); !slices.Equal(got, want) {
			t.Errorf("parameter %d keys = %v, want %v", i, got, want)
		}
		if bsonnav.DGetString(p, "$Type") != "Forms$PageParameter" {
			t.Errorf("parameter %d $Type = %q", i, bsonnav.DGetString(p, "$Type"))
		}
		if req, _ := bsonnav.DGet(p, "IsRequired").(bool); !req {
			t.Errorf("parameter %d is not required; MDL page parameters are", i)
		}
	}
	if n := bsonnav.DGetString(params[0], "Name"); n != "Customer" {
		t.Errorf("first parameter = %q, want Customer", n)
	}
	pt := bsonnav.DGetDoc(params[0], "ParameterType")
	if bsonnav.DGetString(pt, "$Type") != "DataTypes$ObjectType" || bsonnav.DGetString(pt, "Entity") != "Administration.Account" {
		t.Errorf("Customer ParameterType = %v", pt)
	}
	if got := bsonnav.DGetString(bsonnav.DGetDoc(params[1], "ParameterType"), "$Type"); got != "DataTypes$IntegerType" {
		t.Errorf("Count ParameterType = %q, want DataTypes$IntegerType", got)
	}

	// The semantic reader sees them too — what describe prints from.
	pg, err := b.GetPage(id)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if len(pg.Parameters) != 2 || pg.Parameters[0].EntityName != "Administration.Account" {
		t.Errorf("read back %+v", pg.Parameters)
	}
}

func TestAlterPageAddParameter_RefusesDuplicate(t *testing.T) {
	_, m, _ := openPageMutator(t, "Account_Edit")
	err := m.AddParameter(backend.PageParameterSpec{Name: "Account", EntityName: "Administration.Account"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want an 'already exists' refusal, got %v", err)
	}
}

// Account_Edit's data view is bound to $Account. Dropping the parameter would
// leave that binding pointing at nothing, so the drop is refused and storage is
// left as it was.
func TestAlterPageDropParameter_RefusesOneInUse(t *testing.T) {
	b, m, id := openPageMutator(t, "Account_Edit")
	before, err := b.GetRawUnitBytes(id)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	err = m.DropParameter("Account")
	if err == nil || !strings.Contains(err.Error(), "still used") {
		t.Fatalf("want a 'still used' refusal, got %v", err)
	}
	after, _ := b.GetRawUnitBytes(id)
	if !bytes.Equal(before, after) {
		t.Error("a refused drop changed storage")
	}
}

func TestAlterPageDropParameter_RemovesUnused(t *testing.T) {
	b, m, id := openPageMutator(t, "Account_Edit")
	if err := m.AddParameter(backend.PageParameterSpec{Name: "Extra", PrimitiveType: "DataTypes$StringType"}); err != nil {
		t.Fatalf("AddParameter: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if n := len(storedParameters(t, b, id)); n != 2 {
		t.Fatalf("after add: %d parameters, want 2", n)
	}

	m2, err := b.OpenPageForMutation(id)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := m2.DropParameter("Extra"); err != nil {
		t.Fatalf("DropParameter: %v", err)
	}
	if err := m2.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	params := storedParameters(t, b, id)
	if len(params) != 1 || bsonnav.DGetString(params[0], "Name") != "Account" {
		t.Fatalf("after drop: %v", params)
	}
}
