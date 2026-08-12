// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// Studio Pro 11.13 gave pg_read_page a `depth` argument defaulting to 4, and
// replaces anything deeper with the string "...". mxcli's ALTER PAGE is
// read-modify-replace-whole-page, so a truncated read is written back over the
// live page. Captured against 11.13: an ordinary page read at the default depth
// collapsed from 32,594 bytes to 1,052, its whole widget tree reduced to
// {"widgets":["...","..."]}; writing that back was rejected with
// PROP_NOT_PRIMITIVE, breaking ALTER PAGE outright.
//
// These tests pin both halves of the fix: request the full depth where the
// server understands it, and refuse a truncated read rather than write it back.

// deepPage is a LightPage nested past the 11.13 default depth of 4.
const deepPage = `{
  "title": "Deep",
  "layout": "Atlas_Core.Atlas_Default",
  "widgets": [
    {"$Type": "Pages$Content", "slot": "Main", "widgets": [
      {"$Type": "Pages$DivContainer", "name": "lvl1", "widgets": [
        {"$Type": "Pages$DivContainer", "name": "lvl2", "widgets": [
          {"$Type": "Pages$ActionButton", "name": "btnDeep", "ct:caption": "Deep"}
        ]}
      ]}
    ]}
  ]
}`

func TestPgReadPage_RequestsFullDepth_WhenServerSupportsIt(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return deepPage, false
	})
	f.tools = map[string][]string{
		// The 11.13 shape.
		"pg_read_page": {"moduleName", "pageName", "depth", "paths"},
	}
	b := &Backend{client: f.connectClient(t)}

	if _, err := b.pgReadPage("PgTest", "Deep"); err != nil {
		t.Fatalf("pgReadPage: %v", err)
	}
	call, ok := f.callByName("pg_read_page")
	if !ok {
		t.Fatal("pg_read_page was never called")
	}
	got, ok := call.Args["depth"]
	if !ok {
		t.Fatal("depth was not sent; an 11.13 server would truncate the page to depth 4")
	}
	if n, _ := got.(float64); int(n) != pgReadFullDepth {
		t.Fatalf("depth = %v, want %d", got, pgReadFullDepth)
	}
}

func TestPgReadPage_OmitsDepth_OnOlderServers(t *testing.T) {
	// 11.11/11.12 declare pg_read_page additionalProperties:false without a
	// `depth` property, so sending one would fail the whole call.
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if _, sent := args["depth"]; sent {
			return "unknown argument 'depth'", true
		}
		return deepPage, false
	})
	f.tools = map[string][]string{"pg_read_page": {"moduleName", "pageName"}}
	b := &Backend{client: f.connectClient(t)}

	if _, err := b.pgReadPage("PgTest", "Deep"); err != nil {
		t.Fatalf("pgReadPage against a pre-11.13 server: %v", err)
	}
	call, _ := f.callByName("pg_read_page")
	if _, sent := call.Args["depth"]; sent {
		t.Fatal("depth must not be sent to a server that does not advertise it")
	}
}

func TestPgReadPage_RefusesTruncatedPage(t *testing.T) {
	// What 11.13 actually returned for Administration.Account_Overview at the
	// default depth — the entire widget tree replaced by sentinels.
	const truncated = `{"title":"Accounts","layout":"Atlas_Core.Atlas_Default",
	  "parameters":[],"variables":[],
	  "widgets":[{"$Type":"Pages$Content","slot":"Main","widgets":["...","..."]}]}`
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return truncated, false
	})
	f.tools = map[string][]string{"pg_read_page": {"moduleName", "pageName", "depth"}}
	b := &Backend{client: f.connectClient(t)}

	_, err := b.pgReadPage("Administration", "Account_Overview")
	if err == nil {
		t.Fatal("expected a truncated page to be refused; returning it lets ALTER PAGE write placeholders over real widgets")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error should name the cause, got: %v", err)
	}
}

func TestHasTruncationSentinel(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want bool
	}{
		{"clean page", deepPage, false},
		{"sentinel in widgets array", `{"widgets":["..."]}`, true},
		{"sentinel nested deep", `{"a":{"b":[{"c":["..."]}]}}`, true},
		// A page may legitimately contain "..." as text; only a sentinel
		// standing where an element belongs (an array item) counts.
		{"ellipsis caption is not truncation", `{"widgets":[{"ct:caption":"..."}]}`, false},
		{"ellipsis title is not truncation", `{"title":"..."}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tc.doc), &v); err != nil {
				t.Fatalf("bad fixture: %v", err)
			}
			if got := hasTruncationSentinel(v); got != tc.want {
				t.Fatalf("hasTruncationSentinel = %v, want %v", got, tc.want)
			}
		})
	}
}
