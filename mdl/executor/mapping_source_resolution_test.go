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

// ---------------------------------------------------------------------------
// XML schema half of #259
// ---------------------------------------------------------------------------

// xmlBackend returns a backend whose XML schema list is exactly these qualified
// names.
func xmlBackend(qualified ...string) *mock.MockBackend {
	all := make([]*mdltypes.XmlSchema, 0, len(qualified))
	for _, qn := range qualified {
		i := strings.LastIndex(qn, ".")
		all = append(all, &mdltypes.XmlSchema{Module: qn[:i], Name: qn[i+1:]})
	}
	return &mock.MockBackend{
		ListXmlSchemasFunc: func() ([]*mdltypes.XmlSchema, error) { return all, nil },
	}
}

// TestResolveXmlSchemaSourceRefusesUnknown pins the refusal.
//
// mxbuild reports the dangling reference as CE1613 "The selected XML schema 'X'
// no longer exists", a whole build later. There is no `create xml schema` in
// MDL, so the reference can only ever point at something a human put there —
// which is exactly why a typo in it had nothing to catch it.
func TestResolveXmlSchemaSourceRefusesUnknown(t *testing.T) {
	b := xmlBackend("Shop.Orders_Xsd", "Shop.Invoice_Xsd")

	err := resolveXmlSchemaSource(b, ast.QualifiedName{Module: "Shop", Name: "NoSuchThing"})
	if err == nil {
		t.Fatal("accepted an XML schema name that does not resolve — mxbuild reports CE1613")
	}
	for _, want := range []string{"Shop.Orders_Xsd", "Shop.Invoice_Xsd"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not list %q", err, want)
		}
	}
}

// TestResolveXmlSchemaSourceAcceptsKnown is the control: a name that resolves
// must pass, or the refusal has simply been inverted.
func TestResolveXmlSchemaSourceAcceptsKnown(t *testing.T) {
	b := xmlBackend("Shop.Orders_Xsd")

	if err := resolveXmlSchemaSource(b, ast.QualifiedName{Module: "Shop", Name: "Orders_Xsd"}); err != nil {
		t.Fatalf("refused a schema that exists: %v", err)
	}
}

// TestResolveXmlSchemaSourceIsModuleAware pins that the match is on the
// QUALIFIED name. Matching on the bare name would accept `A.Foo` because `B.Foo`
// exists — a dangling reference that still reaches mxbuild, which is the whole
// failure being fixed.
func TestResolveXmlSchemaSourceIsModuleAware(t *testing.T) {
	b := xmlBackend("Other.Orders_Xsd")

	if err := resolveXmlSchemaSource(b, ast.QualifiedName{Module: "Shop", Name: "Orders_Xsd"}); err == nil {
		t.Fatal("accepted Shop.Orders_Xsd because Other.Orders_Xsd exists")
	}
}

// TestResolveXmlSchemaSourceFailsOpenOnEmptyProject pins the direction that
// keeps this rule from being a regression.
//
// MDL cannot create an XML schema, and they are rare: 3 documents in 1 of the 9
// demo apps. A project with no XML schemas is therefore the ordinary case, not
// evidence that the name is wrong; refusing on it would break every such mapping
// in exchange for catching almost nothing.
func TestResolveXmlSchemaSourceFailsOpenOnEmptyProject(t *testing.T) {
	b := xmlBackend()

	if err := resolveXmlSchemaSource(b, ast.QualifiedName{Module: "Shop", Name: "Anything"}); err != nil {
		t.Fatalf("refused against a project with no XML schemas: %v", err)
	}
}

// TestResolveXmlSchemaSourceFailsOpenWhenListUnavailable covers a backend that
// cannot answer at all — an engine that does not implement the read, or an
// unconfigured mock. Taking the name at face value is right: nothing was
// learned, so nothing should be asserted.
func TestResolveXmlSchemaSourceFailsOpenWhenListUnavailable(t *testing.T) {
	b := &mock.MockBackend{
		ListXmlSchemasFunc: func() ([]*mdltypes.XmlSchema, error) {
			return nil, errors.New("backend cannot list")
		},
	}

	if err := resolveXmlSchemaSource(b, ast.QualifiedName{Module: "Shop", Name: "Anything"}); err != nil {
		t.Fatalf("refused when the backend could not answer: %v", err)
	}
}
