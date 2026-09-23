// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"bytes"
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
	var rtOut bytes.Buffer
	rt, err := StartLocalRuntime(LocalRuntimeOptions{
		DeployDir:   deployDir,
		InstallPath: installPath,
		JavaMajor:   javaMajor,
		AdminPass:   defaultLocalAdminPass,
		AppPort:     8087,
		AdminPort:   8097,
		DB:          DBConfig{Type: "HSQLDB", Name: deriveDBName(mprPath)},
		Stdout:      &rtOut,
		Stderr:      &rtOut,
	})
	if err != nil {
		t.Fatalf("boot with the built-in database: %v", err)
	}
	defer rt.Stop()

	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err := client.Get(rt.AppURL())
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("app did not answer 200 at %s\n--- runtime output ---\n%s", rt.AppURL(), rt.Log())
		}
		time.Sleep(time.Second)
	}

	hsqlDir := filepath.Join(deployDir, "data", "database", "hsqldb")
	entries, err := os.ReadDir(hsqlDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected the built-in database files under %s (err=%v, n=%d)", hsqlDir, err, len(entries))
	}
}
