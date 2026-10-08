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

// Shape 1 — two arms of a `case` share an activity before the case's join,
// and a third arm returns, which makes the overlap interleaved (it has two
// entries) and so not something Mode 2 labelled. Every arm was walked with
// its own visited set, so the shared activity was printed once per arm
// (Evora: OIDC.handleAuthorizationCode — the token REST call twice, so the
// description held an activity the model does not have, and MDL063).
const sharedCaseArmsMDL = `create microflow M.SharedCaseArms ($E: M.Kind) returns Boolean
begin
  case $E
    when One then
      log info node 'T' 'one';
      join shared_path;
    when Two then
      log info node 'T' 'two';
      join shared_path;
    when Three then
      log info node 'T' 'three';
      join rejoin_path;
    when Four, (empty) then
      return false;
  end case;
  merge shared_path;
  log info node 'T' 'shared';
  join rejoin_path;
  merge rejoin_path;
  log info node 'T' 'tail';
  return true;
end;`

func TestDescribeIrreducible_SharedCaseArmsArePrintedOnce(t *testing.T) {
	out, oc := describeBuilt(t, sharedCaseArmsMDL)
	assertDescriptionRebuildsGraph(t, out, oc)
	// Every arm now says where it goes, so the description IS the microflow
	// and the MDL-FLOW01 refusal ("must not be re-executed") would be untrue.
	if strings.Contains(out, "NOT equivalent") {
		t.Errorf("a faithful description still carries the #923 refusal:\n%s", out)
	}
	for _, want := range []string{"'shared'", "'tail'"} {
		if n := strings.Count(out, "node 'T' "+want); n != 1 {
			t.Errorf("%s is printed %d times, want once:\n%s", want, n, out)
		}
	}
}

// Shape 3 — a manual loop whose header is reached inside a branch. The top
// level writes `while true` for such a merge; a branch walked through it as a
// plain merge, so the way round came back to an activity already printed and
// stopped in silence. The description had no back-edge and re-executed to a
// flow that ENDS where the original goes round again (Evora:
// AmazonBedrockConnector.AmazonBedrockAgent_Sync and _KnowledgeBase_Sync, the
// pagination loop, MDL003). `while true` with a `return` inside is how such a
// loop is authored: the builder writes a merge and a back-edge, not a loop
// activity.
const loopInsideBranchMDL = `create microflow M.LoopInsideBranch ($R: Boolean) returns Integer
begin
  if $R then
    declare $N Integer = 0;
    while true
    begin
      set $N = $N + 1;
      if $N > 3 then
        return $N;
      end if;
    end while;
  else
    return 0;
  end if;
end;`

func TestDescribeIrreducible_LoopInsideBranchKeepsItsBackEdge(t *testing.T) {
	out, oc := describeBuilt(t, loopInsideBranchMDL)
	assertDescriptionRebuildsGraph(t, out, oc)
}

// The regression control for all three: a properly nested graph describes
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
