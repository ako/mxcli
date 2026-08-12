// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// Task queues (Queues$Queue). The document is small and flat apart from a single
// nested Config node, so it is read and written as raw BSON here rather than
// through a dedicated element type.
//
// The shape follows four Studio Pro-authored queues from the Mendix Business
// Events module. Note that Queues$BasicQueueConfig declares an int32
// `Parallelism` in addition to `ParallelismExpression`, and Studio Pro wrote it
// in none of them — so only the expression is read and written.

const queueUnitType = "Queues$Queue"

// ListQueues reads every task queue in the project.
func (r *Reader) ListQueues() ([]*types.Queue, error) {
	units, err := r.ListRawUnitsByType(queueUnitType)
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
		q.TypeName = queueUnitType
		q.Name, _ = doc["Name"].(string)
		q.Documentation, _ = doc["Documentation"].(string)
		q.Excluded, _ = doc["Excluded"].(bool)
		q.ExportLevel, _ = doc["ExportLevel"].(string)
		if cfg, ok := doc["Config"].(bson.M); ok {
			q.Parallelism, _ = cfg["ParallelismExpression"].(string)
			q.ClusterWide, _ = cfg["ClusterWide"].(bool)
		}
		out = append(out, q)
	}
	return out, nil
}

// CreateQueue inserts a new task queue document.
func (w *Writer) CreateQueue(q *types.Queue) error {
	if q == nil {
		return fmt.Errorf("CreateQueue: nil queue")
	}
	if q.ID == "" {
		q.ID = model.ID(generateUUID())
	}
	contents, err := serializeQueueUnit(q)
	if err != nil {
		return err
	}
	return w.insertUnit(string(q.ID), string(q.ContainerID), "Documents", queueUnitType, contents)
}

// UpdateQueue rewrites an existing task queue in place.
func (w *Writer) UpdateQueue(q *types.Queue) error {
	if q == nil {
		return fmt.Errorf("UpdateQueue: nil queue")
	}
	contents, err := serializeQueueUnit(q)
	if err != nil {
		return err
	}
	return w.UpdateRawUnit(string(q.ID), contents)
}

// DeleteQueue removes a task queue by ID.
func (w *Writer) DeleteQueue(id string) error {
	return w.deleteUnit(id)
}

func serializeQueueUnit(q *types.Queue) ([]byte, error) {
	parallelism := q.Parallelism
	if parallelism == "" {
		parallelism = "1"
	}
	exportLevel := q.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(q.ID))},
		{Key: "$Type", Value: queueUnitType},
		{Key: "Config", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Queues$BasicQueueConfig"},
			{Key: "ClusterWide", Value: q.ClusterWide},
			{Key: "ParallelismExpression", Value: parallelism},
		}},
		{Key: "Documentation", Value: q.Documentation},
		{Key: "Excluded", Value: q.Excluded},
		{Key: "ExportLevel", Value: exportLevel},
		{Key: "Name", Value: q.Name},
	}
	return marshalUnitIDFirst(doc)
}
