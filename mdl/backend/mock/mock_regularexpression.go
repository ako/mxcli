// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
)

// RegularExpressionBackend — regex documents.
//
// The write methods default to a descriptive error rather than nil so a test
// that forgets to configure one fails on the missing stub instead of silently
// reporting a write that never happened.

func (m *MockBackend) ListRegularExpressions() ([]*model.RegularExpression, error) {
	if m.ListRegularExpressionsFunc != nil {
		return m.ListRegularExpressionsFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateRegularExpression(re *model.RegularExpression) error {
	if m.CreateRegularExpressionFunc != nil {
		return m.CreateRegularExpressionFunc(re)
	}
	return fmt.Errorf("MockBackend.CreateRegularExpression not configured")
}

func (m *MockBackend) UpdateRegularExpression(re *model.RegularExpression) error {
	if m.UpdateRegularExpressionFunc != nil {
		return m.UpdateRegularExpressionFunc(re)
	}
	return fmt.Errorf("MockBackend.UpdateRegularExpression not configured")
}

func (m *MockBackend) DeleteRegularExpression(id string) error {
	if m.DeleteRegularExpressionFunc != nil {
		return m.DeleteRegularExpressionFunc(id)
	}
	return fmt.Errorf("MockBackend.DeleteRegularExpression not configured")
}
