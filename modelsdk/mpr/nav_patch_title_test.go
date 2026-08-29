// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func navpTestEntry(d bson.D, key string) (interface{}, bool) {
	for _, entry := range d {
		if entry.Key == key {
			return entry.Value, true
		}
	}
	return nil, false
}

// Studio Pro stores the absence of a page-title override as an explicit null.
// An empty TextTemplate is a real override to "" and raises CW0263.
func TestNavpBuildFormSettingsBson_NoTitleOverrideStaysNull(t *testing.T) {
	settings := navpBuildFormSettingsBson("M.Dash")
	title, present := navpTestEntry(settings, "TitleOverride")
	if !present {
		t.Fatal("TitleOverride key missing; Studio Pro writes an explicit null")
	}
	if title != nil {
		t.Fatalf("TitleOverride = %#v, want nil", title)
	}
}
