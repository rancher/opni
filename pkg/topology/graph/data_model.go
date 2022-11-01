package graph

import (
	"github.com/google/uuid"
	kgraph "github.com/steveteuber/kubectl-graph/pkg/graph"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/simple"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// implements gonum/graph.Node
type ScientificKubeNode struct {
	Id                int64 `json:"id"`
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

func (s *ScientificKubeNode) ID() int64 {
	return s.Id
}

var _ graph.Node = &ScientificKubeNode{}

// ScientificKubeEdge implements gonum/graph.Edge
type ScientificKubeEdge struct {
	F graph.Node
	T graph.Node
	// kubernetes label
	Label string
	// kubernetes relationship attributes
	Attributes map[string]string
}

func (s ScientificKubeEdge) From() graph.Node {
	return s.F
}

func (s ScientificKubeEdge) To() graph.Node {
	return s.T
}

func (s ScientificKubeEdge) ReversedEdge() graph.Edge {
	return &ScientificKubeEdge{
		F: s.T,
		T: s.F,
	}
}

var _ graph.Edge = &ScientificKubeEdge{}

// ScientificKubeGraph implements gonum/graph.Graph
type ScientificKubeGraph struct {
	// we need to keep a copy of this in order to eventually
	// optimize storing / updating differences
	//nolint:allowunused
	KubernetesIdsToGonumIds map[types.UID]int64 `json:"kubernetesIdsToGonumIds,omitempty"`
	*simple.DirectedGraph   `json:"inline,omitempty"`
}

var _ graph.Graph = &ScientificKubeGraph{}

func NewScientificKubeGraph() *ScientificKubeGraph {
	return &ScientificKubeGraph{
		DirectedGraph: simple.NewDirectedGraph(),
	}
}

func (s *ScientificKubeGraph) hardReset() {
	s.DirectedGraph = simple.NewDirectedGraph()
}

func (s *ScientificKubeGraph) IsEmpty() bool {
	return s.DirectedGraph.Nodes().Len() == 0
}

// FromKubectlGraph returns an error if we can't translate the kubectl-graph to a gonum graph
func (s *ScientificKubeGraph) FromKubectlGraph(g *kgraph.Graph) error {
	var (
		anyError error
	)
	defer func() {
		if anyError != nil {
			s.hardReset()
		}
	}()
	// gonum graphs need to be indexed using int64s
	tempTranslation := make(map[types.UID]int64)
	for _, n := range g.NodeList() {
		if _, ok := tempTranslation[n.UID]; !ok {
			uUid, err := uuid.NewUUID() // let's assume this is good enough for now
			if err != nil {
				anyError = err
				return err
			}
			uidIdx := int64(uUid.ID())
			tempTranslation[n.UID] = uidIdx
			s.DirectedGraph.AddNode(&ScientificKubeNode{
				Id:         uidIdx,
				TypeMeta:   n.TypeMeta,
				ObjectMeta: n.ObjectMeta,
			})
		}
	}
	for _, e := range g.RelationshipList() {
		toIdx, fromIdx := tempTranslation[e.To], tempTranslation[e.From]
		s.DirectedGraph.SetEdge(&ScientificKubeEdge{
			F:          s.Node(fromIdx),
			T:          s.Node(toIdx),
			Label:      e.Label,
			Attributes: e.Attr,
		})
	}
	s.KubernetesIdsToGonumIds = tempTranslation
	return nil
}
