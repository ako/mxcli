// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// OQLOptions configures OQL query execution against a running Mendix runtime.
type OQLOptions struct {
	// Host is the hostname of the Mendix admin API (default: localhost).
	Host string

	// Port is the admin API port (default: 8090).
	Port int

	// Token is the M2EE admin password for authentication.
	Token string

	// ProjectPath is the path to the .mpr file (used to find .docker/.env).
	ProjectPath string

	// Direct bypasses docker exec and connects to the admin API directly.
	// By default (when false and ProjectPath is set), the request is routed
	// through "docker compose exec" to reach the container's loopback interface.
	Direct bool

	// Stdout for output.
	Stdout io.Writer

	// Stderr for status messages.
	Stderr io.Writer
}

// OQLResult holds the result of an OQL query execution.
type OQLResult struct {
	Columns []string
	Rows    [][]any
}

// ExecuteOQL runs an OQL query against the Mendix admin API using preview_execute_oql.
//
// By default, when ProjectPath is set, the request is routed through
// "docker compose exec" to reach the container's loopback admin API (port 8090
// binds to 127.0.0.1 inside the container and is unreachable from DinD).
// Set Direct=true to connect via HTTP directly (when the admin API is reachable).
func ExecuteOQL(opts OQLOptions, query string) (*OQLResult, error) {
	m2eeOpts := M2EEOptions{
		Host:        opts.Host,
		Port:        opts.Port,
		Token:       opts.Token,
		ProjectPath: opts.ProjectPath,
		Direct:      opts.Direct,
		Timeout:     10 * time.Second,
	}

	params := map[string]any{
		"oql":            query,
		"numberHandling": "asString",
	}

	// Mendix 11.11+ serves OQL preview as a REST endpoint
	// (POST /dev/preview_execute_oql with the params as the body, returning
	// {"data":[...]} directly). Try it first; on older runtimes it 404s and we
	// fall back to the legacy M2EE action (POST / with {"action","params"}).
	raw, err := previewOQLDev(m2eeOpts, params)
	if errors.Is(err, errDevEndpointNotFound) {
		resp, lerr := CallM2EE(m2eeOpts, "preview_execute_oql", params)
		if lerr != nil {
			return nil, lerr
		}
		if errMsg := resp.M2EEError(); errMsg != "" {
			return nil, fmt.Errorf("OQL error: %s", errMsg)
		}
		return parseOQLFeedback(resp.RawFeedback)
	}
	if err != nil {
		return nil, err
	}

	// The dev endpoint reports query failures as HTTP 200 with an {"error":"..."}
	// body (no "data"), so a bad query must be surfaced here rather than parsed
	// as an empty result.
	if errMsg := oqlDevError(raw); errMsg != "" {
		return nil, fmt.Errorf("OQL error: %s", errMsg)
	}

	return parseOQLFeedback(raw)
}

// oqlDevError returns the message from a dev-endpoint error response, or "" when
// the body is a valid result. Two error shapes are handled:
//   - {"error":"..."} — a query error (bad OQL) reported by the preview servlet.
//   - {"result":<non-zero>,"message":"..."} — the admin dispatcher's response
//     when the request never reached the preview servlet, e.g. the OQL preview
//     servlet isn't mounted ("Action not found") because the app wasn't started
//     with the live-preview dev flags, or auth failed. A successful result carries
//     "data" and no "result"; without this check these would be silently parsed
//     as 0 rows.
func oqlDevError(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var env struct {
		Error   string          `json:"error"`
		Result  *int            `json:"result"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return ""
	}
	if env.Error != "" {
		return env.Error
	}
	if len(env.Data) == 0 && env.Result != nil && *env.Result != 0 {
		msg := env.Message
		if msg == "" {
			msg = fmt.Sprintf("runtime returned result %d", *env.Result)
		}
		// "Action not found" means the OQL preview servlet isn't mounted — the app
		// must be started with the live-preview dev flags (mxcli docker does this).
		if strings.Contains(strings.ToLower(msg), "not found") {
			msg += " -- the running app does not expose the OQL preview endpoint; rebuild and restart with `mxcli docker build && mxcli docker up` (it starts the runtime with the live-preview dev flags)"
		}
		return msg
	}
	return ""
}

// parseOQLFeedback extracts OQL results from the raw M2EE feedback JSON,
// preserving column order from the response.
func parseOQLFeedback(rawFeedback json.RawMessage) (*OQLResult, error) {
	if len(rawFeedback) == 0 {
		return &OQLResult{}, nil
	}

	// Parse the feedback to extract the data field as raw JSON
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rawFeedback, &envelope); err != nil {
		return nil, fmt.Errorf("parsing feedback: %w", err)
	}

	if len(envelope.Data) == 0 {
		return &OQLResult{}, nil
	}

	var rows []json.RawMessage
	if err := json.Unmarshal(envelope.Data, &rows); err != nil {
		return nil, fmt.Errorf("parsing result data: %w", err)
	}

	result := &OQLResult{}

	if len(rows) == 0 {
		return result, nil
	}

	// Extract column order from the first row using json.Decoder Token() method
	columns, err := extractColumnOrder(rows[0])
	if err != nil {
		return nil, fmt.Errorf("extracting columns: %w", err)
	}
	result.Columns = columns

	// Parse each row preserving column order
	for _, rawRow := range rows {
		var rowMap map[string]any
		if err := json.Unmarshal(rawRow, &rowMap); err != nil {
			return nil, fmt.Errorf("parsing row: %w", err)
		}

		row := make([]any, len(columns))
		for i, col := range columns {
			row[i] = rowMap[col]
		}
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

// extractColumnOrder uses json.Decoder to preserve key order from a JSON object.
func extractColumnOrder(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))

	// Read opening brace
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected '{', got %v", t)
	}

	var columns []string
	for dec.More() {
		// Read key
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := t.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key, got %T", t)
		}
		columns = append(columns, key)

		// Skip value
		var discard json.RawMessage
		if err := dec.Decode(&discard); err != nil {
			return nil, err
		}
	}

	return columns, nil
}

// FormatOQLTable writes an OQL result as a pipe-delimited table to w.
func FormatOQLTable(w io.Writer, result *OQLResult) {
	if len(result.Columns) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = len(col)
	}
	for _, row := range result.Rows {
		for i, val := range row {
			s := formatOQLValue(val)
			if len(s) > widths[i] {
				widths[i] = len(s)
			}
		}
	}

	// Cap column widths at 50 characters
	for i := range widths {
		if widths[i] > 50 {
			widths[i] = 50
		}
	}

	// Print header
	fmt.Fprint(w, "|")
	for i, col := range result.Columns {
		fmt.Fprintf(w, " %-*s |", widths[i], truncateOQL(col, widths[i]))
	}
	fmt.Fprintln(w)

	// Print separator
	fmt.Fprint(w, "|")
	for _, wid := range widths {
		fmt.Fprintf(w, "-%s-|", strings.Repeat("-", wid))
	}
	fmt.Fprintln(w)

	// Print rows
	for _, row := range result.Rows {
		fmt.Fprint(w, "|")
		for i, val := range row {
			s := formatOQLValue(val)
			fmt.Fprintf(w, " %-*s |", widths[i], truncateOQL(s, widths[i]))
		}
		fmt.Fprintln(w)
	}
}

// FormatOQLJSON writes an OQL result as a JSON array of objects to w.
func FormatOQLJSON(w io.Writer, result *OQLResult) error {
	objects := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		obj := make(map[string]any, len(result.Columns))
		for i, col := range result.Columns {
			obj[col] = row[i]
		}
		objects = append(objects, obj)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(objects)
}

// formatOQLValue formats a value for table display.
func formatOQLValue(val any) string {
	if val == nil {
		return "NULL"
	}
	s := fmt.Sprintf("%v", val)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// truncateOQL truncates a string to max length with ellipsis.
func truncateOQL(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
