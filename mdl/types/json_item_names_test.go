// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// An array's ITEM element has no JSON key of its own — it is the anonymous
// `[…]` entry — so CUSTOM NAME MAP could not reach it and its name was derived
// and unspellable (ako/mxcli#272). The name matters: a mapping element clones
// the schema element's ExposedName, which is one of the two names a member
// resolves by (#882), so an unnameable item forced every mapping over an
// mxcli-built structure to carry a generated name.

func itemNamed(t *testing.T, snippet string, itemMap map[string]string) []*JsonElement {
	t.Helper()
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, itemMap)
	if err != nil {
		t.Fatalf("BuildJsonElementsFromSnippet: %v", err)
	}
	return elems
}

// find returns the first element at the given path.
func find(elems []*JsonElement, path string) *JsonElement {
	var hit *JsonElement
	var walk func(e *JsonElement)
	walk = func(e *JsonElement) {
		if e.Path == path && hit == nil {
			hit = e
		}
		for _, c := range e.Children {
			walk(c)
		}
	}
	for _, e := range elems {
		walk(e)
	}
	return hit
}

func TestItemOfNamesAnObjectArrayItem(t *testing.T) {
	elems := itemNamed(t, `{"data": [{"id": 1}]}`, map[string]string{"data": "Record"})

	item := find(elems, "(Object)|data|(Object)")
	if item == nil {
		t.Fatal("no item element at (Object)|data|(Object)")
	}
	if item.ExposedName != "Record" {
		t.Errorf("ExposedName = %q, want Record", item.ExposedName)
	}
}

// TestItemOfNamesAPrimitiveArrayWrapper pins that the primitive case is the same
// spelling. The Wrapper IS the item element for a primitive array — one per
// entry — so a second syntax for it would be a second way to say one thing.
func TestItemOfNamesAPrimitiveArrayWrapper(t *testing.T) {
	elems := itemNamed(t, `{"tags": ["a"]}`, map[string]string{"tags": "Label"})

	item := find(elems, "(Object)|tags|(Wrapper)")
	if item == nil {
		t.Fatal("no wrapper element at (Object)|tags|(Wrapper)")
	}
	if item.ExposedName != "Label" {
		t.Errorf("ExposedName = %q, want Label", item.ExposedName)
	}
}

// TestItemOfNamesARootArrayItem pins the sentinel. A root array has no JSON key,
// so without "Root" its item — the element every mapping over an array-rooted
// structure binds (#248) — would stay unnameable.
func TestItemOfNamesARootArrayItem(t *testing.T) {
	elems := itemNamed(t, `[{"id": 1}]`, map[string]string{RootArrayItemKey: "Entry"})

	item := find(elems, "(Array)|(Object)")
	if item == nil {
		t.Fatal("no item element at (Array)|(Object)")
	}
	if item.ExposedName != "Entry" {
		t.Errorf("ExposedName = %q, want Entry", item.ExposedName)
	}
}

// TestUnnamedItemsKeepTheGeneratedName is the control: the new clause must not
// change what a structure that does not use it produces. Changing the default
// would rewrite every stored structure's item names on the next exec, and every
// mapping bound to one.
func TestUnnamedItemsKeepTheGeneratedName(t *testing.T) {
	elems := itemNamed(t, `{"plain": [{"x": 1}], "tags": ["a"]}`, map[string]string{})

	if got := find(elems, "(Object)|plain|(Object)"); got == nil || got.ExposedName != "PlainItem" {
		t.Errorf("object-array item = %v, want PlainItem", got)
	}
	if got := find(elems, "(Object)|tags|(Wrapper)"); got == nil || got.ExposedName != "Tag" {
		t.Errorf("primitive-array wrapper = %v, want Tag", got)
	}
}

// TestItemNameIsIndependentOfTheArrayRename pins that the two clauses do not
// interfere: naming an item must not require renaming its array, and renaming an
// array must not silently rename the item with it.
func TestItemNameIsIndependentOfTheArrayRename(t *testing.T) {
	elems, err := BuildJsonElementsFromSnippet(`{"data": [{"id": 1}]}`,
		map[string]string{"data": "Records"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := find(elems, "(Object)|data"); got == nil || got.ExposedName != "Records" {
		t.Errorf("array = %v, want Records", got)
	}
	// The item still derives from the array's NEW name, which is the documented
	// default — the point is only that it is a default, now overridable.
	if got := find(elems, "(Object)|data|(Object)"); got == nil || got.ExposedName != "RecordsItem" {
		t.Errorf("item = %v, want RecordsItem", got)
	}
}

// TestSnippetKeysReportsWhatAnEntryCanAddress backs the check-time rule: an
// entry naming a key the snippet does not contain applied to nothing and said
// nothing, which is what made the missing `item of` invisible.
func TestSnippetKeysReportsWhatAnEntryCanAddress(t *testing.T) {
	keys, arrays, rootIsArray, err := SnippetKeys(`{"data": [{"id": 1}], "name": "x"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"data", "id", "name"} {
		if !keys[want] {
			t.Errorf("key %q missing", want)
		}
	}
	if !arrays["data"] {
		t.Error("data should be reported as an array")
	}
	if arrays["name"] {
		t.Error("name is not an array")
	}
	if rootIsArray {
		t.Error("root is an object here")
	}
}

func TestSnippetKeysMarksARootArray(t *testing.T) {
	_, arrays, rootIsArray, err := SnippetKeys(`[{"id": 1}]`)
	if err != nil {
		t.Fatal(err)
	}
	if !rootIsArray {
		t.Error("root array not reported")
	}
	if !arrays[RootArrayItemKey] {
		t.Errorf("%q should address the root array", RootArrayItemKey)
	}
}
