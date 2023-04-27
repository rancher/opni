package kubernetes

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/agent/upgrader"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kubernetesAgentUpgrader struct {
	kubernetesAgentUpgraderOptions
	k8sClient   client.Client
	imageClient controlv1.ImageUpgradeClient
}

type kubernetesAgentUpgraderOptions struct {
	restConfig *rest.Config
	namespace  string
}

type kubernetesAgentUpgraderOption func(*kubernetesAgentUpgraderOptions)

func WithRestConfig(config *rest.Config) kubernetesAgentUpgraderOption {
	return func(o *kubernetesAgentUpgraderOptions) {
		o.restConfig = config
	}
}

func WithNamespace(namespace string) kubernetesAgentUpgraderOption {
	return func(o *kubernetesAgentUpgraderOptions) {
		o.namespace = namespace
	}
}

func (o *kubernetesAgentUpgraderOptions) apply(opts ...kubernetesAgentUpgraderOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func NewKubernetesAgentUpgrader(opts ...kubernetesAgentUpgraderOption) (*kubernetesAgentUpgrader, error) {
	options := kubernetesAgentUpgraderOptions{
		namespace: os.Getenv("POD_NAMESPACE"),
	}
	options.apply(opts...)

	if options.restConfig == nil {
		var err error
		options.restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	k8sClient, err := client.New(options.restConfig, client.Options{
		Scheme: apis.NewScheme(),
	})
	if err != nil {
		return nil, err
	}

	return &kubernetesAgentUpgrader{
		kubernetesAgentUpgraderOptions: options,
		k8sClient:                      k8sClient,
	}, nil
}

func (k *kubernetesAgentUpgrader) UpgradeRequired(ctx context.Context) (bool, error) {
	if k.imageClient == nil {
		return false, ErrGatewayClientNotSet
	}

	images, err := k.imageClient.GetAgentImages(ctx, &emptypb.Empty{})
	if err != nil {
		return false, err
	}

	deploy, err := k.getAgentDeployment(ctx)
	if err != nil {
		return false, err
	}

	agent := k.getAgentContainer(deploy.Spec.Template.Spec.Containers)
	controller := k.getControllerContainer(deploy.Spec.Template.Spec.Containers)

	switch {
	case agent != nil && agent.Image != images.AgentImage:
		return true, nil
	case controller != nil && controller.Image != images.ControllerImage:
		return true, nil
	default:
		return false, nil
	}
}

func (k *kubernetesAgentUpgrader) DoUpgrade(ctx context.Context) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deploy, err := k.getAgentDeployment(ctx)
		if err != nil {
			return err
		}

		err = k.updateDeploymentImages(ctx, deploy)
		if err != nil {
			return nil
		}

		return k.k8sClient.Update(ctx, deploy)
	})

	return err
}

func (k *kubernetesAgentUpgrader) CreateGatewayClient(conn grpc.ClientConnInterface) {
	k.imageClient = controlv1.NewImageUpgradeClient(conn)
}

func init() {
	upgrader.RegisterUpgraderBuilder("kubernetes",
		func(args ...any) (upgrader.AgentUpgrader, error) {
			var opts []kubernetesAgentUpgraderOption
			for _, arg := range args {
				switch v := arg.(type) {
				case *rest.Config:
					opts = append(opts, WithRestConfig(v))
				case string:
					opts = append(opts, WithNamespace(v))
				default:
					return nil, fmt.Errorf("unexpected argument: %v", arg)
				}
			}
			return NewKubernetesAgentUpgrader(opts...)
		},
	)
}
