// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// The workflow user task's older shapes (ako/mxcli#1771), lowered here rather
// than in the workflow encoder because `alter workflow` inserts and edits user
// tasks through the raw mutator, which never passes the encoder: every unit
// write passes LowerToProjectShape.

// userTaskTypes are the $Types a user task is stored as, in any version.
var userTaskTypes = map[string]bool{
	"Workflows$SingleUserTaskActivity": true,
	"Workflows$MultiUserTaskActivity":  true,
	"Workflows$UserTask":               true,
}

// userSourceOf maps a user targeting (11.2+) to the user source an older
// version stores the same fact in. Measured with `mx convert -p` on
// Studio Pro-authored workflows (Employee Onboarding 9.1.1, Multilevel Dynamic
// Workflow 9.24.3) taken 9.24.30 → 10.24.24 → 11.13.0: the targeting keeps its
// $ID and its one value byte for byte, only its $Type and the key holding it
// change. Group targeting has no user source: it is 11.2, with the groups it
// targets.
var userSourceOf = map[string]string{
	"Workflows$XPathUserTargeting":     "Workflows$XPathBasedUserSource",
	"Workflows$MicroflowUserTargeting": "Workflows$MicroflowBasedUserSource",
	"Workflows$NoUserTargeting":        "Workflows$EmptyUserSource",
}

// emptyUserSourceSince is the version that introduced Workflows$EmptyUserSource,
// a user task with no targeting (mendixmodelsdk 4.117.0: introduced "9.10.0").
var emptyUserSourceSince = version.Version{Major: 9, Minor: 10}

// userTaskActivitySince is the version that introduced the types
// Workflows$SingleUserTaskActivity and MultiUserTaskActivity, and deleted
// Workflows$UserTask (mendixmodelsdk 4.117.0). Written down here because the
// generated version data holds property versions only, never a type's own.
var userTaskActivitySince = version.Version{Major: 10, Minor: 12}

// completionCriteriaSince holds the multi-user task rules introduced after the
// multi-user task itself (9.22.0, with consensus and veto): mendixmodelsdk
// 4.117.0 StructureVersionInfo.
var completionCriteriaSince = map[string]version.Version{
	"Workflows$MajorityCompletionCriteria":  {Major: 10},
	"Workflows$ThresholdCompletionCriteria": {Major: 10},
	"Workflows$MicroflowCompletionCriteria": {Major: 10, Minor: 3},
}

// completionRule names a completion criteria $Type as the statement that
// writes it does.
var completionRule = map[string]string{
	"Workflows$MajorityCompletionCriteria":  "decide by majority",
	"Workflows$ThresholdCompletionCriteria": "decide by threshold",
	"Workflows$MicroflowCompletionCriteria": "decide by microflow",
}

// lowerUserTask writes a user task in the shape the project's version stores:
// its targeting, its boundary events and, below 10.12, its type.
func (l *shapeLowering) lowerUserTask(task bson.D, typ string) (bson.D, error) {
	var err error
	if l.userSource {
		if task, err = l.lowerUserTargeting(task); err != nil {
			return nil, err
		}
	}
	if l.taskBoundaries {
		if task, err = l.dropTaskBoundaryEvents(task); err != nil {
			return nil, err
		}
	}
	if l.userTaskType && typ != "Workflows$UserTask" {
		return l.lowerUserTaskType(task, typ)
	}
	return task, nil
}

// dropTaskBoundaryEvents removes a user task's BoundaryEvents below 10.14,
// where a user task declares none (mendixmodelsdk: UserTaskActivity
// .boundaryEvents introduced 10.14.0; a Workflows$UserTask has none at all).
// The codec writes an empty list on every task, which is dropped; a boundary
// event is refused, naming the task. The floor never saw the key: it is
// declared on the abstract UserTaskActivity.
func (l *shapeLowering) dropTaskBoundaryEvents(task bson.D) (bson.D, error) {
	i := shapeIndex(task, "BoundaryEvents")
	if i < 0 {
		return task, nil
	}
	if !isEmptyValue(task[i].Value) {
		name, _ := shapeString(task, "Name")
		return nil, fmt.Errorf("user task %q has a boundary event, which Mendix introduced on user tasks in 10.14 "+
			"(this project is %s)", name, l.pv)
	}
	l.changed = true
	return append(append(bson.D{}, task[:i]...), task[i+1:]...), nil
}

// lowerUserTaskType rewrites a Workflows$SingleUserTaskActivity or
// MultiUserTaskActivity as the Workflows$UserTask a version before 10.12
// stores. Studio Pro 9.24.3 stores `{…, UserSource, UserTaskCompletion}`
// (Multilevel Dynamic Workflow, Marketplace 221741), and the completion says
// which kind of task it is (9.22.0):
//
//   - a single-user task: a bare Workflows$SingleInputCompletion;
//   - a multi-user task: a Workflows$MultiInputCompletion holding what the
//     MultiUserTaskActivity holds itself from 10.12: CompletionCriteria (its
//     outcome pointers unchanged, the outcomes stay on the task),
//     TargetUserInput and, from 10.2.0, AwaitAllUsers.
//
// A 9.24.30 project holding the 10.12 types does not load
// (TypeCacheUnknownTypeException). What the older type cannot hold is refused,
// naming the task: a multi-user task below 9.22, a rule introduced later
// (majority and threshold 10.0, microflow 10.3), and `await all users` below
// 10.2. The completion's $ID is derived from the task's, so the same task
// lowers to the same bytes.
func (l *shapeLowering) lowerUserTaskType(task bson.D, typ string) (bson.D, error) {
	name, _ := shapeString(task, "Name")
	hasCompletion := PropertyDeclared("Workflows$UserTask", "UserTaskCompletion", l.pv)
	completion := bson.D{{Key: "$ID", Value: l.derivedID(task, "UserTaskCompletion")}}
	if typ == "Workflows$MultiUserTaskActivity" {
		if !hasCompletion {
			return nil, fmt.Errorf("multi user task %q: Mendix introduced multi-user tasks in 9.22 (this project is %s)",
				name, l.pv)
		}
		completion = append(completion, bson.E{Key: "$Type", Value: "Workflows$MultiInputCompletion"})
		var await, criteria, input any
		task, await = takeKey(task, "AwaitAllUsers")
		task, criteria = takeKey(task, "CompletionCriteria")
		task, input = takeKey(task, "TargetUserInput")
		if PropertyDeclared("Workflows$MultiInputCompletion", "AwaitAllUsers", l.pv) {
			completion = append(completion, bson.E{Key: "AwaitAllUsers", Value: await == true})
		} else if await == true {
			return nil, fmt.Errorf("multi user task %q waits for all users, which Mendix introduced in 10.2 (this project is %s)",
				name, l.pv)
		}
		criteriaDoc, _ := criteria.(bson.D)
		ct, _ := shapeString(criteriaDoc, "$Type")
		if since, ok := completionCriteriaSince[ct]; ok && l.pv.Compare(since) < 0 {
			return nil, fmt.Errorf("multi user task %q is decided by %s, which Mendix introduced in %d.%d (this project is %s)",
				name, completionRule[ct], since.Major, since.Minor, l.pv)
		}
		completion = append(completion,
			bson.E{Key: "CompletionCriteria", Value: criteria},
			bson.E{Key: "TargetUserInput", Value: input})
	} else {
		completion = append(completion, bson.E{Key: "$Type", Value: "Workflows$SingleInputCompletion"})
	}
	task[shapeIndex(task, "$Type")].Value = "Workflows$UserTask"
	l.changed = true
	if !hasCompletion {
		return task, nil
	}
	// In Studio Pro's place: after the user source, its neighbour in the
	// alphabetical order Studio Pro writes.
	at := len(task)
	if i := shapeIndex(task, "UserSource"); i >= 0 {
		at = i + 1
	}
	out := append(append(bson.D{}, task[:at]...), bson.E{Key: "UserTaskCompletion", Value: completion})
	return append(out, task[at:]...), nil
}

// takeKey removes key from d, returning d without it and the value it held.
func takeKey(d bson.D, key string) (bson.D, any) {
	i := shapeIndex(d, key)
	if i < 0 {
		return d, nil
	}
	v := d[i].Value
	return append(append(bson.D{}, d[:i]...), d[i+1:]...), v
}

// derivedID is an $ID for an element the lowering adds to owner, derived from
// the owner's $ID and the key it is added under, as CompletePropertySets
// derives the ones it adds: the same owner always gets the same id, and one
// the unit already holds is stepped past.
func (l *shapeLowering) derivedID(owner bson.D, key string) bson.Binary {
	if l.used == nil {
		l.used = elementIDs(l.root)
	}
	return completionID(elementIDBytes(owner), "LowerToProjectShape/"+key, l.used)
}

// lowerUserTargeting replaces a user task's UserTargeting with the UserSource a
// version before 11.2 stores, in its place. mxcli writes UserTargeting on every
// version, and the floor never saw it: it is declared on the abstract
// UserTaskActivity. 10.12.19 and 10.24.24 then read the task as having no user
// source and report CE1859 "XPath constraint is required" on it, whatever its
// targeting was, and `mx convert` stores an empty XPathBasedUserSource in its
// place — the targeting silently gone. Group targeting is refused, naming the
// task.
func (l *shapeLowering) lowerUserTargeting(task bson.D) (bson.D, error) {
	i := shapeIndex(task, "UserTargeting")
	if i < 0 {
		return task, nil
	}
	targeting, _ := task[i].Value.(bson.D)
	var source bson.D
	if targeting != nil {
		typ, _ := shapeString(targeting, "$Type")
		name, _ := shapeString(task, "Name")
		sourceType, ok := userSourceOf[typ]
		switch {
		case !ok:
			return nil, fmt.Errorf("user task %q targets %s, which Mendix introduced in 11.2 (this project is %s): "+
				"a user task there targets users, by XPath or by a microflow", name, targetingKind(typ), l.pv)
		case sourceType == "Workflows$EmptyUserSource" && l.pv.Compare(emptyUserSourceSince) < 0:
			return nil, fmt.Errorf("user task %q has no targeting, which Mendix introduced in 9.10 (this project is %s): "+
				"give it `targeting users xpath […]` or `targeting users microflow …`", name, l.pv)
		}
		source = append(bson.D{}, targeting...)
		source[shapeIndex(source, "$Type")].Value = sourceType
	}
	task[i] = bson.E{Key: "UserSource", Value: source}
	l.changed = true
	// A stored UserSource beside it (an older document an edit gave a
	// UserTargeting) is the fact the edit replaced.
	for j := range task {
		if j != i && task[j].Key == "UserSource" {
			return append(append(bson.D{}, task[:j]...), task[j+1:]...), nil
		}
	}
	return task, nil
}

// targetingKind names a targeting $Type as the statement that writes it does.
func targetingKind(typ string) string {
	switch typ {
	case "Workflows$XPathGroupTargeting":
		return "groups by XPath (`targeting groups xpath`)"
	case "Workflows$MicroflowGroupTargeting":
		return "groups by a microflow (`targeting groups microflow`)"
	}
	return "with a " + typ
}
