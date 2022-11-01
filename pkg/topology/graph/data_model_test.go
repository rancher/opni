package graph_test

import (
	"bytes"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/topology/graph"
	kgraph "github.com/steveteuber/kubectl-graph/pkg/graph"
)

var _ = Describe("Graph data model tests", Ordered, Label(test.Unit, test.Slow), func() {
	When("we manipulate gonum kubernetes graphs", func() {
		It("should construct & serialize the graph", func() {
			b := test.TestData("topology/graph.json")
			var g kgraph.Graph
			err := json.NewDecoder(bytes.NewReader(b)).Decode(&g)
			Expect(err).To(Succeed())
			Expect(g).ToNot(BeNil())
			diGraph := graph.NewScientificKubeGraph()
			err = diGraph.FromKubectlGraph(&g)
			Expect(err).To(Succeed())
			Expect(diGraph).ToNot(BeNil())
			Expect(diGraph.IsEmpty()).To(BeFalse())

			_, err = diGraph.RenderDOT()
			Expect(err).To(Succeed())
		})
	})
})
