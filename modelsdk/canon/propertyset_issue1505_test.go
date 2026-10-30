// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1505: on Mendix 10.24.18 a nanoflow mxcli re-created, and a fresh
// microflow and entity, lacked keys Studio Pro writes, so `mx diff` threw
//
//	Objects with ID … of type Microflows$Nanoflow do not have the same
//	properties. baseNames = …, ExportLevel, …, ReturnVariableName; newNames = …
//
// The table held every one of them, with "since": "10.24.24" — the lowest
// version anyone had MEASURED, read by appliesTo as the version that introduced
// the key. Each value below is what `mx convert --in-place` wrote on 10.24.18
// and on 10.24.0, the lowest 10.24 patch, for the element mxcli left without
// it (the key-set diff of every $ID before and after the conversion).
func TestCompletePropertySets_Issue1505RowsBelow10_24_24(t *testing.T) {
	cases := []struct {
		typ, key string
		check    func(any) bool
	}{
		{"Microflows$Nanoflow", "ExportLevel", eq("Hidden")},
		{"Microflows$Nanoflow", "ReturnVariableName", eq("")},
		{"Microflows$AggregateAction", "UseExpression", eq(false)},
		{"Microflows$AggregateAction", "Expression", eq("")},
		{"Microflows$AggregateAction", "ReduceInitialValueExpression", eq("")},
		{"Microflows$AggregateAction", "ReduceReturnDataType", isElement("DataTypes$BooleanType")},
		{"Microflows$DeleteAction", "ErrorHandlingType", eq("Rollback")},
		{"Microflows$WhileLoopCondition", "Caption", eq("")},
		{"Microflows$DatabaseRetrieveSource", "XpathConstraint", eq("")},
		{"DomainModels$EntityImpl", "Image", eq("")},
		{"DomainModels$EntityImpl", "Source", eq(nil)},
		{"DomainModels$EntityImpl", "ImageData", isEmptyBinary},
	}
	for _, ver := range []string{"10.24.18", "10.24.0"} {
		for _, c := range cases {
			t.Run(ver+"/"+c.typ+"."+c.key, func(t *testing.T) {
				in := mustMarshal(t, bson.D{
					{Key: "$ID", Value: freshID()},
					{Key: "$Type", Value: "Test$Holder"},
					{Key: "El", Value: bson.D{{Key: "$ID", Value: freshID()}, {Key: "$Type", Value: c.typ}}},
				})
				got, ok := keysOf(t, CompletePropertySets(in, v(ver)), "El")[c.key]
				if !ok {
					t.Fatalf("a Mendix %s project got %s without %s; Studio Pro %s writes it", ver, c.typ, c.key, ver)
				}
				if !c.check(got) {
					t.Fatalf("%s.%s = %#v on %s, not the value mx convert wrote", c.typ, c.key, got, ver)
				}
			})
		}
	}
}

func eq(want any) func(any) bool { return func(got any) bool { return got == want } }

func isElement(typ string) func(any) bool {
	return func(got any) bool {
		d, ok := got.(bson.D)
		if !ok {
			return false
		}
		s, _ := lookupString(d, "$Type")
		return s == typ && elementIDBytes(d) != nil
	}
}

func isEmptyBinary(got any) bool {
	b, ok := got.(bson.Binary)
	return ok && len(b.Data) == 0
}
