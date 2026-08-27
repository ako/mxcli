// SPDX-License-Identifier: Apache-2.0

package offlinepaths

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// step builds one EntityRefStep as it is stored.
func step(assoc string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "DomainModels$EntityRefStep"},
		{Key: "Association", Value: assoc},
	}
}

// attrRef builds a stored DomainModels$AttributeRef navigating the given
// associations. Steps carries a leading typed-array marker, as on disk.
func attrRef(attribute string, assocs ...string) bson.D {
	var entityRef any
	if len(assocs) > 0 {
		steps := bson.A{int32(2)}
		for _, a := range assocs {
			steps = append(steps, step(a))
		}
		entityRef = bson.D{
			{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
			{Key: "Steps", Value: steps},
		}
	}
	return bson.D{
		{Key: "$Type", Value: "DomainModels$AttributeRef"},
		{Key: "Attribute", Value: attribute},
		{Key: "EntityRef", Value: entityRef},
	}
}

func TestScanDocumentCountsAssociationHops(t *testing.T) {
	// CE6206 fires at two hops, not at "any indirect reference". Pinned in Studio
	// Pro 11.14 on one page carrying both: MaintenanceRequest_Asset → AssetName is
	// accepted, MaintenanceRequest_Asset → Asset_Site → SiteName is not. The
	// one-hop case is the control — a rule that flagged it would condemn a
	// construct offline navigation explicitly allows.
	for _, tc := range []struct {
		name   string
		assocs []string
		want   int // 0 = no finding
	}{
		{"own attribute", nil, 0},
		{"one hop is allowed", []string{"Mod.Req_Asset"}, 0},
		{"two hops are CE6206", []string{"Mod.Req_Asset", "Mod.Asset_Site"}, 2},
		{"three hops", []string{"Mod.A", "Mod.B", "Mod.C"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := bson.D{
				{Key: "$Type", Value: "Pages$Page"},
				{Key: "Widgets", Value: bson.A{int32(2), bson.D{
					{Key: "$Type", Value: "Forms$TextBox"},
					{Key: "AttributeRef", Value: attrRef("Mod.Site.SiteName", tc.assocs...)},
				}}},
			}
			got := ScanDocument(doc)
			if tc.want == 0 {
				if len(got) != 0 {
					t.Fatalf("got %d findings, want none: %+v", len(got), got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("got %d findings, want 1: %+v", len(got), got)
			}
			if got[0].Steps != tc.want {
				t.Errorf("Steps = %d, want %d", got[0].Steps, tc.want)
			}
			if got[0].Path != "Mod.Site.SiteName" {
				t.Errorf("Path = %q", got[0].Path)
			}
		})
	}
}

func TestScanDocumentFindsPluggableWidgetBindings(t *testing.T) {
	// The reason the walk keys on $Type rather than on a table of property names:
	// a DataGrid2 column's binding is an AttributeRef nested several levels inside
	// a CustomWidgets$WidgetValue, at a key path no property table would have
	// listed. A scan that only knew about Forms$TextBox.AttributeRef would report
	// a clean project for the widget people actually use.
	doc := bson.D{
		{Key: "$Type", Value: "Pages$Page"},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
			{Key: "Object", Value: bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
				{Key: "Properties", Value: bson.A{int32(2), bson.D{
					{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
					{Key: "Value", Value: bson.D{
						{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
						{Key: "AttributeRef", Value: attrRef("Mod.Site.SiteName", "Mod.Req_Asset", "Mod.Asset_Site")},
					}},
				}}},
			}},
		}}},
	}
	got := ScanDocument(doc)
	if len(got) != 1 || got[0].Steps != 2 {
		t.Fatalf("got %+v, want one 2-step finding", got)
	}
}

func TestScanDocumentIgnoresNonAttributeEntityRefs(t *testing.T) {
	// A data source that navigates associations is a different element, and CE6206
	// is not about it. Matching every IndirectEntityRef in the document — the
	// obvious shortcut — would report constructs offline navigation permits, and a
	// checker that cries wolf teaches people to ignore it.
	doc := bson.D{
		{Key: "$Type", Value: "Pages$Page"},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "Forms$DataView"},
			{Key: "DataSource", Value: bson.D{
				{Key: "$Type", Value: "Forms$AssociationSource"},
				{Key: "EntityRef", Value: bson.D{
					{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
					{Key: "Steps", Value: bson.A{int32(2), step("Mod.A"), step("Mod.B")}},
				}},
			}},
		}}},
	}
	if got := ScanDocument(doc); len(got) != 0 {
		t.Errorf("got %+v, want no findings", got)
	}
}

// --- Scan over a source -----------------------------------------------------

type fakeSource struct {
	pages    []*pages.Page
	snippets []*pages.Snippet
	units    map[string][]byte
	err      error
}

func (f *fakeSource) ListPages() ([]*pages.Page, error)       { return f.pages, f.err }
func (f *fakeSource) ListSnippets() ([]*pages.Snippet, error) { return f.snippets, nil }
func (f *fakeSource) GetRawUnitBytes(id model.ID) ([]byte, error) {
	raw, ok := f.units[string(id)]
	if !ok {
		return nil, errors.New("no such unit")
	}
	return raw, nil
}

func pageUnit(t *testing.T, assocs ...string) []byte {
	t.Helper()
	raw, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Pages$Page"},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "Forms$TextBox"},
			{Key: "AttributeRef", Value: attrRef("Mod.Site.SiteName", assocs...)},
		}}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func newPage(id, name string) *pages.Page {
	p := &pages.Page{Name: name}
	p.ID = model.ID(id)
	return p
}

func TestScanReportsOnlyTheOffendingDocuments(t *testing.T) {
	src := &fakeSource{
		pages: []*pages.Page{
			newPage("clean", "Overview"),
			newPage("onehop", "Detail"),
			newPage("twohop", "Report"),
		},
		units: map[string][]byte{
			"clean":  pageUnit(t),
			"onehop": pageUnit(t, "Mod.Req_Asset"),
			"twohop": pageUnit(t, "Mod.Req_Asset", "Mod.Asset_Site"),
		},
	}
	got, err := Scan(src, func(_ model.ID, name string) string { return "Maintenance." + name })
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if got[0].Document != "Maintenance.Report" || got[0].Kind != "page" {
		t.Errorf("got %+v", got[0])
	}
}

func TestScanSkipsUnreadableDocuments(t *testing.T) {
	// This feeds a warning. A document that cannot be opened must not abort the
	// statement the warning is advising on — the remaining documents are still
	// worth reporting.
	src := &fakeSource{
		pages: []*pages.Page{newPage("missing", "Gone"), newPage("twohop", "Report")},
		units: map[string][]byte{"twohop": pageUnit(t, "Mod.A", "Mod.B")},
	}
	got, err := Scan(src, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].Document != "Report" {
		t.Fatalf("got %+v", got)
	}
}
