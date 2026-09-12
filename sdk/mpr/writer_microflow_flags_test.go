// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

// TestMicroflowApplyEntityAccessRoundTrip is the legacy half of the security
// fix, and exists to keep the two engines from drifting — the modelsdk twin is
// TestMicroflowRoundTrip_ApplyEntityAccess.
//
// This writer wrote `{Key: "ApplyEntityAccess", Value: false}` unconditionally,
// so a microflow that ran under the user's entity access rules came back running
// with full access. Nothing reported it: the model is valid either way.
func TestMicroflowApplyEntityAccessRoundTrip(t *testing.T) {
	for _, want := range []bool{true, false} {
		mf := &microflows.Microflow{Name: "ACT_Secured", ApplyEntityAccess: want}
		mf.ID = model.ID("mf-1")

		w := testWriter()
		raw, err := w.serializeMicroflow(mf)
		if err != nil {
			t.Fatalf("serialize: %v", err)
		}
		var doc map[string]any
		if err := bson.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got, ok := doc["ApplyEntityAccess"].(bool); !ok || got != want {
			t.Errorf("written ApplyEntityAccess = %#v, want %v", doc["ApplyEntityAccess"], want)
		}

		// And the parser has to read it back, or the value never reaches the
		// writer on a rewrite in the first place.
		back := ParseMicroflowFromRaw(doc, "mf-1", "mod-1")
		if back.ApplyEntityAccess != want {
			t.Errorf("parsed ApplyEntityAccess = %v, want %v", back.ApplyEntityAccess, want)
		}
	}
}
