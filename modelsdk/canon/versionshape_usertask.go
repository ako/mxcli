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
