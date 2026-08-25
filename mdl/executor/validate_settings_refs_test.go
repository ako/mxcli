// SPDX-License-Identifier: Apache-2.0

// ALTER SETTINGS is the one statement family that wrote a qualified name to the
// model without resolving it. A misspelled microflow or constant reported
// success, `mxcli check --references` said "All references valid", and mxbuild
// answered CE1613 one build later — "The selected microflow 'X' no longer
// exists." The constant case is worse than a dangling pointer: the override is
// CREATED for a constant that does not exist, which is dead configuration data
// nobody asked for (issue #274).
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func settingsRefStmt(section string, props map[string]any) *ast.AlterSettingsStmt {
	return &ast.AlterSettingsStmt{Section: section, Properties: props}
}

func TestValidateSettingsRefs_MicroflowKeys(t *testing.T) {
	known := map[string]bool{"SM.MF_Startup": true}
	sc := newScriptContext()

	for _, key := range []string{"AfterStartupMicroflow", "BeforeShutdownMicroflow", "HealthCheckMicroflow"} {
		if errs := validateSettingsReferences(
			settingsRefStmt("model", map[string]any{key: "SM.MF_Startup"}), known, nil, sc); len(errs) != 0 {
			t.Errorf("%s: a microflow that exists was rejected: %v", key, errs)
		}
		errs := validateSettingsReferences(
			settingsRefStmt("model", map[string]any{key: "Nope.MF_Missing"}), known, nil, sc)
		if len(errs) != 1 {
			t.Fatalf("%s: errors = %v, want exactly one", key, errs)
		}
		if msg := errs[0].Error(); !strings.Contains(msg, "Nope.MF_Missing") || !strings.Contains(msg, key) {
			t.Errorf("%s: message %q should name both the missing microflow and the setting", key, msg)
		}
	}
}

// An empty value clears the setting — Studio Pro's "(none)" — and must not be
// resolved as a name.
func TestValidateSettingsRefs_EmptyValueClearsAndIsNotAReference(t *testing.T) {
	if errs := validateSettingsReferences(
		settingsRefStmt("model", map[string]any{"AfterStartupMicroflow": ""}),
		map[string]bool{}, nil, newScriptContext()); len(errs) != 0 {
		t.Errorf("clearing the setting was reported as a bad reference: %v", errs)
	}
}

// A microflow created earlier in the same script is not missing — the same
// escape hatch every other statement family has.
func TestValidateSettingsRefs_ScriptCreatedMicroflowCounts(t *testing.T) {
	sc := newScriptContext()
	sc.microflows["SM.MF_Startup"] = true

	if errs := validateSettingsReferences(
		settingsRefStmt("model", map[string]any{"AfterStartupMicroflow": "SM.MF_Startup"}),
		map[string]bool{}, nil, sc); len(errs) != 0 {
		t.Errorf("a microflow created in this script was reported missing: %v", errs)
	}
}

func TestValidateSettingsRefs_WorkflowUserEntity(t *testing.T) {
	entities := map[string]bool{"System.User": true}
	sc := newScriptContext()

	if errs := validateSettingsReferences(
		settingsRefStmt("workflows", map[string]any{"UserEntity": "System.User"}), nil, entities, sc); len(errs) != 0 {
		t.Errorf("an entity that exists was rejected: %v", errs)
	}
	errs := validateSettingsReferences(
		settingsRefStmt("workflows", map[string]any{"UserEntity": "Nope.User"}), nil, entities, sc)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Nope.User") {
		t.Errorf("errors = %v, want one naming Nope.User", errs)
	}
}

// The constant override names a constant document. Writing one for a constant
// that does not exist creates dead configuration data and fails the build.
func TestValidateSettingsRefs_ConstantOverride(t *testing.T) {
	sc := newScriptContext()
	sc.constants["SM.Known"] = true

	stmt := &ast.AlterSettingsStmt{Section: "constant", ConstantId: "SM.Known", Value: "x"}
	if errs := validateSettingsConstantRef(stmt, map[string]bool{}, sc); len(errs) != 0 {
		t.Errorf("a constant created in this script was reported missing: %v", errs)
	}

	stmt = &ast.AlterSettingsStmt{Section: "constant", ConstantId: "SM.Typo", Value: "x"}
	errs := validateSettingsConstantRef(stmt, map[string]bool{"SM.Known": true}, sc)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "SM.Typo") {
		t.Fatalf("errors = %v, want one naming SM.Typo", errs)
	}
	// Dropping an override for a constant that is gone is legitimate cleanup.
	drop := &ast.AlterSettingsStmt{Section: "constant", ConstantId: "SM.Typo", DropConstant: true}
	if errs := validateSettingsConstantRef(drop, map[string]bool{"SM.Known": true}, sc); len(errs) != 0 {
		t.Errorf("dropping an override for a missing constant was rejected: %v", errs)
	}
}

// A backend that cannot list constants yields an empty index, which is
// indistinguishable from a project that has none. The difference matters, so the
// caller only resolves when the index is trustworthy — a check that rejects
// everything on a backend it cannot read is worse than no check.
func TestValidateSettingsRefs_UntrustedIndexIsNotEvidence(t *testing.T) {
	// The guard belongs to the CALLER, so what this pins is that an empty index
	// with no script context does reject — i.e. the caller must not pass one it
	// could not build.
	stmt := &ast.AlterSettingsStmt{Section: "constant", ConstantId: "Mod.Anything", Value: "x"}
	if errs := validateSettingsConstantRef(stmt, map[string]bool{}, newScriptContext()); len(errs) != 1 {
		t.Fatalf("errors = %v, want one: against a KNOWN-empty project every override dangles", errs)
	}
}

// A near-miss gets a suggestion, and only a near-miss: a wrong guess sends the
// reader off looking at the wrong document.
func TestNearestConstant_OnlySuggestsARealNearMiss(t *testing.T) {
	known := map[string]bool{"Mod.BaseUrl": true, "Other.Timeout": true}

	if got := nearestConstant("mod.baseurl", known); got != "Mod.BaseUrl" {
		t.Errorf("case-only difference: got %q, want Mod.BaseUrl", got)
	}
	if got := nearestConstant("Wrong.BaseUrl", known); got != "Mod.BaseUrl" {
		t.Errorf("wrong module, same name: got %q, want Mod.BaseUrl", got)
	}
	if got := nearestConstant("Mod.CompletelyDifferent", known); got != "" {
		t.Errorf("unrelated name got a suggestion: %q", got)
	}
}
