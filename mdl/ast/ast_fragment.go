// SPDX-License-Identifier: Apache-2.0

package ast

// DefineFragmentStmt represents: DEFINE FRAGMENT Name [(params)] AS { widgets }
type DefineFragmentStmt struct {
	Name    string
	Params  []FragmentParam // Typed datasource/action parameters (optional)
	Widgets []*WidgetV3
}

func (s *DefineFragmentStmt) isStatement() {}

// FragmentParam is a typed parameter a fragment declares — a datasource or an
// action handler the caller supplies at `use fragment`.
type FragmentParam struct {
	Name string // Parameter name without the leading '$' (e.g. "data")
	Kind string // "datasource" | "action"
}

// FragmentArg is a value supplied for a fragment parameter at a `use fragment`
// site. Exactly one of DataSource/Action is set; the executor picks which by the
// parameter's declared Kind (they overlap on microflow/nanoflow at parse time).
type FragmentArg struct {
	Name       string
	DataSource *DataSourceV3
	Action     *ActionV3
}

// DescribeFragmentFromStmt represents DESCRIBE FRAGMENT FROM PAGE/SNIPPET ... WIDGET ...
type DescribeFragmentFromStmt struct {
	ContainerType string        // "PAGE" or "SNIPPET"
	ContainerName QualifiedName // Module.PageName or Module.SnippetName
	WidgetName    string        // Target widget name
}

func (s *DescribeFragmentFromStmt) isStatement() {}
