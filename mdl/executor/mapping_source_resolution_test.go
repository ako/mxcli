// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	mdltypes "github.com/mendixlabs/mxcli/mdl/types"
)

// A mapping's schema source was written through unresolved (#259). mxbuild
// reports the dangling reference as CE1613 "… no longer exists", but only at
// build time — `mxcli check` and `check --references` both passed.
//
// The second failure is the one with teeth: the schema index is empty when the
// structure cannot be loaded FOR ANY REASON, and an empty index reads as "there
// is nothing to validate against", so ONE typo in the structure name turned off
// every member check in the mapping. A typo'd source and a member that does not
// exist both went through unremarked.

func structureBackend(names ...string) *mock.MockBackend {
	all := make([]*mdltypes.JsonStructure, 0, len(names))
	for _, n := range names {
		all = append(all, &mdltypes.JsonStructure{Name: n})
	}
	return &mock.MockBackend{
		GetJsonStructureByQualifiedNameFunc: func(_, name string) (*mdltypes.JsonStructure, error) {
			for _, s := range all {
				if s.Name == name {
					return s, nil
				}
			}
			return nil, nil
		},
		ListJsonStructuresFunc: func() ([]*mdltypes.JsonStructure, error) { return all, nil },
	}
}

func TestResolveJsonStructureSourceRefusesUnknown(t *testing.T) {
	b := structureBackend("JSON_Order", "JSON_Customer")

	_, err := resolveJsonStructureSource(b, ast.QualifiedName{Module: "Shop", Name: "NoSuchThing"})
	if err == nil {
		t.Fatal("accepted a structure name that does not resolve — mxbuild reports CE1613, " +
			"and the empty index silently disables every member check")
	}
	// Name what would have worked, the shape #882 established for members.
	for _, want := range []string{"JSON_Order", "JSON_Customer"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not list %q", err, want)
		}
	}
}

// The control: a name that resolves must return a usable index, or the refusal
// has simply been inverted.
func TestResolveJsonStructureSourceAcceptsKnown(t *testing.T) {
	b := structureBackend("JSON_Order")
	b.GetJsonStructureByQualifiedNameFunc = func(_, name string) (*mdltypes.JsonStructure, error) {
		return &mdltypes.JsonStructure{
			Name:     name,
			Elements: []*mdltypes.JsonElement{{Path: "(Object)", ElementType: "Object"}},
		}, nil
	}
	idx, err := resolveJsonStructureSource(b, ast.QualifiedName{Module: "Shop", Name: "JSON_Order"})
	if err != nil {
		t.Fatalf("refused a structure that exists: %v", err)
	}
	if idx == nil || !idx.resolvable() {
		t.Error("resolved structure produced an index with nothing in it")
	}
}

// A backend that cannot LIST must not turn every mapping into an error: only a
// positive "it is not there" is a refusal. An empty project is a different case
// and IS refused — with an error that says to create the structure first.
func TestResolveJsonStructureSourceTolerantOfUnlistableBackend(t *testing.T) {
	b := &mock.MockBackend{
		ListJsonStructuresFunc: func() ([]*mdltypes.JsonStructure, error) {
			return nil, errors.New("this backend cannot list JSON structures")
		},
	}
	idx, err := resolveJsonStructureSource(b, ast.QualifiedName{Module: "Shop", Name: "Whatever"})
	if err != nil {
		t.Fatalf("refused on a backend that cannot list: %v", err)
	}
	if idx == nil {
		t.Fatal("returned no index")
	}
}

// An EMPTY project is not the same as an unlistable one: the name is definitely
// wrong, and the error should say what to do rather than list nothing.
func TestResolveJsonStructureSourceEmptyProject(t *testing.T) {
	_, err := resolveJsonStructureSource(structureBackend(),
		ast.QualifiedName{Module: "Shop", Name: "Whatever"})
	if err == nil {
		t.Fatal("accepted a structure name in a project with no structures at all")
	}
	if !strings.Contains(err.Error(), "create json structure") {
		t.Errorf("error %q does not say how to fix it", err)
	}
}

// `null values` is an enum, not free text: mxcli wrote whatever identifier it
// was given straight into the document, so `null values Banana` reached the
// stored NullValueOption verbatim.
func TestCanonicalNullValueOption(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"", "", true}, // unauthored
		{"LeaveOutElement", "LeaveOutElement", true}, // the two members
		{"SendAsNil", "SendAsNil", true},
		{"sendasnil", "SendAsNil", true}, // spelling is normalised
		{"Banana", "", false},            // the reported case
		{"LeaveOut", "", false},          // a near miss is still a miss
	} {
		got, err := canonicalNullValueOption(tc.in)
		if tc.ok && err != nil {
			t.Errorf("canonicalNullValueOption(%q) errored: %v", tc.in, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("canonicalNullValueOption(%q) was accepted; it is not in the enum", tc.in)
		}
		if tc.ok && got != tc.want {
			t.Errorf("canonicalNullValueOption(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
