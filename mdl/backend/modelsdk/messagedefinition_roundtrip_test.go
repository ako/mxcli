// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"os"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genMsg "github.com/mendixlabs/mxcli/modelsdk/gen/messagedefinitions"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// The fixture is ako/TestApp's OrderMessageDefinitions — a collection authored
// by hand in Studio Pro rather than shipped in a marketplace module, which is
// what makes it the right oracle for a WRITE path.
//
// It also happens to contain the control the direction rule needs: the SAME
// association, Mappings.Order_Customer, appears in both definitions and stores
// a different MaxOccurs each time (1 from Order, -1 from Customer). Nothing
// synthetic would have made that as convincing.
const messageFixture = "testdata/TestApp.OrderMessageDefinitions.bson"

func loadMessageFixture(t *testing.T) (*genMsg.MessageDefinitionCollection, []byte) {
	t.Helper()
	raw, err := os.ReadFile(messageFixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	g := genMsg.NewMessageDefinitionCollection()
	g.InitFromRaw(bson.Raw(raw))
	return g, raw
}

// TestMessageDefinitionCollectionRoundTrips reads a real document into the
// semantic model and writes it back, asserting the re-encoded document is
// semantically identical.
//
// This is the test that caught two derivations being wrong: Path is a chain of
// ORIGINAL names, not exposed ones, and an ASSOCIATION contributes TWO segments
// (its own name, then the target entity's). Both were confirmed afterwards at
// 4,707 of 4,707 elements across this document and the nine demo apps.
func TestMessageDefinitionCollectionRoundTrips(t *testing.T) {
	orig, storedBytes := loadMessageFixture(t)

	// Read into the semantic model exactly as the backend does.
	c := messageCollectionFromGen(orig, "")

	rebuilt, err := messageCollectionToGen(c)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	after, err := (&codec.Encoder{}).Encode(rebuilt)
	if err != nil {
		t.Fatalf("encode rebuilt: %v", err)
	}

	// Compare against the STORED BYTES, not against a re-encoding of the
	// decoded original: a lazily-decoded element that was never marked dirty
	// encodes as an empty document, so that baseline would pass by comparing
	// nothing to nothing.
	diffs := bsonDiff(t, storedBytes, after)
	if len(diffs) > 0 {
		for _, d := range diffs {
			t.Errorf("DIFF %s", d)
		}
	}
}

// TestFixtureCarriesTheDirectionControl pins the property that makes this
// fixture worth having: one association, two directions, two cardinalities.
//
// The rule is that MaxOccurs tracks the direction of traversal, not the
// association's type. Order_Customer is FROM Order TO Customer, so reaching
// Customer from Order is single (following the FK) and reaching Order from
// Customer is unbounded (the reverse). Getting this backwards produces a
// definition that exposes a list as a single object, with no build error behind
// it — which is why the control lives in a test rather than a comment.
func TestFixtureCarriesTheDirectionControl(t *testing.T) {
	g, _ := loadMessageFixture(t)
	c := messageCollectionFromGen(g, "")

	var found []string
	var walk func(n *model.MessageDefinitionElement)
	walk = func(n *model.MessageDefinitionElement) {
		if n == nil {
			return
		}
		if n.Association == "Mappings.Order_Customer" {
			found = append(found, fmt.Sprintf("%s:%d", n.Entity, n.MaxOccurs))
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, def := range c.Definitions {
		walk(def.Root)
	}
	if len(found) != 2 {
		t.Fatalf("expected Order_Customer twice, got %v", found)
	}
	want := map[string]bool{"Mappings.Customer:1": true, "Mappings.Order:-1": true}
	for _, f := range found {
		if !want[f] {
			t.Errorf("unexpected %s — the direction rule is: holder is FROM -> 1, holder is TO -> -1", f)
		}
	}
}

// bsonDiff compares two encoded documents field by field and returns the paths
// that differ, so a failure names what changed rather than dumping two blobs.
func bsonDiff(t *testing.T, a, b []byte) []string {
	t.Helper()
	var da, db bson.M
	if err := bson.Unmarshal(a, &da); err != nil {
		t.Fatalf("unmarshal a: %v", err)
	}
	if err := bson.Unmarshal(b, &db); err != nil {
		t.Fatalf("unmarshal b: %v", err)
	}
	var out []string
	var cmp func(path string, x, y any)
	cmp = func(path string, x, y any) {
		mx, xok := asMap(x)
		my, yok := asMap(y)
		if xok && yok {
			seen := map[string]bool{}
			for k := range mx {
				seen[k] = true
			}
			for k := range my {
				seen[k] = true
			}
			for k := range seen {
				if k == "$ID" {
					continue // freshly minted on rebuild; not a semantic difference
				}
				cmp(path+"/"+k, mx[k], my[k])
			}
			return
		}
		ax, xok := x.(bson.A)
		ay, yok := y.(bson.A)
		if xok && yok {
			if len(ax) != len(ay) {
				out = append(out, fmt.Sprintf("%s: len %d != %d", path, len(ax), len(ay)))
				return
			}
			for i := range ax {
				cmp(fmt.Sprintf("%s/%d", path, i), ax[i], ay[i])
			}
			return
		}
		if fmt.Sprint(x) != fmt.Sprint(y) {
			out = append(out, fmt.Sprintf("%s: %.90v -> %.90v", path, x, y))
		}
	}
	cmp("", da, db)
	return out
}

// asMap normalises the two shapes the driver decodes a sub-document into, so the
// walk recurses instead of falling through to a string compare — which is how a
// nested difference hides behind a matching prefix.
func asMap(v any) (bson.M, bool) {
	switch t := v.(type) {
	case bson.M:
		return t, true
	case bson.D:
		m := make(bson.M, len(t))
		for _, e := range t {
			m[e.Key] = e.Value
		}
		return m, true
	default:
		return nil, false
	}
}
