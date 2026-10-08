// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"strings"
	"testing"
)

// The headless boot (`mxcli test --local`) filled PostgreSQL's host into every
// config, so a file database got "127.0.0.1:5432" and was then pinged there:
// "database not reachable at 127.0.0.1:5432" with no server needed at all.
// mendixlabs/mxcli#1274.
func TestLocalAppOptions_FileDatabaseGetsNoServerDefaults(t *testing.T) {
	o := LocalAppOptions{ProjectPath: "/proj/App.mpr", DB: DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB), Name: "app_test"}}
	o.applyDefaults()
	if o.DB.Host != "" || o.DB.User != "" || o.DB.Password != "" {
		t.Errorf("DB = %+v, want no host or credentials for the built-in file database", o.DB)
	}
	if o.DB.Name != "app_test" {
		t.Errorf("DB.Name = %q, want the caller's %q kept", o.DB.Name, "app_test")
	}
}

func TestLocalAppOptions_PostgreSQLDefaultsUnchanged(t *testing.T) {
	o := LocalAppOptions{ProjectPath: "/proj/App.mpr"}
	o.applyDefaults()
	if o.DB.Type != "PostgreSQL" || o.DB.Host != "127.0.0.1:5432" || o.DB.User != "mendix" || o.DB.Name != "app" {
		t.Errorf("DB = %+v, want the PostgreSQL defaults", o.DB)
	}
}

func TestPrepareLocalAppDatabase_FileDatabaseNeedsNoServer(t *testing.T) {
	o := LocalAppOptions{ProjectPath: "/proj/App.mpr", DB: DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB)}}
	o.applyDefaults()
	if err := prepareLocalAppDatabase(&o, io.Discard); err != nil {
		t.Fatalf("prepareLocalAppDatabase: %v — the file database has no server to reach", err)
	}
}

// EnsureDB provisions PostgreSQL. Reaching it with a file database is a caller
// bug, and running it would fail with the very error --db-type hsqldb avoids.
func TestPrepareLocalAppDatabase_RefusesEnsureDBForFileDatabase(t *testing.T) {
	o := LocalAppOptions{ProjectPath: "/proj/App.mpr", EnsureDB: true, DB: DBConfig{Type: RuntimeDatabaseType(DBTypeHSQLDB)}}
	o.applyDefaults()
	// Checked by message, not by err != nil: EnsureDatabase itself fails on a
	// host without PostgreSQL, which would pass this test for the wrong reason.
	err := prepareLocalAppDatabase(&o, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "file database") {
		t.Fatalf("prepareLocalAppDatabase err = %v, want a refusal naming the file database", err)
	}
}
