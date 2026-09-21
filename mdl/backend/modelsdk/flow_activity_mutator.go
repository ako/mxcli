// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/flowmutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
)

var _ backend.FlowActivityMutationBackend = (*Backend)(nil)

// SetActivitiesDisabled toggles the Disabled flag on the matching activities of
// one stored flow unit, by editing the unit's raw BSON.
//
// The write goes through UpdateRawUnit, which is the same choke point every
// other write uses, so ADR-0008's elision and identity preservation apply
// unchanged: a statement that changes nothing writes nothing, and one that does
// change something keeps every element $ID. Re-running a disable therefore
// reports "Unchanged" rather than dirtying the .mpr.
func (b *Backend) SetActivitiesDisabled(unitID model.ID, filter types.ActivityFilter, disable bool) (backend.FlowActivityChange, error) {
	var out backend.FlowActivityChange
	if b.writer == nil {
		return out, fmt.Errorf("SetActivitiesDisabled: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return out, fmt.Errorf("SetActivitiesDisabled: load unit: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return out, fmt.Errorf("SetActivitiesDisabled: unmarshal: %w", err)
	}

	match := flowmutator.NewMatch(filter, decodableTypeNames())
	out.Matched = flowmutator.Count(d, match)

	// A matching activity with no Disabled property is counted and left alone,
	// never patched. Adding a key the type does not have at that Mendix version
	// is what makes a project Studio Pro cannot open, and the version gate
	// already refuses the authoring path — this is the same rule applied where
	// the document, rather than the project, is the evidence.
	flowmutator.EachActivity(d, func(a bson.D) {
		if match(a) && !flowmutator.HasDisabledKey(a) {
			out.WithoutField++
		}
	})

	out.Changed = flowmutator.Apply(d, match, disable)
	if out.Changed == 0 {
		return out, nil
	}
	outBytes, err := bson.Marshal(d)
	if err != nil {
		return out, fmt.Errorf("SetActivitiesDisabled: marshal: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(unitID), outBytes); err != nil {
		return out, fmt.Errorf("SetActivitiesDisabled: save unit: %w", err)
	}
	return out, nil
}

// decodableTypeNames is the set of `$Type` strings the codec can decode, used to
// resolve an action named by its storage label rather than by an MDL keyword.
//
// Derived from the registry rather than listed, for the reason the catalog's
// getMicroflowActionType gives: a hand-kept parallel list silently buckets what
// it forgets, and here that would mean a filter matching nothing.
func decodableTypeNames() map[string]bool {
	names := codec.DefaultRegistry.TypeNames()
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}
