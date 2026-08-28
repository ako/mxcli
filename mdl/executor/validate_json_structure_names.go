// SPDX-License-Identifier: Apache-2.0

// Check-time validation for CUSTOM NAME MAP entries that match nothing.
//
// An entry naming a key the snippet does not contain was applied to no element
// and reported nothing — so a typo in a custom name was indistinguishable from
// not having written one. That silence is how ako/mxcli#272's real defect stayed
// hidden: `item of` did not exist, and `'data|(Object)' as 'Record'` — the
// obvious thing to try — parsed, executed, and did nothing at all.
//
// Decidable from the statement (the snippet is right there), so it runs in the
// no-project pass and a plain `mxcli check` catches it.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// ValidateJsonStructureNames reports (MDL-JSON01) a CUSTOM NAME MAP entry whose
// key is not in the snippet, and (MDL-JSON02) an `item of` entry whose key is in
// the snippet but does not reach an array.
func ValidateJsonStructureNames(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateJsonStructureStmt)
		if !ok || (len(s.CustomNameMap) == 0 && len(s.CustomItemNameMap) == 0) {
			continue
		}
		keys, arrayKeys, rootIsArray, err := types.SnippetKeys(s.JsonSnippet)
		if err != nil {
			// An unparseable snippet is a different, louder failure reported by
			// the executor; saying it twice here helps nobody.
			continue
		}
		for _, key := range sortedKeysOf(s.CustomNameMap) {
			if !keys[key] {
				out = append(out, unknownKeyViolation(s, key, "", keys))
			}
		}
		for _, key := range sortedKeysOf(s.CustomItemNameMap) {
			if key == types.RootArrayItemKey {
				if !rootIsArray {
					out = append(out, itemNotArrayViolation(s, key,
						"the snippet's root is an object, not an array"))
				}
				continue
			}
			if !keys[key] {
				out = append(out, unknownKeyViolation(s, key, "item of ", keys))
				continue
			}
			if !arrayKeys[key] {
				out = append(out, itemNotArrayViolation(s, key,
					fmt.Sprintf("%q is not an array, so it has no item element", key)))
			}
		}
	}
	return out
}

func unknownKeyViolation(s *ast.CreateJsonStructureStmt, key, prefix string, keys map[string]bool) linter.Violation {
	available := sortedSet(keys)
	msg := fmt.Sprintf("json structure %s: `%s'%s' as …` names a key the snippet does not contain, "+
		"so it renames nothing", s.Name.String(), prefix, key)
	if len(available) > 0 {
		msg += "; available: " + strings.Join(available, ", ")
	}
	return linter.Violation{
		RuleID:   "MDL-JSON01",
		Severity: linter.SeverityError,
		Message:  msg,
		Location: linter.Location{
			Module:       s.Name.Module,
			DocumentType: "json structure",
			DocumentName: s.Name.Name,
		},
		Suggestion: "Use the JSON key exactly as it appears in the snippet",
	}
}

func itemNotArrayViolation(s *ast.CreateJsonStructureStmt, key, why string) linter.Violation {
	return linter.Violation{
		RuleID:   "MDL-JSON02",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf("json structure %s: `item of '%s'` names the item element of an array, but %s",
			s.Name.String(), key, why),
		Location: linter.Location{
			Module:       s.Name.Module,
			DocumentType: "json structure",
			DocumentName: s.Name.Name,
		},
		Suggestion: "Drop `item of` to rename the element itself",
	}
}

func sortedKeysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
