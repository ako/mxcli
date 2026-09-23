// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
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

// TestMicroflowRoundTrip_DeepLinkURL is the fifth property in this struct to go
// the way #723 §A describes, and the first with no checker behind it at all.
//
// A microflow's Url is its deep link (Mendix 10.6+) — Studio Pro's "URL" field,
// e.g. `item/{Key}`. MDL has no syntax for one, so microflowToGen wrote `""`
// unconditionally and microflowFromGen never read the stored value back: a
// CREATE OR MODIFY MICROFLOW that changed only the body deleted the deep link.
//
// Nothing reports it. Unlike the concurrency flags above (CE4899), a microflow
// without a URL is entirely valid, so `mxcli check`, `mx check` and mxbuild all
// pass before and after — the loss is visible only in Studio Pro, which is how
// it reached a user as #1120. UrlSearchParameters is stored beside it and was
// lost with it.
func TestMicroflowRoundTrip_DeepLinkURL(t *testing.T) {
	// The two parameters are deliberately DIFFERENT. A parameter used in the
	// URL path may not also be a search parameter — mxbuild 11.6.6 rejects that
	// with CE5612 ("cannot be used as a URL parameter if it is already a URL
	// search parameter"). The first version of this fixture reused `Key` for
	// both and described a document Mendix refuses to build; the unit tests
	// could not tell, because nothing here validates the model. Measured by
	// seeding a real project and running mx check.
	mf := &microflows.Microflow{
		Name:                "ACT_Item",
		URL:                 "item/{Key}",
		URLSearchParameters: []string{"Mod.ACT_Item.Filter"},
	}
	mf.ID = model.ID("mf-4")

	got := roundTripMicroflow(t, mf)
	if got.URL != "item/{Key}" {
		t.Errorf("deep-link URL lost on round-trip: got %q, want %q", got.URL, "item/{Key}")
	}
	if len(got.URLSearchParameters) != 1 || got.URLSearchParameters[0] != "Mod.ACT_Item.Filter" {
		t.Errorf("UrlSearchParameters lost on round-trip: got %v, want [Mod.ACT_Item.Filter]",
			got.URLSearchParameters)
	}

	// The other direction: a microflow that has no deep link must not acquire
	// one, or the fix is a different silent change in the same place. An empty
	// UrlSearchParameters must stay the empty marker-1 list the codec writes.
	none := &microflows.Microflow{Name: "ACT_Plain"}
	none.ID = model.ID("mf-5")
	if got := roundTripMicroflow(t, none); got.URL != "" || len(got.URLSearchParameters) != 0 {
		t.Errorf("deep link invented on round-trip: URL=%q params=%v", got.URL, got.URLSearchParameters)
	}
}

// TestMicroflowRoundTrip_ExportLevel is the #1120 sibling found by the audit
// the fix prompted: grepping microflowToGen for the constants it writes turned
// up `SetExportLevel("Hidden")` next to `SetUrl("")`.
//
// Export level is Studio Pro's Hidden/API switch — whether the microflow is
// part of the module's public surface when the module is exported as a package.
// Pinning it to Hidden quietly shrinks a protected module's API, and like the
// URL it has no checker behind it: a hidden microflow is a valid microflow.
//
// Measured across three real marketplace modules (Business Events 3.12.0,
// External Database Connector 6.2.3 and 6.3.0): 3 of 3 microflows and 55 of 55
// documents overall store "Hidden", all three modules exporting at module level
// "Source". So Hidden is the right DEFAULT — the assertion below pins that it
// stays one, rather than becoming the only reachable value again.
func TestMicroflowRoundTrip_ExportLevel(t *testing.T) {
	api := &microflows.Microflow{Name: "ACT_PublicApi", ExportLevel: "API"}
	api.ID = model.ID("mf-6")
	if got := roundTripMicroflow(t, api); got.ExportLevel != "API" {
		t.Errorf("export level demoted on round-trip: got %q, want %q — the "+
			"microflow has silently left the module's public API", got.ExportLevel, "API")
	}

	// The default has to hold in both of its forms. A microflow that says
	// nothing must come back "Hidden" — never "", which is not a member of
	// MicroflowsExportLevel and is exactly the kind of value that gives a
	// document mxbuild accepts and Studio Pro cannot open.
	for _, stored := range []string{"", "Hidden"} {
		mf := &microflows.Microflow{Name: "ACT_Internal", ExportLevel: stored}
		mf.ID = model.ID("mf-7")
		if got := roundTripMicroflow(t, mf); got.ExportLevel != "Hidden" {
			t.Errorf("stored %q came back %q, want %q", stored, got.ExportLevel, "Hidden")
		}
	}
}

// TestMicroflowRoundTrip_ConcurrencyErrorHandling covers the other half of the
// concurrency settings: what Mendix does to a second caller when concurrent
// execution is disallowed. Mendix requires one of the two (CE4899), and the
// writer emitted an empty message and no microflow unconditionally, so a
// rewrite deleted the answer — translations included.
func TestMicroflowRoundTrip_ConcurrencyErrorHandling(t *testing.T) {
	mf := &microflows.Microflow{
		Name:                     "ACT_Serial",
		AllowConcurrentExecution: false,
		ConcurrencyErrorMessage: &model.Text{Translations: map[string]string{
			"en_US": "Already running",
			"nl_NL": "Wordt al uitgevoerd",
		}},
		ConcurrencyErrorMicroflow: "MyModule.ACT_OnBusy",
	}
	mf.ID = model.ID("mf-8")

	got := roundTripMicroflow(t, mf)
	if got.AllowConcurrentExecution {
		t.Error("AllowConcurrentExecution flipped to true — the app's concurrency protection is gone")
	}
	if got.ConcurrencyErrorMicroflow != "MyModule.ACT_OnBusy" {
		t.Errorf("ConcurrencyErrorMicroflow = %q, want MyModule.ACT_OnBusy", got.ConcurrencyErrorMicroflow)
	}
	// Every translation, not just the source language: a message that comes
	// back with one of its two languages is the translated-caption loss that
	// the delete-then-create path produced elsewhere.
	if got.ConcurrencyErrorMessage == nil {
		t.Fatal("ConcurrencyErrorMessage lost entirely → CE4899 on a disallow-concurrency microflow")
	}
	for lang, want := range mf.ConcurrencyErrorMessage.Translations {
		if got.ConcurrencyErrorMessage.Translations[lang] != want {
			t.Errorf("translation %s = %q, want %q", lang,
				got.ConcurrencyErrorMessage.Translations[lang], want)
		}
	}
}

// TestMicroflowRoundTrip_NoConcurrencyMessageIsInert is the control that keeps
// the carry from disturbing the ordinary microflow — the one with no
// concurrency message at all, which is nearly all of them.
//
// Before the carry, the writer always emitted a bare empty Texts$Text. A nil
// message must still produce exactly that, or every microflow in every project
// would come out different and defeat write elision (ADR-0008) — a "fix" that
// rewrites the whole project is worse than the bug.
//
// The comparison is canon.Equal on the MESSAGE ELEMENT, and both halves of that
// are deliberate. Not bytes: every encode mints a fresh random $ID per
// sub-element, which is the reason canon compares a canonical form at all. Not
// the whole microflow either: canon.Equal does not mask, and a microflow's
// StableId is a fresh GUID *value* on every encode, so two encodes of one
// unchanged microflow are never Equal — only Reconcile, which masks the
// identity fields, may be asked that question.
func TestMicroflowRoundTrip_NoConcurrencyMessageIsInert(t *testing.T) {
	encMessage := func(t *testing.T, mf *microflows.Microflow) []byte {
		t.Helper()
		mf.ID = model.ID("mf-9")
		raw, err := (&codec.Encoder{}).Encode(microflowToGen(mf, 11).ConcurrencyErrorMessage())
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return raw
	}

	absent := encMessage(t, &microflows.Microflow{Name: "ACT_Plain", AllowConcurrentExecution: true})
	empty := encMessage(t, &microflows.Microflow{
		Name: "ACT_Plain", AllowConcurrentExecution: true,
		ConcurrencyErrorMessage: &model.Text{Translations: map[string]string{}},
	})
	same, err := canon.Equal(absent, empty)
	if err != nil {
		t.Fatalf("canon.Equal: %v", err)
	}
	if !same {
		t.Error("a nil concurrency message no longer encodes as the bare empty Texts$Text " +
			"the writer always wrote; every microflow in every project would be rewritten " +
			"on the next run (ADR-0008 elision)")
	}

	// The control for the control: canon.Equal must still be able to tell an
	// empty message from a real one, or the assertion above proves nothing.
	real := encMessage(t, &microflows.Microflow{
		Name: "ACT_Plain", AllowConcurrentExecution: true,
		ConcurrencyErrorMessage: &model.Text{Translations: map[string]string{"en_US": "Busy"}},
	})
	if same, _ := canon.Equal(absent, real); same {
		t.Error("canon.Equal cannot see a real concurrency message")
	}

	// And the model must not invent one on the way back.
	plain := &microflows.Microflow{Name: "ACT_Plain", AllowConcurrentExecution: true}
	plain.ID = model.ID("mf-9")
	if got := roundTripMicroflow(t, plain); got.ConcurrencyErrorMessage != nil {
		t.Errorf("an absent concurrency message came back as %#v, want nil",
			got.ConcurrencyErrorMessage)
	}
}
