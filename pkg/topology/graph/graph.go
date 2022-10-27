package graph

import (
	"bytes"

	"github.com/rancher/opni/pkg/topology/serialization"
	"github.com/rancher/opni/pkg/util"
	"github.com/steveteuber/kubectl-graph/pkg/graph"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	// Import to initialize client auth plugins
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
)

func NewRuntimeFactory() cmdutil.Factory {
	return cmdutil.NewFactory(&genericclioptions.ConfigFlags{})
}

// panics if run outside a kubernetes cluster LOL
func TraverseTopology(f cmdutil.Factory) (*graph.Graph, error) {
	clientSet, err := kubernetes.NewForConfig(util.Must(rest.InClusterConfig()))
	if err != nil {
		return nil, err
	}
	objs := []*unstructured.Unstructured{}
	r := f.NewBuilder().
		Unstructured().
		NamespaceParam("").DefaultNamespace().AllNamespaces(true).
		//FIXME: reimplement these at a later date
		// FilenameParam(o.ExplicitNamespace, &o.FilenameOptions).
		// LabelSelectorParam(o.LabelSelector).
		// FieldSelectorParam(o.FieldSelector).
		// RequestChunksOf(o.ChunkSize).
		// ResourceTypeOrNameArgs(true, args...).
		ContinueOnError().
		Latest().
		Flatten().
		Do()

	if err := r.Err(); err != nil {
		return nil, err
	}

	infos, err := r.Infos()
	if err != nil {
		return nil, err
	}

	for _, info := range infos {
		objs = append(objs, info.Object.(*unstructured.Unstructured))
	}

	graph, err := graph.NewGraph(clientSet, objs, func() {})
	if err != nil {
		return nil, err
	}
	return graph, nil
}

func RenderKubectlGraphTopology(g graph.Graph) (bytes.Buffer, error) {
	var b bytes.Buffer
	err := serialization.Templates.ExecuteTemplate(&b, "graphviz_default.tmpl", g)
	if err != nil {
		return b, err
	}

	// TODO (Topology)
	return b, nil
}
