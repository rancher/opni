package test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
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

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/mattn/go-tty"
	"github.com/onsi/ginkgo/v2"
	"github.com/phayes/freeport"
	"github.com/pkg/browser"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/agent"
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
	"github.com/rancher/opni/pkg/realtime"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/pkg/webui"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/ttacon/chalk"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var Log = logger.New(logger.WithLogLevel(logger.DefaultLogLevel.Level())).Named("test")

type servicePorts struct {
	Etcd            int
	GatewayGRPC     int
	GatewayHTTP     int
	ManagementGRPC  int
	ManagementHTTP  int
	ManagementWeb   int
	CortexGRPC      int
	CortexHTTP      int
	TestEnvironment int
	RTMetrics       int
}

type RunningAgent struct {
	*agent.Agent
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
	enableEtcd           bool
	enableGateway        bool
	enableCortex         bool
	enableRealtimeServer bool
	defaultAgentOpts     []StartAgentOption
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

func (e *Environment) Start(opts ...EnvironmentOption) error {
	options := EnvironmentOptions{
		enableEtcd:           true,
		enableGateway:        true,
		enableCortex:         true,
		enableRealtimeServer: true,
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

	ports, err := freeport.GetFreePorts(10)
	if err != nil {
		panic(err)
	}
	e.ports = servicePorts{
		Etcd:            ports[0],
		GatewayGRPC:     ports[1],
		GatewayHTTP:     ports[2],
		ManagementGRPC:  ports[3],
		ManagementHTTP:  ports[4],
		ManagementWeb:   ports[5],
		CortexGRPC:      ports[6],
		CortexHTTP:      ports[7],
		TestEnvironment: ports[8],
		RTMetrics:       ports[9],
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
			return fmt.Errorf("failed to parse gateway port: %w", err)
		}
	}
	if portNum, ok := os.LookupEnv("OPNI_GATEWAY_HTTP_PORT"); ok {
		e.ports.GatewayHTTP, err = strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("failed to parse gateway port: %w", err)
		}
	}
	if portNum, ok := os.LookupEnv("TEST_ENV_API_PORT"); ok {
		e.ports.TestEnvironment, err = strconv.Atoi(portNum)
		if err != nil {
			panic(err)
		}
	}

	e.tempDir, err = os.MkdirTemp("", "opni-monitoring-test-*")
	if err != nil {
		return err
	}
	if options.enableEtcd {
		if err := os.Mkdir(path.Join(e.tempDir, "etcd"), 0700); err != nil {
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
		e.startEtcd()
	}
	if options.enableGateway {
		e.startGateway()
	}
	if options.enableCortex {
		e.startCortex()
	}
	if options.enableRealtimeServer {
		e.startRealtimeServer()
	}
	return nil
}

func (e *Environment) StartK8s() (*rest.Config, error) {
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
		return nil, err
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
	return cfg, nil
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
		waitctx.Wait(e.ctx, 20*time.Second)
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
	for e.ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", e.ports.Etcd))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	lg.Info("Etcd started")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
}

type cortexTemplateOptions struct {
	HttpListenPort int
	GrpcListenPort int
	StorageDir     string
}

func (e *Environment) startCortex() {
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
	}); err != nil {
		panic(err)
	}
	configFile.Close()
	cortexBin := filepath.Join(e.TestBin, "../../bin/opni")
	defaultArgs := []string{
		"cortex", fmt.Sprintf("-config.file=%s", path.Join(e.tempDir, "cortex/config.yaml")),
	}
	cmd := exec.CommandContext(e.ctx, cortexBin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		}
	}
	lg.Info("Waiting for cortex to start...")
	for e.ctx.Err() == nil {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/ready", e.ports.GatewayHTTP), nil)
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: e.GatewayTLSConfig(),
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
	lg.Info("Cortex started")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
}

func (e *Environment) startRealtimeServer() {
	if !e.enableRealtimeServer {
		e.Logger.Panic("realtime disabled")
	}

	srv, err := realtime.NewServer(&v1beta1.RealtimeServerSpec{
		ManagementClient: v1beta1.ManagementClientSpec{
			Address: fmt.Sprintf("localhost:%d", e.ports.ManagementGRPC),
		},
		Metrics: v1beta1.MetricsSpec{
			Port: e.ports.RTMetrics,
		},
	})
	if err != nil {
		panic(err)
	}
	go srv.Start(e.ctx)
}

type prometheusTemplateOptions struct {
	ListenPort    int
	RTMetricsPort int
	OpniAgentPort int
}

func (e *Environment) StartPrometheus(opniAgentPort int) int {
	lg := e.Logger
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	configTemplate := TestData("prometheus/config.yaml")
	t := util.Must(template.New("config").Parse(string(configTemplate)))
	configFile, err := os.Create(path.Join(e.tempDir, "prometheus", "config.yaml"))
	if err != nil {
		panic(err)
	}
	if err := t.Execute(configFile, prometheusTemplateOptions{
		ListenPort:    port,
		OpniAgentPort: opniAgentPort,
		RTMetricsPort: e.ports.RTMetrics,
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
	lg.Info("Prometheus started")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
	return port
}

func (e *Environment) newGatewayConfig() *v1beta1.GatewayConfig {
	caCertData := string(TestData("root_ca.crt"))
	servingCertData := string(TestData("localhost.crt"))
	servingKeyData := string(TestData("localhost.key"))
	return &v1beta1.GatewayConfig{
		TypeMeta: meta.TypeMeta{
			APIVersion: "v1beta1",
			Kind:       "GatewayConfig",
		},
		Spec: v1beta1.GatewayConfigSpec{
			Plugins: v1beta1.PluginsSpec{
				Dirs: []string{ // ¯\_(ツ)_/¯
					"bin",
					"../bin",
					"../../bin",
					"../../../bin",
					"../../../../bin",
					"../../../../../bin",
				},
			},
			HTTPListenAddress: fmt.Sprintf("localhost:%d", e.ports.GatewayHTTP),
			GRPCListenAddress: fmt.Sprintf("localhost:%d", e.ports.GatewayGRPC),
			EnableMonitor:     true,
			Management: v1beta1.ManagementSpec{
				GRPCListenAddress: fmt.Sprintf("tcp://127.0.0.1:%d", e.ports.ManagementGRPC),
				HTTPListenAddress: fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementHTTP),
				WebListenAddress:  fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementWeb),
			},
			AuthProvider: "test",
			Certs: v1beta1.CertsSpec{
				CACertData:      &caCertData,
				ServingCertData: &servingCertData,
				ServingKeyData:  &servingKeyData,
			},
			Cortex: v1beta1.CortexSpec{
				Distributor: v1beta1.DistributorSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
					GRPCAddress: fmt.Sprintf("localhost:%d", e.ports.CortexGRPC),
				},
				Ingester: v1beta1.IngesterSpec{
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
		if err := m.ListenAndServe(e.ctx); err != nil {
			lg.With(
				zap.Error(err),
			).Fatal("management server exited with error")
		}
	}))

	pluginLoader.Hook(hooks.OnLoadingCompleted(func(int) {
		if err := g.ListenAndServe(e.ctx); err != nil {
			lg.With(
				zap.Error(err),
			).Error("gateway server exited with error")
		}
	}))

	LoadPlugins(pluginLoader)

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
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
	}
	lg.Info("Gateway started")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
	})
}

type StartAgentOptions struct {
	ctx                  context.Context
	remoteGatewayAddress string
	remoteKubeconfig     string
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

func (e *Environment) StartAgent(id string, token *corev1.BootstrapToken, pins []string, opts ...StartAgentOption) (int, <-chan error) {
	options := &StartAgentOptions{
		ctx: e.ctx,
	}
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
				Discovery: v1beta1.DiscoverySpec{
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
	var a *agent.Agent
	mu := &sync.Mutex{}
	go func() {
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
		a, err = agent.New(options.ctx, agentConfig,
			agent.WithBootstrapper(&bootstrap.ClientConfig{
				Capability:    wellknown.CapabilityMetrics,
				Token:         bt,
				Endpoint:      gatewayAddress,
				TrustStrategy: strategy,
			}))
		if err != nil {
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
			Log.Error(err)
		}
		e.runningAgentsMu.Lock()
		delete(e.runningAgents, id)
		e.runningAgentsMu.Unlock()
	}()
	return port, errC
}

func (e *Environment) GetAgent(id string) RunningAgent {
	e.runningAgentsMu.Lock()
	defer e.runningAgentsMu.Unlock()
	return e.runningAgents[id]
}

func (e *Environment) GatewayTLSConfig() *tls.Config {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(*e.gatewayConfig.Spec.Certs.CACertData))
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}
}

func (e *Environment) GatewayConfig() *v1beta1.GatewayConfig {
	return e.gatewayConfig
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
	options.apply(opts...)

	agentOptions := &StartAgentOptions{}
	agentOptions.apply(options.defaultAgentOpts...)

	environment := &Environment{
		TestBin: "testbin/bin",
	}
	addAgent := func(rw http.ResponseWriter, r *http.Request) {
		Log.Infof("%s %s", r.Method, r.URL.Path)
		switch r.Method {
		case http.MethodPost:
			body := struct {
				Token string   `json:"token"`
				Pins  []string `json:"pins"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				rw.Write([]byte(err.Error()))
				return
			}
			token, err := tokens.ParseHex(body.Token)
			if err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				rw.Write([]byte(err.Error()))
				return
			}
			port, errC := environment.StartAgent(uuid.New().String(), token.ToBootstrapToken(), body.Pins, options.defaultAgentOpts...)
			if err := <-errC; err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				rw.Write([]byte(err.Error()))
				return
			}
			environment.StartPrometheus(port)
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
	signal.Notify(c, os.Interrupt)
	Log.Info(chalk.Blue.Color("Press (ctrl+c) to stop test environment"))
	// listen for spacebar on stdin
	t, err := tty.Open()
	if err == nil {
		if options.enableGateway {
			Log.Info(chalk.Blue.Color("Press (space) to open the web dashboard"))
		}
		if options.enableGateway || agentOptions.remoteKubeconfig != "" {
			Log.Info(chalk.Blue.Color("Press (a) to launch a new agent"))
		}
		var client managementv1.ManagementClient
		if options.enableGateway {
			client = environment.NewManagementClient()
		} else if agentOptions.remoteKubeconfig != "" {
			// c, err := util.NewK8sClient(util.ClientOptions{
			// 	Kubeconfig: &agentOptions.remoteKubeconfig,
			// })
			// if err != nil {
			// 	panic(err)
			// }
			// port-forward to service/opni-monitoring-internal:11090

		}
		go func() {
			for {
				rn, err := t.ReadRune()
				if err != nil {
					Log.Fatal(err)
				}
				switch rn {
				case ' ':
					go browser.OpenURL(fmt.Sprintf("http://localhost:%d", environment.ports.ManagementWeb))
				case 'a':
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
						resp, err := http.Post(fmt.Sprintf("http://localhost:%d/agents", environment.ports.TestEnvironment), "application/json",
							strings.NewReader(fmt.Sprintf(`{"token": "%s", "pins": ["%s"]}`, token.EncodeHex(), certInfo.Chain[len(certInfo.Chain)-1].Fingerprint)))
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
		}()
	}
	<-c
	fmt.Println(chalk.Yellow.Color("\nStopping test environment"))
	if err := environment.Stop(); err != nil {
		panic(err)
	}
}
