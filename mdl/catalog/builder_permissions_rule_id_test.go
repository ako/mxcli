// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Every row an entity access rule produces names that rule and its position on
// the entity (ako/mxcli#1437): two rules that agree on XPath, default rights and
// access are still two rules. A microflow grant comes from no access rule.
func TestPermissionRowsCarryTheirAccessRule(t *testing.T) {
	const mod = model.ID("mod-t")
	rule := func(id string, role string) *domainmodel.AccessRule {
		return &domainmodel.AccessRule{BaseElement: model.BaseElement{ID: model.ID(id)},
			ModuleRoleNames: []string{role}, AllowCreate: true,
			DefaultMemberAccessRights: domainmodel.MemberAccessRightsReadWrite}
	}
	be := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return &model.ProjectSettings{}, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) { return nil, nil },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: mod}, Name: "T"}}, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{BaseElement: model.BaseElement{ID: "dm-1"}, ContainerID: mod,
				Entities: []*domainmodel.Entity{{BaseElement: model.BaseElement{ID: "ent-1"}, Name: "Order",
					Attributes:  []*domainmodel.Attribute{{BaseElement: model.BaseElement{ID: "a-1"}, Name: "Code"}},
					AccessRules: []*domainmodel.AccessRule{rule("rule-a", "T.User"), rule("rule-b", "T.Admin")}}}}}, nil
		},
		ListRulesFunc:     func() ([]*microflows.Rule, error) { return nil, nil },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
		GetNavigationFunc: func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{{BaseElement: model.BaseElement{ID: "mf-1"}, ContainerID: mod,
				Name: "ACT_Save", AllowedModuleRoles: []model.ID{"T.User"}}}, nil
		},
	}
	cat, err := New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	b := NewBuilder(cat, be)
	b.SetFullMode(true)
	if err := b.Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}

	var got []string
	for _, row := range queryRows(t, cat, `SELECT ElementType, ModuleRoleName, AccessType,
		COALESCE(AccessRuleId, ''), COALESCE(AccessRuleIndex, -1) FROM permissions`) {
		got = append(got, fmt.Sprintf("%v %v %v %v@%v", row...))
	}
	sort.Strings(got)
	want := []string{
		"ENTITY T.Admin CREATE rule-b@1",
		"ENTITY T.Admin MEMBER_READ rule-b@1",
		"ENTITY T.Admin MEMBER_WRITE rule-b@1",
		"ENTITY T.Admin READ rule-b@1",
		"ENTITY T.Admin WRITE rule-b@1",
		"ENTITY T.User CREATE rule-a@0",
		"ENTITY T.User MEMBER_READ rule-a@0",
		"ENTITY T.User MEMBER_WRITE rule-a@0",
		"ENTITY T.User READ rule-a@0",
		"ENTITY T.User WRITE rule-a@0",
		"MICROFLOW T.User EXECUTE @-1",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("permission rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
