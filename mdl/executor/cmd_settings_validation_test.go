// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// settingsBackend returns a MockBackend serving a single 'Default' configuration and
// recording whether a write was attempted, so a rejected statement can be shown to
// be a no-op rather than a silent partial write.
func settingsBackend(wrote *bool) *mock.MockBackend {
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			ps := &model.ProjectSettings{
				Model: &model.ModelSettings{
					HashAlgorithm:             "BCrypt",
					BcryptCost:                10,
					AllowUserMultipleSessions: true,
				},
				Workflows: &model.WorkflowsSettings{
					DefaultTaskParallelism:    5,
					WorkflowEngineParallelism: 5,
				},
				Configuration: &model.ConfigurationSettings{
					Configurations: []*model.ServerConfiguration{{
						Name:             "Default",
						HttpPortNumber:   8080,
						ServerPortNumber: 8090,
					}},
				},
			}
			ps.RawParts = []map[string]any{{"$Type": "Settings$ConfigurationSettings"}}
			return ps, nil
		},
		UpdateProjectSettingsFunc: func(*model.ProjectSettings) error {
			*wrote = true
			return nil
		},
	}
}

// TestAlterSettings_RejectsNonIntegerValues is the regression test for
// mendixlabs/mxcli#805: strconv.Atoi's error was discarded, so an invalid value for
// an Integer-typed setting skipped the assignment while the handler still reported
// success — a silent no-op.
func TestAlterSettings_RejectsNonIntegerValues(t *testing.T) {
	tests := []struct {
		name string
		stmt *ast.AlterSettingsStmt
		key  string
	}{
		{
			name: "configuration HttpPortNumber",
			stmt: &ast.AlterSettingsStmt{
				Section:    "configuration",
				ConfigName: "Default",
				Properties: map[string]any{"HttpPortNumber": "not-a-number"},
			},
			key: "HttpPortNumber",
		},
		{
			name: "configuration ServerPortNumber",
			stmt: &ast.AlterSettingsStmt{
				Section:    "configuration",
				ConfigName: "Default",
				Properties: map[string]any{"ServerPortNumber": "8O9O"},
			},
			key: "ServerPortNumber",
		},
		{
			name: "model BcryptCost",
			stmt: &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{"BcryptCost": "high"},
			},
			key: "BcryptCost",
		},
		{
			name: "workflows DefaultTaskParallelism",
			stmt: &ast.AlterSettingsStmt{
				Section:    "workflows",
				Properties: map[string]any{"DefaultTaskParallelism": "many"},
			},
			key: "DefaultTaskParallelism",
		},
		{
			name: "workflows WorkflowEngineParallelism",
			stmt: &ast.AlterSettingsStmt{
				Section:    "workflows",
				Properties: map[string]any{"WorkflowEngineParallelism": ""},
			},
			key: "WorkflowEngineParallelism",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrote := false
			ctx, buf := newMockCtx(t, withBackend(settingsBackend(&wrote)))

			err := alterSettings(ctx, tc.stmt)
			if err == nil {
				t.Fatalf("alterSettings accepted an invalid %s; output was %q", tc.key, buf.String())
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error does not name the setting: %v", err)
			}
			if !strings.Contains(err.Error(), "must be an integer") {
				t.Errorf("error does not say what was expected: %v", err)
			}
			if wrote {
				t.Errorf("a rejected statement still wrote the settings document")
			}
			if out := buf.String(); strings.Contains(out, "Updated") {
				t.Errorf("a rejected statement still reported success: %q", out)
			}
		})
	}
}

// TestAlterSettings_RejectsNonBooleanValues covers the same silent no-op in its
// boolean form: `valStr == "true"` turned anything else into false.
func TestAlterSettings_RejectsNonBooleanValues(t *testing.T) {
	for _, val := range []string{"yes", "ture", "1", ""} {
		t.Run("AllowUserMultipleSessions="+val, func(t *testing.T) {
			wrote := false
			ctx, buf := newMockCtx(t, withBackend(settingsBackend(&wrote)))

			err := alterSettings(ctx, &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{"AllowUserMultipleSessions": val},
			})
			if err == nil {
				t.Fatalf("alterSettings accepted AllowUserMultipleSessions=%q; output was %q", val, buf.String())
			}
			if !strings.Contains(err.Error(), "must be true or false") {
				t.Errorf("unexpected error: %v", err)
			}
			if wrote {
				t.Errorf("a rejected statement still wrote the settings document")
			}
		})
	}
}

// TestAlterSettings_AcceptsValidTypedValues guards against over-rejecting: the valid
// spellings, including a numeric literal from the parser and surrounding whitespace,
// must still be applied and written.
func TestAlterSettings_AcceptsValidTypedValues(t *testing.T) {
	tests := []struct {
		name   string
		stmt   *ast.AlterSettingsStmt
		verify func(*testing.T, *model.ProjectSettings)
	}{
		{
			name: "quoted integer",
			stmt: &ast.AlterSettingsStmt{
				Section:    "configuration",
				ConfigName: "Default",
				Properties: map[string]any{"HttpPortNumber": "8123"},
			},
			verify: func(t *testing.T, ps *model.ProjectSettings) {
				if got := ps.Configuration.Configurations[0].HttpPortNumber; got != 8123 {
					t.Errorf("HttpPortNumber = %d, want 8123", got)
				}
			},
		},
		{
			name: "numeric literal",
			stmt: &ast.AlterSettingsStmt{
				Section:    "configuration",
				ConfigName: "Default",
				Properties: map[string]any{"ServerPortNumber": int64(9090)},
			},
			verify: func(t *testing.T, ps *model.ProjectSettings) {
				if got := ps.Configuration.Configurations[0].ServerPortNumber; got != 9090 {
					t.Errorf("ServerPortNumber = %d, want 9090", got)
				}
			},
		},
		{
			name: "padded integer",
			stmt: &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{"BcryptCost": " 12 "},
			},
			verify: func(t *testing.T, ps *model.ProjectSettings) {
				if got := ps.Model.BcryptCost; got != 12 {
					t.Errorf("BcryptCost = %d, want 12", got)
				}
			},
		},
		{
			name: "boolean false",
			stmt: &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{"AllowUserMultipleSessions": "false"},
			},
			verify: func(t *testing.T, ps *model.ProjectSettings) {
				if ps.Model.AllowUserMultipleSessions {
					t.Error("AllowUserMultipleSessions = true, want false")
				}
			},
		},
		{
			name: "boolean literal",
			stmt: &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{"AllowUserMultipleSessions": true},
			},
			verify: func(t *testing.T, ps *model.ProjectSettings) {
				if !ps.Model.AllowUserMultipleSessions {
					t.Error("AllowUserMultipleSessions = false, want true")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrote := false
			mb := settingsBackend(&wrote)
			var written *model.ProjectSettings
			mb.UpdateProjectSettingsFunc = func(ps *model.ProjectSettings) error {
				wrote = true
				written = ps
				return nil
			}
			ctx, _ := newMockCtx(t, withBackend(mb))

			if err := alterSettings(ctx, tc.stmt); err != nil {
				t.Fatalf("alterSettings rejected a valid value: %v", err)
			}
			if !wrote {
				t.Fatal("a valid statement did not write the settings document")
			}
			tc.verify(t, written)
		})
	}
}

// TestCreateConfiguration_RejectsNonIntegerValues covers the same discarded
// conversion on the CREATE CONFIGURATION path.
func TestCreateConfiguration_RejectsNonIntegerValues(t *testing.T) {
	for _, key := range []string{"HttpPortNumber", "ServerPortNumber"} {
		t.Run(key, func(t *testing.T) {
			wrote := false
			ctx, _ := newMockCtx(t, withBackend(settingsBackend(&wrote)))

			err := createConfiguration(ctx, &ast.CreateConfigurationStmt{
				Name:       "Acceptance",
				Properties: map[string]any{key: "eighty-eighty"},
			})
			if err == nil {
				t.Fatalf("createConfiguration accepted an invalid %s", key)
			}
			if !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "must be an integer") {
				t.Errorf("unexpected error: %v", err)
			}
			if wrote {
				t.Errorf("a rejected CREATE CONFIGURATION still wrote the settings document")
			}
		})
	}
}

// TestValidateSettings_ReportsTypedValueErrors covers the check-time half of the
// fix: `mxcli check` must report the same invalid values the executor rejects, so a
// typo surfaces before the project is opened for writing.
func TestValidateSettings_ReportsTypedValueErrors(t *testing.T) {
	tests := []struct {
		name   string
		stmt   *ast.AlterSettingsStmt
		ruleID string
	}{
		{
			name: "integer",
			stmt: &ast.AlterSettingsStmt{
				Section:    "configuration",
				ConfigName: "Default",
				Properties: map[string]any{"HttpPortNumber": "eighty-eighty"},
			},
			ruleID: "MDL-SET01",
		},
		{
			name: "boolean",
			stmt: &ast.AlterSettingsStmt{
				Section:    "model",
				Properties: map[string]any{"AllowUserMultipleSessions": "yes"},
			},
			ruleID: "MDL-SET02",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateSettings(tc.stmt)
			if len(got) != 1 {
				t.Fatalf("ValidateSettings returned %d violations, want 1: %+v", len(got), got)
			}
			if got[0].RuleID != tc.ruleID {
				t.Errorf("RuleID = %q, want %q", got[0].RuleID, tc.ruleID)
			}
			if got[0].Severity != linter.SeverityError {
				t.Errorf("Severity = %v, want error", got[0].Severity)
			}
			if got[0].Suggestion == "" {
				t.Error("violation has no suggestion")
			}
		})
	}
}

func TestValidateSettings_AcceptsValidAndUnknownSections(t *testing.T) {
	valid := ValidateSettings(&ast.AlterSettingsStmt{
		Section:    "configuration",
		Properties: map[string]any{"HttpPortNumber": "8080", "DatabaseName": "anything goes"},
	})
	if len(valid) != 0 {
		t.Errorf("valid properties reported as violations: %+v", valid)
	}
	// A section with no typed properties (or an unknown one) is not this rule's business.
	if got := ValidateSettings(&ast.AlterSettingsStmt{
		Section:    "language",
		Properties: map[string]any{"DefaultLanguageCode": "en_US"},
	}); len(got) != 0 {
		t.Errorf("language section reported violations: %+v", got)
	}
}

func TestValidateCreateConfiguration_ReportsTypedValueErrors(t *testing.T) {
	got := ValidateCreateConfiguration(&ast.CreateConfigurationStmt{
		Name:       "Acceptance",
		Properties: map[string]any{"ServerPortNumber": "nope"},
	})
	if len(got) != 1 || got[0].RuleID != "MDL-SET01" {
		t.Fatalf("ValidateCreateConfiguration = %+v, want one MDL-SET01", got)
	}
	if !strings.Contains(got[0].Message, "Acceptance") {
		t.Errorf("message does not name the configuration: %q", got[0].Message)
	}
}

// TestTypedSettingsKeys_MatchExecutor is the drift guard for the hand-maintained
// typedSettingsKeys table: every entry must correspond to a property the executor
// actually parses, otherwise `mxcli check` and `mxcli exec` would disagree about
// what is valid.
func TestTypedSettingsKeys_MatchExecutor(t *testing.T) {
	// A value that parses as neither an integer nor a boolean.
	const bad = "definitely-not-typed"

	for section, keys := range typedSettingsKeys {
		for key, kind := range keys {
			t.Run(section+"/"+key, func(t *testing.T) {
				// The validator must flag it.
				if got := ValidateSettings(&ast.AlterSettingsStmt{
					Section:    section,
					ConfigName: "Default",
					Properties: map[string]any{key: bad},
				}); len(got) != 1 {
					t.Errorf("ValidateSettings did not flag %s.%s: %+v", section, key, got)
				}

				// And the executor must refuse to write it.
				wrote := false
				ctx, _ := newMockCtx(t, withBackend(settingsBackend(&wrote)))
				err := alterSettings(ctx, &ast.AlterSettingsStmt{
					Section:    section,
					ConfigName: "Default",
					Properties: map[string]any{key: bad},
				})
				if err == nil {
					t.Errorf("executor accepted an invalid %s.%s — table and executor disagree", section, key)
				}
				if wrote {
					t.Errorf("executor wrote settings despite an invalid %s.%s", section, key)
				}

				// The valid form for this kind must round-trip through both.
				good := "7"
				switch kind {
				case settingsKindBool:
					good = "true"
				case settingsKindDatabaseType:
					good = "PostgreSql"
				case settingsKindEnum:
					// The first member of the property's own enumeration; the
					// table has no single valid value across enum-typed keys.
					members := settingsEnumValues[section+"/"+key]
					if len(members) == 0 {
						t.Fatalf("%s/%s is settingsKindEnum but has no members in settingsEnumValues", section, key)
					}
					good = members[0]
				}
				if got := ValidateSettings(&ast.AlterSettingsStmt{
					Section:    section,
					ConfigName: "Default",
					Properties: map[string]any{key: good},
				}); len(got) != 0 {
					t.Errorf("ValidateSettings flagged a valid %s.%s=%q: %+v", section, key, good, got)
				}
				wrote = false
				ctx2, _ := newMockCtx(t, withBackend(settingsBackend(&wrote)))
				if err := alterSettings(ctx2, &ast.AlterSettingsStmt{
					Section:    section,
					ConfigName: "Default",
					Properties: map[string]any{key: good},
				}); err != nil {
					t.Errorf("executor rejected a valid %s.%s=%q: %v", section, key, good, err)
				}
			})
		}
	}
}
