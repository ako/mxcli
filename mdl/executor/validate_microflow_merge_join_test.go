// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func mergeJoinRuleHits(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var hits []string
	for _, v := range ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt)) {
		if strings.HasPrefix(v.RuleID, "MDL-FLOW0") {
			hits = append(hits, v.RuleID+": "+v.Message)
		}
	}
	return hits
}

// A `join` inside the custom error handler of a REST call is an incoming path
// of its merge. The label walk read handler bodies through a hand-kept list of
// statement types that had no REST call in it, so the join went unseen and the
// merge was reported as joined by nothing — on six Studio Pro-authored
// microflows in Evora Factory Management (OIDC.GET/POST/PUT/PATCH/DELETE,
// AWSAuthentication.POST_v1_GetCallerIdentity_ValidateCredentials) that build at
// 0 errors, which made exec refuse their own describe output.
func TestMergeJoinLabels_JoinInsideRestCallErrorHandler(t *testing.T) {
	src := `create microflow M.Retry ($Url: String)
returns Boolean
begin
  declare $Retried Boolean = false;
  merge rejoin1;
  $Response = call rest service get '{1}' with ({1} = $Url) (
    Timeout: 300,
  ) returns response on error without rollback begin
    if not($Retried) then
      set $Retried = true;
      join rejoin1;
    else
      return false;
    end if;
  end error;
  return true;
end;`
	if hits := mergeJoinRuleHits(t, src); len(hits) > 0 {
		t.Errorf("a join in a REST call's error handler was not counted: %v", hits)
	}
}

// Control: the rule still reports a merge that nothing joins.
func TestMergeJoinLabels_UnjoinedMergeStillReported(t *testing.T) {
	src := `create microflow M.Retry ($Url: String)
returns Boolean
begin
  declare $Retried Boolean = false;
  merge rejoin1;
  $Response = call rest service get '{1}' with ({1} = $Url) (
    Timeout: 300,
  ) returns response on error without rollback begin
    return false;
  end error;
  return true;
end;`
	hits := mergeJoinRuleHits(t, src)
	if len(hits) != 1 || !strings.Contains(hits[0], "nothing joins it") {
		t.Errorf("want one MDL-FLOW02 'nothing joins it', got %v", hits)
	}
}
