package test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	"github.com/google/uuid"
	amCfg "github.com/prometheus/alertmanager/config"

	"slices"

	"log/slog"

	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/extensions"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	node_drivers "github.com/rancher/opni/plugins/alerting/pkg/agent/drivers"
	alerting_drivers "github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v2"
)

func init() {
	routing.DefaultConfig = routing.Config{
		GlobalConfig: routing.GlobalConfig{
			GroupWait:      lo.ToPtr(model.Duration(1 * time.Second)),
			RepeatInterval: lo.ToPtr(model.Duration(5 * time.Hour)),
		},
		SubtreeConfig: routing.SubtreeConfig{
			GroupWait:      lo.ToPtr(model.Duration(1 * time.Second)),
			RepeatInterval: lo.ToPtr(model.Duration(5 * time.Hour)),
		},
		FinalizerConfig: routing.FinalizerConfig{
			InitialDelay:       time.Second * 1,
			ThrottlingDuration: time.Minute * 1,
			RepeatInterval:     time.Hour * 5,
		},
		NotificationConfg: routing.NotificationConfg{},
	}
}

type TestEnvAlertingClusterDriverOptions struct {
	AlertingOptions *shared.AlertingClusterOptions `option:"alertingOptions"`
	Subscribers     []chan client.AlertingClient   `option:"subscribers"`
}

type TestEnvAlertingClusterDriver struct {
	env              *test.Environment
	managedInstances []AlertingServerUnit
	enabled          *atomic.Bool
	ConfigFile       string
	stateMu          *sync.RWMutex
	logger           *slog.Logger

	*shared.AlertingClusterOptions

	*alertops.ClusterConfiguration

	alertops.UnsafeAlertingAdminServer
	client.AlertingClient

	embdServerAddress string
	subscribers       []chan client.AlertingClient
}

var (
	_ alerting_drivers.ClusterDriver = (*TestEnvAlertingClusterDriver)(nil)
	_ alertops.AlertingAdminServer   = (*TestEnvAlertingClusterDriver)(nil)
)

func NewTestEnvAlertingClusterDriver(env *test.Environment, options TestEnvAlertingClusterDriverOptions) *TestEnvAlertingClusterDriver {
	dir := env.GenerateNewTempDirectory("alertmanager-config")
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		panic(err)
	}
	configFile := path.Join(dir, "alertmanager.yaml")
	lg := logger.NewPluginLogger().WithGroup("alerting-test-cluster-driver")
	lg = lg.With("config-file", configFile)

	initial := &atomic.Bool{}
	initial.Store(false)
	ePort := freeport.GetFreePort()
	opniAddr := fmt.Sprintf("127.0.0.1:%d", ePort)
	_ = extensions.StartOpniEmbeddedServer(env.Context(), opniAddr, false)
	rTree := routing.NewRoutingTree(&config.WebhookConfig{
		NotifierConfig: config.NotifierConfig{
			VSendResolved: false,
		},
		URL: &amCfg.URL{
			URL: util.Must(url.Parse(fmt.Sprintf("http://%s%s", opniAddr, shared.AlertingDefaultHookName))),
		},
	})
	rTreeBytes, err := yaml.Marshal(rTree)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(configFile, rTreeBytes, 0644)
	if err != nil {
		panic(err)
	}

	return &TestEnvAlertingClusterDriver{
		env:                    env,
		managedInstances:       []AlertingServerUnit{},
		enabled:                initial,
		ConfigFile:             configFile,
		AlertingClusterOptions: options.AlertingOptions,
		ClusterConfiguration: &alertops.ClusterConfiguration{
			ResourceLimits: &alertops.ResourceLimitSpec{},
		},
		logger:            lg,
		subscribers:       options.Subscribers,
		stateMu:           &sync.RWMutex{},
		embdServerAddress: opniAddr,
	}
}

func (l *TestEnvAlertingClusterDriver) GetDefaultReceiver() *config.WebhookConfig {
	return &config.WebhookConfig{
		NotifierConfig: config.NotifierConfig{
			VSendResolved: false,
		},
		URL: &amCfg.URL{
			URL: util.Must(url.Parse(fmt.Sprintf("http://%s%s", l.embdServerAddress, shared.AlertingDefaultHookName))),
		},
	}
}

func (l *TestEnvAlertingClusterDriver) GetClusterConfiguration(_ context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	return l.ClusterConfiguration, nil
}

func (l *TestEnvAlertingClusterDriver) ConfigureCluster(_ context.Context, configuration *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
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
		l.AlertingClusterOptions.WorkerNodesService = "127.0.0.1"
		l.AlertingClusterOptions.WorkerNodePort = l.managedInstances[1].AlertManagerPort
		l.AlertingClusterOptions.OpniPort = l.managedInstances[1].OpniPort
	}
	l.ClusterConfiguration = configuration

	peers := []client.AlertingPeer{}
	for _, inst := range l.managedInstances {
		peers = append(peers, client.AlertingPeer{
			ApiAddress:      fmt.Sprintf("127.0.0.1:%d", inst.AlertManagerPort),
			EmbeddedAddress: l.embdServerAddress,
		})
	}
	l.AlertingClient.MemberlistClient().SetKnownPeers(peers)

	for _, subscriber := range l.subscribers {
		subscriber <- l.AlertingClient.Clone()
	}
	return &emptypb.Empty{}, nil
}

func (l *TestEnvAlertingClusterDriver) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
	if !l.enabled.Load() {
		return &alertops.InstallStatus{
			State: alertops.InstallState_NotInstalled,
		}, nil
	}
	l.stateMu.RLock()
	defer l.stateMu.RUnlock()
	if l.AlertingClient == nil {
		return &alertops.InstallStatus{
			State: alertops.InstallState_NotInstalled,
		}, nil
	}
	if err := l.AlertingClient.StatusClient().Ready(ctx); err != nil {
		l.logger.Error("error", logger.Err(err))
		return &alertops.InstallStatus{
			State: alertops.InstallState_InstallUpdating,
		}, nil
	}

	return &alertops.InstallStatus{
		State: alertops.InstallState_Installed,
	}, nil
}

func (l *TestEnvAlertingClusterDriver) InstallCluster(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
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
	var err error
	aClient, err := client.NewClient(
		client.WithAlertManagerAddress(
			fmt.Sprintf("127.0.0.1:%d", l.managedInstances[0].AlertManagerPort),
		),
		client.WithProxyAddress(
			fmt.Sprintf("127.0.0.1:%d", l.managedInstances[0].AlertManagerPort),
		),
		client.WithQuerierAddress(
			l.embdServerAddress,
		),
		client.WithTLSConfig(
			l.env.AlertingClientTLSConfig(),
		),
	)
	if err != nil {
		panic(err)
	}
	l.AlertingClient = aClient

	peers := []client.AlertingPeer{}
	for _, inst := range l.managedInstances {
		peers = append(peers, client.AlertingPeer{
			ApiAddress:      fmt.Sprintf("127.0.0.1:%d", inst.AlertManagerPort),
			EmbeddedAddress: l.embdServerAddress,
		})
	}
	l.AlertingClient.MemberlistClient().SetKnownPeers(peers)

	for _, subscriber := range l.subscribers {
		subscriber <- l.AlertingClient
	}

	l.enabled.Store(true)
	l.ClusterSettleTimeout = "1m0s"
	l.ClusterGossipInterval = "200ms"
	l.ClusterPushPullInterval = "1m0s"
	l.ResourceLimits.Cpu = "500m"
	l.ResourceLimits.Memory = "200Mi"
	l.ResourceLimits.Storage = "500Mi"
	l.AlertingClusterOptions.ControllerNodeService = "127.0.0.1"

	l.AlertingClusterOptions.ControllerClusterPort = l.managedInstances[0].ClusterPort
	l.AlertingClusterOptions.ControllerNodePort = l.managedInstances[0].AlertManagerPort
	l.AlertingClusterOptions.OpniPort = l.managedInstances[0].OpniPort
	return &emptypb.Empty{}, nil
}

func (l *TestEnvAlertingClusterDriver) UninstallCluster(_ context.Context, _ *alertops.UninstallRequest) (*emptypb.Empty, error) {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	for _, replica := range l.managedInstances {
		replica.CancelFunc()
	}
	l.managedInstances = []AlertingServerUnit{}
	l.enabled.Store(false)
	for _, subscriber := range l.subscribers {
		subscriber <- l.AlertingClient
	}
	return &emptypb.Empty{}, nil
}

func (l *TestEnvAlertingClusterDriver) Info(_ context.Context, _ *emptypb.Empty) (*alertops.ComponentInfo, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (l *TestEnvAlertingClusterDriver) Name() string {
	return "local-alerting"
}

func (l *TestEnvAlertingClusterDriver) ShouldDisableNode(_ *corev1.Reference) error {
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
		fmt.Sprintf("--syncer.listen.address=127.0.0.1:%d", syncerPort),
		fmt.Sprintf("--syncer.alertmanager.address=%s", "127.0.0.1:"+strconv.Itoa(webPort)),
		fmt.Sprintf("--syncer.gateway.join.address=%s", ":"+strings.Split(l.env.GatewayConfig().Spec.Management.GRPCListenAddress, ":")[2]),
		"syncer",
		fmt.Sprintf(
			"--syncer.tls.server.ca=%s",
			path.Join(l.env.AlertingDataDir(), "root.crt"),
		),
		fmt.Sprintf(
			"--syncer.tls.client.ca=%s",
			path.Join(l.env.AlertingDataDir(), "root.crt"),
		),
		fmt.Sprintf(
			"--syncer.tls.client.cert=%s",
			path.Join(l.env.AlertingDataDir(), "client.crt"),
		),
		fmt.Sprintf(
			"--syncer.tls.client.key=%s",
			path.Join(l.env.AlertingDataDir(), "client.key"),
		),
	}

	clusterPort := freeport.GetFreePort()

	alertmanagerArgs := []string{
		"alerting-server",
		"alertmanager",
		"--log.level=debug",
		"--log.format=json",
		fmt.Sprintf("--config.file=%s", configFilePath),
		fmt.Sprintf("--web.listen-address=127.0.0.1:%d", webPort),
		fmt.Sprintf("--cluster.listen-address=127.0.0.1:%d", clusterPort),
		"--storage.path=/tmp/data",
		fmt.Sprintf("--web.config.file=%s", path.Join(l.env.AlertingDataDir(), "web.yaml")),
	}

	if len(l.managedInstances) > 0 {
		for _, replica := range l.managedInstances {
			alertmanagerArgs = append(alertmanagerArgs,
				fmt.Sprintf("--cluster.peer=127.0.0.1:%d", replica.ClusterPort))
		}
		l.AlertingClusterOptions.WorkerNodesService = "127.0.0.1"
		l.AlertingClusterOptions.WorkerNodePort = webPort
	}

	ctxCa, cancelFunc := context.WithCancel(ctx)
	alertmanagerCmd := exec.CommandContext(ctxCa, opniBin, alertmanagerArgs...)
	plugins.ConfigureSysProcAttr(alertmanagerCmd)
	l.logger.Debug("Starting opni alertmanagwer with : " + strings.Join(alertmanagerArgs, " "))
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
	tlsConfig := l.env.AlertingClientTLSConfig()
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	for ctx.Err() == nil {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/-/ready", webPort), nil)
		if err != nil {
			panic(err)
		}
		resp, err := httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				l.logger.Info("Alertmanager successfully started")
				break
			} else {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					l.logger.Warn(err)
				}
				l.logger.With("code", resp.StatusCode, "resp", string(body)).Warnf("Alertmanager not ready yet : %d", resp.StatusCode)
			}
		}
		time.Sleep(time.Second)
		retries--
		if retries > 90 {
			panic("AlertManager failed to start")
		}
	}

	l.logger.Debug("Syncer starting with : " + strings.Join(syncerArgs, " "))
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

	l.logger.With("address", fmt.Sprintf("http://127.0.0.1:%d", webPort)).Info("AlertManager started")
	context.AfterFunc(ctx, func() {
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

type TestNodeDriver struct {
	whoami string
}

func NewTestNodeDriver() *TestNodeDriver {
	return &TestNodeDriver{
		whoami: uuid.New().String(),
	}
}

func (n *TestNodeDriver) ConfigureNode(_ string, _ *node.AlertingCapabilityConfig) error {
	return nil
}

func (n *TestNodeDriver) DiscoverRules(_ context.Context) (*rules.RuleManifest, error) {
	return &rules.RuleManifest{
		Rules: []*rules.Rule{
			{
				RuleId: &corev1.Reference{
					Id: fmt.Sprintf("test-rule-%s", n.whoami),
				},
				GroupId: &corev1.Reference{
					Id: fmt.Sprintf("test-group-%s", n.whoami),
				},
				Name:        "test",
				Expr:        "sum(up > 0) > 0",
				Duration:    durationpb.New(time.Second),
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
		},
	}, nil
}

func init() {
	node_drivers.NodeDrivers.Register("test_driver", func(ctx context.Context, opts ...driverutil.Option) (node_drivers.NodeDriver, error) {
		return NewTestNodeDriver(), nil
	})
	alerting_drivers.Drivers.Register("test-environment", func(ctx context.Context, opts ...driverutil.Option) (alerting_drivers.ClusterDriver, error) {
		env := test.EnvFromContext(ctx)
		options := TestEnvAlertingClusterDriverOptions{}
		driverutil.ApplyOptions(&options, opts...)
		return NewTestEnvAlertingClusterDriver(env, options), nil
	})
}
