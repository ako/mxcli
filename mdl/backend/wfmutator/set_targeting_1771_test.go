// SPDX-License-Identifier: Apache-2.0

package wfmutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
)

// ako/mxcli#1771: below Mendix 11.2 a stored user task holds its targeting as
// UserSource, not UserTargeting. `alter workflow … set activity '…' targeting
// …` set UserTargeting only where the key already existed, so on 9.24 and
// 10.24 it found none, changed nothing and still reported "Altered workflow"
// (measured: describe printed the old targeting afterwards). The edit replaces
// the stored targeting, whichever key holds it, in the current shape; the
// write lowers it to the project's (canon.LowerToProjectShape).
func TestWorkflowMutator_SetTargetingReplacesAStoredUserSource(t *testing.T) {
	for _, tc := range []struct {
		prop, value, wantType, valueKey string
	}{
		{"targeting_xpath", "[Name = 'reviewer']", "Workflows$XPathUserTargeting", "XPathConstraint"},
		{"targeting_microflow", "M.ACT_Reviewers", "Workflows$MicroflowUserTargeting", "Microflow"},
	} {
		for _, typ := range []string{"Workflows$UserTask", "Workflows$SingleUserTaskActivity"} {
			act := makeWfActivity(typ, "Review", "task1")
			act = append(act,
				bson.E{Key: "UserSource", Value: bson.D{{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "Workflows$XPathBasedUserSource"}, {Key: "XPathConstraint", Value: "[Name = 'admin']"}}},
				bson.E{Key: "UserTaskCompletion", Value: nil})
			sourceAt := keyIndex(act, "UserSource")
			m := newMutator(makeWorkflowDoc(act))

			if err := m.SetActivityProperty("Review", 0, tc.prop, tc.value); err != nil {
				t.Fatalf("%s on a %s: %v", tc.prop, typ, err)
			}
			actDoc, _ := m.findActivityByCaption("Review", 0)
			if bsonnav.DGet(actDoc, "UserSource") != nil {
				t.Errorf("%s on a %s: the stored UserSource is still there beside the new targeting", tc.prop, typ)
			}
			targeting := bsonnav.DGetDoc(actDoc, "UserTargeting")
			if got := bsonnav.DGetString(targeting, "$Type"); got != tc.wantType {
				t.Errorf("%s on a %s: targeting %q, want %s", tc.prop, typ, got, tc.wantType)
				continue
			}
			if got := bsonnav.DGetString(targeting, tc.valueKey); got != tc.value {
				t.Errorf("%s on a %s: %s = %q, want %q", tc.prop, typ, tc.valueKey, got, tc.value)
			}
			// In the stored key's place, so the lowered document keeps its order.
			if i := keyIndex(actDoc, "UserTargeting"); i != sourceAt {
				t.Errorf("%s on a %s: targeting at %d, want the stored source's place %d", tc.prop, typ, i, sourceAt)
			}
		}
	}
}

// A task holding neither key gets the targeting added.
func TestWorkflowMutator_SetTargetingAddsAMissingTargeting(t *testing.T) {
	m := newMutator(makeWorkflowDoc(makeWfActivity("Workflows$SingleUserTaskActivity", "Review", "task1")))
	if err := m.SetActivityProperty("Review", 0, "targeting_xpath", "[Name = 'reviewer']"); err != nil {
		t.Fatal(err)
	}
	actDoc, _ := m.findActivityByCaption("Review", 0)
	if got := bsonnav.DGetString(bsonnav.DGetDoc(actDoc, "UserTargeting"), "XPathConstraint"); got != "[Name = 'reviewer']" {
		t.Errorf("targeting %q, want it added", got)
	}
}

func keyIndex(d bson.D, key string) int {
	for i, e := range d {
		if e.Key == key {
			return i
		}
	}
	return -1
}
