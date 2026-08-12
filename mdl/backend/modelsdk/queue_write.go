// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// queueTypeName is the BSON storage name for a task queue document.
const queueTypeName = "Queues$Queue"

// ListQueues reads every Queues$Queue unit.
func (b *Backend) ListQueues() ([]*types.Queue, error) {
	units, err := b.reader.ListRawUnitsByType(queueTypeName)
	if err != nil {
		return nil, err
	}
	out := make([]*types.Queue, 0, len(units))
	for _, u := range units {
		var doc bson.M
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			return nil, fmt.Errorf("unmarshal queue %s: %w", u.ID, err)
		}
		q := &types.Queue{ContainerID: model.ID(u.ContainerID)}
		q.ID = model.ID(u.ID)
		q.TypeName = queueTypeName
		q.Name, _ = doc["Name"].(string)
		q.Documentation, _ = doc["Documentation"].(string)
		q.Excluded, _ = doc["Excluded"].(bool)
		q.ExportLevel, _ = doc["ExportLevel"].(string)
		// Parallelism and cluster scope live on the nested Config node.
		if cfg, ok := doc["Config"].(bson.M); ok {
			q.Parallelism, _ = cfg["ParallelismExpression"].(string)
			q.ClusterWide, _ = cfg["ClusterWide"].(bool)
		}
		out = append(out, q)
	}
	return out, nil
}

// CreateQueue inserts a new Queues$Queue document.
func (b *Backend) CreateQueue(q *types.Queue) error {
	if q == nil {
		return fmt.Errorf("CreateQueue: nil queue")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateQueue: not connected for writing")
	}
	if q.ID == "" {
		q.ID = model.ID(mmpr.GenerateID())
	}
	return b.writer.InsertUnit(string(q.ID), string(q.ContainerID), "Documents", queueTypeName, serializeQueue(q))
}

// UpdateQueue rewrites an existing queue in place.
func (b *Backend) UpdateQueue(q *types.Queue) error {
	if q == nil {
		return fmt.Errorf("UpdateQueue: nil queue")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateQueue: not connected for writing")
	}
	return b.writer.UpdateRawUnit(string(q.ID), serializeQueue(q))
}

// DeleteQueue removes a queue unit by ID.
func (b *Backend) DeleteQueue(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteQueue: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

// serializeQueue writes the document in the shape Studio Pro produces.
//
// Keys are alphabetical, matching every other writer here and the observed
// documents. Two things are deliberate:
//
//   - ParallelismExpression is a STRING. Queues$BasicQueueConfig also declares an
//     int32 Parallelism, but Studio Pro wrote it in none of the four reference
//     documents, so writing it would be inventing a property.
//   - An empty Parallelism becomes "1", which is what every observed queue has
//     and what Mendix treats as the default; an empty expression is not a
//     meaningful queue configuration.
func serializeQueue(q *types.Queue) []byte {
	parallelism := q.Parallelism
	if parallelism == "" {
		parallelism = "1"
	}
	exportLevel := q.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}
	config := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(mmpr.GenerateID())},
		{Key: "$Type", Value: "Queues$BasicQueueConfig"},
		{Key: "ClusterWide", Value: q.ClusterWide},
		{Key: "ParallelismExpression", Value: parallelism},
	}
	doc := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(string(q.ID))},
		{Key: "$Type", Value: queueTypeName},
		{Key: "Config", Value: config},
		{Key: "Documentation", Value: q.Documentation},
		{Key: "Excluded", Value: q.Excluded},
		{Key: "ExportLevel", Value: exportLevel},
		{Key: "Name", Value: q.Name},
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		// bson.Marshal on a fixed bson.D of primitives cannot fail; a nil return
		// would be written as an empty unit, so surface it loudly instead.
		panic(fmt.Sprintf("serializeQueue: marshal %q: %v", q.Name, err))
	}
	return out
}
