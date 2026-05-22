// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// Issue #585: parseScheduledEvent asserted `raw["Interval"].(int32)`. Studio
// Pro writes Interval as BSON int64, so the assertion failed silently and
// every scheduled event read from a Studio Pro-written MPR appeared with
// Interval=0 — the same misreport pattern fixed in #583 for
// StringAttributeType.Length.
func TestParseScheduledEvent_Interval_BsonNumericWidths(t *testing.T) {
	cases := []struct {
		name     string
		interval any
		want     int
	}{
		{"int32 (mxcli writer)", int32(15), 15},
		{"int64 (Studio Pro writer)", int64(15), 15},
		{"int", int(15), 15},
		{"float64 (extended JSON)", float64(15), 15},
		{"missing field", nil, 0},
	}

	r := &Reader{version: MPRVersionV1}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := bson.M{
				"$Type":        "ScheduledEvents$ScheduledEvent",
				"Name":         "MyEvent",
				"Enabled":      true,
				"IntervalType": "Hour",
			}
			if tc.interval != nil {
				doc["Interval"] = tc.interval
			}
			data, err := bson.Marshal(doc)
			if err != nil {
				t.Fatalf("bson.Marshal: %v", err)
			}
			event, err := r.parseScheduledEvent("unit-id", "container-id", data)
			if err != nil {
				t.Fatalf("parseScheduledEvent: %v", err)
			}
			if event.Interval != tc.want {
				t.Errorf("Interval = %d, want %d (input %T(%v))", event.Interval, tc.want, tc.interval, tc.interval)
			}
		})
	}
}
