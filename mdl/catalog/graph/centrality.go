// SPDX-License-Identifier: Apache-2.0

package graph

import "sort"

// PageRank computes the PageRank of each node over the directed graph (power
// iteration with the given damping factor, typically 0.85). Dangling nodes (no
// out-edges) redistribute their rank uniformly. Deterministic; converges in a few
// dozen iterations on these graphs. Returns a slice indexed by node id, summing
// to ~1.
func (g *Graph) PageRank(damping float64, maxIter int) []float64 {
	n := g.N()
	if n == 0 {
		return nil
	}
	if maxIter <= 0 {
		maxIter = 100
	}
	// Weighted out-degree per node (for distributing rank along edge weights).
	outW := make([]float64, n)
	for i := 0; i < n; i++ {
		for _, w := range g.outAdj[i] {
			outW[i] += w
		}
	}
	rank := make([]float64, n)
	for i := range rank {
		rank[i] = 1.0 / float64(n)
	}
	base := (1 - damping) / float64(n)
	const eps = 1e-9
	for iter := 0; iter < maxIter; iter++ {
		next := make([]float64, n)
		dangling := 0.0
		for i := 0; i < n; i++ {
			if outW[i] == 0 {
				dangling += rank[i]
			}
		}
		danglingShare := damping * dangling / float64(n)
		for i := range next {
			next[i] = base + danglingShare
		}
		for i := 0; i < n; i++ {
			if outW[i] == 0 {
				continue
			}
			ri := damping * rank[i]
			for j, w := range g.outAdj[i] {
				next[j] += ri * w / outW[i]
			}
		}
		diff := 0.0
		for i := range rank {
			d := next[i] - rank[i]
			if d < 0 {
				d = -d
			}
			diff += d
		}
		rank = next
		if diff < eps {
			break
		}
	}
	return rank
}

// Betweenness computes (unweighted, directed) betweenness centrality via Brandes'
// algorithm — the fraction of shortest paths through each node. O(V*E), so it is
// the most expensive metric; callers should gate it on graph size. Deterministic.
// Returns a slice indexed by node id.
func (g *Graph) Betweenness() []float64 {
	n := g.N()
	cb := make([]float64, n)
	for s := 0; s < n; s++ {
		// BFS from s, tracking shortest-path counts and predecessors.
		stack := []int{}
		pred := make([][]int, n)
		sigma := make([]float64, n)
		dist := make([]int, n)
		for i := range dist {
			dist[i] = -1
		}
		sigma[s] = 1
		dist[s] = 0
		queue := []int{s}
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)
			for _, w := range neighborsSorted(g.outAdj[v]) {
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}
		delta := make([]float64, n)
		for i := len(stack) - 1; i >= 0; i-- {
			w := stack[i]
			for _, v := range pred[w] {
				delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
			}
			if w != s {
				cb[w] += delta[w]
			}
		}
	}
	return cb
}

// Degree returns the weighted in-degree, out-degree, and total per node.
func (g *Graph) Degree() (in, out []float64) {
	n := g.N()
	in = make([]float64, n)
	out = make([]float64, n)
	for i := 0; i < n; i++ {
		for _, w := range g.outAdj[i] {
			out[i] += w
		}
		for _, w := range g.inAdj[i] {
			in[i] += w
		}
	}
	return in, out
}

// sortedKeys returns the sorted integer keys of a map (helper for deterministic
// iteration where needed).
func sortedKeys(m map[int]float64) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}
