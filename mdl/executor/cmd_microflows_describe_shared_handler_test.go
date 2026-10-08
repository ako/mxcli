// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// One error handler shared by several activities: each activity's error flow
// lands on the same merge, and the handler body hangs off that merge.
//
// Evora: PrePopulateData.ASU_CheckForWorkforce (three java action calls, one
// handler: log + end) and GenAICommons.ToolCall_ProcessAndExecuteTool (two
// calls, one handler of four activities rejoining the main path). Each
// activity's `on error … begin end error` block came out EMPTY, because the
// handler walk stops at the first merge it meets and this merge — reached by
// no normal path — had no label to `join`. The shared handler, every activity
// in it, simply vanished from the description: re-executing it deletes them.
const sharedHandlerMDL = `create microflow M.SharedHandler ($A: Boolean) returns Boolean
begin
  $X = call microflow M.Sub(In = 1) on error without rollback begin
    join onfail;
  end error;
  $Y = call microflow M.Sub(In = 2) on error without rollback begin
    join onfail;
  end error;
  return true;
  merge onfail;
  log error node 'T' 'failed';
  return false;
end;`

func TestDescribeSharedErrorHandler_PrintedOnceAndJoined(t *testing.T) {
	out, oc := describeBuilt(t, sharedHandlerMDL)
	assertDescriptionRebuildsGraph(t, out, oc)
	if n := strings.Count(out, "node 'T' 'failed'"); n != 1 {
		t.Errorf("the shared handler is printed %d times, want once:\n%s", n, out)
	}
}
