// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// PropertySetCensus counts, across a whole project, which property names the
// elements of each $Type carry, and reports the names some elements of a type
// carry and others do not (ako/mxcli#1505).
//
// # Why
//
// The property-set repair fills only what the measured table
// (canon.CompletePropertySets) holds for the project's Mendix version. A key
// the table has no value for on that version — a version nobody measured, or a
// writer gap no measurement exercised — is invisible to it, so on Mendix
// 10.24.18 `fix property-sets` reported every unit complete while `mx diff`
// threw on a nanoflow without ExportLevel.
//
// The project is its own reference for that question, if not for the value:
// Studio Pro serializes the full property set of an element's type on every
// element of it (on the Studio Pro-authored fixtures pedapp 11.13.0,
// expr-checker 11.6.6, testapp-views 11.14.0 and a blank 10.24.18 app, every
// one of 115–251 types is uniform). So an element of a type without a name the
// others carry — or with one they lack — is a pair of name sets Mendix's merge
// engine refuses to compare.
//
// # What it does not do
//
// It reports; it never fills. Which side is right, and what value Mendix gives
// an absent key on this version, is a measurement (`mx convert` on that
// version), and a value taken from another version or from another element is
// a guess that changes the model when it is wrong. Count the units as the
// repair leaves them, so what the table fills is not reported.
type PropertySetCensus struct {
	types map[string]*typeCensus
}

type typeCensus struct {
	elements int
	sets     map[string]*keySetCensus // by the sorted, NUL-joined key set
}

type keySetCensus struct {
	keys     map[string]bool
	elements int
	units    []PropertySetRepair // the first few units holding such an element
}

// censusExamples bounds the units remembered per key set.
const censusExamples = 3

// NewPropertySetCensus returns an empty census.
func NewPropertySetCensus() *PropertySetCensus {
	return &PropertySetCensus{types: map[string]*typeCensus{}}
}

// Count adds every element of one unit, as the repair (StoredPropertySetGaps or
// CompleteStoredPropertySets) left it. A unit that could not be normalized adds
// nothing; the repair reports it.
func (c *PropertySetCensus) Count(rep PropertySetRepair) {
	if rep.normalized == nil {
		return
	}
	var doc bson.D
	if err := bson.Unmarshal(rep.normalized, &doc); err != nil {
		return
	}
	label := rep
	label.normalized = nil
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			typ := ""
			for _, e := range x {
				if e.Key == "$Type" {
					typ, _ = e.Value.(string)
				}
				walk(e.Value)
			}
			if typ != "" {
				c.add(typ, x, label)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
}

func (c *PropertySetCensus) add(typ string, d bson.D, unit PropertySetRepair) {
	names := make([]string, 0, len(d))
	for _, e := range d {
		names = append(names, e.Key)
	}
	sort.Strings(names)
	sig := strings.Join(names, "\x00")

	tc := c.types[typ]
	if tc == nil {
		tc = &typeCensus{sets: map[string]*keySetCensus{}}
		c.types[typ] = tc
	}
	tc.elements++
	ks := tc.sets[sig]
	if ks == nil {
		ks = &keySetCensus{keys: make(map[string]bool, len(names))}
		for _, n := range names {
			ks.keys[n] = true
		}
		tc.sets[sig] = ks
	}
	ks.elements++
	if len(ks.units) < censusExamples {
		for _, u := range ks.units {
			if u.UnitID == unit.UnitID {
				return
			}
		}
		ks.units = append(ks.units, unit)
	}
}

// MixedPropertyName is one property name that some elements of a $Type carry
// and others do not.
type MixedPropertyName struct {
	Type, Key string
	Elements  int // elements of Type in the project
	With      int // of which carry Key
	// Examples are up to three units holding an element on the smaller side:
	// without the key when most elements carry it, with it otherwise.
	Examples []PropertySetRepair
}

// Without reports whether Examples are units lacking the key (most elements
// carry it) rather than units holding it.
func (m MixedPropertyName) Without() bool { return m.With*2 >= m.Elements }

// Mixed returns every property name the elements of one type disagree on,
// sorted by type and key.
func (c *PropertySetCensus) Mixed() []MixedPropertyName {
	var out []MixedPropertyName
	for typ, tc := range c.types {
		if len(tc.sets) < 2 {
			continue
		}
		with := map[string]int{}
		for _, ks := range tc.sets {
			for k := range ks.keys {
				with[k] += ks.elements
			}
		}
		for k, n := range with {
			if n == tc.elements {
				continue
			}
			m := MixedPropertyName{Type: typ, Key: k, Elements: tc.elements, With: n}
			wantKey := !m.Without()
			for _, ks := range sortedSets(tc.sets) {
				if ks.keys[k] != wantKey {
					continue
				}
				for _, u := range ks.units {
					if len(m.Examples) < censusExamples && !hasUnit(m.Examples, u.UnitID) {
						m.Examples = append(m.Examples, u)
					}
				}
			}
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// sortedSets orders a type's key sets by signature, so the examples a report
// names do not depend on map order.
func sortedSets(sets map[string]*keySetCensus) []*keySetCensus {
	sigs := make([]string, 0, len(sets))
	for s := range sets {
		sigs = append(sigs, s)
	}
	sort.Strings(sigs)
	out := make([]*keySetCensus, len(sigs))
	for i, s := range sigs {
		out[i] = sets[s]
	}
	return out
}

func hasUnit(us []PropertySetRepair, id string) bool {
	for _, u := range us {
		if u.UnitID == id {
			return true
		}
	}
	return false
}
