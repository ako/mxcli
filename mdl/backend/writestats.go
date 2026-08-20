// SPDX-License-Identifier: Apache-2.0

package backend

// WriteStats counts what a session's unit writes actually did to storage.
//
// Storage does not write a unit whose new content is semantically equal to what
// is stored (ADR-0008), so "the handler called UpdateMicroflow and got no error"
// does not mean anything reached disk. Without a way to tell the two apart, a
// re-run of an already-applied script announces "Modified …"/"Replaced …" for
// every statement while changing nothing — which is exactly how the churn in
// #910 was first mis-diagnosed from console output.
//
// Offered counts writes handed to storage; Written counts those that were not
// elided. Both are cumulative for the life of the backend, so a caller measures
// one statement by taking the difference across it.
type WriteStats struct {
	Offered int
	Written int
}

// WriteStatsReporter is implemented by backends that can report the above.
//
// Deliberately not part of FullBackend: it says nothing about the model, only
// about what a particular storage engine did, and a backend with no notion of
// units (a mock, a future MCP/PED backend) has no honest answer. Callers
// type-assert and fall back to reporting the mutation unqualified — the
// behaviour that existed before this interface.
type WriteStatsReporter interface {
	WriteStats() WriteStats
}
