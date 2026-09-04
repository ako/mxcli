// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// An ALTER names a document that must already exist, and nothing checked that it
// did. `alter page RestLab."RestLab_Home"` — no such page — passed
// `mxcli check -p --references` and was then refused by `exec` with "page not
// found" (ako/mxcli-rest FINDINGS #60).
//
// Harmless in itself, and still worth closing, because it inverts the contract
// the check-syntax skill states: check is meant to be the strict gate and exec
// the thing that runs. Here exec was stricter, so a script could pass every
// pre-flight and then stop halfway through, having applied the statements before
// the typo and none after.
//
// The switch above this one already resolves the ALTER's MODULE. The document
// itself was the gap, which is why a misspelled module was reported and a
// misspelled document was not — a distinction with no meaning to the author.
//
// Scope. This covers the ALTER statements whose target is a document resolvable
// from a listing that already exists, and each one below was measured passing
// check and failing exec before it was added. It is not extended speculatively:
// a target this cannot resolve must be left alone rather than guessed at, since
// a false "not found" blocks a script that would have worked, which is worse
// than the silence it replaces.

// validateAlterTarget reports an ALTER whose target document does not exist,
// counting documents the script itself creates earlier.
func validateAlterTarget(ctx *ExecContext, stmt ast.Statement, sc *scriptContext) error {
	if !ctx.Connected() {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.AlterPageStmt:
		// One statement type, three document kinds — the visitor sets
		// ContainerType from the keyword.
		switch strings.ToUpper(s.ContainerType) {
		case "SNIPPET":
			return checkAlterTarget(ctx, sc, "snippet", s.PageName,
				buildSnippetQualifiedNames, func(qn string) bool { return sc.snippets[qn] })
		case "LAYOUT":
			return checkAlterTarget(ctx, sc, "layout", s.PageName,
				buildLayoutQualifiedNames, func(qn string) bool { return sc.layouts[qn] })
		default:
			return checkAlterTarget(ctx, sc, "page", s.PageName,
				buildPageQualifiedNames, func(qn string) bool { return sc.pages[qn] })
		}
	case *ast.AlterEntityStmt:
		return checkAlterTarget(ctx, sc, "entity", s.Name,
			buildEntityQualifiedNames, func(qn string) bool { return sc.entities[qn] })
	}
	return nil
}

// checkAlterTarget resolves one target, and on a miss reports the near names so
// the author can see the typo rather than only that they made one.
func checkAlterTarget(ctx *ExecContext, sc *scriptContext, kind string, name ast.QualifiedName,
	known func(*ExecContext) map[string]bool, inScript func(string) bool) error {
	qn := name.String()
	// An unqualified name is a different error, already reported elsewhere; and
	// a module the script creates has no listing to resolve against yet.
	if name.Module == "" || sc.modules[name.Module] {
		return nil
	}
	if inScript(qn) {
		return nil
	}
	stored := known(ctx)
	if stored[qn] {
		return nil
	}
	// An empty listing means the backend could not answer, not that the project
	// has no pages. Reporting "not found" from it would fail every script.
	if len(stored) == 0 {
		return nil
	}
	return mdlerrors.NewNotFoundMsg(kind, qn, fmt.Sprintf(
		"%s not found: %s (referenced by alter %s)%s", kind, qn, kind,
		nearNamesIn(stored, name.Module)))
}

// nearNamesIn lists what the module does have, capped so a large module does not
// bury the error. The same shape as availableAttributes.
func nearNamesIn(stored map[string]bool, module string) string {
	const max = 8
	var in []string
	for qn := range stored {
		if strings.HasPrefix(qn, module+".") {
			in = append(in, strings.TrimPrefix(qn, module+"."))
		}
	}
	if len(in) == 0 {
		return ""
	}
	sort.Strings(in)
	if len(in) > max {
		return fmt.Sprintf(" — %s has %s and %d more", module,
			strings.Join(in[:max], ", "), len(in)-max)
	}
	return fmt.Sprintf(" — %s has %s", module, strings.Join(in, ", "))
}

// buildLayoutQualifiedNames returns every layout qualified name in the project.
func buildLayoutQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	layouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return result
	}
	for _, l := range layouts {
		if l == nil {
			continue
		}
		result[h.GetQualifiedName(l.ContainerID, l.Name)] = true
	}
	return result
}
