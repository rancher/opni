package test

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
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
		return
	}
	for _, depInfo := range buildInfo.Deps {
		if depInfo.Path == "github.com/cortexproject/cortex" {
			if depInfo.Replace != nil {
				cortexVersion = depInfo.Replace.Version
			} else {
				cortexVersion = depInfo.Version
			}
			return
		}
	}
	panic("could not find cortex dependency in build info")
}

type TestEnvClusterDriver struct {
	cortexops.UnsafeCortexOpsServer

	lock         sync.Mutex
	state        cortexops.InstallState
	cortexCtx    context.Context
	cortexCancel context.CancelFunc

	Env           *Environment
	Configuration *cortexops.ClusterConfiguration
}

func NewTestEnvClusterDriver(env *Environment) *TestEnvClusterDriver {
	return &TestEnvClusterDriver{
		Env:           env,
		Configuration: &cortexops.ClusterConfiguration{},
		state:         cortexops.InstallState_NotInstalled,
	}
}

func (d *TestEnvClusterDriver) Name() string {
	return "test-environment"
}

func (d *TestEnvClusterDriver) ShouldDisableNode(*corev1.Reference) error {
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

func (d *TestEnvClusterDriver) GetClusterConfiguration(context.Context, *emptypb.Empty) (*cortexops.ClusterConfiguration, error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.Configuration, nil
}

func (d *TestEnvClusterDriver) ConfigureCluster(_ context.Context, conf *cortexops.ClusterConfiguration) (*emptypb.Empty, error) {
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

func (d *TestEnvClusterDriver) GetClusterStatus(context.Context, *emptypb.Empty) (*cortexops.InstallStatus, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	return &cortexops.InstallStatus{
		State:    d.state,
		Version:  cortexVersion,
		Metadata: map[string]string{"test-environment": "true"},
	}, nil
}

func (d *TestEnvClusterDriver) UninstallCluster(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
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
