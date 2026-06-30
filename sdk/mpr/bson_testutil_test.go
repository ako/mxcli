// SPDX-License-Identifier: Apache-2.0

package mpr

import "go.mongodb.org/mongo-driver/bson"

// dToMap recursively converts an ordered bson.D (and any nested bson.D/bson.A)
// into the unordered bson.M / map form that several serializer tests assert
// against by key. The writer now emits ordered bson.D values (so that "$ID" is
// the first property, required by Mendix 11.12+); these tests only care about
// field presence and values, not order, so converting to a map keeps them valid.
//
// Order-sensitivity itself ("$ID" first) is covered separately in
// writer_id_order_test.go.
func dToMap(v any) any {
	switch t := v.(type) {
	case bson.D:
		m := bson.M{}
		for _, e := range t {
			m[e.Key] = dToMap(e.Value)
		}
		return m
	case bson.A:
		out := make(bson.A, len(t))
		for i, e := range t {
			out[i] = dToMap(e)
		}
		return out
	default:
		return v
	}
}

// dToM is a convenience wrapper that converts a bson.D storage object to bson.M.
func dToM(d bson.D) bson.M {
	return dToMap(d).(bson.M)
}
