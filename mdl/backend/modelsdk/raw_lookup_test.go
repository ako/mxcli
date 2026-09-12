// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func connectedBackend(t *testing.T) *Backend {
	t.Helper()
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	return b
}

// The name-indexed lookup the executor's fast paths use. Before this existed the
// modelsdk engine answered errUnimplemented and every one of those four call
// sites fell through to an O(n) walk of the module's microflows.
func TestGetRawUnitByName(t *testing.T) {
	b := connectedBackend(t)

	unit, err := b.GetRawUnitByName("microflow", "FeedbackModule.ConvertBase64String")
	if err != nil {
		t.Fatalf("GetRawUnitByName: %v", err)
	}
	if unit == nil {
		t.Fatal("microflow not found by name")
	}
	if len(unit.Contents) == 0 {
		t.Error("unit found but carries no contents; the fast path reads Contents")
	}

	// A name that is not there must not resolve — the callers treat a hit as
	// proof the microflow exists (microflowExists), so a lenient lookup would
	// silence a real "no such microflow" report.
	if got, _ := b.GetRawUnitByName("microflow", "FeedbackModule.NoSuchMicroflow"); got != nil {
		t.Errorf("a missing microflow resolved to %+v", got)
	}
}

// ParseMicroflowBSON is asked about nanoflows too, and the codec decodes those
// to a different gen type — the legacy parser only coped because it walked an
// untyped map. ReturnType is the field every caller reads.
func TestParseMicroflowBSON(t *testing.T) {
	b := connectedBackend(t)

	// Note that a Void microflow still carries a DataType — "no return value" is
	// a type, not a nil. Asserting on presence would have passed against a parse
	// that produced nothing, so these compare the type name.
	for _, tc := range []struct {
		objectType string
		name       string
		wantType   string
	}{
		{"microflow", "FeedbackModule.ConvertBase64String", "String"},
		{"microflow", "Administration.SaveNewAccount", "Void"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unit, err := b.GetRawUnitByName(tc.objectType, tc.name)
			if err != nil || unit == nil {
				t.Fatalf("GetRawUnitByName(%s): %v", tc.name, err)
			}
			mf, err := b.ParseMicroflowBSON(unit.Contents, "", "")
			if err != nil {
				t.Fatalf("ParseMicroflowBSON: %v", err)
			}
			if mf == nil {
				t.Fatal("parsed to nil")
			}
			if mf.ReturnType == nil {
				t.Fatalf("no ReturnType, want %s", tc.wantType)
			}
			if got := mf.ReturnType.GetTypeName(); got != tc.wantType {
				t.Errorf("ReturnType = %s, want %s", got, tc.wantType)
			}
			// The name proves the document was really decoded rather than a
			// zero value returned — a nil ReturnType alone is indistinguishable
			// from a parse that produced nothing.
			if mf.Name == "" {
				t.Error("no Name on the parsed microflow")
			}
		})
	}

	if _, err := b.ParseMicroflowBSON([]byte("not bson"), "", ""); err == nil {
		t.Error("malformed BSON parsed without error")
	}
}

// The fast path is what the four executor call sites use; this is the property
// that makes replacing the slow path safe — both must agree on the return type.
func TestFastPathReturnTypeMatchesTheSlowPath(t *testing.T) {
	b := connectedBackend(t)

	all, err := b.ListMicroflows()
	if err != nil {
		t.Fatalf("ListMicroflows: %v", err)
	}
	byName := map[string]*microflows.Microflow{}
	for _, mf := range all {
		if mf != nil {
			byName[mf.Name] = mf
		}
	}
	if len(byName) == 0 {
		t.Fatal("no microflows in the fixture; the comparison would be vacuous")
	}

	checked := 0
	for _, qualified := range []string{
		"FeedbackModule.ConvertBase64String",
		"FeedbackModule.ConvertUUIDToURL",
		"Administration.SaveNewAccount",
		"Administration.ChangePassword",
	} {
		unit, err := b.GetRawUnitByName("microflow", qualified)
		if err != nil || unit == nil {
			t.Errorf("%s: not found by name", qualified)
			continue
		}
		fast, err := b.ParseMicroflowBSON(unit.Contents, "", "")
		if err != nil || fast == nil {
			t.Errorf("%s: parse failed: %v", qualified, err)
			continue
		}
		slow := byName[fast.Name]
		if slow == nil {
			t.Errorf("%s: the slow path does not list it", qualified)
			continue
		}
		fastType, slowType := "", ""
		if fast.ReturnType != nil {
			fastType = fast.ReturnType.GetTypeName()
		}
		if slow.ReturnType != nil {
			slowType = slow.ReturnType.GetTypeName()
		}
		if fastType != slowType {
			t.Errorf("%s: fast path says %q, slow path says %q", qualified, fastType, slowType)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("compared nothing")
	}
}
