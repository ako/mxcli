// SPDX-License-Identifier: Apache-2.0

// caps.go - the size limits, and where they bite.
//
// Caps are per shard rather than per store. A single file would make the cap a
// project-wide budget, so recording a Sales decision would compete with a
// Finance one and promotion would start refusing on exactly the projects that
// most need the store (PROPOSAL_project_brain.md §4.1).
package brain

// Caps are measured in lines, because lines are what an agent pays for when a
// shard is loaded into context.
//
// project.md is the tightest: it is the only file loaded unconditionally, so
// every line in it is charged to every session on the project. A module shard
// is charged only to sessions touching that module, which is what buys it the
// larger allowance.
const (
	ProjectShardCap = 120
	ModuleShardCap  = 240
)

// CapFor returns the line budget for a shard.
func CapFor(shard string) int {
	if shard == ProjectShard {
		return ProjectShardCap
	}
	return ModuleShardCap
}

// Usage is what `brain show` reports. Every field is computed on the call —
// none of it is ever written into a committed file, because a figure in prose
// is stale the next time anyone promotes (A6).
type Usage struct {
	Shard   string
	Entries int
	Lines   int
	Cap     int
}

// Headroom is the number of lines still available. It goes negative for a shard
// that was edited past its cap by hand, which `show` reports rather than hides.
func (u Usage) Headroom() int { return u.Cap - u.Lines }

// Over reports whether the shard is past its cap.
func (u Usage) Over() bool { return u.Lines > u.Cap }
