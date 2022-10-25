package graph

import (
	"github.com/steveteuber/kubectl-graph/pkg/graph"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	// Import to initialize client auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func Run(f cmdutil.Factory) (*graph.Graph, error) {
	// config, err := f.ToRESTConfig()
	// if err != nil {
	// 	return nil, err
	// }
	clientSet, err := f.KubernetesClientSet()
	if err != nil {
		return nil, err
	}
	objs := []*unstructured.Unstructured{}
	r := f.NewBuilder().
		Unstructured().
		NamespaceParam("").DefaultNamespace().AllNamespaces(true).
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
