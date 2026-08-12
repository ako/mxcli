// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

func (m *MockBackend) ListNavigationDocuments() ([]*types.NavigationDocument, error) {
	if m.ListNavigationDocumentsFunc != nil {
		return m.ListNavigationDocumentsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetNavigation() (*types.NavigationDocument, error) {
	if m.GetNavigationFunc != nil {
		return m.GetNavigationFunc()
	}
	return nil, nil
}

func (m *MockBackend) UpdateNavigationProfile(navDocID model.ID, profileName string, spec types.NavigationProfileSpec) error {
	if m.UpdateNavigationProfileFunc != nil {
		return m.UpdateNavigationProfileFunc(navDocID, profileName, spec)
	}
	return nil
}

func (m *MockBackend) ListMenuDocuments() ([]*types.MenuDocument, error) {
	if m.ListMenuDocumentsFunc != nil {
		return m.ListMenuDocumentsFunc()
	}
	return nil, fmt.Errorf("MockBackend.ListMenuDocuments not configured")
}

func (m *MockBackend) GetMenuDocumentByQualifiedName(moduleName, name string) (*types.MenuDocument, error) {
	if m.GetMenuDocumentByQualifiedNameFunc != nil {
		return m.GetMenuDocumentByQualifiedNameFunc(moduleName, name)
	}
	return nil, fmt.Errorf("MockBackend.GetMenuDocumentByQualifiedName not configured")
}

func (m *MockBackend) CreateMenuDocument(md *types.MenuDocument) error {
	if m.CreateMenuDocumentFunc != nil {
		return m.CreateMenuDocumentFunc(md)
	}
	return fmt.Errorf("MockBackend.CreateMenuDocument not configured")
}

func (m *MockBackend) UpdateMenuDocument(md *types.MenuDocument) error {
	if m.UpdateMenuDocumentFunc != nil {
		return m.UpdateMenuDocumentFunc(md)
	}
	return fmt.Errorf("MockBackend.UpdateMenuDocument not configured")
}

func (m *MockBackend) DeleteMenuDocument(id model.ID) error {
	if m.DeleteMenuDocumentFunc != nil {
		return m.DeleteMenuDocumentFunc(id)
	}
	return fmt.Errorf("MockBackend.DeleteMenuDocument not configured")
}
