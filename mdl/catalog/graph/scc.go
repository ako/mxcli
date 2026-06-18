// SPDX-License-Identifier: Apache-2.0

package graph

import "sort"

// SCCs returns the strongly-connected components of the directed graph via
// Tarjan's algorithm (iterative, to avoid deep recursion). Each component is a
// sorted slice of node ids; the components are returned in a deterministic order
// (sorted by their smallest member). A component with more than one node — or a
// single node with a self-loop — is a dependency cycle.
func (g *Graph) SCCs() [][]int {
	n := g.N()
	const unvisited = -1
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = unvisited
	}
	var stack []int
	idx := 0
	var comps [][]int

	// Iterative Tarjan: each frame tracks the node and its neighbor cursor.
	type frame struct {
		v    int
		next int // position in sorted neighbor list
		nbrs []int
	}
	for s := 0; s < n; s++ {
		if index[s] != unvisited {
			continue
		}
		work := []frame{{v: s, nbrs: neighborsSorted(g.outAdj[s])}}
		index[s], low[s] = idx, idx
		idx++
		stack = append(stack, s)
		onStack[s] = true

		for len(work) > 0 {
			f := &work[len(work)-1]
			if f.next < len(f.nbrs) {
				w := f.nbrs[f.next]
				f.next++
				if index[w] == unvisited {
					index[w], low[w] = idx, idx
					idx++
					stack = append(stack, w)
					onStack[w] = true
					work = append(work, frame{v: w, nbrs: neighborsSorted(g.outAdj[w])})
				} else if onStack[w] && index[w] < low[f.v] {
					low[f.v] = index[w]
				}
				continue
			}
			// Done with f.v: if it's a root, pop its component.
			if low[f.v] == index[f.v] {
				var comp []int
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp = append(comp, w)
					if w == f.v {
						break
					}
				}
				sort.Ints(comp)
				comps = append(comps, comp)
			}
			child := f.v
			work = work[:len(work)-1]
			if len(work) > 0 {
				parent := &work[len(work)-1]
				if low[child] < low[parent.v] {
					low[parent.v] = low[child]
				}
			}
		}
	}
	sort.Slice(comps, func(a, b int) bool { return comps[a][0] < comps[b][0] })
	return comps
}

// hasSelfLoop reports whether node v has a self-edge (a trivial 1-node cycle).
func (g *Graph) hasSelfLoop(v int) bool {
	_, ok := g.outAdj[v][v]
	return ok
}

// Cycles returns only the SCCs that are genuine dependency tangles: more than one
// node, or a single self-looping node.
func (g *Graph) Cycles() [][]int {
	var cyc [][]int
	for _, c := range g.SCCs() {
		if len(c) > 1 || (len(c) == 1 && g.hasSelfLoop(c[0])) {
			cyc = append(cyc, c)
		}
	}
	return cyc
}

// Layers assigns every node a topological layer index by condensing the SCCs to a
// DAG and longest-path-levelling it: a node's layer is 1 + the max layer of its
// dependencies (the nodes it points to), so layer 0 holds nodes that depend on
// nothing (leaves of the dependency relation). Nodes in the same SCC share a
// layer. The result maps node id -> layer; deterministic.
func (g *Graph) Layers() []int {
	comps := g.SCCs()
	compOf := make([]int, g.N())
	for ci, comp := range comps {
		for _, v := range comp {
			compOf[v] = ci
		}
	}
	// Condensed DAG adjacency: edges between distinct components.
	cAdj := make([]map[int]struct{}, len(comps))
	for i := range cAdj {
		cAdj[i] = map[int]struct{}{}
	}
	for v := 0; v < g.N(); v++ {
		for w := range g.outAdj[v] {
			if compOf[v] != compOf[w] {
				cAdj[compOf[v]][compOf[w]] = struct{}{}
			}
		}
	}
	// Longest-path layer per component via memoised DFS over the DAG.
	compLayer := make([]int, len(comps))
	for i := range compLayer {
		compLayer[i] = -1
	}
	var depth func(c int) int
	depth = func(c int) int {
		if compLayer[c] != -1 {
			return compLayer[c]
		}
		compLayer[c] = 0 // guards against any residual cycle (there are none in a DAG)
		best := 0
		deps := make([]int, 0, len(cAdj[c]))
		for d := range cAdj[c] {
			deps = append(deps, d)
		}
		sort.Ints(deps)
		for _, d := range deps {
			if l := depth(d) + 1; l > best {
				best = l
			}
		}
		compLayer[c] = best
		return best
	}
	for c := range comps {
		depth(c)
	}
	layers := make([]int, g.N())
	for v := 0; v < g.N(); v++ {
		layers[v] = compLayer[compOf[v]]
	}
	return layers
}
