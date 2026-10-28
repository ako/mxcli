// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#888: under mdl 1, `create or modify` could not grow a stored flow
// by a guard clause — `if … then return …; end if;` before what it had — and
// refused on every run: "the fragment returns; a return inside an inserted
// fragment is not supported yet". The splice cut a fragment out of the
// builder's graph only when it had a single end event. This is the
// stub-then-real pattern: a flow first created as a stub (so something can
// call it), then grown into the real one by the same statement.
//
// Each return the fragment holds is a new end event, drawn where the builder
// drew it relative to the fragment; the stored activities keep their $IDs. The
// grown flow re-executes as Unchanged (the twice-exec rule), and a changed guard
// condition is written in place.
//
// Named TestSpliceRerun_ so that it runs in the parity suite (#870).

const growStub = `create or modify microflow MyFirstModule.Grow_Guard ($N: Integer)
returns Boolean as $Done
begin
  return false;
end;
`

const growGuard = `create or modify microflow MyFirstModule.Grow_Guard ($N: Integer)
returns Boolean as $Done
begin
  if $N <= 0 then
    return true;
  end if;
  log info node 'Grow' 'n > 0';
  return false;
end;
`

// An mxcli-authored stub grows by a guard: spliced, not rebuilt, and the
// second run of the same script writes nothing.
func TestSpliceRerun_GrowByGuard(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	if err := h.exec("mdl 1;\n" + growStub); err != nil {
		t.Fatalf("stub: %v\n%s", err, h.out.String())
	}
	stub := h.flowUnit(t, "Grow_Guard")

	if err := h.exec("mdl 1;\n" + growGuard); err != nil {
		t.Fatalf("grow: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 inserted)") {
		t.Errorf("want one insert, got:\n%s", h.out.String())
	}
	grown := h.flowUnit(t, "Grow_Guard")
	requireKept(t, stub, grown, "")
	requireDecisionCases(t, grown)
	if got := countType(t, grown, "Microflows$EndEvent"); got != 2 {
		t.Errorf("%d end events after the grow, want the stored one and the guard's", got)
	}
	described := h.mustDescribeMdl0(t, "microflow MyFirstModule.Grow_Guard")
	for _, want := range []string{"if $N <= 0 then", "return true;", "'Grow' 'n > 0';", "return false;"} {
		if !strings.Contains(described, want) {
			t.Errorf("the grown flow does not state %q:\n%s", want, described)
		}
	}

	// The twice-exec rule: the same script again writes nothing.
	settled := h.snapshot()
	if err := h.exec("mdl 1;\n" + growGuard); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "Unchanged microflow: MyFirstModule.Grow_Guard") {
		t.Errorf("the second run did not report Unchanged:\n%s", h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the second run wrote: %s", strings.Join(changed, "; "))
	}
	// And so does the grown flow's own description.
	if err := h.exec("mdl 1;\n" + described); err != nil {
		t.Fatalf("re-exec of the description: %v\n%s", err, h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the description of the grown flow wrote: %s", strings.Join(changed, "; "))
	}

	// Control: a changed guard condition is a change, set in place — the
	// decision and both of its paths stay — and is itself a fixed point.
	before := h.flowUnit(t, "Grow_Guard")
	changed := strings.Replace(growGuard, "$N <= 0", "$N < 1", 1)
	if err := h.exec("mdl 1;\n" + changed); err != nil {
		t.Fatalf("changed condition: %v\n%s", err, h.out.String())
	}
	after := h.flowUnit(t, "Grow_Guard")
	if bytes.Equal(before, after) {
		t.Fatalf("a changed guard condition wrote nothing:\n%s", h.out.String())
	}
	requireKept(t, before, after, "")
	if len(elementIDs(t, after)) != len(elementIDs(t, before)) {
		t.Errorf("%d elements after, %d before: the condition edit added or removed one", len(elementIDs(t, after)), len(elementIDs(t, before)))
	}
	if got := h.mustDescribeMdl0(t, "microflow MyFirstModule.Grow_Guard"); !strings.Contains(got, "if $N < 1 then") {
		t.Errorf("describe does not show the new condition:\n%s", got)
	}
	settled = h.snapshot()
	if err := h.exec("mdl 1;\n" + changed); err != nil {
		t.Fatalf("changed condition, second run: %v\n%s", err, h.out.String())
	}
	if diff := settled.diff(h.snapshot()); len(diff) != 0 {
		t.Errorf("the changed condition's second run wrote: %s", strings.Join(diff, "; "))
	}
}

// A Studio Pro-authored flow grows by a guard before its first activity: every
// stored element stays, the guard's return is a new end event, and the
// second run writes nothing.
func TestSpliceRerun_GrowStudioProFlowByGuard(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow Administration.ShowPasswordForm"
	// The script states no layout, as a hand-written one does: a stated
	// @position is where the script puts a node, and the insert moves the
	// stored nodes after it along to make room.
	described := withoutLayout(h.mustDescribeMdl0(t, target))
	const first = "  $AccountPasswordData = create"
	const guard = "  if $Account = empty then\n    return;\n  end if;\n"
	edited := strings.Replace(described, first, guard+first, 1)
	if edited == described {
		t.Fatalf("describe output changed shape:\n%s", described)
	}
	before := h.flowUnit(t, "ShowPasswordForm")
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("grow: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 inserted)") {
		t.Errorf("want one insert, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "ShowPasswordForm")
	requireKept(t, before, after, "")
	if got := countType(t, after, "Microflows$EndEvent"); got != 2 {
		t.Errorf("%d end events after the grow, want 2", got)
	}
	// The guard is the whole fragment, so the flow that carries on from it
	// leaves the decision: it is the false path, and says so (CE0079).
	requireDecisionCases(t, after)

	settled := h.snapshot()
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the second run wrote: %s\n%s", strings.Join(changed, "; "), h.out.String())
	}
	again := h.mustDescribeMdl0(t, target)
	if err := h.exec("mdl 1;\n" + again); err != nil {
		t.Fatalf("re-exec of the description: %v\n%s", err, h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the description of the grown flow wrote: %s", strings.Join(changed, "; "))
	}

	// Control: the condition changed is written, in place.
	changed := strings.Replace(edited, "if $Account = empty then", "if $Account/FullName = empty then", 1)
	if err := h.exec("mdl 1;\n" + changed); err != nil {
		t.Fatalf("changed condition: %v\n%s", err, h.out.String())
	}
	if len(settled.diff(h.snapshot())) == 0 {
		t.Fatalf("a changed guard condition wrote nothing:\n%s", h.out.String())
	}
	cond := h.flowUnit(t, "ShowPasswordForm")
	requireKept(t, after, cond, "")
	if len(elementIDs(t, cond)) != len(elementIDs(t, after)) {
		t.Errorf("the condition edit added or removed an element")
	}
}

var layoutLine = regexp.MustCompile(`(?m)^\s*@(position|start|curve|anchor)\(.*\n`)

// withoutLayout takes the layout annotations off a description.
func withoutLayout(described string) string { return layoutLine.ReplaceAllString(described, "") }

// A guard whose return would be drawn across a stored flow is refused, and
// nothing is written: inserted after the then-branch's activity, the guard's
// end event lands on the flow by which the else-branch rejoins.
func TestSpliceRerun_GrowByGuardAcrossAFlowIsRefused(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const flow = `create or modify microflow MyFirstModule.Grow_Cross ($N: Integer)
begin
  if $N > 5 then
    log info node 'X' 'a';
  else
    log info node 'X' 'b';
  end if;
  log info node 'X' 'c';
end;
`
	if err := h.exec("mdl 1;\n" + flow); err != nil {
		t.Fatalf("create: %v\n%s", err, h.out.String())
	}
	before := h.snapshot()
	grown := strings.Replace(flow, "'a';\n", "'a';\n    if $N > 9 then\n      return;\n    end if;\n", 1)
	err := h.exec("mdl 1;\n" + grown)
	if err == nil || !strings.Contains(err.Error(), "would be drawn across the flow") {
		t.Fatalf("want the crossing refused, got %v\n%s", err, h.out.String())
	}
	if changed := before.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the refused grow wrote: %s", strings.Join(changed, "; "))
	}
	// Control: the same guard where its branch has free room is spliced.
	free := strings.Replace(flow, "  log info node 'X' 'c';\n", "  log info node 'X' 'c';\n  if $N > 9 then\n    return;\n  end if;\n", 1)
	if err := h.exec("mdl 1;\n" + free); err != nil {
		t.Fatalf("control: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 inserted)") {
		t.Errorf("control: want one insert, got:\n%s", h.out.String())
	}
}

// The condition of a Studio Pro-drawn decision with a caption of its own is
// set in place: every element stays, the caption is kept, and the edited
// description is a fixed point.
func TestSpliceRerun_StudioProDecisionConditionSetInPlace(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow Administration.ChangePassword"
	described := h.mustDescribeMdl0(t, target)
	const old = "if $AccountPasswordData/NewPassword = $AccountPasswordData/ConfirmPassword then"
	const cond = "if $AccountPasswordData/NewPassword = $AccountPasswordData/ConfirmPassword and $AccountPasswordData/NewPassword != empty then"
	edited := strings.Replace(described, old, cond, 1)
	if edited == described {
		t.Fatalf("describe output changed shape:\n%s", described)
	}
	before := h.flowUnit(t, "ChangePassword")
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 edited in place)") {
		t.Errorf("want the condition set, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "ChangePassword")
	requireKept(t, before, after, "")
	if len(elementIDs(t, after)) != len(elementIDs(t, before)) {
		t.Errorf("%d elements after, %d before", len(elementIDs(t, after)), len(elementIDs(t, before)))
	}
	again := h.mustDescribeMdl0(t, target)
	if !strings.Contains(again, cond) || !strings.Contains(again, "@caption 'Passwords equal?'") {
		t.Errorf("want the new condition and the stored caption:\n%s", again)
	}
	settled := h.snapshot()
	for _, script := range []string{edited, again} {
		if err := h.exec("mdl 1;\n" + script); err != nil {
			t.Fatalf("re-exec: %v\n%s", err, h.out.String())
		}
		if changed := settled.diff(h.snapshot()); len(changed) != 0 {
			t.Errorf("re-executing wrote: %s", strings.Join(changed, "; "))
		}
	}
}

// requireDecisionCases fails when a flow leaving a decision has no case value:
// Studio Pro reports CE0079 ("The 'false' condition value should be
// configured") for it, and nothing short of mx check notices.
func requireDecisionCases(t *testing.T, raw []byte) {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode unit: %v", err)
	}
	get := func(d bson.D, key string) any {
		for _, e := range d {
			if e.Key == key {
				return e.Value
			}
		}
		return nil
	}
	splits := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			if get(x, "$Type") == "Microflows$ExclusiveSplit" {
				if b, ok := get(x, "$ID").(bson.Binary); ok {
					splits[string(b.Data)] = true
				}
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, el := range x {
				walk(el)
			}
		}
	}
	walk(doc)
	flows, _ := get(doc, "Flows").(bson.A)
	for _, el := range flows {
		f, ok := el.(bson.D)
		if !ok || get(f, "$Type") != "Microflows$SequenceFlow" {
			continue
		}
		if b, ok := get(f, "OriginPointer").(bson.Binary); !ok || !splits[string(b.Data)] {
			continue
		}
		cased := false
		for _, key := range []string{"CaseValues", "NewCaseValue"} {
			var cases []any
			switch v := get(f, key).(type) {
			case bson.A:
				cases = v
			case bson.D:
				cases = []any{v}
			}
			for _, c := range cases {
				if cd, ok := c.(bson.D); ok && get(cd, "$Type") != "Microflows$NoCase" {
					cased = true
				}
			}
		}
		if !cased {
			t.Errorf("a flow leaving a decision has no case value (CE0079)")
		}
	}
}

// A replacement that ends in a guard leaves the stored flow out of it from the
// decision: that flow is the guard's false path, and is given its case.
func TestSpliceRerun_ReplaceEndingInGuard(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow Administration.ShowPasswordForm"
	described := withoutLayout(h.mustDescribeMdl0(t, target))
	const show = "  show page Administration.ChangePasswordForm(AccountPasswordData = $AccountPasswordData);\n"
	const repl = "  log info node 'Pwd' 'no page';\n  if $AccountPasswordData = empty then\n    return;\n  end if;\n"
	edited := strings.Replace(described, show, repl, 1)
	if edited == described {
		t.Fatalf("describe output changed shape:\n%s", described)
	}
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 replaced with new element IDs)") {
		t.Errorf("want one replace, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "ShowPasswordForm")
	requireDecisionCases(t, after)
	settled := h.snapshot()
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the second run wrote: %s", strings.Join(changed, "; "))
	}
}

// A guard spliced in directly before the final end is described as an
// if/else whose else returns; the script that wrote it, which states the
// guard and leaves the end implicit, is still Unchanged on its second run.
func TestSpliceRerun_GuardBeforeTheFinalEnd(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const stub = `create or modify microflow MyFirstModule.Grow_Last ($N: Integer)
begin
  log info node 'R' 'a';
  log info node 'R' 'b';
end;
`
	const guarded = `create or modify microflow MyFirstModule.Grow_Last ($N: Integer)
begin
  log info node 'R' 'a';
  if $N > 3 then
    return;
  end if;
end;
`
	if err := h.exec("mdl 1;\n" + stub); err != nil {
		t.Fatalf("stub: %v\n%s", err, h.out.String())
	}
	if err := h.exec("mdl 1;\n" + guarded); err != nil {
		t.Fatalf("guard: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 replaced with new element IDs)") {
		t.Errorf("want one replace, got:\n%s", h.out.String())
	}
	requireDecisionCases(t, h.flowUnit(t, "Grow_Last"))
	settled := h.snapshot()
	if err := h.exec("mdl 1;\n" + guarded); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the second run wrote: %s", strings.Join(changed, "; "))
	}
}

// A guard inserted at the start of an `if` branch is spliced onto the flow
// from the decision into that branch. That flow is the one the fragment goes
// on, so it is not a stored flow its return branch could be drawn across: it
// runs to the fragment's entry afterwards. Checked against its old course, it
// refused the grow ("would be drawn across the flow from the ExclusiveSplit")
// although nothing would cross.
func TestSpliceRerun_GrowBranchByGuard(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const flow = `create or modify microflow MyFirstModule.Grow_Branch ($N: Integer)
returns Boolean as $Done
begin
  if $N > 5 then
    log info node 'B' 'big';
  end if;
  log info node 'B' 'a';
  return false;
end;
`
	if err := h.exec("mdl 1;\n" + flow); err != nil {
		t.Fatalf("create: %v\n%s", err, h.out.String())
	}
	before := h.flowUnit(t, "Grow_Branch")
	grown := strings.Replace(flow, "    log info node 'B' 'big';\n",
		"    if $N > 10 then\n      return true;\n    end if;\n    log info node 'B' 'big';\n", 1)
	if err := h.exec("mdl 1;\n" + grown); err != nil {
		t.Fatalf("grow: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 inserted)") {
		t.Errorf("want one insert, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "Grow_Branch")
	requireKept(t, before, after, "")
	requireDecisionCases(t, after)
	if got := countType(t, after, "Microflows$EndEvent"); got != 2 {
		t.Errorf("%d end events after the grow, want 2", got)
	}
	settled := h.snapshot()
	if err := h.exec("mdl 1;\n" + grown); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the second run wrote: %s", strings.Join(changed, "; "))
	}
}
