// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RenameJavaSourceFile moved Old.java to New.java and left `public class Old`
// inside it — not valid Java, which `run --local --watch` hot reload and every
// IDE compile from disk. The class, constructor and toString must follow the
// file; the user code, mentions of the old name included, must not.
func TestRenameJavaSourceFileRenamesTheClass(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "javasource", "sales", "actions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package sales.actions;\r\n\r\n" +
		"public class JA_Old extends UserAction<java.lang.String>\r\n{\r\n" +
		"\tpublic JA_Old(\r\n\t\tIContext context\r\n\t)\r\n\t{\r\n\t\tsuper(context);\r\n\t}\r\n\r\n" +
		"\t@java.lang.Override\r\n\tpublic java.lang.String executeAction() throws Exception\r\n\t{\r\n" +
		"\t\t// BEGIN USER CODE\r\n\t\treturn \"JA_Old\";\r\n\t\t// END USER CODE\r\n\t}\r\n\r\n" +
		"\t@java.lang.Override\r\n\tpublic java.lang.String toString()\r\n\t{\r\n\t\treturn \"JA_Old\";\r\n\t}\r\n}\r\n"
	if err := os.WriteFile(filepath.Join(dir, "JA_Old.java"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	b := &Backend{path: filepath.Join(root, "App.mpr")}
	if err := b.RenameJavaSourceFile("Sales", "JA_Old", "JA_New"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "JA_Old.java")); !os.IsNotExist(err) {
		t.Errorf("JA_Old.java still exists (stat err %v)", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "JA_New.java"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(
		"public class JA_Old", "public class JA_New",
		"public JA_Old(", "public JA_New(",
		"\t\treturn \"JA_Old\";\r\n\t}\r\n}", "\t\treturn \"JA_New\";\r\n\t}\r\n}",
	).Replace(src)
	if string(got) != want {
		t.Errorf("JA_New.java:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(string(got), "// BEGIN USER CODE\r\n\t\treturn \"JA_Old\";") {
		t.Error("the user code's mention of JA_Old was rewritten; mxbuild keeps it")
	}
}

// A missing stub is still not an error: the action may never have had one.
func TestRenameJavaSourceFileMissingIsNotAnError(t *testing.T) {
	b := &Backend{path: filepath.Join(t.TempDir(), "App.mpr")}
	if err := b.RenameJavaSourceFile("Sales", "JA_Old", "JA_New"); err != nil {
		t.Errorf("missing source: %v", err)
	}
}
