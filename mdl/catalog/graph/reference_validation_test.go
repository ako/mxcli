// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestCommunities_EvoraReference validates the native Go Leiden against the
// leidenalg reference run on the same graph (105 communities, modularity ≈0.878,
// see PROPOSAL_graph_analysis.md). It reads an exported edge list if present and
// skips otherwise, so it runs locally but never blocks CI.
func TestCommunities_EvoraReference(t *testing.T) {
	f, err := os.Open("/tmp/evora_edges.tsv")
	if err != nil {
		t.Skip("no /tmp/evora_edges.tsv export; skipping reference validation")
	}
	defer f.Close()

	var edges []Edge
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		p := strings.Split(sc.Text(), "\t")
		if len(p) == 2 && p[0] != "" && p[1] != "" && p[0] != "SourceName" {
			// undirected: add both directions so undAdj is symmetric.
			edges = append(edges, Edge{Source: p[0], Target: p[1]}, Edge{Source: p[1], Target: p[0]})
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read export: %v", err)
	}
	g := New(edges)
	comm := g.Communities(1.0)
	seen := map[int]bool{}
	for _, c := range comm {
		seen[c] = true
	}
	q := g.Modularity(comm, 1.0)
	t.Logf("Evora: %d nodes, %d communities, modularity %.3f (reference: 105, 0.878)", g.N(), len(seen), q)

	// Modularity should be close to the reference; community count in the ballpark.
	if q < 0.80 {
		t.Errorf("modularity %.3f well below reference 0.878", q)
	}
	if len(seen) < 60 || len(seen) > 200 {
		t.Errorf("community count %d far from reference 105", len(seen))
	}
	assertCommunitiesConnected(t, g, comm)
}
