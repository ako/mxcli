// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatOQLTable(t *testing.T) {
	result := &OQLResult{
		Columns: []string{"Name", "Age", "City"},
		Rows: [][]any{
			{"Alice", "30", "Amsterdam"},
			{"Bob", "25", "Berlin"},
			{nil, "42", "Copenhagen"},
		},
	}

	var buf bytes.Buffer
	FormatOQLTable(&buf, result)
	output := buf.String()

	// Check header
	if !strings.Contains(output, "| Name") {
		t.Errorf("missing Name header in:\n%s", output)
	}
	if !strings.Contains(output, "| Age") {
		t.Errorf("missing Age header in:\n%s", output)
	}
	if !strings.Contains(output, "| City") {
		t.Errorf("missing City header in:\n%s", output)
	}

	// Check separator
	if !strings.Contains(output, "|---") {
		t.Errorf("missing separator in:\n%s", output)
	}

	// Check data
	if !strings.Contains(output, "Alice") {
		t.Errorf("missing Alice in:\n%s", output)
	}
	if !strings.Contains(output, "NULL") {
		t.Errorf("nil should be displayed as NULL in:\n%s", output)
	}
}

func TestFormatOQLTableEmpty(t *testing.T) {
	result := &OQLResult{
		Columns: []string{"Name"},
		Rows:    nil,
	}

	var buf bytes.Buffer
	FormatOQLTable(&buf, result)
	output := buf.String()

	// Should have header and separator but no data rows
	if !strings.Contains(output, "Name") {
		t.Errorf("missing header in:\n%s", output)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines (header + separator), got %d:\n%s", len(lines), output)
	}
}

func TestFormatOQLTableNoColumns(t *testing.T) {
	result := &OQLResult{}

	var buf bytes.Buffer
	FormatOQLTable(&buf, result)
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty result, got %q", buf.String())
	}
}

func TestFormatOQLJSON(t *testing.T) {
	result := &OQLResult{
		Columns: []string{"Name", "Count"},
		Rows: [][]any{
			{"Alice", "10"},
			{"Bob", nil},
		},
	}

	var buf bytes.Buffer
	if err := FormatOQLJSON(&buf, result); err != nil {
		t.Fatalf("FormatOQLJSON: %v", err)
	}

	var parsed []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(parsed))
	}
	if parsed[0]["Name"] != "Alice" {
		t.Errorf("first row Name: got %v, want Alice", parsed[0]["Name"])
	}
	if parsed[1]["Count"] != nil {
		t.Errorf("second row Count should be nil, got %v", parsed[1]["Count"])
	}
}

func TestFormatOQLJSONEmpty(t *testing.T) {
	result := &OQLResult{
		Columns: []string{"Name"},
		Rows:    nil,
	}

	var buf bytes.Buffer
	if err := FormatOQLJSON(&buf, result); err != nil {
		t.Fatalf("FormatOQLJSON: %v", err)
	}

	var parsed []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty array, got %d objects", len(parsed))
	}
}

func TestExecuteOQL_Success(t *testing.T) {
	expectedAuth := m2eeAuthHeader("testpass")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pre-11.11 runtime: no dev endpoint, so ExecuteOQL falls back to the
		// legacy action API at "/", which this test exercises.
		if r.URL.Path == devPreviewOQLPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-M2EE-Authentication") != expectedAuth {
			t.Errorf("wrong auth header: got %q, want %q", r.Header.Get("X-M2EE-Authentication"), expectedAuth)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("wrong content type: %s", r.Header.Get("Content-Type"))
		}

		// Verify request body
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["action"] != "preview_execute_oql" {
			t.Errorf("wrong action: %v", body["action"])
		}

		// Return success response matching M2EE feedback envelope format
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":0,"feedback":{"data":[{"Name":"Alice","Age":"30"},{"Name":"Bob","Age":"25"}]}}`))
	}))
	defer server.Close()

	// Parse server URL to get host and port
	host, port := parseTestServerAddr(t, server.URL)

	opts := OQLOptions{
		Host:   host,
		Port:   port,
		Token:  "testpass",
		Direct: true,
	}

	result, err := ExecuteOQL(opts, "SELECT Name, Age FROM Test.Person")
	if err != nil {
		t.Fatalf("ExecuteOQL: %v", err)
	}

	if len(result.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(result.Columns))
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != "Alice" {
		t.Errorf("first row Name: got %v, want Alice", result.Rows[0][0])
	}
}

func TestExecuteOQL_OQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == devPreviewOQLPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := map[string]any{
			"result": 1,
			"cause":  "Entity 'NonExistent.Foo' is unknown",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	host, port := parseTestServerAddr(t, server.URL)

	opts := OQLOptions{
		Host:   host,
		Port:   port,
		Token:  "testpass",
		Direct: true,
	}

	_, err := ExecuteOQL(opts, "SELECT * FROM NonExistent.Foo")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Entity 'NonExistent.Foo' is unknown") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecuteOQL_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	host, port := parseTestServerAddr(t, server.URL)

	opts := OQLOptions{
		Host:   host,
		Port:   port,
		Token:  "wrongpass",
		Direct: true,
	}

	_, err := ExecuteOQL(opts, "SELECT 1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecuteOQL_EmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == devPreviewOQLPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":0,"feedback":{"data":[]}}`))
	}))
	defer server.Close()

	host, port := parseTestServerAddr(t, server.URL)

	opts := OQLOptions{
		Host:   host,
		Port:   port,
		Token:  "testpass",
		Direct: true,
	}

	result, err := ExecuteOQL(opts, "SELECT Name FROM Test.Empty")
	if err != nil {
		t.Fatalf("ExecuteOQL: %v", err)
	}
	if len(result.Columns) != 0 {
		t.Errorf("expected 0 columns, got %d", len(result.Columns))
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Rows))
	}
}

func TestExecuteOQL_ColumnOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == devPreviewOQLPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Return JSON with specific key order — using raw JSON to preserve order
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":0,"feedback":{"data":[{"Zebra":"z","Alpha":"a","Middle":"m"}]}}`))
	}))
	defer server.Close()

	host, port := parseTestServerAddr(t, server.URL)

	opts := OQLOptions{
		Host:   host,
		Port:   port,
		Token:  "testpass",
		Direct: true,
	}

	result, err := ExecuteOQL(opts, "SELECT Zebra, Alpha, Middle FROM Test.T")
	if err != nil {
		t.Fatalf("ExecuteOQL: %v", err)
	}

	// Column order should match JSON key order: Zebra, Alpha, Middle
	expected := []string{"Zebra", "Alpha", "Middle"}
	if len(result.Columns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(result.Columns))
	}
	for i, want := range expected {
		if result.Columns[i] != want {
			t.Errorf("column %d: got %q, want %q", i, result.Columns[i], want)
		}
	}
}

// parseTestServerAddr extracts host and port from an httptest server URL.
func parseTestServerAddr(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	// rawURL is like "http://127.0.0.1:PORT"
	addr := strings.TrimPrefix(rawURL, "http://")
	idx := strings.LastIndexByte(addr, ':')
	if idx < 0 {
		t.Fatalf("no port in test server URL: %s", rawURL)
	}
	host := addr[:idx]
	var port int
	if _, err := fmt.Sscanf(addr[idx+1:], "%d", &port); err != nil {
		t.Fatalf("parsing port from test server URL %s: %v", rawURL, err)
	}
	return host, port
}

// TestExecuteOQL_DevEndpoint verifies the Mendix 11.11+ path: ExecuteOQL POSTs
// the params directly to /dev/preview_execute_oql and parses the {"data":[...]}
// response (no action/params envelope, no feedback wrapper).
func TestExecuteOQL_DevEndpoint(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("X-M2EE-Authentication")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"LanguageCode":"en_US"}]}`))
	}))
	defer server.Close()

	host, port := parseTestServerAddr(t, server.URL)

	result, err := ExecuteOQL(OQLOptions{
		Host: host, Port: port, Token: "testpass", Direct: true,
	}, "SELECT l.Code FROM System.Language l")
	if err != nil {
		t.Fatalf("ExecuteOQL: %v", err)
	}

	if gotPath != "/dev/preview_execute_oql" {
		t.Errorf("path: got %q, want /dev/preview_execute_oql", gotPath)
	}
	// Body must be the params directly, NOT wrapped in {"action","params"}.
	if _, wrapped := gotBody["action"]; wrapped {
		t.Errorf("body should not contain an action envelope: %v", gotBody)
	}
	if gotBody["oql"] != "SELECT l.Code FROM System.Language l" {
		t.Errorf("body.oql: got %v", gotBody["oql"])
	}
	if gotBody["numberHandling"] != "asString" {
		t.Errorf("body.numberHandling: got %v", gotBody["numberHandling"])
	}
	if want := base64Encode("testpass"); gotAuth != want {
		t.Errorf("auth header: got %q, want %q", gotAuth, want)
	}
	if len(result.Columns) != 1 || result.Columns[0] != "LanguageCode" {
		t.Errorf("columns: got %v, want [LanguageCode]", result.Columns)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "en_US" {
		t.Errorf("rows: got %v, want [[en_US]]", result.Rows)
	}
}

// TestExecuteOQL_DevEndpointError verifies that a query failure on the 11.11 dev
// endpoint -- reported as HTTP 200 with {"error":"..."} and no data -- surfaces
// as an error rather than a silent empty result.
func TestExecuteOQL_DevEndpointError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Verbatim shape captured from Studio Pro 11.11 (division by zero).
		w.Write([]byte(`{"error":"An exception has occurred for the following request(s):\n\tInternalOqlTextGetRequest (depth = -1): SELECT (1:0) FROM System.Language"}`))
	}))
	defer server.Close()

	host, port := parseTestServerAddr(t, server.URL)

	_, err := ExecuteOQL(OQLOptions{
		Host: host, Port: port, Token: "testpass", Direct: true,
	}, "SELECT (1:0) FROM System.Language")
	if err == nil {
		t.Fatal("expected error for failing query")
	}
	if !strings.Contains(err.Error(), "An exception has occurred") {
		t.Errorf("error should carry the runtime message, got: %v", err)
	}
}

// TestExecuteOQL_FallbackToLegacy verifies that when the dev endpoint 404s
// (pre-11.11 runtime), ExecuteOQL falls back to the legacy preview_execute_oql
// action at POST / and still parses the feedback envelope.
func TestExecuteOQL_FallbackToLegacy(t *testing.T) {
	var legacyHit bool
	var legacyBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dev/preview_execute_oql" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Legacy action endpoint at "/".
		legacyHit = true
		json.NewDecoder(r.Body).Decode(&legacyBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":0,"feedback":{"data":[{"LanguageCode":"en_US"}]}}`))
	}))
	defer server.Close()

	host, port := parseTestServerAddr(t, server.URL)

	result, err := ExecuteOQL(OQLOptions{
		Host: host, Port: port, Token: "testpass", Direct: true,
	}, "SELECT 1")
	if err != nil {
		t.Fatalf("ExecuteOQL: %v", err)
	}

	if !legacyHit {
		t.Fatal("expected fallback to legacy action endpoint")
	}
	if legacyBody["action"] != "preview_execute_oql" {
		t.Errorf("legacy body.action: got %v, want preview_execute_oql", legacyBody["action"])
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "en_US" {
		t.Errorf("rows: got %v, want [[en_US]]", result.Rows)
	}
}

func TestSplitCurlStatus(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantBody   string
		wantStatus int
		wantOK     bool
	}{
		{"body and status", "{\"data\":[]}\n200", "{\"data\":[]}", 200, true},
		{"trailing newline", "{\"data\":[]}\n200\n", "{\"data\":[]}", 200, true},
		{"404 empty body", "\n404", "", 404, true},
		{"status only", "404", "", 404, true},
		{"body with internal newline", "line1\nline2\n200", "line1\nline2", 200, true},
		{"connection failure 000", "000", "", 0, true},
		{"no numeric status", "not a number", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, status, ok := splitCurlStatus([]byte(tt.in))
			if ok != tt.wantOK {
				t.Fatalf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if string(body) != tt.wantBody {
				t.Errorf("body: got %q, want %q", body, tt.wantBody)
			}
			if status != tt.wantStatus {
				t.Errorf("status: got %d, want %d", status, tt.wantStatus)
			}
		})
	}
}

// base64Encode mirrors m2eeAuthHeader's encoding for test assertions.
func base64Encode(s string) string {
	return m2eeAuthHeader(s)
}

func TestOqlDevError(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    string // "" = no error; otherwise a substring the result must contain
		wantErr bool
	}{
		{"valid rows", `{"data":[{"N":"3"}]}`, "", false},
		{"valid empty", `{"data":[]}`, "", false},
		{"query error", `{"error":"parse error near FROM"}`, "parse error", true},
		{"action not found", `{"result":-5,"message":"Action not found."}`, "not found", true},
		{"action not found hint", `{"result":-5,"message":"Action not found."}`, "live-preview", true},
		{"auth failed", `{"result":-4,"message":"Authentication failed."}`, "Authentication failed", true},
		{"empty body", ``, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := oqlDevError([]byte(c.body))
			if c.wantErr && got == "" {
				t.Fatalf("expected an error message, got empty")
			}
			if !c.wantErr && got != "" {
				t.Fatalf("expected no error, got %q", got)
			}
			if c.want != "" && !contains(got, c.want) {
				t.Errorf("message %q does not contain %q", got, c.want)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
