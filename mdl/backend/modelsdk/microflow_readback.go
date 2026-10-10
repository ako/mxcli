// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"fmt"
	"sort"

	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/modelsdk/version"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ReadBackMicroflow returns mf as the reader would return it once
// CreateMicroflow had stored it: encoded by the same writer, decoded by the
// same reader, and nothing written. What the writer defaults, and what the
// document has no property for, reads back as it would from the project, so
// the result compares with a stored microflow like for like (ako/mxcli#859).
//
// It fails when the reader does not read back everything the writer wrote —
// when the result, written again, is not the document mf was written as. A
// comparison of two read-back flows cannot see a property the reader drops:
// both sides hold nothing there, whatever was written, so a change to it
// would compare as no change and never be written.
func (b *Backend) ReadBackMicroflow(mf *microflows.Microflow) (*microflows.Microflow, error) {
	if mf == nil {
		return nil, fmt.Errorf("ReadBackMicroflow: nil microflow")
	}
	encode := func(m *microflows.Microflow) ([]byte, error) {
		gm := microflowToGen(m, b.majorVersion())
		gm.SetID(element.ID(readBackID(m.ID)))
		assignMicroflowIDs(gm)
		raw, err := (&codec.Encoder{}).Encode(gm)
		return b.completeAsStored(raw), err
	}
	written, err := encode(mf)
	if err != nil {
		return nil, fmt.Errorf("ReadBackMicroflow: encode: %w", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bsonv2.Raw(written))
	if err != nil {
		return nil, fmt.Errorf("ReadBackMicroflow: decode: %w", err)
	}
	g, ok := el.(*genMf.Microflow)
	if !ok {
		return nil, fmt.Errorf("ReadBackMicroflow: decoded a %T", el)
	}
	rb := microflowFromGen(g, mf.ContainerID)
	again, err := encode(rb)
	if err != nil {
		return nil, fmt.Errorf("ReadBackMicroflow: encode the read back: %w", err)
	}
	if err := sameWritten(written, again); err != nil {
		return nil, fmt.Errorf("ReadBackMicroflow: the reader does not read back %w", err)
	}
	return rb, nil
}

// ReadBackNanoflow is ReadBackMicroflow for a nanoflow (CreateNanoflow).
func (b *Backend) ReadBackNanoflow(nf *microflows.Nanoflow) (*microflows.Nanoflow, error) {
	if nf == nil {
		return nil, fmt.Errorf("ReadBackNanoflow: nil nanoflow")
	}
	encode := func(n *microflows.Nanoflow) ([]byte, error) {
		g := nanoflowToGen(n, b.majorVersion())
		g.SetID(element.ID(readBackID(n.ID)))
		assignNanoflowIDs(g)
		raw, err := (&codec.Encoder{}).Encode(g)
		return b.completeAsStored(raw), err
	}
	written, err := encode(nf)
	if err != nil {
		return nil, fmt.Errorf("ReadBackNanoflow: encode: %w", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bsonv2.Raw(written))
	if err != nil {
		return nil, fmt.Errorf("ReadBackNanoflow: decode: %w", err)
	}
	gn, ok := el.(*genMf.Nanoflow)
	if !ok {
		return nil, fmt.Errorf("ReadBackNanoflow: decoded a %T", el)
	}
	rb := nanoflowFromGen(gn, nf.ContainerID)
	again, err := encode(rb)
	if err != nil {
		return nil, fmt.Errorf("ReadBackNanoflow: encode the read back: %w", err)
	}
	if err := sameWritten(written, again); err != nil {
		return nil, fmt.Errorf("ReadBackNanoflow: the reader does not read back %w", err)
	}
	return rb, nil
}

func readBackID(id model.ID) model.ID {
	if id == "" {
		return model.ID(mmpr.GenerateID())
	}
	return id
}

// sameWritten reports where two encodings of one flow differ, other than in
// the element IDs ($ID) a sub-element the model does not keep an ID for is
// minted afresh with on every write.
func sameWritten(a, b []byte) error {
	var x, y any
	if err := bsonv2.Unmarshal(a, &x); err != nil {
		return err
	}
	if err := bsonv2.Unmarshal(b, &y); err != nil {
		return err
	}
	return sameWrittenValue("", x, y)
}

func sameWrittenValue(path string, a, b any) error {
	a, b = writtenValue(a), writtenValue(b)
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return fmt.Errorf("%s (a document written, %T read back)", path, b)
		}
		if t, ok := x["$Type"].(string); ok {
			path += "<" + t + ">"
		}
		keys := map[string]bool{}
		for k := range x {
			keys[k] = true
		}
		for k := range y {
			keys[k] = true
		}
		ks := make([]string, 0, len(keys))
		for k := range keys {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			if k == "$ID" {
				continue
			}
			if err := sameWrittenValue(path+"."+k, x[k], y[k]); err != nil {
				return err
			}
		}
		return nil
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return fmt.Errorf("%s (a list of %d written, %v read back)", path, len(x), b)
		}
		for i := range x {
			if err := sameWrittenValue(fmt.Sprintf("%s[%d]", path, i), x[i], y[i]); err != nil {
				return err
			}
		}
		return nil
	case bsonv2.Binary:
		// An ID or a pointer to one: pointers to objects keep their IDs; a
		// pointer to a sub-element minted afresh is compared by presence.
		if _, ok := b.(bsonv2.Binary); !ok {
			return fmt.Errorf("%s (written, %v read back)", path, b)
		}
		return nil
	default:
		if !writtenEqual(a, b) {
			return fmt.Errorf("%s (%v written, %v read back)", path, a, b)
		}
		return nil
	}
}

func writtenValue(v any) any {
	switch x := v.(type) {
	case bsonv2.D:
		m := make(map[string]any, len(x))
		for _, e := range x {
			m[e.Key] = e.Value
		}
		return m
	case bsonv2.M:
		return map[string]any(x)
	case bsonv2.A:
		return []any(x)
	}
	return v
}

func writtenEqual(a, b any) bool {
	if ab, ok := a.([]byte); ok {
		bb, ok := b.([]byte)
		return ok && bytes.Equal(ab, bb)
	}
	return fmt.Sprintf("%T:%v", a, a) == fmt.Sprintf("%T:%v", b, b)
}

// completeAsStored gives encoded contents the property set storage would give
// them on the write (canon.StripUndeclaredProperties and
// canon.CompletePropertySets, applied by the mpr writer to every unit). A read-back that skipped it would compare a declared flow
// without those properties against a stored one with them — a difference on
// every re-run wherever the comparison sees bytes, as it does for a raw
// `call web service` payload (mendixlabs/mxcli#1373).
func (b *Backend) completeAsStored(contents []byte) []byte {
	pv := b.ProjectVersion()
	if pv == nil || pv.MajorVersion == 0 || contents == nil {
		return contents
	}
	v := version.Version{Major: pv.MajorVersion, Minor: pv.MinorVersion, Patch: pv.PatchVersion}
	if stripped, err := canon.StripUndeclaredProperties(contents, &v); err == nil {
		// A refusal is the write's to report; the prediction keeps the bytes.
		contents = stripped
	}
	return canon.CompletePropertySets(contents, &v)
}
