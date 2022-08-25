package cortex

import (
	"context"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
)

func (p *Plugin) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*cortexops.ClusterStatus, error) {
	cs, err := p.cortexClientSet.GetContext(ctx)
	if err != nil {
		return nil, err
	}

	stat := &cortexops.ClusterStatus{
		CortexServices: &cortexops.CortexServicesStatus{},
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() (err error) {
		stat.CortexServices.Distributor, err = cs.Distributor().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.CortexServices.Ingester, err = cs.Ingester().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.CortexServices.Ruler, err = cs.Ruler().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.CortexServices.Purger, err = cs.Purger().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.CortexServices.Compactor, err = cs.Compactor().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.CortexServices.StoreGateway, err = cs.StoreGateway().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.CortexServices.QueryFrontend, err = cs.QueryFrontend().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.CortexServices.Querier, err = cs.Querier().Status(ctx)
		return
	})

	if err := eg.Wait(); err != nil {
		p.logger.With(
			zap.Error(err),
		).Error("failed to get cluster status")
		return nil, err
	}
	return stat, nil
}

func (p *Plugin) GetClusterConfig(ctx context.Context, req *cortexops.ClusterConfigRequest) (*cortexops.ClusterConfigResponse, error) {
	resp := &cortexops.ClusterConfigResponse{
		ConfigYaml: make([]string, len(req.ConfigModes)),
	}

	eg, ctx := errgroup.WithContext(ctx)

	for i, mode := range req.ConfigModes {
		i := i
		mode := mode
		eg.Go(func() (err error) {
			resp.ConfigYaml[i], err = p.cortexClientSet.Get().Distributor().Config(ctx, ConfigMode(mode))
			return
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return resp, nil
}
