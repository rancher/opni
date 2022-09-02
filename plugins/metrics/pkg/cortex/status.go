package cortex

import (
	"context"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*cortexadmin.ClusterStatus, error) {
	cs, err := p.cortexClientSet.GetContext(ctx)
	if err != nil {
		return nil, err
	}

	stat := &cortexadmin.ClusterStatus{}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() (err error) {
		stat.Distributor, err = cs.Distributor().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Ingester, err = cs.Ingester().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Ruler, err = cs.Ruler().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Purger, err = cs.Purger().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Compactor, err = cs.Compactor().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.StoreGateway, err = cs.StoreGateway().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.QueryFrontend, err = cs.QueryFrontend().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Querier, err = cs.Querier().Status(ctx)
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

func (p *Plugin) GetClusterConfig(ctx context.Context, req *cortexadmin.ConfigRequest) (*cortexadmin.ConfigResponse, error) {
	resp := &cortexadmin.ConfigResponse{
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
