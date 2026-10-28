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
