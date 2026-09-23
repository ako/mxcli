// SPDX-License-Identifier: Apache-2.0

package docker

import "testing"

func TestNormalizeDBType(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", DBTypePostgreSQL, false},
		{"postgresql", DBTypePostgreSQL, false},
		{"PostgreSQL", DBTypePostgreSQL, false},
		{"postgres", DBTypePostgreSQL, false},
		{"hsqldb", DBTypeHSQLDB, false},
		{"HSQLDB", DBTypeHSQLDB, false},
		{" Hsqldb ", DBTypeHSQLDB, false},
		{"mysql", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeDBType(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormalizeDBType(%q) = %q, want error", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("NormalizeDBType(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestRuntimeDatabaseType(t *testing.T) {
	if got := RuntimeDatabaseType(DBTypeHSQLDB); got != "HSQLDB" {
		t.Errorf("RuntimeDatabaseType(hsqldb) = %q, want HSQLDB", got)
	}
	if got := RuntimeDatabaseType(DBTypePostgreSQL); got != "PostgreSQL" {
		t.Errorf("RuntimeDatabaseType(postgresql) = %q, want PostgreSQL", got)
	}
}
