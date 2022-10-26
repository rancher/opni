package test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/mattn/go-tty"
	"github.com/onsi/ginkgo/v2"
	"github.com/phayes/freeport"
	"github.com/pkg/browser"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samber/lo"
	"github.com/ttacon/chalk"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/agent"
	agentv1 "github.com/rancher/opni/pkg/agent/v1"
	agentv2 "github.com/rancher/opni/pkg/agent/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/gateway"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	pluginmeta "github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/realtime"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/pkg/webui"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
)

var Log = logger.New(logger.WithLogLevel(logger.DefaultLogLevel.Level())).Named("test")
var collectorWriteSync sync.Mutex
var agentList map[string]context.CancelFunc = make(map[string]context.CancelFunc)
var agentListMu sync.Mutex

type servicePorts struct {
	Etcd             int
	Jetstream        int
	GatewayGRPC      int
	GatewayHTTP      int
	GatewayMetrics   int
	ManagementGRPC   int
	ManagementHTTP   int
	ManagementWeb    int
	CortexGRPC       int
	CortexHTTP       int
	TestEnvironment  int
	RTMetrics        int
	DisconnectPort   int
	NodeExporterPort int
}

type RunningAgent struct {
	Agent agent.AgentInterface
	*sync.Mutex
}

type Environment struct {
	EnvironmentOptions

	TestBin           string
	Logger            *zap.SugaredLogger
	CRDDirectoryPaths []string

	mockCtrl *gomock.Controller

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once

	tempDir string
	ports   servicePorts

	runningAgents   map[string]RunningAgent
	runningAgentsMu sync.Mutex

	gatewayConfig *v1beta1.GatewayConfig
	k8sEnv        *envtest.Environment

	Processes struct {
		Etcd      future.Future[*os.Process]
		APIServer future.Future[*os.Process]
	}
}

type EnvironmentOptions struct {
	enableEtcd                bool
	enableJetstream           bool
	enableGateway             bool
	enableCortex              bool
	enableRealtimeServer      bool
	enableCortexClusterDriver bool
	delayStartEtcd            chan struct{}
	delayStartCortex          chan struct{}
	defaultAgentOpts          []StartAgentOption
	agentIdSeed               int64
	defaultAgentVersion       string
	enableDisconnectServer    bool
	enableNodeExporter        bool
}

type EnvironmentOption func(*EnvironmentOptions)

func (o *EnvironmentOptions) apply(opts ...EnvironmentOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithEnableEtcd(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableEtcd = enable
	}
}

func WithEnableJetstream(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableJetstream = enable
	}
}

func WithEnableDisconnectServer(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableDisconnectServer = enable
	}
}

func WithEnableGateway(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableGateway = enable
	}
}

func WithEnableCortex(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableCortex = enable
	}
}

func WithEnableRealtimeServer(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableRealtimeServer = enable
	}
}

func WithDefaultAgentOpts(opts ...StartAgentOption) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.defaultAgentOpts = opts
	}
}

func WithDelayStartEtcd(delay chan struct{}) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.delayStartEtcd = delay
	}
}

func WithDelayStartCortex(delay chan struct{}) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.delayStartCortex = delay
	}
}

func WithAgentIdSeed(seed int64) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.agentIdSeed = seed
	}
}

func WithEnableCortexClusterDriver(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableCortexClusterDriver = enable
	}
}

func WithEnableNodeExporter(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableNodeExporter = enable
	}
}

func defaultAgentVersion() string {
	if v, ok := os.LookupEnv("TEST_ENV_DEFAULT_AGENT_VERSION"); ok {
		return v
	}
	return "v2"
}

func (e *Environment) Start(opts ...EnvironmentOption) error {
	options := EnvironmentOptions{
		enableEtcd:             true,
		enableJetstream:        true,
		enableNodeExporter:     false,
		enableGateway:          true,
		enableCortex:           true,
		enableDisconnectServer: true,
		enableRealtimeServer:   true,
		agentIdSeed:            time.Now().UnixNano(),
		defaultAgentVersion:    defaultAgentVersion(),
	}
	options.apply(opts...)

	e.Logger = Log.Named("env")

	e.EnvironmentOptions = options
	e.Processes.Etcd = future.New[*os.Process]()

	lg := e.Logger
	lg.Info("Starting test environment")

	e.initCtx()
	e.runningAgents = make(map[string]RunningAgent)

	var t gomock.TestReporter
	if strings.HasSuffix(os.Args[0], ".test") {
		t = ginkgo.GinkgoT()
	}
	e.mockCtrl = gomock.NewController(t)

	ports, err := freeport.GetFreePorts(14)
	if err != nil {
		panic(err)
	}
	e.ports = servicePorts{
		Etcd:             ports[0],
		GatewayGRPC:      ports[1],
		GatewayHTTP:      ports[2],
		GatewayMetrics:   ports[3],
		ManagementGRPC:   ports[4],
		ManagementHTTP:   ports[5],
		ManagementWeb:    ports[6],
		CortexGRPC:       ports[7],
		CortexHTTP:       ports[8],
		TestEnvironment:  ports[9],
		RTMetrics:        ports[10],
		Jetstream:        ports[11],
		DisconnectPort:   ports[12],
		NodeExporterPort: ports[13],
	}
	if portNum, ok := os.LookupEnv("OPNI_MANAGEMENT_GRPC_PORT"); ok {
		e.ports.ManagementGRPC, err = strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("failed to parse management GRPC port: %w", err)
		}
	}
	if portNum, ok := os.LookupEnv("OPNI_MANAGEMENT_HTTP_PORT"); ok {
		e.ports.ManagementHTTP, err = strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("failed to parse management HTTP port: %w", err)
		}
	}
	if portNum, ok := os.LookupEnv("OPNI_MANAGEMENT_WEB_PORT"); ok {
		e.ports.ManagementWeb, err = strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("failed to parse management web port: %w", err)
		}
	}
	if portNum, ok := os.LookupEnv("OPNI_GATEWAY_GRPC_PORT"); ok {
		e.ports.GatewayGRPC, err = strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("failed to parse gateway grpc port: %w", err)
		}
	}
	if portNum, ok := os.LookupEnv("OPNI_GATEWAY_HTTP_PORT"); ok {
		e.ports.GatewayHTTP, err = strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("failed to parse gateway http port: %w", err)
		}
	}
	if portNum, ok := os.LookupEnv("OPNI_GATEWAY_METRICS_PORT"); ok {
		e.ports.GatewayMetrics, err = strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("failed to parse gateway metrics port: %w", err)
		}
	}
	if portNum, ok := os.LookupEnv("TEST_ENV_API_PORT"); ok {
		e.ports.TestEnvironment, err = strconv.Atoi(portNum)
		if err != nil {
			panic(err)
		}
	}
	if portNum, ok := os.LookupEnv("ETCD_PORT"); ok {
		e.ports.Etcd, err = strconv.Atoi(portNum)
		if err != nil {
			panic(err)
		}
	}
	if portNum, ok := os.LookupEnv("CORTEX_HTTP_PORT"); ok {
		e.ports.CortexHTTP, err = strconv.Atoi(portNum)
		if err != nil {
			panic(err)
		}
	}
	if portNum, ok := os.LookupEnv("CORTEX_GRPC_PORT"); ok {
		e.ports.CortexGRPC, err = strconv.Atoi(portNum)
		if err != nil {
			panic(err)
		}
	}
	if portNum, ok := os.LookupEnv("AGENT_DISCONNECT_PORT"); ok {
		e.ports.DisconnectPort, err = strconv.Atoi(portNum)
		if err != nil {
			panic(err)
		}
	}
	if portNum, ok := os.LookupEnv("NODE_EXPORTER_PORT"); ok {
		e.ports.NodeExporterPort, err = strconv.Atoi(portNum)
		if err != nil {
			panic(err)
		}
	}

	e.tempDir, err = os.MkdirTemp("", "opni-test-*")
	if err != nil {
		return err
	}
	if options.enableEtcd {
		if err := os.Mkdir(path.Join(e.tempDir, "etcd"), 0700); err != nil {
			return err
		}
	}
	if options.enableJetstream {
		if err := os.MkdirAll(path.Join(e.tempDir, "jetstream/data"), 0700); err != nil {
			return err
		}
		if err := os.MkdirAll(path.Join(e.tempDir, "jetstream/seed"), 0700); err != nil {
			return err
		}
	}
	if options.enableCortex {
		cortexTempDir := path.Join(e.tempDir, "cortex")
		if err := os.MkdirAll(path.Join(cortexTempDir, "rules"), 0700); err != nil {
			return err
		}

		entries, _ := fs.ReadDir(TestDataFS, "testdata/cortex")
		lg.Infof("Copying %d files from embedded testdata/cortex to %s", len(entries), cortexTempDir)
		for _, entry := range entries {
			if err := os.WriteFile(path.Join(cortexTempDir, entry.Name()), TestData("cortex/"+entry.Name()), 0644); err != nil {
				return err
			}
		}
	}
	if err := os.Mkdir(path.Join(e.tempDir, "prometheus"), 0700); err != nil {
		return err
	}
	os.WriteFile(path.Join(e.tempDir, "prometheus", "sample-rules.yaml"), TestData("prometheus/sample-rules.yaml"), 0644)

	if options.enableEtcd {
		if options.delayStartEtcd != nil {
			go func() {
				select {
				case <-e.ctx.Done():
					return
				case <-options.delayStartEtcd:
				}
				e.startEtcd()
			}()
		} else {
			e.startEtcd()
		}
	}
	if options.enableJetstream {
		e.startJetstream()
	}

	if options.enableDisconnectServer {
		e.StartAgentDisconnectServer()
	}

	if options.enableNodeExporter {
		e.StartNodeExporter()
	}

	if options.enableCortex {
		if options.delayStartCortex != nil && options.enableCortexClusterDriver {
			return fmt.Errorf("cannot specify both delayStartCortex and enableCortexClusterDriver")
		}
		if options.delayStartCortex != nil {
			go func() {
				select {
				case <-e.ctx.Done():
					return
				case <-options.delayStartCortex:
				}
				e.StartCortex(e.ctx)
			}()
		} else if options.enableCortexClusterDriver {
			drivers.RegisterPersistentClusterDriver(func() drivers.ClusterDriver {
				return NewTestEnvClusterDriver(e)
			})
		} else {
			e.StartCortex(e.ctx)
		}
	}
	if options.enableGateway {
		e.startGateway()
	}
	if options.enableRealtimeServer {
		e.startRealtimeServer()
	}
	return nil
}

func (e *Environment) Context() context.Context {
	return e.ctx
}

func (e *Environment) StartK8s() (*rest.Config, *runtime.Scheme, error) {
	e.initCtx()
	e.Processes.APIServer = future.New[*os.Process]()

	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	scheme := apis.NewScheme()

	for _, path := range e.CRDDirectoryPaths {
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			panic(fmt.Errorf("k8s CRDS : %v", err))
		}
	}

	e.k8sEnv = &envtest.Environment{
		BinaryAssetsDirectory: e.TestBin,
		CRDDirectoryPaths:     e.CRDDirectoryPaths,
		Scheme:                scheme,
		CRDs:                  DownloadCertManagerCRDs(scheme),
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				SecureServing: envtest.SecureServing{
					ListenAddr: envtest.ListenAddr{
						Address: "127.0.0.1",
						Port:    fmt.Sprint(port),
					},
				},
			},
		},
	}

	cfg, err := e.k8sEnv.Start()
	if err != nil {
		return nil, nil, err
	}
	pid := os.Getpid()
	threads, err := os.ReadDir(fmt.Sprintf("/proc/%d/task/", pid))
	if err != nil {
		panic(err)
	}
	possiblePIDs := []int{}
	for _, thread := range threads {
		childProcessIDs, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/children", pid, thread.Name()))
		if err != nil {
			continue
		}
		if len(childProcessIDs) > 0 {
			parts := strings.Split(string(childProcessIDs), " ")
			for _, part := range parts {
				if pid, err := strconv.Atoi(part); err == nil {
					possiblePIDs = append(possiblePIDs, pid)
				}
			}
		}
	}
	var apiserverPID int
	for _, pid := range possiblePIDs {
		exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil {
			continue
		}
		if filepath.Base(exe) == "kube-apiserver" {
			apiserverPID = pid
			break
		}
	}
	if apiserverPID == 0 {
		panic("could not find kube-apiserver PID")
	}
	proc, err := os.FindProcess(apiserverPID)
	if err != nil {
		panic(err)
	}
	e.Processes.APIServer.Set(proc)
	return cfg, scheme, nil
}

func (e *Environment) StartManager(restConfig *rest.Config, reconcilers ...Reconciler) ctrl.Manager {
	ports := util.Must(freeport.GetFreePorts(2))

	manager := util.Must(ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 e.k8sEnv.Scheme,
		MetricsBindAddress:     fmt.Sprintf(":%d", ports[0]),
		HealthProbeBindAddress: fmt.Sprintf(":%d", ports[1]),
	}))
	for _, reconciler := range reconcilers {
		util.Must(reconciler.SetupWithManager(manager))
	}
	go func() {
		if err := manager.Start(e.ctx); err != nil {
			panic(err)
		}
	}()
	return manager
}

func (e *Environment) Stop() error {
	if e.cancel != nil {
		e.cancel()
		waitctx.WaitWithTimeout(e.ctx, 40*time.Second, 20*time.Second)
	}
	if e.k8sEnv != nil {
		e.k8sEnv.Stop()
	}
	if e.mockCtrl != nil {
		e.mockCtrl.Finish()
	}
	if e.tempDir != "" {
		os.RemoveAll(e.tempDir)
	}
	return nil
}

func (e *Environment) initCtx() {
	e.once.Do(func() {
		e.ctx, e.cancel = context.WithCancel(waitctx.Background())
	})
}

func (e *Environment) startJetstream() {
	if !e.enableJetstream {
		e.Logger.Panic("jetstream disabled")
	}
	lg := e.Logger
	// set up keys
	user, err := nkeys.CreateUser()
	if err != nil {
		lg.Error(err)
	}
	seed, err := user.Seed()
	if err != nil {
		lg.Error(err)
	}
	publicKey, err := user.PublicKey()
	if err != nil {
		lg.Error(err)
	}
	t := template.Must(template.New("jetstream").Parse(`
	authorization : {
		users : [
			{ nkey : {{.PublicKey}} }
		]
	}
	`))
	var b bytes.Buffer
	t.Execute(&b, map[string]string{
		"PublicKey": publicKey,
	})
	conf := filepath.Join(e.tempDir, "jetstream", "jetstream.conf")
	err = os.WriteFile(conf, b.Bytes(), 0644)
	if err != nil {
		panic(err)
	}
	defaultArgs := []string{
		"--jetstream",
		fmt.Sprintf("--config=%s", conf),
		fmt.Sprintf("--auth=%s", seed),
		fmt.Sprintf("--store_dir=%s", path.Join(e.tempDir, "jetstream", "data")),
		fmt.Sprintf("--port=%d", e.ports.Jetstream),
	}
	jetstreamBin := path.Join(e.TestBin, "nats-server")
	cmd := exec.CommandContext(e.ctx, jetstreamBin, defaultArgs...)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		} else {
			return
		}
	}
	os.Setenv("NATS_SERVER_URL", fmt.Sprintf("http://localhost:%d", e.ports.Jetstream))
	authConfigFile := path.Join(e.tempDir, "jetstream", "seed", "nats-auth.conf")
	err = os.WriteFile(authConfigFile, []byte(seed), 0644)
	if err != nil {
		panic("failed to write jetstream auth config")
	}
	os.Setenv("NKEY_SEED_FILENAME", authConfigFile)
	lg.Info("Waiting for jetstream to start...")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
	for e.ctx.Err() == nil {
		natsURL := os.Getenv("NATS_SERVER_URL")
		natsSeedPath := os.Getenv("NKEY_SEED_FILENAME")

		opt, err := nats.NkeyOptionFromSeed(natsSeedPath)
		if err != nil {
			panic(err)

		}
		retryBackoff := backoff.NewExponentialBackOff()
		time.Sleep(time.Second)
		if nc, err := nats.Connect(
			natsURL,
			opt,
			nats.MaxReconnects(-1),
			nats.CustomReconnectDelay(
				func(i int) time.Duration {
					if i == 1 {
						retryBackoff.Reset()
					}
					return retryBackoff.NextBackOff()
				},
			),
			nats.DisconnectErrHandler(
				func(nc *nats.Conn, err error) {
					lg.With(
						"err", err,
					).Warn("nats disconnected")
				},
			),
		); err == nil {
			defer nc.Close()
			break
		} else {
			lg.Error(err)
		}
		time.Sleep(time.Second)
	}
	lg.Info("Jetstream started")
}

func (e *Environment) startEtcd() {
	if !e.enableEtcd {
		e.Logger.Panic("etcd disabled")
	}
	lg := e.Logger
	defaultArgs := []string{
		fmt.Sprintf("--listen-client-urls=http://localhost:%d", e.ports.Etcd),
		fmt.Sprintf("--advertise-client-urls=http://localhost:%d", e.ports.Etcd),
		"--listen-peer-urls=http://localhost:0",
		"--log-level=error",
		fmt.Sprintf("--data-dir=%s", path.Join(e.tempDir, "etcd")),
	}
	etcdBin := path.Join(e.TestBin, "etcd")
	cmd := exec.CommandContext(e.ctx, etcdBin, defaultArgs...)
	cmd.Env = []string{"ALLOW_NONE_AUTHENTICATION=yes"}
	plugins.ConfigureSysProcAttr(cmd)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		} else {
			return
		}
	}
	e.Processes.Etcd.Set(cmd.Process)

	lg.Info("Waiting for etcd to start...")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
	for e.ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", e.ports.Etcd))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	lg.Info("Etcd started")
}

type cortexTemplateOptions struct {
	HttpListenPort int
	GrpcListenPort int
	StorageDir     string
	EtcdPort       int
}

func (e *Environment) StartCortex(ctx context.Context) {
	if !e.enableCortex {
		e.Logger.Panic("cortex disabled")
	}
	lg := e.Logger
	configTemplate := TestData("cortex/config.yaml")
	t := util.Must(template.New("config").Parse(string(configTemplate)))
	configFile, err := os.Create(path.Join(e.tempDir, "cortex", "config.yaml"))
	if err != nil {
		panic(err)
	}
	if err := t.Execute(configFile, cortexTemplateOptions{
		HttpListenPort: e.ports.CortexHTTP,
		GrpcListenPort: e.ports.CortexGRPC,
		StorageDir:     path.Join(e.tempDir, "cortex"),
		EtcdPort:       e.ports.Etcd,
	}); err != nil {
		panic(err)
	}
	configFile.Close()
	cortexBin := filepath.Join(e.TestBin, "../../bin/opni")
	defaultArgs := []string{
		"cortex", fmt.Sprintf("-config.file=%s", path.Join(e.tempDir, "cortex/config.yaml")),
	}
	cmd := exec.CommandContext(ctx, cortexBin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			panic(err)
		}
	}
	lg.Info("Waiting for cortex to start...")
	for ctx.Err() == nil {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/ready", e.ports.CortexHTTP), nil)
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: e.CortexTLSConfig(),
			},
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			lg.With(
				zap.Error(err),
				"status", resp.Status,
			).Info("Waiting for cortex to start...")
		}
		time.Sleep(time.Second)
	}
	lg.With(
		"httpAddress", fmt.Sprintf("https://localhost:%d", e.ports.CortexHTTP),
		"grpcAddress", fmt.Sprintf("localhost:%d", e.ports.CortexGRPC),
	).Info("Cortex started")
	waitctx.Go(ctx, func() {
		<-ctx.Done()
		lg.Info("Cortex stopping...")
		session.Wait()
		lg.Info("Cortex stopped")
	})
}

func (e *Environment) startRealtimeServer() {
	if !e.enableRealtimeServer {
		e.Logger.Panic("realtime disabled")
	}

	srv, err := realtime.NewServer(&v1beta1.RealtimeServerSpec{
		MetricsListenAddress: fmt.Sprintf("localhost:%d", e.ports.RTMetrics),
		ManagementClient: v1beta1.ManagementClientSpec{
			Address: fmt.Sprintf("localhost:%d", e.ports.ManagementGRPC),
		},
	})
	if err != nil {
		panic(err)
	}
	go srv.Start(e.ctx)
}

type PrometheusJob struct {
	JobName    string
	ScrapePort int
}

type prometheusTemplateOptions struct {
	ListenPort    int
	RTMetricsPort int
	OpniAgentPort int
	// these fill in a text/template defined as {{.range Jobs}} /* */ {{end}}
	Jobs []PrometheusJob
}

type overridePrometheusConfig struct {
	configContents string
	jobs           []PrometheusJob
}

func NewOverridePrometheusConfig(configPath string, jobs []PrometheusJob) *overridePrometheusConfig {
	return &overridePrometheusConfig{
		configContents: string(TestData(configPath)),
		jobs:           jobs,
	}
}

// `prometheus/config.yaml` is the default monitoring config.
// `slo/prometheus/config.yaml` is the default SLO config.
func (e *Environment) StartPrometheus(opniAgentPort int, override ...*overridePrometheusConfig) int {
	lg := e.Logger
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	var configTemplate string
	var jobs []PrometheusJob

	configTemplate = string(TestData("prometheus/config.yaml"))
	if len(override) > 1 {
		panic("Too many overrides, only one is allowed")
	}
	if len(override) == 1 {
		configTemplate = override[0].configContents
		jobs = override[0].jobs
	}
	t, err := template.New("").Parse(configTemplate)
	if err != nil {
		panic(err)
	}
	configFile, err := os.Create(path.Join(e.tempDir, "prometheus", "config.yaml"))
	if err != nil {
		panic(err)
	}

	if err := t.Execute(configFile, prometheusTemplateOptions{
		ListenPort:    port,
		OpniAgentPort: opniAgentPort,
		RTMetricsPort: e.ports.RTMetrics,
		Jobs:          jobs,
	}); err != nil {
		panic(err)
	}

	configFile.Close()
	prometheusBin := path.Join(e.TestBin, "prometheus")
	defaultArgs := []string{
		fmt.Sprintf("--config.file=%s", path.Join(e.tempDir, "prometheus/config.yaml")),
		fmt.Sprintf("--storage.agent.path=%s", path.Join(e.tempDir, "prometheus", fmt.Sprint(opniAgentPort))),
		fmt.Sprintf("--web.listen-address=127.0.0.1:%d", port),
		"--log.level=error",
		"--web.enable-lifecycle",
		"--enable-feature=agent",
	}
	cmd := exec.CommandContext(e.ctx, prometheusBin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		}
	}
	lg.Info("Waiting for prometheus to start...")
	for e.ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", port))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	lg.With("address", fmt.Sprintf("http://localhost:%d", port)).Info("Prometheus started")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
	return port
}

// Starts a server that exposes Prometheus metrics
//
// Returns port number of the server & a channel that shutdowns the server
func (e *Environment) StartInstrumentationServer(ctx context.Context) (int, chan struct{}) {
	// lg := e.Logger
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	reg := prometheus.NewRegistry()

	for queryName, queryObj := range query.AvailableQueries {
		// register each prometheus collector
		reg.MustRegister(queryObj.GetCollector())

		// create an endpoint simulating good events
		mux.HandleFunc(fmt.Sprintf("/%s/%s", queryName, "good"), queryObj.GetGoodEventGenerator())
		// create an endpoint simulating bad events
		mux.HandleFunc(fmt.Sprintf("/%s/%s", queryName, "bad"), queryObj.GetBadEventGenerator())

	}
	// expose prometheus metrics

	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry: reg,
	}))

	autoInstrumentationServer := &http.Server{
		Addr:           fmt.Sprintf("127.0.0.1:%d", port),
		Handler:        mux,
		ReadTimeout:    1 * time.Second,
		WriteTimeout:   1 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	done := make(chan struct{})
	waitctx.Restrictive.Go(e.ctx, func() {
		go func() {
			err := autoInstrumentationServer.ListenAndServe()
			if !errors.Is(err, http.ErrServerClosed) {
				panic(err)
			}
		}()
		defer autoInstrumentationServer.Shutdown(context.Background())
		select {
		case <-e.ctx.Done():
		case <-done:
		}
	})
	return port, done
}

func (e *Environment) StartMockKubernetesMetricServer(ctx context.Context) (port int) {
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	reg := prometheus.NewRegistry()

	registeredCollectors := map[string]*prometheus.GaugeVec{}

	kubeMetricsIsDefined := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metrics.KubeMetricsIsDefinedMetricName,
	}, []string{"namespace"})

	kubeMetricsIsDefined.WithLabelValues("kube-system").Set(1)

	setObjHandler := func(w http.ResponseWriter, r *http.Request) {
		collectorWriteSync.Lock()
		defer collectorWriteSync.Unlock()
		objType := r.URL.Query().Get("obj")
		name := r.URL.Query().Get("name")
		namespace := r.URL.Query().Get("namespace")
		phase := r.URL.Query().Get("phase")
		uid := r.URL.Query().Get("uid")

		accessedState := false
		var b bytes.Buffer
		err := metrics.KubeObjMetricCreator.Execute(&b, map[string]string{
			"ObjType": objType,
		})
		if err != nil {
			panic(err)
		}
		var newOrExistingKubeObjStateCollector *prometheus.GaugeVec
		if _, ok := registeredCollectors[b.String()]; !ok {
			newOrExistingKubeObjStateCollector = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: b.String(),
			}, []string{objType, "namespace", "phase", "uid"})
			registeredCollectors[b.String()] = newOrExistingKubeObjStateCollector
			reg.MustRegister(newOrExistingKubeObjStateCollector)
		} else {
			newOrExistingKubeObjStateCollector = registeredCollectors[b.String()]
		}

		for _, validPhase := range metrics.KubeStates {
			if phase == validPhase {
				accessedState = true
				newOrExistingKubeObjStateCollector.WithLabelValues(name, namespace, validPhase, uid).Set(1)
			} else {
				newOrExistingKubeObjStateCollector.WithLabelValues(name, namespace, validPhase, uid).Set(0)
			}
		}
		if accessedState == false {
			panic(fmt.Sprintf("Set state for kube metrics api must be one of %s", strings.Join(metrics.KubeStates, ",")))
		}
	}

	reg.MustRegister(kubeMetricsIsDefined)

	//mux.HandleFunc("/setKubePodState", setPhaseHandler)
	mux.HandleFunc("/set", setObjHandler)
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry: reg,
	}))

	autoKubernetesMetricsServer := &http.Server{
		Addr:           fmt.Sprintf("127.0.0.1:%d", port),
		Handler:        mux,
		ReadTimeout:    1 * time.Second,
		WriteTimeout:   1 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	waitctx.Permissive.Go(e.ctx, func() {
		go func() {
			err := autoKubernetesMetricsServer.ListenAndServe()
			if err != http.ErrServerClosed {
				panic(err)
			}
		}()
		defer autoKubernetesMetricsServer.Shutdown(context.Background())
		select {
		case <-e.ctx.Done():
		}
	})

	return port
}

func (e *Environment) StartAgentDisconnectServer() {
	setDisconnect := func(w http.ResponseWriter, r *http.Request) {
		agentListMu.Lock()
		defer agentListMu.Unlock()
		agent := r.URL.Query().Get("agent")
		if agent == "" {
			e.Logger.Error("agent not specified")
			return
		}
		if _, ok := agentList[agent]; !ok {
			e.Logger.Error("could not find agent to disconnect")
			return
		}
		agentList[agent]()
		delete(agentList, agent)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/disconnect", setDisconnect)

	disconnectServer := &http.Server{
		Addr:           fmt.Sprintf("127.0.0.1:%d", e.ports.DisconnectPort),
		Handler:        mux,
		ReadTimeout:    1 * time.Second,
		WriteTimeout:   1 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	waitctx.Permissive.Go(e.ctx, func() {
		go func() {
			err := disconnectServer.ListenAndServe()
			if err != http.ErrServerClosed {
				panic(err)
			}
		}()
		defer disconnectServer.Shutdown(context.Background())
		select {
		case <-e.ctx.Done():
		}
	})
	Log.Infof(chalk.Green.Color("Agent Disconnect server listening on %d"), e.ports.DisconnectPort)
}

func (e *Environment) StartNodeExporter() {
	nodeExporterBin := path.Join(e.TestBin, "node_exporter")
	defaultArgs := []string{
		fmt.Sprintf("--web.listen-address=127.0.0.1:%d", e.ports.NodeExporterPort),
		"--log.level=error",
	}
	cmd := exec.CommandContext(e.ctx, nodeExporterBin, defaultArgs...)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		}
	}
	e.Logger.Info("Waiting for node_exporter to start...")
	for e.ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", e.ports.NodeExporterPort))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	e.Logger.With("address", fmt.Sprintf("http://localhost:%d", e.ports.NodeExporterPort)).Info("Node exporter started")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
}

func (e *Environment) simulateKubeObject(kPort int) {
	// sample a random phase
	namespaces := []string{"kube-system", "default", "opni"}
	namespace := namespaces[rand.Intn(len(namespaces))]
	sampleObjects := []string{"pod", "deployment", "statefulset", "daemonset", "job", "cronjob", "service", "ingress"}
	sampleObject := sampleObjects[rand.Intn(len(sampleObjects))]
	sampleState := metrics.KubeStates[rand.Intn(len(metrics.KubeStates))]

	queryUrl := fmt.Sprintf("http://localhost:%d/set", kPort)
	client := &http.Client{
		Transport: &http.Transport{},
	}
	req, err := http.NewRequest("GET", queryUrl, nil)
	if err != nil {
		panic(err)
	}
	values := url.Values{}
	values.Set("obj", sampleObject)
	values.Set("name", RandomName(time.Now().UnixNano()))
	values.Set("namespace", namespace)
	values.Set("phase", sampleState)
	values.Set("uid", uuid.New().String())
	req.URL.RawQuery = values.Encode()
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			e.Logger.Error("got error from mock kube metrics api : ", zap.Error(err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			e.Logger.Error("got response code %d from mock kube metrics api", resp.StatusCode)
		}
	}()
}

func (e *Environment) newGatewayConfig() *v1beta1.GatewayConfig {
	caCertData := TestData("root_ca.crt")
	servingCertData := TestData("localhost.crt")
	servingKeyData := TestData("localhost.key")
	return &v1beta1.GatewayConfig{
		TypeMeta: meta.TypeMeta{
			APIVersion: "v1beta1",
			Kind:       "GatewayConfig",
		},
		Spec: v1beta1.GatewayConfigSpec{
			HTTPListenAddress:    fmt.Sprintf("localhost:%d", e.ports.GatewayHTTP),
			GRPCListenAddress:    fmt.Sprintf("localhost:%d", e.ports.GatewayGRPC),
			MetricsListenAddress: fmt.Sprintf("localhost:%d", e.ports.GatewayMetrics),
			Management: v1beta1.ManagementSpec{
				GRPCListenAddress: fmt.Sprintf("tcp://127.0.0.1:%d", e.ports.ManagementGRPC),
				HTTPListenAddress: fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementHTTP),
				WebListenAddress:  fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementWeb),
			},
			AuthProvider: "test",
			Certs: v1beta1.CertsSpec{
				CACertData:      caCertData,
				ServingCertData: servingCertData,
				ServingKeyData:  servingKeyData,
			},
			Cortex: v1beta1.CortexSpec{
				Management: v1beta1.ClusterManagementSpec{
					ClusterDriver: lo.Ternary(e.enableCortexClusterDriver, "test-environment", ""),
				},
				Distributor: v1beta1.DistributorSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
					GRPCAddress: fmt.Sprintf("localhost:%d", e.ports.CortexGRPC),
				},
				Ingester: v1beta1.IngesterSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
					GRPCAddress: fmt.Sprintf("localhost:%d", e.ports.CortexGRPC),
				},
				Compactor: v1beta1.CompactorSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				StoreGateway: v1beta1.StoreGatewaySpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
					GRPCAddress: fmt.Sprintf("localhost:%d", e.ports.CortexGRPC),
				},
				Alertmanager: v1beta1.AlertmanagerSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				Ruler: v1beta1.RulerSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				QueryFrontend: v1beta1.QueryFrontendSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
					GRPCAddress: fmt.Sprintf("localhost:%d", e.ports.CortexGRPC),
				},
				Purger: v1beta1.PurgerSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				Certs: v1beta1.MTLSSpec{
					ServerCA:   path.Join(e.tempDir, "cortex/root.crt"),
					ClientCA:   path.Join(e.tempDir, "cortex/root.crt"),
					ClientCert: path.Join(e.tempDir, "cortex/client.crt"),
					ClientKey:  path.Join(e.tempDir, "cortex/client.key"),
				},
			},
			Storage: v1beta1.StorageSpec{
				Type: v1beta1.StorageTypeEtcd,
				Etcd: &v1beta1.EtcdStorageSpec{
					Endpoints: []string{fmt.Sprintf("http://localhost:%d", e.ports.Etcd)},
				},
			},
			Alerting: v1beta1.AlertingSpec{
				//Endpoints:                 []string{"opni-alerting:9093"},
				ConfigMap: "alertmanager-config",
				Namespace: "default",
				//StatefulSetName:           "opni-alerting-internal",
				WorkerNodeService:     "opni-alerting",
				WorkerStatefulSet:     "opni-alerting-internal",
				WorkerPort:            9093,
				ControllerNodeService: "opni-alerting-controller",
				ControllerStatefulSet: "opni-alerting-controller-internal",
				ControllerNodePort:    9093,
				ControllerClusterPort: 9094,
				ManagementHookHandler: shared.AlertingCortexHookHandler,
			},
		},
	}
}

func (e *Environment) NewManagementClient() managementv1.ManagementClient {
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}
	c, err := clients.NewManagementClient(e.ctx,
		clients.WithAddress(fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementGRPC)),
		clients.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
	)
	if err != nil {
		panic(err)
	}
	return c
}
func (e *Environment) ManagementClientConn() grpc.ClientConnInterface {
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}
	cc, err := grpc.DialContext(e.ctx, fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementGRPC),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}
	return cc
}

func (e *Environment) NewCortexAdminClient() cortexadmin.CortexAdminClient {
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}
	c, err := cortexadmin.NewClient(e.ctx,
		cortexadmin.WithListenAddress(fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementGRPC)),
		cortexadmin.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
	)
	if err != nil {
		panic(err)
	}
	return c
}

func (e *Environment) NewCortexOpsClient() cortexops.CortexOpsClient {
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}
	c, err := cortexops.NewClient(e.ctx,
		cortexops.WithListenAddress(fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementGRPC)),
		cortexops.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
	)
	if err != nil {
		panic(err)
	}
	return c
}

func (e *Environment) PrometheusAPIEndpoint() string {
	return fmt.Sprintf("https://localhost:%d/prometheus/api/v1", e.ports.GatewayHTTP)
}

func (e *Environment) startGateway() {
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}
	lg := e.Logger
	e.gatewayConfig = e.newGatewayConfig()
	pluginLoader := plugins.NewPluginLoader()

	lifecycler := config.NewLifecycler(meta.ObjectList{e.gatewayConfig, &v1beta1.AuthProvider{
		TypeMeta: meta.TypeMeta{
			APIVersion: "v1beta1",
			Kind:       "AuthProvider",
		},
		ObjectMeta: meta.ObjectMeta{
			Name: "test",
		},
		Spec: v1beta1.AuthProviderSpec{
			Type: "test",
		},
	}})
	g := gateway.NewGateway(e.ctx, e.gatewayConfig, pluginLoader,
		gateway.WithLifecycler(lifecycler),
	)
	m := management.NewServer(e.ctx, &e.gatewayConfig.Spec.Management, g, pluginLoader,
		management.WithCapabilitiesDataSource(g),
		management.WithHealthStatusDataSource(g),
		management.WithLifecycler(lifecycler),
	)

	pluginLoader.Hook(hooks.OnLoadingCompleted(func(numLoaded int) {
		lg.Infof("loaded %d plugins", numLoaded)
	}))

	pluginLoader.Hook(hooks.OnLoadingCompleted(func(int) {
		waitctx.AddOne(e.ctx)
		defer waitctx.Done(e.ctx)
		if err := m.ListenAndServe(e.ctx); err != nil {
			lg.With(
				zap.Error(err),
			).Warn("management server exited with error")
		}
	}))

	pluginLoader.Hook(hooks.OnLoadingCompleted(func(int) {
		waitctx.AddOne(e.ctx)
		defer waitctx.Done(e.ctx)
		if err := g.ListenAndServe(e.ctx); err != nil {
			lg.With(
				zap.Error(err),
			).Warn("gateway server exited with error")
		}
	}))

	LoadPlugins(pluginLoader, pluginmeta.ModeGateway)

	lg.Info("Waiting for gateway to start...")
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/healthz",
			e.gatewayConfig.Spec.HTTPListenAddress), nil)
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: e.GatewayTLSConfig(),
			},
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
	}
	lg.Info("Gateway started")
}

type StartAgentOptions struct {
	ctx                  context.Context
	remoteGatewayAddress string
	remoteKubeconfig     string
	version              string
}

type StartAgentOption func(*StartAgentOptions)

func (o *StartAgentOptions) apply(opts ...StartAgentOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithContext(ctx context.Context) StartAgentOption {
	return func(o *StartAgentOptions) {
		o.ctx = ctx
	}
}

func WithRemoteGatewayAddress(addr string) StartAgentOption {
	return func(o *StartAgentOptions) {
		o.remoteGatewayAddress = addr
	}
}

func WithRemoteKubeconfig(kubeconfig string) StartAgentOption {
	return func(o *StartAgentOptions) {
		o.remoteKubeconfig = kubeconfig
	}
}

func WithAgentVersion(version string) StartAgentOption {
	return func(o *StartAgentOptions) {
		o.version = version
	}
}

func (e *Environment) StartAgent(id string, token *corev1.BootstrapToken, pins []string, opts ...StartAgentOption) (int, <-chan error) {
	options := &StartAgentOptions{
		version: e.defaultAgentVersion,
		ctx:     e.ctx,
	}
	options.apply(e.defaultAgentOpts...)
	options.apply(opts...)
	if !e.enableGateway && options.remoteGatewayAddress == "" {
		e.Logger.Panic("gateway disabled")
	}

	errC := make(chan error, 2)
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}

	if err := ident.RegisterProvider(id, func() ident.Provider {
		return NewTestIdentProvider(e.mockCtrl, id)
	}); err != nil {
		if !errors.Is(err, ident.ErrProviderAlreadyExists) {
			panic(err)
		}
	}

	gatewayAddress := fmt.Sprintf("localhost:%d", e.ports.GatewayGRPC)
	if options.remoteGatewayAddress != "" {
		gatewayAddress = options.remoteGatewayAddress
	}

	agentConfig := &v1beta1.AgentConfig{
		Spec: v1beta1.AgentConfigSpec{
			TrustStrategy:    v1beta1.TrustStrategyPKP,
			ListenAddress:    fmt.Sprintf("localhost:%d", port),
			GatewayAddress:   gatewayAddress,
			IdentityProvider: id,
			Rules: &v1beta1.RulesSpec{
				Discovery: &v1beta1.DiscoverySpec{
					Filesystem: &v1beta1.FilesystemRulesSpec{
						PathExpressions: []string{
							path.Join(e.tempDir, "prometheus", "sample-rules.yaml"),
						},
					},
				},
			},
			Storage: v1beta1.StorageSpec{
				Type: v1beta1.StorageTypeEtcd,
				Etcd: &v1beta1.EtcdStorageSpec{
					Endpoints: []string{fmt.Sprintf("http://localhost:%d", e.ports.Etcd)},
				},
			},
		},
	}

	Log.With(
		"id", id,
		"address", agentConfig.Spec.ListenAddress,
		"version", options.version,
	).Info("starting agent")

	publicKeyPins := []*pkp.PublicKeyPin{}
	for _, pin := range pins {
		d, err := pkp.DecodePin(pin)
		if err != nil {
			errC <- err
			return 0, errC
		}
		publicKeyPins = append(publicKeyPins, d)
	}
	bt, err := tokens.FromBootstrapToken(token)
	if err != nil {
		errC <- err
		return 0, errC
	}
	var a agent.AgentInterface
	mu := &sync.Mutex{}
	waitctx.Permissive.Go(options.ctx, func() {
		mu.Lock()
		publicKeyPins := make([]*pkp.PublicKeyPin, len(pins))
		for i, pin := range pins {
			d, err := pkp.DecodePin(pin)
			if err != nil {
				errC <- err
				return
			}
			publicKeyPins[i] = d
		}
		conf := trust.StrategyConfig{
			PKP: &trust.PKPConfig{
				Pins: trust.NewPinSource(publicKeyPins),
			},
		}
		strategy, err := conf.Build()
		if err != nil {
			errC <- err
			return
		}
		switch options.version {
		case "v1":
			a, err = agentv1.New(options.ctx, agentConfig,
				agentv1.WithBootstrapper(&bootstrap.ClientConfig{
					Capability:    wellknown.CapabilityMetrics,
					Token:         bt,
					Endpoint:      gatewayAddress,
					TrustStrategy: strategy,
				}))
		case "v2":
			ctx, cancel := context.WithCancel(options.ctx)
			pl := plugins.NewPluginLoader()
			a, err = agentv2.New(ctx, agentConfig,
				agentv2.WithBootstrapper(&bootstrap.ClientConfigV2{
					Token:         bt,
					Endpoint:      gatewayAddress,
					TrustStrategy: strategy,
				}),
				agentv2.WithUnmanagedPluginLoader(pl),
			)
			LoadPlugins(pl, pluginmeta.ModeAgent)
			agentListMu.Lock()
			agentList[id] = cancel
			agentListMu.Unlock()
		default:
			errC <- fmt.Errorf("unknown agent version %q (expected \"v1\" or \"v2\")", options.version)
			return
		}
		if err != nil {
			Log.With(zap.Error(err)).Error("failed to start agent")
			errC <- err
			mu.Unlock()
			return
		}
		e.runningAgentsMu.Lock()
		e.runningAgents[id] = RunningAgent{
			Agent: a,
			Mutex: mu,
		}
		e.runningAgentsMu.Unlock()
		mu.Unlock()
		errC <- nil
		if err := a.ListenAndServe(options.ctx); err != nil {
			Log.Errorf("agent %q exited: %v", id, err)
		}
		e.runningAgentsMu.Lock()
		delete(e.runningAgents, id)
		e.runningAgentsMu.Unlock()
	})
	return port, errC
}

func (e *Environment) GetAgent(id string) RunningAgent {
	e.runningAgentsMu.Lock()
	defer e.runningAgentsMu.Unlock()
	return e.runningAgents[id]
}

func (e *Environment) GatewayTLSConfig() *tls.Config {
	pool := x509.NewCertPool()
	switch {
	case e.gatewayConfig.Spec.Certs.CACert != nil:
		data, err := os.ReadFile(*e.gatewayConfig.Spec.Certs.CACert)
		if err != nil {
			e.Logger.Panic(err)
		}
		if !pool.AppendCertsFromPEM(data) {
			e.Logger.Panic("failed to load gateway CA cert")
		}
	case e.gatewayConfig.Spec.Certs.CACertData != nil:
		if !pool.AppendCertsFromPEM(e.gatewayConfig.Spec.Certs.CACertData) {
			e.Logger.Panic("failed to load gateway CA cert")
		}
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}
}

func (e *Environment) CortexTLSConfig() *tls.Config {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(TestData("cortex/root.crt")) {
		e.Logger.Panic("failed to load Cortex CA cert")
	}
	clientCert := TestData("cortex/client.crt")
	clientKey := TestData("cortex/client.key")
	cert, err := tls.X509KeyPair(clientCert, clientKey)
	if err != nil {
		e.Logger.Panic(err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
	}
}

func (e *Environment) GatewayConfig() *v1beta1.GatewayConfig {
	return e.gatewayConfig
}

func (e *Environment) GetAlertingManagementWebhookEndpoint() string {
	return "https://" +
		e.GatewayConfig().Spec.HTTPListenAddress +
		e.GatewayConfig().Spec.Alerting.ManagementHookHandler
}

func (e *Environment) EtcdClient() (*clientv3.Client, error) {
	if !e.enableEtcd {
		e.Logger.Panic("etcd disabled")
	}
	return clientv3.New(clientv3.Config{
		Endpoints: []string{fmt.Sprintf("http://localhost:%d", e.ports.Etcd)},
		Context:   e.ctx,
		Logger:    e.Logger.Desugar(),
	})
}

func (e *Environment) EtcdConfig() *v1beta1.EtcdStorageSpec {
	if !e.enableEtcd {
		e.Logger.Panic("etcd disabled")
	}
	return &v1beta1.EtcdStorageSpec{
		Endpoints: []string{fmt.Sprintf("http://localhost:%d", e.ports.Etcd)},
	}
}

func StartStandaloneTestEnvironment(opts ...EnvironmentOption) {
	options := &EnvironmentOptions{
		enableGateway:    true,
		enableEtcd:       true,
		enableCortex:     true,
		defaultAgentOpts: []StartAgentOption{},
	}
	err := os.Setenv(shared.LocalBackendEnvToggle, "true")
	if err != nil {
		panic(err)
	}
	options.apply(opts...)

	agentOptions := &StartAgentOptions{}
	agentOptions.apply(options.defaultAgentOpts...)

	environment := &Environment{
		TestBin: "testbin/bin",
	}
	randSrc := rand.New(rand.NewSource(0))
	var iPort int
	var kPort int
	addAgent := func(rw http.ResponseWriter, r *http.Request) {
		Log.Infof("%s %s", r.Method, r.URL.Path)
		switch r.Method {
		case http.MethodPost:
			body := struct {
				Token   string   `json:"token"`
				Pins    []string `json:"pins"`
				ID      string   `json:"id"`
				Version string   `json:"version"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				rw.Write([]byte(err.Error()))
				return
			}
			if body.ID == "" {
				body.ID = util.Must(uuid.NewRandomFromReader(randSrc)).String()
			}
			if body.Version == "" {
				body.Version = "v1"
			}
			token, err := tokens.ParseHex(body.Token)
			if err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				rw.Write([]byte(err.Error()))
				return
			}
			startOpts := slices.Clone(options.defaultAgentOpts)
			port, errC := environment.StartAgent(body.ID, token.ToBootstrapToken(), body.Pins,
				append(startOpts, WithAgentVersion(body.Version))...)
			if err := <-errC; err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				rw.Write([]byte(err.Error()))
				return
			}
			environment.StartPrometheus(port, NewOverridePrometheusConfig(
				"alerting/prometheus/config.yaml",
				[]PrometheusJob{
					{
						JobName:    query.MockTestServerName,
						ScrapePort: iPort,
					},
					{
						JobName:    "kubernetes",
						ScrapePort: kPort,
					},
				}),
			)

			rw.WriteHeader(http.StatusOK)
			rw.Write([]byte(fmt.Sprintf("%d", port)))
		}
	}
	webui.AddExtraHandler("/opni-test/agents", addAgent)
	http.HandleFunc("/agents", addAgent)
	if err := environment.Start(opts...); err != nil {
		panic(err)
	}
	go func() {
		addr := fmt.Sprintf("127.0.0.1:%d", environment.ports.TestEnvironment)
		Log.Infof(chalk.Green.Color("Test environment API listening on %s"), addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			panic(err)
		}
	}()
	c := make(chan os.Signal, 2)
	var closeOnce sync.Once
	signal.Notify(c, os.Interrupt)
	iPort, _ = environment.StartInstrumentationServer(context.Background())
	kPort = environment.StartMockKubernetesMetricServer(context.Background())
	for i := 0; i < 100; i++ {
		environment.simulateKubeObject(kPort)
	}
	Log.Infof(chalk.Green.Color("Instrumentation server listening on %d"), iPort)
	Log.Info(chalk.Blue.Color("Press (ctrl+c) or (q) to stop test environment"))
	var client managementv1.ManagementClient
	if options.enableGateway {
		client = environment.NewManagementClient()
	} else if agentOptions.remoteKubeconfig != "" {
		ports, err := testutil.PortForward(environment.ctx, types.NamespacedName{
			Namespace: "opni",
			Name:      "opni-internal",
		}, []string{"11090"}, util.Must(k8sutil.NewRestConfig(k8sutil.ClientOptions{
			Kubeconfig: &agentOptions.remoteKubeconfig,
		})), apis.NewScheme())
		if err != nil || len(ports) != 1 {
			Log.Panic(err)
		}
		client, err = clients.NewManagementClient(environment.ctx,
			clients.WithAddress(fmt.Sprintf("127.0.0.1:%d", ports[0].Local)),
		)
	}

	handleKey := func(rn rune) {
		switch rn {
		case ' ':
			go browser.OpenURL(fmt.Sprintf("http://localhost:%d", environment.ports.ManagementWeb))
		case 'q':
			closeOnce.Do(func() {
				signal.Stop(c)
				close(c)
			})
		case 'a', 'A':
			go func() {
				bt, err := client.CreateBootstrapToken(environment.ctx, &managementv1.CreateBootstrapTokenRequest{
					Ttl: durationpb.New(1 * time.Minute),
				})
				if err != nil {
					Log.Error(err)
					return
				}
				token, err := tokens.FromBootstrapToken(bt)
				if err != nil {
					Log.Error(err)
					return
				}
				certInfo, err := client.CertsInfo(environment.ctx, &emptypb.Empty{})
				if err != nil {
					Log.Error(err)
					return
				}
				var version string
				switch rn {
				case 'a':
					version = "v1"
				case 'A':
					version = "v2"
				}
				resp, err := http.Post(fmt.Sprintf("http://localhost:%d/agents", environment.ports.TestEnvironment), "application/json",
					strings.NewReader(fmt.Sprintf(`{"token": "%s", "pins": ["%s"], "version": "%s"}`,
						token.EncodeHex(), certInfo.Chain[len(certInfo.Chain)-1].Fingerprint, version)))
				if err != nil {
					Log.Error(err)
					return
				}
				if resp.StatusCode != http.StatusOK {
					Log.Errorf("%s", resp.Status)
					return
				}
			}()
		}
	}

	Log.Infof(chalk.Green.Color("Kubernetes metric server listening on %d"), kPort)
	Log.Info(chalk.Blue.Color("Press (ctrl+c) to stop test environment"))
	// listen for spacebar on stdin
	t, err := tty.Open()
	if err == nil {
		if options.enableGateway {
			Log.Info(chalk.Blue.Color("Press (space) to open the web dashboard"))
		}
		if options.enableGateway || agentOptions.remoteKubeconfig != "" {
			Log.Info(chalk.Blue.Color("Press (a) to launch a new v1 agent"))
			Log.Info(chalk.Blue.Color("Press (A) to launch a new v2 agent"))
		}
		go func() {
			for {
				rn, err := t.ReadRune()
				if err != nil {
					Log.Panic(err)
				}
				handleKey(rn)
			}
		}()
	}

	if sim, ok := os.LookupEnv("TEST_ENV_SIM_KEYS"); ok {
		// syntax: <key>[;<key>][;sleep:<duration>]...
		go func() {
			for _, cmd := range strings.Split(sim, ";") {
				if strings.HasPrefix(cmd, "sleep:") {
					d, err := time.ParseDuration(strings.TrimPrefix(cmd, "sleep:"))
					if err != nil {
						Log.Panic(err)
					}
					time.Sleep(d)
				} else {
					handleKey(rune(cmd[0]))
				}
			}
		}()
	}

	<-c
	fmt.Println(chalk.Yellow.Color("\nStopping test environment"))
	if err := environment.Stop(); err != nil {
		panic(err)
	}
}
