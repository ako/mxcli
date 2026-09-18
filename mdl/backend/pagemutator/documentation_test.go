// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#527: ALTER PAGE could not set Documentation, so documenting an
// existing page meant re-running its CREATE — which for a real page means
// re-emitting its whole widget tree, and a describe → exec round trip is only as
// complete as what MDL can spell.
package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// Documentation is a plain top-level string on the document, the same shape as
// Url — gen binds it as property.NewPrimitive[string]("Documentation") on Page,
// Layout and Snippet alike, and pageToGen has always written it on CREATE.
func TestSetPageLevel_Documentation(t *testing.T) {
	m := &Mutator{rawData: makeRawPage(), widgetFinder: findBsonWidget}

	const doc = "Triage step. Coordinator sets priority and accepts the request."
	if err := m.SetWidgetProperty("", "Documentation", doc); err != nil {
		t.Fatalf("SET Documentation failed: %v", err)
	}
	if got := bsonnav.DGet(m.rawData, "Documentation"); got != doc {
		t.Errorf("Documentation = %v, want %q", got, doc)
	}
}

// An existing value is replaced rather than appended beside itself — a second
// Documentation key is a document Studio Pro resolves against the type's
// property list and cannot open.
func TestSetPageLevel_Documentation_ReplacesExisting(t *testing.T) {
	raw := makeRawPage()
	raw = append(raw, bson.E{Key: "Documentation", Value: "old"})
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}

	if err := m.SetWidgetProperty("", "Documentation", "new"); err != nil {
		t.Fatalf("SET Documentation failed: %v", err)
	}
	if got := bsonnav.DGet(m.rawData, "Documentation"); got != "new" {
		t.Errorf("Documentation = %v, want \"new\"", got)
	}
	n := 0
	for _, e := range m.rawData {
		if e.Key == "Documentation" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("document carries %d Documentation keys, want 1", n)
	}
}

// Clearing is a real operation — a doc comment removed from a script should be
// removable from the document — so an empty string is stored, not rejected.
func TestSetPageLevel_Documentation_EmptyClears(t *testing.T) {
	raw := makeRawPage()
	raw = append(raw, bson.E{Key: "Documentation", Value: "old"})
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}

	if err := m.SetWidgetProperty("", "Documentation", ""); err != nil {
		t.Fatalf("SET Documentation = '' failed: %v", err)
	}
	if got := bsonnav.DGet(m.rawData, "Documentation"); got != "" {
		t.Errorf("Documentation = %v, want empty", got)
	}
}

func TestSetPageLevel_Documentation_NonString(t *testing.T) {
	m := &Mutator{rawData: makeRawPage(), widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("", "Documentation", 42); err == nil {
		t.Error("non-string Documentation should be rejected")
	}
}

// The unsupported-property message is the only guidance a reader gets, so it has
// to list what it now accepts. A stale list sends someone to the CREATE
// workaround this change exists to remove.
func TestSetPageLevel_UnsupportedMessageNamesDocumentation(t *testing.T) {
	m := &Mutator{rawData: makeRawPage(), widgetFinder: findBsonWidget}
	err := m.SetWidgetProperty("", "NotARealProperty", 1)
	if err == nil {
		t.Fatal("expected an error for an unsupported page-level property")
	}
	if !strings.Contains(err.Error(), "Documentation") {
		t.Errorf("the supported-property list does not mention Documentation: %v", err)
	}
}
