package drivers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"sync"
	"syscall"
	"time"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/phayes/freeport"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"google.golang.org/protobuf/types/known/emptypb"
)

var RuntimeBinaryPath = "./"

type localRunningInstance struct {
	apiPort     int
	clusterPort int
	ctx         context.Context
	cancelFunc  context.CancelFunc
}

func startNewReplica(ctx context.Context, configFilePath string, lg *zap.SugaredLogger, optionalClusterJoinPort *int) localRunningInstance {
	port, err := freeport.GetFreePort()
	fmt.Printf("AlertManager port %d", port)
	if err != nil {
		panic(err)
	}
	clusterPort, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}

	//TODO: fixme relative path only works for one of tests or mage test:env, but not both
	amBin := path.Join(RuntimeBinaryPath, "bin/opni")
	defaultArgs := []string{}
	if optionalClusterJoinPort != nil {
		defaultArgs = []string{
			"alertmanager",
			fmt.Sprintf("--config.file=%s", configFilePath),
			fmt.Sprintf("--web.listen-address=:%d", port),
			fmt.Sprintf("--cluster.peer=:%s", "http://localhost:"+strconv.Itoa(*optionalClusterJoinPort)),
			"--storage.path=/tmp/data",
			"--log.level=debug",
		}
	} else {
		defaultArgs = []string{
			"alertmanager",
			fmt.Sprintf("--config.file=%s", configFilePath),
			fmt.Sprintf("--web.listen-address=:%d", port),
			fmt.Sprintf("--cluster.listen-address=:%d", clusterPort),
			"--storage.path=/tmp/data",
			"--log.level=debug",
		}
	}

	ctxCa, cancelFunc := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, amBin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	lg.With("port", port).Info("Starting AlertManager")
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			panic(fmt.Sprintf("%s : ambin path : %s", err, amBin))
		} else {
			panic(err)
		}
	}
	for ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", port))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	lg.With("address", fmt.Sprintf("http://localhost:%d", port)).Info("AlertManager started")
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		cmd, _ := session.G()
		if cmd != nil {
			cmd.Signal(os.Signal(syscall.SIGTERM))
		}
	})
	return localRunningInstance{
		apiPort:     port,
		clusterPort: clusterPort,
		ctx:         ctxCa,
		cancelFunc:  cancelFunc,
	}
}

type LocalManager struct {
	alertingOptionsMu sync.RWMutex
	configPersistMu   sync.RWMutex

	replicaInstance []localRunningInstance
	enabled         bool
	clusterPort     int
	*LocalManagerDriverOptions

	alertops.UnsafeAlertingAdminServer
	alertops.UnsafeDynamicAlertingServer
}

var _ ClusterDriver = (*LocalManager)(nil)
var _ alertops.AlertingAdminServer = (*LocalManager)(nil)
var _ alertops.DynamicAlertingServer = (*LocalManager)(nil)

type LocalManagerDriverOptions struct {
	*alertops.ClusterConfiguration
	*alertops.InstallStatus
	*shared.NewAlertingOptions
	alertManagerConfigPath string
	internalRoutingPath    string
	Logger                 *zap.SugaredLogger
	ctx                    context.Context
}

type LocalManagerDriverOption func(*LocalManagerDriverOptions)

func (o *LocalManagerDriverOptions) apply(opts ...LocalManagerDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLocalManagerLogger(logger *zap.SugaredLogger) LocalManagerDriverOption {
	return func(o *LocalManagerDriverOptions) {
		o.Logger = logger
	}
}

func WithLocalAlertingOptions(options shared.NewAlertingOptions) LocalManagerDriverOption {
	return func(o *LocalManagerDriverOptions) {
		o.NewAlertingOptions = &options
	}
}

func WithLocalAlertingConfigPath(path string) LocalManagerDriverOption {
	return func(o *LocalManagerDriverOptions) {
		o.alertManagerConfigPath = path
	}
}

func WithLocalRoutingConfigPath(path string) LocalManagerDriverOption {
	return func(o *LocalManagerDriverOptions) {
		o.internalRoutingPath = path
	}
}

func WithLocalManagerContext(ctx context.Context) LocalManagerDriverOption {
	return func(o *LocalManagerDriverOptions) {
		o.ctx = ctx
	}
}

func NewLocalManager(opts ...LocalManagerDriverOption) *LocalManager {
	options := &LocalManagerDriverOptions{
		alertManagerConfigPath: "/tmp/alertmanager.yaml",
		internalRoutingPath:    "/tmp/routing.yaml",
		NewAlertingOptions:     &shared.NewAlertingOptions{},
		ClusterConfiguration: &alertops.ClusterConfiguration{
			NumReplicas:    1,
			ResourceLimits: &alertops.ResourceLimitSpec{},
		},
	}
	options.apply(opts...)
	rTree := routing.NewDefaultRoutingTree("http://localhost:11080")
	rTreeBytes, err := rTree.Marshal()
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(options.alertManagerConfigPath, rTreeBytes, 0644)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(options.internalRoutingPath, []byte{}, 0644)
	if err != nil {
		panic(err)
	}
	return &LocalManager{
		replicaInstance:           []localRunningInstance{},
		enabled:                   false,
		LocalManagerDriverOptions: options,
	}
}

func (l *LocalManager) GetClusterConfiguration(ctx context.Context, empty *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	return l.ClusterConfiguration, nil
}

func (l *LocalManager) ConfigureCluster(ctx context.Context, configuration *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	if err := configuration.Validate(); err != nil {
		return nil, err
	}
	cur := l.ClusterConfiguration
	if int(cur.NumReplicas) != len(l.replicaInstance) {
		panic(fmt.Sprintf("current cluster  config indicates %d replicas but we have %d replicas running",
			cur.NumReplicas, len(l.replicaInstance)))
	}
	if cur.NumReplicas > configuration.NumReplicas { // shrink replicas
		for i := cur.NumReplicas - 1; i > configuration.NumReplicas-1; i-- {
			l.replicaInstance[i].cancelFunc()
		}
		l.replicaInstance = slices.Delete(l.replicaInstance, int(configuration.NumReplicas-1), int(cur.NumReplicas-1))
	} else if cur.NumReplicas < configuration.NumReplicas { // grow replicas
		for i := cur.NumReplicas; i < configuration.NumReplicas; i++ {
			l.replicaInstance = append(l.replicaInstance, startNewReplica(l.ctx, l.alertManagerConfigPath, l.Logger, &l.clusterPort))
		}
	}
	if len(l.replicaInstance) > 1 {
		l.NewAlertingOptions.WorkerNodesService = "http://localhost"
		l.NewAlertingOptions.WorkerNodePort = l.replicaInstance[1].apiPort
	}
	l.ClusterConfiguration = configuration
	return &emptypb.Empty{}, nil
}

func (l *LocalManager) GetClusterStatus(ctx context.Context, empty *emptypb.Empty) (*alertops.InstallStatus, error) {
	if l.enabled {
		return &alertops.InstallStatus{
			State: alertops.InstallState_Installed,
		}, nil
	} else {
		return &alertops.InstallStatus{
			State: alertops.InstallState_NotInstalled,
		}, nil
	}
}

func (l *LocalManager) InstallCluster(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	if len(l.replicaInstance) > 0 {
		panic("should not have existing replicas")
	}
	if l.enabled {
		return &emptypb.Empty{}, nil
	}
	l.NumReplicas = 1
	// TODO: set cluster configuration to defaults
	l.enabled = true
	l.ClusterSettleTimeout = "1m0s"
	l.ClusterGossipInterval = "200ms"
	l.ClusterPushPullInterval = "1m0s"
	l.ResourceLimits.Cpu = "500m"
	l.ResourceLimits.Memory = "200Mi"
	l.ResourceLimits.Storage = "500Mi"

	l.replicaInstance = append(l.replicaInstance, startNewReplica(l.ctx, l.alertManagerConfigPath, l.Logger, nil))
	l.clusterPort = l.replicaInstance[0].clusterPort
	l.LocalManagerDriverOptions.NewAlertingOptions.ControllerNodeService = "http://localhost"
	l.LocalManagerDriverOptions.NewAlertingOptions.ControllerClusterPort = l.replicaInstance[0].clusterPort
	l.LocalManagerDriverOptions.NewAlertingOptions.ControllerNodePort = l.replicaInstance[0].apiPort
	return &emptypb.Empty{}, nil
}

func (l *LocalManager) UninstallCluster(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	for _, replica := range l.replicaInstance {
		replica.cancelFunc()
	}
	l.replicaInstance = []localRunningInstance{}
	l.enabled = false
	return &emptypb.Empty{}, nil
}

func (l *LocalManager) Fetch(ctx context.Context, empty *emptypb.Empty) (*alertops.AlertingConfig, error) {
	alertManagerBytes, err := os.ReadFile(l.alertManagerConfigPath)
	if err != nil {
		return nil, err
	}
	routingBytes, err := os.ReadFile(l.internalRoutingPath)
	if err != nil {
		return nil, err
	}
	return &alertops.AlertingConfig{
		RawAlertManagerConfig: string(alertManagerBytes),
		RawInternalRouting:    string(routingBytes),
	}, nil
}

func (l *LocalManager) Update(ctx context.Context, config *alertops.AlertingConfig) (*emptypb.Empty, error) {
	if err := os.WriteFile(l.alertManagerConfigPath, []byte(config.RawAlertManagerConfig), 0644); err != nil {
		return nil, err
	}

	if err := os.WriteFile(l.internalRoutingPath, []byte(config.RawInternalRouting), 0644); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (l *LocalManager) Reload(ctx context.Context, info *alertops.ReloadInfo) (*emptypb.Empty, error) {
	lg := l.Logger.With("alerting-backend", "k8s", "action", "reload")

	wg := sync.WaitGroup{}
	errors := &sharedErrors{}
	for _, replica := range l.replicaInstance {
		wg.Add(1)
		endpoint := "http://localhost:" + strconv.Itoa(replica.apiPort)
		pipelineRetrier := backoffv2.Exponential(
			backoffv2.WithMinInterval(time.Second*2),
			backoffv2.WithMaxInterval(time.Second*5),
			backoffv2.WithMaxRetries(3),
			backoffv2.WithMultiplier(1.2),
		)

		go func() {
			defer wg.Done()
			pipelineErr := backend.NewApiPipline(
				ctx,
				[]*backend.AlertManagerAPI{
					backend.NewAlertManagerReloadClient(
						endpoint,
						ctx,
						backend.WithRetrier(pipelineRetrier),
						backend.WithExpectClosure(backend.NewExpectStatusOk()),
					),
					backend.NewAlertManagerReadyClient(
						endpoint,
						ctx,
						backend.WithRetrier(pipelineRetrier),
						backend.WithExpectClosure(backend.NewExpectStatusOk()),
					),
					backend.NewAlertManagerStatusClient(
						endpoint,
						ctx,
						backend.WithRetrier(pipelineRetrier),
						backend.WithExpectClosure(backend.NewExpectStatusOk()),
					),
				},
				&pipelineRetrier,
			)
			if pipelineErr != nil {
				lg.Error(pipelineErr)
				appendError(errors, fmt.Errorf("pipeline error for %s : %s", endpoint, pipelineErr))
			}
		}()
	}
	wg.Wait()
	return &emptypb.Empty{}, nil
}

func (l *LocalManager) Name() string {
	return "local-alerting"
}

func (l *LocalManager) ShouldDisableNode(reference *corev1.Reference) error {
	return nil
}

// read only view
func (l *LocalManager) GetRuntimeOptions() (shared.NewAlertingOptions, error) {
	if l.NewAlertingOptions == nil {
		return shared.NewAlertingOptions{}, fmt.Errorf("no runtime options set")
	}
	return *l.NewAlertingOptions, nil
}

func (l *LocalManager) ConfigFromBackend(ctx context.Context) (*routing.RoutingTree, *routing.OpniInternalRouting, error) {
	mu.Lock()
	defer mu.Unlock()
	rawConfig, err := l.Fetch(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, nil, err
	}
	config, err := routing.NewRoutingTreeFrom(rawConfig.RawAlertManagerConfig)
	if err != nil {
		return nil, nil, err
	}
	internal, err := routing.NewOpniInternalRoutingFrom(rawConfig.RawInternalRouting)
	if err != nil {
		return nil, nil, err
	}
	return config, internal, nil
}

func (l *LocalManager) ApplyConfigToBackend(ctx context.Context, config *routing.RoutingTree, internal *routing.OpniInternalRouting) error {
	rawAlertManagerData, err := config.Marshal()
	if err != nil {
		return err
	}
	rawInternalRoutingData, err := internal.Marshal()
	if err != nil {
		return err
	}
	_, err = l.Update(ctx, &alertops.AlertingConfig{
		RawAlertManagerConfig: string(rawAlertManagerData),
		RawInternalRouting:    string(rawInternalRoutingData),
	})
	if err != nil {
		return err
	}
	_, err = l.Reload(ctx, &alertops.ReloadInfo{
		UpdatedConfig: string(rawAlertManagerData),
	})
	if err != nil {
		return err
	}
	return nil
}
