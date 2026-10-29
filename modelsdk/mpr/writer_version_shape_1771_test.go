// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
)

// ako/mxcli#1771: a workflow user task as mxcli builds it is the 11.2 shape,
// and Mendix stored a user task differently before two version boundaries.
// Measured with `mx convert -p` on Studio Pro-authored projects (the
// Marketplace's Employee Onboarding, 9.1.1, XPath targeting; Multilevel Dynamic
// Workflow, 9.24.3, microflow targeting), converted 9.24.30 → 10.24.24 → 11.13.0:
//
//	9.x–10.11   Workflows$UserTask             UserSource + UserTaskCompletion
//	10.12–11.1  Workflows$SingleUserTaskActivity UserSource
//	11.2+       Workflows$SingleUserTaskActivity UserTargeting
//
// The targeting value is carried byte-identical across both conversions; only
// its holder changes: XPathBasedUserSource ↔ XPathUserTargeting and
// MicroflowBasedUserSource ↔ MicroflowUserTargeting, each {$ID, $Type, value}.

var eidSeq byte

// eid is a distinct, non-zero element $ID, so a test can tell elements apart
// and follow a pointer.
func eid() bson.Binary {
	eidSeq++
	return bson.Binary{Subtype: 0, Data: []byte{0x17, 0x71, eidSeq, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, eidSeq}}
}

func xpathUserTargeting(xpath string) bson.D {
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$XPathUserTargeting"}, {Key: "XPathConstraint", Value: xpath}}
}

func microflowUserTargeting(mf string) bson.D {
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$MicroflowUserTargeting"}, {Key: "Microflow", Value: mf}}
}

func noUserTargeting() bson.D {
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$NoUserTargeting"}}
}

func stringTemplate(text string) bson.D {
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Microflows$StringTemplate"},
		{Key: "Parameters", Value: bson.A{int32(2)}}, {Key: "Text", Value: text}}
}

func userTaskOutcome(value string) bson.D {
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$UserTaskOutcome"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$Flow"}, {Key: "Activities", Value: bson.A{int32(3)}}}},
		{Key: "PersistentId", Value: pid()}, {Key: "Value", Value: value}}
}

// userTaskDoc is a user task as userTaskToGen builds it (below 11.1, where the
// encoder leaves out RelativeMiddlePoint and Size): the keys it writes, in its
// order, with the codec's BoundaryEvents and Annotation defaults last. multi
// adds AwaitAllUsers, a consensus CompletionCriteria falling back to the first
// outcome, and TargetUserInput, as a `multi user task` with no rule.
func userTaskDoc(name string, multi bool, targeting bson.D) bson.D {
	typ := "Workflows$SingleUserTaskActivity"
	if multi {
		typ = "Workflows$MultiUserTaskActivity"
	}
	approve, reject := userTaskOutcome("Approve"), userTaskOutcome("Reject")
	d := bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: typ},
		{Key: "AutoAssignSingleTargetUser", Value: false}}
	if multi {
		d = append(d, bson.E{Key: "AwaitAllUsers", Value: false})
	}
	d = append(d, bson.E{Key: "Caption", Value: name})
	if multi {
		approveID, _ := get(approve, "$ID")
		d = append(d, bson.E{Key: "CompletionCriteria", Value: bson.D{{Key: "$ID", Value: eid()},
			{Key: "$Type", Value: "Workflows$ConsensusCompletionCriteria"}, {Key: "FallbackOutcomePointer", Value: approveID}}})
	}
	d = append(d,
		bson.E{Key: "DueDate", Value: ""},
		bson.E{Key: "Name", Value: name},
		bson.E{Key: "OnCreatedEvent", Value: bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$NoEvent"}}},
		bson.E{Key: "Outcomes", Value: bson.A{int32(3), approve, reject}},
		bson.E{Key: "PersistentId", Value: pid()},
		bson.E{Key: "TaskDescription", Value: stringTemplate("")},
		bson.E{Key: "TaskName", Value: stringTemplate(name)},
		bson.E{Key: "TaskPage", Value: bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$PageReference"}, {Key: "Page", Value: "M.TaskPage"}}})
	if multi {
		d = append(d, bson.E{Key: "TargetUserInput", Value: bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$AllUserInput"}}})
	}
	return append(d,
		bson.E{Key: "UserTargeting", Value: targeting},
		bson.E{Key: "BoundaryEvents", Value: bson.A{int32(2)}},
		bson.E{Key: "Annotation", Value: nil})
}

// workflowWithUserTasks wraps tasks in a workflow unit.
func workflowWithUserTasks(tasks ...bson.D) bson.D {
	acts := bson.A{int32(3)}
	for _, t := range tasks {
		acts = append(acts, t)
	}
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$Flow"}, {Key: "Activities", Value: acts}}},
		{Key: "Name", Value: "WF"},
		{Key: "Title", Value: "WF"}}
}

// findNamed returns the first document in v named name.
func findNamed(v any, name string) bson.D {
	switch x := v.(type) {
	case bson.D:
		if n, _ := get(x, "Name"); n == name {
			return x
		}
		for _, e := range x {
			if d := findNamed(e.Value, name); d != nil {
				return d
			}
		}
	case bson.A:
		for _, el := range x {
			if d := findNamed(el, name); d != nil {
				return d
			}
		}
	}
	return nil
}

// sameID reports whether two stored $IDs (bson.Binary, not comparable with ==)
// are the same.
func sameID(a, b any) bool {
	x, ok1 := a.(bson.Binary)
	y, ok2 := b.(bson.Binary)
	return ok1 && ok2 && bytes.Equal(x.Data, y.Data)
}

func vlabel(v *version.ProjectVersion) string {
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.PatchVersion)
}

func sortedKeys(d bson.D) string {
	ks := keysOf(d)
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

// User targeting is UserTargeting from 11.2 and UserSource before it
// (mendixmodelsdk: UserTaskActivity.userSource deleted, userTargeting
// introduced 11.2.0). UserTargeting is declared on the abstract
// UserTaskActivity, which the floor does not look up, so it was written to
// every version: on 10.12.19 and 10.24.24 the task then had no UserSource, and
// mx check reported CE1859 "XPath constraint is required" on every user task —
// the microflow-targeted ones too — while `mx convert` 10.24.24 dropped the
// targeting and stored an empty XPathBasedUserSource in its place.
func TestNormalize_UserTaskTargetingBelowMendix11_2(t *testing.T) {
	kinds := []struct {
		name       string
		targeting  func() bson.D
		sourceType string
		valueKey   string
		value      string
	}{
		{"users xpath", func() bson.D { return xpathUserTargeting("[Name = 'admin']") }, "Workflows$XPathBasedUserSource", "XPathConstraint", "[Name = 'admin']"},
		{"users microflow", func() bson.D { return microflowUserTargeting("M.ACT_GetUsers") }, "Workflows$MicroflowBasedUserSource", "Microflow", "M.ACT_GetUsers"},
		{"none", noUserTargeting, "Workflows$EmptyUserSource", "", ""},
	}
	for _, tt := range []struct {
		v       *version.ProjectVersion
		lowered bool
	}{
		{pv(9, 24, 30), true},
		{pv(10, 12, 19), true},
		{pv(10, 24, 24), true},
		{pv(11, 1, 0), true},
		{pv(11, 2, 0), false},
		{pv(11, 13, 0), false}, // control
	} {
		for _, k := range kinds {
			for _, multi := range []bool{false, true} {
				targeting := k.targeting()
				wantID, _ := get(targeting, "$ID")
				d, err := normalizeShape(t, tt.v, workflowWithUserTasks(userTaskDoc("Task", multi, targeting)))
				label := vlabel(tt.v) + " " + k.name
				if multi {
					label += " (multi)"
				}
				if err != nil {
					t.Errorf("%s: refused: %v", label, err)
					continue
				}
				task := findNamed(d, "Task")
				_, hasTargeting := get(task, "UserTargeting")
				srcAny, hasSource := get(task, "UserSource")
				if !tt.lowered {
					if !hasTargeting || hasSource {
						t.Errorf("%s: keys %v, want UserTargeting and no UserSource", label, keysOf(task))
					}
					continue
				}
				if hasTargeting || !hasSource {
					t.Errorf("%s: keys %v, want UserSource and no UserTargeting", label, keysOf(task))
					continue
				}
				src, _ := srcAny.(bson.D)
				if typ, _ := get(src, "$Type"); typ != k.sourceType {
					t.Errorf("%s: UserSource is %v, want %s", label, typ, k.sourceType)
				}
				if id, _ := get(src, "$ID"); !sameID(id, wantID) {
					t.Errorf("%s: UserSource $ID %v, want the targeting's own %v", label, id, wantID)
				}
				wantKeys := "$ID,$Type"
				if k.valueKey != "" {
					wantKeys += "," + k.valueKey
					if v, _ := get(src, k.valueKey); v != k.value {
						t.Errorf("%s: %s = %q, want %q", label, k.valueKey, v, k.value)
					}
				}
				if got := strings.Join(keysOf(src), ","); got != wantKeys {
					t.Errorf("%s: UserSource keys %s, want %s", label, got, wantKeys)
				}
			}
		}
	}
}

// Group targeting is 11.2 (XPathGroupTargeting, MicroflowGroupTargeting, and
// the workflow groups they target): before, a user task targets users only.
// Refused naming the task, not written as a targeting the version reads as
// "XPath constraint is required".
func TestNormalize_RefusesGroupTargetingBelowMendix11_2(t *testing.T) {
	group := func(typ, key, value string) bson.D {
		return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: typ}, {Key: key, Value: value}}
	}
	for _, g := range []bson.D{
		group("Workflows$XPathGroupTargeting", "XPathConstraint", "[Name = 'Reviewers']"),
		group("Workflows$MicroflowGroupTargeting", "Microflow", "M.ACT_GetGroups"),
	} {
		for _, v := range []*version.ProjectVersion{pv(10, 24, 24), pv(11, 1, 0)} {
			_, err := normalizeShape(t, v, workflowWithUserTasks(userTaskDoc("ByGroup", false, g)))
			if err == nil || !strings.Contains(err.Error(), "ByGroup") || !strings.Contains(err.Error(), "11.2") {
				t.Errorf("%s: group targeting on %s was not refused naming the task and 11.2: %v", g[1].Value, vlabel(v), err)
			}
		}
		if _, err := normalizeShape(t, pv(11, 2, 0), workflowWithUserTasks(userTaskDoc("ByGroup", false, g))); err != nil {
			t.Errorf("%s: refused on 11.2: %v", g[1].Value, err)
		}
	}
}
