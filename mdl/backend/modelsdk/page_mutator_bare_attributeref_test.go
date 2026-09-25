// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ALTER PAGE does not go through encodePage: the page mutator patches the stored
// BSON tree and saves the bytes with UpdateRawUnit. A bare attribute in a column
// template parameter (what an ALTER inside a data container with no resolvable
// entity produces — the column builder writes the name as given) reached disk
// that way, and Mendix cannot LOAD a project holding one (ArgumentNullException
// setting 'Attribute', 11.13.0). The same statement, run on a copy of a real
// project, left `mx check` unable to open it.
//
// So the refusal sits at the writer, and this test goes through the real
// mutator and the real writer — the wiring is what came undone.

func openAccountOverviewMutator(t *testing.T) (*Backend, backend.PageMutator, model.ID) {
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
		if p.Name == "Account_Overview" {
			id = p.ID
		}
	}
	if id == "" {
		t.Fatal("fixture has no Account_Overview page")
	}
	m, err := b.OpenPageForMutation(id)
	if err != nil {
		t.Fatalf("OpenPageForMutation: %v", err)
	}
	return b, m, id
}

func captionColumn(attr string) *backend.DataGridColumnSpec {
	return &backend.DataGridColumnSpec{
		Caption: "{1}",
		CaptionParams: []*pages.ClientTemplateParameter{{
			BaseElement:  model.BaseElement{ID: model.ID("0d4b8c1e-1111-4a1a-9a1a-111111111111")},
			AttributeRef: attr,
		}},
		ShowContentAs: "dynamicText",
		Content:       "x",
	}
}

func TestAlterPageSaveRefusesABareAttributeRef(t *testing.T) {
	b, m, id := openAccountOverviewMutator(t)
	before, err := b.GetRawUnitBytes(id)
	if err != nil {
		t.Fatalf("read stored page: %v", err)
	}

	if err := m.InsertColumns("dataGrid21", "WebServiceUser", backend.InsertPosition("after"),
		[]*backend.DataGridColumnSpec{captionColumn("FullName")}); err != nil {
		t.Fatalf("InsertColumns: %v", err)
	}
	err = m.Save()
	if err == nil {
		t.Fatal("ALTER saved a page holding a bare attribute reference; Mendix cannot load that project")
	}
	for _, want := range []string{`"FullName"`, "Module.Entity.Attribute"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %s: %v", want, err)
		}
	}

	after, err := b.GetRawUnitBytes(id)
	if err != nil {
		t.Fatalf("read page after refusal: %v", err)
	}
	if string(after) != string(before) {
		t.Error("the refused ALTER still changed the stored page")
	}
}

func TestAlterPageSaveAcceptsAQualifiedAttributeRef(t *testing.T) {
	// The control: the same column, qualified, is an ordinary ALTER.
	b, m, id := openAccountOverviewMutator(t)
	before, _ := b.GetRawUnitBytes(id)
	if err := m.InsertColumns("dataGrid21", "WebServiceUser", backend.InsertPosition("after"),
		[]*backend.DataGridColumnSpec{captionColumn("Administration.Account.FullName")}); err != nil {
		t.Fatalf("InsertColumns: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("a qualified attribute reference was refused: %v", err)
	}
	after, _ := b.GetRawUnitBytes(id)
	if string(after) == string(before) {
		t.Fatal("control did not write: the accepted ALTER must reach the stored page")
	}
	if !strings.Contains(string(after), "Administration.Account.FullName") {
		t.Error("the qualified reference is not in the stored page")
	}
}
