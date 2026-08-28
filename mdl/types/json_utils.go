// SPDX-License-Identifier: Apache-2.0

package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

// occursUnbounded is the MaxOccurs value Mendix reads as "unbounded" (`0..*`).
// Note that 0 is NOT a stand-in for "unspecified": Mendix reads MaxOccurs=0
// literally as "never occurs", so an element written that way is silently
// skipped by every import mapping bound to the structure (#841).
const occursUnbounded = -1

// rootMinOccurs is the MinOccurs written for the root element.
//
// It stays 0 even though a document root arguably "always occurs once", because
// Mendix cross-validates a *mapping* element against its *schema* element and
// the mapping serializers hardcode `MinOccurs: 0` (serExportMappingElement /
// serImportMappingElement in both engines) — only MaxOccurs is propagated from
// the schema. Writing 1 here makes the two sides disagree and every mapping
// bound to the structure fails to build:
//
//	[error] [CE5015] "The mapping does not align with the underlying schema
//	anymore. Details: Attribute 'MinOccurs' does not match schema element
//	'(Object)'." at Object mapping element 'Root'
//
// That blocker is gone: #277/#279 made every mapping element mirror the bound
// schema element's MinOccurs instead of hardcoding 0, in all three serializers.
// With both sides agreeing, the root can carry the 1 Studio Pro writes — uniform
// across all nine Studio Pro-authored structures pinned in the round-trip
// fixture, and the last of ako/mxcli#272's measured differences that has a
// well-founded target.
//
// Do NOT change this back without changing the mapping serializers with it: the
// two sides are cross-validated and CE5015 is what disagreement looks like.
const rootMinOccurs = 1

// iso8601Pattern matches common ISO 8601 datetime strings that Mendix Studio Pro
// recognizes as DateTime primitive types in JSON structures.
var iso8601Pattern = regexp.MustCompile(
	`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2})?(\.\d+)?(Z|[+-]\d{2}:?\d{2})?$`,
)

// PrettyPrintJSON re-formats a JSON string with standard indentation.
// Returns the original string if it is not valid JSON.
func PrettyPrintJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", "  "); err != nil {
		return s
	}
	return buf.String()
}

// normalizeDateTimeValue pads fractional seconds to 7 digits to match
// Studio Pro's .NET DateTime format (e.g., "2015-05-22T14:56:29.000Z" → "2015-05-22T14:56:29.0000000Z").
func normalizeDateTimeValue(s string) string {
	// Find the decimal point after seconds
	dotIdx := strings.Index(s, ".")
	if dotIdx == -1 {
		// No fractional part — insert .0000000 before timezone suffix.
		// Search from index 19+ to avoid matching the '-' in the date portion (YYYY-MM-DD).
		if len(s) >= 19 {
			if idx := strings.IndexAny(s[19:], "Z+-"); idx >= 0 {
				pos := 19 + idx
				return s[:pos] + ".0000000" + s[pos:]
			}
		}
		// No timezone suffix — append fractional part at end
		return s + ".0000000"
	}
	// Find where fractional digits end (at Z, +, - or end of string)
	fracEnd := len(s)
	for i := dotIdx + 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			fracEnd = i
			break
		}
	}
	frac := s[dotIdx+1 : fracEnd]
	if len(frac) < 7 {
		frac = frac + strings.Repeat("0", 7-len(frac))
	} else {
		frac = frac[:7]
	}
	return s[:dotIdx+1] + frac + s[fracEnd:]
}

// BuildJsonElementsFromSnippet parses a JSON snippet and builds the element tree
// that Mendix Studio Pro would generate. Returns the root element.
//
// customNameMap maps JSON keys to custom ExposedNames (Studio Pro's "Custom
// name" column). itemNameMap does the same for an ARRAY's ITEM element, keyed by
// the array's JSON key — an item is the anonymous `[…]` entry and has no key of
// its own, so it is unreachable from customNameMap and its name was previously
// derived and unspellable (ako/mxcli#272). "Root" addresses a root-level array's
// item, which likewise has no key.
//
// Both are optional; unmapped elements use the generated names.
func BuildJsonElementsFromSnippet(snippet string, customNameMap, itemNameMap map[string]string) ([]*JsonElement, error) {
	// Validate JSON
	if !json.Valid([]byte(snippet)) {
		return nil, fmt.Errorf("invalid JSON snippet")
	}

	// Detect root type (object or array)
	dec := json.NewDecoder(strings.NewReader(snippet))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON snippet: %w", err)
	}

	b := &snippetBuilder{customNameMap: customNameMap, itemNameMap: itemNameMap}
	tracker := &nameTracker{seen: make(map[string]int)}

	switch tok {
	case json.Delim('{'):
		root := b.buildElementFromRawObject("Root", "(Object)", snippet, tracker)
		root.MinOccurs = rootMinOccurs
		root.MaxOccurs = 1
		root.Nillable = true
		return []*JsonElement{root}, nil

	case json.Delim('['):
		root := b.buildElementFromRawRootArray("Root", "(Array)", snippet, tracker)
		// The array element itself occurs once; its item child carries the
		// repetition, mirroring the nested-array case.
		root.MinOccurs = rootMinOccurs
		root.MaxOccurs = 1
		root.Nillable = true
		return []*JsonElement{root}, nil

	default:
		return nil, fmt.Errorf("JSON snippet must be an object or array at root level")
	}
}

// rootArrayItemKey is the key `item of` uses for a ROOT-level array, which has
// no JSON key of its own. "Root" is the exposed name Studio Pro gives every root
// element (9 of 9 pinned structures), and a root array has no keys at its own
// level, so there is nothing for it to collide with.
const rootArrayItemKey = "Root"

// snippetBuilder holds state for building the element tree from a JSON snippet.
type snippetBuilder struct {
	customNameMap map[string]string // JSON key → custom ExposedName
	itemNameMap   map[string]string // array's JSON key → its ITEM element's name
}

// itemName returns the name for the item element of the array reached by
// jsonKey, or fallback when the script did not name it.
func (b *snippetBuilder) itemName(jsonKey, fallback string) string {
	if b.itemNameMap != nil {
		if custom, ok := b.itemNameMap[jsonKey]; ok {
			return custom
		}
	}
	return fallback
}

// reservedExposedNames are element names Mendix will not accept as an
// ExposedName. Studio Pro avoids them by prefixing an underscore and keeping the
// key's ORIGINAL case — `id` becomes `_id`, `object` becomes `_object` (both
// measured in OpenAI_API.JSON_OpenAI_Response) — which is what
// resolveExposedName does.
//
// The list was only {Id, Type}, so an ordinary payload member like `owner` or
// `class` produced a structure mxbuild REFUSES TO BUILD:
//
//	CE9524 "JSON element '(Object)/owner' has an invalid custom name 'Owner'.
//	        Custom name should start with a letter or underscore followed by
//	        either letters, digits or underscores."
//
// The message is a red herring — 'Owner' matches that pattern perfectly. The
// real rule, measured against mxbuild 11.13 one name at a time, is a
// case-insensitive reserved set:
//
//	every Java keyword and literal (53, each measured and rejected)
//	the four system attribute names: Owner, ChangedDate, CreatedDate, ChangedBy
//	CurrentUser and Guid
//
// It is NOT the entity-member reserved list: `Type` and `Id` are accepted by
// mxbuild here, and are kept below only because Studio Pro avoids them too —
// this list is "what Studio Pro avoids", a superset of "what mxbuild rejects".
// `Object` is in the same category, added from the same reference document.
//
// Lookup is case-insensitive because Mendix's own check is: the member `class`
// derives `Class`, which is not literally a Java keyword, and is still rejected.
var reservedExposedNames = map[string]bool{
	// Studio Pro avoids these; mxbuild accepts them.
	"id": true, "type": true, "object": true,

	// Mendix system attribute names.
	"owner": true, "changeddate": true, "createddate": true, "changedby": true,

	// Mendix reserved.
	"currentuser": true, "guid": true,

	// Java keywords and literals.
	"abstract": true, "assert": true, "boolean": true, "break": true, "byte": true,
	"case": true, "catch": true, "char": true, "class": true, "const": true,
	"continue": true, "default": true, "do": true, "double": true, "else": true,
	"enum": true, "extends": true, "false": true, "final": true, "finally": true,
	"float": true, "for": true, "goto": true, "if": true, "implements": true,
	"import": true, "instanceof": true, "int": true, "interface": true, "long": true,
	"native": true, "new": true, "null": true, "package": true, "private": true,
	"protected": true, "public": true, "return": true, "short": true, "static": true,
	"strictfp": true, "super": true, "switch": true, "synchronized": true, "this": true,
	"throw": true, "throws": true, "transient": true, "true": true, "try": true,
	"void": true, "volatile": true, "while": true,
}

// resolveExposedName returns the custom name if mapped, otherwise capitalizes the
// JSON key. A name Mendix reserves is prefixed with an underscore, keeping the
// key's original case, which is what Studio Pro stores.
func (b *snippetBuilder) resolveExposedName(jsonKey string) string {
	if b.customNameMap != nil {
		if custom, ok := b.customNameMap[jsonKey]; ok {
			return custom
		}
	}
	key := sanitizeExposedName(jsonKey)
	name := capitalizeFirst(key)
	if reservedExposedNames[strings.ToLower(name)] {
		return "_" + key
	}
	return name
}

// sanitizeExposedName replaces every character Mendix will not accept in a
// custom name with an underscore.
//
// Mendix rejects anything else outright:
//
//	CE9524 "JSON element '(Object)/$type' has an invalid custom name '$type'.
//	        Custom name should start with a letter or underscore followed by
//	        either letters, digits or underscores."
//
// and here — unlike the reserved-name case in ako/mxcli#300 — the message is
// accurate. mxcli only capitalised, so an ordinary API member made the project
// unbuildable: `$type` occurs 43 times in the demo apps, alongside
// `research%3Aread` and `https%3A//sws.siemens.com/sam/claims/tenantId`.
//
// The rule is Studio Pro's, read off its own structures (Evora-FactoryManagement,
// CRS_ModelSimulator):
//
//	confidence(No)                -> Confidence_No_
//	Support Prediction            -> Support_Prediction
//	first(Importance)_Payload (kg)-> First_Importance__Payload__kg_
//	weights__base                 -> Weights__base
//
// A leading digit is prefixed rather than replaced: the name must START with a
// letter or underscore, and turning "1st" into "_st" would lose a character
// where "_1st" keeps it.
func sanitizeExposedName(jsonKey string) string {
	if jsonKey == "" {
		return jsonKey
	}
	out := make([]rune, 0, len(jsonKey)+1)
	for _, r := range jsonKey {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = append([]rune{'_'}, out...)
	}
	return string(out)
}

// nameTracker tracks used ExposedNames at each level to handle duplicates.
type nameTracker struct {
	seen map[string]int
}

func (t *nameTracker) uniqueName(base string) string {
	t.seen[base]++
	count := t.seen[base]
	if count == 1 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, count)
}

func (t *nameTracker) child() *nameTracker {
	return &nameTracker{seen: make(map[string]int)}
}

// capitalizeFirst capitalizes the first letter of a string for ExposedName.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// buildElementFromRawObject builds an Object element by decoding a raw JSON object string,
// preserving the original key order (Go's map[string]any loses order).
func (b *snippetBuilder) buildElementFromRawObject(exposedName, path, rawJSON string, tracker *nameTracker) *JsonElement {
	elem := &JsonElement{
		ExposedName:    exposedName,
		Path:           path,
		ElementType:    "Object",
		PrimitiveType:  "Unknown",
		MinOccurs:      0,
		MaxOccurs:      1,
		Nillable:       true,
		MaxLength:      -1,
		FractionDigits: -1,
		TotalDigits:    -1,
	}

	childTracker := tracker.child()

	// Decode with key order preserved
	dec := json.NewDecoder(strings.NewReader(rawJSON))
	if _, err := dec.Token(); err != nil { // opening {
		return elem
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		key, ok := tok.(string)
		if !ok {
			continue
		}
		// Capture the raw value to pass down for nested objects/arrays
		var rawVal json.RawMessage
		if err := dec.Decode(&rawVal); err != nil {
			break
		}

		childName := childTracker.uniqueName(b.resolveExposedName(key))
		childPath := path + "|" + key
		child := b.buildElementFromRawValue(childName, childPath, key, rawVal, childTracker)
		elem.Children = append(elem.Children, child)
	}

	return elem
}

// buildElementFromRawValue inspects a json.RawMessage to determine its type and build the element.
func (b *snippetBuilder) buildElementFromRawValue(exposedName, path, jsonKey string, raw json.RawMessage, tracker *nameTracker) *JsonElement {
	trimmed := strings.TrimSpace(string(raw))

	// Object — recurse with raw JSON to preserve key order
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return b.buildElementFromRawObject(exposedName, path, trimmed, tracker)
	}

	// Array
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return b.buildElementFromRawArray(exposedName, path, jsonKey, trimmed, tracker)
	}

	// Primitive — unmarshal to determine type
	var val any
	if err := json.Unmarshal(raw, &val); err != nil {
		return buildValueElement(exposedName, path, "Unknown", string(raw))
	}

	switch v := val.(type) {
	case string:
		primitiveType := "String"
		if iso8601Pattern.MatchString(v) {
			primitiveType = "DateTime"
			v = normalizeDateTimeValue(v)
		}
		return buildValueElement(exposedName, path, primitiveType, fmt.Sprintf("%q", v))
	case float64:
		// Check the raw JSON text for a decimal point — Go's %v drops ".0" from 41850.0
		if v == math.Trunc(v) && !strings.Contains(trimmed, ".") && v >= -(1<<53) && v <= (1<<53) {
			return buildValueElement(exposedName, path, "Integer", fmt.Sprintf("%v", int64(v)))
		}
		return buildValueElement(exposedName, path, "Decimal", fmt.Sprintf("%v", v))
	case bool:
		return buildValueElement(exposedName, path, "Boolean", fmt.Sprintf("%v", v))
	case nil:
		// JSON null → Unknown primitive type (matches Studio Pro)
		return buildValueElement(exposedName, path, "Unknown", "")
	default:
		return buildValueElement(exposedName, path, "String", "")
	}
}

// buildElementFromRawRootArray builds a root-level Array element.
// Studio Pro names the child object "JsonObject" (not "RootItem") for root arrays.
func (b *snippetBuilder) buildElementFromRawRootArray(exposedName, path, rawJSON string, tracker *nameTracker) *JsonElement {
	arrayElem := &JsonElement{
		ExposedName:    exposedName,
		Path:           path,
		ElementType:    "Array",
		PrimitiveType:  "Unknown",
		MinOccurs:      0,
		MaxOccurs:      1,
		Nillable:       true,
		MaxLength:      -1,
		FractionDigits: -1,
		TotalDigits:    -1,
	}

	dec := json.NewDecoder(strings.NewReader(rawJSON))
	if _, err := dec.Token(); err != nil { // opening [
		return arrayElem
	}
	if dec.More() {
		var firstItem json.RawMessage
		if err := dec.Decode(&firstItem); err != nil {
			return arrayElem
		}

		itemPath := path + "|(Object)"
		trimmed := strings.TrimSpace(string(firstItem))

		// A root array has no JSON key, so `item of 'Root'` addresses its item —
		// Root being the fixed exposed name of the root element.
		rootItemName := b.itemName(rootArrayItemKey, "JsonObject")

		if len(trimmed) > 0 && trimmed[0] == '{' {
			itemElem := b.buildElementFromRawObject(rootItemName, itemPath, trimmed, tracker)
			itemElem.MinOccurs = 0
			itemElem.MaxOccurs = occursUnbounded
			itemElem.Nillable = true
			arrayElem.Children = append(arrayElem.Children, itemElem)
		} else {
			child := b.buildElementFromRawValue(rootItemName, itemPath, "", firstItem, tracker)
			child.MinOccurs = 0
			child.MaxOccurs = occursUnbounded
			arrayElem.Children = append(arrayElem.Children, child)
		}
	}

	return arrayElem
}

// buildElementFromRawArray builds an Array element, using the first item's raw JSON for ordering.
// For primitive arrays (strings, numbers), Studio Pro creates a Wrapper element with a Value child.
func (b *snippetBuilder) buildElementFromRawArray(exposedName, path, jsonKey, rawJSON string, tracker *nameTracker) *JsonElement {
	arrayElem := &JsonElement{
		ExposedName:    exposedName,
		Path:           path,
		ElementType:    "Array",
		PrimitiveType:  "Unknown",
		MinOccurs:      0,
		MaxOccurs:      1,
		Nillable:       true,
		MaxLength:      -1,
		FractionDigits: -1,
		TotalDigits:    -1,
	}

	// Decode array and get first element as raw JSON
	dec := json.NewDecoder(strings.NewReader(rawJSON))
	if _, err := dec.Token(); err != nil { // opening [
		return arrayElem
	}
	if dec.More() {
		var firstItem json.RawMessage
		if err := dec.Decode(&firstItem); err != nil {
			return arrayElem
		}

		trimmed := strings.TrimSpace(string(firstItem))

		if len(trimmed) > 0 && trimmed[0] == '{' {
			// Object array: child is NameItem object unless the script named it
			itemName := b.itemName(jsonKey, exposedName+"Item")
			itemPath := path + "|(Object)"
			itemElem := b.buildElementFromRawObject(itemName, itemPath, trimmed, tracker)
			itemElem.MinOccurs = 0
			itemElem.MaxOccurs = occursUnbounded
			itemElem.Nillable = true
			arrayElem.Children = append(arrayElem.Children, itemElem)
		} else {
			// Primitive array: Studio Pro wraps in a Wrapper element with singular name
			// e.g., tags: ["a","b"] → Tag (Wrapper) → Value (String)
			// The Wrapper IS the item element here — one per array entry — so
			// `item of` names it too, rather than there being a second spelling
			// for the primitive case.
			wrapperName := b.itemName(jsonKey, singularize(exposedName))
			// The markers are "(Wrapper)" and "(Value)", not the object array's
			// "(Object)" — measured on Studio Pro's KrogerAPI.JSON_ProductList,
			// whose primitive array is
			//   …|categories|(Wrapper)        Wrapper
			//   …|categories|(Wrapper)|(Value) Value
			// mxcli wrote "…|(Object)" and a value path ending in a bare "|",
			// which no mapping could resolve against (#268).
			wrapperPath := path + "|(Wrapper)"
			wrapper := &JsonElement{
				ExposedName:    wrapperName,
				Path:           wrapperPath,
				ElementType:    "Wrapper",
				PrimitiveType:  "Unknown",
				MinOccurs:      0,
				MaxOccurs:      occursUnbounded,
				Nillable:       true,
				MaxLength:      -1,
				FractionDigits: -1,
				TotalDigits:    -1,
			}
			valueElem := b.buildElementFromRawValue("Value", wrapperPath+"|(Value)", jsonKey, firstItem, tracker)
			valueElem.MinOccurs = 0
			valueElem.MaxOccurs = 1
			wrapper.Children = append(wrapper.Children, valueElem)
			arrayElem.Children = append(arrayElem.Children, wrapper)
		}
	}

	return arrayElem
}

// SnippetKeys reports which JSON keys a snippet contains, and which of those
// reach an ARRAY — the two things a CUSTOM NAME MAP entry can address.
//
// It builds the same element tree the writer does rather than re-walking the
// JSON, so the answer cannot drift from what the names would actually be applied
// to. rootIsArray says whether `item of "Root"` is meaningful.
func SnippetKeys(snippet string) (keys, arrayKeys map[string]bool, rootIsArray bool, err error) {
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		return nil, nil, false, err
	}
	keys = map[string]bool{}
	arrayKeys = map[string]bool{}
	var walk func(e *JsonElement)
	walk = func(e *JsonElement) {
		parts := strings.Split(e.Path, "|")
		last := parts[len(parts)-1]
		if last != "" && last[0] != '(' {
			keys[last] = true
			if e.ElementType == "Array" {
				arrayKeys[last] = true
			}
		}
		for _, c := range e.Children {
			walk(c)
		}
	}
	for _, e := range elems {
		walk(e)
		if e.ElementType == "Array" {
			rootIsArray = true
			arrayKeys[rootArrayItemKey] = true
		}
	}
	return keys, arrayKeys, rootIsArray, nil
}

// SingularizeExposedName exposes the primitive-array wrapper's default name so
// DESCRIBE can tell a generated name from a chosen one.
func SingularizeExposedName(s string) string { return singularize(s) }

// rootArrayItemKeyExported lets other packages spell the root sentinel without
// duplicating the literal.
const RootArrayItemKey = rootArrayItemKey

// singularize returns a naive singular form by stripping trailing "s".
// Handles common cases: Tags→Tag, Items→Item. Known-incorrect for some words
// (e.g. Addresses→Addresse) — this matches Studio Pro's behavior.
func singularize(s string) string {
	if len(s) > 1 && strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}

func buildValueElement(exposedName, path, primitiveType, originalValue string) *JsonElement {
	maxLength := -1
	if primitiveType == "String" {
		maxLength = 0
	}
	return &JsonElement{
		ExposedName:    exposedName,
		Path:           path,
		ElementType:    "Value",
		PrimitiveType:  primitiveType,
		MinOccurs:      0,
		MaxOccurs:      1,
		Nillable:       true,
		MaxLength:      maxLength,
		FractionDigits: -1,
		TotalDigits:    -1,
		OriginalValue:  originalValue,
	}
}
