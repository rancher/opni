package test

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/ruler"
	"github.com/cortexproject/cortex/pkg/storage/tsdb"
	"github.com/go-kit/log"
	"github.com/kralicky/gpkg/sync/atomic"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	metrics_agent_drivers "github.com/rancher/opni/plugins/metrics/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex/configutil"
	metrics_drivers "github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	cortexVersion string
)

func init() {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		panic("could not read build info")
	}
	// https://github.com/golang/go/issues/33976
	if buildInfo.Main.Path == "" {
		cortexVersion = "(unknown)"
	} else {
		var found bool
		for _, depInfo := range buildInfo.Deps {
			if depInfo.Path == "github.com/cortexproject/cortex" {
				if depInfo.Replace != nil {
					cortexVersion = depInfo.Replace.Version
				} else {
					cortexVersion = depInfo.Version
				}
				found = true
				break
			}
		}
		if !found {
			panic("could not find cortex dependency in build info")
		}
	}

	metrics_drivers.ClusterDrivers.Register("test-environment", func(ctx context.Context, _ ...driverutil.Option) (metrics_drivers.ClusterDriver, error) {
		env := test.EnvFromContext(ctx)
		return NewTestEnvMetricsClusterDriver(env), nil
	})

	metrics_agent_drivers.NodeDrivers.Register("test-environment-prometheus", func(ctx context.Context, _ ...driverutil.Option) (metrics_agent_drivers.MetricsNodeDriver, error) {
		env := test.EnvFromContext(ctx)
		return NewTestEnvPrometheusNodeDriver(env), nil
	})

	metrics_agent_drivers.NodeDrivers.Register("test-environment-otel", func(ctx context.Context, _ ...driverutil.Option) (metrics_agent_drivers.MetricsNodeDriver, error) {
		env := test.EnvFromContext(ctx)
		return NewTestEnvOtelNodeDriver(env), nil
	})
}

type TestEnvMetricsClusterDriver struct {
	cortexops.UnsafeCortexOpsServer

	lock         sync.RWMutex
	state        atomic.Value[cortexops.InstallState]
	cortexCtx    context.Context
	cortexCancel context.CancelFunc

	Env             *test.Environment
	ResourceVersion string
	activeConfig    *cortexops.CapabilityBackendConfigSpec
	configTracker   *driverutil.DefaultingConfigTracker[*cortexops.CapabilityBackendConfigSpec]
}

// ListPresets implements cortexops.CortexOpsServer.
func (*TestEnvMetricsClusterDriver) ListPresets(context.Context, *emptypb.Empty) (*cortexops.PresetList, error) {
	return &cortexops.PresetList{
		Items: []*cortexops.Preset{
			{
				Id: &corev1.Reference{Id: "test-environment"},
				Metadata: &cortexops.PresetMetadata{
					DisplayName: "Test Environment",
					Description: "Runs cortex in single-binary mode from bin/opni",
					Notes: []string{
						"Configuration is stored in a temporary directory; press (i) for details.",
						"Workload configuration is ignored in the test environment.",
					},
				},
				Spec: &cortexops.CapabilityBackendConfigSpec{
					CortexWorkloads: &cortexops.CortexWorkloadsConfig{
						Targets: map[string]*cortexops.CortexWorkloadSpec{
							"all": {
								Replicas: lo.ToPtr[int32](1),
							},
						},
					},
					CortexConfig: &cortexops.CortexApplicationConfig{
						LogLevel: lo.ToPtr("warn"),
					},
				},
			},
		},
	}, nil
}

func (k *TestEnvMetricsClusterDriver) Uninstall(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	err := k.configTracker.ApplyConfig(ctx, &cortexops.CapabilityBackendConfigSpec{
		Enabled: lo.ToPtr[bool](false),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to uninstall monitoring cluster: %s", err.Error())
	}
	return &emptypb.Empty{}, nil
}

func (k *TestEnvMetricsClusterDriver) Install(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	err := k.configTracker.ApplyConfig(ctx, &cortexops.CapabilityBackendConfigSpec{
		Enabled: lo.ToPtr[bool](true),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to install monitoring cluster: %s", err.Error())
	}
	return &emptypb.Empty{}, nil
}

func (k *TestEnvMetricsClusterDriver) GetDefaultConfiguration(ctx context.Context, _ *emptypb.Empty) (*cortexops.CapabilityBackendConfigSpec, error) {
	return k.configTracker.GetDefaultConfig(ctx)
}

func (k *TestEnvMetricsClusterDriver) SetDefaultConfiguration(ctx context.Context, spec *cortexops.CapabilityBackendConfigSpec) (*emptypb.Empty, error) {
	spec.Enabled = nil
	if err := k.configTracker.SetDefaultConfig(ctx, spec); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (k *TestEnvMetricsClusterDriver) ResetDefaultConfiguration(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if err := k.configTracker.ResetDefaultConfig(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (k *TestEnvMetricsClusterDriver) GetConfiguration(ctx context.Context, _ *emptypb.Empty) (*cortexops.CapabilityBackendConfigSpec, error) {
	return k.configTracker.GetConfigOrDefault(ctx)
}

func (d *TestEnvMetricsClusterDriver) SetConfiguration(ctx context.Context, conf *cortexops.CapabilityBackendConfigSpec) (*emptypb.Empty, error) {
	conf.Enabled = nil
	if err := d.configTracker.ApplyConfig(ctx, conf); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (k *TestEnvMetricsClusterDriver) ResetConfiguration(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if err := k.configTracker.ResetConfig(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DryRun implements cortexops.CortexOpsServer.
func (k *TestEnvMetricsClusterDriver) DryRun(ctx context.Context, req *cortexops.DryRunRequest) (*cortexops.DryRunResponse, error) {
	res, err := k.configTracker.DryRun(ctx, req.Target, req.Action, req.Spec)
	if err != nil {
		return nil, err
	}
	return &cortexops.DryRunResponse{
		Current:          res.Current,
		Modified:         res.Modified,
		ValidationErrors: configutil.CollectValidationErrorLogs(res.Modified.GetCortexConfig()),
	}, nil
}

var _ cortexops.CortexOpsServer = (*TestEnvMetricsClusterDriver)(nil)

func NewTestEnvMetricsClusterDriver(env *test.Environment) *TestEnvMetricsClusterDriver {
	d := &TestEnvMetricsClusterDriver{
		Env: env,
	}
	d.state.Store(cortexops.InstallState_NotInstalled)
	defaultStore := storage.NewInMemoryValueStore[*cortexops.CapabilityBackendConfigSpec]()
	activeStore := storage.NewInMemoryValueStore[*cortexops.CapabilityBackendConfigSpec](d.onActiveConfigChanged)
	d.configTracker = driverutil.NewDefaultingConfigTracker[*cortexops.CapabilityBackendConfigSpec](
		defaultStore, activeStore, flagutil.LoadDefaults[*cortexops.CapabilityBackendConfigSpec],
	)
	return d
}

func (d *TestEnvMetricsClusterDriver) onActiveConfigChanged(old, new *cortexops.CapabilityBackendConfigSpec) {
	if !d.lock.TryLock() {
		start := time.Now()
		d.Env.Logger.Info("Waiting for a previous config update to complete...")
		d.lock.Lock()
		d.Env.Logger.With(
			zap.Duration("waited", time.Since(start)),
		).Info("Previous config update completed, continuing")
	}
	defer d.lock.Unlock()

	d.Env.Logger.With(
		zap.Any("old", old),
		zap.Any("new", new),
	).Info("Config changed")

	oldCtx, oldCancel := d.cortexCtx, d.cortexCancel

	ctx, ca := context.WithCancel(waitctx.FromContext(d.Env.Context()))
	d.cortexCtx = ctx
	d.cortexCancel = ca

	if oldCancel != nil {
		oldCancel()
		waitctx.Wait(oldCtx)
	}

	if !new.GetEnabled() {
		d.state.Store(cortexops.InstallState_NotInstalled)
		return
	}

	d.state.Store(cortexops.InstallState_Updating)

	d.Env.StartCortex(ctx, func(cco test.CortexConfigOptions, iso test.ImplementationSpecificOverrides) ([]byte, []byte, error) {
		cconf, rtconf, err := configutil.CortexAPISpecToCortexConfig(d.activeConfig.GetCortexConfig(),
			configutil.MergeOverrideLists(
				configutil.NewHostOverrides(cco),
				configutil.NewImplementationSpecificOverrides(iso),
				[]configutil.CortexConfigOverrider{
					configutil.NewOverrider(func(t *ring.LifecyclerConfig) bool {
						t.Addr = "localhost"
						t.JoinAfter = 1 * time.Millisecond
						t.MinReadyDuration = 1 * time.Millisecond
						t.FinalSleep = 1 * time.Millisecond
						return true
					}),
					configutil.NewOverrider(func(t *ruler.Config) bool {
						t.EvaluationInterval = 1 * time.Second
						t.PollInterval = 1 * time.Second
						t.Notifier.TLS.ServerName = "127.0.0.1"
						return true
					}),
					configutil.NewOverrider(func(t *tsdb.BucketStoreConfig) bool {
						t.SyncInterval = 10 * time.Second
						return true
					}),
				},
			)...,
		)
		if err != nil {
			return nil, nil, err
		}

		if err := cconf.Validate(log.NewNopLogger()); err != nil {
			d.Env.Logger.With(
				zap.Error(err),
			).Warn("Cortex config failed validation (ignoring)")
		}

		cconfBytes, err := configutil.MarshalCortexConfig(cconf)
		if err != nil {
			return nil, nil, err
		}
		rtconfBytes, err := configutil.MarshalRuntimeConfig(rtconf)
		if err != nil {
			return nil, nil, err
		}
		return cconfBytes, rtconfBytes, nil
	})

	d.state.Store(cortexops.InstallState_Installed)
}

func NewTestEnvPrometheusNodeDriver(env *test.Environment) *TestEnvPrometheusNodeDriver {
	return &TestEnvPrometheusNodeDriver{
		env: env,
	}
}

func NewTestEnvOtelNodeDriver(env *test.Environment) *TestEnvOtelNodeDriver {
	return &TestEnvOtelNodeDriver{
		env: env,
	}
}

func (d *TestEnvMetricsClusterDriver) Name() string {
	return "test-environment"
}

func (d *TestEnvMetricsClusterDriver) ShouldDisableNode(*corev1.Reference) error {
	d.lock.RLock()
	defer d.lock.RUnlock()

	switch d.state.Load() {
	case cortexops.InstallState_NotInstalled, cortexops.InstallState_Uninstalling:
		return status.Error(codes.Unavailable, fmt.Sprintf("Cortex cluster is not installed"))
	case cortexops.InstallState_Updating, cortexops.InstallState_Installed:
		return nil
	case cortexops.InstallState_Unknown:
		fallthrough
	default:
		// can't determine cluster status, so don't disable the node
		return nil
	}
}

func (d *TestEnvMetricsClusterDriver) Status(context.Context, *emptypb.Empty) (*cortexops.InstallStatus, error) {
	d.lock.RLock()
	defer d.lock.RUnlock()

	return &cortexops.InstallStatus{
		State:    d.state.Load(),
		Version:  cortexVersion,
		Metadata: map[string]string{"test-environment": "true"},
	}, nil
}

type TestEnvPrometheusNodeDriver struct {
	env *test.Environment

	prometheusMu     sync.Mutex
	prometheusCtx    context.Context
	prometheusCancel context.CancelFunc
}

func (d *TestEnvPrometheusNodeDriver) ConfigureRuleGroupFinder(config *v1beta1.RulesSpec) notifier.Finder[rules.RuleGroup] {
	if config.GetDiscovery().GetFilesystem() != nil {
		return rules.NewFilesystemRuleFinder(config.Discovery.Filesystem)
	}
	return nil
}

var _ metrics_agent_drivers.MetricsNodeDriver = (*TestEnvPrometheusNodeDriver)(nil)

// ConfigureNode implements drivers.MetricsNodeDriver
func (d *TestEnvPrometheusNodeDriver) ConfigureNode(nodeId string, conf *node.MetricsCapabilityConfig) error {
	lg := d.env.Logger.With(
		"node", nodeId,
		"driver", "prometheus",
	)
	lg.Debug("configuring node")

	d.prometheusMu.Lock()
	defer d.prometheusMu.Unlock()

	exists := d.prometheusCtx != nil && d.prometheusCancel != nil
	shouldExist := conf.Enabled && conf.GetSpec().GetPrometheus() != nil

	if exists && !shouldExist {
		lg.Info("stopping prometheus")
		d.prometheusCancel()
		waitctx.Wait(d.prometheusCtx)
		d.prometheusCancel = nil
		d.prometheusCtx = nil
	} else if !exists && shouldExist {
		lg.Info("starting prometheus")
		ctx, ca := context.WithCancel(context.TODO())
		ctx = waitctx.FromContext(ctx)
		_, err := d.env.StartPrometheusContext(ctx, nodeId)
		if err != nil {
			lg.With("err", err).Error("failed to start prometheus")
			ca()
			return err
		}
		d.prometheusCtx = ctx
		d.prometheusCancel = ca
	} else if exists && shouldExist {
		lg.Debug("nothing to do (already running)")
	} else {
		lg.Debug("nothing to do (already stopped)")
	}

	return nil
}

// DiscoverPrometheuses implements drivers.MetricsNodeDriver
func (*TestEnvPrometheusNodeDriver) DiscoverPrometheuses(context.Context, string) ([]*remoteread.DiscoveryEntry, error) {
	return nil, nil
}

type TestEnvOtelNodeDriver struct {
	env *test.Environment

	otelMu     sync.Mutex
	otelCtx    context.Context
	otelCancel context.CancelFunc
}

// ConfigureNode implements drivers.MetricsNodeDriver.
func (d *TestEnvOtelNodeDriver) ConfigureNode(nodeId string, conf *node.MetricsCapabilityConfig) error {
	lg := d.env.Logger.With(
		"node", nodeId,
		"driver", "otel",
	)
	lg.Debug("configuring node")

	d.otelMu.Lock()
	defer d.otelMu.Unlock()

	exists := d.otelCtx != nil && d.otelCancel != nil
	shouldExist := conf.Enabled && conf.GetSpec().GetOtel() != nil

	if exists && !shouldExist {
		lg.Info("stopping otel")
		d.otelCancel()
		waitctx.Wait(d.otelCtx)
		d.otelCancel = nil
		d.otelCtx = nil
	} else if !exists && shouldExist {
		lg.Info("starting otel")
		ctx, ca := context.WithCancel(context.TODO())
		ctx = waitctx.FromContext(ctx)
		err := d.env.StartOTELCollectorContext(ctx, nodeId, node.CompatOTELStruct(conf.GetSpec().GetOtel()))
		if err != nil {
			lg.With("err", err).Error("failed to configure otel collector")
			ca()
			return fmt.Errorf("failed to configure otel collector: %w", err)
		}
		d.otelCtx = ctx
		d.otelCancel = ca
	} else if exists && shouldExist {
		lg.Debug("nothing to do (already running)")
	} else {
		lg.Debug("nothing to do (already stopped)")
	}

	return nil
}

// ConfigureRuleGroupFinder implements drivers.MetricsNodeDriver.
func (*TestEnvOtelNodeDriver) ConfigureRuleGroupFinder(config *v1beta1.RulesSpec) notifier.Finder[rules.RuleGroup] {
	if config.GetDiscovery().GetFilesystem() != nil {
		return rules.NewFilesystemRuleFinder(config.Discovery.Filesystem)
	}
	return nil
}

// DiscoverPrometheuses implements drivers.MetricsNodeDriver.
func (*TestEnvOtelNodeDriver) DiscoverPrometheuses(context.Context, string) ([]*remoteread.DiscoveryEntry, error) {
	return nil, nil
}

var _ metrics_agent_drivers.MetricsNodeDriver = (*TestEnvOtelNodeDriver)(nil)
