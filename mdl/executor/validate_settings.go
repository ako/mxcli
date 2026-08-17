// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// settingsValueKind classifies the settings properties whose value the executor
// parses rather than storing verbatim. Anything unlisted is a string as written.
type settingsValueKind int

const (
	settingsKindInt settingsValueKind = iota
	settingsKindBool
	settingsKindDatabaseType
	// settingsKindEnum covers any other enumeration-typed property; its accepted
	// members are looked up per section/key in settingsEnumValues.
	settingsKindEnum
)

// typedSettingsKeys maps a lower-cased ALTER SETTINGS section to the properties
// whose value must parse as something other than a string. It mirrors the
// assignment switches in cmd_settings.go; TestTypedSettingsKeys_MatchExecutor
// fails if the two drift apart.
var typedSettingsKeys = map[string]map[string]settingsValueKind{
	"model": {
		"BcryptCost":                         settingsKindInt,
		"DecimalScale":                       settingsKindInt,
		"AllowUserMultipleSessions":          settingsKindBool,
		"EnableDataStorageOptimisticLocking": settingsKindBool,
		"UseDatabaseForeignKeyConstraints":   settingsKindBool,
		"UseOQLVersion2":                     settingsKindBool,
		"FirstDayOfWeek":                     settingsKindEnum,
		"SslCertificateAlgorithm":            settingsKindEnum,
	},
	"workflows": {
		"DefaultTaskParallelism":    settingsKindInt,
		"WorkflowEngineParallelism": settingsKindInt,
	},
	"configuration": {
		"HttpPortNumber":   settingsKindInt,
		"ServerPortNumber": settingsKindInt,
		"DatabaseType":     settingsKindDatabaseType,
	},
}

// ValidateSettings reports ALTER SETTINGS values that will not parse as their
// property's type. The executor rejects them too, but reporting them at check time
// means a typo surfaces before the project is opened for writing (#805).
func ValidateSettings(stmt *ast.AlterSettingsStmt) []linter.Violation {
	keys, ok := typedSettingsKeys[strings.ToLower(stmt.Section)]
	if !ok {
		return nil
	}
	return validateTypedSettings(strings.ToLower(stmt.Section), keys, stmt.Properties,
		"alter settings "+strings.ToLower(stmt.Section))
}

// ValidateCreateConfiguration reports the same for CREATE CONFIGURATION, which
// accepts the configuration properties directly.
func ValidateCreateConfiguration(stmt *ast.CreateConfigurationStmt) []linter.Violation {
	return validateTypedSettings("configuration", typedSettingsKeys["configuration"], stmt.Properties,
		fmt.Sprintf("create configuration '%s'", stmt.Name))
}

func validateTypedSettings(section string, keys map[string]settingsValueKind, props map[string]any, what string) []linter.Violation {
	if len(keys) == 0 || len(props) == 0 {
		return nil
	}
	// Iterate the properties in a stable order: a map would make the diagnostics
	// order (and so the check output) non-deterministic.
	names := make([]string, 0, len(props))
	for key := range props {
		names = append(names, key)
	}
	sort.Strings(names)

	loc := linter.Location{DocumentType: "settings", DocumentName: what}
	var out []linter.Violation
	for _, key := range names {
		kind, ok := keys[key]
		if !ok {
			continue
		}
		valStr := settingsValueToString(props[key])
		switch kind {
		case settingsKindInt:
			if _, err := settingsInt(key, valStr); err != nil {
				out = append(out, linter.Violation{
					RuleID:     "MDL-SET01",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    fmt.Sprintf("%s: %s must be an integer, got %q", what, key, valStr),
					Suggestion: fmt.Sprintf("Use a whole number, quoted or not: `%s = 10` or `%s = '10'`.", key, key),
				})
			}
		case settingsKindBool:
			if _, err := settingsBool(key, valStr); err != nil {
				out = append(out, linter.Violation{
					RuleID:     "MDL-SET02",
					Severity:   linter.SeverityError,
					Location:   loc,
					Message:    fmt.Sprintf("%s: %s must be true or false, got %q", what, key, valStr),
					Suggestion: fmt.Sprintf("Use `%s = true` or `%s = false`.", key, key),
				})
			}
		case settingsKindDatabaseType:
			if _, err := settingsDatabaseType(key, valStr); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-SET03",
					Severity: linter.SeverityError,
					Location: loc,
					Message: fmt.Sprintf("%s: %s must be a Mendix database type, got %q",
						what, key, valStr),
					Suggestion: fmt.Sprintf("Use one of: %s.", strings.Join(databaseTypes, ", ")),
				})
			}
		case settingsKindEnum:
			members := settingsEnumValues[section+"/"+key]
			if _, err := settingsEnum(key, valStr, members); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-SET03",
					Severity: linter.SeverityError,
					Location: loc,
					Message: fmt.Sprintf("%s: %s must be one of %s, got %q",
						what, key, strings.Join(members, ", "), valStr),
					Suggestion: fmt.Sprintf("Use one of: %s.", strings.Join(members, ", ")),
				})
			}
		}
	}
	return out
}
