# run --local with the built-in HSQLDB database — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `mxcli run --local --db-type hsqldb` boot an app against Mendix's built-in, file-based HSQLDB database, so no external database server is needed.

**Architecture:** A new `--db-type` flag feeds `DBConfig.Type`. A new `applyDatabaseDefaults()` validates the type and fills type-appropriate connection settings (PostgreSQL keeps today's defaults; HSQLDB clears host/user/password). The pre-boot reachability check is skipped for file-based databases. The runtime builds the HSQLDB file URL itself from `DatabaseType` + `DatabaseName`.

**Tech Stack:** Go 1.26, cobra CLI, Mendix standalone runtime 11.12.2 (HSQLDB 2.7.4 bundled).

**Spec:** `docs/11-proposals/PROPOSAL_run_local_builtin_database.md`

**Branch:** `feat/run-local-hsqldb` (based on `fix/windows-local-run-process-helpers` / PR #1147).

---

## File Structure

| File | Responsibility |
|---|---|
| `cmd/mxcli/docker/dbtype.go` (new) | Canonical DB-type values + `NormalizeDBType` / `IsFileBasedDBType` / `RuntimeDatabaseType`. |
| `cmd/mxcli/docker/dbtype_test.go` (new) | Unit tests for the above. |
| `cmd/mxcli/docker/runlocal.go` | Move DB defaults out of `applyDefaults` into `applyDatabaseDefaults() error`; call it from `RunLocal`; skip TCP check for file-based DBs. |
| `cmd/mxcli/docker/runlocal_test.go` | Update two existing tests; add HSQLDB default/validation tests. |
| `cmd/mxcli/docker/localboot.go` | Add `DBConfig.IsFileBased()`. |
| `cmd/mxcli/docker/localboot_test.go` | Add `TestRuntimeConfigParams_HSQLDB`. |
| `cmd/mxcli/cmd_run.go` | Register `--db-type`, pass it into `DBConfig.Type`, document it. |
| `cmd/mxcli/docker/runlocal_hsqldb_integration_test.go` (new) | `//go:build integration`: boot with HSQLDB, assert HTTP 200 and the files exist. |
| `CHANGELOG.md` | `[Unreleased] / Added` entry. |

---

### Task 1: DB-type helpers

**Files:**
- Create: `cmd/mxcli/docker/dbtype.go`
- Test: `cmd/mxcli/docker/dbtype_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/mxcli/docker/dbtype_test.go`:

```go
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
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `go test -run 'TestNormalizeDBType|TestRuntimeDatabaseType' ./cmd/mxcli/docker/`
Expected: FAIL — `undefined: DBTypePostgreSQL` (and friends).

- [ ] **Step 3: Write the implementation**

Create `cmd/mxcli/docker/dbtype.go`:

```go
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
```

- [ ] **Step 4: Run the test and confirm it passes**

Run: `go test -run 'TestNormalizeDBType|TestRuntimeDatabaseType' ./cmd/mxcli/docker/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/mxcli/docker/dbtype.go cmd/mxcli/docker/dbtype_test.go
git commit -m "feat: add db-type normalization helpers for run --local"
```

---

### Task 2: Type-aware database defaults

**Files:**
- Modify: `cmd/mxcli/docker/runlocal.go` (`applyDefaults` at ~166-200, `RunLocal` at ~542)
- Test: `cmd/mxcli/docker/runlocal_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/mxcli/docker/runlocal_test.go`:

```go
func TestLocalRunOptions_DatabaseDefaults_HSQLDB(t *testing.T) {
	o := LocalRunOptions{ProjectPath: "/proj/App1112.mpr", DB: DBConfig{Type: "hsqldb"}}
	o.applyDefaults()
	if err := o.applyDatabaseDefaults(); err != nil {
		t.Fatalf("applyDatabaseDefaults: %v", err)
	}
	if o.DB.Type != "HSQLDB" {
		t.Errorf("Type = %q, want HSQLDB", o.DB.Type)
	}
	if o.DB.Host != "" || o.DB.User != "" || o.DB.Password != "" {
		t.Errorf("HSQLDB must carry no host/credentials, got %+v", o.DB)
	}
	if o.DB.Name != "app1112" {
		t.Errorf("DB.Name = %q, want app1112", o.DB.Name)
	}
}

func TestLocalRunOptions_DatabaseDefaults_HSQLDBRejectsConnectionFlags(t *testing.T) {
	for _, db := range []DBConfig{
		{Type: "hsqldb", Host: "127.0.0.1:5432"},
		{Type: "hsqldb", User: "u"},
		{Type: "hsqldb", Password: "p"},
	} {
		o := LocalRunOptions{ProjectPath: "/proj/App.mpr", DB: db}
		o.applyDefaults()
		if err := o.applyDatabaseDefaults(); err == nil {
			t.Errorf("hsqldb with %+v: want an error", db)
		}
	}
}

func TestLocalRunOptions_DatabaseDefaults_HSQLDBRejectsEnsureDB(t *testing.T) {
	o := LocalRunOptions{ProjectPath: "/proj/App.mpr", DB: DBConfig{Type: "hsqldb"}, EnsureDB: true}
	o.applyDefaults()
	if err := o.applyDatabaseDefaults(); err == nil {
		t.Error("hsqldb + --ensure-db: want an error")
	}
}

func TestLocalRunOptions_DatabaseDefaults_UnknownType(t *testing.T) {
	o := LocalRunOptions{ProjectPath: "/proj/App.mpr", DB: DBConfig{Type: "mysql"}}
	o.applyDefaults()
	if err := o.applyDatabaseDefaults(); err == nil {
		t.Error("unknown db type: want an error")
	}
}
```

- [ ] **Step 2: Run them and confirm they fail**

Run: `go test -run 'TestLocalRunOptions_DatabaseDefaults' ./cmd/mxcli/docker/`
Expected: FAIL — `o.applyDatabaseDefaults undefined`.

- [ ] **Step 3: Remove the DB block from `applyDefaults` and add `applyDatabaseDefaults`**

In `cmd/mxcli/docker/runlocal.go`, delete this block from `applyDefaults()`:

```go
	if o.DB.Type == "" {
		o.DB.Type = "PostgreSQL"
	}
	if o.DB.Host == "" {
		o.DB.Host = "127.0.0.1:5432"
	}
	if o.DB.User == "" {
		o.DB.User = "mendix"
	}
	if o.DB.Password == "" {
		o.DB.Password = "mendix"
	}
	if o.DB.Name == "" {
		o.DB.Name = deriveDBName(o.ProjectPath)
	}
```

Add this method immediately after `applyDefaults()`:

```go
// applyDatabaseDefaults validates --db-type and fills the connection settings the
// chosen database needs. It is separate from applyDefaults because it can fail:
// a flag combination that cannot work (--db-type hsqldb with --db-host) is a user
// error, not something to silently normalise away.
func (o *LocalRunOptions) applyDatabaseDefaults() error {
	kind, err := NormalizeDBType(o.DB.Type)
	if err != nil {
		return err
	}
	if o.DB.Name == "" {
		o.DB.Name = deriveDBName(o.ProjectPath)
	}
	if IsFileBasedDBType(kind) {
		// The built-in database is a file: it has no host or credentials, and
		// accepting them would imply a connection that never happens.
		if o.DB.Host != "" || o.DB.User != "" || o.DB.Password != "" {
			return fmt.Errorf("--db-type hsqldb uses the built-in file database and takes no " +
				"--db-host, --db-user or --db-password")
		}
		if o.EnsureDB {
			return fmt.Errorf("--ensure-db provisions PostgreSQL; the built-in HSQLDB database " +
				"needs no provisioning — drop --ensure-db")
		}
		o.DB.Type = RuntimeDatabaseType(kind)
		o.DB.Host, o.DB.User, o.DB.Password = "", "", ""
		return nil
	}
	// PostgreSQL (unchanged behaviour).
	if o.DB.Host == "" {
		o.DB.Host = "127.0.0.1:5432"
	}
	if o.DB.User == "" {
		o.DB.User = "mendix"
	}
	if o.DB.Password == "" {
		o.DB.Password = "mendix"
	}
	o.DB.Type = RuntimeDatabaseType(kind)
	return nil
}
```

- [ ] **Step 4: Call it from `RunLocal`**

In `RunLocal`, change the first two lines:

```go
	opts.applyDefaults()
	w, stderr := opts.Stdout, opts.Stderr
```

to:

```go
	opts.applyDefaults()
	if err := opts.applyDatabaseDefaults(); err != nil {
		return err
	}
	w, stderr := opts.Stdout, opts.Stderr
```

- [ ] **Step 5: Fix the two existing tests that relied on DB defaults in `applyDefaults`**

In `cmd/mxcli/docker/runlocal_test.go`, in `TestLocalRunOptions_Defaults`, after `o.applyDefaults()` add:

```go
	if err := o.applyDatabaseDefaults(); err != nil {
		t.Fatalf("applyDatabaseDefaults: %v", err)
	}
```

In `TestLocalRunOptions_DefaultsRespectOverrides`, after `o.applyDefaults()` add:

```go
	if err := o.applyDatabaseDefaults(); err != nil {
		t.Fatalf("applyDatabaseDefaults: %v", err)
	}
```

- [ ] **Step 6: Run the package tests**

Run: `go test -run 'TestLocalRunOptions' ./cmd/mxcli/docker/`
Expected: PASS (all `TestLocalRunOptions_*`).

- [ ] **Step 7: Commit**

```bash
git add cmd/mxcli/docker/runlocal.go cmd/mxcli/docker/runlocal_test.go
git commit -m "feat: validate --db-type and fill type-aware database defaults"
```

---

### Task 3: Skip the TCP reachability check for a file database

**Files:**
- Modify: `cmd/mxcli/docker/localboot.go` (add method near `DBConfig`)
- Modify: `cmd/mxcli/docker/runlocal.go` (the DB check at ~608)
- Test: `cmd/mxcli/docker/runlocal_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/mxcli/docker/runlocal_test.go`:

```go
func TestDBConfig_IsFileBased(t *testing.T) {
	if !(DBConfig{Type: "HSQLDB"}).IsFileBased() {
		t.Error("HSQLDB should be file-based")
	}
	if (DBConfig{Type: "PostgreSQL"}).IsFileBased() {
		t.Error("PostgreSQL is not file-based")
	}
	if (DBConfig{Type: "hsqldb"}).IsFileBased() {
		t.Error("the check must use the runtime spelling set by applyDatabaseDefaults, not the raw flag")
	}
}
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `go test -run TestDBConfig_IsFileBased ./cmd/mxcli/docker/`
Expected: FAIL — `c.IsFileBased undefined`.

- [ ] **Step 3: Add the method**

In `cmd/mxcli/docker/localboot.go`, immediately after the `DBConfig` struct, add:

```go
// IsFileBased reports whether this is the runtime's built-in file database, which
// has no host to reach. It keys on the runtime spelling set by
// applyDatabaseDefaults.
func (c DBConfig) IsFileBased() bool {
	return c.Type == RuntimeDatabaseType(DBTypeHSQLDB)
}
```

- [ ] **Step 4: Skip the check in `RunLocal`**

In `cmd/mxcli/docker/runlocal.go`, change:

```go
	} else if err := pingTCP(opts.DB.Host, 3*time.Second); err != nil {
```

to:

```go
	} else if !opts.DB.IsFileBased() {
		if err := pingTCP(opts.DB.Host, 3*time.Second); err != nil {
			return fmt.Errorf("database not reachable at %s: %w\n"+
				"  Pass --ensure-db to provision it, or start Postgres and create the '%s' database (user %q).",
				opts.DB.Host, err, opts.DB.Name, opts.DB.User)
		}
	}
```

(Delete the original `return fmt.Errorf(...)` body that followed the old `else if`, so the block above replaces it exactly.)

- [ ] **Step 5: Run the package tests**

Run: `go test -run 'TestDBConfig_IsFileBased|TestLocalRunOptions' ./cmd/mxcli/docker/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/mxcli/docker/localboot.go cmd/mxcli/docker/runlocal.go cmd/mxcli/docker/runlocal_test.go
git commit -m "feat: skip the TCP check for the built-in file database"
```

---

### Task 4: The runtime receives the HSQLDB configuration

**Files:**
- Test: `cmd/mxcli/docker/localboot_test.go`

`runtimeConfigParams` already forwards `o.DB.Type/Host/User/Password`; this task
pins the HSQLDB shape so a future edit cannot regress it.

- [ ] **Step 1: Write the test**

Append to `cmd/mxcli/docker/localboot_test.go`:

```go
func TestRuntimeConfigParams_HSQLDB(t *testing.T) {
	o := testLocalOpts()
	o.DB = DBConfig{Type: "HSQLDB", Name: "appdb"}
	p := runtimeConfigParams(o, nil)
	checks := map[string]any{
		"DatabaseType":     "HSQLDB",
		"DatabaseHost":     "",
		"DatabaseName":     "appdb",
		"DatabaseUserName": "",
		"DatabasePassword": "",
	}
	for k, want := range checks {
		if p[k] != want {
			t.Errorf("%s = %v, want %v", k, p[k], want)
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test -run TestRuntimeConfigParams_HSQLDB ./cmd/mxcli/docker/`
Expected: PASS (the mapping already forwards these).

- [ ] **Step 3: Commit**

```bash
git add cmd/mxcli/docker/localboot_test.go
git commit -m "test: pin the HSQLDB boot configuration the runtime receives"
```

---

### Task 5: Wire up the `--db-type` flag

**Files:**
- Modify: `cmd/mxcli/cmd_run.go` (flag registration ~316; flag read ~150; `DBConfig` ~221; long help ~38)

- [ ] **Step 1: Read the flag**

In `cmd/mxcli/cmd_run.go`, next to the other DB flags, add:

```go
		dbType, _ := cmd.Flags().GetString("db-type")
```

- [ ] **Step 2: Pass it into `DBConfig`**

Change the `DB` literal:

```go
			DB: docker.DBConfig{
				Host:     dbHost,
				Name:     dbName,
				User:     dbUser,
				Password: dbPassword,
			},
```

to:

```go
			DB: docker.DBConfig{
				Type:     dbType,
				Host:     dbHost,
				Name:     dbName,
				User:     dbUser,
				Password: dbPassword,
			},
```

- [ ] **Step 3: Register the flag**

Next to the other `db-*` registrations, add:

```go
	runCmd.Flags().String("db-type", "", "Database type for a local run: postgresql (default) or hsqldb (the runtime's built-in file database — no server, no --db-host/--db-user/--db-password, data under <project>/deployment/data/database/hsqldb/)")
```

- [ ] **Step 4: Document it in the long help**

In the `run` command's long help, after the line about `--db-host/--db-name/...`, add:

```
  - --db-type hsqldb runs against the built-in file database instead of
    PostgreSQL, so no database server is needed. Its data lives under the
    project's deployment directory and is meant for local development only.
```

- [ ] **Step 5: Build and smoke-test the flag**

Run:
```bash
go build -o bin/mxcli.exe ./cmd/mxcli
./bin/mxcli.exe run --help | grep -A2 -- --db-type
```
Expected: the flag is listed with the HSQLDB description.

- [ ] **Step 6: Verify the refusal is user-facing**

Run:
```bash
./bin/mxcli.exe run --local -p EmptyApp/EmptyApp.mpr --db-type hsqldb --db-host 127.0.0.1:5432
```
Expected: exits non-zero with `--db-type hsqldb uses the built-in file database and takes no --db-host...`.

- [ ] **Step 7: Commit**

```bash
git add cmd/mxcli/cmd_run.go
git commit -m "feat: add the --db-type flag to run --local"
```

---

### Task 6: Integration test — HSQLDB needs no database server

**Files:**
- Create: `cmd/mxcli/docker/runlocal_hsqldb_integration_test.go`

- [ ] **Step 1: Write the test**

Create `cmd/mxcli/docker/runlocal_hsqldb_integration_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The point of the built-in database: an app boots with NO database server.
// Modeled on localapp_integration_test.go — scaffold a project when no fixture
// is given, build it with a real mxbuild serve, then boot the runtime with
// HSQLDB and require it to answer. Self-contained: HSQLDB is a file, so there is
// nothing else to stand up.
func TestRunLocal_HSQLDBNeedsNoDatabaseServer(t *testing.T) {
	mprPath := os.Getenv("MXCLI_IT_PROJECT")
	if mprPath == "" {
		mxPath, err := ResolveMx("")
		if err != nil {
			t.Skipf("mx not resolvable and MXCLI_IT_PROJECT unset: %v", err)
		}
		dir, err := os.MkdirTemp("", "mxhsql")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		scaffold := exec.Command(mxPath, "create-project")
		scaffold.Dir = dir
		if out, err := scaffold.CombinedOutput(); err != nil {
			t.Skipf("mx create-project failed: %v\n%s", err, out)
		}
		mprPath = filepath.Join(dir, "App.mpr")
	}
	if _, err := os.Stat(mprPath); err != nil {
		t.Skipf("no project fixture at %s: %v", mprPath, err)
	}

	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Skipf("cannot open project: %v", err)
	}
	version := reader.ProjectVersion().ProductVersion
	reader.Disconnect()

	installPath, err := resolveRuntimeInstall(version, io.Discard)
	if err != nil {
		t.Skipf("no runtime for %s: %v", version, err)
	}
	javaMajor, _ := ProjectJavaMajor(mprPath)

	serve, err := StartServe(ServeOptions{Version: version, JavaMajor: javaMajor, Host: "127.0.0.1", Port: 6549})
	if err != nil {
		t.Skipf("cannot start mxbuild serve: %v", err)
	}
	defer serve.Stop()

	build, err := serve.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: mprPath})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !build.OK() {
		t.Fatalf("build failed: %s", build.Message)
	}

	deployDir := filepath.Join(filepath.Dir(mprPath), "deployment")
	rt, err := StartLocalRuntime(LocalRuntimeOptions{
		DeployDir:   deployDir,
		InstallPath: installPath,
		JavaMajor:   javaMajor,
		AdminPass:   defaultLocalAdminPass,
		AppPort:     8087,
		AdminPort:   8097,
		DB:          DBConfig{Type: "HSQLDB", Name: deriveDBName(mprPath)},
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	})
	if err != nil {
		t.Fatalf("boot with the built-in database: %v", err)
	}
	defer rt.Stop()

	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err := http.Get(rt.AppURL())
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("app did not answer 200 at %s", rt.AppURL())
		}
		time.Sleep(time.Second)
	}

	hsqlDir := filepath.Join(deployDir, "data", "database", "hsqldb")
	entries, err := os.ReadDir(hsqlDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected the built-in database files under %s (err=%v, n=%d)", hsqlDir, err, len(entries))
	}
}
```

- [ ] **Step 2: Compile the integration build**

Run: `go test -tags integration -run TestRunLocal_HSQLDBNeedsNoDatabaseServer -count=1 ./cmd/mxcli/docker/ -c -o /dev/null`
Expected: compiles (no `undefined`).

- [ ] **Step 3: Run it where a runtime is cached (Linux CI / this dev box)**

Run: `go test -tags integration -run TestRunLocal_HSQLDBNeedsNoDatabaseServer -count=1 -v ./cmd/mxcli/docker/`
Expected: PASS (or SKIP with a clear reason when mxbuild/runtime are absent). If it fails because the HSQLDB files are not under `data/database/hsqldb/`, record the actual directory the runtime logged and update the assertion — that is the one empirical fact this test exists to pin.

The directory assertion was empirically confirmed: the files land at
`deployment/data/database/hsqldb/<name>/app.properties` and `app.script`. The
Gradle bin directory must be on `PATH` (e.g. `/c/Program Files/Mendix/gradle-8.5/bin`)
with `MENDIX_GRADLE_HOME` at the Gradle root, or `mxbuild serve` cannot build.

- [ ] **Step 4: Commit**

```bash
git add cmd/mxcli/docker/runlocal_hsqldb_integration_test.go
git commit -m "test: prove run --local boots on the built-in database with no server"
```

---

### Task 6b: Create the schema when the database does not exist yet

**Files:**
- Modify: `cmd/mxcli/docker/runtime_controller.go` (`needsDBUpdate`)
- Test: `cmd/mxcli/docker/runtime_controller_test.go` (`TestNeedsDBUpdate`)

The integration test proved that a **fresh** HSQLDB database does not boot: Mendix's
HSQLDB URL carries `ifexists=true`, so it never creates the file, and `start`
answers M2EE result 2 ("The database to be used does not exist."). `needsDBUpdate`
only recognised result 3.

- [ ] Add test cases `{"result2", &M2EEResponse{Result: 2}, true}` and
  `{"no-existing-db-message", &M2EEResponse{Message: "The database to be used does not exist."}, true}`.
- [ ] In `needsDBUpdate`, return true for `resp.Result == 2 || resp.Result == 3`, and
  match the lower-cased message `"database to be used does not exist"` as well as
  `"database has to be updated"`.
- [ ] In `Start`, track that the schema step ran and report
  `start failed after creating or updating the database schema: %s` if the second
  start still fails.
- [ ] Commit: `fix: create the schema when the database does not exist yet`.

---

### Task 7: CHANGELOG, full verification, PR

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add the changelog entry**

Under `## [Unreleased]` → `### Added`, add:

```markdown
- **`run --local --db-type hsqldb` — boot with no database server** — a local run
  can now use Mendix's built-in, file-based HSQLDB instead of PostgreSQL. The
  runtime already ships the driver and derives the file URL from `DatabaseType`
  and `DatabaseName`, so the data lands under
  `<project>/deployment/data/database/hsqldb/`. `--db-host`, `--db-user` and
  `--db-password` are refused with HSQLDB (it is a file, not a connection) and
  `--ensure-db` is refused too (there is nothing to provision); PostgreSQL
  remains the default and is unchanged. Intended for local development and
  demos only — Mendix marks HSQLDB as local-testing-only.
```

- [ ] **Step 2: Run the full unit suite for the package**

Run: `go test -count=1 ./cmd/mxcli/docker/`
Expected: PASS on Linux/CI. On Windows this package has pre-existing POSIX-only
failures (#897) — run the scoped set instead:
`go test -count=1 -run 'TestNormalizeDBType|TestRuntimeDatabaseType|TestLocalRunOptions|TestDBConfig_IsFileBased|TestRuntimeConfigParams' ./cmd/mxcli/docker/`

- [ ] **Step 3: gofmt + vet**

Run:
```bash
gofmt -l cmd/mxcli/docker/dbtype.go cmd/mxcli/docker/dbtype_test.go cmd/mxcli/docker/runlocal.go cmd/mxcli/docker/runlocal_test.go cmd/mxcli/docker/localboot.go cmd/mxcli/docker/localboot_test.go cmd/mxcli/cmd_run.go cmd/mxcli/docker/runlocal_hsqldb_integration_test.go
go vet ./cmd/mxcli/docker/ ./cmd/mxcli/
```
Expected: no output from `gofmt -l`; `go vet` clean.

- [ ] **Step 4: Commit and push**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog for the built-in HSQLDB database"
git push -u origin feat/run-local-hsqldb
```

- [ ] **Step 5: Open the PR**

Base it on `main` (or on `fix/windows-local-run-process-helpers` until #1147 merges):

```bash
gh pr create --repo mendixlabs/mxcli --base main --head engalar:feat/run-local-hsqldb \
  --title "feat: run --local with the built-in HSQLDB database" \
  --body-file - <<'EOF'
Adds `--db-type postgresql|hsqldb` to `run --local`. With `hsqldb` the app boots
against Mendix's built-in file database, so no database server is needed; the
data lands under `<project>/deployment/data/database/hsqldb/`.

- `--db-type` defaults to `postgresql`; existing behaviour is unchanged.
- HSQLDB refuses `--db-host`/`--db-user`/`--db-password` and `--ensure-db`.
- The pre-boot TCP check is skipped for the file database.
- Unit tests pin the type mapping and validations; an integration test boots a
  scaffolded app with HSQLDB and asserts HTTP 200 plus the data files.

Spec: docs/11-proposals/PROPOSAL_run_local_builtin_database.md
Depends on #1147 (Windows process-helper fix).
EOF
```

Expected: a PR URL.

---

## Self-Review

- **Spec coverage:** CLI (`--db-type`) → Task 5; config mapping → Tasks 2-4; skip reachability → Task 3; `--ensure-db` refusal → Task 2; data location → Task 6; unit + integration tests → Tasks 1-6; CHANGELOG/PR → Task 7. No gaps.
- **Placeholders:** none — every code step carries the full code.
- **Type consistency:** `NormalizeDBType`, `IsFileBasedDBType`, `RuntimeDatabaseType`, `DBConfig.IsFileBased`, `applyDatabaseDefaults` are used with the same names and signatures throughout.
- **Known risk:** the exact HSQLDB directory (`data/database/hsqldb`) comes from strings read out of the runtime jars; Task 6 Step 3 is the checkpoint that confirms it and is the only place a correction would be needed.
