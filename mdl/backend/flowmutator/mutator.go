// SPDX-License-Identifier: Apache-2.0

// Package flowmutator toggles the Disabled flag on the activities of a stored
// microflow, nanoflow or rule, by walking the unit's raw BSON tree.
//
// Raw BSON rather than a model round trip, for the reason ADR-0005 gives:
// `create or modify microflow` rebuilds the whole document from MDL and is only
// as faithful as what MDL can spell, and the flows someone reaches into to
// disable a step are exactly the ones holding constructs it cannot — a queued
// call, a web-service call, an action mxcli does not model. Setting one boolean
// on the stored document leaves every other byte alone, so nothing has to be
// reproducible for the statement to be safe.
//
// The walk is over the whole tree rather than a maintained list of containers.
// A loop's body lives in a nested ObjectCollection, an error handler's in
// another, and Mendix adds container shapes between versions; a walk that
// descends into every document and array cannot miss one, and the only nodes it
// acts on are those whose $Type is Microflows$ActionActivity.
package flowmutator

import (
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// disabledKey is the property name on Microflows$ActionActivity. Introduced in
// Mendix 9.12.0 — see the microflows/disabled_activity version gate.
const disabledKey = "Disabled"

// actionActivityType is the only microflow object that carries the flag.
// `Disabled` occurs exactly once in generated/metamodel, on this type.
const actionActivityType = "Microflows$ActionActivity"

// Match reports whether one activity satisfies the filter. Separated from the
// walk so the matching rules can be tested against hand-built documents without
// a project.
type Match func(activity bson.D) bool

// Apply sets Disabled to want on every ActionActivity in doc that matches, and
// returns how many it CHANGED — not how many it matched.
//
// The difference is the whole report: `disable … where action = log` run twice
// says 3 the first time and 0 the second, which is what tells the reader the
// second run did nothing. Counting matches would say 3 both times and the
// statement would look like it kept working.
func Apply(doc bson.D, match Match, want bool) (changed int) {
	walk(doc, func(activity bson.D) bson.D {
		if !match(activity) {
			return activity
		}
		out, did := setDisabled(activity, want)
		if did {
			changed++
		}
		return out
	})
	return changed
}

// Count reports how many activities match, changing nothing. Used to tell "no
// activity matched" from "every match was already in that state" — two outcomes
// that both write nothing and mean very different things to the reader.
func Count(doc bson.D, match Match) int {
	n := 0
	walk(doc, func(activity bson.D) bson.D {
		if match(activity) {
			n++
		}
		return activity
	})
	return n
}

// walk descends the whole tree, calling visit on every Microflows$ActionActivity
// document and substituting what it returns.
//
// bson.D is a slice of elements, so an in-place edit of a nested document only
// sticks when the parent's element value is reassigned — hence visit returning
// the document rather than mutating it.
func walk(d bson.D, visit func(bson.D) bson.D) {
	for i := range d {
		d[i].Value = walkValue(d[i].Value, visit)
	}
}

func walkValue(v any, visit func(bson.D) bson.D) any {
	switch t := v.(type) {
	case bson.D:
		if typeOf(t) == actionActivityType {
			t = visit(t)
		}
		walk(t, visit)
		return t
	case bson.A:
		for i := range t {
			t[i] = walkValue(t[i], visit)
		}
		return t
	case []any:
		for i := range t {
			t[i] = walkValue(t[i], visit)
		}
		return t
	default:
		return v
	}
}

// setDisabled writes the flag, and reports whether the document changed.
//
// The key is only ever REPLACED, never added: every ActionActivity mxcli or
// Studio Pro writes on a supported version already carries it, and adding a key
// to a document that lacks one is how a project becomes unopenable when the
// property does not exist at that Mendix version (CLAUDE.md's overlay-writes
// rule — write only keys the document already carries). A document without the
// key is left alone and reported by the caller rather than patched.
func setDisabled(activity bson.D, want bool) (bson.D, bool) {
	for i := range activity {
		if activity[i].Key != disabledKey {
			continue
		}
		if cur, ok := activity[i].Value.(bool); ok && cur == want {
			return activity, false
		}
		activity[i].Value = want
		return activity, true
	}
	return activity, false
}

// HasDisabledKey reports whether an activity document carries the property at
// all. False means the project predates 9.12 (or the document was written by
// something that omitted it), and the caller says so instead of adding it.
func HasDisabledKey(activity bson.D) bool {
	for _, e := range activity {
		if e.Key == disabledKey {
			return true
		}
	}
	return false
}

// NewMatch builds the Match for a filter. knownTypes is the set of decodable
// `$Type` strings, used to resolve an action named by its storage label.
//
// A filter whose conditions cannot be resolved is NOT silently permissive: the
// caller validates with types.CheckActivityFilter first and refuses, so by the
// time a Match is built every action word has resolved.
func NewMatch(f types.ActivityFilter, knownTypes map[string]bool) Match {
	return func(activity bson.D) bool {
		for _, c := range f.Conditions {
			if !matchCondition(activity, c, knownTypes) {
				return false
			}
		}
		return true
	}
}

func matchCondition(activity bson.D, c types.ActivityCondition, knownTypes map[string]bool) bool {
	switch strings.ToLower(c.Column) {
	case types.ActivityColAction:
		got := typeOf(childDoc(activity, "Action"))
		return matchSet(got, c, func(want string) (string, bool) {
			return types.ResolveActivityAction(want, knownTypes)
		})
	case types.ActivityColLevel:
		// `level` is a property of the log action. On any other activity the
		// action carries no Level, so the condition is false — which is why a
		// filter pairing `level` with a different `action` is refused at check
		// time rather than run to an empty result.
		got := stringField(childDoc(activity, "Action"), "Level")
		return matchSet(got, c, identityFold)
	case types.ActivityColCaption:
		got := stringField(activity, "Caption")
		if c.Like {
			return likeMatch(got, c.Values) != c.Negate
		}
		return matchSet(got, c, identityFold)
	case types.ActivityColDisabled:
		got := "false"
		if boolField(activity, disabledKey) {
			got = "true"
		}
		return matchSet(got, c, identityFold)
	}
	return false
}

func identityFold(want string) (string, bool) { return want, true }

// matchSet applies one condition's values to a single field, honouring IN and
// negation. normalise maps the written value into the stored currency (for
// `action`, a word into a $Type); a value it cannot map matches nothing.
func matchSet(got string, c types.ActivityCondition, normalise func(string) (string, bool)) bool {
	hit := false
	for _, v := range c.Values {
		want, ok := normalise(v)
		if !ok {
			continue
		}
		if strings.EqualFold(got, want) {
			hit = true
			break
		}
	}
	return hit != c.Negate
}

// likeMatch is SQL LIKE with `%` (any run) and `_` (one character), matched
// case-insensitively because captions are prose.
func likeMatch(got string, patterns []string) bool {
	for _, p := range patterns {
		if likeOne(strings.ToLower(got), strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func likeOne(s, pattern string) bool {
	// Iterative two-pointer glob, so a pathological pattern cannot blow the
	// stack the way naive recursion does.
	si, pi := 0, 0
	star, match := -1, 0
	for si < len(s) {
		switch {
		case pi < len(pattern) && (pattern[pi] == '_' || pattern[pi] == s[si]):
			si++
			pi++
		case pi < len(pattern) && pattern[pi] == '%':
			star = pi
			match = si
			pi++
		case star >= 0:
			pi = star + 1
			match++
			si = match
		default:
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '%' {
		pi++
	}
	return pi == len(pattern)
}

// --- small BSON readers -----------------------------------------------------

func typeOf(d bson.D) string { return stringField(d, "$Type") }

func stringField(d bson.D, key string) string {
	for _, e := range d {
		if e.Key == key {
			if s, ok := e.Value.(string); ok {
				return s
			}
			return ""
		}
	}
	return ""
}

func boolField(d bson.D, key string) bool {
	for _, e := range d {
		if e.Key == key {
			b, _ := e.Value.(bool)
			return b
		}
	}
	return false
}

func childDoc(d bson.D, key string) bson.D {
	for _, e := range d {
		if e.Key == key {
			if c, ok := e.Value.(bson.D); ok {
				return c
			}
			return nil
		}
	}
	return nil
}

// EachActivity calls fn for every ActionActivity document in the tree, in
// document order. The caller uses it to report which activities a statement
// touched, and to detect a pre-9.12 document with no Disabled key.
func EachActivity(doc bson.D, fn func(bson.D)) {
	walk(doc, func(a bson.D) bson.D {
		fn(a)
		return a
	})
}
