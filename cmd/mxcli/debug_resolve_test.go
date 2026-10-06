// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestExtractActivities(t *testing.T) {
	mf := &microflows.Microflow{
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{
				&microflows.StartEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: "id-start"}}},
				&microflows.Annotation{BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: "id-note"}}, Caption: "Give hint"},
			},
		},
	}
	acts := extractActivities(mf)
	if len(acts) != 2 {
		t.Fatalf("got %d activities, want 2", len(acts))
	}
	if acts[0].Index != 1 || acts[0].Type != "StartEvent" || acts[0].Caption != "" || acts[0].ObjectID != "id-start" {
		t.Errorf("act[0] = %+v", acts[0])
	}
	if acts[1].Index != 2 || acts[1].Type != "Annotation" || acts[1].Caption != "Give hint" || acts[1].ObjectID != "id-note" {
		t.Errorf("act[1] = %+v", acts[1])
	}
}

func TestExtractActivities_NilCollection(t *testing.T) {
	if got := extractActivities(&microflows.Microflow{}); got != nil {
		t.Errorf("nil ObjectCollection should yield nil, got %v", got)
	}
}

func TestMatchActivity(t *testing.T) {
	acts := []activityInfo{
		{Index: 1, Type: "StartEvent", Caption: "", ObjectID: "g1"},
		{Index: 2, Type: "ActionActivity", Caption: "Create 'Game'", ObjectID: "g2"},
		{Index: 3, Type: "ActionActivity", Caption: "Commit 'Game'", ObjectID: "g3"},
	}
	cases := []struct {
		selector string
		wantID   string
		wantErr  bool
	}{
		{"#2", "g2", false},
		{"#1", "g1", false},
		{"create", "g2", false},        // caption substring, case-insensitive
		{"Commit 'Game'", "g3", false}, // exact caption
		{"#0", "", true},               // out of range
		{"#9", "", true},               // out of range
		{"nope", "", true},             // no caption match
		{"game", "", true},             // ambiguous (matches g2 and g3)
		{"", "", true},                 // empty
	}
	for _, c := range cases {
		got, err := matchActivity(acts, c.selector)
		if c.wantErr {
			if err == nil {
				t.Errorf("matchActivity(%q): want error, got %+v", c.selector, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("matchActivity(%q): unexpected error %v", c.selector, err)
			continue
		}
		if got.ObjectID != c.wantID {
			t.Errorf("matchActivity(%q) = %q, want %q", c.selector, got.ObjectID, c.wantID)
		}
	}
}

func TestExtractPausedFlows(t *testing.T) {
	cases := []struct {
		name string
		json string
		want []pausedFlowSummary
	}{
		{
			name: "top-level array",
			json: `[{"debug_id":"d1","microflow_name":"Sudoku.ACT_Hint"}]`,
			want: []pausedFlowSummary{{DebugID: "d1", Microflow: "Sudoku.ACT_Hint"}},
		},
		{
			name: "nested under a key, alt field names",
			json: `{"paused_microflows":[{"id":"d2","microflow":"M.F"},{"debugId":"d3","name":"M.G"}]}`,
			want: []pausedFlowSummary{{DebugID: "d2", Microflow: "M.F"}, {DebugID: "d3", Microflow: "M.G"}},
		},
		{
			name: "empty object",
			json: `{}`,
			want: nil,
		},
		{
			name: "invalid json",
			json: `not json`,
			want: nil,
		},
	}
	for _, c := range cases {
		got := extractPausedFlows([]byte(c.json))
		if len(got) != len(c.want) {
			t.Errorf("%s: got %d flows, want %d (%+v)", c.name, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s[%d] = %+v, want %+v", c.name, i, got[i], c.want[i])
			}
		}
	}
}

func TestExtractPausedFromEvents(t *testing.T) {
	// A paused nanoflow surfaces only in poll_events, as a paused_microflow event
	// whose data uses the microflow_name field.
	json := `{"events":[
		{"type":"log","data":{"message":"hi"}},
		{"type":"paused_microflow","data":{"debug_id":"d-nano","microflow_name":"Sudoku.NF_ToggleNotes","object_id":"o1"}}
	]}`
	got := extractPausedFromEvents([]byte(json))
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (%+v)", len(got), got)
	}
	if got[0].DebugID != "d-nano" || got[0].Microflow != "Sudoku.NF_ToggleNotes" {
		t.Errorf("got %+v", got[0])
	}
	// No paused entries → nil.
	if g := extractPausedFromEvents([]byte(`{"events":[{"type":"log"}]}`)); g != nil {
		t.Errorf("want nil for no paused entries, got %+v", g)
	}
	if g := extractPausedFromEvents([]byte(`not json`)); g != nil {
		t.Errorf("want nil for invalid json, got %+v", g)
	}
}

func TestBreakpointRegistry_UpsertRemove(t *testing.T) {
	var bps []localBreakpoint
	bps = upsertBreakpoint(bps, localBreakpoint{Microflow: "M.F", Activity: "A", ObjectID: "g1"})
	bps = upsertBreakpoint(bps, localBreakpoint{Microflow: "M.F", Activity: "B", ObjectID: "g2"})
	if len(bps) != 2 {
		t.Fatalf("want 2 breakpoints, got %d", len(bps))
	}
	// Upsert with the same object ID replaces, not appends.
	bps = upsertBreakpoint(bps, localBreakpoint{Microflow: "M.F", Activity: "A2", ObjectID: "g1", Condition: "x > 0"})
	if len(bps) != 2 {
		t.Fatalf("upsert on same ID should replace; got %d", len(bps))
	}
	if bps[0].Activity != "A2" || bps[0].Condition != "x > 0" {
		t.Errorf("upsert did not replace: %+v", bps[0])
	}
	// Remove.
	bps = removeBreakpoint(bps, "g1")
	if len(bps) != 1 || bps[0].ObjectID != "g2" {
		t.Errorf("after remove: %+v", bps)
	}
}

func TestBreakpointRegistry_SaveLoad(t *testing.T) {
	path := t.TempDir() + "/.mxcli/debug-breakpoints.json"
	want := []localBreakpoint{{Microflow: "M.F", Activity: "Create", ObjectID: "g1", Condition: "y"}}
	if err := saveBreakpoints(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadBreakpoints(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}
	// A missing file loads as empty, not an error.
	if bps, err := loadBreakpoints(t.TempDir() + "/nope.json"); err != nil || bps != nil {
		t.Errorf("missing file: got %v err=%v, want nil,nil", bps, err)
	}
}

// Every object type that carries a Caption reports it, and the type name is the
// bare struct name, after objectTypeCaption stopped reading the object through
// reflect.ValueOf (CodeQL go/clear-text-logging: a Send Email activity's
// Password expression appeared to reach the `debug activities` output).
func TestObjectTypeCaption_EveryCaptionedType(t *testing.T) {
	email := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{Caption: "Send confirmation"},
		Action:       &microflows.SendEmailAction{Password: "'s3cret'"},
	}
	for _, tc := range []struct {
		obj      microflows.MicroflowObject
		typeName string
		caption  string
	}{
		{email, "ActionActivity", "Send confirmation"},
		{&microflows.ExclusiveSplit{Caption: "Valid?"}, "ExclusiveSplit", "Valid?"},
		{&microflows.InheritanceSplit{Caption: "Kind"}, "InheritanceSplit", "Kind"},
		{&microflows.LoopedActivity{Caption: "Each line"}, "LoopedActivity", "Each line"},
		{&microflows.Annotation{Caption: "Note"}, "Annotation", "Note"},
		{&microflows.EndEvent{}, "EndEvent", ""},
	} {
		gotType, gotCaption := objectTypeCaption(tc.obj)
		if gotType != tc.typeName || gotCaption != tc.caption {
			t.Errorf("objectTypeCaption(%T) = (%q, %q), want (%q, %q)", tc.obj, gotType, gotCaption, tc.typeName, tc.caption)
		}
	}
}
