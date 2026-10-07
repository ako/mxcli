// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"regexp"
	"strings"
)

// RenameSource rewrites a Java action's source for a rename from oldName to
// newName the way mxbuild regenerates it: the three generated places that carry
// the action's name — `public class Old`, the constructor `public Old(` and
// toString's `return "Old";` — and nothing else. The user code and extra code
// are the author's and are left byte-for-byte, a mention of the old name
// included; so is everything else, CRLF line endings among it.
//
// Measured against mxbuild 11.14.0 (testdata/mxbuild/JA_Renamed*): renaming the
// file alone left `public class Old` in New.java, which is not valid Java.
func RenameSource(source, oldName, newName string) string {
	if oldName == newName || oldName == "" {
		return source
	}
	old := regexp.QuoteMeta(oldName)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(\bclass\s+)` + old + `\b`),
		regexp.MustCompile(`(\bpublic\s+)` + old + `(\s*\()`),
		regexp.MustCompile(`(\breturn\s+)"` + old + `"(\s*;)`),
	}
	replacements := []string{"${1}" + newName, "${1}" + newName + "${2}", `${1}"` + newName + `"${2}`}

	rewrite := func(generated string) string {
		for i, re := range patterns {
			generated = re.ReplaceAllString(generated, replacements[i])
		}
		return generated
	}

	// Rewrite only the generated stretches between the author's sections.
	var b strings.Builder
	rest := source
	for {
		start, end, ok := nextAuthoredSection(rest)
		if !ok {
			b.WriteString(rewrite(rest))
			return b.String()
		}
		b.WriteString(rewrite(rest[:start]))
		b.WriteString(rest[start:end])
		rest = rest[end:]
	}
}

// authoredSections are the marker pairs whose contents mxbuild keeps as written.
var authoredSections = [][2]string{
	{"// BEGIN USER CODE", "// END USER CODE"},
	{"// BEGIN EXTRA CODE", "// END EXTRA CODE"},
}

// nextAuthoredSection finds the first authored section in s, markers included.
// Markers match case-insensitively, as in RetainedSections.
func nextAuthoredSection(s string) (start, end int, ok bool) {
	lower := asciiLower(s)
	start = -1
	for _, m := range authoredSections {
		bi := strings.Index(lower, asciiLower(m[0]))
		if bi == -1 || (start != -1 && bi > start) {
			continue
		}
		ei := strings.Index(lower[bi:], asciiLower(m[1]))
		if ei == -1 {
			continue
		}
		start, end = bi, bi+ei+len(m[1])
	}
	return start, end, start != -1
}
