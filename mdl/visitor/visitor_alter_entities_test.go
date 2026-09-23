// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestBuildAlterEntities(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		module     string
		filter     ast.EntityPersistenceFilter
		actions    int
		firstAttr  string
		ifNotExist bool
	}{
		{
			name:   "module scoped, two actions, persistent filter",
			src:    "alter entities in Sales add attribute if not exists CreatedDate: AutoCreatedDate, add attribute if not exists ChangedDate: AutoChangedDate where persistent;",
			module: "Sales", filter: ast.EntityFilterPersistent, actions: 2,
			firstAttr: "CreatedDate", ifNotExist: true,
		},
		{
			name:   "no module, no filter",
			src:    "alter entities add attribute Note: string(10);",
			module: "", filter: ast.EntityFilterAll, actions: 1, firstAttr: "Note",
		},
		{
			name:   "non-persistent filter",
			src:    "alter entities in Sales add attribute Scratch: string(5) where non-persistent;",
			module: "Sales", filter: ast.EntityFilterNonPersistent, actions: 1, firstAttr: "Scratch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(tt.src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("got %d statements, want 1", len(prog.Statements))
			}
			s, ok := prog.Statements[0].(*ast.AlterEntitiesStmt)
			if !ok {
				t.Fatalf("got %T, want *ast.AlterEntitiesStmt", prog.Statements[0])
			}
			if s.Module != tt.module {
				t.Errorf("Module = %q, want %q", s.Module, tt.module)
			}
			if s.Filter != tt.filter {
				t.Errorf("Filter = %v, want %v", s.Filter, tt.filter)
			}
			if len(s.Actions) != tt.actions {
				t.Fatalf("got %d actions, want %d", len(s.Actions), tt.actions)
			}
			a := s.Actions[0]
			if a.Attribute == nil || a.Attribute.Name != tt.firstAttr {
				t.Errorf("first attribute = %v, want %q", a.Attribute, tt.firstAttr)
			}
			if a.IfNotExists != tt.ifNotExist {
				t.Errorf("IfNotExists = %v, want %v", a.IfNotExists, tt.ifNotExist)
			}
			// The per-entity name is filled in by the executor, never by the
			// visitor: a bulk action that carried a name would silently apply
			// to that one entity instead of the set.
			if a.Name.Name != "" || a.Name.Module != "" {
				t.Errorf("action carries a name %v; the executor must fill it per entity", a.Name)
			}
		})
	}
}

// The single-entity form must be unaffected by the new alternative.
func TestBuildAlterEntitySingularStillParses(t *testing.T) {
	prog, errs := Build("alter entity Sales.Order add attribute if not exists Note: string(10);")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if _, ok := prog.Statements[0].(*ast.AlterEntityStmt); !ok {
		t.Fatalf("got %T, want *ast.AlterEntityStmt", prog.Statements[0])
	}
}
