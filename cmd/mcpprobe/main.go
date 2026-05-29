// Command mcpprobe is a minimal MCP "streamable HTTP" client used to talk to a
// Studio Pro MCP (PED) server from inside a devcontainer.
//
// It exists because curl cannot complete the MCP handshake: the server may
// answer a single POST with either application/json or a text/event-stream
// (SSE) body, it assigns a session via the Mcp-Session-Id response header that
// must be echoed on subsequent calls, and Studio Pro's DNS-rebinding guard
// rejects any request whose Host header is not "localhost". This tool handles
// all three, so we can dump tools/list and experiment with ped_* calls during
// Phase 0 reconnaissance (see docs/11-proposals/PROPOSAL_mcp_backend.md).
//
// Usage:
//
//	go run ./cmd/mcpprobe                          # tools/list against localhost:7782
//	go run ./cmd/mcpprobe -method tools/list
//	go run ./cmd/mcpprobe -method tools/call -params '{"name":"ped_get_schema","arguments":{"types":["DomainModels$Entity"]}}'
//	go run ./cmd/mcpprobe -url http://localhost:7782/mcp -dial host.docker.internal:7782
//
// From a devcontainer the server runs on the host: -dial defaults to
// host.docker.internal:<port> while the Host header stays localhost.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type client struct {
	http      *http.Client
	endpoint  string // URL the server sees (drives the Host header)
	sessionID string
	protocol  string
	verbose   bool
	nextID    int
}

func main() {
	var (
		rawURL   = flag.String("url", "http://localhost:7782/mcp", "MCP endpoint URL as the server sees it (sets the Host header)")
		dial     = flag.String("dial", "", "host:port to actually connect to (default: host.docker.internal:<port> when url host is localhost)")
		method   = flag.String("method", "tools/list", "JSON-RPC method to call after initialize (empty = just initialize)")
		params   = flag.String("params", "", "JSON params for the method (e.g. '{\"name\":\"ped_get_schema\",...}')")
		protocol = flag.String("protocol", "2025-06-18", "MCP protocol version to advertise")
		timeout  = flag.Duration("timeout", 20*time.Second, "per-request timeout")
		verbose  = flag.Bool("v", false, "verbose: print raw request/response framing")
	)
	flag.Parse()

	u, err := url.Parse(*rawURL)
	if err != nil {
		fatal("invalid -url: %v", err)
	}

	dialAddr := *dial
	if dialAddr == "" {
		dialAddr = defaultDial(u.Host)
	}

	c := &client{
		http:     newHTTPClient(dialAddr, *timeout),
		endpoint: *rawURL,
		protocol: *protocol,
		verbose:  *verbose,
	}

	logf("→ dialing %s, Host: %s, endpoint %s", dialAddr, u.Host, *rawURL)

	// 1. initialize
	initRes, err := c.call("initialize", map[string]any{
		"protocolVersion": *protocol,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mcpprobe", "version": "0.1.0"},
	})
	if err != nil {
		fatal("initialize failed: %v", err)
	}
	fmt.Println("=== initialize result ===")
	printJSON(initRes.Result)
	if c.sessionID != "" {
		logf("session: %s", c.sessionID)
	}

	// 2. notifications/initialized (notification — no id, no response expected)
	if err := c.notify("notifications/initialized", nil); err != nil {
		logf("warning: notifications/initialized: %v", err)
	}

	if *method == "" {
		return
	}

	// 3. the requested call
	var p any
	if strings.TrimSpace(*params) != "" {
		if err := json.Unmarshal([]byte(*params), &p); err != nil {
			fatal("invalid -params JSON: %v", err)
		}
	}
	res, err := c.call(*method, p)
	if err != nil {
		fatal("%s failed: %v", *method, err)
	}
	fmt.Printf("=== %s result ===\n", *method)
	if res.Error != nil {
		b, _ := json.MarshalIndent(res.Error, "", "  ")
		fmt.Printf("ERROR: %s\n", b)
		os.Exit(1)
	}
	printJSON(res.Result)
}

// defaultDial maps a localhost endpoint to the Docker host gateway so the probe
// works from inside a devcontainer, while the Host header still says localhost.
func defaultDial(host string) string {
	h, port, err := net.SplitHostPort(host)
	if err != nil { // no port
		h, port = host, "80"
	}
	if h == "localhost" || h == "127.0.0.1" {
		return net.JoinHostPort("host.docker.internal", port)
	}
	return host
}

func newHTTPClient(dialAddr string, timeout time.Duration) *http.Client {
	// Pin every connection to dialAddr regardless of the request URL host, so
	// the URL (and thus the Host header) can stay "localhost" while we actually
	// connect to the host gateway.
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, dialAddr)
		},
		ForceAttemptHTTP2: false,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

// call sends a JSON-RPC request and returns the matching response, handling both
// application/json and text/event-stream (SSE) reply framing.
func (c *client) call(method string, params any) (*rpcResponse, error) {
	c.nextID++
	id := c.nextID
	body := rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params}
	resp, err := c.send(body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	c.captureSession(resp)

	ct := resp.Header.Get("Content-Type")
	logf("← %s  Content-Type: %s", resp.Status, ct)
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	switch {
	case strings.HasPrefix(ct, "text/event-stream"):
		return c.readSSEResponse(resp.Body, id)
	case strings.HasPrefix(ct, "application/json"):
		var r rpcResponse
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			return nil, fmt.Errorf("decode json response: %w", err)
		}
		return &r, nil
	default:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("unexpected Content-Type %q, body: %s", ct, strings.TrimSpace(string(b)))
	}
}

// notify sends a JSON-RPC notification (no id); the server replies 202 with no
// JSON-RPC payload, which we simply drain.
func (c *client) notify(method string, params any) error {
	resp, err := c.send(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.captureSession(resp)
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	logf("← %s (notification)", resp.Status)
	return nil
}

func (c *client) send(payload rpcRequest) (*http.Response, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if c.verbose {
		logf("→ %s", buf)
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", c.protocol)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	return c.http.Do(req)
}

func (c *client) captureSession(resp *http.Response) {
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" && c.sessionID == "" {
		c.sessionID = id
	}
}

// readSSEResponse scans an SSE stream for the first JSON-RPC message whose id
// matches wantID (server-to-client requests/notifications in between are logged
// and skipped).
func (c *client) readSSEResponse(r io.Reader, wantID int) (*rpcResponse, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var data strings.Builder
	flush := func() (*rpcResponse, bool, error) {
		if data.Len() == 0 {
			return nil, false, nil
		}
		raw := data.String()
		data.Reset()
		if c.verbose {
			logf("← sse data: %s", raw)
		}
		var probe struct {
			ID     *int `json:"id"`
			Method *string `json:"method"`
		}
		_ = json.Unmarshal([]byte(raw), &probe)
		if probe.ID != nil && *probe.ID == wantID {
			var resp rpcResponse
			if err := json.Unmarshal([]byte(raw), &resp); err != nil {
				return nil, false, fmt.Errorf("decode sse json: %w", err)
			}
			return &resp, true, nil
		}
		if probe.Method != nil {
			logf("(server message: %s)", *probe.Method)
		}
		return nil, false, nil
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "": // event boundary
			if resp, done, err := flush(); err != nil || done {
				return resp, err
			}
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		case strings.HasPrefix(line, ":"), strings.HasPrefix(line, "event:"), strings.HasPrefix(line, "id:"), strings.HasPrefix(line, "retry:"):
			// comment / metadata lines — ignore
		}
	}
	if resp, done, err := flush(); err != nil || done {
		return resp, err
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read sse: %w", err)
	}
	return nil, fmt.Errorf("stream ended without a response for id %d", wantID)
}

func printJSON(raw json.RawMessage) {
	if len(raw) == 0 {
		fmt.Println("(empty)")
		return
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		fmt.Println(string(raw))
		return
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mcpprobe: "+format+"\n", args...)
	os.Exit(1)
}
