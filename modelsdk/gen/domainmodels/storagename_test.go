// SPDX-License-Identifier: Apache-2.0

package domainmodels

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestRegExRuleInfoUsesStorageName pins the BSON key of the regex reference.
//
// The generator that produced this package bound properties by their SDK
// *name* where Mendix stores them under a *storage name*. The reflection data
// carries both, and the in-repo generator (cmd/codegen) keeps them apart:
//
//	generated/metamodel/types.go
//	    RegularExpression model.QualifiedName `json:"regExIdentifier,omitempty"`
//	                    ^ SDK name                    ^ storage name
//
// So "RegExIdentifier" is the key on disk, and a writer emitting
// "RegularExpression" produces a property Mendix does not have. Both the encode
// side (initRegExRuleInfo) and the decode side (InitFromRaw) must agree on it —
// they are two separate literals and have drifted before.
func TestRegExRuleInfoUsesStorageName(t *testing.T) {
	const storageKey = "RegExIdentifier"

	t.Run("encode", func(t *testing.T) {
		o := NewRegExRuleInfo()
		o.SetRegularExpressionQualifiedName("MyModule.EmailPattern")

		props := o.Properties()
		if len(props) != 1 {
			t.Fatalf("got %d properties, want 1", len(props))
		}
		if got := props[0].Name(); got != storageKey {
			t.Errorf("property key = %q, want %q — this key is written to the .mxunit", got, storageKey)
		}
	})

	t.Run("decode", func(t *testing.T) {
		raw, err := bson.Marshal(bson.D{
			{Key: "$Type", Value: "DomainModels$RegExRuleInfo"},
			{Key: storageKey, Value: "MyModule.EmailPattern"},
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		o := initRegExRuleInfo()
		o.InitFromRaw(bson.Raw(raw))

		if got := o.RegularExpressionQualifiedName(); got != "MyModule.EmailPattern" {
			t.Errorf("decoded %q, want MyModule.EmailPattern — InitFromRaw is reading the wrong key, "+
				"so every stored regex rule reads back empty", got)
		}
	})
}
