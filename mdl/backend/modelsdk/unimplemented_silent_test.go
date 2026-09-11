// SPDX-License-Identifier: Apache-2.0

// gen_unimplemented.go promises that a method the modelsdk Backend does not
// override "fails loudly rather than silently dropping data". That promise is
// conditional: the generated stub reports failure by returning
// errUnimplemented, so it only holds for methods that HAVE an error to fail
// through. A method returning a bare value gets `var r0 T; return r0`.
//
// mendixlabs/mxcli#1080 is what that costs. ContentsDir() returns a plain
// string, was never overridden, and the zero value is not nonsense — "" is the
// in-band answer for "MPR v1, no mprcontents/". So `mxcli diff-local` on the
// default engine reported "mprcontents directory not found" for a v2 project
// whose directory was right there, and the missing implementation was
// indistinguishable from a v1 project.
package modelsdkbackend

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// methodsDeclaredOnBackend parses the package's own sources and returns the
// names of every method declared on Backend. Reading the source is the point:
// a promoted method and a declared one are indistinguishable through reflection
// (Go synthesises a wrapper named (*Backend).X for both), so runtime.FuncForPC
// cannot tell an override from the embedded stub.
func methodsDeclaredOnBackend(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	out := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if id, ok := star.X.(*ast.Ident); ok && id.Name == "Backend" {
				out[fd.Name.Name] = true
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("found no methods on *Backend — the guard would pass vacuously")
	}
	return out
}

// TestErrorlessBackendMethodsAreImplemented is the guard for the whole class:
// every FullBackend method that cannot report failure must be implemented by
// hand, because for those the generated stub is silent by construction.
//
// A method listed here is a decision, not an omission — record why.
func TestErrorlessBackendMethodsAreImplemented(t *testing.T) {
	exempt := map[string]string{}

	declared := methodsDeclaredOnBackend(t)
	iface := reflect.TypeOf((*backend.FullBackend)(nil)).Elem()

	var errorless, missing []string
	errType := reflect.TypeOf((*error)(nil)).Elem()
	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		hasErr := false
		for j := 0; j < m.Type.NumOut(); j++ {
			if m.Type.Out(j) == errType {
				hasErr = true
				break
			}
		}
		if hasErr {
			continue
		}
		errorless = append(errorless, m.Name)
		if !declared[m.Name] && exempt[m.Name] == "" {
			missing = append(missing, m.Name)
		}
	}
	if len(errorless) == 0 {
		t.Fatal("no errorless methods found on FullBackend — the guard would pass vacuously")
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these FullBackend methods return no error, so the generated stub\n"+
			"returns a zero value silently, and *Backend does not override them:\n  %s\n"+
			"Implement each one, or add it to `exempt` with the reason the zero value is correct.",
			strings.Join(missing, "\n  "))
	}
}
