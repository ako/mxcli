// SPDX-License-Identifier: Apache-2.0

package settingsoverlay

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// mendix924Part is the Settings$ModelSettings a blank Mendix 9.24 project stores,
// key-for-key. It carries 12 properties; a blank 11.13 project carries 17. The
// five it lacks — DecimalScale, JavaMajorVersion, SslCertificateAlgorithm,
// UseDatabaseForeignKeyConstraints, UseOQLVersion2 — are the ones this test is
// about.
func mendix924Part() map[string]any {
	return map[string]any{
		"$Type":                              "Settings$ModelSettings",
		"AfterStartupMicroflow":              "",
		"AllowUserMultipleSessions":          true,
		"BcryptCost":                         int64(10),
		"BeforeShutdownMicroflow":            "",
		"DefaultTimeZoneCode":                "",
		"EnableDataStorageOptimisticLocking": false,
		"FirstDayOfWeek":                     "Default",
		"HashAlgorithm":                      "BCrypt",
		"HealthCheckMicroflow":               "",
		"RoundingMode":                       "HalfUp",
		"ScheduledEventTimeZoneCode":         "Etc/UTC",
		"UseSystemContextForBackgroundTasks": false,
	}
}

// TestSetModelSettings_DoesNotIntroduceAbsentProperties is the regression test for
// the leak found while triaging the mxcli-banking findings: writing any model
// setting to a Mendix 9.24 project also wrote DecimalScale = 0 and
// UseDatabaseForeignKeyConstraints = false, neither of which that version stores.
//
// Both were wrong as values too — 11.13 defaults them to 8 and true — so one
// unrelated statement silently changed two other settings. And a property the
// type may not define is the mendixlabs/mxcli#759 shape: Studio Pro throws
// "Sequence contains no matching element" while mxbuild loads the model happily,
// so the build is not a safety net.
func TestSetModelSettings_DoesNotIntroduceAbsentProperties(t *testing.T) {
	raw := mendix924Part()
	before := len(raw)

	// A model carrying values for every property mxcli knows, as the reader would
	// produce it: the absent ones hold their parse defaults.
	ms := &model.ModelSettings{
		HashAlgorithm:                    "BCrypt",
		BcryptCost:                       11,
		DecimalScale:                     0,
		UseDatabaseForeignKeyConstraints: true,
		UseOQLVersion2:                   true,
		SslCertificateAlgorithm:          "",
		JavaVersion:                      "Java21",
	}
	SetModelSettings(ms, raw)

	for _, key := range []string{
		"DecimalScale",
		"UseDatabaseForeignKeyConstraints",
		"UseOQLVersion2",
		"SslCertificateAlgorithm",
		"JavaVersion",
		"JavaMajorVersion",
	} {
		if v, has := raw[key]; has {
			t.Errorf("introduced %s = %#v into a document that did not carry it", key, v)
		}
	}
	if len(raw) != before {
		t.Errorf("property count %d -> %d; the overlay must not add or remove keys", before, len(raw))
	}
}

// The gate must not become a blanket refusal to write: a property the document
// does carry is still updated.
func TestSetModelSettings_UpdatesPresentProperties(t *testing.T) {
	raw := mendix924Part()
	ms := &model.ModelSettings{
		HashAlgorithm:                      "SHA256",
		BcryptCost:                         13,
		AllowUserMultipleSessions:          false,
		EnableDataStorageOptimisticLocking: true,
		FirstDayOfWeek:                     "Monday",
		UseSystemContextForBackgroundTasks: true,
	}
	SetModelSettings(ms, raw)

	for key, want := range map[string]any{
		"HashAlgorithm":                      "SHA256",
		"BcryptCost":                         int64(13),
		"AllowUserMultipleSessions":          false,
		"EnableDataStorageOptimisticLocking": true,
		"FirstDayOfWeek":                     "Monday",
		"UseSystemContextForBackgroundTasks": true,
	} {
		if raw[key] != want {
			t.Errorf("%s = %#v, want %#v", key, raw[key], want)
		}
	}
}

// An 11.13 document carries the whole set, so every property is writable there.
func TestSetModelSettings_WritesFullPropertySetWhenStored(t *testing.T) {
	raw := mendix924Part()
	raw["DecimalScale"] = int64(8)
	raw["JavaMajorVersion"] = "21"
	raw["SslCertificateAlgorithm"] = "PKIX"
	raw["UseDatabaseForeignKeyConstraints"] = true
	raw["UseOQLVersion2"] = true

	ms := &model.ModelSettings{
		DecimalScale:                     4,
		JavaVersion:                      "Java21",
		SslCertificateAlgorithm:          "SunX509",
		UseDatabaseForeignKeyConstraints: false,
		UseOQLVersion2:                   false,
	}
	SetModelSettings(ms, raw)

	for key, want := range map[string]any{
		"DecimalScale":                     int64(4),
		"SslCertificateAlgorithm":          "SunX509",
		"UseDatabaseForeignKeyConstraints": false,
		"UseOQLVersion2":                   false,
		// Written back under the key the document uses, in that key's value format.
		"JavaMajorVersion": "21",
	} {
		if raw[key] != want {
			t.Errorf("%s = %#v, want %#v", key, raw[key], want)
		}
	}
	if _, has := raw["JavaVersion"]; has {
		t.Error("wrote the 11.6 JavaVersion spelling into a document using JavaMajorVersion")
	}
}

// ModelSettingsKeys is hand-maintained beside the overlay; a name in it that the
// overlay does not actually write would misdescribe what mxcli round-trips.
func TestModelSettingsKeys_AllWritten(t *testing.T) {
	for _, key := range ModelSettingsKeys {
		t.Run(key, func(t *testing.T) {
			// Seed the document with the key so the presence gate lets it through.
			raw := map[string]any{"$Type": "Settings$ModelSettings"}
			stored := key
			if key == "JavaVersion" {
				stored = JavaMajorVersionKey
			}
			raw[stored] = "sentinel"

			SetModelSettings(&model.ModelSettings{JavaVersion: "Java21"}, raw)

			if raw[stored] == "sentinel" {
				t.Errorf("%s is listed in ModelSettingsKeys but the overlay never writes it", key)
			}
		})
	}
}
