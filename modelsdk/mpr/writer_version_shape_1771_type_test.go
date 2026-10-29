// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
)

// studioPro924UserTask is the key set Studio Pro 9.24.3 stores on a user task
// (Multilevel Dynamic Workflow, Marketplace 221741). Employee Onboarding
// (9.1.1), converted by mx convert 9.24.30, stores the same set.
const studioPro924UserTask = "$ID,$Type,Annotation,AutoAssignSingleTargetUser,Caption,DueDate,Name,OnCreatedEvent," +
	"Outcomes,TaskDescription,TaskName,TaskPage,UserSource,UserTaskCompletion"

// Below 10.12 a user task is a Workflows$UserTask. mendixmodelsdk deletes
// UserTask and introduces SingleUserTaskActivity and MultiUserTaskActivity in
// 10.12.0. Whether the task is single- or multi-user is its UserTaskCompletion
// (9.22.0): a SingleInputCompletion, or a MultiInputCompletion holding the
// completion criteria, the participants and, from 10.2.0, AwaitAllUsers.
// mxcli wrote the 10.12 types on every version, and 9.24.30 then did not load
// the project (TypeCacheUnknownTypeException … Workflows$SingleUserTaskActivity).
func TestNormalize_UserTaskIsAWorkflowsUserTaskBelowMendix10_12(t *testing.T) {
	for _, tt := range []struct {
		v        *version.ProjectVersion
		userTask bool
	}{
		{pv(9, 24, 30), true},
		{pv(10, 6, 0), true},
		{pv(10, 11, 0), true},
		{pv(10, 12, 0), false},
		{pv(10, 24, 24), false}, // control
	} {
		d, err := normalizeShape(t, tt.v, workflowWithUserTasks(userTaskDoc("Single", false, xpathUserTargeting("[Name = 'admin']"))))
		if err != nil {
			t.Errorf("%s: refused: %v", vlabel(tt.v), err)
			continue
		}
		task := findNamed(d, "Single")
		typ, _ := get(task, "$Type")
		if !tt.userTask {
			if typ != "Workflows$SingleUserTaskActivity" {
				t.Errorf("%s: $Type %v, want Workflows$SingleUserTaskActivity", vlabel(tt.v), typ)
			}
			if _, ok := get(task, "UserTaskCompletion"); ok {
				t.Errorf("%s: a SingleUserTaskActivity holds a UserTaskCompletion", vlabel(tt.v))
			}
			continue
		}
		if typ != "Workflows$UserTask" {
			t.Errorf("%s: $Type %v, want Workflows$UserTask", vlabel(tt.v), typ)
		}
		if got := sortedKeys(task); got != studioPro924UserTask {
			t.Errorf("%s: user task keys\n  %s\nwant Studio Pro 9.24's\n  %s", vlabel(tt.v), got, studioPro924UserTask)
		}
		completion, _ := get(task, "UserTaskCompletion")
		cd, _ := completion.(bson.D)
		if ct, _ := get(cd, "$Type"); ct != "Workflows$SingleInputCompletion" || len(cd) != 2 {
			t.Errorf("%s: UserTaskCompletion %v, want a bare Workflows$SingleInputCompletion", vlabel(tt.v), cd)
		}
		if id, _ := get(cd, "$ID"); !nonZeroID(id) {
			t.Errorf("%s: UserTaskCompletion has no $ID: %v", vlabel(tt.v), id)
		}
	}
}

// The completion's $ID is derived from the task's, so writing the same task
// twice produces the same bytes.
func TestNormalize_UserTaskCompletionIDIsStable(t *testing.T) {
	doc := workflowWithUserTasks(userTaskDoc("Single", false, xpathUserTargeting("[Name = 'admin']")))
	a, err := normalizeShape(t, pv(9, 24, 30), doc)
	if err != nil {
		t.Fatal(err)
	}
	b, err := normalizeShape(t, pv(9, 24, 30), doc)
	if err != nil {
		t.Fatal(err)
	}
	ida, _ := get(find(a, "Workflows$SingleInputCompletion"), "$ID")
	idb, _ := get(find(b, "Workflows$SingleInputCompletion"), "$ID")
	if !sameID(ida, idb) {
		t.Errorf("the completion's $ID differs between two writes of the same task: %v, %v", ida, idb)
	}
}

func nonZeroID(v any) bool {
	b, ok := v.(bson.Binary)
	return ok && len(b.Data) == 16 && !bytes.Equal(b.Data, make([]byte, 16))
}

// A multi-user task below 10.12 is a Workflows$UserTask whose
// UserTaskCompletion is a Workflows$MultiInputCompletion. From 10.12 the
// MultiUserTaskActivity holds the same things itself: CompletionCriteria (its
// outcome pointers unchanged), TargetUserInput and, from 10.2, AwaitAllUsers.
func TestNormalize_MultiUserTaskBelowMendix10_12(t *testing.T) {
	for _, tt := range []struct {
		v          *version.ProjectVersion
		completion string // keys of the MultiInputCompletion; "" = a MultiUserTaskActivity
	}{
		{pv(9, 24, 30), "$ID,$Type,CompletionCriteria,TargetUserInput"},
		{pv(10, 1, 0), "$ID,$Type,CompletionCriteria,TargetUserInput"},
		{pv(10, 2, 0), "$ID,$Type,AwaitAllUsers,CompletionCriteria,TargetUserInput"},
		{pv(10, 11, 0), "$ID,$Type,AwaitAllUsers,CompletionCriteria,TargetUserInput"},
		{pv(10, 12, 0), ""},
		{pv(10, 24, 24), ""}, // control
	} {
		doc := userTaskDoc("Multi", true, xpathUserTargeting("[Name = 'admin']"))
		criteria, _ := get(doc, "CompletionCriteria")
		input, _ := get(doc, "TargetUserInput")
		d, err := normalizeShape(t, tt.v, workflowWithUserTasks(doc))
		if err != nil {
			t.Errorf("%s: refused: %v", vlabel(tt.v), err)
			continue
		}
		task := findNamed(d, "Multi")
		typ, _ := get(task, "$Type")
		if tt.completion == "" {
			if typ != "Workflows$MultiUserTaskActivity" {
				t.Errorf("%s: $Type %v, want Workflows$MultiUserTaskActivity", vlabel(tt.v), typ)
			}
			continue
		}
		if typ != "Workflows$UserTask" {
			t.Errorf("%s: $Type %v, want Workflows$UserTask", vlabel(tt.v), typ)
		}
		if got := sortedKeys(task); got != studioPro924UserTask {
			t.Errorf("%s: user task keys\n  %s\nwant\n  %s", vlabel(tt.v), got, studioPro924UserTask)
		}
		completion, _ := get(task, "UserTaskCompletion")
		cd, _ := completion.(bson.D)
		if ct, _ := get(cd, "$Type"); ct != "Workflows$MultiInputCompletion" {
			t.Errorf("%s: UserTaskCompletion is %v, want Workflows$MultiInputCompletion", vlabel(tt.v), ct)
			continue
		}
		if got := strings.Join(keysOf(cd), ","); got != tt.completion {
			t.Errorf("%s: MultiInputCompletion keys %s, want %s", vlabel(tt.v), got, tt.completion)
		}
		gotCriteria, _ := get(cd, "CompletionCriteria")
		gotInput, _ := get(cd, "TargetUserInput")
		if !sameValue(gotCriteria, criteria) || !sameValue(gotInput, input) {
			t.Errorf("%s: criteria %v and participants %v, want the task's own %v and %v", vlabel(tt.v), gotCriteria, gotInput, criteria, input)
		}
	}
}

// What a Workflows$UserTask cannot hold is refused, naming the task:
//   - a multi-user task below 9.22;
//   - a rule Mendix had not introduced yet: majority and threshold are 10.0,
//     a microflow rule is 10.3;
//   - `await all users` below 10.2;
//   - a boundary event on a user task below 10.14.
//
// An empty BoundaryEvents list is the codec's default, and it is dropped.
func TestNormalize_RefusesWhatAnOlderUserTaskCannotHold(t *testing.T) {
	withCriteria := func(typ string, extra ...bson.E) bson.D {
		doc := userTaskDoc("Multi", true, xpathUserTargeting("[Name = 'admin']"))
		cc := bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: typ}}
		cc = append(cc, extra...)
		doc[indexOf(doc, "CompletionCriteria")].Value = cc
		return doc
	}
	awaiting := userTaskDoc("Multi", true, xpathUserTargeting("[Name = 'admin']"))
	awaiting[indexOf(awaiting, "AwaitAllUsers")].Value = true
	withBoundary := userTaskDoc("Single", false, xpathUserTargeting("[Name = 'admin']"))
	withBoundary[indexOf(withBoundary, "BoundaryEvents")].Value = bson.A{int32(2),
		bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$InterruptingTimerBoundaryEvent"}, {Key: "Caption", Value: "Late"}}}
	for _, tt := range []struct {
		name string
		v    *version.ProjectVersion
		doc  bson.D
		want string
	}{
		{"multi-user task", pv(9, 21, 0), userTaskDoc("Multi", true, xpathUserTargeting("[Name = 'admin']")), "9.22"},
		{"await all users", pv(9, 24, 30), awaiting, "10.2"},
		{"majority", pv(9, 24, 30), withCriteria("Workflows$MajorityCompletionCriteria", bson.E{Key: "CompletionType", Value: "Absolute"}), "10.0"},
		{"threshold", pv(9, 24, 30), withCriteria("Workflows$ThresholdCompletionCriteria", bson.E{Key: "Threshold", Value: int32(2)}), "10.0"},
		{"microflow rule", pv(10, 2, 0), withCriteria("Workflows$MicroflowCompletionCriteria", bson.E{Key: "Microflow", Value: "M.Decide"}), "10.3"},
		{"boundary event 9.24", pv(9, 24, 30), withBoundary, "10.14"},
		{"boundary event 10.13", pv(10, 13, 0), withBoundary, "10.14"},
	} {
		name, _ := get(tt.doc, "Name")
		_, err := normalizeShape(t, tt.v, workflowWithUserTasks(tt.doc))
		if err == nil || !strings.Contains(err.Error(), name.(string)) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s on %s was not refused naming the task and %s: %v", tt.name, vlabel(tt.v), tt.want, err)
		}
	}
	// Control: each is written where Mendix has it.
	for _, tt := range []struct {
		v   *version.ProjectVersion
		doc bson.D
	}{
		{pv(9, 22, 0), userTaskDoc("Multi", true, xpathUserTargeting("[Name = 'admin']"))},
		{pv(10, 2, 0), awaiting},
		{pv(10, 0, 0), withCriteria("Workflows$MajorityCompletionCriteria", bson.E{Key: "CompletionType", Value: "Absolute"})},
		{pv(10, 3, 0), withCriteria("Workflows$MicroflowCompletionCriteria", bson.E{Key: "Microflow", Value: "M.Decide"})},
		{pv(10, 14, 0), withBoundary},
	} {
		if _, err := normalizeShape(t, tt.v, workflowWithUserTasks(tt.doc)); err != nil {
			t.Errorf("control %s: refused: %v", vlabel(tt.v), err)
		}
	}
	// The empty list the codec writes on every task is dropped below 10.14.
	d, err := normalizeShape(t, pv(10, 13, 0), workflowWithUserTasks(userTaskDoc("Single", false, xpathUserTargeting("[Name = 'admin']"))))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := get(findNamed(d, "Single"), "BoundaryEvents"); ok {
		t.Errorf("10.13: an empty BoundaryEvents is kept on a user task, which declares none before 10.14")
	}
}

func indexOf(d bson.D, key string) int {
	for i, e := range d {
		if e.Key == key {
			return i
		}
	}
	panic("no " + key)
}

// sameValue compares two stored values by their BSON bytes.
func sameValue(a, b any) bool {
	x, err1 := bson.Marshal(bson.D{{Key: "v", Value: a}})
	y, err2 := bson.Marshal(bson.D{{Key: "v", Value: b}})
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}
