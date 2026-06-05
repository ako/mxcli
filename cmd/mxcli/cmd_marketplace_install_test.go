// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeMpk builds a minimal .mpk (zip) containing the given package.xml body.
func writeMpk(t *testing.T, packageXML string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.mpk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	if packageXML != "" {
		w, err := zw.Create("package.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(packageXML)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestModuleNameFromMpk_Module(t *testing.T) {
	const moduleXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/">
  <modelerProject xmlns="http://www.mendix.com/modelerProject/1.0/">
    <module name="DatabaseConnector" />
    <projectFile path="project.mpr" />
  </modelerProject>
</package>`
	name, err := moduleNameFromMpk(writeMpk(t, moduleXML))
	if err != nil {
		t.Fatal(err)
	}
	if name != "DatabaseConnector" {
		t.Errorf("name = %q, want DatabaseConnector", name)
	}
}

func TestModuleNameFromMpk_Widget(t *testing.T) {
	const widgetXML = `<?xml version="1.0" encoding="utf-8" ?>
<package xmlns="http://www.mendix.com/package/1.0/">
    <clientModule name="Badge" version="3.2.2" xmlns="http://www.mendix.com/clientModule/1.0/">
        <widgetFiles><widgetFile path="Badge.xml" /></widgetFiles>
    </clientModule>
</package>`
	name, err := moduleNameFromMpk(writeMpk(t, widgetXML))
	if err != nil {
		t.Fatal(err)
	}
	if name != "Badge" {
		t.Errorf("name = %q, want Badge", name)
	}
}

func TestModuleNameFromMpk_NoPackageXML(t *testing.T) {
	if _, err := moduleNameFromMpk(writeMpk(t, "")); err == nil {
		t.Fatal("expected error when package.xml is absent")
	}
}
