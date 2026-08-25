// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// settingsMicroflowKeys are the model settings whose value is a microflow's
// qualified name. Mendix resolves each on load and reports CE1613 — "The
// selected microflow 'X' no longer exists." — when it cannot.
var settingsMicroflowKeys = []string{
	"AfterStartupMicroflow",
	"BeforeShutdownMicroflow",
	"HealthCheckMicroflow",
}

// validateSettingsReferences resolves the qualified names an ALTER SETTINGS
// statement writes into the model.
//
// ALTER SETTINGS was the one statement family that wrote a name without
// resolving it: a misspelled microflow reported success, `--references` said
// "All references valid", and mxbuild answered CE1613 one build later (#274).
//
// Empty is not a reference — it clears the setting, which is Studio Pro's
// "(none)" — and a name created earlier in the same script is not missing, the
// same escape hatch every other family has.
func validateSettingsReferences(stmt *ast.AlterSettingsStmt, knownMicroflows, knownEntities map[string]bool, sc *scriptContext) []error {
	if stmt == nil {
		return nil
	}
	var errs []error
	section := strings.ToLower(stmt.Section)

	if section == "model" {
		for _, key := range settingsMicroflowKeys {
			raw, ok := stmt.Properties[key]
			if !ok {
				continue
			}
			ref := settingsValueToString(raw)
			if ref == "" {
				continue // clearing the setting
			}
			if knownMicroflows[ref] || (sc != nil && sc.microflows[ref]) {
				continue
			}
			errs = append(errs, mdlerrors.NewValidationf(
				"microflow not found: %s (referenced by %s) — the model stores the name as written, "+
					"so the build reports CE1613 \"The selected microflow no longer exists\"",
				ref, key))
		}
	}

	if section == "workflows" {
		if raw, ok := stmt.Properties["UserEntity"]; ok {
			ref := settingsValueToString(raw)
			if ref != "" && !knownEntities[ref] && !(sc != nil && sc.entities[ref]) {
				errs = append(errs, mdlerrors.NewValidationf(
					"entity not found: %s (referenced by UserEntity)", ref))
			}
		}
	}
	return errs
}

// validateSettingsConstantRef resolves the constant an override names.
//
// This one is not merely a dangling pointer: `alter settings constant 'Typo'
// value 'x'` CREATES the override, so a misspelled name leaves dead
// configuration data behind and the build fails. Dropping an override is exempt
// — removing one whose constant is already gone is legitimate cleanup, and
// refusing it would leave no way to tidy up.
func validateSettingsConstantRef(stmt *ast.AlterSettingsStmt, knownConstants map[string]bool, sc *scriptContext) []error {
	if stmt == nil || !strings.EqualFold(stmt.Section, "constant") || stmt.DropConstant {
		return nil
	}
	ref := stmt.ConstantId
	if ref == "" || knownConstants[ref] || (sc != nil && sc.constants[ref]) {
		return nil
	}
	msg := fmt.Sprintf(
		"constant not found: %s (referenced by alter settings constant) — the override would be written "+
			"for a constant that does not exist, which the build reports as CE1613", ref)
	if near := nearestConstant(ref, knownConstants); near != "" {
		msg += fmt.Sprintf("; did you mean %s?", near)
	}
	return []error{mdlerrors.NewValidation(msg)}
}

// nearestConstant returns a constant whose name differs from ref only in case or
// module, the two ways a hand-written override goes wrong. It is deliberately not
// a general edit-distance suggestion: a wrong guess sends the reader off looking
// at the wrong document.
func nearestConstant(ref string, known map[string]bool) string {
	shortRef := ref
	if i := strings.LastIndex(ref, "."); i >= 0 {
		shortRef = ref[i+1:]
	}
	var candidates []string
	for name := range known {
		if strings.EqualFold(name, ref) {
			return name
		}
		short := name
		if i := strings.LastIndex(name, "."); i >= 0 {
			short = name[i+1:]
		}
		if strings.EqualFold(short, shortRef) {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Strings(candidates)
	return candidates[0]
}
