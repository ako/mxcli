// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Graph-analysis Starlark builtins. They expose the facts computed by
// `refresh catalog communities` (community membership, layers, cycles, module
// dependencies, centrality, integration surface) so a user's lint rule can
// enforce its *own* architecture policy. mxcli ships the facts, not the opinion.
//
// All builtins return empty/None when the graph tables are absent or empty (e.g.
// communities were never built) rather than failing the lint run.

// graphRows runs a query and returns rows as ordered maps keyed by column alias.
func (r *StarlarkRule) graphRows(query string, args ...any) []map[string]any {
	if r.ctx == nil {
		return nil
	}
	rows, err := r.ctx.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return out
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	return out
}

func anyToStarlark(v any) starlark.Value {
	switch x := v.(type) {
	case nil:
		return starlark.None
	case int64:
		return starlark.MakeInt64(x)
	case float64:
		return starlark.Float(x)
	case []byte:
		return starlark.String(string(x))
	case string:
		return starlark.String(x)
	default:
		return starlark.String(fmt.Sprintf("%v", x))
	}
}

func rowToStruct(name string, m map[string]any) starlark.Value {
	d := starlark.StringDict{}
	for k, v := range m {
		d[k] = anyToStarlark(v)
	}
	return starlarkstruct.FromStringDict(starlark.String(name), d)
}

func rowsToList(name string, rows []map[string]any) starlark.Value {
	vals := make([]starlark.Value, 0, len(rows))
	for _, m := range rows {
		vals = append(vals, rowToStruct(name, m))
	}
	return starlark.NewList(vals)
}

// community_of(asset) -> struct{id, label} or None.
func (r *StarlarkRule) builtinCommunityOf(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var asset starlark.String
	if err := starlark.UnpackArgs("community_of", args, kwargs, "asset", &asset); err != nil {
		return nil, err
	}
	rows := r.graphRows(
		`SELECT c.CommunityId AS id,
			(SELECT Label FROM community_summary s WHERE s.CommunityId = c.CommunityId) AS label
		 FROM communities_data c WHERE c.AssetName = ? LIMIT 1`, string(asset))
	if len(rows) == 0 {
		return starlark.None, nil
	}
	return rowToStruct("community", rows[0]), nil
}

// layer_of(asset) -> int (topological sequence number) or None.
func (r *StarlarkRule) builtinLayerOf(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var asset starlark.String
	if err := starlark.UnpackArgs("layer_of", args, kwargs, "asset", &asset); err != nil {
		return nil, err
	}
	rows := r.graphRows(`SELECT Layer AS layer FROM graph_layers_data WHERE AssetName = ? LIMIT 1`, string(asset))
	if len(rows) == 0 {
		return starlark.None, nil
	}
	return anyToStarlark(rows[0]["layer"]), nil
}

// cycles() -> list of struct{id, size, members (list of names)}.
func (r *StarlarkRule) builtinCycles(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	rows := r.graphRows(
		`SELECT CycleId AS id, CycleSize AS size, group_concat(AssetName) AS members
		 FROM graph_cycles_data GROUP BY CycleId ORDER BY size DESC`)
	vals := make([]starlark.Value, 0, len(rows))
	for _, m := range rows {
		members := []starlark.Value{}
		if s, ok := m["members"].(string); ok && s != "" {
			for _, name := range strings.Split(s, ",") {
				members = append(members, starlark.String(name))
			}
		}
		vals = append(vals, starlarkstruct.FromStringDict(starlark.String("cycle"), starlark.StringDict{
			"id":      anyToStarlark(m["id"]),
			"size":    anyToStarlark(m["size"]),
			"members": starlark.NewList(members),
		}))
	}
	return starlark.NewList(vals), nil
}

// module_dependencies() -> list of struct{source_module, target_module, ref_kind, edges}.
func (r *StarlarkRule) builtinModuleDependencies(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	rows := r.graphRows(
		`SELECT SourceModule AS source_module, TargetModule AS target_module, RefKind AS ref_kind, Edges AS edges
		 FROM graph_module_dependencies`)
	return rowsToList("module_dependency", rows), nil
}

// centrality(asset) -> struct{in, out, total, pagerank, betweenness} or None.
func (r *StarlarkRule) builtinCentrality(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var asset starlark.String
	if err := starlark.UnpackArgs("centrality", args, kwargs, "asset", &asset); err != nil {
		return nil, err
	}
	rows := r.graphRows(
		`SELECT InDegree AS in, OutDegree AS out, Degree AS total, PageRank AS pagerank, Betweenness AS betweenness
		 FROM graph_god_nodes WHERE Asset = ? LIMIT 1`, string(asset))
	if len(rows) == 0 {
		return starlark.None, nil
	}
	return rowToStruct("centrality", rows[0]), nil
}

// god_nodes(metric="degree"|"pagerank"|"betweenness", min=N) -> list of high-centrality assets.
func (r *StarlarkRule) builtinGodNodes(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	metric := starlark.String("degree")
	var minVal starlark.Value = starlark.MakeInt(0)
	if err := starlark.UnpackArgs("god_nodes", args, kwargs, "metric?", &metric, "min?", &minVal); err != nil {
		return nil, err
	}
	col := map[string]string{"degree": "Degree", "pagerank": "PageRank", "betweenness": "Betweenness"}[string(metric)]
	if col == "" {
		return nil, fmt.Errorf("god_nodes: metric must be degree, pagerank or betweenness")
	}
	min, _ := starlark.AsFloat(minVal) // accepts int or float
	rows := r.graphRows(
		`SELECT Asset AS asset, ObjectType AS object_type, ModuleName AS module_name,
			Degree AS degree, PageRank AS pagerank, Betweenness AS betweenness
		 FROM graph_god_nodes WHERE `+col+` >= ? ORDER BY `+col+` DESC`, min)
	return rowsToList("god_node", rows), nil
}

// integration_surface() -> list of struct{source_community, target_community, ref_kind, edges, mechanism}.
func (r *StarlarkRule) builtinIntegrationSurface(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	rows := r.graphRows(
		`SELECT SourceCommunity AS source_community, TargetCommunity AS target_community,
			RefKind AS ref_kind, Edges AS edges, Mechanism AS mechanism
		 FROM graph_integration_surface ORDER BY Edges DESC`)
	return rowsToList("integration_edge", rows), nil
}

// refs_from(source) -> outbound references (complements refs_to).
func (r *StarlarkRule) builtinRefsFrom(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source starlark.String
	if err := starlark.UnpackArgs("refs_from", args, kwargs, "source_name", &source); err != nil {
		return nil, err
	}
	rows := r.graphRows(
		`SELECT TargetType AS target_type, TargetName AS target_name, RefKind AS ref_kind
		 FROM refs WHERE SourceName = ?`, string(source))
	return rowsToList("reference", rows), nil
}
