// SPDX-License-Identifier: Apache-2.0

// Package regularexpressions holds the BSON codec for regular expression
// documents (RegularExpressions$RegularExpression).
//
// It lives outside both engines because both write this document, and because
// the property key is a trap worth encoding once: modelsdk/gen binds the
// pattern to the BSON key "RegEx", while every Studio Pro-authored document
// stores it as "Expression". generated/metamodel agrees with the documents (its
// JSON tag is `expression`), so gen is the outlier — the same class of mismatch
// as the int32/int64 and datetime/string errors in scheduled events (#585).
//
// Writing "RegEx" would put a property on the node that the type does not
// carry, which is the shape mxbuild accepts and Studio Pro cannot open; reading
// it would return an empty pattern for every real document.
package regularexpressions

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
)

// TypeName is the BSON storage name for a regular expression document.
const TypeName = "RegularExpressions$RegularExpression"

// PatternKey is the BSON key holding the pattern. Named so the trap is greppable.
const PatternKey = "Expression"

// Serialize writes the document in the shape Studio Pro produces.
//
// Pinned against five Studio Pro-authored documents (Mendix Email Connector
// 6.4.2 ×1, Community Commons 11.5.1 ×4), which agree exactly on the property
// set and on alphabetical key order.
func Serialize(re *model.RegularExpression) ([]byte, error) {
	exportLevel := re.ExportLevel
	if exportLevel == "" {
		// Every reference document uses Hidden.
		exportLevel = "Hidden"
	}
	doc := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(string(re.ID))},
		{Key: "$Type", Value: TypeName},
		{Key: "Documentation", Value: re.Documentation},
		{Key: "Excluded", Value: re.Excluded},
		{Key: "ExportLevel", Value: exportLevel},
		{Key: PatternKey, Value: re.Expression},
		{Key: "Name", Value: re.Name},
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("serialize regular expression %q: %w", re.Name, err)
	}
	return out, nil
}

// Parse converts a stored document to the semantic type.
func Parse(doc bson.M, id, containerID model.ID) *model.RegularExpression {
	re := &model.RegularExpression{ContainerID: containerID}
	re.ID = id
	re.TypeName = TypeName
	re.Name, _ = doc["Name"].(string)
	re.Documentation, _ = doc["Documentation"].(string)
	re.Expression, _ = doc[PatternKey].(string)
	re.ExportLevel, _ = doc["ExportLevel"].(string)
	re.Excluded, _ = doc["Excluded"].(bool)
	return re
}
