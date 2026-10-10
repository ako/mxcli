// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// GetRawUnitByName reads only a unit's Name; it must agree with the map
// decode it replaced on every shape of document.
func TestRawUnitName_AgreesWithMapDecode(t *testing.T) {
	docs := map[string][]byte{
		"named":       unitDoc(t, "MF_Chain001"),
		"nameless":    mustMarshal(t, bson.D{{Key: "$Type", Value: "Microflows$Microflow"}}),
		"number name": mustMarshal(t, bson.D{{Key: "Name", Value: int32(7)}}),
		"truncated":   unitDoc(t, "Cut")[:12],
	}
	for label, doc := range docs {
		var raw map[string]any
		wantOK := bson.Unmarshal(doc, &raw) == nil
		want, _ := raw["Name"].(string)
		got, ok := rawUnitName(doc)
		if ok != wantOK || (ok && got != want) {
			t.Errorf("%s: rawUnitName = %q, %v; map decode = %q, %v", label, got, ok, want, wantOK)
		}
	}
}

func mustMarshal(t *testing.T, d bson.D) []byte {
	t.Helper()
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
