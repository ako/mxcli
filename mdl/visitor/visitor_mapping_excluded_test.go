// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// DESCRIBE prints @excluded for an excluded import/export mapping
// (mendixlabs/mxcli#1185), so the statement has to read it back — otherwise a
// describe -> exec round trip creates the mapping live.
func TestCreateMapping_ExcludedAnnotation(t *testing.T) {
	input := `@excluded
create or modify import mapping MyModule.IMM_Pet with json structure MyModule.JSON_Pet {
	create MyModule.Pet { Name = name }
};
@excluded
create or modify export mapping MyModule.EXM_Pet with json structure MyModule.JSON_Pet {
	MyModule.Pet { name = Name }
};
create import mapping MyModule.IMM_Live with json structure MyModule.JSON_Pet {
	create MyModule.Pet { Name = name }
};`
	prog, errs := Build(input)
	for _, e := range errs {
		t.Fatalf("parse error: %v", e)
	}
	if len(prog.Statements) != 3 {
		t.Fatalf("got %d statements, want 3", len(prog.Statements))
	}
	if im, ok := prog.Statements[0].(*ast.CreateImportMappingStmt); !ok || !im.Excluded {
		t.Errorf("import mapping: want Excluded=true, got %#v", prog.Statements[0])
	}
	if em, ok := prog.Statements[1].(*ast.CreateExportMappingStmt); !ok || !em.Excluded {
		t.Errorf("export mapping: want Excluded=true, got %#v", prog.Statements[1])
	}
	if im, ok := prog.Statements[2].(*ast.CreateImportMappingStmt); !ok || im.Excluded {
		t.Errorf("unannotated import mapping must not be excluded, got %#v", prog.Statements[2])
	}
}
