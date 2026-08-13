// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// upstream #881: the import activity's Range round trip.
//
// Mendix stores two variants and mxcli only ever wrote the first, so the
// "Custom" setting was not merely undescribed but UNREPRESENTABLE:
//
//	Microflows$ConstantRange{SingleObject}                     All / First
//	Microflows$CustomRange{LimitExpression, OffsetExpression}  Custom
//
// The reader has to dispatch on $Type for the same reason: without it a Custom
// range read back as All and the limit was lost on the next write.

func importRangeBSON(rng bson.D, variableType string) bson.Raw {
	return mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "a-1"},
		{Key: "$Type", Value: "Microflows$ImportXmlAction"},
		{Key: "ErrorHandlingType", Value: "Rollback"},
		{Key: "XmlDocumentVariableName", Value: "resp"},
		{Key: "ResultHandling", Value: bson.D{
			{Key: "$ID", Value: "rh-1"},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "ResultVariableName", Value: "out"},
			{Key: "ImportMappingCall", Value: bson.D{
				{Key: "$ID", Value: "imc-1"},
				{Key: "$Type", Value: "Microflows$ImportMappingCall"},
				{Key: "ReturnValueMapping", Value: "M.IMM"},
				{Key: "ForceSingleOccurrence", Value: false},
				{Key: "Range", Value: rng},
			}},
			{Key: "VariableType", Value: bson.D{
				{Key: "$Type", Value: variableType},
				{Key: "Entity", Value: "M.Root"},
			}},
		}},
	})
}

func readImportRange(t *testing.T, raw bson.Raw) *microflows.ResultHandlingMapping {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	act := decodeAction(t, d)
	im, ok := act.(*microflows.ImportXmlAction)
	if !ok {
		t.Fatalf("actionFromGen → %T, want *microflows.ImportXmlAction", act)
	}
	if im.ResultHandling == nil {
		t.Fatal("ResultHandling nil")
	}
	return im.ResultHandling
}

// A CustomRange must survive the read. Before #881 the reader looked only at
// SingleObject, so a bounded import described as `all` and the next exec wrote
// it back unbounded.
func TestReadImportRange_Custom(t *testing.T) {
	h := readImportRange(t, importRangeBSON(bson.D{
		{Key: "$ID", Value: "r-1"},
		{Key: "$Type", Value: "Microflows$CustomRange"},
		{Key: "LimitExpression", Value: "10"},
		{Key: "OffsetExpression", Value: "5"},
	}, "DataTypes$ListType"))

	if h.LimitExpression != "10" || h.OffsetExpression != "5" {
		t.Errorf("limit/offset = %q/%q, want 10/5", h.LimitExpression, h.OffsetExpression)
	}
	if h.SingleObject {
		t.Error("SingleObject = true, want false (ListType variable)")
	}
}

// The shape Mendix itself ships in the blank app
// (FeedbackModule.SUB_Feedback_PostToAppInsights): range All against an OBJECT
// variable. The two axes disagree here, so a reader that folds one into the
// other necessarily loses a setting — this one described as `first` and rewrote
// the activity on the next exec.
func TestReadImportRange_AllAgainstAnObjectVariable(t *testing.T) {
	h := readImportRange(t, importRangeBSON(bson.D{
		{Key: "$ID", Value: "r-1"},
		{Key: "$Type", Value: "Microflows$ConstantRange"},
		{Key: "SingleObject", Value: false},
	}, "DataTypes$ObjectType"))

	if h.RangeSingleObject == nil || *h.RangeSingleObject {
		t.Errorf("RangeSingleObject = %v, want explicit false — the range is All", h.RangeSingleObject)
	}
	if !h.SingleObject {
		t.Error("SingleObject = false, want true — the stored ObjectType is the authority " +
			"on the variable's cardinality, not the range")
	}
}

func TestReadImportRange_First(t *testing.T) {
	h := readImportRange(t, importRangeBSON(bson.D{
		{Key: "$ID", Value: "r-1"},
		{Key: "$Type", Value: "Microflows$ConstantRange"},
		{Key: "SingleObject", Value: true},
	}, "DataTypes$ObjectType"))

	if h.RangeSingleObject == nil || !*h.RangeSingleObject {
		t.Errorf("RangeSingleObject = %v, want explicit true", h.RangeSingleObject)
	}
	if !h.SingleObject {
		t.Error("SingleObject = false, want true")
	}
}

// The write side of the same separation: a limit selects CustomRange, and the
// VariableType follows SingleObject rather than the range. Emitting a ListType
// for an object-rooted mapping is mxbuild's CE0243.
func TestWriteImportRange(t *testing.T) {
	single := true
	for _, tc := range []struct {
		name       string
		h          *microflows.ResultHandlingMapping
		wantRange  string
		wantVarTyp string
	}{
		{
			"custom range against an object variable",
			&microflows.ResultHandlingMapping{SingleObject: true, LimitExpression: "10", OffsetExpression: "5"},
			"Microflows$CustomRange", "DataTypes$ObjectType",
		},
		{
			"all against an object variable",
			&microflows.ResultHandlingMapping{SingleObject: true, RangeSingleObject: new(bool)},
			"Microflows$ConstantRange", "DataTypes$ObjectType",
		},
		{
			"first against a list mapping",
			&microflows.ResultHandlingMapping{SingleObject: true, RangeSingleObject: &single},
			"Microflows$ConstantRange", "DataTypes$ObjectType",
		},
	} {
		g := importXmlActionToGen(&microflows.ImportXmlAction{ResultHandling: tc.h})
		raw, err := (&codec.Encoder{}).Encode(g)
		if err != nil {
			t.Fatalf("%s: encode: %v", tc.name, err)
		}
		var doc bson.Raw = raw
		rh, ok := doc.Lookup("ResultHandling").DocumentOK()
		if !ok {
			t.Fatalf("%s: no ResultHandling", tc.name)
		}
		imc, _ := rh.Lookup("ImportMappingCall").DocumentOK()
		rng, _ := imc.Lookup("Range").DocumentOK()
		if got := rawStr(rng, "$Type"); got != tc.wantRange {
			t.Errorf("%s: Range $Type = %q, want %q", tc.name, got, tc.wantRange)
		}
		vt, _ := rh.Lookup("VariableType").DocumentOK()
		if got := rawStr(vt, "$Type"); got != tc.wantVarTyp {
			t.Errorf("%s: VariableType = %q, want %q", tc.name, got, tc.wantVarTyp)
		}
	}

	// A CustomRange carries the expressions, not SingleObject: a bounded range is
	// always bounded, and SingleObject has no meaning there.
	g := importXmlActionToGen(&microflows.ImportXmlAction{
		ResultHandling: &microflows.ResultHandlingMapping{LimitExpression: "$Size", OffsetExpression: "$Skip"},
	})
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.Raw = raw
	rh, _ := doc.Lookup("ResultHandling").DocumentOK()
	imc, _ := rh.Lookup("ImportMappingCall").DocumentOK()
	rng, _ := imc.Lookup("Range").DocumentOK()
	if got := rawStr(rng, "LimitExpression"); got != "$Size" {
		t.Errorf("LimitExpression = %q, want $Size", got)
	}
	if got := rawStr(rng, "OffsetExpression"); got != "$Skip" {
		t.Errorf("OffsetExpression = %q, want $Skip", got)
	}
	if _, ok := rng.Lookup("SingleObject").BooleanOK(); ok {
		t.Error("a CustomRange must not carry SingleObject")
	}
}
