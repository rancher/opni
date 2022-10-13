package serialization

// go:embed templates/graphviz.tmpl
var graphvizDigraph []byte

func init() {
	// TODO(topology) : convert templates to text/template

	// TODO(topology) : set up a map of GraphRepr -> GraphSerailization Objects
}

type GraphSeralization interface {
	Serialize([]byte) ([]byte, error)
	Deserialize([]byte) ([]byte, error)
}
