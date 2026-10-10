// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"
)

// A consumed REST operation's JSON body is Rest$JsonBody only on a project that
// has the type (11.0+). On 10.24 `mx convert` refused the whole project —
// TypeCacheUnknownTypeException, so Studio Pro could not open it — while
// `mx check` passed, and every POST/PUT/PATCH defaults to a JSON body.
func TestRestBodyToGen_ByVersion(t *testing.T) {
	for _, c := range []struct {
		name     string
		bodyType string
		forms    restBodyForms
		wantType string
		wantKey  string // the key holding the body text
	}{
		{"json, 11.x", "JSON", restBodyForms{jsonBody: true, valueTemplate: true}, "Rest$JsonBody", "Value"},
		{"json, 10.24", "JSON", restBodyForms{jsonBody: false, valueTemplate: true}, "Rest$StringBody", "ValueTemplate"},
		{"json, 10.10", "JSON", restBodyForms{jsonBody: false, valueTemplate: false}, "Rest$StringBody", "Value"},
		{"default kind, 10.24", "", restBodyForms{jsonBody: false, valueTemplate: true}, "Rest$StringBody", "ValueTemplate"},
		// Control: a template body is a string body on every version.
		{"template, 11.x", "TEMPLATE", restBodyForms{jsonBody: true, valueTemplate: true}, "Rest$StringBody", "ValueTemplate"},
	} {
		m := encodeToMap(t, restBodyToGen(c.bodyType, "$ItemData", c.forms))
		if m["$Type"] != c.wantType {
			t.Errorf("%s: $Type = %v, want %s", c.name, m["$Type"], c.wantType)
			continue
		}
		v, ok := m[c.wantKey]
		if !ok {
			t.Errorf("%s: no %s; keys %v", c.name, c.wantKey, keysOf(m))
			continue
		}
		text := v
		if vt, isDoc := v.(map[string]any); isDoc {
			text = vt["Value"]
		}
		if text != "$ItemData" {
			t.Errorf("%s: body text = %#v, want $ItemData", c.name, text)
		}
	}
}
