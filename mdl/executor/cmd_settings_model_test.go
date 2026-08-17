// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
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

// modelSettingsBackend serves a project whose Settings$ModelSettings part carries
// exactly the given keys, so the presence gate can be exercised. The real backends
// populate RawParts from the stored document; settingsBackend does not carry a
// model part at all, which is the "cannot tell" case the gate treats permissively.
func modelSettingsBackend(out **model.ProjectSettings, keys ...string) *mock.MockBackend {
	part := map[string]any{"$Type": "Settings$ModelSettings"}
	for _, k := range keys {
		part[k] = ""
	}
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{
				Model:    &model.ModelSettings{HashAlgorithm: "BCrypt", BcryptCost: 10},
				RawParts: []map[string]any{part},
			}, nil
		},
		UpdateProjectSettingsFunc: func(ps *model.ProjectSettings) error {
			*out = ps
			return nil
		},
	}
}

// TestAlterSettingsModel_NewSettings covers the six properties added alongside
// optimistic locking. Each was already parsed and written by both engines and
// simply had no executor case, so MDL could not reach it.
func TestAlterSettingsModel_NewSettings(t *testing.T) {
	tests := []struct {
		key   string
		given string
		check func(*model.ModelSettings) any
		want  any
	}{
		{"DecimalScale", "4", func(m *model.ModelSettings) any { return m.DecimalScale }, 4},
		{"UseDatabaseForeignKeyConstraints", "false", func(m *model.ModelSettings) any { return m.UseDatabaseForeignKeyConstraints }, false},
		{"UseOQLVersion2", "false", func(m *model.ModelSettings) any { return m.UseOQLVersion2 }, false},
		{"DefaultTimeZoneCode", "Etc/UTC", func(m *model.ModelSettings) any { return m.DefaultTimeZoneCode }, "Etc/UTC"},
		{"FirstDayOfWeek", "Monday", func(m *model.ModelSettings) any { return m.FirstDayOfWeek }, "Monday"},
		{"SslCertificateAlgorithm", "SunX509", func(m *model.ModelSettings) any { return m.SslCertificateAlgorithm }, "SunX509"},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			var written *model.ProjectSettings
			ctx, _ := newMockCtx(t, withBackend(modelSettingsBackend(&written, tc.key)))

			if err := alterSettings(ctx, &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{tc.key: tc.given},
			}); err != nil {
				t.Fatalf("alterSettings: %v", err)
			}
			if written == nil {
				t.Fatal("no settings written")
			}
			if got := tc.check(written.Model); got != tc.want {
				t.Errorf("%s = %#v, want %#v", tc.key, got, tc.want)
			}
		})
	}
}

// The two enumeration-typed settings must be canonicalised to the member Mendix
// stores and refuse anything else, rather than writing a string through that the
// metamodel cannot resolve (the mendixlabs/mxcli#759 shape).
func TestAlterSettingsModel_EnumsCanonicaliseAndRefuse(t *testing.T) {
	tests := []struct {
		key   string
		given string
		want  string // "" = must be refused
	}{
		{"FirstDayOfWeek", "monday", "Monday"},
		{"FirstDayOfWeek", "Monday", "Monday"},
		{"FirstDayOfWeek", "Caturday", ""},
		{"SslCertificateAlgorithm", "pkix", "PKIX"},
		{"SslCertificateAlgorithm", "SunX509", "SunX509"},
		{"SslCertificateAlgorithm", "MD5", ""},
	}

	for _, tc := range tests {
		t.Run(tc.key+"/"+tc.given, func(t *testing.T) {
			var written *model.ProjectSettings
			ctx, _ := newMockCtx(t, withBackend(modelSettingsBackend(&written, tc.key)))

			err := alterSettings(ctx, &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{tc.key: tc.given},
			})

			if tc.want == "" {
				if err == nil {
					t.Fatalf("accepted %q, which is not a member of the enumeration", tc.given)
				}
				if written != nil {
					t.Error("a rejected statement must not write")
				}
				return
			}
			if err != nil {
				t.Fatalf("alterSettings: %v", err)
			}
			got := written.Model.FirstDayOfWeek
			if tc.key == "SslCertificateAlgorithm" {
				got = written.Model.SslCertificateAlgorithm
			}
			if got != tc.want {
				t.Errorf("%s = %q, want %q (the enum member Mendix stores)", tc.key, got, tc.want)
			}
		})
	}
}

// TestAlterSettingsModel_RefusesPropertyThisVersionDoesNotStore is the executor
// half of the presence gate. The overlay will not introduce a property the stored
// document does not carry, so without this refusal the statement would report
// success and change nothing.
func TestAlterSettingsModel_RefusesPropertyThisVersionDoesNotStore(t *testing.T) {
	var written *model.ProjectSettings
	// A Mendix 9.24-shaped document: no UseOQLVersion2.
	ctx, _ := newMockCtx(t, withBackend(modelSettingsBackend(&written, "HashAlgorithm", "BcryptCost")))

	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section:    "model",
		Properties: map[string]any{"UseOQLVersion2": "true"},
	})
	if err == nil {
		t.Fatal("expected a refusal for a property this project does not store, got nil")
	}
	if !strings.Contains(err.Error(), "UseOQLVersion2") {
		t.Errorf("error should name the property, got: %v", err)
	}
	if written != nil {
		t.Error("a refused statement must not write")
	}
}

// The gate must not block a property the document does carry.
func TestAlterSettingsModel_AllowsStoredProperty(t *testing.T) {
	var written *model.ProjectSettings
	ctx, _ := newMockCtx(t, withBackend(modelSettingsBackend(&written, "UseOQLVersion2")))

	if err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section:    "model",
		Properties: map[string]any{"UseOQLVersion2": "false"},
	}); err != nil {
		t.Fatalf("alterSettings: %v", err)
	}
	if written == nil {
		t.Fatal("no settings written")
	}
}

// DESCRIBE must emit only what the project stores, or replaying its output hits
// the refusal above.
func TestDescribeSettings_OmitsPropertiesThisVersionDoesNotStore(t *testing.T) {
	var written *model.ProjectSettings
	ctx, buf := newMockCtx(t, withBackend(modelSettingsBackend(&written,
		"HashAlgorithm", "BcryptCost", "EnableDataStorageOptimisticLocking")))

	if err := describeSettings(ctx, ""); err != nil {
		t.Fatalf("describeSettings: %v", err)
	}
	out := buf.String()
	assertContainsStr(t, out, "EnableDataStorageOptimisticLocking")
	for _, absent := range []string{"UseOQLVersion2", "DecimalScale", "SslCertificateAlgorithm"} {
		if strings.Contains(out, absent) {
			t.Errorf("described %s, which this project does not store — the output will not replay", absent)
		}
	}
}

// TestAlterSettingsModel_WithdrawnSettingNotWritable: Mendix withdrew
// UseSystemContextForBackgroundTasks — `mx check` on 11.13 rejects a project
// holding true with CE9436 "not supported anymore" — so mxcli must not offer it
// for writing even though it still parses and round-trips the stored value.
func TestAlterSettingsModel_WithdrawnSettingNotWritable(t *testing.T) {
	var written *model.ProjectSettings
	ctx, _ := newMockCtx(t, withBackend(modelSettingsBackend(&written, "UseSystemContextForBackgroundTasks")))

	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section:    "model",
		Properties: map[string]any{"UseSystemContextForBackgroundTasks": "true"},
	})
	if err == nil {
		t.Fatal("expected a refusal: setting this to true fails mx check with CE9436")
	}
	if written != nil {
		t.Error("a refused statement must not write")
	}
}

// ...but it must still survive a write that touches other settings, rather than
// being dropped or reset.
func TestAlterSettingsModel_WithdrawnSettingSurvivesOtherWrites(t *testing.T) {
	var written *model.ProjectSettings
	ctx, _ := newMockCtx(t, withBackend(modelSettingsBackend(&written,
		"BcryptCost", "UseSystemContextForBackgroundTasks")))

	if err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section:    "model",
		Properties: map[string]any{"BcryptCost": "13"},
	}); err != nil {
		t.Fatalf("alterSettings: %v", err)
	}
	if written == nil {
		t.Fatal("no settings written")
	}
	if written.Model.BcryptCost != 13 {
		t.Errorf("BcryptCost = %d, want 13", written.Model.BcryptCost)
	}
}

// TestAlterSettingsModel_UnknownKeyNamesItself: an unknown key and a real key
// this Mendix version does not store are different problems with different
// fixes, and the presence check cannot tell them apart. Running the presence
// check first answered a typo with "this project does not store
// NotARealSetting … upgrade the project", pointing at a version problem that
// does not exist.
func TestAlterSettingsModel_UnknownKeyNamesItself(t *testing.T) {
	var written *model.ProjectSettings
	ctx, _ := newMockCtx(t, withBackend(modelSettingsBackend(&written, "BcryptCost")))

	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section:    "model",
		Properties: map[string]any{"NotARealSetting": "true"},
	})
	if err == nil {
		t.Fatal("expected a refusal for an unknown model setting")
	}
	if !strings.Contains(err.Error(), "unknown model setting") {
		t.Errorf("error should say the key is unknown, got: %v", err)
	}
	if !strings.Contains(err.Error(), "valid keys") {
		t.Errorf("error should list the valid keys, got: %v", err)
	}
	if strings.Contains(err.Error(), "does not store") {
		t.Errorf("error blames the project's Mendix version for a key that does not exist: %v", err)
	}
	if written != nil {
		t.Error("a refused statement must not write")
	}
}

// The version-specific message must still be reachable for a real key.
func TestAlterSettingsModel_KnownKeyStillReportsVersionGap(t *testing.T) {
	var written *model.ProjectSettings
	ctx, _ := newMockCtx(t, withBackend(modelSettingsBackend(&written, "BcryptCost")))

	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section:    "model",
		Properties: map[string]any{"UseOQLVersion2": "true"},
	})
	if err == nil {
		t.Fatal("expected a refusal: this project does not store UseOQLVersion2")
	}
	if !strings.Contains(err.Error(), "does not store") {
		t.Errorf("a real key absent from the document should get the version message, got: %v", err)
	}
}
