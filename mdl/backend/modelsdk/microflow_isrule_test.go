// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
)

// TestIsRule guards issue #723 §A (A4 / CE0117). The modelsdk backend never
// implemented IsRule, so it fell back to the embedded `unimplemented` stub,
// which returns an error. The flow-builder's rule detection
// (`isRule, err := backend.IsRule(...); if err != nil || !isRule { return nil }`)
// then treated every `if Module.SomeRule(...)` as a plain expression and emitted
// an invalid ExpressionSplitCondition → mx check CE0117 "Error in expression".
//
// IsRule must (a) NOT error on the modelsdk engine, (b) return true for a real
// rule's qualified name, and (c) return false for a non-rule.
func TestIsRule(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName(MyFirstModule) = %v, %v", mod, err)
	}

	// Insert a Microflows$Rule document into the module (rules share the
	// microflow namespace but are stored under a distinct BSON $Type). MDL
	// cannot create rules, so build one directly.
	const ruleID = "11111111-1111-1111-1111-111111111111"
	r := genMf.NewRule()
	r.SetID(element.ID(ruleID))
	r.SetName("Rule_IsAdmin")
	r.SetExportLevel("Hidden")
	contents, err := (&codec.Encoder{}).Encode(r)
	if err != nil {
		t.Fatalf("encode rule: %v", err)
	}
	if err := b.writer.InsertUnit(ruleID, string(mod.ID), "Documents", "Microflows$Rule", contents); err != nil {
		t.Fatalf("insert rule unit: %v", err)
	}

	// (a)+(b): the rule's qualified name resolves as a rule, with no error.
	isRule, err := b.IsRule("MyFirstModule.Rule_IsAdmin")
	if err != nil {
		t.Fatalf("IsRule returned error (unimplemented on modelsdk → CE0117): %v", err)
	}
	if !isRule {
		t.Error("IsRule(MyFirstModule.Rule_IsAdmin) = false, want true")
	}

	// (c): a non-rule name is not a rule (and still no error).
	notRule, err := b.IsRule("MyFirstModule.NotARule")
	if err != nil {
		t.Fatalf("IsRule(non-rule) error: %v", err)
	}
	if notRule {
		t.Error("IsRule(MyFirstModule.NotARule) = true, want false")
	}

	// Empty name short-circuits to false, no error.
	if ok, err := b.IsRule(""); ok || err != nil {
		t.Errorf("IsRule(\"\") = %v, %v; want false, nil", ok, err)
	}
}

// TestIsRule_ResolvesOneModule bounds what one IsRule costs. Resolving a
// rule's module lists the whole project, and on an MPR v1 project every
// listing reads every unit's contents from SQLite (137 MB on Evora). IsRule
// used to resolve the module of EVERY rule before comparing names, and
// moduleNameFor listed the modules again for every ancestor it walked — one
// call took 53 s on a 29-rule app, and describe of a flow with two rule splits
// asked once per split per derived-layout round, so it never finished.
//
// The rule asked for is inserted last, so a lookup that resolves every rule
// it passes resolves all of them; the rule count is the control.
func TestIsRule_ResolvesOneModule(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName(MyFirstModule) = %v, %v", mod, err)
	}
	// Rules nested two folders deep, as marketplace modules keep them
	// (Evora's sits in Private/Objects/PromptToUse): the module is found by
	// walking up, and the walk must not list the project once per step.
	container := mod.ID
	for _, f := range []string{"Private", "Rules"} {
		folder := &model.Folder{Name: f, ContainerID: container}
		if err := b.CreateFolder(folder); err != nil {
			t.Fatalf("create folder %s: %v", f, err)
		}
		container = folder.ID
	}
	const rules = 6
	for i := 0; i < rules; i++ {
		id := fmt.Sprintf("22222222-2222-2222-2222-%012d", i)
		r := genMf.NewRule()
		r.SetID(element.ID(id))
		r.SetName(fmt.Sprintf("Rule_%d", i))
		r.SetExportLevel("Hidden")
		contents, err := (&codec.Encoder{}).Encode(r)
		if err != nil {
			t.Fatalf("encode rule: %v", err)
		}
		if err := b.writer.InsertUnit(id, string(container), "Documents", "Microflows$Rule", contents); err != nil {
			t.Fatalf("insert rule unit: %v", err)
		}
	}

	for _, tc := range []struct {
		name     string
		want     bool
		listings int64 // at most
	}{
		// One module resolution: the unit listing plus the module listing.
		{"MyFirstModule.Rule_5", true, 2},
		// A name no rule carries resolves nothing.
		{"MyFirstModule.NotARule", false, 0},
		// The right name in the wrong module is resolved, and rejected.
		{"OtherModule.Rule_3", false, 2},
	} {
		before := moduleNameListings.Load()
		got, err := b.IsRule(tc.name)
		listings := moduleNameListings.Load() - before
		if err != nil || got != tc.want {
			t.Errorf("IsRule(%s) = %v, %v; want %v", tc.name, got, err, tc.want)
		}
		if listings > tc.listings {
			t.Errorf("IsRule(%s) listed the project %d times with %d rules; want at most %d",
				tc.name, listings, rules, tc.listings)
		}
	}
}
