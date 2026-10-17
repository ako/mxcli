// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A compound statement inside a custom error handler (ako/mxcli#1456,
// mendixlabs/mxcli#1559). The handler walk stopped at the first merge it met,
// and the merge closing an `if` inside the handler is exactly that — so
// everything after `end if;` was dropped, and rebuilding the description
// rerouted the handler into the main flow. A loop came out as its bare header.
const handlerIfThenMoreMDL = `create microflow M.HandlerIf ($Flag: Boolean) returns Integer
begin
  $X = call microflow M.Sub(In = 1) on error without rollback begin
    if $Flag then
      log info node 'T' 'one';
    else
      log warning node 'T' 'two';
    end if;
    log info node 'T' 'after';
    return -1;
  end error;
  log info node 'T' 'done';
  return 1;
end;`

func TestDescribeErrorHandler_StatementsAfterIfAreKept(t *testing.T) {
	out, oc := describeBuilt(t, handlerIfThenMoreMDL)
	handler := handlerBlock(t, out)
	for _, want := range []string{"end if;", "'after'", "return -1;"} {
		if !strings.Contains(handler, want) {
			t.Errorf("handler lost %q after `end if;`:\n%s", want, out)
		}
	}
	assertDescriptionRebuildsGraph(t, out, oc)
}

const handlerIfNoElseMDL = `create microflow M.HandlerIfNoElse ($Flag: Boolean) returns Integer
begin
  $X = call microflow M.Sub(In = 1) on error without rollback begin
    if $Flag then
      log info node 'T' 'one';
    end if;
    return -1;
  end error;
  return 1;
end;`

func TestDescribeErrorHandler_ReturnAfterIfWithoutElseIsKept(t *testing.T) {
	out, oc := describeBuilt(t, handlerIfNoElseMDL)
	if !strings.Contains(handlerBlock(t, out), "return -1;") {
		t.Errorf("handler lost its `return` after `end if;`:\n%s", out)
	}
	assertDescriptionRebuildsGraph(t, out, oc)
}

const handlerLoopMDL = `create microflow M.HandlerLoop ($Items: List of M.Item) returns Integer
begin
  $X = call microflow M.Sub(In = 1) on error without rollback begin
    loop $Item in $Items begin
      log info node 'T' 'inside';
    end loop;
    return -1;
  end error;
  return 1;
end;`

func TestDescribeErrorHandler_LoopKeepsItsBody(t *testing.T) {
	out, oc := describeBuilt(t, handlerLoopMDL)
	handler := handlerBlock(t, out)
	for _, want := range []string{"loop $Item in $Items", "begin", "'inside'", "end loop;", "return -1;"} {
		if !strings.Contains(handler, want) {
			t.Errorf("handler loop is missing %q:\n%s", want, out)
		}
	}
	assertDescriptionRebuildsGraph(t, out, oc)
}

// handlerBlock returns the text between the first `on error … begin` and its
// `end error;`.
func handlerBlock(t *testing.T, out string) string {
	t.Helper()
	start := strings.Index(out, "on error without rollback begin")
	end := strings.Index(out, "end error;")
	if start < 0 || end < start {
		t.Fatalf("no error handler block in description:\n%s", out)
	}
	return out[start:end]
}
