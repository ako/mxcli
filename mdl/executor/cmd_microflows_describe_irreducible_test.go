// SPDX-License-Identifier: Apache-2.0

package executor

// DESCRIBE MICROFLOW on graphs whose branches do not nest, held to the one law
// that matters: executing the description rebuilds the stored graph
// (assertDescriptionRebuildsGraph). Each fixture is the reduced shape of a
// microflow in Evora Factory Management whose description `mxcli check`
// rejected — correctly, because it was not the stored flow.
//
// The fixtures are written in MDL with `merge`/`join`, which is how such a graph
// is authored today, and then described: the round trip is the test.

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// describeBuilt builds src and returns its description (body only) and the
// built collection, which stands in for the stored model.
func describeBuilt(t *testing.T, src string) (string, *microflows.MicroflowObjectCollection) {
	t.Helper()
	oc := buildMicroflowFromMDL(t, src)
	mf := &microflows.Microflow{ObjectCollection: oc}
	e := newTestExecutor()
	return strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n"), oc
}

// Shape 2 — a crossed merge walked THROUGH by a nested split's continuation.
//
// The #923 crossing (split1's false arm and split2's true... arm meet at
// `fail` before the shared tail) nested one level down, inside another
// decision. The nested split's continuation (continueAfterNestedSplitJoin)
// walked through the crossed tail merge instead of joining it, so the tail was
// printed inline there; the merge was then visited, its section never declared
// it, and the description ended in `merge shared1; join shared2;` with nothing
// declaring shared2 — MDL-FLOW02 and MDL003 (Evora: SnowflakeIntegration and
// SnowflakeRESTSQL Request_Validation).
const crossedInsideBranchMDL = `create microflow M.CrossedInsideBranch ($R: Boolean, $A: Boolean, $B: Boolean) returns Boolean
begin
  if $R then
    if $A then
      if $B then
        join ok_path;
      else
        log info node 'T' 'b failed';
        join failed_path;
      end if;
    else
      log info node 'T' 'a failed';
      join failed_path;
    end if;
  else
    log info node 'T' 'no request';
    return false;
  end if;
  merge failed_path;
  join ok_path;
  merge ok_path;
  log info node 'T' 'tail';
  return true;
end;`

func TestDescribeIrreducible_CrossedMergeInsideBranchIsJoinedNotWalked(t *testing.T) {
	out, oc := describeBuilt(t, crossedInsideBranchMDL)
	assertDescriptionRebuildsGraph(t, out, oc)
	if n := strings.Count(out, "node 'T' 'tail'"); n != 1 {
		t.Errorf("the shared tail is printed %d times, want once:\n%s", n, out)
	}
}

// Shape 2, the duplicating variant — the same crossing inside one arm of a
// multi-way split. Each arm of a `case` is walked with its own copy of the
// visited set, so the crossed tail walked through inline was NOT marked visited
// for the main traversal, and its section printed it a second time
// (Evora: AgentCommons.Tool_Validate, inside a `split type` arm: one stored
// call of MicroflowSelection_GetCreate, two in the description — MDL063).
const crossedInsideCaseArmMDL = `create microflow M.CrossedInsideCaseArm ($E: M.Kind, $A: Boolean, $B: Boolean) returns Boolean
begin
  case $E
    when One then
      if $A then
        if $B then
          join ok_path;
        else
          log info node 'T' 'b failed';
          join failed_path;
        end if;
      else
        log info node 'T' 'a failed';
        join failed_path;
      end if;
    when Two, (empty) then
      return false;
  end case;
  merge failed_path;
  join ok_path;
  merge ok_path;
  log info node 'T' 'tail';
  return true;
end;`

func TestDescribeIrreducible_CrossedMergeInsideCaseArmIsPrintedOnce(t *testing.T) {
	out, oc := describeBuilt(t, crossedInsideCaseArmMDL)
	assertDescriptionRebuildsGraph(t, out, oc)
	if n := strings.Count(out, "node 'T' 'tail'"); n != 1 {
		t.Errorf("the shared tail is printed %d times, want once:\n%s", n, out)
	}
}

// The regression control: a properly nested graph describes
// exactly as before. Pinned as text, because "no labels and no warnings" is
// the claim, and a graph check alone would accept a reshuffled description.
func TestDescribeIrreducible_NestedGraphUnchanged(t *testing.T) {
	const src = `create microflow M.Nested ($A: Boolean, $B: Boolean) returns Boolean
begin
  if $A then
    if $B then
      log info node 'T' 'both';
    else
      log info node 'T' 'a only';
    end if;
  else
    log info node 'T' 'neither';
  end if;
  return true;
end;`
	out, oc := describeBuilt(t, src)
	assertDescriptionRebuildsGraph(t, out, oc)
	for _, banned := range []string{"merge ", "join ", "while true", "WARNING"} {
		if strings.Contains(out, banned) {
			t.Errorf("a nested graph's description gained %q:\n%s", banned, out)
		}
	}
}
