// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1097: three keys `mx convert` added to a project built through MDL
// (Mendix 11.13.0) that the table did not have. Each value is the one Studio Pro
// itself stores on an element that does not use the property, read off the
// Studio Pro-authored fixtures: testdata/pedapp (11.13.0) holds EntityRef null on
// every unpathed AttributeRef and MessageDefinition2 "" on both FeedbackModule
// mappings; testdata/expr-checker (11.6.6) holds the same EntityRef and no
// MessageDefinition2 at all, which gen's Mappings$MappingDocument records as
// Introduced 11.10.0.
//
// EntityRef's floor was the lowest fixture that held it, not the version that
// introduced it. ako/mxcli#1505 measured it lower: Studio Pro 10.24.0 stores
// EntityRef null on all 75 AttributeRefs of a blank app, and `mx convert`
// 10.24.0 and 10.24.18 write the same null back where it is stripped.
func TestCompletePropertySets_Issue1097Rows(t *testing.T) {
	cases := []struct {
		typ, key string
		want     any
		since    string
		below    string
	}{
		{"DomainModels$AttributeRef", "EntityRef", nil, "10.24.0", "10.23.0"},
		{"ImportMappings$ImportMapping", "MessageDefinition2", "", "11.10.0", "11.9.9"},
		{"ExportMappings$ExportMapping", "MessageDefinition2", "", "11.10.0", "11.9.9"},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			in := mustMarshal(t, bson.D{
				{Key: "$ID", Value: freshID()},
				{Key: "$Type", Value: "Test$Holder"},
				{Key: "El", Value: bson.D{{Key: "$ID", Value: freshID()}, {Key: "$Type", Value: c.typ}}},
			})
			got, ok := keysOf(t, CompletePropertySets(in, v(c.since)), "El")[c.key]
			if !ok || got != c.want {
				t.Fatalf("%s on %s: %#v (present %v), want %#v", c.key, c.since, got, ok, c.want)
			}
			if _, ok := keysOf(t, CompletePropertySets(in, v(c.below)), "El")[c.key]; ok {
				t.Fatalf("%s added to a %s project, older than its floor %s", c.key, c.below, c.since)
			}
		})
	}
}
