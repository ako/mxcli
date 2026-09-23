// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/model"
)

// bulkUpdateCtx wires a context whose only page is the stored document passed
// in, opened through the real BSON mutator — so the assignments below succeed or
// fail for the reasons they would in a project, not because a stub said so.
func bulkUpdateCtx(t *testing.T, stored bson.D) (*ExecContext, *bytes.Buffer, *countingDeps) {
	t.Helper()
	deps := &countingDeps{}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(stored, unitID, deps), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	buf := &bytes.Buffer{}
	ctx.Output = buf
	return ctx, buf, deps
}

func bulkRefs(names ...string) []widgetRef {
	refs := make([]widgetRef, 0, len(names))
	for _, n := range names {
		refs = append(refs, widgetRef{
			Name: n, WidgetType: "com.mendix.widget.web.datagrid.Datagrid",
			ContainerID: "c1", ContainerName: "MyModule.P", ContainerType: "page",
		})
	}
	return refs
}

func bulkAssign(pairs ...string) []ast.WidgetPropertyAssignment {
	out := make([]ast.WidgetPropertyAssignment, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, ast.WidgetPropertyAssignment{PropertyPath: p, Value: true})
	}
	return out
}

// ako/mxcli#520.
//
// UPDATE WIDGETS counted widgets it FOUND, not assignments that SUCCEEDED, so a
// run where nothing could be written reported success — after warning about
// every failure. Measured on a blank 11.12.2 project, setting two of Data grid
// 2's real Atlas design properties (they live in Appearance.DesignProperties,
// which SetWidgetProperty does not reach — the capability gap is ako/mxcli#515):
//
//	Found 2 widget(s) in 2 container(s) matching the criteria
//	  Warning: Failed to set 'Compact' on dgA: pluggable property "Compact" not found
//	  Warning: Failed to set 'Striped' on dgA: pluggable property "Striped" not found
//	  … same for dgB …
//	Updated 2 widget(s)
//
// Four failures out of four, then "Updated 2". `describe styling` afterwards
// reported "No styled widgets found".
func TestUpdateWidgets_NoAssignmentSucceedsIsNotAnUpdate(t *testing.T) {
	ctx, _, deps := bulkUpdateCtx(t, storedGridPage())

	out, err := updateWidgetsInContainer(ctx, "c1",
		bulkRefs("dgProducts"), bulkAssign("Compact", "Striped"), false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.WidgetsChanged != 0 {
		t.Errorf("WidgetsChanged = %d, want 0 — every assignment was refused", out.WidgetsChanged)
	}
	if out.WidgetsUnchanged != 1 {
		t.Errorf("WidgetsUnchanged = %d, want 1", out.WidgetsUnchanged)
	}
	if len(out.Failures) != 2 {
		t.Errorf("Failures = %v, want both properties named", out.Failures)
	}
	// The container must not be saved when nothing changed. Elision would skip
	// the bytes anyway, but offering the write is how "Updated 2" got printed.
	if deps.saves != 0 {
		t.Errorf("saved the container %d time(s) with nothing changed", deps.saves)
	}
}

// The control that stops the fix being "always report zero": a property the
// widget really has must still count, and still save.
func TestUpdateWidgets_SuccessfulAssignmentCounts(t *testing.T) {
	ctx, _, deps := bulkUpdateCtx(t, storedGridPage())

	out, err := updateWidgetsInContainer(ctx, "c1",
		bulkRefs("dgProducts"), bulkAssign("pageSize"), false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.WidgetsChanged != 1 {
		t.Errorf("WidgetsChanged = %d, want 1 — pageSize is a real template key", out.WidgetsChanged)
	}
	if len(out.Failures) != 0 {
		t.Errorf("unexpected failures: %v", out.Failures)
	}
	if deps.saves != 1 {
		t.Errorf("saved %d time(s), want 1", deps.saves)
	}
}

// A partial run must report BOTH halves rather than rounding to success or to
// failure — the shape a real sweep produces, where a property exists on some of
// the matched widgets and not others.
func TestUpdateWidgets_PartialRunReportsBothHalves(t *testing.T) {
	ctx, _, deps := bulkUpdateCtx(t, storedGridPage())

	out, err := updateWidgetsInContainer(ctx, "c1",
		bulkRefs("dgProducts"), bulkAssign("pageSize", "Compact"), false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.WidgetsChanged != 1 {
		t.Errorf("WidgetsChanged = %d, want 1 — one assignment landed", out.WidgetsChanged)
	}
	if len(out.Failures) != 1 || !strings.Contains(out.Failures[0], "Compact") {
		t.Errorf("Failures = %v, want the one refused property named", out.Failures)
	}
	if deps.saves != 1 {
		t.Errorf("saved %d time(s), want 1 — something did change", deps.saves)
	}
}

// A widget the catalog lists but the document does not carry is neither changed
// nor a property failure; it is its own outcome, and rounding it into either
// one is how a stale catalog reads as success.
func TestUpdateWidgets_MissingWidgetIsItsOwnOutcome(t *testing.T) {
	ctx, _, deps := bulkUpdateCtx(t, storedGridPage())

	out, err := updateWidgetsInContainer(ctx, "c1",
		bulkRefs("ghostWidget"), bulkAssign("pageSize"), false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.WidgetsChanged != 0 || out.WidgetsMissing != 1 {
		t.Errorf("got changed=%d missing=%d, want 0 and 1", out.WidgetsChanged, out.WidgetsMissing)
	}
	if deps.saves != 0 {
		t.Errorf("saved the container for a widget that is not in it")
	}
}
