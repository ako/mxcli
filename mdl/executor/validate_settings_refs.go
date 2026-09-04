// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
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
func validateSettingsReferences(stmt *ast.AlterSettingsStmt, knownMicroflows, knownEntities map[string]bool, returnTypes map[string]string, sc *scriptContext) []error {
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
			if !knownMicroflows[ref] && !(sc != nil && sc.microflows[ref]) {
				errs = append(errs, mdlerrors.NewValidationf(
					"microflow not found: %s (referenced by %s) — the model stores the name as written, "+
						"so the build reports CE1613 \"The selected microflow no longer exists\"",
					ref, key))
				continue
			}
			if err := checkAfterStartupReturnsBoolean(key, ref, returnTypes, sc); err != nil {
				errs = append(errs, err)
			}
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

// checkAfterStartupReturnsBoolean reports an after-startup microflow that does
// not return Boolean.
//
// Mendix requires it, and the build says so late and out of context:
// CE0142 "After startup microflow should return a boolean". The setting itself
// resolves fine — the name exists — so #274's existence check passes and the
// script writes a project that cannot build. The trip-up in the wild is a
// seed/demo-data microflow wired to after-startup: it does its work, returns
// nothing, and the build refuses it (CapTrackV2 FINDINGS §6).
//
// Only AfterStartupMicroflow is checked. BeforeShutdown and HealthCheck have
// their own rules, and neither has been measured here — asserting one on a
// guess would be the same defect in the other direction.
//
// A microflow whose return type is unknown is left alone. That covers a
// script-created flow the context did not record and a backend that could not
// list return types: in both cases nothing is KNOWN to be wrong, and a refusal
// would block a script that builds.
func checkAfterStartupReturnsBoolean(key, ref string, returnTypes map[string]string, sc *scriptContext) error {
	if !strings.EqualFold(key, "AfterStartupMicroflow") {
		return nil
	}
	var actual string
	switch {
	case sc != nil && sc.microflows[ref]:
		sig, ok := sc.flowParams[strings.ToLower(ref)]
		if !ok || sig == nil {
			return nil // created in this script, signature not recorded
		}
		if sig.ReturnKind == ast.TypeBoolean {
			return nil
		}
		actual = sig.ReturnKind.String()
		if sig.ReturnKind == ast.TypeVoid {
			actual = "nothing"
		}
	default:
		stored, ok := returnTypes[ref]
		if !ok {
			return nil // return type unavailable — say nothing rather than guess
		}
		if stored == "Boolean" {
			return nil
		}
		actual = stored
		if stored == "" {
			actual = "nothing"
		}
	}

	return mdlerrors.NewValidationf(
		"after-startup microflow %s returns %s, but Mendix requires Boolean — the build reports "+
			"CE0142 \"After startup microflow should return a boolean\". The setting stores only the "+
			"name, so this is not caught by resolving the reference.",
		ref, actual)
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

// ValidateAfterStartupReturnType (MDL073) is the project-less half of the
// after-startup check.
//
// The rule needs the microflow's return type, and the overwhelmingly common
// shape is one script that CREATES the seed microflow and wires it in the same
// breath — so the answer is in the script itself and `mxcli check` with no
// project can give it. The project path (validateSettingsReferences) covers the
// other half, where the microflow was already stored.
//
// Both call checkAfterStartupReturnsBoolean, so the two cannot drift.
func ValidateAfterStartupReturnType(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}

	returnTypes := map[string]string{}
	for _, stmt := range prog.Statements {
		mf, ok := stmt.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		if mf.ReturnType == nil {
			returnTypes[mf.Name.String()] = ""
			continue
		}
		returnTypes[mf.Name.String()] = mf.ReturnType.Type.Kind.String()
	}
	if len(returnTypes) == 0 {
		return nil
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterSettingsStmt)
		if !ok || !strings.EqualFold(s.Section, "model") {
			continue
		}
		raw, ok := s.Properties["AfterStartupMicroflow"]
		if !ok {
			continue
		}
		ref := settingsValueToString(raw)
		if ref == "" {
			continue
		}
		// Only a microflow this script defines: anything else needs the project,
		// and reporting it from here would guess.
		if _, defined := returnTypes[ref]; !defined {
			continue
		}
		if err := checkAfterStartupReturnsBoolean("AfterStartupMicroflow", ref, returnTypes, nil); err != nil {
			out = append(out, linter.Violation{
				RuleID:     "MDL073",
				Severity:   linter.SeverityError,
				Message:    err.Error(),
				Suggestion: "End the microflow with `return true;` and declare `returns boolean` on it",
			})
		}
	}
	return out
}
