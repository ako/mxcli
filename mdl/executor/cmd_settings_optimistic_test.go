// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// TestAlterSettingsModel_OptimisticLocking covers the gap reported by the
// mxcli-banking app: Mendix ships optimistic locking as an app setting (App
// Settings → Runtime), which is the mitigation for a read-then-write balance
// race in a transfer microflow, and ALTER SETTINGS MODEL refused every spelling
// of it with "unknown model setting". The property was already read and written
// by both engines — only the executor's assignment switch was missing.
//
// The stored key is EnableDataStorageOptimisticLocking, verified against a
// Studio-Pro-created project on both Mendix 9.24 and 11.13. Unlike JavaVersion
// (renamed to JavaMajorVersion between 11.6 and 11.12) this one did not move, so
// a single spelling is correct for every version mxcli supports.
func TestAlterSettingsModel_OptimisticLocking(t *testing.T) {
	tests := []struct {
		name  string
		given string
		want  bool
	}{
		{name: "enable", given: "true", want: true},
		{name: "disable", given: "false", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var written *model.ProjectSettings
			ctx, _ := newMockCtx(t, withBackend(captureSettingsBackend(&written)))

			err := alterSettings(ctx, &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{"EnableDataStorageOptimisticLocking": tc.given},
			})
			if err != nil {
				t.Fatalf("alterSettings: %v", err)
			}
			if written == nil {
				t.Fatal("no settings written")
			}
			if got := written.Model.EnableDataStorageOptimisticLocking; got != tc.want {
				t.Errorf("EnableDataStorageOptimisticLocking = %v, want %v", got, tc.want)
			}
		})
	}
}

// A non-boolean value must be refused rather than silently skipped — the same
// silent-no-op shape as mendixlabs/mxcli#805, where a bad Integer value skipped
// the assignment while the handler reported success.
func TestAlterSettingsModel_OptimisticLockingRejectsNonBool(t *testing.T) {
	wrote := false
	ctx, _ := newMockCtx(t, withBackend(settingsBackend(&wrote)))

	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section:    "model",
		Properties: map[string]any{"EnableDataStorageOptimisticLocking": "yes-please"},
	})
	if err == nil {
		t.Fatal("expected an error for a non-boolean value, got nil")
	}
	if !strings.Contains(err.Error(), "EnableDataStorageOptimisticLocking") {
		t.Errorf("error should name the offending key, got: %v", err)
	}
	if wrote {
		t.Error("a rejected statement must not write")
	}
}

// DESCRIBE must emit the property so a describe → edit → exec round trip can
// carry it. Without this the setting is writable but invisible, which is how a
// user discovers it does not exist.
func TestDescribeSettings_EmitsOptimisticLocking(t *testing.T) {
	var written *model.ProjectSettings
	ctx, buf := newMockCtx(t, withBackend(captureSettingsBackend(&written)))

	if err := describeSettings(ctx, ""); err != nil {
		t.Fatalf("describeSettings: %v", err)
	}
	assertContainsStr(t, buf.String(), "EnableDataStorageOptimisticLocking")
}

// TestModelSettingKeys_AllAccepted is the drift guard for the hand-maintained
// modelSettingKeys list: a name that appears in the error message's "valid keys"
// but is not actually assigned by the switch would send the next reader down the
// same dead end the bare message did.
func TestModelSettingKeys_AllAccepted(t *testing.T) {
	for _, key := range modelSettingKeys {
		t.Run(key, func(t *testing.T) {
			wrote := false
			ctx, _ := newMockCtx(t, withBackend(settingsBackend(&wrote)))

			// A value valid for every kind in the list: parses as a string, and
			// the typed keys are covered by TestTypedSettingsKeys_MatchExecutor.
			val := "1"
			if kind, ok := typedSettingsKeys["model"][key]; ok && kind == settingsKindBool {
				val = "true"
			}
			err := alterSettings(ctx, &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{key: val},
			})
			if err != nil && strings.Contains(err.Error(), "unknown model setting") {
				t.Errorf("%s is listed as valid but the executor rejects it: %v", key, err)
			}
		})
	}
}

// The unknown-setting error must list what IS accepted. The banking app tried
// OptimisticLocking, UseOptimisticLocking and EnableOptimisticLocking in turn
// and got a bare "unknown model setting" each time, with nothing pointing at the
// real name.
func TestAlterSettingsModel_UnknownKeyListsValidKeys(t *testing.T) {
	wrote := false
	ctx, _ := newMockCtx(t, withBackend(settingsBackend(&wrote)))

	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section:    "model",
		Properties: map[string]any{"OptimisticLocking": "true"},
	})
	if err == nil {
		t.Fatal("expected an error for an unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "EnableDataStorageOptimisticLocking") {
		t.Errorf("error should point at the real key name, got: %v", err)
	}
	if wrote {
		t.Error("a rejected statement must not write")
	}
}
