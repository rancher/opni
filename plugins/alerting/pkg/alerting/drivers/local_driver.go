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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"google.golang.org/protobuf/types/known/emptypb"
)

var LocalDriverBinaryPath = "./"

type LocalManager struct {
	alertingOptionsMu sync.RWMutex
	configPersistMu   sync.RWMutex

	managedInstances []LocalClusterMember
	enabled          *atomic.Bool
	clusterPort      int
	*LocalManagerDriverOptions

	alertops.UnsafeAlertingAdminServer
}

var _ ClusterDriver = (*LocalManager)(nil)
var _ alertops.AlertingAdminServer = (*LocalManager)(nil)

type LocalManagerDriverOptions struct {
	*alertops.ClusterConfiguration
	*alertops.InstallStatus
	*shared.NewAlertingOptions
	alertManagerConfigPath string
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

func WithLocalManagerContext(ctx context.Context) LocalManagerDriverOption {
	return func(o *LocalManagerDriverOptions) {
		o.ctx = ctx
	}
}

func NewLocalManager(opts ...LocalManagerDriverOption) *LocalManager {
	options := &LocalManagerDriverOptions{
		alertManagerConfigPath: "/tmp/alertmanager.yaml",
		NewAlertingOptions:     &shared.NewAlertingOptions{},
		ClusterConfiguration: &alertops.ClusterConfiguration{
			NumReplicas:    1,
			ResourceLimits: &alertops.ResourceLimitSpec{},
		},
	}
	options.apply(opts...)
	rTree := routing.NewDefaultRoutingTree("http://localhost:11080")
	rTreeBytes, err := yaml.Marshal(rTree)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(options.alertManagerConfigPath, rTreeBytes, 0644)
	if err != nil {
		panic(err)
	}
	initial := &atomic.Bool{}
	initial.Store(false)
	return &LocalManager{

		managedInstances:          []LocalClusterMember{},
		enabled:                   initial,
		LocalManagerDriverOptions: options,
	}
}

func (l *LocalManager) GetClusterConfiguration(_ context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	return l.ClusterConfiguration, nil
}

func (l *LocalManager) ConfigureCluster(_ context.Context, configuration *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	if err := configuration.Validate(); err != nil {
		return nil, err
	}
	//TODO
	// cur := l.ClusterConfiguration
	// if int(cur.NumReplicas) != len(l.replicaInstance) {
	// 	panic(fmt.Sprintf("current cluster  config indicates %d replicas but we have %d replicas running",
	// 		cur.NumReplicas, len(l.replicaInstance)))
	// }
	// if cur.NumReplicas > configuration.NumReplicas { // shrink replicas
	// 	for i := cur.NumReplicas - 1; i > configuration.NumReplicas-1; i-- {
	// 		l.replicaInstance[i].cancelFunc()
	// 	}
	// 	l.replicaInstance = slices.Delete(l.replicaInstance, int(configuration.NumReplicas-1), int(cur.NumReplicas-1))
	// } else if cur.NumReplicas < configuration.NumReplicas { // grow replicas
	// 	for i := cur.NumReplicas; i < configuration.NumReplicas; i++ {
	// 		l.replicaInstance = append(l.replicaInstance, startNewReplica(l.ctx, l.alertManagerConfigPath, l.Logger, &l.clusterPort))
	// 	}
	// }
	// if len(l.replicaInstance) > 1 {
	// 	l.NewAlertingOptions.WorkerNodesService = "http://localhost"
	// 	l.NewAlertingOptions.WorkerNodePort = l.replicaInstance[1].apiPort
	// }
	// l.ClusterConfiguration = configuration
	return &emptypb.Empty{}, nil
}

func (l *LocalManager) GetClusterStatus(ctx context.Context, empty *emptypb.Empty) (*alertops.InstallStatus, error) {
	if l.enabled.Load() {
		return &alertops.InstallStatus{
			State: alertops.InstallState_Installed,
		}, nil
	}
	return &alertops.InstallStatus{
		State: alertops.InstallState_NotInstalled,
	}, nil
}

func (l *LocalManager) InstallCluster(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	if len(l.managedInstances) > 0 {
		panic("should not have existing replicas")
	}
	if l.enabled.Load() {
		return &emptypb.Empty{}, nil
	}
	l.NumReplicas = 1
	l.enabled = &atomic.Bool{}
	l.enabled.Store(true)
	l.ClusterSettleTimeout = "1m0s"
	l.ClusterGossipInterval = "200ms"
	l.ClusterPushPullInterval = "1m0s"
	l.ResourceLimits.Cpu = "500m"
	l.ResourceLimits.Memory = "200Mi"
	l.ResourceLimits.Storage = "500Mi"
	l.LocalManagerDriverOptions.NewAlertingOptions.ControllerNodeService = "http://localhost"

	// TODO : set the cluster join port when we start the instance
	// TODO : set the default cluster work port here too
	// l.replicaInstance = append(l.replicaInstance, startNewReplica(l.ctx, l.alertManagerConfigPath, l.Logger, nil))
	// l.clusterPort = l.replicaInstance[0].clusterPort

	// l.LocalManagerDriverOptions.NewAlertingOptions.ControllerClusterPort = l.replicaInstance[0].clusterPort
	// l.LocalManagerDriverOptions.NewAlertingOptions.ControllerNodePort = l.replicaInstance[0].apiPort
	return &emptypb.Empty{}, nil
}

func (l *LocalManager) UninstallCluster(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	for _, replica := range l.managedInstances {
		err := replica.Stop()
		if err != nil {
			l.Logger.Error(err)
		}
	}
	l.managedInstances = []LocalClusterMember{}
	l.enabled.Store(false)
	return &emptypb.Empty{}, nil
}

func (l *LocalManager) Name() string {
	return "local-alerting"
}

func (l *LocalManager) ShouldDisableNode(_ *corev1.Reference) error {
	return nil
}

// read only view
func (l *LocalManager) GetRuntimeOptions() (shared.NewAlertingOptions, error) {
	if l.NewAlertingOptions == nil {
		return shared.NewAlertingOptions{}, fmt.Errorf("no runtime options set")
	}
	return *l.NewAlertingOptions, nil
}

type LocalClusterMember interface {
	Start() error
	Stop() error
}

type alertManagerInstance struct {
	apiPort     int
	clusterPort int
	ctx         context.Context
	cancelFunc  context.CancelFunc
}

type alertSyncerInstance struct {
	port       int
	ctx        context.Context
	cancelFunc context.CancelFunc
}

func (l *LocalManager) startEmbeddedAlertManager(
	ctx context.Context,
	configFilePath string,
	port int,
	clusterPort int,
	optionalClusterJoinPort *int,
) alertManagerInstance {
	amBin := path.Join(LocalDriverBinaryPath, "bin/opni")
	defaultArgs := []string{}
	if optionalClusterJoinPort != nil {
		defaultArgs = []string{
			"alerting-server",
			"alertmanager",
			fmt.Sprintf("--config.file=%s", configFilePath),
			fmt.Sprintf("--web.listen-address=:%d", port),
			fmt.Sprintf("--cluster.peer=:%s", "http://localhost:"+strconv.Itoa(*optionalClusterJoinPort)),
			"--storage.path=/tmp/data",
			"--log.level=debug",
		}
	} else {
		defaultArgs = []string{
			"alerting-server",
			"alertmanager",
			fmt.Sprintf("--config.file=%s", configFilePath),
			fmt.Sprintf("--web.listen-address=:%d", port),
			fmt.Sprintf("--cluster.listen-address=:%d", clusterPort),
			"--storage.path=/tmp/data",
			"--log.level=debug",
		}
	}

	ctxCa, cancelFunc := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctxCa, amBin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	l.Logger.With("port", port).Info("Starting AlertManager")
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
	l.Logger.With("address", fmt.Sprintf("http://localhost:%d", port)).Info("AlertManager started")
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		cmd, _ := session.G()
		if cmd != nil {
			cmd.Signal(os.Signal(syscall.SIGTERM))
		}
	})
	return alertManagerInstance{
		apiPort:     port,
		clusterPort: clusterPort,
		ctx:         ctxCa,
		cancelFunc:  cancelFunc,
	}
}

func startEmbeddedSyncServer(ctx context.Context) LocalClusterMember {
	//TODO
	return nil
}

func (l *LocalManager) startAlertingClusterMember(
	ctx context.Context,
	workerPort int,
	optionalClusterJoinPort *int) LocalClusterMember {
	// TODO
	return nil
}
