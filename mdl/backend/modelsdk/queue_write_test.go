// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// TestSerializeQueue_MatchesStudioProShape pins the document against four real
// Studio Pro-authored queues from the Mendix Business Events module
// (Consumer_Queue, Consumer_Processor_Queue, Producer_Queue,
// Outbox_Cleanup_Queue), which agree exactly on this shape.
//
// Two assertions are the whole point of the test:
//
//   - ParallelismExpression is a STRING. Queues$BasicQueueConfig also declares an
//     int32 `Parallelism`, and writing that instead (or as well) would be
//     inventing a property Mendix does not write.
//   - `Parallelism` must be ABSENT. It appeared in none of the four references.
func TestSerializeQueue_MatchesStudioProShape(t *testing.T) {
	q := &types.Queue{Name: "OrderProcessing", Parallelism: "3", ClusterWide: true}
	q.ID = "11111111-1111-1111-1111-111111111111"

	var doc bson.M
	if err := bson.Unmarshal(serializeQueue(q), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := doc["$Type"]; got != "Queues$Queue" {
		t.Errorf("$Type = %v, want Queues$Queue", got)
	}
	if got := doc["Name"]; got != "OrderProcessing" {
		t.Errorf("Name = %v", got)
	}
	if got := doc["ExportLevel"]; got != "Hidden" {
		t.Errorf("ExportLevel = %v, want Hidden (every reference document uses it)", got)
	}
	if _, ok := doc["Excluded"].(bool); !ok {
		t.Errorf("Excluded missing or not a bool: %v", doc["Excluded"])
	}

	cfg, ok := doc["Config"].(bson.M)
	if !ok {
		t.Fatalf("Config is not a document: %T", doc["Config"])
	}
	if got := cfg["$Type"]; got != "Queues$BasicQueueConfig" {
		t.Errorf("Config.$Type = %v", got)
	}
	if got, ok := cfg["ParallelismExpression"].(string); !ok || got != "3" {
		t.Errorf("ParallelismExpression = %#v, want the string \"3\" — Mendix stores an expression, not a number", cfg["ParallelismExpression"])
	}
	if _, present := cfg["Parallelism"]; present {
		t.Error("Parallelism must not be written: Studio Pro wrote it in none of the four reference queues")
	}
	if got := cfg["ClusterWide"]; got != true {
		t.Errorf("ClusterWide = %v, want true", got)
	}
}

// TestSerializeQueue_Defaults covers the empty case: every observed queue has a
// parallelism, so an unspecified one becomes "1" rather than an empty
// expression, which is not a meaningful configuration.
func TestSerializeQueue_Defaults(t *testing.T) {
	q := &types.Queue{Name: "Plain"}
	q.ID = "22222222-2222-2222-2222-222222222222"

	var doc bson.M
	if err := bson.Unmarshal(serializeQueue(q), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg := doc["Config"].(bson.M)
	if got := cfg["ParallelismExpression"]; got != "1" {
		t.Errorf("default ParallelismExpression = %v, want \"1\"", got)
	}
	if got := cfg["ClusterWide"]; got != false {
		t.Errorf("default ClusterWide = %v, want false", got)
	}
	if got := doc["ExportLevel"]; got != "Hidden" {
		t.Errorf("default ExportLevel = %v, want Hidden", got)
	}
}

// TestQueueRoundTrip writes a queue and reads it back through the real backend.
func TestQueueRoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	if err := b.CreateQueue(&types.Queue{
		ContainerID: mod.ID, Name: "ZzQueue", Parallelism: "5", ClusterWide: true,
	}); err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	queues, err := b2.ListQueues()
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	for _, q := range queues {
		if q.Name != "ZzQueue" {
			continue
		}
		if q.Parallelism != "5" {
			t.Errorf("Parallelism = %q, want 5", q.Parallelism)
		}
		if !q.ClusterWide {
			t.Error("ClusterWide did not round-trip")
		}
		return
	}
	t.Fatalf("ZzQueue not found after create (got %d queues)", len(queues))
}
