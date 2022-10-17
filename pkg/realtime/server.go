package realtime

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/realtime/modules"
	slomodule "github.com/rancher/opni/pkg/realtime/modules/slo"
)

type RealtimeServer struct {
	RealtimeServerOptions
	config     *v1beta1.RealtimeServerSpec
	mgmtClient managementv1.ManagementClient
}

type RealtimeServerOptions struct {
	moduleSet *modules.ModuleSet
	logger    *zap.SugaredLogger
}

type RealtimeServerOption func(*RealtimeServerOptions)

func (o *RealtimeServerOptions) Apply(opts ...RealtimeServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithModuleSet(moduleSet *modules.ModuleSet) RealtimeServerOption {
	return func(o *RealtimeServerOptions) {
		o.moduleSet = moduleSet
	}
}

func WithLogger(logger *zap.SugaredLogger) RealtimeServerOption {
	return func(o *RealtimeServerOptions) {
		o.logger = logger
	}
}

func NewServer(conf *v1beta1.RealtimeServerSpec, opts ...RealtimeServerOption) (*RealtimeServer, error) {
	options := RealtimeServerOptions{
		moduleSet: AllModules(),
	}
	options.Apply(opts...)

	if options.logger == nil {
		options.logger = logger.New().Named("rt")
	} else {
		options.logger = options.logger.Named("rt")
	}

	return &RealtimeServer{
		RealtimeServerOptions: options,
		config:                conf,
	}, nil
}

func (rt *RealtimeServer) Start(ctx context.Context) error {
	mgmtClient, err := clients.NewManagementClient(ctx,
		clients.WithAddress(rt.config.ManagementClient.Address))
	if err != nil {
		return err
	}
	rt.mgmtClient = mgmtClient

	reg := prometheus.NewRegistry()
	mux := http.NewServeMux()
	mux.Handle(rt.config.Metrics.GetPath(), promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry: reg,
	}))
	go func() {
		rt.logger.With(
			"address", rt.config.MetricsListenAddress,
		).Info("starting metrics server")
		if err := http.ListenAndServe(rt.config.MetricsListenAddress, mux); err != nil {
			rt.logger.With(
				zap.Error(err),
			).Fatal("failed to start metrics server")
		}
	}()

	for _, mod := range rt.moduleSet.Modules() {
		lg := rt.logger.With("module", mod.Name())
		lg.Info("Starting RT module")

		mod := mod
		go func() {
			if err := mod.Run(ctx, &modules.ModuleContext{
				Log:    rt.logger.Named(mod.Name()),
				Client: rt.mgmtClient,
				Reg:    reg,
			}); err != nil {
				lg.With(
					zap.Error(err),
				).Error("RT module exited with error")
			}
			lg.Info("RT module stopped")
		}()
	}

	return nil
}

func AllModules() *modules.ModuleSet {
	set := modules.NewModuleSet()
	slomodule.AddToModuleSet(set)
	return set
}
