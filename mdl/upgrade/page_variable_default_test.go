// SPDX-License-Identifier: Apache-2.0

package upgrade

import "testing"

// A page variable's default written in a string (MDL-DEPR086) is rewritten to
// the bare expression; a default that is itself a string stays as it is.
func TestUpgrade_PageVariableDefaultsBare(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"create page",
			"create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default,\n" +
				"  Variables: ( $show: boolean = 'true', $n: integer = '20', $s: string = '''x''', $c: boolean = 'if (3 < 4) then true else false' )) { };",
			"create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default,\n" +
				"  Variables: ( $show: boolean = true, $n: integer = 20, $s: string = '''x''', $c: boolean = if (3 < 4) then true else false )) { };",
		},
		{
			"alter page add variables",
			"alter page M.P { add variables $show: boolean = 'false' };",
			"alter page M.P { add variables $show: boolean = false };",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := mustUpgrade(t, tc.src, Options{})
			if res.Source != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", res.Source, tc.want)
			}
			if len(res.Unrewritten) != 0 {
				t.Errorf("Unrewritten = %+v", res.Unrewritten)
			}
		})
	}
}
