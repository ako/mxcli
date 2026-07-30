// SPDX-License-Identifier: Apache-2.0

// A pluggable widget's children are serialized through widgetobj.ChildSerializer,
// which returns BSON with no error channel. A construct the codec engine cannot
// represent — a nanoflow datasource on a DataGrid2, say — was therefore logged and
// dropped, and the write still reported success: `Created page`, with a grid that
// has no data source. ADR-0004 requires the backend to refuse rather than drop.
package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestChildSerializeErr_RecordedAndDrained(t *testing.T) {
	t.Cleanup(func() { _ = takeChildSerializeErr() })
	if err := takeChildSerializeErr(); err != nil {
		t.Fatalf("accumulator not empty at start: %v", err)
	}

	// A nanoflow datasource is not yet representable by the codec engine.
	got := codecChildSerializer{}.SerializeCustomWidgetDataSource(
		&pages.NanoflowSource{Nanoflow: "M.GetOrders"})
	if got != nil {
		t.Errorf("unsupported datasource serialized to %v, want nil", got)
	}

	err := takeChildSerializeErr()
	if err == nil {
		t.Fatal("failure was not recorded; it would be dropped silently")
	}
	if !strings.Contains(err.Error(), "NanoflowSource") {
		t.Errorf("error does not name the construct: %v", err)
	}

	// Draining clears, so the next write is not failed by a stale error.
	if err := takeChildSerializeErr(); err != nil {
		t.Errorf("accumulator not cleared after drain: %v", err)
	}
}

func TestChildSerializeErr_SupportedSourceRecordsNothing(t *testing.T) {
	t.Cleanup(func() { _ = takeChildSerializeErr() })
	_ = takeChildSerializeErr()

	got := codecChildSerializer{}.SerializeCustomWidgetDataSource(
		&pages.MicroflowSource{Microflow: "M.GetOrders"})
	if got == nil {
		t.Fatal("supported microflow datasource serialized to nil")
	}
	if err := takeChildSerializeErr(); err != nil {
		t.Errorf("supported datasource recorded an error: %v", err)
	}
}

// TestCreatePage_FailsOnDroppedChild pins the drain: a recorded failure must fail
// the statement rather than let the page land with the piece missing.
func TestCreatePage_FailsOnDroppedChild(t *testing.T) {
	t.Cleanup(func() { _ = takeChildSerializeErr() })
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	// Simulate the executor building a widget tree whose child could not be
	// serialized, exactly as SerializeCustomWidgetDataSource does above.
	codecChildSerializer{}.SerializeCustomWidgetDataSource(&pages.NanoflowSource{Nanoflow: "M.GetOrders"})

	page := &pages.Page{Name: "DropProbe"}
	page.ID = model.ID("")
	err := b.CreatePage(page)
	if err == nil {
		t.Fatal("CreatePage succeeded after a child was dropped")
	}
	if !strings.Contains(err.Error(), "NanoflowSource") {
		t.Errorf("error does not explain what was dropped: %v", err)
	}
}
