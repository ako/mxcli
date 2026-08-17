// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/generated/metamodel"
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/settingsoverlay"
	"github.com/mendixlabs/mxcli/model"
)

// listSettings displays an overview table of all settings parts.
func listSettings(ctx *ExecContext) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	tr := &TableResult{
		Columns: []string{"Section", "Key Values"},
	}

	if ps.Model != nil {
		ms := ps.Model
		values := []string{}
		if ms.AfterStartupMicroflow != "" {
			values = append(values, "AfterStartup: "+ms.AfterStartupMicroflow)
		}
		values = append(values, "Hash: "+ms.HashAlgorithm)
		values = append(values, "Java: "+ms.JavaVersion)
		tr.Rows = append(tr.Rows, []any{"Model Settings", strings.Join(values, ", ")})
	}

	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			values := []string{}
			values = append(values, cfg.DatabaseType)
			// An empty DatabaseUrl used to render as a bare ", ," — a gap where
			// a value should be, which reads as a bug in the reader.
			if cfg.DatabaseUrl != "" {
				values = append(values, cfg.DatabaseUrl)
			}
			values = append(values, "db="+cfg.DatabaseName)
			values = append(values, fmt.Sprintf("http=%d", cfg.HttpPortNumber))
			// The root URL decides the host the app answers on, so "did my root
			// URL land?" is the obvious question to ask this command — and the
			// summary used to be unable to answer it (mxcli-formula1 #8).
			if cfg.ApplicationRootUrl != "" {
				values = append(values, "url="+cfg.ApplicationRootUrl)
			}
			if len(cfg.ConstantValues) > 0 {
				values = append(values, fmt.Sprintf("%d constants", len(cfg.ConstantValues)))
			}
			tr.Rows = append(tr.Rows, []any{
				fmt.Sprintf("Configuration '%s'", cfg.Name),
				strings.Join(values, ", "),
			})
		}
	}

	if ps.Language != nil {
		tr.Rows = append(tr.Rows, []any{"Language Settings", "Default: " + ps.Language.DefaultLanguageCode})
	}

	if ps.Workflows != nil {
		ws := ps.Workflows
		values := []string{}
		if ws.UserEntity != "" {
			values = append(values, "UserEntity: "+ws.UserEntity)
		}
		if ws.DefaultTaskParallelism > 0 {
			values = append(values, fmt.Sprintf("TaskParallelism: %d", ws.DefaultTaskParallelism))
		}
		tr.Rows = append(tr.Rows, []any{"Workflow Settings", strings.Join(values, ", ")})
	}

	if ps.Convention != nil {
		tr.Rows = append(tr.Rows, []any{"Convention Settings", "AssocStorage: " + ps.Convention.DefaultAssociationStorage})
	}

	if ps.WebUI != nil {
		tr.Rows = append(tr.Rows, []any{"Web UI Settings", "OptimizedClient: " + ps.WebUI.UseOptimizedClient})
	}

	return writeResult(ctx, tr)
}

// describeSettings outputs the full MDL description of all settings.
// describeSettings prints the project settings as re-executable `alter settings`
// statements. With configName set (DESCRIBE SETTINGS CONFIGURATION 'X') it
// prints only that configuration — the read form of `alter settings
// configuration 'X'`, which used to be a parse error.
func describeSettings(ctx *ExecContext, configName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	if configName != "" {
		return describeSettingsConfiguration(ctx, ps, configName)
	}

	// Model settings
	if ps.Model != nil {
		ms := ps.Model
		// Emit only the properties this project's Mendix version actually stores,
		// so the output replays: the executor refuses an ALTER naming one the
		// document does not carry (see refuseIfNotStored), and the versions differ
		// by five properties between 9.24 and 11.13.
		stored := storedModelSettings(ps)
		var parts []string
		// add emits a property only if this project's Mendix version stores it, so
		// the output replays: the executor refuses an ALTER naming a property the
		// document does not carry (refuseIfNotStored), and the set differs by five
		// properties between a blank 9.24 and a blank 11.13 project.
		add := func(key, format string, args ...any) {
			if stored != nil && !settingsoverlay.Has(stored, key) {
				return
			}
			parts = append(parts, "  "+fmt.Sprintf(format, args...))
		}
		// addIfSet additionally skips a property holding no value, so a blank
		// project's describe output is not a wall of empty strings.
		addIfSet := func(key, format string, v string) {
			if v != "" {
				add(key, format, v)
			}
		}

		addIfSet("AfterStartupMicroflow", "AfterStartupMicroflow = '%s'", ms.AfterStartupMicroflow)
		addIfSet("BeforeShutdownMicroflow", "BeforeShutdownMicroflow = '%s'", ms.BeforeShutdownMicroflow)
		addIfSet("HealthCheckMicroflow", "HealthCheckMicroflow = '%s'", ms.HealthCheckMicroflow)
		add("HashAlgorithm", "HashAlgorithm = '%s'", ms.HashAlgorithm)
		add("BcryptCost", "BcryptCost = %d", ms.BcryptCost)
		// JavaVersion is stored under either JavaVersion or JavaMajorVersion; emit it
		// when the document carries whichever spelling, in mxcli's single input name.
		if settingsoverlay.JavaVersionKey(stored) != "" || stored == nil {
			parts = append(parts, fmt.Sprintf("  JavaVersion = '%s'", ms.JavaVersion))
		}
		add("RoundingMode", "RoundingMode = '%s'", ms.RoundingMode)
		add("AllowUserMultipleSessions", "AllowUserMultipleSessions = %t", ms.AllowUserMultipleSessions)
		add("EnableDataStorageOptimisticLocking", "EnableDataStorageOptimisticLocking = %t", ms.EnableDataStorageOptimisticLocking)
		add("UseDatabaseForeignKeyConstraints", "UseDatabaseForeignKeyConstraints = %t", ms.UseDatabaseForeignKeyConstraints)
		add("UseOQLVersion2", "UseOQLVersion2 = %t", ms.UseOQLVersion2)
		add("DecimalScale", "DecimalScale = %d", ms.DecimalScale)
		addIfSet("FirstDayOfWeek", "FirstDayOfWeek = '%s'", ms.FirstDayOfWeek)
		addIfSet("SslCertificateAlgorithm", "SslCertificateAlgorithm = '%s'", ms.SslCertificateAlgorithm)
		addIfSet("ScheduledEventTimeZoneCode", "ScheduledEventTimeZoneCode = '%s'", ms.ScheduledEventTimeZoneCode)
		addIfSet("DefaultTimeZoneCode", "DefaultTimeZoneCode = '%s'", ms.DefaultTimeZoneCode)
		fmt.Fprintf(ctx.Output, "alter settings model\n%s;\n\n", strings.Join(parts, ",\n"))
	}

	// Configuration settings
	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			writeSettingsConfiguration(ctx, cfg)
		}
	}

	// Language settings
	if ps.Language != nil {
		fmt.Fprintf(ctx.Output, "alter settings LANGUAGE\n  DefaultLanguageCode = '%s';\n\n", ps.Language.DefaultLanguageCode)
	}

	// Workflow settings
	if ps.Workflows != nil {
		ws := ps.Workflows
		var parts []string
		if ws.UserEntity != "" {
			parts = append(parts, fmt.Sprintf("  UserEntity = '%s'", ws.UserEntity))
		}
		if ws.DefaultTaskParallelism > 0 {
			parts = append(parts, fmt.Sprintf("  DefaultTaskParallelism = %d", ws.DefaultTaskParallelism))
		}
		if ws.WorkflowEngineParallelism > 0 {
			parts = append(parts, fmt.Sprintf("  WorkflowEngineParallelism = %d", ws.WorkflowEngineParallelism))
		}
		if len(parts) > 0 {
			fmt.Fprintf(ctx.Output, "alter settings workflows\n%s;\n\n", strings.Join(parts, ",\n"))
		}
	}

	return nil
}

// modelSettingKeys names every property the ALTER SETTINGS MODEL switch below
// assigns, so an unknown key can be rejected with a list of what would have
// worked. A bare "unknown model setting" sent the mxcli-banking app through
// three wrong guesses at the optimistic-locking property without ever naming the
// real one.
//
// Hand-maintained alongside the switch: add a case, add it here.
// TestModelSettingKeys_AllAccepted fails when a listed key is not accepted.
//
// Deliberately absent: UseSystemContextForBackgroundTasks. The property is still
// stored, and mxcli reads and round-trips it, but Mendix has withdrawn it —
// `mx check` on 11.13 rejects a project holding true with
// CE9436 "The project setting 'System context tasks' is not supported anymore."
// Its only legal value on a current version is its default, so an ALTER for it
// could only ever produce a project that does not build.
var modelSettingKeys = []string{
	"AfterStartupMicroflow",
	"BeforeShutdownMicroflow",
	"HealthCheckMicroflow",
	"HashAlgorithm",
	"BcryptCost",
	"JavaVersion",
	"RoundingMode",
	"AllowUserMultipleSessions",
	"ScheduledEventTimeZoneCode",
	"DefaultTimeZoneCode",
	"FirstDayOfWeek",
	"DecimalScale",
	"EnableDataStorageOptimisticLocking",
	"UseDatabaseForeignKeyConstraints",
	"UseOQLVersion2",
	"SslCertificateAlgorithm",
}

// isKnownModelSetting reports whether ALTER SETTINGS MODEL has a case for a key.
// Kept beside modelSettingKeys so the membership test and the "valid keys" list
// in the error can never disagree.
func isKnownModelSetting(key string) bool {
	for _, k := range modelSettingKeys {
		if k == key {
			return true
		}
	}
	return false
}

// firstDayOfWeekValues and sslCertificateAlgorithmValues are the members of the
// corresponding Mendix enumerations as Studio Pro spells them in BSON. Passing an
// unrecognised string through is the mendixlabs/mxcli#759 shape: the metamodel
// cannot resolve it and Studio Pro throws "Sequence contains no matching element",
// while mxbuild loads the project fine.
//
// generated/metamodel is a snapshot of Mendix 11.6, so a member added in a later
// version would be rejected here. That is the safer direction — the error names
// what is accepted, so an over-strict refusal is obvious rather than silent.
var firstDayOfWeekValues = []string{
	string(metamodel.SettingsFirstDayOfWeekDefault),
	string(metamodel.SettingsFirstDayOfWeekMonday),
	string(metamodel.SettingsFirstDayOfWeekTuesday),
	string(metamodel.SettingsFirstDayOfWeekWednesday),
	string(metamodel.SettingsFirstDayOfWeekThursday),
	string(metamodel.SettingsFirstDayOfWeekFriday),
	string(metamodel.SettingsFirstDayOfWeekSaturday),
	string(metamodel.SettingsFirstDayOfWeekSunday),
}

var sslCertificateAlgorithmValues = []string{
	string(metamodel.SettingsSslCertificateAlgorithmPKIX),
	string(metamodel.SettingsSslCertificateAlgorithmSunX509),
}

// settingsEnumValues maps "<section>/<key>" to the enumeration members accepted for
// it, for the properties validated as settingsKindEnum.
var settingsEnumValues = map[string][]string{
	"model/FirstDayOfWeek":          firstDayOfWeekValues,
	"model/SslCertificateAlgorithm": sslCertificateAlgorithmValues,
}

// settingsEnum canonicalises an enumeration-typed settings value to the member
// Mendix stores, matching case-insensitively. Anything unrecognised is rejected
// rather than written through — same contract as settingsDatabaseType.
func settingsEnum(key, valStr string, members []string) (string, error) {
	want := strings.TrimSpace(valStr)
	for _, m := range members {
		if strings.EqualFold(m, want) {
			return m, nil
		}
	}
	return "", mdlerrors.NewValidationf("%s must be one of %s, got %q",
		key, strings.Join(members, ", "), valStr)
}

// storedModelSettings returns the raw Settings$ModelSettings part, so the handler
// can tell "this project does not store that property" from "that property is
// false". Returns nil when the part is not in RawParts.
func storedModelSettings(ps *model.ProjectSettings) map[string]any {
	for _, part := range ps.RawParts {
		if t, _ := part["$Type"].(string); t == "Settings$ModelSettings" {
			return part
		}
	}
	return nil
}

// refuseIfNotStored rejects an ALTER naming a property this project's Mendix
// version does not store. The overlay is presence-gated (it will not introduce a
// property the type may not define), so without this check the statement would
// report success and change nothing — the silent no-op shape of #805.
//
// JavaVersion is exempt: it is stored under either JavaVersion or
// JavaMajorVersion, and settingsoverlay.JavaVersionKey resolves which.
func refuseIfNotStored(raw map[string]any, key string) error {
	if raw == nil || key == "JavaVersion" {
		return nil
	}
	if settingsoverlay.Has(raw, key) {
		return nil
	}
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"this project does not store the model setting %s\n"+
			"  Mendix adds model settings over time — a blank 9.24 project stores 12 of them, "+
			"a blank 11.13 project stores 17.\n"+
			"  mxcli will not introduce a property the project's Mendix version may not define: "+
			"Studio Pro refuses to open a model carrying one, and mxbuild does not catch it.\n"+
			"  hint: set it in Studio Pro once, or upgrade the project", key))
}

// alterSettings modifies project settings based on ALTER SETTINGS statement.
func alterSettings(ctx *ExecContext, stmt *ast.AlterSettingsStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	section := strings.ToLower(stmt.Section)
	switch section {
	case "model":
		if ps.Model == nil {
			return mdlerrors.NewNotFound("settings section", "model")
		}
		storedModel := storedModelSettings(ps)
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			// Order matters: a key mxcli does not know at all is a different
			// problem from a real key this Mendix version does not store, and the
			// presence check cannot tell them apart. Running it first answered a
			// typo with "this project does not store NotARealSetting … upgrade the
			// project", sending the reader after a version problem that does not
			// exist instead of at the misspelling in front of them.
			if !isKnownModelSetting(key) {
				return mdlerrors.NewUnsupported(fmt.Sprintf(
					"unknown model setting: %s\n  valid keys: %s",
					key, strings.Join(modelSettingKeys, ", ")))
			}
			// The overlay will not introduce a property the stored document does
			// not carry, so refuse here rather than reporting a success that
			// changes nothing.
			if err := refuseIfNotStored(storedModel, key); err != nil {
				return err
			}
			switch key {
			case "AfterStartupMicroflow":
				ps.Model.AfterStartupMicroflow = valStr
			case "BeforeShutdownMicroflow":
				ps.Model.BeforeShutdownMicroflow = valStr
			case "HealthCheckMicroflow":
				ps.Model.HealthCheckMicroflow = valStr
			case "HashAlgorithm":
				ps.Model.HashAlgorithm = valStr
			case "BcryptCost":
				v, err := settingsInt(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.BcryptCost = v
			case "JavaVersion":
				ps.Model.JavaVersion = valStr
			case "RoundingMode":
				ps.Model.RoundingMode = valStr
			case "AllowUserMultipleSessions":
				v, err := settingsBool(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.AllowUserMultipleSessions = v
			case "ScheduledEventTimeZoneCode":
				ps.Model.ScheduledEventTimeZoneCode = valStr
			case "EnableDataStorageOptimisticLocking":
				// Mendix's App Settings → Runtime → "Optimistic locking". With it
				// on, the runtime tracks an MxObjectVersion per persistable entity
				// and a commit whose version no longer matches the database throws
				// ConcurrentModificationRuntimeException — the mitigation for a
				// read-then-write race (check balance, then write it) inside a
				// microflow, which the transaction alone does not make serialisable.
				// It detects rather than retries: the handler must catch, reload,
				// re-apply and re-commit.
				v, err := settingsBool(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.EnableDataStorageOptimisticLocking = v
			case "DefaultTimeZoneCode":
				ps.Model.DefaultTimeZoneCode = valStr
			case "FirstDayOfWeek":
				v, err := settingsEnum(key, valStr, firstDayOfWeekValues)
				if err != nil {
					return err
				}
				ps.Model.FirstDayOfWeek = v
			case "SslCertificateAlgorithm":
				v, err := settingsEnum(key, valStr, sslCertificateAlgorithmValues)
				if err != nil {
					return err
				}
				ps.Model.SslCertificateAlgorithm = v
			case "DecimalScale":
				v, err := settingsInt(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.DecimalScale = v
			case "UseDatabaseForeignKeyConstraints":
				v, err := settingsBool(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.UseDatabaseForeignKeyConstraints = v
			case "UseOQLVersion2":
				v, err := settingsBool(key, valStr)
				if err != nil {
					return err
				}
				ps.Model.UseOQLVersion2 = v
			default:
				return mdlerrors.NewUnsupported(fmt.Sprintf(
					"unknown model setting: %s\n  valid keys: %s",
					key, strings.Join(modelSettingKeys, ", ")))
			}
		}

	case "language":
		if ps.Language == nil {
			return mdlerrors.NewNotFound("settings section", "language")
		}
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			switch key {
			case "DefaultLanguageCode":
				if err := validateLanguageCode(ps.Language, valStr); err != nil {
					return err
				}
				ps.Language.DefaultLanguageCode = valStr
			default:
				return mdlerrors.NewUnsupported("unknown language setting: " + key)
			}
		}

	case "workflows":
		if ps.Workflows == nil {
			return mdlerrors.NewNotFound("settings section", "workflows")
		}
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			switch key {
			case "UserEntity":
				ps.Workflows.UserEntity = valStr
			case "DefaultTaskParallelism":
				v, err := settingsInt(key, valStr)
				if err != nil {
					return err
				}
				ps.Workflows.DefaultTaskParallelism = v
			case "WorkflowEngineParallelism":
				v, err := settingsInt(key, valStr)
				if err != nil {
					return err
				}
				ps.Workflows.WorkflowEngineParallelism = v
			default:
				return mdlerrors.NewUnsupported("unknown workflow setting: " + key)
			}
		}

	case "configuration":
		return alterSettingsConfiguration(ctx, ps, stmt)

	case "constant":
		return alterSettingsConstant(ctx, ps, stmt)

	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("unknown settings section: %s (expected model, configuration, constant, LANGUAGE, or workflows)", section))
	}

	// Write updated settings
	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Updated %s settings\n", section)
	return nil
}

// validateLanguageCode rejects a DefaultLanguageCode that is not one of the
// project's configured languages. Mendix has no such guard: `alter settings
// LANGUAGE` would accept e.g. 'nl_NL' on an en_US-only project, the write would
// report success, and the *next* `mx check` would die with an unhandled
// NullReferenceException rather than a model error (FINDINGS #6). Skipped when the
// project's language list is empty (unavailable) to avoid false rejections.
func validateLanguageCode(ls *model.LanguageSettings, code string) error {
	if ls == nil || len(ls.Languages) == 0 {
		return nil
	}
	avail := make([]string, 0, len(ls.Languages))
	for _, l := range ls.Languages {
		if l.Code == code {
			return nil
		}
		avail = append(avail, l.Code)
	}
	sort.Strings(avail)
	return mdlerrors.NewValidationf(
		"language %q is not configured in this project (available: %s) — add it in Studio Pro (Project ▸ Settings ▸ Languages) before making it the default",
		code, strings.Join(avail, ", "))
}

func alterSettingsConfiguration(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	// Find the named configuration
	var cfg *model.ServerConfiguration
	for _, c := range ps.Configuration.Configurations {
		if strings.EqualFold(c.Name, stmt.ConfigName) {
			cfg = c
			break
		}
	}
	if cfg == nil {
		return mdlerrors.NewNotFound("configuration", stmt.ConfigName)
	}

	for key, val := range stmt.Properties {
		valStr := settingsValueToString(val)
		switch key {
		case "DatabaseType":
			v, err := settingsDatabaseType(key, valStr)
			if err != nil {
				return err
			}
			cfg.DatabaseType = v
		case "DatabaseUrl":
			cfg.DatabaseUrl = valStr
		case "DatabaseName":
			cfg.DatabaseName = valStr
		case "DatabaseUserName":
			cfg.DatabaseUserName = valStr
		case "DatabasePassword":
			cfg.DatabasePassword = valStr
		case "HttpPortNumber":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			cfg.HttpPortNumber = v
		case "ServerPortNumber":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			cfg.ServerPortNumber = v
		case "ApplicationRootUrl":
			cfg.ApplicationRootUrl = valStr
		default:
			return mdlerrors.NewUnsupported("unknown configuration setting: " + key)
		}
	}

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Updated configuration '%s'\n", stmt.ConfigName)
	return nil
}

func alterSettingsConstant(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	// Find the target configuration
	targetConfig := stmt.ConfigName
	if targetConfig == "" {
		// Default to first configuration
		if len(ps.Configuration.Configurations) > 0 {
			targetConfig = ps.Configuration.Configurations[0].Name
		} else {
			return mdlerrors.NewValidation("no configurations found")
		}
	}

	var cfg *model.ServerConfiguration
	for _, c := range ps.Configuration.Configurations {
		if strings.EqualFold(c.Name, targetConfig) {
			cfg = c
			break
		}
	}
	if cfg == nil {
		return mdlerrors.NewNotFound("configuration", targetConfig)
	}

	if stmt.DropConstant {
		// Remove the constant override
		for i, cv := range cfg.ConstantValues {
			if cv.ConstantId == stmt.ConstantId {
				cfg.ConstantValues = append(cfg.ConstantValues[:i], cfg.ConstantValues[i+1:]...)
				if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
					return mdlerrors.NewBackend("update project settings", err)
				}
				fmt.Fprintf(ctx.Output, "Dropped constant '%s' from configuration '%s'\n",
					stmt.ConstantId, targetConfig)
				return nil
			}
		}
		return mdlerrors.NewNotFoundMsg("constant", stmt.ConstantId, fmt.Sprintf("constant '%s' not found in configuration '%s'", stmt.ConstantId, targetConfig))
	}

	// Find or create the constant value
	found := false
	for _, cv := range cfg.ConstantValues {
		if cv.ConstantId == stmt.ConstantId {
			// Setting a value on a private override would convert it to a shared one,
			// publishing into the shared model a value the developer chose to keep on
			// their workstation — and breaking their local binding. The shared/private
			// choice belongs to the constant, so refuse rather than flip it silently.
			if cv.IsPrivate {
				return mdlerrors.NewValidationf(
					"constant '%s' has a private value in configuration '%s'; "+
						"its value is stored on the developer's workstation, not in the shared model. "+
						"Change the constant to a shared value in Studio Pro first, "+
						"or use `alter settings drop constant '%s' in configuration '%s'` to remove the override",
					stmt.ConstantId, targetConfig, stmt.ConstantId, targetConfig)
			}
			cv.Value = stmt.Value
			found = true
			break
		}
	}
	if !found {
		cv := &model.ConstantValue{
			ConstantId: stmt.ConstantId,
			Value:      stmt.Value,
		}
		cv.TypeName = "Settings$ConstantValue"
		cfg.ConstantValues = append(cfg.ConstantValues, cv)
	}

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Updated constant '%s' = '%s' in configuration '%s'\n",
		stmt.ConstantId, stmt.Value, targetConfig)
	return nil
}

// createConfiguration handles CREATE CONFIGURATION 'name' [properties...].
func createConfiguration(ctx *ExecContext, stmt *ast.CreateConfigurationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	// Check if configuration already exists
	for _, cfg := range ps.Configuration.Configurations {
		if strings.EqualFold(cfg.Name, stmt.Name) {
			return mdlerrors.NewAlreadyExists("configuration", stmt.Name)
		}
	}

	// Mirror the configuration Studio Pro creates: the enum member "Hsqldb" (not
	// "HSQLDB", which no metamodel member matches) and both default ports, so a
	// fresh configuration is runnable and loads without repair (#759).
	newCfg := &model.ServerConfiguration{
		Name:             stmt.Name,
		DatabaseType:     string(metamodel.SettingsDatabaseTypeHsqldb),
		HttpPortNumber:   8080,
		ServerPortNumber: 8090,
		ConstantValues:   []*model.ConstantValue{},
	}
	newCfg.TypeName = "Settings$ServerConfiguration"

	// Apply optional properties
	for key, val := range stmt.Properties {
		valStr := settingsValueToString(val)
		switch key {
		case "DatabaseType":
			v, err := settingsDatabaseType(key, valStr)
			if err != nil {
				return err
			}
			newCfg.DatabaseType = v
		case "DatabaseUrl":
			newCfg.DatabaseUrl = valStr
		case "DatabaseName":
			newCfg.DatabaseName = valStr
		case "DatabaseUserName":
			newCfg.DatabaseUserName = valStr
		case "DatabasePassword":
			newCfg.DatabasePassword = valStr
		case "HttpPortNumber":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			newCfg.HttpPortNumber = v
		case "ServerPortNumber":
			v, err := settingsInt(key, valStr)
			if err != nil {
				return err
			}
			newCfg.ServerPortNumber = v
		case "ApplicationRootUrl":
			newCfg.ApplicationRootUrl = valStr
		default:
			return mdlerrors.NewUnsupported("unknown configuration property: " + key)
		}
	}

	ps.Configuration.Configurations = append(ps.Configuration.Configurations, newCfg)

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Created configuration: %s\n", stmt.Name)
	return nil
}

// dropConfiguration handles DROP CONFIGURATION 'name'.
func dropConfiguration(ctx *ExecContext, stmt *ast.DropConfigurationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	for i, cfg := range ps.Configuration.Configurations {
		if strings.EqualFold(cfg.Name, stmt.Name) {
			ps.Configuration.Configurations = append(
				ps.Configuration.Configurations[:i],
				ps.Configuration.Configurations[i+1:]...,
			)
			if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
				return mdlerrors.NewBackend("update project settings", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped configuration: %s\n", stmt.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("configuration", stmt.Name)
}

// settingsInt parses an Integer-typed settings value. The error used to be
// discarded (`if v, err := strconv.Atoi(...); err == nil`), so a non-numeric value
// skipped the assignment while the handler still printed its success line — the
// field silently kept its old value (#805).
func settingsInt(key, valStr string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(valStr))
	if err != nil {
		return 0, mdlerrors.NewValidationf("%s must be an integer, got %q", key, valStr)
	}
	return v, nil
}

// settingsBool parses a Boolean-typed settings value. Comparing against "true"
// turned every other spelling — a typo, or a plausible value like 'yes' — into
// false while still reporting success, the same silent no-op as settingsInt.
func settingsBool(key, valStr string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(valStr)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, mdlerrors.NewValidationf("%s must be true or false, got %q", key, valStr)
}

// databaseTypes lists the members of the Mendix Settings.DatabaseType enumeration
// exactly as Studio Pro spells them in BSON (generated/metamodel.SettingsDatabaseType).
// A configuration stored with anything else — mxcli hardcoded "HSQLDB" for every
// CREATE CONFIGURATION — is a value the metamodel cannot resolve, and Studio Pro
// throws "Sequence contains no matching element" when it loads the configuration
// (mendixlabs/mxcli#759).
var databaseTypes = []string{
	string(metamodel.SettingsDatabaseTypeDb2),
	string(metamodel.SettingsDatabaseTypeHsqldb),
	string(metamodel.SettingsDatabaseTypeMySql),
	string(metamodel.SettingsDatabaseTypeOracle),
	string(metamodel.SettingsDatabaseTypePostgreSql),
	string(metamodel.SettingsDatabaseTypeSapHana),
	string(metamodel.SettingsDatabaseTypeSqlServer),
}

// settingsDatabaseType canonicalises a DatabaseType value to the enum member
// Mendix stores, matching case-insensitively so 'postgresql' and 'PostgreSql' both
// land on the stored spelling. Anything unrecognised is rejected rather than
// written through.
func settingsDatabaseType(key, valStr string) (string, error) {
	want := strings.TrimSpace(valStr)
	for _, dt := range databaseTypes {
		if strings.EqualFold(dt, want) {
			return dt, nil
		}
	}
	return "", mdlerrors.NewValidationf("%s must be one of %s, got %q",
		key, strings.Join(databaseTypes, ", "), valStr)
}

// settingsValueToString converts an AST settings value to string.
func settingsValueToString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// writeSettingsConfiguration emits one configuration as re-executable MDL.
func writeSettingsConfiguration(ctx *ExecContext, cfg *model.ServerConfiguration) {
	var parts []string
	parts = append(parts, fmt.Sprintf("  DatabaseType = '%s'", cfg.DatabaseType))
	parts = append(parts, fmt.Sprintf("  DatabaseUrl = '%s'", cfg.DatabaseUrl))
	parts = append(parts, fmt.Sprintf("  DatabaseName = '%s'", cfg.DatabaseName))
	parts = append(parts, fmt.Sprintf("  DatabaseUserName = '%s'", cfg.DatabaseUserName))
	parts = append(parts, fmt.Sprintf("  DatabasePassword = '%s'", cfg.DatabasePassword))
	parts = append(parts, fmt.Sprintf("  HttpPortNumber = %d", cfg.HttpPortNumber))
	parts = append(parts, fmt.Sprintf("  ServerPortNumber = %d", cfg.ServerPortNumber))
	if cfg.ApplicationRootUrl != "" {
		parts = append(parts, fmt.Sprintf("  ApplicationRootUrl = '%s'", cfg.ApplicationRootUrl))
	}
	fmt.Fprintf(ctx.Output, "alter settings configuration '%s'\n%s;\n\n", cfg.Name, strings.Join(parts, ",\n"))

	// Output constant overrides. A private override has no value in the
	// model — emitting `value ''` would round-trip into a *shared* empty
	// override, moving a value that is deliberately kept off the shared model
	// into it. MDL does not author the shared/private choice, so describe
	// reports it as a comment instead of a re-executable statement.
	for _, cv := range cfg.ConstantValues {
		if cv.IsPrivate {
			fmt.Fprintf(ctx.Output, "-- constant '%s' has a private value in configuration '%s'\n"+
				"-- (stored on the developer's workstation; not part of the shared model)\n\n",
				cv.ConstantId, cfg.Name)
			continue
		}
		fmt.Fprintf(ctx.Output, "alter settings constant '%s' value '%s'\n  in configuration '%s';\n\n",
			cv.ConstantId, cv.Value, cfg.Name)
	}
}

// describeSettingsConfiguration prints a single named configuration, or names
// the ones that exist when the requested name is not among them.
func describeSettingsConfiguration(ctx *ExecContext, ps *model.ProjectSettings, name string) error {
	var available []string
	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			if strings.EqualFold(cfg.Name, name) {
				writeSettingsConfiguration(ctx, cfg)
				return nil
			}
			available = append(available, "'"+cfg.Name+"'")
		}
	}
	return mdlerrors.NewNotFoundMsg("settings configuration", name,
		fmt.Sprintf("settings configuration not found: '%s' (available: %s)", name, strings.Join(available, ", ")))
}
