// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestNanoflowCreateRejected(t *testing.T) {
	b := &Backend{}
	nf := &microflows.Nanoflow{Name: "NF_Greet"}
	for _, err := range []error{b.CreateNanoflow(nf), b.UpdateNanoflow(nf)} {
		if err == nil || !strings.Contains(err.Error(), "cannot be created") {
			t.Errorf("nanoflow create should be rejected with a clear error, got %v", err)
		}
	}
}
