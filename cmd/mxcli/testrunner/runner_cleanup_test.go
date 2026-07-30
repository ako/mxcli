// SPDX-License-Identifier: Apache-2.0

// Regression tests for the three `mxcli test` cleanup defects:
//
//   - #803 the after-startup value was mis-parsed, so restoring it produced
//     unparseable MDL, the failure was printed as a warning, and the project was
//     left with its after-startup pointing at the microflow cleanup then deleted
//   - #804 cleanup dropped only the microflow, leaving an empty MxTest module
//   - #802 the Security Level was forced OFF and restored to a hardcoded
//     PRODUCTION, regardless of what the project actually used
package testrunner

import (
	"strings"
	"testing"
)

// TestParseSettingValue covers the lines DESCRIBE SETTINGS actually emits.
// Properties are comma-separated and the statement ends with a semicolon, so the
// trailing punctuation has to come off before the quotes — trimming quotes first
// stops at the comma and leaves the punctuation inside the value (#803).
func TestParseSettingValue(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "trailing comma (the reported case)",
			line: "  AfterStartupMicroflow = 'MyFirstModule.ASU_Startup',",
			want: "MyFirstModule.ASU_Startup",
		},
		{
			name: "trailing semicolon (last property in the statement)",
			line: "  AfterStartupMicroflow = 'MyFirstModule.ASU_Startup';",
			want: "MyFirstModule.ASU_Startup",
		},
		{
			name: "no trailing punctuation",
			line: "  AfterStartupMicroflow = 'MyFirstModule.ASU_Startup'",
			want: "MyFirstModule.ASU_Startup",
		},
		{
			name: "double quotes",
			line: `  AfterStartupMicroflow = "MyFirstModule.ASU_Startup",`,
			want: "MyFirstModule.ASU_Startup",
		},
		{
			name: "empty value",
			line: "  AfterStartupMicroflow = '',",
			want: "",
		},
		{
			name: "value containing an equals sign is not truncated",
			line: "  SomeSetting = 'a=b',",
			want: "a=b",
		},
		{
			name: "no equals sign at all",
			line: "  AfterStartupMicroflow",
			want: "AfterStartupMicroflow",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseSettingValue(tc.line); got != tc.want {
				t.Errorf("parseSettingValue(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// TestParseSettingValue_RoundTripsThroughQuoting is the property that actually
// broke: whatever is parsed out must go back in as a well-formed MDL literal.
func TestParseSettingValue_RoundTripsThroughQuoting(t *testing.T) {
	for _, line := range []string{
		"  AfterStartupMicroflow = 'MyFirstModule.ASU_Startup',",
		"  AfterStartupMicroflow = 'MyFirstModule.ASU_Startup';",
		"  AfterStartupMicroflow = 'Mod.Flow'",
	} {
		got := quoteMDLString(parseSettingValue(line))
		if strings.Count(got, "'") != 2 {
			t.Errorf("re-quoting %q produced %q — not a single well-formed literal", line, got)
		}
		if strings.HasSuffix(got, ",'") || strings.HasSuffix(got, ";'") {
			t.Errorf("punctuation leaked into the value: %q", got)
		}
	}
}

func TestQuoteMDLString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Mod.Flow", "'Mod.Flow'"},
		{"", "''"},
		// Mendix escapes an embedded quote by doubling it, never with a backslash.
		{"it's", "'it''s'"},
		{"a'b'c", "'a''b''c'"},
	}
	for _, tc := range tests {
		if got := quoteMDLString(tc.in); got != tc.want {
			t.Errorf("quoteMDLString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestNoSecurityLevelManipulation pins #802: the Security Level is the project's
// business. Neither setup nor cleanup may touch it.
func TestNoSecurityLevelManipulation(t *testing.T) {
	all := append(setupCommands(), cleanupCommands(projectState{}, true)...)
	all = append(all, cleanupCommands(projectState{afterStartup: "Mod.Flow", createdMxTest: true}, true)...)
	for _, cmd := range all {
		if strings.Contains(strings.ToUpper(cmd), "SECURITY LEVEL") {
			t.Errorf("the runner still alters the project Security Level: %q (#802)", cmd)
		}
	}
}

func TestCleanupCommands(t *testing.T) {
	tests := []struct {
		name    string
		state   projectState
		present bool
		want    []string
	}{
		{
			name:    "restores an existing after-startup and drops the module it created",
			state:   projectState{afterStartup: "MyFirstModule.ASU_Startup", createdMxTest: true},
			present: true,
			want: []string{
				"ALTER SETTINGS MODEL AfterStartupMicroflow = 'MyFirstModule.ASU_Startup'",
				"DROP MODULE MxTest",
			},
		},
		{
			name:    "clears after-startup when the project had none",
			state:   projectState{createdMxTest: true},
			present: true,
			want: []string{
				"ALTER SETTINGS MODEL AfterStartupMicroflow = ''",
				"DROP MODULE MxTest",
			},
		},
		{
			// A pre-existing MxTest module belongs to the user: only the generated
			// microflow may be removed, or the run destroys their work.
			name:    "keeps a pre-existing MxTest module",
			state:   projectState{afterStartup: "Mod.Flow"},
			present: true,
			want: []string{
				"ALTER SETTINGS MODEL AfterStartupMicroflow = 'Mod.Flow'",
				"DROP MICROFLOW MxTest.TestRunner",
			},
		},
		{
			// The injection never landed: restore the setting, drop nothing, and do
			// not report a cleanup failure for a module that was never created.
			name:    "nothing to drop when the module is absent",
			state:   projectState{afterStartup: "Mod.Flow", createdMxTest: true},
			present: false,
			want: []string{
				"ALTER SETTINGS MODEL AfterStartupMicroflow = 'Mod.Flow'",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanupCommands(tc.state, tc.present)
			if len(got) != len(tc.want) {
				t.Fatalf("cleanupCommands = %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("command %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestCleanupCommands_RestoreIsWellFormed is the end of the #803 chain: whatever
// DESCRIBE SETTINGS produced must come back as a parseable statement.
func TestCleanupCommands_RestoreIsWellFormed(t *testing.T) {
	parsed := parseSettingValue("  AfterStartupMicroflow = 'MyFirstModule.ASU_Startup',")
	restore := cleanupCommands(projectState{afterStartup: parsed}, true)[0]
	want := "ALTER SETTINGS MODEL AfterStartupMicroflow = 'MyFirstModule.ASU_Startup'"
	if restore != want {
		t.Errorf("restore command = %q, want %q", restore, want)
	}
	if strings.Count(restore, "'") != 2 {
		t.Errorf("restore command is not a single well-formed literal: %q", restore)
	}
}
