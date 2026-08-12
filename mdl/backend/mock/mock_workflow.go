// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ---------------------------------------------------------------------------
// WorkflowBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListWorkflows() ([]*workflows.Workflow, error) {
	if m.ListWorkflowsFunc != nil {
		return m.ListWorkflowsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetWorkflow(id model.ID) (*workflows.Workflow, error) {
	if m.GetWorkflowFunc != nil {
		return m.GetWorkflowFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) CreateWorkflow(wf *workflows.Workflow) error {
	if m.CreateWorkflowFunc != nil {
		return m.CreateWorkflowFunc(wf)
	}
	return nil
}

func (m *MockBackend) UpdateWorkflow(wf *workflows.Workflow) error {
	if m.UpdateWorkflowFunc != nil {
		return m.UpdateWorkflowFunc(wf)
	}
	return nil
}

func (m *MockBackend) DeleteWorkflow(id model.ID) error {
	if m.DeleteWorkflowFunc != nil {
		return m.DeleteWorkflowFunc(id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SettingsBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) GetProjectSettings() (*model.ProjectSettings, error) {
	if m.GetProjectSettingsFunc != nil {
		return m.GetProjectSettingsFunc()
	}
	return nil, nil
}

func (m *MockBackend) UpdateProjectSettings(ps *model.ProjectSettings) error {
	if m.UpdateProjectSettingsFunc != nil {
		return m.UpdateProjectSettingsFunc(ps)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ImageBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListImageCollections() ([]*types.ImageCollection, error) {
	if m.ListImageCollectionsFunc != nil {
		return m.ListImageCollectionsFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListIconCollections() ([]*types.IconCollection, error) {
	if m.ListIconCollectionsFunc != nil {
		return m.ListIconCollectionsFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateImageCollection(ic *types.ImageCollection) error {
	if m.CreateImageCollectionFunc != nil {
		return m.CreateImageCollectionFunc(ic)
	}
	return nil
}

func (m *MockBackend) UpdateImageCollection(ic *types.ImageCollection) error {
	if m.UpdateImageCollectionFunc != nil {
		return m.UpdateImageCollectionFunc(ic)
	}
	return fmt.Errorf("MockBackend.UpdateImageCollection not configured")
}

func (m *MockBackend) DeleteImageCollection(id string) error {
	if m.DeleteImageCollectionFunc != nil {
		return m.DeleteImageCollectionFunc(id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ScheduledEventBackend
// ---------------------------------------------------------------------------

func (m *MockBackend) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	if m.ListScheduledEventsFunc != nil {
		return m.ListScheduledEventsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetScheduledEvent(id model.ID) (*model.ScheduledEvent, error) {
	if m.GetScheduledEventFunc != nil {
		return m.GetScheduledEventFunc(id)
	}
	return nil, nil
}

// The write methods default to a descriptive error rather than nil so a test
// that forgets to configure one fails on the missing stub instead of silently
// reporting a write that never happened.

func (m *MockBackend) CreateScheduledEvent(ev *model.ScheduledEvent) error {
	if m.CreateScheduledEventFunc != nil {
		return m.CreateScheduledEventFunc(ev)
	}
	return fmt.Errorf("MockBackend.CreateScheduledEvent not configured")
}

func (m *MockBackend) UpdateScheduledEvent(ev *model.ScheduledEvent) error {
	if m.UpdateScheduledEventFunc != nil {
		return m.UpdateScheduledEventFunc(ev)
	}
	return fmt.Errorf("MockBackend.UpdateScheduledEvent not configured")
}

func (m *MockBackend) DeleteScheduledEvent(id string) error {
	if m.DeleteScheduledEventFunc != nil {
		return m.DeleteScheduledEventFunc(id)
	}
	return fmt.Errorf("MockBackend.DeleteScheduledEvent not configured")
}
