// SPDX-License-Identifier: Apache-2.0

// Package executor — task queue commands (CREATE/DROP/SHOW/DESCRIBE QUEUE).
//
// A Mendix task queue (Queues$Queue) governs how many instances of a queued
// microflow call run at once, and whether that limit is per runtime instance or
// cluster-wide.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// findQueue returns the queue with the given module-qualified name, or nil.
func findQueue(ctx *ExecContext, moduleName, name string) *types.Queue {
	queues, err := ctx.Backend.ListQueues()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, q := range queues {
		if !strings.EqualFold(q.Name, name) {
			continue
		}
		mod := h.GetModuleName(h.FindModuleID(q.ContainerID))
		if strings.EqualFold(mod, moduleName) {
			return q
		}
	}
	return nil
}

// execCreateQueue handles CREATE [OR REPLACE|MODIFY] QUEUE Module.Name (...).
func execCreateQueue(ctx *ExecContext, s *ast.CreateQueueStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	existing := findQueue(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("queue", s.Name.String())
	}

	containerID := module.ID
	if existing != nil {
		containerID = existing.ContainerID
	}

	q := &types.Queue{
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		Parallelism:   s.Parallelism,
		ClusterWide:   s.ClusterWide,
		ExportLevel:   s.ExportLevel,
	}

	if existing != nil {
		q.ID = existing.ID
		if err := ctx.Backend.UpdateQueue(q); err != nil {
			return mdlerrors.NewBackend("update queue", err)
		}
		fmt.Fprintf(ctx.Output, "Modified queue: %s\n", s.Name.String())
		return nil
	}
	if err := ctx.Backend.CreateQueue(q); err != nil {
		return mdlerrors.NewBackend("create queue", err)
	}
	fmt.Fprintf(ctx.Output, "Created queue: %s\n", s.Name.String())
	return nil
}

// execDropQueue handles DROP QUEUE Module.Name.
func execDropQueue(ctx *ExecContext, s *ast.DropQueueStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	existing := findQueue(ctx, s.Name.Module, s.Name.Name)
	if existing == nil {
		return mdlerrors.NewNotFound("queue", s.Name.String())
	}
	if err := ctx.Backend.DeleteQueue(string(existing.ID)); err != nil {
		return mdlerrors.NewBackend("drop queue", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped queue: %s\n", s.Name.String())
	return nil
}

// execShowQueues handles SHOW|LIST QUEUES [IN Module].
func execShowQueues(ctx *ExecContext, s *ast.ShowQueuesStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	queues, err := ctx.Backend.ListQueues()
	if err != nil {
		return mdlerrors.NewBackend("list queues", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	type row struct{ qualified, parallelism, clusterWide string }
	var rows []row
	for _, q := range queues {
		mod := h.GetModuleName(h.FindModuleID(q.ContainerID))
		if s.Module != "" && !strings.EqualFold(mod, s.Module) {
			continue
		}
		rows = append(rows, row{
			qualified:   mod + "." + q.Name,
			parallelism: q.Parallelism,
			clusterWide: fmt.Sprintf("%t", q.ClusterWide),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].qualified < rows[j].qualified })

	result := &TableResult{
		Columns: []string{"Queue", "Parallelism", "Cluster Wide"},
		Summary: fmt.Sprintf("(%d queue(s))", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualified, r.parallelism, r.clusterWide})
	}
	return writeResult(ctx, result)
}

// execDescribeQueue handles DESCRIBE QUEUE Module.Name, emitting re-executable
// MDL so describe → exec round-trips.
func execDescribeQueue(ctx *ExecContext, s *ast.DescribeQueueStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	q := findQueue(ctx, s.Name.Module, s.Name.Name)
	if q == nil {
		return mdlerrors.NewNotFound("queue", s.Name.String())
	}

	if q.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", q.Documentation)
	}
	fmt.Fprintf(ctx.Output, "create or modify queue %s (\n", s.Name.String())
	// Parallelism is an expression string; quote it unless it is a plain integer,
	// so an expression survives the round-trip.
	fmt.Fprintf(ctx.Output, "  Parallelism: %s,\n", formatParallelism(q.Parallelism))
	fmt.Fprintf(ctx.Output, "  ClusterWide: %t,\n", q.ClusterWide)
	fmt.Fprint(ctx.Output, ");\n")
	return nil
}

// formatParallelism renders the expression bare when it is a plain integer and
// quoted otherwise.
func formatParallelism(expr string) string {
	if expr == "" {
		return "1"
	}
	for _, r := range expr {
		if r < '0' || r > '9' {
			return "'" + strings.ReplaceAll(expr, "'", "''") + "'"
		}
	}
	return expr
}
