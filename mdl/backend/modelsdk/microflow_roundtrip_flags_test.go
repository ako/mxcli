// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// roundTripMicroflow encodes a semantic microflow to gen, through the codec, and
// back to the semantic model — the exact write→read path any UpdateMicroflow
// caller takes. It is the precise reproduction for issue #723 §A: fields set on
// the way out but never read back are silently reset on the return trip.
func roundTripMicroflow(t *testing.T, mf *microflows.Microflow) *microflows.Microflow {
	t.Helper()
	gm := microflowToGen(mf, 11)
	raw, err := (&codec.Encoder{}).Encode(gm)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	dec, ok := el.(*genMf.Microflow)
	if !ok {
		t.Fatalf("decoded %T, want *genMf.Microflow", el)
	}
	return microflowFromGen(dec, "mod-1")
}

// TestMicroflowRoundTrip_ConcurrentExecutionFlags guards issue #723 §A (CE4899).
// AllowConcurrentExecution / MarkAsUsed were written by microflowToGen but never
// read back by microflowFromGen, so an UpdateMicroflow round-trip reset them to
// the Go zero value (false). A microflow that allowed concurrent execution then
// came back as "disallow concurrent execution" with no error message/microflow
// configured, and mx check reported CE4899 "concurrent execution: error message
// or microflow required".
func TestMicroflowRoundTrip_ConcurrentExecutionFlags(t *testing.T) {
	mf := &microflows.Microflow{
		Name:                     "ACT_Test",
		AllowConcurrentExecution: true,
		MarkAsUsed:               true,
	}
	mf.ID = model.ID("mf-1")

	got := roundTripMicroflow(t, mf)
	if !got.AllowConcurrentExecution {
		t.Error("AllowConcurrentExecution lost on round-trip → CE4899 (want true)")
	}
	if !got.MarkAsUsed {
		t.Error("MarkAsUsed lost on round-trip (want true)")
	}
}

// TestMicroflowRoundTrip_ApplyEntityAccess is the third property in this struct
// to go the way #723 §A describes, and the only one with a security consequence.
//
// "Apply entity access" makes a microflow run under the current user's entity
// access rules rather than with full access, so it only ever NARROWS. Both
// writers hardcoded false and microflowFromGen did not read it back, so every
// rewrite turned it off — widening what the microflow may read and write, with
// nothing to report it: `mxcli check` is quiet, mxbuild is quiet, and the model
// is valid either way.
//
// Measured across 342 microflows in 4 projects (11.14.0): every microflow
// storing true came back false. The write half is the assertion below; the
// executor's preserve-on-rewrite rule is TestCarriedApplyEntityAccess.
func TestMicroflowRoundTrip_ApplyEntityAccess(t *testing.T) {
	mf := &microflows.Microflow{Name: "ACT_Secured", ApplyEntityAccess: true}
	mf.ID = model.ID("mf-2")

	if got := roundTripMicroflow(t, mf); !got.ApplyEntityAccess {
		t.Error("ApplyEntityAccess lost on round-trip — the microflow now runs with " +
			"FULL access instead of the user's (want true)")
	}

	// The other direction has to survive too: a stored false must not become
	// true, or the fix would be a different silent change in the same place.
	off := &microflows.Microflow{Name: "ACT_Plain"}
	off.ID = model.ID("mf-3")
	if got := roundTripMicroflow(t, off); got.ApplyEntityAccess {
		t.Error("ApplyEntityAccess invented on round-trip (want false)")
	}
}
