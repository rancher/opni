package test

import (
	"context"
	"fmt"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/ruler"
	"github.com/cortexproject/cortex/pkg/storage/bucket/filesystem"
	"github.com/cortexproject/cortex/pkg/storage/tsdb"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
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

type installStatusLocker struct {
	mu sync.Mutex
	s  cortexops.InstallStatus
}

func (l *installStatusLocker) Use(f func(*cortexops.InstallStatus)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f(&l.s)
}

type TestEnvMetricsClusterDriver struct {
	cortexops.UnsafeCortexOpsServer
	driverutil.DefaultConfigurableServer[*cortexops.CapabilityBackendConfigSpec, *cortexops.GetRequest]

	status     atomic.Pointer[installStatusLocker]
	configLock sync.RWMutex

	cortexCtx    context.Context
	cortexCancel context.CancelFunc

	Env             *test.Environment
	ResourceVersion string
	configTracker   *driverutil.DefaultingConfigTracker[*cortexops.CapabilityBackendConfigSpec]
}

// ListPresets implements cortexops.CortexOpsServer.
func (d *TestEnvMetricsClusterDriver) ListPresets(context.Context, *emptypb.Empty) (*cortexops.PresetList, error) {
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
						Storage: &storagev1.Config{
							Backend:    lo.ToPtr(storagev1.Filesystem),
							Filesystem: &storagev1.FilesystemConfig{},
						},
					},
				},
			},
		},
	}, nil
}

// DryRun implements cortexops.CortexOpsServer.
func (d *TestEnvMetricsClusterDriver) DryRun(ctx context.Context, req *cortexops.DryRunRequest) (*cortexops.DryRunResponse, error) {
	res, err := d.configTracker.DryRun(ctx, req.Target, req.Action, req.Spec)
	if err != nil {
		return nil, err
	}
	return &cortexops.DryRunResponse{
		Current:          res.Current,
		Modified:         res.Modified,
		ValidationErrors: configutil.ValidateConfiguration(res.Modified),
	}, nil
}

// ConfigurationHistory implements cortexops.CortexOpsServer.
func (d *TestEnvMetricsClusterDriver) ConfigurationHistory(ctx context.Context, req *cortexops.ConfigurationHistoryRequest) (*cortexops.ConfigurationHistoryResponse, error) {
	revisions, err := d.configTracker.History(ctx, req.GetTarget(), storage.IncludeValues(req.GetIncludeValues()))
	if err != nil {
		return nil, err
	}
	resp := &cortexops.ConfigurationHistoryResponse{
		Entries: make([]*cortexops.CapabilityBackendConfigSpec, len(revisions)),
	}
	for i, rev := range revisions {
		if req.IncludeValues {
			spec := rev.Value()
			spec.Revision = corev1.NewRevision(rev.Revision(), rev.Timestamp())
			resp.Entries[i] = spec
		} else {
			resp.Entries[i] = &cortexops.CapabilityBackendConfigSpec{
				Revision: corev1.NewRevision(rev.Revision(), rev.Timestamp()),
			}
		}
	}
	return resp, nil
}

var _ cortexops.CortexOpsServer = (*TestEnvMetricsClusterDriver)(nil)

func NewTestEnvMetricsClusterDriver(env *test.Environment) *TestEnvMetricsClusterDriver {
	d := &TestEnvMetricsClusterDriver{
		Env: env,
	}
	d.status.Store(&installStatusLocker{})
	defaultStore := inmemory.NewValueStore[*cortexops.CapabilityBackendConfigSpec](util.ProtoClone)
	activeStore := inmemory.NewValueStore[*cortexops.CapabilityBackendConfigSpec](util.ProtoClone)
	updateC, err := activeStore.Watch(env.Context())
	if err != nil {
		panic(err)
	}
	go func() {
		for entry := range updateC {
			go d.onActiveConfigChanged(entry.Previous.Value(), entry.Current.Value())
		}
	}()

	d.configTracker = driverutil.NewDefaultingConfigTracker[*cortexops.CapabilityBackendConfigSpec](
		defaultStore, activeStore, flagutil.LoadDefaults[*cortexops.CapabilityBackendConfigSpec],
	)
	d.DefaultConfigurableServer = driverutil.NewDefaultConfigurableServer[cortexops.CortexOpsServer](d.configTracker)
	return d
}

func (d *TestEnvMetricsClusterDriver) onActiveConfigChanged(old, new *cortexops.CapabilityBackendConfigSpec) {
	if !d.configLock.TryLock() {
		start := time.Now()
		d.Env.Logger.Info("Waiting for a previous config update to complete...")
		d.configLock.Lock()
		d.Env.Logger.With(
			zap.Duration("waited", time.Since(start)),
		).Info("Previous config update completed, continuing")
	}
	defer d.configLock.Unlock()

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

	// NB: apply changes to currentStatus instead of d.status in this function
	// so that context callbacks always mutate the status object that was active
	// at the time the context was created.
	currentStatus := &installStatusLocker{}
	d.status.Swap(currentStatus)

	if new == nil {
		return
	}

	currentStatus.Use(func(s *cortexops.InstallStatus) {
		s.ConfigState = cortexops.ConfigurationState_Configured
	})

	if !new.GetEnabled() {
		return
	}

	currentStatus.Use(func(s *cortexops.InstallStatus) {
		s.InstallState = cortexops.InstallState_Installed
		s.AppState = cortexops.ApplicationState_Pending
	})

	cmdCtx, err := d.Env.StartCortex(ctx, func(cco test.CortexConfigOptions, iso test.ImplementationSpecificOverrides) ([]byte, []byte, error) {
		overriders := configutil.MergeOverrideLists(
			configutil.NewHostOverrides(cco),
			configutil.NewImplementationSpecificOverrides(iso),
			[]configutil.CortexConfigOverrider{
				configutil.NewOverrider(func(t *filesystem.Config) bool {
					t.Directory = path.Join(d.Env.GetTempDirectory(), "cortex", "data")
					return true
				}),
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
					t.ResendDelay = 1 * time.Second
					t.Notifier.TLS.ServerName = "127.0.0.1"
					return true
				}),
				configutil.NewOverrider(func(t *tsdb.BucketStoreConfig) bool {
					t.SyncInterval = 10 * time.Second
					return true
				}),
			},
		)

		errs := configutil.ValidateConfiguration(new, overriders...)
		if len(errs) > 0 {
			currentStatus.Use(func(s *cortexops.InstallStatus) {
				for _, err := range errs {
					s.Warnings = append(s.Warnings, fmt.Sprintf("%s: %s", strings.ToLower(err.Severity.String()), err.Message))
				}
			})
		}

		cconf, rtconf, err := configutil.CortexAPISpecToCortexConfig(new.GetCortexConfig(), overriders...)
		if err != nil {
			return nil, nil, err
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

	if err != nil {
		currentStatus.Use(func(s *cortexops.InstallStatus) {
			s.AppState = cortexops.ApplicationState_Failed
			s.Warnings = append(s.Warnings, fmt.Sprintf("error: failed to start cortex"))
		})

		d.Env.Logger.Error("failed to start cortex")
		d.Env.Logger.Error(err)
		return
	}

	context.AfterFunc(cmdCtx, func() {
		currentStatus.Use(func(s *cortexops.InstallStatus) {
			s.AppState = cortexops.ApplicationState_Failed
			s.Warnings = append(s.Warnings, fmt.Sprintf("error: cortex exited unexpectedly"))
		})
	})

	currentStatus.Use(func(s *cortexops.InstallStatus) {
		if err != nil {
			s.AppState = cortexops.ApplicationState_Failed
		} else {
			s.AppState = cortexops.ApplicationState_Running
		}
	})
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

func (d *TestEnvMetricsClusterDriver) ShouldDisableNode(*corev1.Reference) (err error) {
	d.status.Load().Use(func(s *cortexops.InstallStatus) {
		switch s.InstallState {
		case cortexops.InstallState_NotInstalled, cortexops.InstallState_Uninstalling:
			err = status.Error(codes.Unavailable, fmt.Sprintf("Cortex cluster is not installed"))
		case cortexops.InstallState_Installed:
			err = nil
		default:
			// can't determine cluster status, so don't disable the node
			err = nil
		}
	})
	return
}

func (d *TestEnvMetricsClusterDriver) Status(context.Context, *emptypb.Empty) (out *cortexops.InstallStatus, _ error) {
	d.status.Load().Use(func(s *cortexops.InstallStatus) {
		out = s.DeepCopy()
		out.Version = cortexVersion
		out.Metadata = map[string]string{"test-environment": "true"}
	})
	return
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
		ctx, ca := context.WithCancel(d.env.Context())
		ctx = waitctx.FromContext(ctx)
		// this is the only place where UnsafeStartPrometheus is safe
		_, err := d.env.UnsafeStartPrometheus(ctx, nodeId)
		if err != nil {
			lg.With("err", err).Error("failed to start prometheus")
			ca()
			return err
		}
		lg.Info("started prometheus")
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
		ctx, ca := context.WithCancel(d.env.Context())
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
