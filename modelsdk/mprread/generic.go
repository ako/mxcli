// SPDX-License-Identifier: Apache-2.0

package mprread

import (
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// Unit pairs a decoded gen-typed element with its SQLite ContainerID.
// ContainerID is a DB-level column that is not embedded in the BSON content,
// so callers that need it (e.g. building module-level trees) can obtain it
// here without a second query.
type Unit[T element.Element] struct {
	Element     T
	ContainerID model.ID
}

// bsonTypeName returns the canonical BSON $Type name for the type parameter T
// by looking up the Go concrete type in the codec DefaultRegistry reverse map.
// It panics at startup if T is not registered, which catches configuration
// errors at init time rather than silently returning empty results at runtime.
func bsonTypeName[T element.Element]() string {
	var zero T
	goType := reflect.TypeOf(&zero).Elem()
	name, ok := codec.DefaultRegistry.TypeNameOf(goType)
	if !ok {
		panic(fmt.Sprintf("mprread: type %v not registered in codec.DefaultRegistry — import the gen package", goType))
	}
	return name
}

// ListUnitsByType decodes every unit of type T and returns them as []T.
// The BSON $Type name is inferred from T via the codec registry — no magic
// string needed. Import the relevant gen/* package to trigger registration.
//
// ContainerID is not included; use ListUnitsWithContainer if you need it.
func ListUnitsByType[T element.Element](r *mmpr.Reader) ([]T, error) {
	units, err := ListUnitsWithContainer[T](r)
	if err != nil {
		return nil, err
	}
	out := make([]T, len(units))
	for i, u := range units {
		out[i] = u.Element
	}
	return out, nil
}

// ListUnitsWithContainer decodes every unit of type T and returns each element
// paired with its ContainerID. Uses ref.Contents from the unit index (already
// loaded by ListUnitsByType on the Reader) to avoid a second per-unit read.
// The BSON $Type name is inferred from T via the codec registry.
func ListUnitsWithContainer[T element.Element](r *mmpr.Reader) ([]Unit[T], error) {
	return ListUnitsWithContainerWhere[T](r, nil)
}

// ListUnitsWithContainerWhere is ListUnitsWithContainer for the units whose
// stored document keep accepts (nil keeps all). keep sees the raw bytes, so a
// lookup by name or ID decodes the one document it wants instead of every
// document of the type — the decode was most of a flow lookup's cost, and
// check makes several per statement (mendixlabs/mxcli#1272).
func ListUnitsWithContainerWhere[T element.Element](r *mmpr.Reader, keep func(bson.Raw) bool) ([]Unit[T], error) {
	typeName := bsonTypeName[T]()
	refs, err := r.ListUnitsByType(typeName)
	if err != nil {
		return nil, err
	}
	dec := codec.NewDecoder(codec.DefaultRegistry)
	out := make([]Unit[T], 0, len(refs))
	for _, ref := range refs {
		if ref.Type != typeName {
			continue
		}
		if keep != nil && !keep(bson.Raw(ref.Contents)) {
			continue
		}
		elem, err := dec.Decode(bson.Raw(ref.Contents))
		if err != nil {
			return nil, fmt.Errorf("decode unit %s: %w", ref.ID, err)
		}
		typed, ok := elem.(T)
		if !ok {
			return nil, fmt.Errorf("unit %s decoded as %T, want %T", ref.ID, elem, *new(T))
		}
		out = append(out, Unit[T]{Element: typed, ContainerID: model.ID(ref.ContainerID)})
	}
	return out, nil
}

// UnitHeader is what a name listing needs of a unit, read without decoding it.
type UnitHeader struct {
	ID          model.ID
	ContainerID model.ID
	Name        string
	Excluded    bool
}

// ListUnitHeaders lists the ID, container, Name and Excluded of every unit of
// type T without decoding any of them: for callers that build a name set or an
// ID-to-name map and would otherwise decode the whole flow of every document.
func ListUnitHeaders[T element.Element](r *mmpr.Reader) ([]UnitHeader, error) {
	typeName := bsonTypeName[T]()
	refs, err := r.ListUnitsByType(typeName)
	if err != nil {
		return nil, err
	}
	out := make([]UnitHeader, 0, len(refs))
	for _, ref := range refs {
		if ref.Type != typeName {
			continue
		}
		raw := bson.Raw(ref.Contents)
		if err := raw.Validate(); err != nil {
			return nil, fmt.Errorf("decode unit %s: %w", ref.ID, err)
		}
		name, _ := raw.Lookup("Name").StringValueOK()
		excluded, _ := raw.Lookup("Excluded").BooleanOK()
		out = append(out, UnitHeader{
			ID:          model.ID(codec.PeekID(raw)),
			ContainerID: model.ID(ref.ContainerID),
			Name:        name,
			Excluded:    excluded,
		})
	}
	return out, nil
}

// HasName keeps a document whose top-level Name is name.
func HasName(name string) func(bson.Raw) bool {
	return func(raw bson.Raw) bool {
		n, ok := raw.Lookup("Name").StringValueOK()
		return ok && n == name
	}
}

// HasID keeps the document whose $ID is id.
func HasID(id model.ID) func(bson.Raw) bool {
	return func(raw bson.Raw) bool { return model.ID(codec.PeekID(raw)) == id }
}
