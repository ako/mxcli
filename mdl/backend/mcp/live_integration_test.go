// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"os"
	"strings"
	"testing"
)

// TestLive_EntityRoundTrip exercises the real PED entity choreography against a
// running Studio Pro MCP server. It is skipped unless MXCLI_MCP_URL is set.
//
// Example (Studio Pro open, project has an empty user module "MyFirstModule"):
//
//	MXCLI_MCP_URL=http://localhost/mcp \
//	MXCLI_MCP_DIAL=host.docker.internal:7782 \
//	MXCLI_MCP_MODULE=MyFirstModule \
//	go test ./mdl/backend/mcp/ -run TestLive -v
//
// The test creates a uniquely-named entity, validates it, reads it back, and
// removes it, leaving the model as it found it.
func TestLive_EntityRoundTrip(t *testing.T) {
	url := os.Getenv("MXCLI_MCP_URL")
	if url == "" {
		t.Skip("set MXCLI_MCP_URL to run the live MCP integration test")
	}
	module := os.Getenv("MXCLI_MCP_MODULE")
	if module == "" {
		module = "MyFirstModule"
	}

	c, err := NewClient(ClientOptions{URL: url, Dial: os.Getenv("MXCLI_MCP_DIAL")})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	si, err := c.Initialize()
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Logf("connected to %s %s", si.Name, si.Version)

	b := &Backend{client: c}
	const name = "MxcliMcpProbe"

	// create
	entity := newPersistentEntity(name, attr("Title", stringType{}))
	if err := b.ensureSchema("DomainModels$Entity", "DomainModels$Attribute"); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	value, err := b.buildEntityValue(entity)
	if err != nil {
		t.Fatalf("buildEntityValue: %v", err)
	}
	if err := b.pedUpdate(module, pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "add", Value: value}}); err != nil {
		t.Fatalf("add entity: %v", err)
	}

	// validate + read back
	if err := b.pedCheckErrors(module); err != nil {
		t.Errorf("check errors after create: %v", err)
	}
	idx, err := b.entityIndex(module, name)
	if err != nil {
		t.Fatalf("entity not found after create: %v", err)
	}
	t.Logf("created %s.%s at /entities/%d", module, name, idx)

	// cleanup
	if err := b.pedUpdate(module, pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "remove", Index: &idx}}); err != nil {
		t.Fatalf("remove entity: %v", err)
	}
	if _, err := b.entityIndex(module, name); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("entity should be gone after remove, got: %v", err)
	}
}

// stringType is a tiny AttributeType for the live test (avoids importing the
// full domainmodel constructors here).
type stringType struct{}

func (stringType) GetTypeName() string { return "String" }
