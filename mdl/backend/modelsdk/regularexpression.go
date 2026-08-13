// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	regex "github.com/mendixlabs/mxcli/mdl/regularexpressions"
	"github.com/mendixlabs/mxcli/model"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// Regular expressions. Read and written as raw BSON through
// mdl/regularexpressions rather than through modelsdk/gen, whose generated type
// binds the pattern to the wrong BSON key — see that package's doc comment.

func (b *Backend) ListRegularExpressions() ([]*model.RegularExpression, error) {
	units, err := b.reader.ListRawUnitsByType(regex.TypeName)
	if err != nil {
		return nil, err
	}
	out := make([]*model.RegularExpression, 0, len(units))
	for _, u := range units {
		var doc bson.M
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			return nil, fmt.Errorf("unmarshal regular expression %s: %w", u.ID, err)
		}
		out = append(out, regex.Parse(doc, model.ID(u.ID), model.ID(u.ContainerID)))
	}
	return out, nil
}

func (b *Backend) CreateRegularExpression(re *model.RegularExpression) error {
	if re == nil {
		return fmt.Errorf("CreateRegularExpression: nil regular expression")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateRegularExpression: not connected for writing")
	}
	if re.ID == "" {
		re.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := regex.Serialize(re)
	if err != nil {
		return err
	}
	return b.writer.InsertUnit(string(re.ID), string(re.ContainerID), "Documents", regex.TypeName, contents)
}

func (b *Backend) UpdateRegularExpression(re *model.RegularExpression) error {
	if re == nil {
		return fmt.Errorf("UpdateRegularExpression: nil regular expression")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateRegularExpression: not connected for writing")
	}
	contents, err := regex.Serialize(re)
	if err != nil {
		return err
	}
	return b.writer.UpdateRawUnit(string(re.ID), contents)
}

func (b *Backend) DeleteRegularExpression(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteRegularExpression: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}
