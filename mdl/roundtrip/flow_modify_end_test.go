// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#805: the end event under diff-then-patch. `describe` prints the
// flow's final end event as a trailing `@position(x, y) return;`, and a script
// that ends without a return states the same end implicitly — the builder draws
// one. The splice compared the two as a stored `return` the script had
// dropped, and refused to drop an end event, so under mdl 1 every re-run of a
// `create or modify` whose body does not spell its trailing return failed, and
// a changed return value was refused as "a return inside an inserted fragment".
// Every flow here is Studio Pro-authored (PedApp).

const showPasswordForm = "microflow Administration.ShowPasswordForm"

// withoutTrailingReturn removes the `@position(…)\n  return;` describe prints
// as the last statement of a void flow, leaving the end implicit.
func withoutTrailingReturn(t *testing.T, described, position string) string {
	t.Helper()
	tail := "  " + position + "\n  return;\nend;"
	out := strings.Replace(described, tail, "end;", 1)
	if out == described {
		t.Fatalf("describe output has no trailing %q:\n%s", tail, described)
	}
	return out
}

// An implicit end is the stored end event: the unchanged definition without
// its trailing return writes nothing, under mdl 1 as under mdl 0.
func TestFlowModify_ImplicitEndIsTheStoredEnd(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	implicit := withoutTrailingReturn(t, h.mustDescribeMdl0(t, showPasswordForm), "@position(695, 200)")
	for _, header := range []string{"", "mdl 1;\n"} {
		before := h.flowUnit(t, "ShowPasswordForm")
		if err := h.exec(header + implicit); err != nil {
			t.Fatalf("header %q: %v", header, err)
		}
		if !bytes.Equal(h.flowUnit(t, "ShowPasswordForm"), before) {
			t.Fatalf("header %q: the unchanged definition without its trailing return rewrote the unit", header)
		}
		if strings.Contains(h.out.String(), "MDL-V1-REBUILD") {
			t.Fatalf("header %q: the implicit end fell back to the rebuild:\n%s", header, h.out.String())
		}
	}
	if changed := h.orig.diff(h.snapshot()); len(changed) != 0 {
		t.Fatalf("the unchanged definitions wrote: %s", strings.Join(changed, "; "))
	}
}

// A statement appended before an implicit end is spliced in, and the stored
// end event stays (its $ID with it).
func TestFlowModify_AppendBeforeImplicitEnd(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	implicit := withoutTrailingReturn(t, h.mustDescribeMdl0(t, showPasswordForm), "@position(695, 200)")
	edited := strings.Replace(implicit, "\nend;", "\n  log info node 'Pwd' 'shown';\nend;", 1)
	before := h.flowUnit(t, "ShowPasswordForm")
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 inserted)") {
		t.Errorf("want a splice report, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "ShowPasswordForm")
	requireKept(t, before, after, "")
	if got := countType(t, after, "Microflows$EndEvent"); got != 1 {
		t.Errorf("%d end events after the insert, want the 1 stored", got)
	}
}

// A changed return value is an edit of the stored end event: its $ID, the
// note attached to it and every other element stay, and the value is written.
func TestFlowModify_ReturnValueIsSetInPlace(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow FeedbackModule.ConvertUUIDToURL"
	described := h.mustDescribeMdl0(t, target)
	const old = "return 'https://appinsights.mendix.com/link/showfeedback/'+$uuid;"
	edited := strings.Replace(described, old, "return 'https://example.com/feedback/'+$uuid;", 1)
	if edited == described {
		t.Fatalf("describe output has no %q:\n%s", old, described)
	}
	before := h.flowUnit(t, "ConvertUUIDToURL")
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 edited in place)") {
		t.Errorf("want a splice report, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "ConvertUUIDToURL")
	requireKept(t, before, after, "")
	if len(elementIDs(t, after)) != len(elementIDs(t, before)) {
		t.Errorf("%d elements after, %d before: the edit added or removed one", len(elementIDs(t, after)), len(elementIDs(t, before)))
	}
	again := h.mustDescribeMdl0(t, target)
	if !strings.Contains(again, "return 'https://example.com/feedback/'+$uuid;") {
		t.Errorf("describe does not show the new value:\n%s", again)
	}
	// The splice's output is a fixed point: its description writes nothing.
	settled := h.flowUnit(t, "ConvertUUIDToURL")
	if err := h.exec("mdl 1;\n" + again); err != nil {
		t.Fatalf("re-exec: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, "ConvertUUIDToURL"), settled) {
		t.Error("re-executing the description of the edited flow rewrote it")
	}
}

// A statement inserted before a return whose value changes too: one insert and
// one in-place edit, no rebuild.
func TestFlowModify_InsertBeforeChangedReturn(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow FeedbackModule.ConvertUUIDToURL"
	described := h.mustDescribeMdl0(t, target)
	const old = "  @position(700, 200)\n"
	edited := strings.Replace(described, old, "  log info node 'Url' $uuid;\n"+old, 1)
	edited = strings.Replace(edited, "+$uuid;\nend;", "+$uuid+'/';\nend;", 1)
	if !strings.Contains(edited, "log info") || !strings.Contains(edited, "+'/';") {
		t.Fatalf("describe output changed shape:\n%s", described)
	}
	before := h.flowUnit(t, "ConvertUUIDToURL")
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 inserted, 1 edited in place)") {
		t.Errorf("want a splice report, got:\n%s", h.out.String())
	}
	requireKept(t, before, h.flowUnit(t, "ConvertUUIDToURL"), "")
	again := h.mustDescribeMdl0(t, target)
	if !strings.Contains(again, "log node 'Url'") || !strings.Contains(again, "+$uuid+'/';") {
		t.Errorf("describe does not show both changes:\n%s", again)
	}
}

// A return in a branch is an end event like the last one: a changed value in
// the else branch is set in place.
func TestFlowModify_BranchReturnValueIsSetInPlace(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow FeedbackModule.ConvertBase64String"
	described := h.mustDescribeMdl0(t, target)
	edited := strings.Replace(described, "    return empty;\n", "    return '';\n", 1)
	if edited == described {
		t.Fatalf("describe output has no `return empty;`:\n%s", described)
	}
	before := h.flowUnit(t, "ConvertBase64String")
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec: %v", err)
	}
	after := h.flowUnit(t, "ConvertBase64String")
	requireKept(t, before, after, "")
	if !strings.Contains(h.mustDescribeMdl0(t, target), "    return '';\n") {
		t.Error("describe does not show the new value")
	}
}

// What the splice genuinely cannot do with an end event is refused, naming it:
// a return taken out of a branch makes the branch fall through, which changes
// the shape of the flow.
func TestFlowModify_DroppedBranchReturnIsRefused(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow FeedbackModule.ConvertBase64String"
	described := h.mustDescribeMdl0(t, target)
	// The then-branch no longer returns: it falls through to a new trailing
	// return after the if.
	edited := strings.Replace(described, "    return substring(", "    declare $X String = substring(", 1)
	edited = strings.Replace(edited, "  end if;\nend;", "  end if;\n  return empty;\nend;", 1)
	if !strings.Contains(edited, "declare $X String = substring(") || !strings.Contains(edited, "  return empty;\nend;") {
		t.Fatalf("describe output changed shape:\n%s", described)
	}
	err := h.exec("mdl 1;\n" + edited)
	if err == nil || !strings.Contains(err.Error(), "cannot be spliced") || !strings.Contains(err.Error(), "return") {
		t.Fatalf("want a refusal naming the return, got %v", err)
	}
	if changed := h.orig.diff(h.snapshot()); len(changed) != 0 {
		t.Fatalf("the refused statement wrote: %s", strings.Join(changed, "; "))
	}
}

// A run of statements that are stored ones redrawn elsewhere is a run of
// moves (ako/mxcli#818). It used to be refused under mdl 1 and rebuilt under
// mdl 0; as a replace and drops it would have written new nodes and lost every
// position the script states (the rewrite in mdl-examples/bug-tests/951 moved
// three activities and none of them moved). Now each stored node moves where
// the script says — its $ID, its flows and their curves stay — and a start
// event mxcli's layout placed follows the first statement, as does the end the
// body falls through to. A @start stated on a new first statement moves the
// start event. The flow is mxcli-authored: this is about what is written, not
// about identity, which the Studio Pro tests in flow_modify_header_test.go
// cover.
func TestFlowModify_MovedRunIsMoved(t *testing.T) {
	const create = `create or modify microflow MyFirstModule.Moved ()
begin
  @position(200, 200)
  log 'one';
  @position(360, 200)
  log 'two';
end;`
	moved := strings.NewReplacer("(200, 200)", "(360, 340)", "(360, 200)", "(520, 340)").Replace(create)
	// A new first statement stating @start: inserted, with the start event
	// moved where the script says.
	started := strings.Replace(create, "begin\n", "begin\n  @start(-10, 200)\n  @position(40, 200)\n  log 'zero';\n", 1)
	for _, header := range []string{"mdl 1;\n", ""} {
		for _, c := range []struct {
			name, script string
			want         []string
		}{
			// The start (40;200) and the end (600;200) were where the layout
			// derives them, so they follow the activities.
			{"moved run", moved, []string{"StartEvent 200;340", "ActionActivity 360;340", "ActionActivity 520;340", "EndEvent 760;340"}},
			{"moved start", started, []string{"StartEvent -10;200", "ActionActivity 40;200", "ActionActivity 200;200",
				"ActionActivity 360;200", "EndEvent 600;200"}},
		} {
			t.Run(fmt.Sprintf("%s %q", c.name, header), func(t *testing.T) {
				h := newHarness(t)
				defer h.close()
				if err := h.exec(create); err != nil {
					t.Fatalf("create: %v", err)
				}
				before := h.flowUnit(t, "Moved")
				if err := h.exec(header + c.script); err != nil {
					t.Fatalf("exec: %v", err)
				}
				if strings.Contains(h.out.String(), "MDL-V1-REBUILD") {
					t.Fatalf("fell back to the rebuild:\n%s", h.out.String())
				}
				after := h.flowUnit(t, "Moved")
				requireKept(t, before, after, "")
				sort.Strings(c.want)
				if got := objectPositions(t, after); strings.Join(got, ", ") != strings.Join(c.want, ", ") {
					t.Errorf("drawn at %v, want %v", got, c.want)
				}
			})
		}
	}
}

// objectPositions lists a unit's top-level objects as "Type x;y", sorted.
func objectPositions(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range doc {
		if e.Key != "ObjectCollection" {
			continue
		}
		oc, _ := e.Value.(bson.D)
		for _, f := range oc {
			if f.Key != "Objects" {
				continue
			}
			list, _ := f.Value.(bson.A)
			for _, el := range list {
				if d, ok := el.(bson.D); ok {
					var p string
					for _, g := range d {
						if g.Key == "RelativeMiddlePoint" {
							p, _ = g.Value.(string)
						}
					}
					out = append(out, typeOf(d)+" "+p)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// `alter microflow … { replace return … }` goes the same way: a single return
// replacing an end event sets its value in place.
func TestAlterFlow_ReplaceReturnSetsTheValue(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	before := h.flowUnit(t, "ConvertUUIDToURL")
	if err := h.exec("alter microflow FeedbackModule.ConvertUUIDToURL {\n  replace return * with begin return 'x'+$uuid; end;\n};"); err != nil {
		t.Fatalf("alter: %v", err)
	}
	after := h.flowUnit(t, "ConvertUUIDToURL")
	requireKept(t, before, after, "")
	if len(elementIDs(t, after)) != len(elementIDs(t, before)) {
		t.Error("the alter added or removed an element")
	}
	if !strings.Contains(h.mustDescribeMdl0(t, "microflow FeedbackModule.ConvertUUIDToURL"), "return 'x'+$uuid;") {
		t.Error("describe does not show the new value")
	}
}
