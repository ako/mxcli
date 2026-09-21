// SPDX-License-Identifier: Apache-2.0

package ast

import "github.com/mendixlabs/mxcli/mdl/types"

// FlowFlavour is which kind of flow document a statement targets.
type FlowFlavour string

const (
	FlowMicroflow FlowFlavour = "microflow"
	FlowNanoflow  FlowFlavour = "nanoflow"
	FlowRule      FlowFlavour = "rule"
)

// AlterFlowActivitiesStmt is
//
//	ALTER MICROFLOW Mod.Flow  DISABLE ACTIVITIES WHERE …
//	ALTER MICROFLOWS [IN Mod] ENABLE  ACTIVITIES WHERE …
//
// It is Studio Pro's right-click Disable applied to a flow that already exists,
// as opposed to `@disabled`, which states the flag while authoring one.
//
// The two are not interchangeable. `create or modify microflow` rewrites the
// whole document from MDL, so it is only as faithful as what MDL can spell —
// and the flows someone wants to reach into are exactly the ones holding
// constructs it cannot. This statement changes one boolean on the STORED
// document and leaves every other byte alone.
type AlterFlowActivitiesStmt struct {
	Flavour FlowFlavour

	// Name is set for the singular form. Empty for the bulk form.
	Name QualifiedName
	// Bulk is true for ALTER MICROFLOWS; Module then scopes it, and an empty
	// Module means the whole project.
	Bulk   bool
	Module string

	// Disable is true for DISABLE, false for ENABLE.
	Disable bool

	// Filter selects the activities. Never empty: the grammar requires WHERE,
	// and a filter with no condition is refused rather than treated as "all"
	// (see types.CheckActivityFilter).
	Filter types.ActivityFilter
}

func (s *AlterFlowActivitiesStmt) isStatement() {}
