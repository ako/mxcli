// SPDX-License-Identifier: Apache-2.0

// Package mcp implements a backend.FullBackend that executes model writes
// against a live Studio Pro instance via its MCP ("PED") server, while
// serving reads from the local mounted .mpr file.
//
// This is the first vertical slice (domain-model entities) described in
// docs/11-proposals/PROPOSAL_mcp_backend.md. Operations outside the slice
// return errUnsupported via the generated unsupportedBackend base.
package mcp

import "fmt"

//go:generate go run gen_unsupported.go

// errUnsupported reports that an operation is not implemented by (or not
// expressible through) the MCP backend. Such operations must run against a local
// .mpr (drop the --mcp flag).
func errUnsupported(op string) error {
	return fmt.Errorf("%s: not supported by the MCP backend; run without --mcp to author against a local .mpr", op)
}
