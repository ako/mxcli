// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	bsonv1 "go.mongodb.org/mongo-driver/bson"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ParseMicroflowFromRaw builds a Microflow from an already-unmarshalled BSON
// document that did NOT come from this project's storage — diff-local hands it
// a historical unit read back with `git show`, so there is no unit to look up.
//
// The codec decodes bytes, not maps, so the map is re-marshalled first. That is
// safe here because the result is read-only (it is rendered as MDL and thrown
// away): nothing this produces is written back, so the field reordering a map
// round-trip causes cannot reach storage.
func (b *Backend) ParseMicroflowFromRaw(raw map[string]any, unitID, containerID model.ID) *microflows.Microflow {
	data, err := bsonv1.Marshal(raw)
	if err != nil {
		return nil
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bsonv2.Raw(data))
	if err != nil {
		return nil
	}
	mf, ok := elem.(*genMf.Microflow)
	if !ok {
		return nil
	}
	out := microflowFromGen(mf, containerID)
	if out != nil && unitID != "" {
		out.ID = unitID
	}
	return out
}
