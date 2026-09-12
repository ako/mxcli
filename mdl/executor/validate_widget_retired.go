// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A widget keyword whose Mendix type no longer exists.
//
// `statictext` writes `Forms$Text`, and Mendix 11 has no such type. The result
// is not a build error — it is a project that cannot be LOADED:
//
//	ERROR: System.AggregateException: One or more errors occurred.
//	 (The type cache does not contain a type with qualified name Forms$Text.)
//	 ---> Mendix.Modeler.Storage.Caches.TypeCacheUnknownTypeException
//
// `mx check` fails there, before any validation runs, and Studio Pro fails the
// same way — so the page is unopenable and unfixable in the modeler. Measured on
// mxbuild 11.14.0 against ako/TestApp, and reproduced identically with
// MXCLI_ENGINE=legacy: both writers emit the type, so this is not an engine
// difference and switching engines is not a workaround.
//
// `Forms$Text` is absent from BOTH generated sources — zero occurrences in
// modelsdk/gen and no type in generated/metamodel — and from 3,257 Studio
// Pro-authored text holders in ako/TestApp. The modern widget is
// `Forms$DynamicText`, which `dynamictext` writes.
//
// The READERS keep handling `Forms$Text` (cmd_pages_describe_parse.go,
// cmd_page_wireframe.go, theme_reader.go): a project converted up from an old
// Mendix version can still carry one, and describing it is useful. Only writing
// a new one is refused.
var retiredWidgetKinds = map[string]struct {
	storedType string
	replacedBy string
}{
	"statictext": {storedType: "Forms$Text", replacedBy: "dynamictext"},
}

// validateRetiredWidgetKind refuses a widget keyword whose stored $Type Mendix
// no longer has.
//
// This is deliberately not version-gated. mxcli has no Mendix version in which
// `Forms$Text` is known — the oldest reflection snapshot in the repo
// (generated/metamodel, 11.6.0) does not declare it either — so a gate would
// have no version to let through, and guessing one would trade a clear refusal
// for an unopenable project.
func validateRetiredWidgetKind(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || w.TypeIsGeneric {
		return nil
	}
	r, ok := retiredWidgetKinds[strings.ToLower(w.Type)]
	if !ok {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET27",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf("%s: `%s` writes %s, a type Mendix does not have — the project cannot be opened afterwards",
			locationPrefix, strings.ToLower(w.Type), r.storedType),
		Suggestion: fmt.Sprintf("use `%s` instead; it writes the widget Mendix actually has", r.replacedBy),
	}}
}
