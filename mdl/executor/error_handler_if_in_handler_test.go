// SPDX-License-Identifier: Apache-2.0

package executor

// An `if` without `else` whose THEN ends in RETURN has no merge: its FALSE edge
// is left pending, as the case the NEXT flow must carry. Every body loop takes
// that case except the one that builds a custom error handler's body, which
// drew the next edge as a plain flow — so inside `on error … end error` the
// decision left on `true` and NoCase, and mx check failed with CE0079 at the
// decision and CE0773 at the flow (ako/mxcli#1455, mendixlabs/mxcli#1558).
// check and exec accepted it, and DESCRIBE read the NoCase edge back as an
// ELSE, so the tests here assert on the edges themselves.

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// flowCaseLabel is the case a flow carries, as the decision it leaves sees it:
// "true"/"false" (or an enumeration value), or "" for no case at all.
func flowCaseLabel(cv microflows.CaseValue) string {
	switch c := cv.(type) {
	case *microflows.ExpressionCase:
		return c.Expression
	case microflows.ExpressionCase:
		return c.Expression
	case microflows.EnumerationCase:
		return c.Value
	case *microflows.EnumerationCase:
		return c.Value
	case microflows.BooleanCase:
		if c.Value {
			return "true"
		}
		return "false"
	case *microflows.BooleanCase:
		if c.Value {
			return "true"
		}
		return "false"
	}
	return ""
}

// decisionCases returns, per decision caption, the sorted cases of its
// outgoing normal flows.
func decisionCases(oc *microflows.MicroflowObjectCollection) map[string][]string {
	splits := map[model.ID]string{}
	for _, o := range oc.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok {
			splits[s.ID] = s.Caption
		}
	}
	got := map[string][]string{}
	for _, f := range oc.Flows {
		caption, ok := splits[f.OriginID]
		if !ok || f.IsErrorHandler {
			continue
		}
		got[caption] = append(got[caption], flowCaseLabel(f.CaseValue))
	}
	for k := range got {
		sort.Strings(got[k])
	}
	return got
}

func assertBooleanDecisions(t *testing.T, oc *microflows.MicroflowObjectCollection, captions ...string) {
	t.Helper()
	got := decisionCases(oc)
	for _, c := range captions {
		cases, ok := got[c]
		if !ok {
			t.Fatalf("no decision captioned %q; decisions: %v", c, got)
		}
		if strings.Join(cases, ",") != "false,true" {
			t.Errorf("decision %q leaves on cases %q, want [false true] — an empty case is NoCase (CE0079/CE0773)", c, cases)
		}
	}
}

func TestIfWithoutElseInErrorHandlerKeepsItsFalseCase(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		captions []string
	}{
		{
			// The report's first microflow, and the control beside it: the same
			// `if` in the main flow was always stored with true/false.
			name: "call microflow handler, then returns",
			src: `create microflow M.CallWithHandler ($Flag: Boolean) returns Integer
begin
  $Value = call microflow M.Target(Flag = $Flag)
    on error without rollback begin
      if $Flag then
        return 1;
      end if;
      return -1;
    end error;
  if $Value > 0 then
    return 2;
  end if;
  return $Value;
end;`,
			captions: []string{"$Flag", "$Value > 0"},
		},
		{
			// The report's second microflow.
			name: "rest call handler on $latestHttpResponse",
			src: `create microflow M.RestWithHandler () returns Integer
begin
  $Response = call rest service get 'https://example.com/x' (
    Timeout: 30,
  ) returns response
    on error without rollback begin
      if $latestHttpResponse != empty then
        return $latestHttpResponse/StatusCode;
      end if;
      return -1;
    end error;
  return $Response/StatusCode;
end;`,
			captions: []string{"$latestHttpResponse != empty"},
		},
		{
			// Not ending in RETURN: the handler falls through after the guard.
			name: "fall-through handler continues after the guard",
			src: `create microflow M.FallThrough ($Flag: Boolean)
begin
  call microflow M.Callee0 ()
    on error without rollback begin
      if $Flag then
        return;
      end if;
      log warning node 'P' 'failed';
    end error;
  log info node 'P' 'end';
end;`,
			captions: []string{"$Flag"},
		},
		{
			// Two guards in a row: the second one is entered on the first one's
			// FALSE edge, and leaves on its own.
			name: "two guards in one handler",
			src: `create microflow M.TwoGuards ($Flag: Boolean, $Flag2: Boolean) returns Integer
begin
  $Value = call microflow M.Target(Flag = $Flag)
    on error without rollback begin
      if $Flag then
        return 1;
      end if;
      if $Flag2 then
        return 2;
      end if;
      return -1;
    end error;
  return $Value;
end;`,
			captions: []string{"$Flag", "$Flag2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oc := buildProbeFromMDL(t, tc.src)
			assertBooleanDecisions(t, oc, tc.captions...)

			// DESCRIBE must spell it so that it rebuilds with the same cases.
			text := describeCollection(t, oc)
			header := tc.src[:strings.Index(tc.src, "begin")]
			rebuilt := buildProbeFromMDL(t, header+"begin\n"+text+"\nend;")
			assertBooleanDecisions(t, rebuilt, tc.captions...)
		})
	}
}

// The guard's FALSE edge is deferred with its @anchor as well as its case; the
// handler loop must apply that anchor to the edge it draws, as every other body
// loop does (pendingFlowAnchors).
func TestIfWithoutElseInErrorHandlerKeepsItsFalseAnchor(t *testing.T) {
	src := `create microflow M.Anchored ($Flag: Boolean) returns Integer
begin
  $Value = call microflow M.Target(Flag = $Flag)
    on error without rollback begin
      @anchor(false: (from: bottom, to: top))
      if $Flag then
        return 1;
      end if;
      return -1;
    end error;
  return $Value;
end;`
	oc := buildProbeFromMDL(t, src)
	assertBooleanDecisions(t, oc, "$Flag")

	var split model.ID
	for _, o := range oc.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok && s.Caption == "$Flag" {
			split = s.ID
		}
	}
	for _, f := range oc.Flows {
		if f.OriginID != split || flowCaseLabel(f.CaseValue) != "false" {
			continue
		}
		if f.OriginConnectionIndex != AnchorBottom || f.DestinationConnectionIndex != AnchorTop {
			t.Errorf("false edge anchored %d→%d, want bottom (%d) → top (%d) as the @anchor states",
				f.OriginConnectionIndex, f.DestinationConnectionIndex, AnchorBottom, AnchorTop)
		}
		return
	}
	t.Fatal("no false edge leaves the decision")
}
