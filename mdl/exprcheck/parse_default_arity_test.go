// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// mendixlabs/mxcli#1216: the parse functions take an optional default value,
// returned when the text does not parse. `check` rejected it — "parseDateTimeUTC()
// expects 2 argument(s), got 3. [E006]" — while mx check on 11.14.0 reports 0
// errors. Each row below was built against mx check 11.14.0; the "bad" rows are
// CE0117 there, so the fix must not widen past them.
func TestFuncChecker_ParseDefaultValue_Arity(t *testing.T) {
	ok := []string{
		"parseDateTime($T, 'yyyy-MM-dd')",
		"parseDateTime($T, 'yyyy-MM-dd', empty)",
		"parseDateTime($T, 'yyyy-MM-dd', $Fallback)",
		"parseDateTimeUTC($T, 'yyyy-MM-dd')",
		"parseDateTimeUTC($T, 'yyyy-MM-dd', empty)",
		"parseDateTimeUTC($T, 'yyyy-MM-dd', $Fallback)",
		"parseInteger($T)",
		"parseInteger($T, 0)",
		"parseDecimal($T)",
		"parseDecimal($T, 0)",
		"parseDecimal($T, '#.##')",
		"parseDecimal($T, '#.##', 0)",
	}
	for _, src := range ok {
		_, hs := NewParser().Parse(src, Context{Microflow: "M.F"})
		if hasCode(hs, "E006") {
			t.Errorf("%s must not fire E006: %+v", src, hs)
		}
	}

	bad := []string{
		"parseDateTime($T)",
		"parseDateTimeUTC($T)",
		"parseDateTime($T, 'yyyy-MM-dd', empty, empty)",
		"parseDateTimeUTC($T, 'yyyy-MM-dd', empty, empty)",
		"parseInteger($T, 0, 0)",
		"parseDecimal($T, '#.##', 0, 0)",
		"parseBoolean($T, false)",
	}
	for _, src := range bad {
		_, hs := NewParser().Parse(src, Context{Microflow: "M.F"})
		if !hasCode(hs, "E006") {
			t.Errorf("%s must fire E006, got %+v", src, hs)
		}
	}
}
