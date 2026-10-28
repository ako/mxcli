// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
)

// ako/mxcli#1163: what the 9.24 doctype run found mxcli writing that Mendix 9
// stores differently, or not at all. Each case is the document as mxcli builds
// it, normalized for the project's version the way every unit write is.

// aggregateMicroflow is a microflow holding one Aggregate list activity as the
// writer builds it: Count/Sum/… carry no fold; Reduce, All and Any carry
// ReduceInitialValueExpression and ReduceReturnDataType.
func aggregateMicroflow(function string) bson.D {
	action := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Microflows$AggregateAction"},
		{Key: "AggregateFunction", Value: function},
		{Key: "AggregateVariableName", Value: "Items"},
		{Key: "Attribute", Value: "M.Item.Price"},
		{Key: "ErrorHandlingType", Value: "Rollback"}}
	switch function {
	case "Reduce", "All", "Any":
		initial := ""
		if function == "Reduce" {
			initial = "0"
		}
		action = append(action,
			bson.E{Key: "ReduceInitialValueExpression", Value: initial},
			bson.E{Key: "ReduceReturnDataType", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "DataTypes$BooleanType"}}})
	}
	action = append(action, bson.E{Key: "VariableName", Value: "Total"})
	activity := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Microflows$ActionActivity"}, {Key: "Action", Value: action}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Microflows$Microflow"}, {Key: "Name", Value: "Agg"},
		{Key: "ObjectCollection", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3), activity}}}}}
}

// Microflows$AggregateAction.AggregateFunction is declared on every version
// mxcli writes. The generated version data floored it at 10.0.0, so every
// count/sum/average/minimum/maximum on 9.x was refused ("cannot store
// Microflows$AggregateAction.AggregateFunction (Mendix 10.0.0+)"). Studio Pro
// 9.24.0 stores it (Workflow Commons 2.6.0: 10 of 10 aggregates, `Count`), `mx
// convert` 9.24.30 keeps it, and mendixmodelsdk gives it no version at all; the
// 10.0.0 floor belongs to ReduceReturnDataType, the fold Reduce added.
func TestNormalize_AggregateFunctionIsDeclaredOnMendix9(t *testing.T) {
	for _, v := range []*version.ProjectVersion{pv(9, 0, 5), pv(9, 24, 30), pv(10, 0, 0), pv(11, 13, 0)} {
		d, err := normalizeShape(t, v, aggregateMicroflow("Sum"))
		if err != nil {
			t.Errorf("%d.%d: a sum aggregate is refused: %v", v.MajorVersion, v.MinorVersion, err)
			continue
		}
		if f, _ := get(find(d, "Microflows$AggregateAction"), "AggregateFunction"); f != "Sum" {
			t.Errorf("%d.%d: AggregateFunction = %v, want Sum", v.MajorVersion, v.MinorVersion, f)
		}
	}
}

// The fold is what Mendix 10.0 added (mendixmodelsdk: ReduceReturnDataType
// introduced 10.0.0). `all` carries an empty initial value, so the fold's
// return type is the one key that says it: on 9.24 it is refused, naming it,
// rather than written into a document Studio Pro 9 cannot resolve. Before, the
// generated data had no row for it and it passed through.
func TestNormalize_ReduceFoldIsRefusedBelowMendix10(t *testing.T) {
	for _, fn := range []string{"Reduce", "All"} {
		if _, err := normalizeShape(t, pv(9, 24, 30), aggregateMicroflow(fn)); err == nil ||
			!strings.Contains(err.Error(), "Microflows$AggregateAction.ReduceReturnDataType (Mendix 10.0.0+)") {
			t.Errorf("%s on 9.24 was not refused naming its fold's return type: %v", fn, err)
		}
		d, err := normalizeShape(t, pv(10, 0, 0), aggregateMicroflow(fn))
		if err != nil {
			t.Fatalf("%s on 10.0 is refused: %v", fn, err)
		}
		if _, ok := get(find(d, "Microflows$AggregateAction"), "ReduceReturnDataType"); !ok {
			t.Errorf("%s on 10.0 lost ReduceReturnDataType", fn)
		}
	}
}

// pid is a fresh PersistentId: a non-zero GUID, as the writer mints one (a zero
// GUID is an empty value, which the floor would drop rather than refuse).
func pid() bson.Binary {
	pidSeq++
	return bson.Binary{Subtype: 0, Data: []byte{pidSeq, 0x11, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}}
}

var pidSeq byte

// workflowWithPersistentIDs is a workflow as mxcli builds it: a fresh
// PersistentId on the workflow, on every activity and on every outcome.
func workflowWithPersistentIDs() bson.D {
	outcome := func(value bool) bson.D {
		return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$BooleanConditionOutcome"},
			{Key: "Flow", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Flow"}, {Key: "Activities", Value: bson.A{int32(3)}}}},
			{Key: "PersistentId", Value: pid()}, {Key: "Value", Value: value}}
	}
	split := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$ExclusiveSplitActivity"},
		{Key: "Caption", Value: "Active?"}, {Key: "Expression", Value: "$WorkflowContext/Active"}, {Key: "Name", Value: "split"},
		{Key: "Outcomes", Value: bson.A{int32(3), outcome(true), outcome(false)}},
		{Key: "PersistentId", Value: pid()}}
	call := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$CallMicroflowTask"},
		{Key: "BoundaryEvents", Value: bson.A{int32(2)}}, {Key: "Caption", Value: "Validate"},
		{Key: "Microflow", Value: "M.Validate"}, {Key: "Name", Value: "call"}, {Key: "PersistentId", Value: pid()}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: bson.A{int32(3), call, split}}}},
		{Key: "Name", Value: "WF"},
		{Key: "PersistentId", Value: pid()},
		{Key: "Title", Value: "WF"}}
}

// persistentIDHolders returns the $Type of every element in v carrying a
// PersistentId.
func persistentIDHolders(v any) []string {
	var out []string
	switch x := v.(type) {
	case bson.D:
		typ, _ := get(x, "$Type")
		for _, e := range x {
			if e.Key == "PersistentId" {
				out = append(out, typ.(string))
			}
			out = append(out, persistentIDHolders(e.Value)...)
		}
	case bson.A:
		for _, el := range x {
			out = append(out, persistentIDHolders(el)...)
		}
	}
	return out
}

// Workflows exist from 9.0, PersistentId from 10.21.0 — on the workflow and on
// WorkflowActivity, Outcome and BoundaryEvent (mendixmodelsdk). Below that a
// workflow was refused ("cannot store Workflows$Workflow.PersistentId (Mendix
// 10.21.0+)"), and the activities' and outcomes' were never seen by the floor,
// which does not look up the abstract supertypes that declare them: `mx
// convert` 9.24.30 strips all of them. A PersistentId is minted by the writer,
// never stated by a script, so below 10.21 it is not written.
func TestNormalize_WorkflowPersistentIDsBelowMendix10_21(t *testing.T) {
	for _, tt := range []struct {
		v    *version.ProjectVersion
		want int
	}{
		{pv(9, 24, 30), 0},
		{pv(10, 20, 0), 0},
		{pv(10, 21, 0), 5},
		{pv(10, 24, 24), 5},
		{pv(11, 13, 0), 5}, // control
	} {
		d, err := normalizeShape(t, tt.v, workflowWithPersistentIDs())
		if err != nil {
			t.Errorf("%d.%d: a workflow is refused: %v", tt.v.MajorVersion, tt.v.MinorVersion, err)
			continue
		}
		if got := persistentIDHolders(d); len(got) != tt.want {
			t.Errorf("%d.%d: PersistentId on %d elements %v, want %d", tt.v.MajorVersion, tt.v.MinorVersion, len(got), got, tt.want)
		}
	}
}
