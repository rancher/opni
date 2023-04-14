package test

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/test"
	metrics_agent_drivers "github.com/rancher/opni/plugins/metrics/pkg/agent/drivers"
	metrics_drivers "github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"

	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
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

	metrics_drivers.RegisterClusterDriverBuilder("test-environment", func(ctx context.Context, _ ...any) (metrics_drivers.ClusterDriver, error) {
		env := test.EnvFromContext(ctx)
		return NewTestEnvMetricsClusterDriver(env), nil
	})

	metrics_agent_drivers.RegisterNodeDriverBuilder("test-environment", func(ctx context.Context, _ ...any) (metrics_agent_drivers.MetricsNodeDriver, error) {
		env := test.EnvFromContext(ctx)
		return NewTestEnvMetricsNodeDriver(env), nil
	})

}

type TestEnvMetricsClusterDriver struct {
	cortexops.UnsafeCortexOpsServer

	lock         sync.Mutex
	state        cortexops.InstallState
	cortexCtx    context.Context
	cortexCancel context.CancelFunc

	Env           *test.Environment
	Configuration *cortexops.ClusterConfiguration
}

func NewTestEnvMetricsClusterDriver(env *test.Environment) *TestEnvMetricsClusterDriver {
	return &TestEnvMetricsClusterDriver{
		Env:           env,
		Configuration: &cortexops.ClusterConfiguration{},
		state:         cortexops.InstallState_NotInstalled,
	}
}

func NewTestEnvMetricsNodeDriver(env *test.Environment) *TestEnvMetricsNodeDriver {
	return &TestEnvMetricsNodeDriver{
		env: env,
	}
}

func (d *TestEnvMetricsClusterDriver) Name() string {
	return "test-environment"
}

func (d *TestEnvMetricsClusterDriver) ShouldDisableNode(*corev1.Reference) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	switch d.state {
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

func (d *TestEnvMetricsClusterDriver) GetClusterConfiguration(context.Context, *emptypb.Empty) (*cortexops.ClusterConfiguration, error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.Configuration, nil
}

func (d *TestEnvMetricsClusterDriver) ConfigureCluster(_ context.Context, conf *cortexops.ClusterConfiguration) (*emptypb.Empty, error) {
	d.lock.Lock()

	switch d.state {
	case cortexops.InstallState_NotInstalled, cortexops.InstallState_Installed:
		d.state = cortexops.InstallState_Updating
	case cortexops.InstallState_Updating:
		return nil, status.Error(codes.FailedPrecondition, "cluster is already being updated")
	case cortexops.InstallState_Uninstalling:
		return nil, status.Error(codes.FailedPrecondition, "cluster is currently being uninstalled")
	default:
		panic("bug: unknown state")
	}

	oldCtx, oldCancel := d.cortexCtx, d.cortexCancel

	ctx, ca := context.WithCancel(waitctx.FromContext(d.Env.Context()))
	d.cortexCtx = ctx
	d.cortexCancel = ca
	d.Configuration = conf

	d.lock.Unlock()

	go func() {
		if oldCancel != nil {
			oldCancel()
			waitctx.Wait(oldCtx)
		}
		d.Env.StartCortex(ctx)
		d.lock.Lock()
		defer d.lock.Unlock()
		d.state = cortexops.InstallState_Installed
	}()

	return &emptypb.Empty{}, nil
}

func (d *TestEnvMetricsClusterDriver) GetClusterStatus(context.Context, *emptypb.Empty) (*cortexops.InstallStatus, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	return &cortexops.InstallStatus{
		State:    d.state,
		Version:  cortexVersion,
		Metadata: map[string]string{"test-environment": "true"},
	}, nil
}

func (d *TestEnvMetricsClusterDriver) UninstallCluster(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	d.lock.Lock()

	switch d.state {
	case cortexops.InstallState_NotInstalled:
		return nil, status.Error(codes.FailedPrecondition, "cluster is not installed")
	case cortexops.InstallState_Installed, cortexops.InstallState_Updating:
		d.state = cortexops.InstallState_Uninstalling
	case cortexops.InstallState_Uninstalling:
		return nil, status.Error(codes.FailedPrecondition, "cluster is already being uninstalled")
	default:
		panic("bug: unknown state")
	}

	oldCtx, oldCancel := d.cortexCtx, d.cortexCancel

	d.lock.Unlock()

	go func() {
		oldCancel()
		waitctx.Wait(oldCtx)
		d.lock.Lock()
		defer d.lock.Unlock()
		d.cortexCtx = nil
		d.cortexCancel = nil
		d.state = cortexops.InstallState_NotInstalled
	}()

	return &emptypb.Empty{}, nil
}

type TestEnvMetricsNodeDriver struct {
	env *test.Environment

	prometheusMu     sync.Mutex
	prometheusCtx    context.Context
	prometheusCancel context.CancelFunc
}

func (d *TestEnvMetricsNodeDriver) ConfigureRuleGroupFinder(config *v1beta1.RulesSpec) notifier.Finder[rules.RuleGroup] {
	if config.GetDiscovery().Filesystem != nil {
		return rules.NewFilesystemRuleFinder(config.Discovery.Filesystem)
	}
	return nil
}

var _ metrics_agent_drivers.MetricsNodeDriver = (*TestEnvMetricsNodeDriver)(nil)

// ConfigureNode implements drivers.MetricsNodeDriver
func (d *TestEnvMetricsNodeDriver) ConfigureNode(nodeId string, conf *node.MetricsCapabilityConfig) {
	lg := d.env.Logger.With("node", nodeId)
	lg.Debug("configuring node")

	d.prometheusMu.Lock()
	defer d.prometheusMu.Unlock()

	exists := d.prometheusCtx != nil && d.prometheusCancel != nil
	shouldExist := conf.Enabled && conf.GetSpec().GetPrometheus().GetDeploymentStrategy() == "testEnvironment"

	if exists && !shouldExist {
		lg.Info("stopping prometheus")
		d.prometheusCancel()
		d.prometheusCancel = nil
		d.prometheusCtx = nil
	} else if !exists && shouldExist {
		lg.Info("starting prometheus")
		ctx, ca := context.WithCancel(d.env.Context())
		d.env.StartPrometheusContext(ctx, nodeId)
		d.prometheusCtx = ctx
		d.prometheusCancel = ca
	} else if exists && shouldExist {
		lg.Debug("nothing to do (already running)")
	} else {
		lg.Debug("nothing to do (already stopped)")
	}
}

// DiscoverPrometheuses implements drivers.MetricsNodeDriver
func (*TestEnvMetricsNodeDriver) DiscoverPrometheuses(context.Context, string) ([]*remoteread.DiscoveryEntry, error) {
	return nil, nil
}
