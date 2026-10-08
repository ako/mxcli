// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// An error handler that goes back round: its path runs through a merge only it
// reaches (a no-op merge, one way in — Studio Pro leaves these behind when a
// flow is re-routed) and on to the loop header, so the failed call is retried.
//
// Evora: SnowflakeRESTSQL.GET_v1_RetrievePartition — on a 401 with $Retry set,
// the handler sets $Retry to false and goes back to regenerate the token. The
// handler walk stopped at the first merge it met, the private one, which has no
// label, so the block ended there: `set $Retry = false` and the way back round
// were gone, and re-executing the description turns a retry into a silent
// fall-through.
const handlerRetryMDL = `create microflow M.HandlerRetry () returns Boolean
begin
  declare $Retry Boolean = true;
  merge again;
  log info node 'T' 'token';
  $R = call microflow M.Sub(In = 1) on error without rollback begin
    if $Retry then
      join hop;
    else
      raise error;
    end if;
  end error;
  return true;
  merge hop;
  set $Retry = false;
  join again;
end;`

func TestDescribeHandlerRetry_WalksThroughAPrivateMergeBackToTheHeader(t *testing.T) {
	out, oc := describeBuilt(t, handlerRetryMDL)
	assertDescriptionRebuildsGraph(t, out, oc)
	if !strings.Contains(out, "set $Retry = false;") {
		t.Errorf("the handler's retry step is missing:\n%s", out)
	}
}

// The describer's answer for GET_v1_RetrievePartition — `merge rejoin1;` before
// a `while true` whose REST call's handler joins it — must pass the label check
// exec runs. It reported "merge rejoin1: nothing joins it" (MDL-FLOW02) for two
// reasons, either enough: the label walk read handler bodies through
// getErrorHandlerBody, which does not know a REST call has one, and it treated
// every `while` as a loop activity, where a `while true` around a return is a
// merge and a back-edge in the enclosing graph.
func TestCheckMergeJoin_JoinInARestHandlerInsideWhileTrue(t *testing.T) {
	src := `create microflow M.Retry ($Retry: Boolean) returns Boolean
begin
  merge again;
  log info node 'T' 'token';
  while true
  begin
    $R = call rest service get 'http://x' returns response on error without rollback begin
      if $Retry then
        join again;
      else
        raise error;
      end if;
    end error;
    if $R/StatusCode = 200 then
      return true;
    end if;
  end while;
end;`
	for _, v := range checkMicroflowSource(t, src) {
		if strings.HasPrefix(v.RuleID, "MDL-FLOW") {
			t.Errorf("%s: %s", v.RuleID, v.Message)
		}
	}
}

// Control: a label inside a real loop activity is still refused, including one
// in a handler there — a sequence flow cannot leave a loop's collection.
func TestCheckMergeJoin_JoinInAHandlerInsideALoopIsRefused(t *testing.T) {
	src := `create microflow M.InLoop ($L: List of M.E) returns Boolean
begin
  merge outside;
  loop $I in $L
  begin
    $R = call rest service get 'http://x' returns response on error without rollback begin
      join outside;
    end error;
  end loop;
  return true;
end;`
	found := false
	for _, v := range checkMicroflowSource(t, src) {
		if v.RuleID == "MDL-FLOW04" {
			found = true
		}
	}
	if !found {
		t.Error("a join in a handler inside a loop activity was not refused (MDL-FLOW04)")
	}
}
