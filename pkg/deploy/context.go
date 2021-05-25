package deploy

import (
	"context"

	"github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io"
	"github.com/mitchellh/go-homedir"
	"github.com/rancher/wrangler-api/pkg/generated/controllers/apps"
	"github.com/rancher/wrangler-api/pkg/generated/controllers/batch"
	"github.com/rancher/wrangler-api/pkg/generated/controllers/core"
	"github.com/rancher/wrangler-api/pkg/generated/controllers/rbac"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Context struct {
	Helm  *helm.Factory
	Batch *batch.Factory
	Apps  *apps.Factory
	Core  *core.Factory
	Auth  *rbac.Factory
	K8s   kubernetes.Interface
	Apply apply.Apply
}

func (c *Context) Start(ctx context.Context) error {
	return start.All(ctx, 5, c.Helm, c.Apps, c.Batch, c.Core)
}

func NewContext(ctx context.Context, cfg string) (*Context, error) {
	// expand tilde
	cfg, err := homedir.Expand(cfg)
	if err != nil {
		return nil, err
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", cfg)
	if err != nil {
		return nil, err
	}

	if err := crds(ctx, restConfig); err != nil {
		return nil, err
	}

	k8s := kubernetes.NewForConfigOrDie(restConfig)
	return &Context{
		Helm:  helm.NewFactoryFromConfigOrDie(restConfig),
		K8s:   k8s,
		Auth:  rbac.NewFactoryFromConfigOrDie(restConfig),
		Apps:  apps.NewFactoryFromConfigOrDie(restConfig),
		Batch: batch.NewFactoryFromConfigOrDie(restConfig),
		Core:  core.NewFactoryFromConfigOrDie(restConfig),
		Apply: apply.New(k8s, apply.NewClientFactory(restConfig)).WithDynamicLookup(),
	}, nil
}

func crds(ctx context.Context, config *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(config)
	if err != nil {
		return err
	}

	factory.BatchCreateCRDs(ctx, crd.NamespacedTypes(
		"HelmChart.helm.cattle.io/v1",
		"HelmChartConfig.helm.cattle.io/v1")...)

	return factory.BatchWait()
}
