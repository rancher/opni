package drivers

import (
	"context"
	"fmt"

	"github.com/rancher/opni/apis"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/topology/apis/orchestrator"
	"google.golang.org/protobuf/types/known/emptypb"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TopologyManager struct {
	*TopologyManagerClusterDriverOptions
	orchestrator.UnsafeTopologyOrchestratorServer
}

var _ orchestrator.TopologyOrchestratorServer = &TopologyManager{}
var _ ClusterDriver = &TopologyManager{}

type TopologyManagerClusterDriverOptions struct {
	k8sClient client.Client
}

type TopologyManagerClusterDriverOption func(*TopologyManagerClusterDriverOptions)

func (o *TopologyManagerClusterDriverOptions) apply(opts ...TopologyManagerClusterDriverOption) {
	for _, op := range opts {
		op(o)
	}
}
func WithK8sClient(k8sClient client.Client) TopologyManagerClusterDriverOption {
	return func(o *TopologyManagerClusterDriverOptions) {
		o.k8sClient = k8sClient
	}
}

func NewTopologyManagerClusterDriver(opts ...TopologyManagerClusterDriverOption) (*TopologyManager, error) {
	options := &TopologyManagerClusterDriverOptions{}
	options.apply(opts...)
	if options.k8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.k8sClient = c
	}
	return &TopologyManager{
		TopologyManagerClusterDriverOptions: options,
	}, nil
}

func (t *TopologyManager) Name() string {
	return "topology-manager"
}

func (t *TopologyManager) GetClusterStatus(_ context.Context, _ *emptypb.Empty) (*orchestrator.InstallStatus, error) {
	return &orchestrator.InstallStatus{
		State: orchestrator.InstallState_Installed,
	}, nil
}

func (t *TopologyManager) ShouldDisableNode(_ *corev1.Reference) error {
	return nil
}
