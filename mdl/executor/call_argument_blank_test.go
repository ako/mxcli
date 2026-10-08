// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A call parameter whose argument field Studio Pro left blank is stored as a
// parameter mapping with `Argument: ""` — the mapping is present, the
// expression is not (Evora Factory Management: GenAICommons.Request_Create's
// `ID`, ConversationalUI.SpanTreeView_Create's `TraceID`, ten flows in all).
// describe printed it as `ID = )` / `TraceID = ,`, which does not parse.
//
// The spelling has to round-trip to the same stored document (ADR-0008), and
// the existing spellings cannot: `ID = empty` stores the Mendix value `empty`
// (a different, legitimate document Studio Pro also writes), and omitting the
// argument drops the mapping. So a blank argument is `ID = nothing`, which
// stores `""`.

func TestFormatAction_MicroflowCall_BlankArgumentIsNothing(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.MicroflowCallAction{
		MicroflowCall: &microflows.MicroflowCall{
			Microflow: "M.Create",
			ParameterMappings: []*microflows.MicroflowCallParameterMapping{
				{Parameter: "M.Create.SpanID", Argument: "$Span/SpanId"},
				{Parameter: "M.Create.TraceID", Argument: ""},
				{Parameter: "M.Create.Note", Argument: "empty"},
				{Parameter: "M.Create.Last", Argument: "  "},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "call microflow M.Create(SpanID = $Span/SpanId, TraceID = nothing, Note = empty, Last = nothing);"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestFormatAction_NanoflowCall_BlankArgumentIsNothing(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.NanoflowCallAction{
		NanoflowCall: &microflows.NanoflowCall{
			Nanoflow: "M.NF",
			ParameterMappings: []*microflows.NanoflowCallParameterMapping{
				{Parameter: "M.NF.A", Argument: ""},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	if !strings.Contains(got, "(A = nothing)") {
		t.Errorf("got %q, want the blank argument spelled `A = nothing`", got)
	}
}

// TestBuildCall_NothingStoresBlankArgument is the write half: `nothing` must
// store the empty string, in its own position, and must not be confused with
// `empty`.
func TestBuildCall_NothingStoresBlankArgument(t *testing.T) {
	fb := buildFlowFromMDL(t, "  call microflow M.Create(SpanID = 'a', TraceID = nothing, Note = empty);\n"+
		"  call nanoflow M.NF(A = nothing);")
	var mf []*microflows.MicroflowCallParameterMapping
	var nf []*microflows.NanoflowCallParameterMapping
	for _, o := range fb.objects {
		a, ok := o.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch act := a.Action.(type) {
		case *microflows.MicroflowCallAction:
			mf = act.MicroflowCall.ParameterMappings
		case *microflows.NanoflowCallAction:
			nf = act.NanoflowCall.ParameterMappings
		}
	}
	if len(mf) != 3 {
		t.Fatalf("microflow call mappings: got %d, want 3", len(mf))
	}
	for i, want := range []struct{ param, arg string }{
		{"M.Create.SpanID", "'a'"}, {"M.Create.TraceID", ""}, {"M.Create.Note", "empty"},
	} {
		if mf[i].Parameter != want.param || mf[i].Argument != want.arg {
			t.Errorf("mapping %d = (%q, %q), want (%q, %q)", i, mf[i].Parameter, mf[i].Argument, want.param, want.arg)
		}
	}
	if len(nf) != 1 || nf[0].Argument != "" {
		t.Errorf("nanoflow call mappings = %+v, want one blank argument", nf)
	}
}

// TestNothingArgumentRefusedOutsideFlowCalls: the blank argument is a
// microflow/nanoflow-call concept. A Java action has its own unbound-argument
// spelling (`empty`, PROPOSAL_microflow_empty_java_action_argument.md) and
// other calls store arguments in shapes where a blank is not a meaningful
// value, so `nothing` there is refused rather than stored as something.
func TestNothingArgumentRefusedOutsideFlowCalls(t *testing.T) {
	_, errs := visitor.Build("create microflow M.ACT_T()\nbegin\n  call java action M.JA(P = nothing);\nend;")
	if len(errs) == 0 {
		t.Fatal("`P = nothing` in a java action call was accepted")
	}
	if !strings.Contains(errs[0].Error(), "nothing") {
		t.Errorf("error does not name the spelling: %v", errs[0])
	}
}
