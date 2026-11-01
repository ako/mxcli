// SPDX-License-Identifier: Apache-2.0

package mcp

import "testing"

// handlerSessionError is Studio Pro 11.16.0's verbatim ped_delete_document
// failure, measured live: the tool ran, failed, and still said isError:false.
const handlerSessionError = "Error executing tool handler: Session 'mcp-server' not found"

func TestCallTool_HandlerErrorIsAnError(t *testing.T) {
	f := newFakePED(t, func(string, map[string]any) (string, bool) { return handlerSessionError, false })
	res, err := f.connectClient(t).CallTool("ped_delete_document", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("a %q result must be reported as an error, got IsError=false", handlerSessionError)
	}
}
