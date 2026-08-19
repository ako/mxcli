// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ADR-0008 decision 1, end to end on a real MPR v2 project: writing the same
// microflow twice must leave the stored unit byte-identical the second time, and
// no write may re-mint the microflow's StableId.
//
// The scenario under test is *re-running a script*, so the source of both writes
// is the same in-memory microflow. Reading the stored unit and writing it back
// would test something else — the fidelity of the read/rebuild round-trip, which
// is independently lossy on this fixture today (a rebuild of one fixture
// microflow is ~116 bytes smaller than what is stored). That is a real gap, but
// it is not this one, and folding it in would make the test unable to fail for
// the reason it exists.
//
// A microflow is the case that matters: it dominates a script-authored app, its
// rebuild is a graph rather than a tree (so every sub-element gets a fresh random
// $ID), and it is the only document type carrying an identity field. The same
// test over a page would pass with neither mechanism present.

// storedUnit returns a unit's raw BSON as it currently sits in the project.
func storedUnit(t *testing.T, proj string, id model.ID) []byte {
	t.Helper()
	r, err := mmpr.OpenWithOptions(proj, mmpr.OpenOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	defer r.Close()
	raw, err := r.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes(%s): %v", id, err)
	}
	return append([]byte(nil), raw...)
}

// stableIDOf reads a microflow unit's StableId, or nil when absent.
func stableIDOf(t *testing.T, raw []byte) []byte {
	t.Helper()
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}
	v, ok := doc["StableId"]
	if !ok {
		return nil
	}
	b, ok := v.(primitive.Binary)
	if !ok {
		t.Fatalf("StableId is %T, expected binary", v)
	}
	return b.Data
}

// aMicroflow picks a deterministic microflow from the fixture — lowest ID, so
// the choice cannot drift with iteration order.
func aMicroflow(t *testing.T, proj string) *microflows.Microflow {
	t.Helper()
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer b.Disconnect()

	list, err := b.ListMicroflows()
	if err != nil {
		t.Fatalf("ListMicroflows: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("fixture has no microflows — this test would prove nothing")
	}
	best := list[0]
	for _, mf := range list {
		if mf.ID < best.ID {
			best = mf
		}
	}
	full, err := b.GetMicroflow(best.ID)
	if err != nil || full == nil {
		t.Fatalf("GetMicroflow(%s) = %v, %v", best.ID, full, err)
	}
	return full
}

// writeMicroflow runs one write of mf against the project, in its own session,
// the way two consecutive `mxcli exec` runs would.
func writeMicroflow(t *testing.T, proj string, mf *microflows.Microflow) {
	t.Helper()
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := b.UpdateMicroflow(mf); err != nil {
		t.Fatalf("UpdateMicroflow: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
}

// TestWriteMicroflowTwice_SecondWriteIsElided is decision 1: re-running a write
// whose content already matches what is stored does not touch the unit.
func TestWriteMicroflowTwice_SecondWriteIsElided(t *testing.T) {
	proj := copyFixture(t)
	mf := aMicroflow(t, proj)

	writeMicroflow(t, proj, mf)
	first := storedUnit(t, proj, mf.ID)

	writeMicroflow(t, proj, mf)
	second := storedUnit(t, proj, mf.ID)

	if !bytes.Equal(first, second) {
		t.Errorf("re-writing the same microflow (%q) changed its stored unit: %d bytes -> %d bytes",
			mf.Name, len(first), len(second))
	}
}

// rebuildMicroflow returns the bytes the codec produces for mf — the write path
// of UpdateMicroflow up to but not including storage, so the rebuild can be
// observed before Reconcile has had a chance to normalise it.
func rebuildMicroflow(t *testing.T, proj string, mf *microflows.Microflow) []byte {
	t.Helper()
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer b.Disconnect()

	gm := microflowToGen(mf, b.majorVersion())
	gm.SetID(element.ID(mf.ID))
	assignMicroflowIDs(gm)
	contents, err := (&codec.Encoder{}).Encode(gm)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return contents
}

// TestRebuildChurnsSubElementIDs is the control both storage tests rest on: the
// rebuild really is a function of the script *and a random source*, so a test
// showing stable stored bytes is showing the mechanism working rather than a
// rebuild that was byte-stable all along. Without this, the tests here would
// pass against a build that never had either fix — which is how PR #125 shipped
// green.
func TestRebuildChurnsSubElementIDs(t *testing.T) {
	proj := copyFixture(t)
	mf := aMicroflow(t, proj)

	first := rebuildMicroflow(t, proj, mf)
	second := rebuildMicroflow(t, proj, mf)

	if bytes.Equal(first, second) {
		t.Fatal("two rebuilds of the same microflow produced identical bytes — the fixture " +
			"cannot demonstrate either mechanism, so pick one whose rebuild mints sub-element IDs")
	}
	// Two things are re-minted per rebuild and they are handled by different
	// mechanisms: StableId, a top-level identity field CarryIdentity copies over,
	// and every sub-element $ID, which the transplant matches up. Carrying the
	// first leaves exactly the second, which is what the storage tests above
	// depend on being there.
	eq, err := canon.Equal(canon.CarryIdentity(second, first), first)
	if err != nil {
		t.Fatalf("canon.Equal: %v", err)
	}
	if !eq {
		t.Fatal("with StableId carried, the two rebuilds still differ by more than their " +
			"choice of element IDs — this control is measuring some other instability")
	}
}

// TestWriteMicroflowTwice_ForcedWritesStayByteStable is ADR-0008's other half —
// carrying the stored element $IDs onto a write that lands. Elision is turned
// off, so both writes really do reach storage; the bytes are identical anyway
// because the second write inherits the ids the first one left behind.
//
// The same mechanism is what keeps a *changed* document's diff down to what
// changed: without it, editing one argument of one activity re-minted 36 of a
// nanoflow's 37 element identities.
func TestWriteMicroflowTwice_ForcedWritesStayByteStable(t *testing.T) {
	t.Setenv("MXCLI_ALWAYS_WRITE", "1")
	proj := copyFixture(t)
	mf := aMicroflow(t, proj)

	writeMicroflow(t, proj, mf)
	first := storedUnit(t, proj, mf.ID)

	writeMicroflow(t, proj, mf)
	second := storedUnit(t, proj, mf.ID)

	if !bytes.Equal(first, second) {
		t.Errorf("a forced rewrite of an unchanged microflow (%q) changed its stored bytes: "+
			"%d -> %d; stored element identities were not carried onto the rebuild",
			mf.Name, len(first), len(second))
	}
}

// TestWriteStatsSeeTheElision wires the executor's "Modified …" vs "Unchanged …"
// reporting to something real. The counter has to move for the *offer* and stay
// put for the *write*, because that difference is the only evidence the executor
// has that a write was skipped — a counter that merely tracked calls would let
// the console keep claiming writes that never happened (#910).
func TestWriteStatsSeeTheElision(t *testing.T) {
	proj := copyFixture(t)
	mf := aMicroflow(t, proj)
	writeMicroflow(t, proj, mf) // first write lands; second is the one measured

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer b.Disconnect()

	before := b.WriteStats()
	if err := b.UpdateMicroflow(mf); err != nil {
		t.Fatalf("UpdateMicroflow: %v", err)
	}
	after := b.WriteStats()

	if after.Offered <= before.Offered {
		t.Errorf("Offered %d -> %d: the write never reached storage's elision check",
			before.Offered, after.Offered)
	}
	if after.Written != before.Written {
		t.Errorf("Written %d -> %d: an unchanged microflow was counted as written",
			before.Written, after.Written)
	}
}

// TestWriteMicroflow_PreservesStableId is the identity half, and it is asserted
// against a write that actually lands: elision would otherwise hide a churning
// StableId behind "nothing was written". Every client-callable microflow's
// operation id in the deployed model is derived from this value.
func TestWriteMicroflow_PreservesStableId(t *testing.T) {
	t.Setenv("MXCLI_ALWAYS_WRITE", "1") // force both writes to land
	proj := copyFixture(t)
	mf := aMicroflow(t, proj)

	original := stableIDOf(t, storedUnit(t, proj, mf.ID))
	if len(original) != 16 {
		t.Fatalf("fixture microflow %q has no 16-byte StableId; this test cannot prove preservation", mf.Name)
	}

	writeMicroflow(t, proj, mf)
	if got := stableIDOf(t, storedUnit(t, proj, mf.ID)); !bytes.Equal(got, original) {
		t.Fatalf("first write re-minted StableId: %x -> %x", original, got)
	}
	writeMicroflow(t, proj, mf)
	if got := stableIDOf(t, storedUnit(t, proj, mf.ID)); !bytes.Equal(got, original) {
		t.Errorf("second write re-minted StableId: %x -> %x", original, got)
	}
}

// TestWriteMicroflow_RealEditStillLands is the safety direction. A false "equal"
// silently discards the user's intent, which is strictly worse than a redundant
// write, so elision must never swallow an actual change.
func TestWriteMicroflow_RealEditStillLands(t *testing.T) {
	proj := copyFixture(t)
	mf := aMicroflow(t, proj)

	writeMicroflow(t, proj, mf) // settle, so the edit is the only difference
	mf.Documentation = "edited by TestWriteMicroflow_RealEditStillLands"
	writeMicroflow(t, proj, mf)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	defer b.Disconnect()
	got, err := b.GetMicroflow(mf.ID)
	if err != nil || got == nil {
		t.Fatalf("GetMicroflow after edit = %v, %v", got, err)
	}
	if got.Documentation != mf.Documentation {
		t.Errorf("edit was elided: Documentation = %q, want %q", got.Documentation, mf.Documentation)
	}
}
