// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"fmt"
	"testing"
)

// zacharyEdges is the classic Zachary's Karate Club (34 nodes, 78 undirected
// edges) — a standard community-detection benchmark with modularity ≈ 0.4.
var zacharyPairs = [][2]int{
	{0, 1}, {0, 2}, {0, 3}, {0, 4}, {0, 5}, {0, 6}, {0, 7}, {0, 8}, {0, 10}, {0, 11},
	{0, 12}, {0, 13}, {0, 17}, {0, 19}, {0, 21}, {0, 31}, {1, 2}, {1, 3}, {1, 7}, {1, 13},
	{1, 17}, {1, 19}, {1, 21}, {1, 30}, {2, 3}, {2, 7}, {2, 8}, {2, 9}, {2, 13}, {2, 27},
	{2, 28}, {2, 32}, {3, 7}, {3, 12}, {3, 13}, {4, 6}, {4, 10}, {5, 6}, {5, 10}, {5, 16},
	{6, 16}, {8, 30}, {8, 32}, {8, 33}, {9, 33}, {13, 33}, {14, 32}, {14, 33}, {15, 32}, {15, 33},
	{18, 32}, {18, 33}, {19, 33}, {20, 32}, {20, 33}, {22, 32}, {22, 33}, {23, 25}, {23, 27}, {23, 29},
	{23, 32}, {23, 33}, {24, 25}, {24, 27}, {24, 31}, {25, 31}, {26, 29}, {26, 33}, {27, 33}, {28, 31},
	{28, 33}, {29, 32}, {29, 33}, {30, 32}, {30, 33}, {31, 32}, {31, 33}, {32, 33},
}

func zacharyGraph() *Graph {
	var edges []Edge
	for _, p := range zacharyPairs {
		// undirected → add a single directed edge each way so undAdj is symmetric.
		edges = append(edges,
			Edge{Source: fmt.Sprintf("n%02d", p[0]), Target: fmt.Sprintf("n%02d", p[1])},
			Edge{Source: fmt.Sprintf("n%02d", p[1]), Target: fmt.Sprintf("n%02d", p[0])})
	}
	return New(edges)
}

func TestCommunities_Zachary(t *testing.T) {
	g := zacharyGraph()
	comm := g.Communities(1.0)

	// Distinct community count.
	seen := map[int]bool{}
	for _, c := range comm {
		seen[c] = true
	}
	if len(seen) < 2 || len(seen) > 6 {
		t.Errorf("expected 2–6 communities, got %d", len(seen))
	}

	// Modularity should be in the well-known good range for this graph.
	q := g.Modularity(comm, 1.0)
	if q < 0.35 {
		t.Errorf("modularity %.3f too low (expected ≈0.4)", q)
	}

	// Determinism: same input → identical partition.
	comm2 := g.Communities(1.0)
	for i := range comm {
		if comm[i] != comm2[i] {
			t.Fatalf("non-deterministic at node %d: %d vs %d", i, comm[i], comm2[i])
		}
	}

	// Leiden guarantee: every community is internally connected.
	assertCommunitiesConnected(t, g, comm)

	// Higher resolution → at least as many communities.
	hi := g.Communities(2.0)
	seenHi := map[int]bool{}
	for _, c := range hi {
		seenHi[c] = true
	}
	if len(seenHi) < len(seen) {
		t.Errorf("resolution 2.0 gave fewer communities (%d) than 1.0 (%d)", len(seenHi), len(seen))
	}
}

func assertCommunitiesConnected(t *testing.T, g *Graph, comm []int) {
	t.Helper()
	n := g.N()
	visited := make([]bool, n)
	for s := 0; s < n; s++ {
		if visited[s] {
			continue
		}
		c := comm[s]
		// BFS the connected component within community c.
		members := map[int]bool{}
		for i := 0; i < n; i++ {
			if comm[i] == c {
				members[i] = true
			}
		}
		reached := map[int]bool{s: true}
		queue := []int{s}
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			visited[v] = true
			for w := range g.undAdj[v] {
				if members[w] && !reached[w] {
					reached[w] = true
					queue = append(queue, w)
				}
			}
		}
		if len(reached) != len(members) {
			t.Errorf("community %d is not internally connected (%d reached of %d)", c, len(reached), len(members))
		}
	}
}

func TestSCC_AndCycles(t *testing.T) {
	// a→b→c→a (cycle), c→d, d→e (DAG tail), plus isolated self-loop f→f.
	g := New([]Edge{
		{Source: "a", Target: "b"}, {Source: "b", Target: "c"}, {Source: "c", Target: "a"},
		{Source: "c", Target: "d"}, {Source: "d", Target: "e"},
		{Source: "f", Target: "f"},
	})
	sccs := g.SCCs()
	// Find the {a,b,c} component.
	var multi int
	for _, c := range sccs {
		if len(c) == 3 {
			multi++
		}
	}
	if multi != 1 {
		t.Errorf("expected exactly one 3-node SCC, got sccs=%v", sccs)
	}
	cyc := g.Cycles()
	// {a,b,c} and the self-loop f are cycles; d,e are not.
	if len(cyc) != 2 {
		t.Errorf("expected 2 cycles (abc + self-loop f), got %d: %v", len(cyc), cyc)
	}
}

func TestLayers_DAG(t *testing.T) {
	// a→b→c and a→c. Layer = 1 + max(dependency layers); leaves (no out) = 0.
	g := New([]Edge{
		{Source: "a", Target: "b"}, {Source: "b", Target: "c"}, {Source: "a", Target: "c"},
	})
	layers := g.Layers()
	la, lb, lc := layers[g.index["a"]], layers[g.index["b"]], layers[g.index["c"]]
	if lc != 0 {
		t.Errorf("c is a leaf (depends on nothing), want layer 0, got %d", lc)
	}
	if lb != 1 {
		t.Errorf("b depends on c, want layer 1, got %d", lb)
	}
	if la != 2 {
		t.Errorf("a depends on b (and c), want layer 2, got %d", la)
	}
}

func TestPageRank_SumAndOrder(t *testing.T) {
	// A hub everyone points to should rank highest.
	g := New([]Edge{
		{Source: "a", Target: "hub"}, {Source: "b", Target: "hub"},
		{Source: "c", Target: "hub"}, {Source: "hub", Target: "a"},
	})
	pr := g.PageRank(0.85, 100)
	sum := 0.0
	for _, p := range pr {
		sum += p
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("PageRank should sum to ~1, got %.4f", sum)
	}
	if pr[g.index["hub"]] <= pr[g.index["b"]] {
		t.Errorf("hub (%.4f) should outrank a leaf b (%.4f)", pr[g.index["hub"]], pr[g.index["b"]])
	}
}

func TestBetweenness_Star(t *testing.T) {
	// Directed star where the centre sits on every leaf→leaf path.
	g := New([]Edge{
		{Source: "l1", Target: "c"}, {Source: "c", Target: "l2"},
		{Source: "l3", Target: "c"}, {Source: "c", Target: "l4"},
	})
	bt := g.Betweenness()
	if bt[g.index["c"]] <= 0 {
		t.Errorf("centre should have positive betweenness, got %.2f", bt[g.index["c"]])
	}
	for _, leaf := range []string{"l1", "l2", "l3", "l4"} {
		if bt[g.index[leaf]] != 0 {
			t.Errorf("leaf %s should have 0 betweenness, got %.2f", leaf, bt[g.index[leaf]])
		}
	}
}
