package kubernetes

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/agent/upgrader"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentPackage string

const (
	agentPackageAgent  AgentPackage = "agent"
	agentPackageClient AgentPackage = "client"
)

type kubernetesAgentUpgrader struct {
	kubernetesAgentUpgraderOptions
	k8sClient client.Client
}

type kubernetesAgentUpgraderOptions struct {
	restConfig   *rest.Config
	namespace    string
	repoOverride *string
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

func WithRepoOverride(repo *string) kubernetesAgentUpgraderOption {
	return func(o *kubernetesAgentUpgraderOptions) {
		o.repoOverride = repo
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

func (k *kubernetesAgentUpgrader) SyncAgent(ctx context.Context, entries []*controlv1.UpdateManifestEntry) error {
	upgradeRequired, err := k.upgradeRequired(ctx, entries)
	if err != nil {
		return err
	}

	if upgradeRequired {
		return k.doUpgrade(ctx, entries)
	}

	return nil
}

func (k *kubernetesAgentUpgrader) upgradeRequired(
	ctx context.Context,
	entries []*controlv1.UpdateManifestEntry,
) (bool, error) {
	deploy, err := k.getAgentDeployment(ctx)
	if err != nil {
		return false, err
	}

	agent := k.getAgentContainer(deploy.Spec.Template.Spec.Containers)
	agentDesired := k.buildAgentImage(entries)
	controller := k.getControllerContainer(deploy.Spec.Template.Spec.Containers)
	controllerDesired := k.buildClientImage(entries)

	if agentDesired == "" {
		agentDesired = controllerDesired
	}

	if agent.Image != agentDesired {
		return true, nil
	}

	if controller.Image != controllerDesired {
		return true, nil
	}

	return false, nil
}

func (k *kubernetesAgentUpgrader) doUpgrade(ctx context.Context, entries []*controlv1.UpdateManifestEntry) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deploy, err := k.getAgentDeployment(ctx)
		if err != nil {
			return err
		}

		err = k.updateDeploymentImages(ctx, deploy, entries)
		if err != nil {
			return nil
		}

		return k.k8sClient.Update(ctx, deploy)
	})

	return err
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
				case *string:
					opts = append(opts, WithRepoOverride(v))
				default:
					return nil, fmt.Errorf("unexpected argument: %v", arg)
				}
			}
			return NewKubernetesAgentUpgrader(opts...)
		},
	)
}
