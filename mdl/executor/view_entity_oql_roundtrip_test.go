// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// parseViewEntityQuery parses one `create view entity` statement and returns
// the OQL text exec would store (CreateViewEntityStmt.Query.RawQuery is
// written verbatim to the ViewEntitySourceDocument and the entity).
func parseViewEntityQuery(t *testing.T, src string) string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse failed: %v\n%s", errs, src)
	}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateViewEntityStmt); ok {
			return s.Query.RawQuery
		}
	}
	t.Fatalf("no create view entity statement in:\n%s", src)
	return ""
}

// ako/mxcli#653: describe → exec of a view entity added two spaces of
// indentation to lines 2…n of the stored OQL on every cycle, so exec reported
// `Modified view entity` forever. Describe indents every line by two; exec
// kept everything after the query's first token verbatim, including those two.
// One pass hides it — the drift only shows when you describe again — so this
// runs the cycle three times and requires the stored text to stay put.
func TestViewEntityOQL_DescribeExecRoundTripIsStable(t *testing.T) {
	cases := map[string]string{
		"column on its own line": `create view entity Shop.Ind (
  Total: Integer
) as (
  select
    sum(s.Amount) as Total
  from Shop.Sale as s
);`,
		"column on the select line": `create view entity Shop.Ind (
  Total: Integer
) as (
  select sum(s.Amount) as Total
  from Shop.Sale as s
);`,
	}
	// Queries already in a project, as Studio Pro stores them: flat, tab-indented,
	// with blank lines and comments. Describe → exec must hand each back
	// byte-identical — these were never written through mxcli's source text.
	storedCases := map[string]string{
		"studio pro flat":         "SELECT s.Amount AS Total\nFROM Shop.Sale AS s",
		"tabs and blank line":     "select\n\tsum(s.Amount) as Total\n\n\t-- every sale\nfrom Shop.Sale as s",
		"relative indent kept":    "select\n    sum(s.Amount) as Total\n  from Shop.Sale as s\n  where s.Amount > 0",
		"comment between clauses": "select sum(s.Amount) as Total\n/* all sales */\nfrom Shop.Sale as s",
	}
	for name, src := range cases {
		storedCases["script: "+name] = src
	}
	for name, src := range storedCases {
		t.Run(name, func(t *testing.T) {
			stored := src
			if _, fromScript := strings.CutPrefix(name, "script: "); fromScript {
				stored = parseViewEntityQuery(t, src) // the first exec
			}

			mod := mkModule("Shop")
			dmID := nextID("dm")
			ind := &domainmodel.Entity{
				BaseElement: model.BaseElement{ID: nextID("ent")},
				ContainerID: dmID,
				Name:        "Ind",
				Persistable: true,
				Source:      "DomainModels$OqlViewEntitySource",
				OqlQuery:    stored,
				Attributes:  []*domainmodel.Attribute{{Name: "Total"}},
			}
			dm := &domainmodel.DomainModel{
				BaseElement: model.BaseElement{ID: dmID},
				ContainerID: mod.ID,
				Entities:    []*domainmodel.Entity{ind},
			}
			h := mkHierarchy(mod)
			withContainer(h, dm.ID, mod.ID)
			mb := &mock.MockBackend{
				IsConnectedFunc:    func() bool { return true },
				ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
			}
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

			for cycle := 1; cycle <= 3; cycle++ {
				described, err := describeEntityToString(ctx, ast.QualifiedName{Module: "Shop", Name: "Ind"})
				if err != nil {
					t.Fatalf("cycle %d: describe: %v", cycle, err)
				}
				got := parseViewEntityQuery(t, described)
				if got != stored {
					t.Fatalf("cycle %d: describe → exec changed the stored OQL\nwant %q\n got %q\ndescribed:\n%s",
						cycle, stored, got, described)
				}
				ind.OqlQuery = got
			}
		})
	}
}
