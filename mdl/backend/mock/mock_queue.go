// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// QueueBackend — task queues (Queues$Queue).
//
// The write methods default to a descriptive error rather than nil so a test
// that forgets to configure one fails on the missing stub instead of silently
// reporting a write that never happened.

func (m *MockBackend) ListQueues() ([]*types.Queue, error) {
	if m.ListQueuesFunc != nil {
		return m.ListQueuesFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateQueue(q *types.Queue) error {
	if m.CreateQueueFunc != nil {
		return m.CreateQueueFunc(q)
	}
	return fmt.Errorf("MockBackend.CreateQueue not configured")
}

func (m *MockBackend) UpdateQueue(q *types.Queue) error {
	if m.UpdateQueueFunc != nil {
		return m.UpdateQueueFunc(q)
	}
	return fmt.Errorf("MockBackend.UpdateQueue not configured")
}

func (m *MockBackend) DeleteQueue(id string) error {
	if m.DeleteQueueFunc != nil {
		return m.DeleteQueueFunc(id)
	}
	return fmt.Errorf("MockBackend.DeleteQueue not configured")
}
