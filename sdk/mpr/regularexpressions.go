// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	regex "github.com/mendixlabs/mxcli/mdl/regularexpressions"
	"github.com/mendixlabs/mxcli/model"
)

// Regular expressions for the legacy engine. The document shape lives in
// mdl/regularexpressions so both engines write exactly the same bytes.

// ListRegularExpressions reads every regular expression in the project.
func (r *Reader) ListRegularExpressions() ([]*model.RegularExpression, error) {
	units, err := r.ListRawUnitsByType(regex.TypeName)
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

// CreateRegularExpression inserts a new regular expression document.
func (w *Writer) CreateRegularExpression(re *model.RegularExpression) error {
	if re == nil {
		return fmt.Errorf("CreateRegularExpression: nil regular expression")
	}
	if re.ID == "" {
		re.ID = model.ID(generateUUID())
	}
	contents, err := regex.Serialize(re)
	if err != nil {
		return err
	}
	return w.insertUnit(string(re.ID), string(re.ContainerID), "Documents", regex.TypeName, contents)
}

// UpdateRegularExpression rewrites an existing regular expression in place.
func (w *Writer) UpdateRegularExpression(re *model.RegularExpression) error {
	if re == nil {
		return fmt.Errorf("UpdateRegularExpression: nil regular expression")
	}
	contents, err := regex.Serialize(re)
	if err != nil {
		return err
	}
	return w.UpdateRawUnit(string(re.ID), contents)
}

// DeleteRegularExpression removes a regular expression by ID.
func (w *Writer) DeleteRegularExpression(id string) error {
	return w.deleteUnit(id)
}
