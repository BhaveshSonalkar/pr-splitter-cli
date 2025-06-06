package partition

import "pr-splitter-cli/internal/types"

// TarjanSCC implements Tarjan's algorithm for finding strongly connected components
type TarjanSCC struct {
	graph    *types.DependencyGraph
	index    int
	stack    []string
	indices  map[string]int
	lowlinks map[string]int
	onStack  map[string]bool
	sccs     []types.StronglyConnectedComponent
}

// NewTarjanSCC creates a new Tarjan SCC finder
func NewTarjanSCC(graph *types.DependencyGraph) *TarjanSCC {
	return &TarjanSCC{
		graph:    graph,
		index:    0,
		stack:    make([]string, 0),
		indices:  make(map[string]int),
		lowlinks: make(map[string]int),
		onStack:  make(map[string]bool),
		sccs:     make([]types.StronglyConnectedComponent, 0),
	}
}

// FindSCCs finds all strongly connected components in the graph
func (t *TarjanSCC) FindSCCs() []types.StronglyConnectedComponent {
	// Reset state
	t.index = 0
	t.stack = t.stack[:0]
	t.indices = make(map[string]int)
	t.lowlinks = make(map[string]int)
	t.onStack = make(map[string]bool)
	t.sccs = t.sccs[:0]

	// Run algorithm on all unvisited nodes
	for _, node := range t.graph.Nodes {
		if _, visited := t.indices[node]; !visited {
			t.strongConnect(node)
		}
	}

	return t.sccs
}

// strongConnect is the main recursive function of Tarjan's algorithm
func (t *TarjanSCC) strongConnect(v string) {
	// Set the depth index for v to the smallest unused index
	t.indices[v] = t.index
	t.lowlinks[v] = t.index
	t.index++
	t.pushStack(v)

	// Consider successors of v
	for _, w := range t.graph.Adjacency[v] {
		if _, exists := t.indices[w]; !exists {
			// Successor w has not yet been visited; recurse on it
			t.strongConnect(w)
			t.lowlinks[v] = min(t.lowlinks[v], t.lowlinks[w])
		} else if t.onStack[w] {
			// Successor w is in stack S and hence in the current SCC
			t.lowlinks[v] = min(t.lowlinks[v], t.indices[w])
		}
	}

	// If v is a root node, pop the stack and generate an SCC
	if t.lowlinks[v] == t.indices[v] {
		scc := t.popSCC(v)
		if len(scc) > 0 {
			t.sccs = append(t.sccs, types.StronglyConnectedComponent{
				Files: scc,
				Size:  len(scc),
			})
		}
	}
}

// pushStack adds a node to the stack
func (t *TarjanSCC) pushStack(node string) {
	t.stack = append(t.stack, node)
	t.onStack[node] = true
}

// popSCC pops nodes from stack until we reach the root node
func (t *TarjanSCC) popSCC(root string) []string {
	var scc []string

	for {
		if len(t.stack) == 0 {
			break
		}

		w := t.stack[len(t.stack)-1]
		t.stack = t.stack[:len(t.stack)-1]
		t.onStack[w] = false
		scc = append(scc, w)

		if w == root {
			break
		}
	}

	return scc
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
