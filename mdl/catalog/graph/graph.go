// SPDX-License-Identifier: Apache-2.0

// Package graph provides pure-Go graph algorithms over the catalog reference
// graph: Leiden community detection, Tarjan strongly-connected components and
// topological layering, and PageRank / betweenness centrality. No CGO, no
// external dependencies — the graphs involved (a few thousand nodes) are small
// enough that straightforward implementations run in well under a second.
//
// All algorithms are deterministic: node ids are assigned in sorted name order
// and every iteration processes nodes in id order with deterministic tie-breaks,
// so the same input always yields the same output (required for stable catalog
// storage).
package graph

import "sort"

// Edge is a directed, weighted edge between two named nodes.
type Edge struct {
	Source string
	Target string
	Weight float64
}

// Graph is a weighted directed graph over integer node ids [0,N). Parallel edges
// between the same ordered pair are collapsed by summing their weights. It also
// maintains an undirected weighted adjacency for modularity-based clustering.
type Graph struct {
	names []string       // id -> name (sorted)
	index map[string]int // name -> id

	outAdj []map[int]float64 // directed: outAdj[i][j] = weight i->j
	inAdj  []map[int]float64 // directed: inAdj[j][i]  = weight i->j
	undAdj []map[int]float64 // undirected: undAdj[i][j] = combined weight
}

// New builds a graph from edges. Node ids are assigned by sorted node name, so
// the construction is fully deterministic regardless of edge order. Self-loops
// are kept in the directed adjacency but dropped from the undirected one (they
// don't affect community structure).
func New(edges []Edge) *Graph {
	nameSet := map[string]struct{}{}
	for _, e := range edges {
		if e.Source != "" {
			nameSet[e.Source] = struct{}{}
		}
		if e.Target != "" {
			nameSet[e.Target] = struct{}{}
		}
	}
	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}
	sort.Strings(names)

	g := &Graph{
		names:  names,
		index:  make(map[string]int, len(names)),
		outAdj: make([]map[int]float64, len(names)),
		inAdj:  make([]map[int]float64, len(names)),
		undAdj: make([]map[int]float64, len(names)),
	}
	for i, n := range names {
		g.index[n] = i
		g.outAdj[i] = map[int]float64{}
		g.inAdj[i] = map[int]float64{}
		g.undAdj[i] = map[int]float64{}
	}
	for _, e := range edges {
		if e.Source == "" || e.Target == "" {
			continue
		}
		w := e.Weight
		if w == 0 {
			w = 1
		}
		s, t := g.index[e.Source], g.index[e.Target]
		g.outAdj[s][t] += w
		g.inAdj[t][s] += w
		if s != t {
			g.undAdj[s][t] += w
			g.undAdj[t][s] += w
		}
	}
	return g
}

// N returns the number of nodes.
func (g *Graph) N() int { return len(g.names) }

// Name returns the node name for an id.
func (g *Graph) Name(id int) string { return g.names[id] }

// neighborsSorted returns the sorted neighbor ids of i in the given adjacency.
func neighborsSorted(adj map[int]float64) []int {
	ns := make([]int, 0, len(adj))
	for j := range adj {
		ns = append(ns, j)
	}
	sort.Ints(ns)
	return ns
}
