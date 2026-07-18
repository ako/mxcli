// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeriveDBName(t *testing.T) {
	cases := map[string]string{
		"/path/App1112.mpr": "app1112",
		"/path/My App.mpr":  "my_app",
		"/x/Sales-CRM.mpr":  "sales_crm",
		"/x/123Numbers.mpr": "db_123numbers",
		"/x/__weird__.mpr":  "weird",
		"/x/.mpr":           "mxlocal",
	}
	for in, want := range cases {
		if got := deriveDBName(in); got != want {
			t.Errorf("deriveDBName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalRunOptions_Defaults(t *testing.T) {
	o := LocalRunOptions{ProjectPath: "/proj/App1112.mpr"}
	o.applyDefaults()
	if o.DeployDir != filepath.FromSlash("/proj/deployment") {
		t.Errorf("DeployDir = %q", o.DeployDir)
	}
	if o.AppPort != 8080 || o.AdminPort != 8090 || o.ServePort != 6543 {
		t.Errorf("ports = %d/%d/%d", o.AppPort, o.AdminPort, o.ServePort)
	}
	if o.AdminPass != defaultLocalAdminPass {
		t.Errorf("AdminPass = %q", o.AdminPass)
	}
	if o.DB.Type != "PostgreSQL" || o.DB.Host != "127.0.0.1:5432" || o.DB.User != "mendix" || o.DB.Password != "mendix" {
		t.Errorf("DB defaults = %+v", o.DB)
	}
	if o.DB.Name != "app1112" {
		t.Errorf("DB.Name = %q, want app1112", o.DB.Name)
	}
	if o.PollInterval != time.Second {
		t.Errorf("PollInterval = %v", o.PollInterval)
	}
}

func TestLocalRunOptions_DefaultsRespectOverrides(t *testing.T) {
	o := LocalRunOptions{
		ProjectPath: "/proj/App.mpr",
		AppPort:     9000,
		DB:          DBConfig{Host: "db:5432", Name: "custom", User: "u", Password: "p"},
	}
	o.applyDefaults()
	if o.AppPort != 9000 {
		t.Errorf("AppPort override lost: %d", o.AppPort)
	}
	if o.DB.Host != "db:5432" || o.DB.Name != "custom" || o.DB.User != "u" || o.DB.Password != "p" {
		t.Errorf("DB overrides lost: %+v", o.DB)
	}
}

func TestEnsureMxBuildRuntimeSibling(t *testing.T) {
	// Point the cache roots at a temp HOME so we don't touch the real cache.
	home := t.TempDir()
	t.Setenv("HOME", home)

	version := "99.99.99"
	// Build the runtime cache with a runtime/ dir.
	runtimeCache, _ := RuntimeCacheDir(version)
	realRuntime := filepath.Join(runtimeCache, "runtime")
	if err := os.MkdirAll(realRuntime, 0o755); err != nil {
		t.Fatal(err)
	}
	// mxbuild cache exists (modeler/) but has no runtime/ sibling yet.
	mxbuildCache, _ := MxBuildCacheDir(version)
	if err := os.MkdirAll(filepath.Join(mxbuildCache, "modeler"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ensureMxBuildRuntimeSibling(version, io.Discard); err != nil {
		t.Fatalf("ensureMxBuildRuntimeSibling: %v", err)
	}
	link := filepath.Join(mxbuildCache, "runtime")
	if _, err := os.Stat(link); err != nil {
		t.Errorf("runtime sibling not created: %v", err)
	}
	// Idempotent second call.
	if err := ensureMxBuildRuntimeSibling(version, io.Discard); err != nil {
		t.Errorf("second call should be a no-op, got %v", err)
	}
}

func TestEnsureMxBuildRuntimeSibling_MissingSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	version := "99.99.98"
	mxbuildCache, _ := MxBuildCacheDir(version)
	_ = os.MkdirAll(filepath.Join(mxbuildCache, "modeler"), 0o755)
	// No runtime cache -> error.
	if err := ensureMxBuildRuntimeSibling(version, io.Discard); err == nil {
		t.Error("expected error when the runtime source is absent")
	}
}

func TestProjectMTime(t *testing.T) {
	dir := t.TempDir()
	deploy := filepath.Join(dir, "deployment")
	_ = os.MkdirAll(deploy, 0o755)
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0o755)

	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := projectMTime(dir, deploy)
	if base.IsZero() {
		t.Fatal("expected a non-zero mtime")
	}

	// A change under deployment/ must NOT advance the signal.
	future := time.Now().Add(time.Hour)
	deployFile := filepath.Join(deploy, "model.mdp")
	_ = os.WriteFile(deployFile, []byte("y"), 0o644)
	_ = os.Chtimes(deployFile, future, future)
	if got := projectMTime(dir, deploy); got.After(base) {
		t.Error("deployment/ changes should be ignored by the watch signal")
	}

	// A change to the project file MUST advance the signal.
	_ = os.Chtimes(mpr, future, future)
	if got := projectMTime(dir, deploy); !got.After(base) {
		t.Error("project file change should advance the watch signal")
	}
}
