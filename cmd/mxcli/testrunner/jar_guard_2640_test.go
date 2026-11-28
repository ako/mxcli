// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testProxy is MxTest's generated microflow proxy, the class an injected test
// microflow adds a call stub to.
const testProxy = "mxtest/proxies/microflows/Microflows.class"

func jarOf(t *testing.T, stamp time.Time, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"a/A.class", "b/B.class", "c/C.class", testProxy} {
		body, ok := files[name]
		if !ok {
			continue
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: stamp})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// An --attach build rewrote the running app's project.jar in place, and the
// runtime then failed to load any class it had not loaded yet: "ZipFile
// invalid LOC header" (ako/mxcli#2640). The same classes rebuilt get the
// original bytes back; other classes are reported as a change the running
// runtime cannot take.
func TestJarGuard_2640(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	jar := projectJarPath(mpr)
	if err := os.MkdirAll(filepath.Dir(jar), 0o755); err != nil {
		t.Fatal(err)
	}
	classes := map[string]string{"a/A.class": "AAAA", "b/B.class": "BBBB"}
	orig := jarOf(t, time.Date(2026, 11, 27, 14, 35, 6, 0, time.UTC), classes)
	if err := os.WriteFile(jar, orig, 0o644); err != nil {
		t.Fatal(err)
	}

	g := guardProjectJar(mpr)
	// The build: the same classes, written again with a new timestamp.
	rebuilt := jarOf(t, time.Date(2026, 11, 27, 14, 35, 51, 0, time.UTC), classes)
	if bytes.Equal(rebuilt, orig) {
		t.Fatal("fixture: the rebuilt jar must differ in bytes")
	}
	_ = os.WriteFile(jar, rebuilt, 0o644)
	changed, err := g.settle()
	if err != nil || changed {
		t.Fatalf("same classes: changed=%v err=%v, want unchanged", changed, err)
	}
	if now, _ := os.ReadFile(jar); !bytes.Equal(now, orig) {
		t.Error("the jar the runtime opened was not put back")
	}

	// Control: a build that changes a class is reported, and the running
	// app's jar is still put back.
	g = guardProjectJar(mpr)
	_ = os.WriteFile(jar, jarOf(t, time.Now(), map[string]string{"a/A.class": "AAAA", "b/B.class": "CHANGED"}), 0o644)
	if changed, err := g.settle(); err != nil || !changed {
		t.Errorf("changed classes: changed=%v err=%v, want changed", changed, err)
	}
	if now, _ := os.ReadFile(jar); !bytes.Equal(now, orig) {
		t.Error("the running app's jar was not put back after a Java change")
	}

	// The attach's own change on Mendix 11.15/11.16, where its build recompiles
	// the proxies: MxTest's microflow proxy gains the injected test microflow's
	// call stub, and nothing else changes. That is no Java the running runtime
	// needs, so nothing is refused, and its jar still goes back.
	withProxy := jarOf(t, time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC),
		map[string]string{"a/A.class": "AAAA", "b/B.class": "BBBB", testProxy: "stubs for 2 microflows"})
	_ = os.WriteFile(jar, withProxy, 0o644)
	g = guardProjectJar(mpr)
	_ = os.WriteFile(jar, jarOf(t, time.Now(),
		map[string]string{"a/A.class": "AAAA", "b/B.class": "BBBB", testProxy: "stubs for 3 microflows, one a test"}), 0o644)
	if changed, err := g.settle(); err != nil || changed {
		t.Errorf("only the microflow proxy changed: changed=%v err=%v, want unchanged", changed, err)
	}
	if now, _ := os.ReadFile(jar); !bytes.Equal(now, withProxy) {
		t.Error("the jar the runtime opened was not put back after a proxy-only change")
	}

	// Control: the proxy changing beside a real class is still a Java change.
	g = guardProjectJar(mpr)
	_ = os.WriteFile(jar, jarOf(t, time.Now(),
		map[string]string{"a/A.class": "CHANGED", "b/B.class": "BBBB", testProxy: "stubs for 3 microflows"}), 0o644)
	if changed, err := g.settle(); err != nil || !changed {
		t.Errorf("a class and the proxy changed: changed=%v err=%v, want changed", changed, err)
	}

	// No jar: nothing to guard.
	if g := guardProjectJar(filepath.Join(t.TempDir(), "X.mpr")); g != nil {
		t.Error("guarded a jar that does not exist")
	}
}
