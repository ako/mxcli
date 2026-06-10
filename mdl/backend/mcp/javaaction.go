// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// Java actions cannot be authored over MCP. PED refuses to create the
// JavaActions$JavaAction document type outright ("Document type
// 'JavaActions$JavaAction' cannot be created."), because a Java action is backed
// by a .java source file Studio Pro generates and manages — the model document
// can't be conjured standalone. CREATE / CREATE OR MODIFY are therefore rejected
// with an actionable error rather than the generic unsupported message. (DROP of
// an existing Java action is feasible via Concord's delete_document, like enums
// and constants, but is not wired since the document can't be created here in the
// first place.)
//
// Calling a Java action from a microflow is unaffected — that is supported (see
// JavaActionCallAction in microflow.go); only authoring the action document is not.

func (b *Backend) CreateJavaAction(ja *javaactions.JavaAction) error {
	return errJavaActionAuthoring(ja.Name)
}

func (b *Backend) UpdateJavaAction(ja *javaactions.JavaAction) error {
	return errJavaActionAuthoring(ja.Name)
}

func errJavaActionAuthoring(name string) error {
	return fmt.Errorf("java action %q cannot be authored via the MCP backend — Studio Pro's MCP server refuses to create JavaActions$JavaAction documents (a Java action is backed by a .java source file the IDE generates); create it against a local .mpr or in Studio Pro", name)
}
