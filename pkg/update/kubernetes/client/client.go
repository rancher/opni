package client

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/opni/apis"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/oci"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/update/kubernetes"
	"github.com/rancher/opni/pkg/urn"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kubernetesAgentUpgrader struct {
	kubernetesOptions
	k8sClient client.Client
	lg        *zap.SugaredLogger
}

type kubernetesOptions struct {
	restConfig   *rest.Config
	namespace    string
	repoOverride *string
}

func (o *kubernetesOptions) apply(opts ...kubernetesOption) {
	for _, opt := range opts {
		opt(o)
	}
}

type kubernetesOption func(*kubernetesOptions)

func WithRestConfig(config *rest.Config) kubernetesOption {
	return func(o *kubernetesOptions) {
		o.restConfig = config
	}
}

func WithNamespace(namespace string) kubernetesOption {
	return func(o *kubernetesOptions) {
		o.namespace = namespace
	}
}

func WithRepoOverride(repo *string) kubernetesOption {
	return func(o *kubernetesOptions) {
		o.repoOverride = repo
	}
}

func NewKubernetesAgentUpgrader(lg *zap.SugaredLogger, opts ...kubernetesOption) (*kubernetesAgentUpgrader, error) {
	options := kubernetesOptions{
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
		kubernetesOptions: options,
		k8sClient:         k8sClient,
		lg:                lg,
	}, nil
}

func (k *kubernetesAgentUpgrader) Strategy() string {
	return kubernetes.UpdateStrategy
}

func (k *kubernetesAgentUpgrader) GetCurrentManifest(ctx context.Context) (*controlv1.UpdateManifest, error) {
	var entries []*controlv1.UpdateManifestEntry
	agentDeploy, err := k.getAgentDeployment(ctx)
	if err != nil {
		return nil, err
	}

	// calculate agent manifest entry
	entries = append(entries, makeEntry(
		getAgentContainer(agentDeploy.Spec.Template.Spec.Containers),
		"minimal",
	))

	// calculate controller manifest entry
	entries = append(entries, makeEntry(
		getControllerContainer(agentDeploy.Spec.Template.Spec.Containers),
		"opni",
	))

	return &controlv1.UpdateManifest{
		Items: entries,
	}, nil
}

func (k *kubernetesAgentUpgrader) HandleSyncResults(ctx context.Context, results *controlv1.SyncResults) error {
	agentDeploy, err := k.getAgentDeployment(ctx)
	containers := agentDeploy.Spec.Template.Spec.Containers
	if err != nil {
		return err
	}

	for _, patch := range results.GetRequiredPatches().GetItems() {
		if patch.GetOp() == controlv1.PatchOp_None {
			continue
		}

		var container *corev1.Container
		patchURN, err := urn.ParseString(patch.GetPackage())
		if err != nil {
			k.lg.With(
				zap.Error(err),
			).Error("malformed package URN")
			continue
		}

		switch kubernetes.ComponentType(patchURN.Component) {
		case kubernetes.AgentComponent:
			container = getAgentContainer(containers)
		case kubernetes.ControllerComponent:
			container = getControllerContainer(containers)
		default:
			k.lg.Warnf("received patch for unknown component %s", patchURN.Component)
			continue
		}
		err = k.patchContainer(container, patch)
		if err != nil {
			return kubernetes.ErrComponentUpdate(kubernetes.ComponentType(patchURN.Component), err)
		}
		containers = replaceContainer(containers, container)
	}

	agentDeploy.Spec.Template.Spec.Containers = containers
	return k.k8sClient.Update(ctx, agentDeploy)
}

func (k *kubernetesAgentUpgrader) getAgentDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	list := &appsv1.DeploymentList{}
	if err := k.k8sClient.List(context.TODO(), list,
		client.InNamespace(k.namespace),
		client.MatchingLabels{
			"opni.io/app": "agent",
		},
	); err != nil {
		return nil, err
	}

	if len(list.Items) != 1 {
		return nil, kubernetes.ErrInvalidAgentList
	}
	return &list.Items[0], nil
}

func (k *kubernetesAgentUpgrader) patchContainer(container *corev1.Container, patch *controlv1.PatchSpec) error {
	oldImage := oci.Parse(container.Image)
	if oldImage.Digest != patch.GetOldDigest() {
		return kubernetes.ErrOldDigestMismatch
	}
	image := oci.Parse(patch.GetPath())
	image.Digest = patch.GetNewDigest()
	if k.repoOverride != nil {
		image.Registry = *k.repoOverride
	}
	container.Image = image.String()
	return nil
}

func getAgentContainer(containers []corev1.Container) *corev1.Container {
	for _, container := range containers {
		if slices.Contains(container.Args, "agentv2") {
			return &container
		}
	}
	return nil
}

func getControllerContainer(containers []corev1.Container) *corev1.Container {
	for _, container := range containers {
		if slices.Contains(container.Args, "client") {
			return &container
		}
	}
	return nil
}

func makeEntry(container *corev1.Container, packageType string) *controlv1.UpdateManifestEntry {
	entryURN := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, packageType)
	image := oci.Parse(container.Image)
	return &controlv1.UpdateManifestEntry{
		Package: entryURN.String(),
		Path:    image.Path(),
		Digest:  image.Digest,
	}
}

func replaceContainer(containers []corev1.Container, container *corev1.Container) []corev1.Container {
	for i, c := range containers {
		if c.Name == container.Name {
			containers[i] = *container
		}
	}
	return containers
}

func init() {
	update.RegisterAgentSyncHandlerBuilder(kubernetes.UpdateStrategy, func(args ...any) (update.SyncHandler, error) {
		lg, ok := args[0].(*zap.SugaredLogger)
		if !ok {
			return nil, fmt.Errorf("expected *zap.Logger, got %T", args[0])
		}

		var opts []kubernetesOption
		for _, arg := range args[1:] {
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

		return NewKubernetesAgentUpgrader(lg, opts...)
	})
}
