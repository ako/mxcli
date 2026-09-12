// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The only two FullBackend methods the modelsdk engine did not implement that
// anything actually calls through the interface.
//
// Measured with scripts/backend-reachability.sh, which removes one method from
// its interface at a time and rebuilds: 17 of the 19 unimplemented methods have
// no caller through a backend value at all, and these two have four, all in
// mdl/executor/cmd_microflows_builder.go (lookupMicroflowReturnType,
// lookupNanoflowReturnType, microflowExists, nanoflowExists).
//
// Nothing was broken by their absence, which is why this went unnoticed: each of
// the four is a fast path with a slow-path fallback, so on the default engine
// the fast path errored with "not implemented yet" and the code fell through to
// an O(n) walk that loads and parses every microflow in the module. The cost was
// speed, not correctness — and the ONLY remaining reason a default-engine run
// could hit errUnimplemented, which is what stands between here and dropping
// legacy from nightly.

// GetRawUnitByName returns a unit's raw info by object type and qualified name.
// The codec reader already indexes by name; this was simply never exposed as a
// Backend method (the same gap units.go describes for GetRawUnitBytes).
func (b *Backend) GetRawUnitByName(objectType, qualifiedName string) (*types.RawUnitInfo, error) {
	return b.reader.GetRawUnitByName(objectType, qualifiedName)
}

// GetRawMicroflowByName returns a microflow unit's raw BSON by qualified name.
//
// Unreachable through the interface today — the reachability probe found no
// caller — but it is the sibling of the method above on the same interface, the
// reader already has it, and leaving exactly one of a pair stubbed is the shape
// that produces a puzzling failure later.
func (b *Backend) GetRawMicroflowByName(qualifiedName string) ([]byte, error) {
	return b.reader.GetRawMicroflowByName(qualifiedName)
}

// ParseMicroflowBSON decodes a stored microflow or nanoflow document.
//
// Both, despite the name: the callers pass nanoflow contents to it as well and
// read ReturnType off the result. The legacy parser is untyped — it walks a
// bson map, so a nanoflow document parses into a Microflow struct by accident of
// sharing key names. The codec is typed and decodes to two different gen types,
// so the nanoflow case has to be handled deliberately rather than falling out.
//
// containerID is passed through rather than resolved: the callers already know
// which module they asked about, and a lookup here would undo the point of the
// fast path.
func (b *Backend) ParseMicroflowBSON(contents []byte, unitID, containerID model.ID) (*microflows.Microflow, error) {
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bsonv2.Raw(contents))
	if err != nil {
		return nil, err
	}
	var out *microflows.Microflow
	switch g := elem.(type) {
	case *genMf.Microflow:
		out = microflowFromGen(g, containerID)
	case *genMf.Nanoflow:
		// The interface returns a Microflow, so a nanoflow is carried in one.
		// Only the fields the callers read are meaningful here; a nanoflow is
		// not a microflow and this value must not be written back.
		nf := nanoflowFromGen(g, containerID)
		if nf == nil {
			return nil, nil
		}
		out = &microflows.Microflow{
			ContainerID:        nf.ContainerID,
			Name:               nf.Name,
			Documentation:      nf.Documentation,
			Excluded:           nf.Excluded,
			AllowedModuleRoles: nf.AllowedModuleRoles,
			ReturnType:         nf.ReturnType,
			Parameters:         nf.Parameters,
			ObjectCollection:   nf.ObjectCollection,
		}
		out.ID = nf.ID
		out.TypeName = "Microflows$Nanoflow"
	default:
		return nil, nil
	}
	if out != nil && unitID != "" {
		out.ID = unitID
	}
	return out, nil
}
