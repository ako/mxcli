// SPDX-License-Identifier: Apache-2.0

package canon

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// UndeclaredTypeError refuses a unit that holds an element whose $Type the
// project's Mendix version does not have yet: introduced in a later version, by
// the Model SDK's own version data (ako/mxcli#1774). Every element is checked,
// the unit's root and everything nested in it. Nil when there is none, and for
// a nil or zero version.
//
// # Why
//
// The property floor (StripUndeclaredProperties) compares property versions;
// the generated version data holds nothing else, so a type newer than the
// project went through it. Such a document does not load: mx check and Studio
// Pro die with `TypeCacheUnknownTypeException: The type cache does not contain
// a type with qualified name …`, naming no document. It shipped four times
// before this check existed: workflows below 11.3 (#1754), page shapes (#1209),
// the parallel split end marker, a 10.21 type written to 9.x and 10.0–10.20
// (#1163), and user tasks, 10.12 types written to 9.x (#1771).
//
// # Refused, not dropped
//
// Dropping the element would write a different model from the one the
// statement described, and the parent often requires it (an activity in a
// flow, an event on a task). Where an older version stores the same fact in an
// older shape, LowerToProjectShape writes that shape first — this runs after
// it, so it sees only what could not be lowered.
//
// # Deletions are not checked
//
// The data carries the SDK's deletion versions too, but they are not enforced.
// A type the SDK renamed keeps its storage name: the SDK deletes
// Workflows$CallMicroflowTask in 11.9 and introduces CallMicroflowActivity,
// which the codec stores as Workflows$CallMicroflowTask still. And a deleted
// type can still load: the SDK deletes Workflows$TimerBoundaryEvent in 10.20,
// and mxbuild 11.x loads a project holding one (only the runtime lacks the
// class, which is why MDL-WF07 exists). Some deletions are real
// (MessageDefinitions$MessageDefinitionCollection in 11.15, which mxcli refuses
// there on its own), but telling them apart takes a measurement per row. An
// introduction floor cannot misfire on a rename (the new name is simply never
// stored); a deletion floor can.
func UndeclaredTypeError(contents []byte, pv *version.Version) error {
	if pv == nil || pv.IsZero() || len(contents) == 0 {
		return nil
	}
	floors := typeFloors()
	if floors.newest.Compare(*pv) <= 0 {
		return nil // every type the data knows of is declared
	}
	var refused []string
	seen := map[string]bool{}
	walkTypes(bson.Raw(contents), func(typ string) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		if since, ok := floors.introduced[typ]; ok && pv.Compare(since) < 0 {
			refused = append(refused, fmt.Sprintf("%s (Mendix %s+)", typ, since))
		}
	})
	if len(refused) == 0 {
		return nil
	}
	sort.Strings(refused)
	return fmt.Errorf("this Mendix %s project cannot store %s: the type%s did not exist yet in this version, "+
		"and Studio Pro cannot open a document that holds one. Remove what needs it from the statement, or upgrade the project",
		pv, strings.Join(refused, "; "), map[bool]string{true: "", false: "s"}[len(refused) == 1])
}

// TypeIntroduced returns the version that introduced a storage $Type, by the
// Model SDK's version data. ok is false for a type the data has no
// introduction for: one Mendix had before the SDK's history starts, or one it
// does not know.
func TypeIntroduced(typeName string) (version.Version, bool) {
	v, ok := typeFloors().introduced[typeName]
	return v, ok
}

// TypeDeleted returns the version the Model SDK says deleted a storage $Type.
// It is data, not a floor: UndeclaredTypeError does not refuse on it (see
// there for why).
func TypeDeleted(typeName string) (version.Version, bool) {
	v, ok := typeFloors().deleted[typeName]
	return v, ok
}

// TypeDeclared reports whether a project of version pv has the storage $Type
// typeName, by the data UndeclaredTypeError refuses on. A type with no data is
// declared, as it is to the check.
func TypeDeclared(typeName string, pv version.Version) bool {
	since, ok := TypeIntroduced(typeName)
	return !ok || pv.Compare(since) >= 0
}

// walkTypes calls fn with the $Type of every element in raw, nested ones
// included.
func walkTypes(raw bson.Raw, fn func(string)) {
	elems, err := raw.Elements()
	if err != nil {
		return
	}
	for _, e := range elems {
		v := e.Value()
		switch v.Type {
		case bson.TypeString:
			if e.Key() == "$Type" {
				fn(v.StringValue())
			}
		case bson.TypeEmbeddedDocument:
			walkTypes(v.Document(), fn)
		case bson.TypeArray:
			walkTypes(bson.Raw(v.Array()), fn)
		}
	}
}

//go:embed mendixmodelsdk_type_versions.json
var typeVersionsJSON []byte

type typeFloorData struct {
	introduced map[string]version.Version
	deleted    map[string]version.Version
	// newest is the latest introduction in the data: a project at or past it
	// has every type, and the walk is skipped.
	newest version.Version
}

var (
	typeFloorsOnce sync.Once
	typeFloorsData typeFloorData
)

func typeFloors() typeFloorData {
	typeFloorsOnce.Do(func() {
		d, err := parseTypeVersions(typeVersionsJSON)
		if err != nil {
			// The embedded file is checked by TestTypeVersionsParse; a broken
			// one must not take every write down with it.
			d = typeFloorData{}
		}
		typeFloorsData = d
	})
	return typeFloorsData
}

// typeVersionsFile is mendixmodelsdk_type_versions.json, generated by
// gen_type_versions.go from the Model SDK.
type typeVersionsFile struct {
	Source string `json:"source"`
	Types  []struct {
		Type       string   `json:"type"`
		Introduced string   `json:"introduced"`
		Deleted    string   `json:"deleted"`
		SDK        []string `json:"sdk"`
	} `json:"types"`
}

func parseTypeVersions(data []byte) (typeFloorData, error) {
	var f typeVersionsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return typeFloorData{}, fmt.Errorf("type versions: %w", err)
	}
	if !strings.HasPrefix(f.Source, "mendixmodelsdk@") || len(f.Types) == 0 {
		return typeFloorData{}, fmt.Errorf("type versions: no source or no types")
	}
	d := typeFloorData{introduced: map[string]version.Version{}, deleted: map[string]version.Version{}}
	seen := make(map[string]bool, len(f.Types))
	for _, t := range f.Types {
		if !strings.Contains(t.Type, "$") {
			return typeFloorData{}, fmt.Errorf("type versions: %q is not a $Type", t.Type)
		}
		if seen[t.Type] {
			return typeFloorData{}, fmt.Errorf("type versions: %s listed twice", t.Type)
		}
		seen[t.Type] = true
		if t.Deleted != "" {
			v := version.Parse(t.Deleted)
			if v.IsZero() {
				return typeFloorData{}, fmt.Errorf("type versions: %s deleted %q", t.Type, t.Deleted)
			}
			d.deleted[t.Type] = v
		}
		if t.Introduced == "" {
			continue
		}
		v := version.Parse(t.Introduced)
		if v.IsZero() {
			return typeFloorData{}, fmt.Errorf("type versions: %s introduced %q", t.Type, t.Introduced)
		}
		d.introduced[t.Type] = v
		if d.newest.Compare(v) < 0 {
			d.newest = v
		}
	}
	return d, nil
}
