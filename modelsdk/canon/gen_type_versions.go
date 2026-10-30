//go:build ignore

// SPDX-License-Identifier: Apache-2.0

// gen_type_versions extracts every metamodel TYPE's own introduction and
// deletion version from the Mendix Model SDK into
// mendixmodelsdk_type_versions.json, which the type floor (typefloor.go) reads
// (ako/mxcli#1774).
//
// The generated version data (modelsdk/version, from modelsdk/gen/*/version.go)
// holds property versions only, and the generated packages cannot be
// regenerated here (they carry hand fixes, ako/mxcli#1163), so the type data
// comes straight from the SDK package. It is read as TEXT — never executed —
// from each `X.versionInfo = new exports.StructureVersionInfo({...})` in
// src/gen/*.js: the object's top-level `introduced` and `deleted` (the nested
// ones under `properties` are property versions, already in modelsdk/version).
//
// # Storage names
//
// The SDK names a type by its qualified name; a document stores it under its
// storage name, which differs for some (Pages$ is stored as Forms$, a
// CreateObjectAction as Microflows$CreateChangeAction, …). The storage names are
// taken from modelsdk/gen/*/types.go (each initX's SetTypeName). A storage name
// can then belong to more than one SDK type — a type the SDK renamed keeps its
// storage name (Workflows$CallMicroflowTask is the storage name of both the
// SDK's CallMicroflowTask, deleted 11.9, and CallMicroflowActivity, introduced
// 11.9). So each row is the UNION of every SDK type that can be stored under its
// name: introduced is the earliest of them (none if any of them has no
// introduction), deleted only if all of them are deleted, the latest. Both
// directions can only loosen a floor, never tighten one. A storage name only an
// alias reaches gets no row at all (see main).
//
// The rows are checked against Studio Pro-authored projects by
// modelsdk/mpr's TestTypeFloor_StoredProjectsHoldNoUndeclaredType; run it with
// MXCLI_TYPEFLOOR_PROJECTS after a refresh.
//
// Refresh:
//
//	npm pack mendixmodelsdk@<version>       # in a scratch directory
//	cd modelsdk/canon
//	go run gen_type_versions.go -sdk /path/to/mendixmodelsdk-<version>.tgz
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// storageDomain is the storage domain of an SDK domain stored under another
// name as a whole.
var storageDomain = map[string]string{"Pages": "Forms"}

type sdkType struct {
	file, qualified      string
	introduced, deleted  string
	hasVersionInfo, used bool
}

func main() {
	sdk := flag.String("sdk", "", "mendixmodelsdk .tgz (npm pack) or its extracted package directory")
	gen := flag.String("gen", "../gen", "the generated metamodel packages (storage names)")
	out := flag.String("o", "mendixmodelsdk_type_versions.json", "output file")
	flag.Parse()
	if *sdk == "" {
		fail("-sdk is required")
	}
	files, pkgVersion, err := readSDK(*sdk)
	if err != nil {
		fail("%v", err)
	}
	var types []*sdkType
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ts, err := parseFile(n, files[n])
		if err != nil {
			fail("%s: %v", n, err)
		}
		types = append(types, ts...)
	}
	aliases, err := storageAliases(*gen)
	if err != nil {
		fail("%v", err)
	}

	// candidates: storage name → every SDK type that can be stored under it.
	candidates := map[string][]*sdkType{}
	add := func(storage string, t *sdkType) {
		for _, c := range candidates[storage] {
			if c == t {
				return
			}
		}
		candidates[storage] = append(candidates[storage], t)
	}
	for _, t := range types {
		add(defaultStorage(t.qualified), t)
	}
	// An aliased type joins the row of its storage name only where that name
	// is also some SDK type's own: a name only an alias reaches can be the
	// storage of an older type the SDK does not list at all, and a row from
	// the alias alone would be a floor that type never had. Forms$FormCallArgument
	// is the measured case: the alias of the SDK's PageParameterMapping (9.7),
	// stored by Studio Pro 9.1.1 and 9.4.0 (Marketplace 117785, 117680).
	for _, t := range types {
		_, name, _ := strings.Cut(t.qualified, "$")
		if s, ok := aliases[t.file+"."+name]; ok && s != defaultStorage(t.qualified) {
			if _, own := candidates[s]; own {
				add(s, t)
			}
		}
	}

	type row struct {
		Type       string   `json:"type"`
		Introduced string   `json:"introduced,omitempty"`
		Deleted    string   `json:"deleted,omitempty"`
		SDK        []string `json:"sdk,omitempty"`
	}
	var rows []row
	for storage, cs := range candidates {
		r := row{Type: storage}
		intro, del := "", ""
		always, live := false, false
		for _, c := range cs {
			if c.qualified != storage {
				r.SDK = append(r.SDK, c.qualified)
			}
			if !c.hasVersionInfo || c.introduced == "" {
				always = true
			} else if intro == "" || less(c.introduced, intro) {
				intro = c.introduced
			}
			if c.deleted == "" {
				live = true
			} else if del == "" || less(del, c.deleted) {
				del = c.deleted
			}
		}
		if !always {
			r.Introduced = intro
		}
		if !live {
			r.Deleted = del
		}
		if r.Introduced == "" && r.Deleted == "" {
			continue
		}
		sort.Strings(r.SDK)
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Type < rows[j].Type })

	var b bytes.Buffer
	fmt.Fprintf(&b, "{\n  \"source\": %q,\n  \"types\": [\n", "mendixmodelsdk@"+pkgVersion)
	for i, r := range rows {
		line, err := json.Marshal(r)
		if err != nil {
			fail("%v", err)
		}
		sep := ","
		if i == len(rows)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "    %s%s\n", line, sep)
	}
	b.WriteString("  ]\n}\n")
	if err := os.WriteFile(*out, b.Bytes(), 0o644); err != nil {
		fail("%v", err)
	}
	fmt.Fprintf(os.Stderr, "gen_type_versions: %d SDK types, %d rows from mendixmodelsdk %s\n", len(types), len(rows), pkgVersion)
}

// defaultStorage is the storage name of an SDK type with no alias: its
// qualified name, in its domain's storage name.
func defaultStorage(qualified string) string {
	dom, name, _ := strings.Cut(qualified, "$")
	if d, ok := storageDomain[dom]; ok {
		dom = d
	}
	return dom + "$" + name
}

// readSDK returns the package's src/gen/*.js files by base name, and its
// version, from a .tgz or an extracted directory.
func readSDK(p string) (map[string][]byte, string, error) {
	files := map[string][]byte{}
	var pkgJSON []byte
	st, err := os.Stat(p)
	if err != nil {
		return nil, "", err
	}
	if st.IsDir() {
		matches, _ := filepath.Glob(filepath.Join(p, "src", "gen", "*.js"))
		for _, m := range matches {
			b, err := os.ReadFile(m)
			if err != nil {
				return nil, "", err
			}
			files[strings.TrimSuffix(filepath.Base(m), ".js")] = b
		}
		if pkgJSON, err = os.ReadFile(filepath.Join(p, "package.json")); err != nil {
			return nil, "", err
		}
	} else {
		f, err := os.Open(p)
		if err != nil {
			return nil, "", err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, "", err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, "", err
			}
			name := path.Clean(h.Name)
			switch {
			case name == "package/package.json":
				if pkgJSON, err = io.ReadAll(tr); err != nil {
					return nil, "", err
				}
			case path.Dir(name) == "package/src/gen" && strings.HasSuffix(name, ".js"):
				b, err := io.ReadAll(tr)
				if err != nil {
					return nil, "", err
				}
				files[strings.TrimSuffix(path.Base(name), ".js")] = b
			}
		}
	}
	var pkg struct {
		Name, Version string
	}
	if err := json.Unmarshal(pkgJSON, &pkg); err != nil || pkg.Name != "mendixmodelsdk" || pkg.Version == "" {
		return nil, "", fmt.Errorf("%s is not a mendixmodelsdk package (%v)", p, err)
	}
	if len(files) == 0 {
		return nil, "", fmt.Errorf("%s: no src/gen/*.js", p)
	}
	return files, pkg.Version, nil
}

var (
	reTypeName    = regexp.MustCompile(`(\w+)\.structureTypeName = "([^"]+)";`)
	reVersionInfo = regexp.MustCompile(`(\w+)\.versionInfo = new exports\.StructureVersionInfo\(`)
)

// parseFile returns the types one src/gen/<file>.js declares, with the
// top-level introduced/deleted of each one's StructureVersionInfo.
func parseFile(file string, src []byte) ([]*sdkType, error) {
	byClass := map[string]*sdkType{}
	var order []*sdkType
	for _, m := range reTypeName.FindAllSubmatch(src, -1) {
		cls, q := string(m[1]), string(m[2])
		if !strings.Contains(q, "$") {
			return nil, fmt.Errorf("structure type name %q has no domain", q)
		}
		if _, dup := byClass[cls]; dup {
			return nil, fmt.Errorf("class %s declared twice", cls)
		}
		t := &sdkType{file: file, qualified: q}
		byClass[cls] = t
		order = append(order, t)
	}
	for _, loc := range reVersionInfo.FindAllSubmatchIndex(src, -1) {
		cls := string(src[loc[2]:loc[3]])
		t, ok := byClass[cls]
		if !ok {
			return nil, fmt.Errorf("versionInfo for %s, which has no structureTypeName", cls)
		}
		top, err := topLevelStrings(src[loc[1]:])
		if err != nil {
			return nil, fmt.Errorf("%s.versionInfo: %w", cls, err)
		}
		t.hasVersionInfo = true
		t.introduced, t.deleted = top["introduced"], top["deleted"]
		for _, v := range []string{t.introduced, t.deleted} {
			if v != "" && !validVersion(v) {
				return nil, fmt.Errorf("%s: version %q", t.qualified, v)
			}
		}
	}
	return order, nil
}

// topLevelStrings reads the object literal src starts with and returns its
// top-level keys whose values are string literals.
func topLevelStrings(src []byte) (map[string]string, error) {
	out := map[string]string{}
	depth := 0
	key := ""
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '"' || c == '\'':
			j := i + 1
			for ; j < len(src) && src[j] != c; j++ {
				if src[j] == '\\' {
					j++
				}
			}
			if j >= len(src) {
				return nil, fmt.Errorf("unterminated string")
			}
			if depth == 1 && key != "" {
				out[key] = string(src[i+1 : j])
			}
			key = ""
			i = j
		case c == '{' || c == '[':
			depth++
			key = ""
		case c == '}' || c == ']':
			depth--
			key = ""
			if depth == 0 {
				return out, nil
			}
		case depth == 1 && isIdentStart(c) && (i == 0 || !isIdent(src[i-1])):
			j := i
			for j < len(src) && isIdent(src[j]) {
				j++
			}
			k := j
			for k < len(src) && (src[k] == ' ' || src[k] == '\t' || src[k] == '\n' || src[k] == '\r') {
				k++
			}
			if k < len(src) && src[k] == ':' {
				key = string(src[i:j])
				i = k
			} else {
				key = ""
				i = j - 1
			}
		case c == ',':
			key = ""
		}
	}
	return nil, fmt.Errorf("unterminated object")
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdent(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

var reInit = regexp.MustCompile(`(?m)^func init(\w+)\(\) \*\w+ \{\n\to := &\w+\{\}\n\to\.SetTypeName\("([^"]+)"\)`)

// storageAliases maps "<package>.<class>" to the storage name the generated
// codec gives that class (initX's SetTypeName).
func storageAliases(genDir string) (map[string]string, error) {
	matches, err := filepath.Glob(filepath.Join(genDir, "*", "types.go"))
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("no %s/*/types.go (%v)", genDir, err)
	}
	out := map[string]string{}
	for _, m := range matches {
		src, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		pkg := filepath.Base(filepath.Dir(m))
		for _, sm := range reInit.FindAllSubmatch(src, -1) {
			out[pkg+"."+string(sm[1])] = string(sm[2])
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no storage names found in %s", genDir)
	}
	return out, nil
}

func validVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

// less compares two dotted versions numerically.
func less(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen_type_versions: "+format+"\n", args...)
	os.Exit(1)
}
