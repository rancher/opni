package client

import (
	"context"
	"fmt"
	"os"

	"slices"

	"log/slog"

	"github.com/rancher/opni/apis"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/oci"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/update/kubernetes"
	"github.com/rancher/opni/pkg/urn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kubernetesAgentUpgrader struct {
	kubernetesOptions
	k8sClient client.Client
	lg        *slog.Logger
}

type kubernetesOptions struct {
	restConfig   *rest.Config
	namespace    string
	repoOverride *string
}

func (o *kubernetesOptions) apply(opts ...KubernetesOption) {
	for _, opt := range opts {
		opt(o)
	}
}

type KubernetesOption func(*kubernetesOptions)

func WithRestConfig(config *rest.Config) KubernetesOption {
	return func(o *kubernetesOptions) {
		o.restConfig = config
	}
}

func WithNamespace(namespace string) KubernetesOption {
	return func(o *kubernetesOptions) {
		o.namespace = namespace
	}
}

func WithRepoOverride(repo *string) KubernetesOption {
	return func(o *kubernetesOptions) {
		o.repoOverride = repo
	}
}

func NewKubernetesAgentUpgrader(lg *slog.Logger, opts ...KubernetesOption) (update.SyncHandler, error) {
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
		return nil, fmt.Errorf("failed to get agent deployment: %w", err)
	}

	// calculate agent manifest entry
	container := getAgentContainer(agentDeploy.Spec.Template.Spec.Containers)
	if container != nil {
		entries = append(entries, makeEntry(
			getAgentContainer(agentDeploy.Spec.Template.Spec.Containers),
			kubernetes.AgentComponent,
		))
	} else {
		return nil, fmt.Errorf("agent container not found in deployment")
	}

	// calculate controller manifest entry
	container = getControllerContainer(agentDeploy.Spec.Template.Spec.Containers)
	if container != nil {
		entries = append(entries, makeEntry(
			getControllerContainer(agentDeploy.Spec.Template.Spec.Containers),
			kubernetes.ControllerComponent,
		))
	} else {
		return nil, fmt.Errorf("controller container not found in deployment")
	}

	return &controlv1.UpdateManifest{
		Items: entries,
	}, nil
}

func (k *kubernetesAgentUpgrader) HandleSyncResults(ctx context.Context, results *controlv1.SyncResults) error {
	updateType, err := update.GetType(results.RequiredPatches.GetItems())
	if err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}

	if updateType != urn.Agent {
		return status.Errorf(codes.Unimplemented, "client can only handle agent updates")
	}

	agentDeploy, err := k.getAgentDeployment(ctx)
	containers := agentDeploy.Spec.Template.Spec.Containers
	if err != nil {
		return err
	}

	for _, patch := range results.GetRequiredPatches().GetItems() {
		if patch.GetOp() == controlv1.PatchOp_None {
			k.lg.With(
				"urn", patch.GetPackage(),
			).Info("update not required")
			continue
		}

		var container *corev1.Container
		patchURN, err := urn.ParseString(patch.GetPackage())
		if err != nil {
			k.lg.With(
				logger.Err(err),
			).Error("malformed package URN")
			continue
		}

		k.lg.With(
			"urn", patchURN.String(),
			"op", patch.GetOp().String(),
			"old", patch.GetOldDigest(),
			"new", patch.GetNewDigest(),
		).Info("applying patch")

		switch kubernetes.ComponentType(patchURN.Component) {
		case kubernetes.AgentComponent:
			container = getAgentContainer(containers)
		case kubernetes.ControllerComponent:
			container = getControllerContainer(containers)
		default:
			k.lg.Warn(fmt.Sprintf("received patch for unknown component %s", patchURN.Component))
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
	if err := k.k8sClient.List(ctx, list,
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
	oldImage, err := oci.Parse(container.Image)
	if err != nil {
		return err
	}
	if oldImage.DigestOrTag() != patch.GetOldDigest() {
		return kubernetes.ErrOldDigestMismatch
	}
	image, err := oci.Parse(patch.GetPath())
	if err != nil {
		return err
	}

	err = image.UpdateDigestOrTag(patch.GetNewDigest())
	if err != nil {
		return err
	}
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

func makeEntry(container *corev1.Container, packageType kubernetes.ComponentType) *controlv1.UpdateManifestEntry {
	entryURN := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, string(packageType))
	image, err := oci.Parse(container.Image)
	if err != nil {
		return nil
	}
	return &controlv1.UpdateManifestEntry{
		Package: entryURN.String(),
		Path:    image.Path(),
		Digest:  image.DigestOrTag(),
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
		lg, ok := args[0].(*slog.Logger)
		if !ok {
			return nil, fmt.Errorf("expected *slog.Logger, got %T", args[0])
		}

		var opts []KubernetesOption
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
