// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"strings"
)

// Canonical database types a local run understands. The runtime's own enum also
// has Db2/MySql/Oracle/SapHana/SqlServer; those all need a server and are out of
// scope for `run --local`.
const (
	DBTypePostgreSQL = "postgresql"
	DBTypeHSQLDB     = "hsqldb"
)

// NormalizeDBType maps a user-supplied --db-type to a canonical value. Empty
// means PostgreSQL, which is what a local run has always used, so the flag stays
// backwards compatible.
func NormalizeDBType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", DBTypePostgreSQL, "postgres":
		return DBTypePostgreSQL, nil
	case DBTypeHSQLDB:
		return DBTypeHSQLDB, nil
	default:
		return "", fmt.Errorf("unknown --db-type %q (want postgresql or hsqldb)", raw)
	}
}

// IsFileBasedDBType reports whether a canonical type is the runtime's built-in
// file database, which has no host to reach and no credentials.
func IsFileBasedDBType(canonical string) bool {
	return canonical == DBTypeHSQLDB
}

// RuntimeDatabaseType maps a canonical type to the spelling the runtime's
// DatabaseType parameter expects (the runtime enum is upper case).
func RuntimeDatabaseType(canonical string) string {
	if canonical == DBTypeHSQLDB {
		return "HSQLDB"
	}
	return "PostgreSQL"
}
