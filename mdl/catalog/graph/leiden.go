// SPDX-License-Identifier: Apache-2.0

package graph

import "sort"

// Communities runs Leiden-style community detection on the undirected weighted
// graph at the given resolution γ (1.0 = balanced; higher = more, smaller
// communities). It is Louvain (greedy local moving + aggregation) followed by a
// connectivity-split refinement that gives Leiden's guarantee — every returned
// community is internally connected. Deterministic: nodes are processed in id
// order with deterministic tie-breaks. Returns a slice mapping node id ->
// community id (dense, assigned in order of smallest member).
func (g *Graph) Communities(resolution float64) []int {
	n := g.N()
	if n == 0 {
		return nil
	}
	if resolution <= 0 {
		resolution = 1.0
	}

	lg := g.buildLouvain()
	nodeComm := make([]int, n) // original node -> current super-node id
	for i := range nodeComm {
		nodeComm[i] = i
	}
	for {
		comm := lg.localMoving(resolution)
		dense, k := denseRenumber(comm)
		if k == len(lg.adj) {
			break // no node changed community → converged
		}
		for i := range nodeComm {
			nodeComm[i] = dense[comm[nodeComm[i]]]
		}
		lg = lg.aggregate(comm, dense, k)
		if k == 1 {
			break
		}
	}
	return relabelBySmallest(g.splitDisconnected(nodeComm))
}

// Modularity scores a partition (for testing / reporting) at the given resolution.
func (g *Graph) Modularity(comm []int, resolution float64) float64 {
	if resolution <= 0 {
		resolution = 1.0
	}
	twoM := 0.0
	deg := make([]float64, g.N())
	for i := 0; i < g.N(); i++ {
		for _, w := range g.undAdj[i] {
			deg[i] += w
		}
		twoM += deg[i]
	}
	if twoM == 0 {
		return 0
	}
	q := 0.0
	for i := 0; i < g.N(); i++ {
		for j, w := range g.undAdj[i] {
			if comm[i] == comm[j] {
				q += w - resolution*deg[i]*deg[j]/twoM
			}
		}
	}
	return q / twoM
}

// louvainGraph is a mutable weighted undirected graph used during aggregation.
// self[i] is the weight of edges internal to super-node i; deg[i] is its weighted
// degree (sum of adjacency + 2*self).
type louvainGraph struct {
	adj  []map[int]float64
	self []float64
	deg  []float64
	twoM float64
}

func (g *Graph) buildLouvain() *louvainGraph {
	n := g.N()
	lg := &louvainGraph{
		adj:  make([]map[int]float64, n),
		self: make([]float64, n),
		deg:  make([]float64, n),
	}
	for i := 0; i < n; i++ {
		lg.adj[i] = g.undAdj[i] // read-only share; localMoving never mutates adj
		d := 0.0
		for _, w := range g.undAdj[i] {
			d += w
		}
		lg.deg[i] = d
		lg.twoM += d
	}
	return lg
}

// localMoving greedily moves each node to the neighbouring community that most
// increases modularity, repeating until stable. Returns node -> community.
func (lg *louvainGraph) localMoving(resolution float64) []int {
	n := len(lg.adj)
	comm := make([]int, n)
	sigmaTot := make([]float64, n)
	for i := 0; i < n; i++ {
		comm[i] = i
		sigmaTot[i] = lg.deg[i]
	}
	for improved := true; improved; {
		improved = false
		for i := 0; i < n; i++ {
			ci := comm[i]
			wToComm := map[int]float64{}
			for j, w := range lg.adj[i] {
				wToComm[comm[j]] += w
			}
			sigmaTot[ci] -= lg.deg[i] // remove i from its community
			best := ci
			bestGain := wToComm[ci] - resolution*sigmaTot[ci]*lg.deg[i]/lg.twoM
			for _, c := range sortedKeys(wToComm) {
				if c == ci {
					continue
				}
				gain := wToComm[c] - resolution*sigmaTot[c]*lg.deg[i]/lg.twoM
				if gain > bestGain+1e-12 {
					bestGain, best = gain, c
				}
			}
			sigmaTot[best] += lg.deg[i]
			if best != ci {
				comm[i] = best
				improved = true
			}
		}
	}
	return comm
}

// aggregate collapses each community into a super-node. dense maps the (possibly
// sparse) community ids of comm to 0..k-1.
func (lg *louvainGraph) aggregate(comm, dense []int, k int) *louvainGraph {
	nl := &louvainGraph{
		adj:  make([]map[int]float64, k),
		self: make([]float64, k),
		deg:  make([]float64, k),
		twoM: lg.twoM,
	}
	for i := 0; i < k; i++ {
		nl.adj[i] = map[int]float64{}
	}
	internalDouble := make([]float64, k)
	for s := 0; s < len(lg.adj); s++ {
		cs := dense[comm[s]]
		nl.self[cs] += lg.self[s]
		for t, w := range lg.adj[s] {
			ct := dense[comm[t]]
			if cs == ct {
				internalDouble[cs] += w
			} else {
				nl.adj[cs][ct] += w
			}
		}
	}
	for c := 0; c < k; c++ {
		nl.self[c] += internalDouble[c] / 2
		d := 0.0
		for _, w := range nl.adj[c] {
			d += w
		}
		nl.deg[c] = d + 2*nl.self[c]
	}
	return nl
}

// splitDisconnected re-partitions so every community is connected in the original
// undirected graph: BFS each community's connected components into separate ids.
func (g *Graph) splitDisconnected(comm []int) []int {
	n := g.N()
	out := make([]int, n)
	visited := make([]bool, n)
	next := 0
	for s := 0; s < n; s++ {
		if visited[s] {
			continue
		}
		c := comm[s]
		queue := []int{s}
		visited[s] = true
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			out[v] = next
			for _, w := range neighborsSorted(g.undAdj[v]) {
				if !visited[w] && comm[w] == c {
					visited[w] = true
					queue = append(queue, w)
				}
			}
		}
		next++
	}
	return out
}

// denseRenumber maps the community ids present in comm to a dense 0..k-1 range
// (ascending), returning the mapping (indexed by old community id) and k.
func denseRenumber(comm []int) ([]int, int) {
	n := len(comm)
	present := make([]bool, n)
	for _, c := range comm {
		present[c] = true
	}
	dense := make([]int, n)
	next := 0
	for c := 0; c < n; c++ {
		if present[c] {
			dense[c] = next
			next++
		}
	}
	return dense, next
}

// relabelBySmallest renumbers communities so ids are assigned in order of their
// smallest-id member — deterministic and stable to read.
func relabelBySmallest(comm []int) []int {
	first := map[int]int{}
	for v, c := range comm {
		if _, ok := first[c]; !ok {
			first[c] = v
		}
	}
	cs := make([]int, 0, len(first))
	for c := range first {
		cs = append(cs, c)
	}
	sort.Slice(cs, func(a, b int) bool { return first[cs[a]] < first[cs[b]] })
	remap := make(map[int]int, len(cs))
	for i, c := range cs {
		remap[c] = i
	}
	out := make([]int, len(comm))
	for v, c := range comm {
		out[v] = remap[c]
	}
	return out
}
