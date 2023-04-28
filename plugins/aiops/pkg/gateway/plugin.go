package gateway

import (
	"context"
	"sync"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/versions"
	"github.com/rancher/opni/plugins/aiops/pkg/features"
	"go.uber.org/zap"
)

type AIOpsPlugin struct {
	system.UnimplementedSystemPluginClient
	ctx    context.Context
	Logger *zap.SugaredLogger

	Features []features.Feature
}

func NewPlugin(ctx context.Context, opts ...driverutil.Option) *AIOpsPlugin {
	p := &AIOpsPlugin{
		Logger: logger.NewPluginLogger().Named("aiops"),
		ctx:    ctx,
	}
	featureList := features.Features.List()
	p.Logger.With("features", featureList).Info("loading features")

	for _, featureName := range featureList {
		builder, _ := features.Features.Get(featureName)
		feat, err := builder(ctx, append([]driverutil.Option{
			driverutil.NewOption("version", versions.Version),
			driverutil.NewOption("logger", p.Logger),
		}, opts...)...)
		if err != nil {
			p.Logger.Fatal(err)
		}
		p.Features = append(p.Features, feat)
	}
	return p
}

func SchemeWith(ctx context.Context, opts ...driverutil.Option) meta.Scheme {
	return scheme(NewPlugin(ctx, opts...))
}

func Scheme(ctx context.Context) meta.Scheme {
	return scheme(NewPlugin(ctx))
}

func scheme(p *AIOpsPlugin) meta.Scheme {
	scheme := meta.NewScheme()

	scheme.Add(system.SystemPluginID, system.NewPlugin(p))

	var services []util.ServicePackInterface
	for _, feature := range p.Features {
		services = append(services, feature.ManagementAPIExtensionServices()...)
	}
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(services...))
	return scheme
}

func (p *AIOpsPlugin) UseManagementAPI(managementClient managementv1.ManagementClient) {
	wg := sync.WaitGroup{}
	for _, feature := range p.Features {
		feature := feature
		wg.Add(1)
		go func() {
			defer wg.Done()
			feature.UseManagementAPI(managementClient)
		}()
	}
	wg.Wait()
}
