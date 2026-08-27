package types

import (
	"testing"
)

// walkElements visits every element in the tree, depth-first.
func walkElements(elems []*JsonElement, visit func(*JsonElement)) {
	for _, e := range elems {
		if e == nil {
			continue
		}
		visit(e)
		walkElements(e.Children, visit)
	}
}

// TestBuildJsonElementsFromSnippet_NoZeroMaxOccurs guards issue #841.
//
// Every element was written with MinOccurs=0, MaxOccurs=0. Mendix reads that
// literally as "never occurs", so an import mapping bound to the structure
// silently produced zero objects — the REST call succeeded and no data arrived.
// Nothing flagged it: no validation error, and mx check passed.
//
// MaxOccurs=0 is never a legitimate value. This is the invariant that matters
// most, independent of the exact bound chosen for any one element.
func TestBuildJsonElementsFromSnippet_NoZeroMaxOccurs(t *testing.T) {
	snippets := map[string]string{
		"object root":     `{"pagination":{"page":1},"items":[{"id":"a","name":"b"}]}`,
		"array root":      `[{"id":"a"}]`,
		"primitive array": `{"tags":["a","b"]}`,
		"scalars":         `{"s":"x","n":1,"b":true,"nil":null}`,
	}
	for name, snippet := range snippets {
		t.Run(name, func(t *testing.T) {
			elems, err := BuildJsonElementsFromSnippet(snippet, nil)
			if err != nil {
				t.Fatalf("BuildJsonElementsFromSnippet: %v", err)
			}
			if len(elems) == 0 {
				t.Fatal("no elements produced")
			}
			walkElements(elems, func(e *JsonElement) {
				if e.MaxOccurs == 0 {
					t.Errorf("element %q (%s) has MaxOccurs=0 — Mendix reads this as \"never occurs\"", e.ExposedName, e.ElementType)
				}
			})
		})
	}
}

// TestBuildJsonElementsFromSnippet_RootOccursOnce pins the root to 1..1.
//
// MaxOccurs=1 was the first half: the root previously carried MaxOccurs=0, which
// Mendix reads as "never occurs".
//
// MinOccurs=1 is what Studio Pro writes — uniformly, across all nine Studio
// Pro-authored structures in the round-trip fixture (ako/mxcli#272).
//
// It could not be 1 until recently, and the reason is worth keeping: Mendix
// cross-validates a mapping element against its schema element, and the mapping
// serializers used to hardcode MinOccurs=0, so writing 1 here failed every bound
// mapping with CE5015 "Attribute 'MinOccurs' does not match schema element
// '(Object)'". #277/#279 made every mapping element mirror the schema element
// instead, so both sides now agree.
//
// The two must move together. If this is ever changed back, change the mapping
// serializers with it — CE5015 is what disagreement looks like, and it is caught
// by the integration suite rather than here, because it needs a mapping bound to
// the structure and a real mx check.
func TestBuildJsonElementsFromSnippet_RootOccursOnce(t *testing.T) {
	for name, snippet := range map[string]string{
		"object root": `{"a":1}`,
		"array root":  `[{"a":1}]`,
	} {
		t.Run(name, func(t *testing.T) {
			elems, err := BuildJsonElementsFromSnippet(snippet, nil)
			if err != nil {
				t.Fatalf("BuildJsonElementsFromSnippet: %v", err)
			}
			root := elems[0]
			if root.MaxOccurs != 1 {
				t.Errorf("root MaxOccurs = %d, want 1 (0 means \"never occurs\")", root.MaxOccurs)
			}
			if root.MinOccurs != 1 {
				t.Errorf("root MinOccurs = %d, want 1 — what Studio Pro writes; the mapping "+
					"elements mirror it, and a mismatch fails every bound mapping with CE5015",
					root.MinOccurs)
			}
		})
	}
}

// TestBuildJsonElementsFromSnippet_RepeatingElementsUnbounded pins the elements
// that actually repeat to unbounded (-1).
//
// A nested object array's item already got this right; the root-array item and
// the primitive-array wrapper did not, which is what made the bug look
// intermittent — one element in the tree came out correct.
func TestBuildJsonElementsFromSnippet_RepeatingElementsUnbounded(t *testing.T) {
	t.Run("nested object array item", func(t *testing.T) {
		elems, err := BuildJsonElementsFromSnippet(`{"items":[{"id":"a"}]}`, nil)
		if err != nil {
			t.Fatalf("BuildJsonElementsFromSnippet: %v", err)
		}
		item := findByName(elems, "ItemsItem")
		if item == nil {
			t.Fatal("ItemsItem not found")
		}
		if item.MaxOccurs != -1 {
			t.Errorf("array item MaxOccurs = %d, want -1 (unbounded)", item.MaxOccurs)
		}
	})

	// A root array follows the same rule as a nested one: the array element
	// occurs once, its item child repeats. Before the fix the root-array item
	// was 0..0 while the nested-array item was already 0..-1 — the same
	// construct written two different ways in two builders.
	t.Run("root array item", func(t *testing.T) {
		elems, err := BuildJsonElementsFromSnippet(`[{"id":"a"}]`, nil)
		if err != nil {
			t.Fatalf("BuildJsonElementsFromSnippet: %v", err)
		}
		root := elems[0]
		if root.ElementType != "Array" {
			t.Fatalf("root ElementType = %q, want Array", root.ElementType)
		}
		item := findByName(elems, "JsonObject")
		if item == nil {
			t.Fatal("JsonObject item not found")
		}
		if item.MaxOccurs != -1 {
			t.Errorf("root array item MaxOccurs = %d, want -1 (unbounded)", item.MaxOccurs)
		}
	})

	t.Run("primitive array wrapper", func(t *testing.T) {
		elems, err := BuildJsonElementsFromSnippet(`{"tags":["a","b"]}`, nil)
		if err != nil {
			t.Fatalf("BuildJsonElementsFromSnippet: %v", err)
		}
		wrapper := findByName(elems, "Tag")
		if wrapper == nil {
			t.Fatal("Tag wrapper not found")
		}
		if wrapper.MaxOccurs != -1 {
			t.Errorf("primitive array wrapper MaxOccurs = %d, want -1 (unbounded)", wrapper.MaxOccurs)
		}
	})
}

func findByName(elems []*JsonElement, name string) *JsonElement {
	var found *JsonElement
	walkElements(elems, func(e *JsonElement) {
		if found == nil && e.ExposedName == name {
			found = e
		}
	})
	return found
}
