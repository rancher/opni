package graph

import "gonum.org/v1/gonum/graph/encoding/dot"

// From : https://pkg.go.dev/gonum.org/v1/gonum/graph/encoding/dot
// the Dot encoding depends on the gonum Node, Attributer, Porter,
// Attributers, Structurer, Subgrapher and Graph interfaces.

func (s *ScientificKubeGraph) RenderDOT() ([]byte, error) {
	// TODO (topology) :implement dot  encoding  interfaces
	return dot.Marshal(s, "name-string", "prefix-string", "indent-string")
}
