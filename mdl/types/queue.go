// SPDX-License-Identifier: Apache-2.0

package types

import "github.com/mendixlabs/mxcli/model"

// Queue is a Mendix task queue (Queues$Queue) — the configuration that governs
// how many instances of a queued microflow call run at once, and whether that
// limit is per-node or cluster-wide.
//
// The shape here follows four Studio Pro-authored queues from the Mendix
// Business Events module (Consumer_Queue, Consumer_Processor_Queue,
// Producer_Queue, Outbox_Cleanup_Queue), which agree exactly:
//
//	{ "$Type": "Queues$Queue", "Name": "Consumer_Queue",
//	  "Documentation": "", "Excluded": false, "ExportLevel": "Hidden",
//	  "Config": { "$Type": "Queues$BasicQueueConfig",
//	              "ClusterWide": false, "ParallelismExpression": "1" } }
//
// Two details are load-bearing and are not guesses:
//
//  1. Parallelism is stored as ParallelismExpression, a STRING holding a Mendix
//     expression ("1"). Queues$BasicQueueConfig also declares an int32
//     `Parallelism`, but Studio Pro wrote it zero times out of four — so it is
//     not written here either.
//  2. Config was Queues$BasicQueueConfig in all four, and it is the only config
//     type in the metamodel, so there is no variant to dispatch on.
type Queue struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Excluded      bool     `json:"excluded,omitempty"`
	ExportLevel   string   `json:"exportLevel,omitempty"`

	// Parallelism is the expression form (Config.ParallelismExpression), kept as
	// a string because that is how Mendix stores it: usually a literal like "3",
	// but any Mendix expression is valid.
	Parallelism string `json:"parallelism,omitempty"`
	// ClusterWide makes the parallelism limit apply across the cluster rather
	// than per runtime instance (Config.ClusterWide).
	ClusterWide bool `json:"clusterWide,omitempty"`
}

// GetName returns the queue's name.
func (q *Queue) GetName() string { return q.Name }

// GetContainerID returns the container ID.
func (q *Queue) GetContainerID() model.ID { return q.ContainerID }
