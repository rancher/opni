package opnicluster

import (
	"fmt"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/output"
	"github.com/rancher/opni/apis/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	clusterFlowName   = "opni-clusterflow"
	clusterOutputName = "opni-clusteroutput"
)

func (r *Reconciler) buildClusterFlow() *loggingv1beta1.ClusterFlow {
	clusterFlow := &loggingv1beta1.ClusterFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterFlowName,
			Namespace: r.instanceNamespace,
		},
		Spec: loggingv1beta1.ClusterFlowSpec{
			Match: []loggingv1beta1.ClusterMatch{
				{
					ClusterExclude: &loggingv1beta1.ClusterExclude{
						Namespaces: []string{
							r.instanceNamespace,
						},
					},
				},
				{
					ClusterSelect: &loggingv1beta1.ClusterSelect{},
				},
			},
			Filters: []loggingv1beta1.Filter{
				{
					Dedot: &filter.DedotFilterConfig{
						Separator: "-",
						Nested:    true,
					},
				},
				{
					Grep: &filter.GrepConfig{
						Exclude: []filter.ExcludeSection{
							{
								Key:     "log",
								Pattern: `^\n$`,
							},
						},
					},
				},
				{
					DetectExceptions: &filter.DetectExceptions{
						Languages: []string{
							"java",
							"python",
							"go",
							"ruby",
							"js",
							"csharp",
							"php",
						},
						MultilineFlushInterval: "0.1",
					},
				},
			},
			GlobalOutputRefs: []string{
				clusterOutputName,
			},
		},
	}
	r.setOwner(clusterFlow)
	return clusterFlow
}

func (r *Reconciler) buildClusterOutput() *loggingv1beta1.ClusterOutput {
	clusteroutput := &loggingv1beta1.ClusterOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterOutputName,
			Namespace: r.instanceNamespace,
		},
		Spec: loggingv1beta1.ClusterOutputSpec{
			OutputSpec: loggingv1beta1.OutputSpec{
				HTTPOutput: &output.HTTPOutputConfig{
					Endpoint:    fmt.Sprintf("http://%s.%s", v1beta2.PayloadReceiverService.ServiceName(), r.instanceNamespace),
					ContentType: "application/json",
					JsonArray:   true,
					Buffer: &output.Buffer{
						Tags:           "[]",
						FlushInterval:  "2s",
						ChunkLimitSize: "1mb",
					},
				},
			},
		},
	}
	r.setOwner(clusteroutput)
	return clusteroutput
}
