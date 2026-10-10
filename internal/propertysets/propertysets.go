// SPDX-License-Identifier: Apache-2.0

// Package propertysets measures which properties mxcli leaves out of the
// elements it writes, using Mendix itself as the reference.
//
// The method (mendixlabs/mxcli#1373): write content with mxcli, copy the
// project, run `mx convert -p` on the copy, and compare the two trees element by
// element. `mx convert` re-serializes exactly the units that are not canonical
// for its version — on a project nothing else wrote, it changes none — so every
// property it adds to an element is one Studio Pro always writes and mxcli did
// not, and the value it adds is the one Mendix gives the absent property.
//
// Mendix's merge engine compares an element's property NAMES between two
// revisions and throws when they differ, so each such gap makes the document
// unmergeable the first time Studio Pro saves it. The measured values are the
// table canon.CompletePropertySets fills them from
// (modelsdk/canon/studiopro_property_defaults.json).
package propertysets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// Gap is one (type, key) Mendix added to elements mxcli wrote.
type Gap struct {
	Type  string
	Key   string
	Count int
	// Values holds every distinct value Mendix wrote, as canonical extended
	// JSON of {"v": value} with element $IDs blanked, so two default
	// sub-elements that differ only by identity compare equal.
	Values []string
	// Example is one unit the gap was seen in, for a failure message.
	Example string
}

// Extra is one (type, key) mxcli wrote that Mendix removed.
type Extra struct {
	Type    string
	Key     string
	Count   int
	Example string
}

// Result is one measurement.
type Result struct {
	UnitsChanged int
	Gaps         []Gap
	Extras       []Extra
	// Declared holds, per $Type, every key Mendix wrote on the converted side
	// of a compared element: what this version declares, as far as the
	// measured content exercised the type. Table uses it to find the version a
	// key stopped existing in.
	Declared map[string]map[string]bool
}

// Measure compares the mprcontents trees written (what mxcli wrote) and
// converted (the same after `mx convert -p`). Only elements present in both
// with the same $ID and $Type are compared.
func Measure(written, converted string) (*Result, error) {
	return MeasureUnits(written, converted, nil)
}

// Snapshot fingerprints every unit file under an mprcontents tree, keyed by its
// path relative to it. Taken before mxcli writes, it lets MeasureUnits judge
// only what mxcli wrote: a source project that is not canonical for the
// installed mx would otherwise be measured as mxcli's gaps.
func Snapshot(dir string) (map[string][32]byte, error) {
	out := map[string][32]byte{}
	err := filepath.WalkDir(dir, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() || !strings.HasSuffix(p, ".mxunit") {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = sha256.Sum256(b)
		return nil
	})
	return out, err
}

// MeasureUnits is Measure restricted to the units that differ from before (a
// Snapshot of the tree before mxcli wrote); a nil before measures every unit.
func MeasureUnits(written, converted string, before map[string][32]byte) (*Result, error) {
	gaps := map[string]*Gap{}
	gapValues := map[string]map[string]bool{}
	extras := map[string]*Extra{}
	res := &Result{Declared: map[string]map[string]bool{}}
	err := filepath.WalkDir(written, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() || !strings.HasSuffix(p, ".mxunit") {
			return nil
		}
		rel, err := filepath.Rel(written, p)
		if err != nil {
			return err
		}
		a, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if before != nil {
			if h, ok := before[rel]; ok && h == sha256.Sum256(a) {
				return nil // not written by mxcli
			}
		}
		b, err := os.ReadFile(filepath.Join(converted, rel))
		if err != nil || bytes.Equal(a, b) {
			return nil // removed by convert, or untouched
		}
		res.UnitsChanged++
		ea, err := elements(a)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		eb, err := elements(b)
		if err != nil {
			return fmt.Errorf("%s (converted): %w", rel, err)
		}
		for id, x := range ea {
			y, ok := eb[id]
			if !ok || x.typ != y.typ {
				continue
			}
			if res.Declared[x.typ] == nil {
				res.Declared[x.typ] = map[string]bool{}
			}
			for k := range y.props {
				res.Declared[x.typ][k] = true
			}
			for k, v := range y.props {
				if _, ok := x.props[k]; ok {
					continue
				}
				key := x.typ + "." + k
				g := gaps[key]
				if g == nil {
					g = &Gap{Type: x.typ, Key: k, Example: rel}
					gaps[key] = g
					gapValues[key] = map[string]bool{}
				}
				g.Count++
				ej, err := bson.MarshalExtJSON(bson.D{{Key: "v", Value: blankIDs(v)}}, true, false)
				if err != nil {
					return fmt.Errorf("%s: %s: %w", rel, key, err)
				}
				gapValues[key][string(ej)] = true
			}
			for k := range x.props {
				if _, ok := y.props[k]; ok {
					continue
				}
				key := x.typ + "." + k
				e := extras[key]
				if e == nil {
					e = &Extra{Type: x.typ, Key: k, Example: rel}
					extras[key] = e
				}
				e.Count++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for key, g := range gaps {
		for v := range gapValues[key] {
			g.Values = append(g.Values, v)
		}
		sort.Strings(g.Values)
		res.Gaps = append(res.Gaps, *g)
	}
	sort.Slice(res.Gaps, func(i, j int) bool {
		return res.Gaps[i].Type+"."+res.Gaps[i].Key < res.Gaps[j].Type+"."+res.Gaps[j].Key
	})
	for _, e := range extras {
		res.Extras = append(res.Extras, *e)
	}
	sort.Slice(res.Extras, func(i, j int) bool {
		return res.Extras[i].Type+"."+res.Extras[i].Key < res.Extras[j].Type+"."+res.Extras[j].Key
	})
	return res, nil
}

// Entry is one row of the defaults table, in its on-disk form.
type Entry struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Since string `json:"since"`
	// Until, when set, is the first version the key must NOT be written to
	// (exclusive).
	Until string `json:"until,omitempty"`
	// Seen is the newest version Mendix was measured writing the key on. It is
	// bookkeeping for the next Merge — which cannot see earlier measurements —
	// and is not read when writing.
	Seen  string          `json:"seen,omitempty"`
	Value json.RawMessage `json:"value"`
}

// Table builds the defaults table from measurements taken on several Mendix
// versions. It is Merge into an empty table.
func Table(byVersion map[string]*Result) (entries []Entry, varying []string) {
	return Merge(nil, byVersion)
}

// Merge folds measurements taken on several Mendix versions into an existing
// defaults table (nil for a new one).
//
// A gap measured on a version older than its entry's Since lowers Since; a new
// gap becomes an entry whose Since is the oldest version it was measured on. A
// gap whose value differs — between elements, between versions, or from the
// entry already in the table — is returned in varying and changes nothing:
// Mendix derived it from context, so no constant is right.
//
// An entry gets an Until when a NEWER measured version exercised the type and
// Mendix did not write the key there: the property was renamed or removed. The
// bound is the minor version after the last one the key was seen on, the
// conservative end of the unknown range — a version in between that no longer
// declares it would receive a key it cannot open, while one that still does
// merely keeps the gap it had before.
func Merge(existing []Entry, byVersion map[string]*Result) (entries []Entry, varying []string) {
	type acc struct {
		entry    Entry
		since    version.Version
		lastSeen version.Version
		values   map[string]bool
	}
	vers := make([]version.Version, 0, len(byVersion))
	results := map[version.Version]*Result{}
	for ver, res := range byVersion {
		v := version.Parse(ver)
		vers = append(vers, v)
		results[v] = res
	}
	sort.Slice(vers, func(i, j int) bool { return vers[i].Compare(vers[j]) < 0 })

	all := map[string]*acc{}
	for _, e := range existing {
		since := version.Parse(e.Since)
		seen := since
		if s := version.Parse(e.Seen); s.Compare(seen) > 0 {
			seen = s
		}
		all[e.Type+"\x00"+e.Key] = &acc{entry: e, since: since, lastSeen: seen,
			values: map[string]bool{compactJSON(e.Value): true}}
	}
	vary := map[string]bool{}
	for _, v := range vers {
		for _, g := range results[v].Gaps {
			key := g.Type + "\x00" + g.Key
			a := all[key]
			if a == nil {
				a = &acc{entry: Entry{Type: g.Type, Key: g.Key}, since: v, lastSeen: v, values: map[string]bool{}}
				all[key] = a
			}
			for _, val := range g.Values {
				a.values[compactJSON(json.RawMessage(val))] = true
			}
			if v.Compare(a.since) < 0 {
				a.since = v
			}
			if v.Compare(a.lastSeen) > 0 {
				a.lastSeen = v
			}
			if len(a.values) != 1 {
				vary[g.Type+"."+g.Key] = true
			}
		}
		for typ, keys := range results[v].Declared {
			for k := range keys {
				if a := all[typ+"\x00"+k]; a != nil && v.Compare(a.lastSeen) > 0 {
					a.lastSeen = v
				}
			}
		}
	}
	for key, a := range all {
		typ, k, _ := strings.Cut(key, "\x00")
		if vary[typ+"."+k] {
			varying = append(varying, typ+"."+k)
			if a.entry.Value == nil {
				continue // never in the table; stays out
			}
			entries = append(entries, a.entry) // keep what was there
			continue
		}
		e := a.entry
		e.Since = a.since.String()
		e.Seen = a.lastSeen.String()
		if e.Value == nil {
			for val := range a.values {
				e.Value = json.RawMessage(val)
			}
		}
		if e.Until != "" && a.lastSeen.Compare(version.Parse(e.Until)) >= 0 {
			// Seen again at or past the bound. The evidence that set it — a
			// newer version without the key — is not in the table, so the bound
			// moves just past the new sighting rather than disappearing.
			e.Until = version.Version{Major: a.lastSeen.Major, Minor: a.lastSeen.Minor + 1}.String()
		}
		if e.Until == "" {
			for _, v := range vers {
				d, exercised := results[v].Declared[typ]
				if exercised && !d[k] && v.Compare(a.lastSeen) > 0 {
					e.Until = version.Version{Major: a.lastSeen.Major, Minor: a.lastSeen.Minor + 1}.String()
					break
				}
			}
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Type+"."+entries[i].Key < entries[j].Type+"."+entries[j].Key
	})
	sort.Strings(varying)
	return entries, varying
}

// compactJSON normalises a table value for comparison.
func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// MarshalTable renders the table as the committed JSON file: one entry per line,
// so a regeneration diffs by entry.
func MarshalTable(entries []Entry) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("[\n")
	for i, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		buf.WriteString("  ")
		buf.Write(line)
		if i < len(entries)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("]\n")
	return buf.Bytes(), nil
}

type elem struct {
	typ   string
	props map[string]any
}

// elements indexes every element in a unit by its binary $ID.
func elements(raw []byte) (map[string]elem, error) {
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	out := map[string]elem{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			e := elem{props: map[string]any{}}
			id := ""
			for _, kv := range x {
				e.props[kv.Key] = kv.Value
				switch kv.Key {
				case "$ID":
					if b, ok := kv.Value.(bson.Binary); ok {
						id = hex.EncodeToString(b.Data)
					}
				case "$Type":
					e.typ, _ = kv.Value.(string)
				}
				walk(kv.Value)
			}
			if id != "" {
				out[id] = e
			}
		case bson.A:
			for _, it := range x {
				walk(it)
			}
		}
	}
	walk(d)
	return out, nil
}

// blankIDs copies v with every $ID replaced by sixteen zero bytes: the table
// stores a template, and CompletePropertySets mints fresh ids on use.
func blankIDs(v any) any {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, len(x))
		for i, e := range x {
			if e.Key == "$ID" {
				out[i] = bson.E{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}}
				continue
			}
			out[i] = bson.E{Key: e.Key, Value: blankIDs(e.Value)}
		}
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = blankIDs(e)
		}
		return out
	}
	return v
}

// Combine merges several measurements of the same Mendix version (one per
// script, in the gate) into one.
func Combine(rs []*Result) *Result {
	out := &Result{Declared: map[string]map[string]bool{}}
	gaps := map[string]*Gap{}
	gapValues := map[string]map[string]bool{}
	extras := map[string]*Extra{}
	for _, r := range rs {
		out.UnitsChanged += r.UnitsChanged
		for _, g := range r.Gaps {
			key := g.Type + "." + g.Key
			if gaps[key] == nil {
				c := g
				c.Count, c.Values = 0, nil
				gaps[key] = &c
				gapValues[key] = map[string]bool{}
			}
			gaps[key].Count += g.Count
			for _, v := range g.Values {
				gapValues[key][v] = true
			}
		}
		for _, e := range r.Extras {
			key := e.Type + "." + e.Key
			if extras[key] == nil {
				c := e
				c.Count = 0
				extras[key] = &c
			}
			extras[key].Count += e.Count
		}
		for typ, keys := range r.Declared {
			if out.Declared[typ] == nil {
				out.Declared[typ] = map[string]bool{}
			}
			for k := range keys {
				out.Declared[typ][k] = true
			}
		}
	}
	for key, g := range gaps {
		for v := range gapValues[key] {
			g.Values = append(g.Values, v)
		}
		sort.Strings(g.Values)
		out.Gaps = append(out.Gaps, *g)
	}
	sort.Slice(out.Gaps, func(i, j int) bool {
		return out.Gaps[i].Type+"."+out.Gaps[i].Key < out.Gaps[j].Type+"."+out.Gaps[j].Key
	})
	for _, e := range extras {
		out.Extras = append(out.Extras, *e)
	}
	sort.Slice(out.Extras, func(i, j int) bool {
		return out.Extras[i].Type+"."+out.Extras[i].Key < out.Extras[j].Type+"."+out.Extras[j].Key
	})
	return out
}

// Recording is one line of a record file: a measurement and the Mendix
// version it was taken on.
type Recording struct {
	Version string  `json:"version"`
	Result  *Result `json:"result"`
}
