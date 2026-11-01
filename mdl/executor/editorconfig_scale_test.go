// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The end-anchored regexps the suffix scans replaced (#1848). Kept here as the
// reference the scans must agree with.
var (
	refHideCallTailRE = regexp.MustCompile(`hide(?:Property|Properties|NestedProperties)In$`)
	refNsPrefixRE     = regexp.MustCompile(`[A-Za-z_$][\w$]*\.$`)
)

func TestSuffixScansMatchTheRegexps(t *testing.T) {
	inputs := []string{
		"", ".", "a.", "_.", "$.", "9.", "a9.", "9a.", "x 99a.", "e.foo&&_.", "M.", "ab.cd.",
		"hidePropertyIn", "_.hidePropertiesIn", "x,hideNestedPropertiesIn", "hidePropertyInx",
		"ehidePropertyIn", "hidePropertyIn ", "a.b",
	}
	js, err := os.ReadFile("testdata/datagrid.editorConfig.js")
	if err != nil {
		t.Fatal(err)
	}
	// Every prefix of a real editorConfig that ends where a scan is applied.
	for i := range js {
		if js[i] == '.' || js[i] == 'n' {
			inputs = append(inputs, string(js[:i+1]))
		}
	}
	for _, in := range inputs {
		want := -1
		if loc := refNsPrefixRE.FindStringIndex(in); loc != nil {
			want = loc[0]
		}
		if got := nsPrefix(in); got != want {
			t.Fatalf("nsPrefix(%q) = %d, regexp says %d", tail(in), got, want)
		}
		want = -1
		if loc := refHideCallTailRE.FindStringIndex(in); loc != nil {
			want = loc[0]
		}
		if got := hideCallTail(in); got != want {
			t.Fatalf("hideCallTail(%q) = %d, regexp says %d", tail(in), got, want)
		}
	}
}

// The extractor ran an end-anchored regexp over everything before each hide
// call, so its time grew with the square of the editorConfig's size: 0.5 s
// for Maps' 400 KB one, on every refresh (#1848). Growing the input 8× must
// not grow the time anywhere near 64×.
//
// The input is shaped like minified editorConfig: many small functions with a
// hide each. Work per hide that is bounded by its own function (the alias scan)
// stays out of the measurement; work over the whole file does not.
func TestExtractVisibilityRulesIsLinear(t *testing.T) {
	gen := func(n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, `function g%d(e,t){var r=e.mode;return "v%d"===r&&_.hidePropertyIn(t,e,"p%d"),t}`, i, i, i)
		}
		return b.String()
	}
	timeOf := func(js string) time.Duration {
		best := time.Duration(1 << 62)
		for range 3 {
			start := time.Now()
			extractVisibilityRulesFromJS(js)
			best = min(best, time.Since(start))
		}
		return best
	}
	small, large := timeOf(gen(300)), timeOf(gen(2400))
	if large > 20*small && large > 200*time.Millisecond {
		t.Errorf("8× the input took %v against %v: the extractor is superlinear again", large, small)
	}
}

func tail(s string) string {
	if len(s) > 40 {
		return "…" + s[len(s)-40:]
	}
	return s
}
