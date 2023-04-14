package test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"emperror.dev/errors"
	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/waitctx"
	alerting_drivers "github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v2"
)

func init() {
	alerting_drivers.RegisterClusterDriverBuilder("test-environment", func(ctx context.Context, args ...any) (alerting_drivers.ClusterDriver, error) {
		env := test.EnvFromContext(ctx)
		return NewTestEnvAlertingClusterDriver(env, args...), nil
	})
}

type TestEnvAlertingClusterDriver struct {
	env              *test.Environment
	managedInstances []AlertingServerUnit
	enabled          *atomic.Bool
	ConfigFile       string
	stateMu          *sync.RWMutex
	logger           *zap.SugaredLogger

	*shared.AlertingClusterOptions

	*alertops.ClusterConfiguration

	alertops.UnsafeAlertingAdminServer

	subscribers []chan shared.AlertingClusterNotification
}

var _ alerting_drivers.ClusterDriver = (*TestEnvAlertingClusterDriver)(nil)
var _ alertops.AlertingAdminServer = (*TestEnvAlertingClusterDriver)(nil)

func NewTestEnvAlertingClusterDriver(env *test.Environment, args ...any) *TestEnvAlertingClusterDriver {
	dir := env.GenerateNewTempDirectory("alertmanager-config")
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		panic(err)
	}
	configFile := path.Join(dir, "alertmanager.yaml")
	lg := logger.NewPluginLogger().Named("alerting-test-cluster-driver")
	lg = lg.With("config-file", configFile)
	rTree := routing.NewRoutingTree("http://localhost:6000")
	rTreeBytes, err := yaml.Marshal(rTree)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(configFile, rTreeBytes, 0644)
	if err != nil {
		panic(err)
	}
	initial := &atomic.Bool{}
	initial.Store(false)

	var (
		subscribers    []chan shared.AlertingClusterNotification
		clusterOptions = &shared.AlertingClusterOptions{}
	)
	for _, arg := range args {
		switch opt := arg.(type) {
		case chan shared.AlertingClusterNotification:
			subscribers = append(subscribers, opt)
		case *shared.AlertingClusterOptions:
			clusterOptions = opt
		default:
			lg.With("option", opt).Warn("unexpected option")
		}
	}

	return &TestEnvAlertingClusterDriver{
		env:                    env,
		managedInstances:       []AlertingServerUnit{},
		enabled:                initial,
		ConfigFile:             configFile,
		AlertingClusterOptions: clusterOptions,
		ClusterConfiguration: &alertops.ClusterConfiguration{
			ResourceLimits: &alertops.ResourceLimitSpec{},
		},
		logger:      lg,
		subscribers: subscribers,
		stateMu:     &sync.RWMutex{},
	}
}

func (l *TestEnvAlertingClusterDriver) GetClusterConfiguration(ctx context.Context, empty *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	return l.ClusterConfiguration, nil
}

func (l *TestEnvAlertingClusterDriver) ConfigureCluster(ctx context.Context, configuration *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	if err := configuration.Validate(); err != nil {
		return nil, err
	}
	cur := l.ClusterConfiguration
	if int(cur.NumReplicas) != len(l.managedInstances) {
		panic(fmt.Sprintf("current cluster  config indicates %d replicas but we have %d replicas running",
			cur.NumReplicas, len(l.managedInstances)))
	}
	if cur.NumReplicas > configuration.NumReplicas { // shrink replicas
		for i := cur.NumReplicas - 1; i > configuration.NumReplicas-1; i-- {
			l.managedInstances[i].CancelFunc()
		}
		l.managedInstances = slices.Delete(l.managedInstances, int(configuration.NumReplicas-1), int(cur.NumReplicas-1))
	} else if cur.NumReplicas < configuration.NumReplicas { // grow replicas
		for i := cur.NumReplicas; i < configuration.NumReplicas; i++ {
			l.managedInstances = append(
				l.managedInstances,
				l.StartAlertingBackendServer(l.env.Context(), l.ConfigFile),
			)
		}
	}
	if len(l.managedInstances) > 1 {
		l.AlertingClusterOptions.WorkerNodesService = "localhost"
		l.AlertingClusterOptions.WorkerNodePort = l.managedInstances[1].AlertManagerPort
		l.AlertingClusterOptions.OpniPort = l.managedInstances[1].OpniPort
	}
	l.ClusterConfiguration = configuration

	for _, subscriber := range l.subscribers {
		subscriber <- shared.AlertingClusterNotification{
			A: true,
			B: l.AlertingClusterOptions,
		}
	}
	return &emptypb.Empty{}, nil
}

func (l *TestEnvAlertingClusterDriver) GetClusterStatus(ctx context.Context, empty *emptypb.Empty) (*alertops.InstallStatus, error) {
	if !l.enabled.Load() {
		return &alertops.InstallStatus{
			State: alertops.InstallState_NotInstalled,
		}, nil
	}
	l.stateMu.RLock()
	defer l.stateMu.RUnlock()
	for _, replica := range l.managedInstances {
		apiNode := backend.NewAlertManagerReadyClient(ctx, fmt.Sprintf("127.0.0.1:%d", replica.AlertManagerPort))
		if err := apiNode.DoRequest(); err != nil {
			return &alertops.InstallStatus{
				State: alertops.InstallState_InstallUpdating,
			}, nil
		}
	}

	return &alertops.InstallStatus{
		State: alertops.InstallState_Installed,
	}, nil
}

func (l *TestEnvAlertingClusterDriver) InstallCluster(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	if l.enabled.Load() {
		return &emptypb.Empty{}, nil
	}
	if len(l.managedInstances) > 0 {
		panic("should not have existing replicas")
	}
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	l.NumReplicas = 1
	for i := 0; i < int(l.NumReplicas); i++ {
		l.managedInstances = append(
			l.managedInstances,
			l.StartAlertingBackendServer(l.env.Context(), l.ConfigFile),
		)
	}

	l.enabled.Store(true)
	l.ClusterSettleTimeout = "1m0s"
	l.ClusterGossipInterval = "200ms"
	l.ClusterPushPullInterval = "1m0s"
	l.ResourceLimits.Cpu = "500m"
	l.ResourceLimits.Memory = "200Mi"
	l.ResourceLimits.Storage = "500Mi"
	l.AlertingClusterOptions.ControllerNodeService = "localhost"

	l.AlertingClusterOptions.ControllerClusterPort = l.managedInstances[0].ClusterPort
	l.AlertingClusterOptions.ControllerNodePort = l.managedInstances[0].AlertManagerPort
	l.AlertingClusterOptions.OpniPort = l.managedInstances[0].OpniPort

	rTree := routing.NewRoutingTree(fmt.Sprintf("http://localhost:%d", l.managedInstances[0].OpniPort))
	rTreeBytes, err := yaml.Marshal(rTree)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(l.ConfigFile, rTreeBytes, 0644)
	if err != nil {
		panic(err)
	}

	for _, subscriber := range l.subscribers {
		subscriber <- shared.AlertingClusterNotification{
			A: true,
			B: l.AlertingClusterOptions,
		}
	}
	return &emptypb.Empty{}, nil
}

func (l *TestEnvAlertingClusterDriver) UninstallCluster(ctx context.Context, req *alertops.UninstallRequest) (*emptypb.Empty, error) {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	for _, replica := range l.managedInstances {
		replica.CancelFunc()
	}
	l.managedInstances = []AlertingServerUnit{}
	l.enabled.Store(false)
	for _, subscriber := range l.subscribers {
		subscriber <- shared.AlertingClusterNotification{
			A: false,
			B: nil,
		}
	}
	return &emptypb.Empty{}, nil
}

func (l *TestEnvAlertingClusterDriver) GetRuntimeOptions() shared.AlertingClusterOptions {
	return *l.AlertingClusterOptions
}

func (l *TestEnvAlertingClusterDriver) Name() string {
	return "local-alerting"
}

func (l *TestEnvAlertingClusterDriver) ShouldDisableNode(reference *corev1.Reference) error {
	return nil
}

type AlertingServerUnit struct {
	AlertManagerPort int
	ClusterPort      int
	OpniPort         int
	Ctx              context.Context
	CancelFunc       context.CancelFunc
}

func (l *TestEnvAlertingClusterDriver) StartAlertingBackendServer(
	ctx context.Context,
	configFilePath string,
) AlertingServerUnit {
	opniBin := path.Join(l.env.TestBin, "../../bin/opni")
	webPort := freeport.GetFreePort()
	opniPort := freeport.GetFreePort()
	syncerPort := freeport.GetFreePort()
	syncerArgs := []string{
		"alerting-server",
		fmt.Sprintf("--syncer.alertmanager.config.file=%s", configFilePath),
		fmt.Sprintf("--syncer.listen.address=:%d", syncerPort),
		fmt.Sprintf("--syncer.alertmanager.address=%s", "http://localhost:"+strconv.Itoa(webPort)),
		fmt.Sprintf("--syncer.gateway.join.address=%s", ":"+strings.Split(l.env.GatewayConfig().Spec.Management.GRPCListenAddress, ":")[2]),
		"syncer",
	}

	l.logger.Debug("Syncer start : " + strings.Join(syncerArgs, " "))

	clusterPort := freeport.GetFreePort()

	alertmanagerArgs := []string{
		"alerting-server",
		"alertmanager",
		fmt.Sprintf("--config.file=%s", configFilePath),
		fmt.Sprintf("--web.listen-address=:%d", webPort),
		fmt.Sprintf("--opni.listen-address=:%d", opniPort),
		fmt.Sprintf("--cluster.listen-address=:%d", clusterPort),
		"--storage.path=/tmp/data",
		// "--log.level=debug",
	}

	if len(l.managedInstances) > 0 {
		for _, replica := range l.managedInstances {
			alertmanagerArgs = append(alertmanagerArgs,
				fmt.Sprintf("--cluster.peer=localhost:%d", replica.ClusterPort))
		}
		l.AlertingClusterOptions.WorkerNodesService = "localhost"
		l.AlertingClusterOptions.WorkerNodePort = webPort
	}

	ctxCa, cancelFunc := context.WithCancel(ctx)
	alertmanagerCmd := exec.CommandContext(ctxCa, opniBin, alertmanagerArgs...)
	plugins.ConfigureSysProcAttr(alertmanagerCmd)
	l.logger.With("alertmanager-port", webPort, "opni-port", opniPort).Info("Starting AlertManager")
	session, err := testutil.StartCmd(alertmanagerCmd)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			panic(fmt.Sprintf("%s : opni bin path : %s", err, opniBin))
		} else {
			panic(err)
		}
	}
	retries := 0
	for ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", webPort))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
		retries += 1
		if retries > 90 {
			panic("AlertManager failed to start")
		}
	}

	syncerCmd := exec.CommandContext(ctxCa, opniBin, syncerArgs...)
	plugins.ConfigureSysProcAttr(syncerCmd)
	l.logger.With("port", syncerPort).Info("Starting AlertManager Syncer")
	_, err = testutil.StartCmd(syncerCmd)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			panic(fmt.Sprintf("%s : opni bin path : %s", err, opniBin))
		} else {
			panic(err)
		}
	}

	l.logger.With("address", fmt.Sprintf("http://localhost:%d", webPort)).Info("AlertManager started")
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		cmd, _ := session.G()
		if cmd != nil {
			cmd.Signal(os.Signal(syscall.SIGTERM))
		}
	})
	return AlertingServerUnit{
		AlertManagerPort: webPort,
		ClusterPort:      clusterPort,
		OpniPort:         opniPort,
		Ctx:              ctxCa,
		CancelFunc:       cancelFunc,
	}
}
