// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePED is a minimal stand-in for Studio Pro's MCP server. It records every
// tools/call it receives and lets the test script its responses.
type fakePED struct {
	srv     *httptest.Server
	calls   []recordedCall
	respond func(name string, args map[string]any) (text string, isError bool)
	// rpcErr, when set and returning ok=true, makes the tools/call answer with a
	// JSON-RPC error object (e.g. Studio Pro's -32000 "Request timed out")
	// instead of a tool result.
	rpcErr func(name string, args map[string]any) (code int, msg string, ok bool)
}

type recordedCall struct {
	Name string
	Args map[string]any
}

func newFakePED(t *testing.T, respond func(name string, args map[string]any) (string, bool)) *fakePED {
	t.Helper()
	f := &fakePED{respond: respond}
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &req)

		w.Header().Set("Mcp-Session-Id", "test-session")
		if req.ID == nil { // notification
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2025-06-18",
				"serverInfo":      map[string]any{"name": "fake-studio-pro", "version": "1.0.0"},
			}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			f.calls = append(f.calls, recordedCall{Name: p.Name, Args: p.Arguments})
			if f.rpcErr != nil {
				if code, msg, ok := f.rpcErr(p.Name, p.Arguments); ok {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"jsonrpc": "2.0", "id": req.ID,
						"error": map[string]any{"code": code, "message": msg},
					})
					return
				}
			}
			text, isErr := "OK", false
			if f.respond != nil {
				text, isErr = f.respond(p.Name, p.Arguments)
			}
			result = map[string]any{
				"isError": isErr,
				"content": []map[string]any{{"type": "text", "text": text}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID, "result": result,
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// connect builds a Backend wired to the fake server (no local reader needed for
// the helper-level tests, which exercise only the PED call shapes).
func (f *fakePED) connectClient(t *testing.T) *Client {
	t.Helper()
	addr := strings.TrimPrefix(f.srv.URL, "http://")
	c, err := NewClient(ClientOptions{URL: f.srv.URL + "/mcp", Dial: addr})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

func (f *fakePED) callByName(name string) (recordedCall, bool) {
	for _, c := range f.calls {
		if c.Name == name {
			return c, true
		}
	}
	return recordedCall{}, false
}

func TestClient_HandshakeAndToolCall(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return "SUCCESS: Applying operations (1)", false
	})
	c := f.connectClient(t)
	res, err := c.CallTool("ped_update_document", map[string]any{"documentName": "X"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError || !strings.Contains(res.Text, "SUCCESS") {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestPedUpdate_SendsCorrectAddOperation(t *testing.T) {
	f := newFakePED(t, func(string, map[string]any) (string, bool) { return "SUCCESS", false })
	b := &Backend{client: f.connectClient(t)}

	err := b.pedUpdate("MyFirstModule", pedOpEntry{
		Path: "/entities",
		Operation: pedOperation{Type: "add", Value: &pedEntity{
			SType: "DomainModels$Entity", Name: "Order",
			Attributes: []pedAttribute{{SType: "DomainModels$Attribute", Name: "Total", Type: "Decimal"}},
		}},
	})
	if err != nil {
		t.Fatalf("pedUpdate: %v", err)
	}

	call, ok := f.callByName("ped_update_document")
	if !ok {
		t.Fatal("ped_update_document was not called")
	}
	if call.Args["documentType"] != "DomainModels$DomainModel" {
		t.Errorf("documentType = %v", call.Args["documentType"])
	}
	if call.Args["documentName"] != "MyFirstModule" {
		t.Errorf("documentName = %v", call.Args["documentName"])
	}
	// Round-trip the operations through JSON to assert their shape.
	raw, _ := json.Marshal(call.Args["operations"])
	for _, want := range []string{
		`"path":"/entities"`, `"type":"add"`,
		`"$Type":"DomainModels$Entity"`, `"name":"Order"`, `"type":"Decimal"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("operations missing %s: %s", want, raw)
		}
	}
}

func TestPedCheckErrors_SurfacesValidationFailure(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_check_errors" {
			return "Entity 'Order' has an error", true
		}
		return "OK", false
	})
	b := &Backend{client: f.connectClient(t)}

	err := b.pedCheckErrors("MyFirstModule")
	if err == nil || !strings.Contains(err.Error(), "Order") {
		t.Fatalf("expected validation error surfaced, got: %v", err)
	}
}

func TestEntityIndex_FindsByName(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		// Shape mirrors a real ped_read_document /entities response.
		return `{"results":[{"path":"/entities","result":[{"name":"Alpha"},{"name":"Beta"},{"name":"Gamma"}]}]}`, false
	})
	b := &Backend{client: f.connectClient(t)}

	idx, err := b.entityIndex("MyFirstModule", "Gamma")
	if err != nil {
		t.Fatalf("entityIndex: %v", err)
	}
	if idx != 2 {
		t.Fatalf("entityIndex = %d, want 2", idx)
	}
	if _, err := b.entityIndex("MyFirstModule", "Missing"); err == nil {
		t.Error("expected error for missing entity")
	}
}
