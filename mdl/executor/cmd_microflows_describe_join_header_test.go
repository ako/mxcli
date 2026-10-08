// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A decision's join that is also a loop header: the branches of an `if` meet
// at a merge, and a later path comes back round to that same merge.
//
// Evora: SnowflakeRESTSQL.POST_v1_ExecuteStatement — the request body is
// optionally rebound, then the REST call is polled every 5 s while it answers
// 202. Studio Pro draws one merge for both jobs. DESCRIBE paired it with the
// `if` and walked on after `end if`, so the poll's way back came round to an
// activity already printed and stopped: the description had no back-edge, and
// re-executed it calls the service once and then carries on as if it had
// answered.
//
// MDL writes the two jobs as two statements, an `if` and a `while true`, so the
// fixture is built that way and the two merges then folded into one — exactly
// the stored shape.
const joinIsHeaderMDL = `create microflow M.JoinIsHeader ($V: Boolean) returns Integer
begin
  declare $N Integer = 0;
  if $V then
    set $N = 1;
  end if;
  while true
  begin
    set $N = $N + 1;
    if $N > 3 then
      return $N;
    end if;
    log info node 'T' 'again';
  end while;
end;`

func TestDescribeJoinIsLoopHeader_KeepsTheBackEdge(t *testing.T) {
	oc := buildMicroflowFromMDL(t, joinIsHeaderMDL)
	if !foldMergeIntoMerge(oc) {
		t.Fatal("fixture: no merge leading straight into another merge")
	}
	mf := &microflows.Microflow{ObjectCollection: oc}
	e := newTestExecutor()
	out := strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n")
	assertDescriptionRebuildsGraph(t, out, oc)
}

// foldMergeIntoMerge removes the first merge whose only way out is straight
// into another merge, pointing its incoming flows at that merge instead.
func foldMergeIntoMerge(oc *microflows.MicroflowObjectCollection) bool {
	objects := map[model.ID]microflows.MicroflowObject{}
	for _, o := range oc.Objects {
		objects[o.GetID()] = o
	}
	out := map[model.ID][]*microflows.SequenceFlow{}
	for _, f := range oc.Flows {
		out[f.OriginID] = append(out[f.OriginID], f)
	}
	for _, o := range oc.Objects {
		if _, ok := o.(*microflows.ExclusiveMerge); !ok || len(out[o.GetID()]) != 1 {
			continue
		}
		next := out[o.GetID()][0]
		if _, ok := objects[next.DestinationID].(*microflows.ExclusiveMerge); !ok {
			continue
		}
		gone := o.GetID()
		var flows []*microflows.SequenceFlow
		for _, f := range oc.Flows {
			if f == next {
				continue
			}
			if f.DestinationID == gone {
				f.DestinationID = next.DestinationID
			}
			flows = append(flows, f)
		}
		oc.Flows = flows
		var objs []microflows.MicroflowObject
		for _, x := range oc.Objects {
			if x.GetID() != gone {
				objs = append(objs, x)
			}
		}
		oc.Objects = objs
		return true
	}
	return false
}
