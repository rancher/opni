package test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/kralicky/totem"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/onsi/ginkgo/v2"
	"github.com/rancher/opni/pkg/agent"
	agentv2 "github.com/rancher/opni/pkg/agent/v2"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/gateway"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring/ephemeral"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/otel"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	pluginmeta "github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test/freeport"
	mock_ident "github.com/rancher/opni/pkg/test/mock/ident"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/samber/lo"
	"github.com/ttacon/chalk"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	_ "github.com/rancher/opni/pkg/oci/noop"
	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/update/noop"
	_ "github.com/rancher/opni/pkg/update/noop"
)

var (
	collectorWriteSync sync.Mutex
	agentList          = make(map[string]context.CancelFunc)
	agentListMu        sync.Mutex
)

type ServicePorts struct {
	Etcd             int `env:"ETCD_PORT"`
	Jetstream        int `env:"JETSTREAM_PORT"`
	GatewayGRPC      int `env:"OPNI_GATEWAY_GRPC_PORT"`
	GatewayHTTP      int `env:"OPNI_GATEWAY_HTTP_PORT"`
	GatewayMetrics   int `env:"OPNI_GATEWAY_METRICS_PORT"`
	ManagementGRPC   int `env:"OPNI_MANAGEMENT_GRPC_PORT"`
	ManagementHTTP   int `env:"OPNI_MANAGEMENT_HTTP_PORT"`
	ManagementWeb    int `env:"OPNI_MANAGEMENT_WEB_PORT"`
	CortexGRPC       int `env:"CORTEX_GRPC_PORT"`
	CortexHTTP       int `env:"CORTEX_HTTP_PORT"`
	TestEnvironment  int `env:"TEST_ENV_API_PORT"`
	DisconnectPort   int `env:"AGENT_DISCONNECT_PORT"`
	NodeExporterPort int `env:"NODE_EXPORTER_PORT"`
}

func newServicePorts() (ServicePorts, error) {
	// use environment variables if available, then fallback to random ports
	var p ServicePorts
	setRandomPorts := []func(int){}
	v := reflect.ValueOf(&p).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.CanSet() {
			envName := v.Type().Field(i).Tag.Get("env")
			if envName == "" {
				panic("missing env tag for " + v.Type().Field(i).Name)
			}
			if portStr, ok := os.LookupEnv(envName); ok {
				port, err := strconv.Atoi(portStr)
				if err != nil {
					return ServicePorts{}, fmt.Errorf("invalid port %s for %s: %w", portStr, envName, err)
				}
				field.SetInt(int64(port))
			} else {
				setRandomPorts = append(setRandomPorts, func(i int) {
					field.SetInt(int64(i))
				})
			}
		}
	}
	if len(setRandomPorts) > 0 {
		randomPorts := freeport.GetFreePorts(len(setRandomPorts))
		for i, port := range randomPorts {
			setRandomPorts[i](port)
		}
	}
	return p, nil
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

	tempDir        string
	certDir        string
	ports          ServicePorts
	localAgentOnce sync.Once

	runningAgents   map[string]RunningAgent
	runningAgentsMu sync.Mutex

	gatewayConfig *v1beta1.GatewayConfig

	embeddedJS *natsserver.Server

	nodeConfigOverridesMu sync.Mutex
	nodeConfigOverrides   map[string]*OverridePrometheusConfig
}

type EnvironmentOptions struct {
	enableEtcd             bool
	enableJetstream        bool
	enableGateway          bool
	defaultAgentOpts       []StartAgentOption
	defaultAgentVersion    string
	enableDisconnectServer bool
	enableNodeExporter     bool
	storageBackend         v1beta1.StorageType
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

func WithDefaultAgentOpts(opts ...StartAgentOption) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.defaultAgentOpts = opts
	}
}

func WithEnableNodeExporter(enable bool) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.enableNodeExporter = enable
	}
}

func WithStorageBackend(backend v1beta1.StorageType) EnvironmentOption {
	return func(o *EnvironmentOptions) {
		o.storageBackend = backend
	}
}

func defaultAgentVersion() string {
	if v, ok := os.LookupEnv("TEST_ENV_DEFAULT_AGENT_VERSION"); ok {
		return v
	}
	return "v2"
}

func defaultStorageBackend() v1beta1.StorageType {
	if v, ok := os.LookupEnv("TEST_ENV_DEFAULT_STORAGE_BACKEND"); ok {
		return v1beta1.StorageType(v)
	}
	return "jetstream"
}

func FindTestBin() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
WALK:
	for {
		if wd == "/" {
			return "", errors.New("could not find go.mod for github.com/rancher/opni")
		}
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			// check to make sure it's the right one
			f, err := os.Open(filepath.Join(wd, "go.mod"))
			if err != nil {
				return "", err
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "module") {
					if line == "module github.com/rancher/opni" {
						f.Close()
						break WALK
					}
					// not the right one (probably a sub-module)
					break
				}
			}
			f.Close()
		}
		wd = filepath.Dir(wd)
	}
	testbin := filepath.Join(wd, "testbin", "bin")
	if _, err := os.Stat(testbin); err != nil {
		return "", fmt.Errorf("testbin directory not found at its expected location: %s", testbin)
	}

	return testbin, nil
}

func (e *Environment) Start(opts ...EnvironmentOption) error {
	// TODO : bootstrap with otelcollector
	options := EnvironmentOptions{
		enableEtcd:             false,
		enableJetstream:        true,
		enableNodeExporter:     false,
		enableGateway:          true,
		enableDisconnectServer: false,
		defaultAgentVersion:    defaultAgentVersion(),
		storageBackend:         defaultStorageBackend(),
	}
	options.apply(opts...)

	if e.TestBin == "" {
		var err error
		e.TestBin, err = FindTestBin()
		if err != nil {
			return err
		}
	}

	if options.storageBackend == "etcd" && !options.enableEtcd {
		options.enableEtcd = true
	} else if options.storageBackend == "jetstream" && !options.enableJetstream {
		options.enableJetstream = true
	}

	e.Logger = testlog.Log.Named("env")
	e.nodeConfigOverrides = make(map[string]*OverridePrometheusConfig)

	e.EnvironmentOptions = options

	lg := e.Logger
	lg.Info("Starting test environment")

	e.initCtx()
	e.runningAgents = make(map[string]RunningAgent)

	var t gomock.TestReporter
	if strings.HasSuffix(os.Args[0], ".test") {
		t = ginkgo.GinkgoT()
	}
	e.mockCtrl = gomock.NewController(t)

	var err error
	e.ports, err = newServicePorts()
	if err != nil {
		return err
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

	{
		cortexTempDir := path.Join(e.tempDir, "cortex")
		if err := os.MkdirAll(path.Join(cortexTempDir, "rules"), 0700); err != nil {
			return err
		}
		if err := os.MkdirAll(path.Join(cortexTempDir, "data"), 0700); err != nil {
			return err
		}

		entries, _ := fs.ReadDir(testdata.TestDataFS, "testdata/cortex")
		lg.Infof("Copying %d files from embedded testdata/cortex to %s", len(entries), cortexTempDir)
		for _, entry := range entries {
			if err := os.WriteFile(path.Join(cortexTempDir, entry.Name()), testdata.TestData("cortex/"+entry.Name()), 0644); err != nil {
				return err
			}
		}
	}

	if err := os.Mkdir(path.Join(e.tempDir, "prometheus"), 0700); err != nil {
		return err
	}
	os.WriteFile(path.Join(e.tempDir, "prometheus", "sample-rules.yaml"), testdata.TestData("prometheus/sample-rules.yaml"), 0644)

	if err := os.Mkdir(path.Join(e.tempDir, "keyring"), 0700); err != nil {
		return err
	}

	key := ephemeral.NewKey(ephemeral.Authentication, map[string]string{
		session.AttributeLabelKey: "local",
	})
	keyData, _ := json.Marshal(key)
	os.WriteFile(path.Join(e.tempDir, "keyring", "local-agent.json"), keyData, 0400)

	if options.enableEtcd {
		e.startEtcd()
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

	if options.enableGateway {
		e.startGateway()
	}
	return nil
}

func (e *Environment) GetPorts() ServicePorts {
	return e.ports
}

func (e *Environment) PutTestData(inputPath string, data []byte) string {
	testPath := filepath.Join(e.tempDir, inputPath)
	mkdir := filepath.Dir(testPath)
	err := os.MkdirAll(mkdir, 0777)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(testPath, data, 0777)
	if err != nil {
		panic(err)
	}
	return testPath
}

func (e *Environment) GenerateNewTempDirectory(prefix string) string {
	return path.Join(e.tempDir, fmt.Sprintf("%s-%s", prefix, uuid.New().String()))
}

func (e *Environment) GetTempDirectory() string {
	return e.tempDir
}

func (e *Environment) Context() context.Context {
	return e.ctx
}

func (e *Environment) StartEmbeddedJetstream() (*nats.Conn, error) {
	ports := freeport.GetFreePorts(1)

	opts := natstest.DefaultTestOptions
	opts.Port = ports[0]

	e.embeddedJS = natstest.RunServer(&opts)
	e.embeddedJS.EnableJetStream(nil)
	if !e.embeddedJS.ReadyForConnections(2 * time.Second) {
		return nil, errors.New("starting nats server: timeout")
	}

	sUrl := fmt.Sprintf("nats://127.0.0.1:%d", ports[0])
	return nats.Connect(sUrl)
}

func (e *Environment) Stop() error {
	os.Unsetenv("NATS_SERVER_URL")
	os.Unsetenv("NKEY_SEED_FILENAME")

	if e.cancel != nil {
		e.cancel()
		waitctx.WaitWithTimeout(e.ctx, 40*time.Second, 20*time.Second)
	}
	if e.embeddedJS != nil {
		e.embeddedJS.Shutdown()
	}
	if e.mockCtrl != nil {
		e.mockCtrl.Finish()
	}
	if e.tempDir != "" {
		os.RemoveAll(e.tempDir)
	}
	return nil
}

type envContextKeyType struct{}

var envContextKey = envContextKeyType{}

func (e *Environment) initCtx() {
	e.once.Do(func() {
		e.ctx, e.cancel = context.WithCancel(context.WithValue(waitctx.Background(), envContextKey, e))
	})
}

func EnvFromContext(ctx context.Context) *Environment {
	return ctx.Value(envContextKey).(*Environment)
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
	plugins.ConfigureSysProcAttr(cmd)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		} else {
			return
		}
	}

	natsURL := fmt.Sprintf("nats://localhost:%d", e.ports.Jetstream)
	authConfigFile := path.Join(e.tempDir, "jetstream", "seed", "nats-auth.conf")
	err = os.WriteFile(authConfigFile, []byte(seed), 0644)
	if err != nil {
		panic("failed to write jetstream auth config")
	}
	lg.Info("Waiting for jetstream to start...")
	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
	for e.ctx.Err() == nil {
		opt, err := nats.NkeyOptionFromSeed(authConfigFile)
		if err != nil {
			panic(err)
		}
		if nc, err := nats.Connect(
			natsURL,
			opt,
			nats.MaxReconnects(-1),
			nats.RetryOnFailedConnect(true),
			nats.CustomReconnectDelay(func(attempts int) time.Duration {
				return 10 * time.Millisecond
			}),
		); err == nil {
			defer nc.Close()
			for e.ctx.Err() == nil {
				if _, err := nc.RTT(); err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			break
		} else {
			lg.Error(err)
		}
	}
	lg.Info("Jetstream started")
}

type AlertManagerPorts struct {
	ApiPort      int
	ClusterPort  int
	EmbeddedPort int
}

func (e *Environment) StartEmbeddedAlertManager(
	ctx context.Context,
	configFilePath string,
	opniPort *int,
	peers ...string,
) (ports AlertManagerPorts) {
	storagePath := path.Join(e.tempDir, "alertmanager_data", uuid.New().String())
	if err := os.MkdirAll(storagePath, 0700); err != nil {
		panic(err)
	}
	amBin := path.Join(e.TestBin, "../../bin/opni")
	fPorts := freeport.GetFreePorts(2)
	ports.ApiPort = fPorts[0]
	ports.ClusterPort = fPorts[1]
	ports.EmbeddedPort = lo.FromPtrOr(opniPort, 3000)
	defaultArgs := []string{
		"alerting-server",
		"alertmanager",
		fmt.Sprintf("--config.file=%s", configFilePath),
		fmt.Sprintf("--web.listen-address=:%d", ports.ApiPort),
		fmt.Sprintf("--cluster.listen-address=:%d", ports.ClusterPort),
		fmt.Sprintf("--storage.path=%s", storagePath),
		"--cluster.pushpull-interval=5s",
		"--cluster.peer-timeout=3s",
		"--cluster.gossip-interval=200ms",
		"--log.level=debug",
		"--no-opni.send-k8s",
	}
	if opniPort != nil {
		defaultArgs = append(defaultArgs, fmt.Sprintf("--opni.listen-address=:%d", *opniPort))
	}
	for _, peer := range peers {
		defaultArgs = append(defaultArgs, fmt.Sprintf("--cluster.peer=%s", peer))
	}
	cmd := exec.CommandContext(ctx, amBin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		}
	}
	e.Logger.Info("Waiting for alertmanager to start...")
	for e.ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", ports.ApiPort))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	e.Logger.With(
		"address", fmt.Sprintf("http://localhost:%d", ports.ApiPort),
		"opni-address", fmt.Sprintf("http://localhost:%d", opniPort)).
		Info("AlertManager started")
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		cmd, _ := session.G()
		if cmd != nil {
			cmd.Signal(os.Signal(syscall.SIGTERM))
		}
	})
	return
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
	HttpListenPort           int
	GrpcListenPort           int
	StorageDir               string
	AlertmanagerProxyAddress string
	CertDir                  string
}

func (e *Environment) StartCortex(ctx context.Context) {
	lg := e.Logger
	configTemplate := testdata.TestData("cortex/config.yaml")
	t := util.Must(template.New("config").Parse(string(configTemplate)))
	configFile, err := os.Create(path.Join(e.tempDir, "cortex", "config.yaml"))
	if err != nil {
		panic(err)
	}
	if err := t.Execute(configFile, cortexTemplateOptions{
		HttpListenPort:           e.ports.CortexHTTP,
		GrpcListenPort:           e.ports.CortexGRPC,
		StorageDir:               path.Join(e.tempDir, "cortex"),
		AlertmanagerProxyAddress: fmt.Sprintf("https://127.0.0.1:%d/plugin_alerting/alertmanager", e.ports.GatewayHTTP),
		CertDir:                  e.certDir,
	}); err != nil {
		panic(err)
	}
	configFile.Close()
	bin := filepath.Join(e.TestBin, "../../bin/opni")
	defaultArgs := []string{
		"cortex", fmt.Sprintf("--config.file=%s", path.Join(e.tempDir, "cortex/config.yaml")),
	}
	cmd := exec.CommandContext(ctx, bin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			panic(err)
		}
		return
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
		time.Sleep(50 * time.Millisecond)
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

type PrometheusJob struct {
	JobName     string
	ScrapePort  int
	MetricsPath string
}

type prometheusTemplateOptions struct {
	ListenPort       int
	OpniAgentAddress string
	// these fill in a text/template defined as {{.range Jobs}} /* */ {{end}}
	Jobs []PrometheusJob
}

type OverridePrometheusConfig struct {
	configContents string
	jobs           []PrometheusJob
}

func NewOverridePrometheusConfig(configPath string, jobs []PrometheusJob) *OverridePrometheusConfig {
	return &OverridePrometheusConfig{
		configContents: string(testdata.TestData(configPath)),
		jobs:           jobs,
	}
}

func (e *Environment) SetPrometheusNodeConfigOverride(agentId string, override *OverridePrometheusConfig) {
	e.nodeConfigOverridesMu.Lock()
	defer e.nodeConfigOverridesMu.Unlock()
	e.nodeConfigOverrides[agentId] = override
}

// `prometheus/config.yaml` is the default monitoring config.
// `slo/prometheus/config.yaml` is the default SLO config.
func (e *Environment) UnsafeStartPrometheus(ctx waitctx.PermissiveContext, opniAgentId string, override ...*OverridePrometheusConfig) (int, error) {
	if len(override) == 0 {
		e.nodeConfigOverridesMu.Lock()
		if v, ok := e.nodeConfigOverrides[opniAgentId]; ok {
			override = append(override, v)
		}
		e.nodeConfigOverridesMu.Unlock()
	}
	lg := e.Logger
	port := freeport.GetFreePort()

	var configTemplate string
	var jobs []PrometheusJob

	configTemplate = string(testdata.TestData("prometheus/config.yaml"))
	if len(override) > 1 {
		return 0, errors.New("Too many overrides, only one is allowed")
	}
	if len(override) == 1 && override[0] != nil {
		configTemplate = override[0].configContents
		jobs = override[0].jobs
	}
	t, err := template.New("").Parse(configTemplate)
	if err != nil {
		return 0, err
	}
	promDir := path.Join(e.tempDir, "prometheus", opniAgentId)
	if err := os.MkdirAll(promDir, 0755); err != nil {
		return 0, err
	}
	configFile, err := os.Create(path.Join(promDir, "config.yaml"))
	if err != nil {
		return 0, err
	}

	agent := e.GetAgent(opniAgentId)
	if agent.Agent == nil {
		panic("test bug: agent not found: " + opniAgentId)
	}

	if err := t.Execute(configFile, prometheusTemplateOptions{
		ListenPort:       port,
		OpniAgentAddress: agent.Agent.ListenAddress(),
		Jobs:             jobs,
	}); err != nil {
		return 0, err
	}

	configFile.Close()
	prometheusBin := path.Join(e.TestBin, "prometheus")
	defaultArgs := []string{
		fmt.Sprintf("--config.file=%s", path.Join(promDir, "config.yaml")),
		fmt.Sprintf("--storage.agent.path=%s", promDir),
		fmt.Sprintf("--web.listen-address=127.0.0.1:%d", port),
		"--log.level=error",
		"--web.enable-lifecycle",
		"--enable-feature=agent",
	}
	cmd := exec.CommandContext(ctx, prometheusBin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			return 0, err
		}
	}
	lg.Info("Waiting for prometheus to start...")
	for {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("failed to start prometheus: %w", ctx.Err())
		}
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", port))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	lg.With(
		"address", fmt.Sprintf("http://localhost:%d", port),
		"dir", promDir,
		"agentId", opniAgentId,
	).Info("Prometheus started")
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		cmd.Process.Kill()
		session.Wait()
		os.RemoveAll(promDir)
	})
	return port, nil
}

type TestNodeConfig struct {
	AggregatorAddress string
	ReceiverFile      string
	Instance          string
	Logs              otel.LoggingConfig
	Metrics           otel.MetricsConfig
	Containerized     bool
}

func (t TestNodeConfig) MetricReceivers() []string {
	res := []string{}
	if t.Metrics.Enabled {
		res = append(res, "prometheus/self")
		if lo.FromPtrOr(t.Metrics.Spec.HostMetrics, false) {
			res = append(res, "hostmetrics")
			if t.Containerized {
				res = append(res, "kubeletstats")
			}
		}
	}
	return res
}

type TestAggregatorConfig struct {
	AggregatorAddress  string
	HealthCheckAddress string
	AgentEndpoint      string
	LogsEnabled        bool
	Metrics            otel.MetricsConfig
	Containerized      bool
}

func (t TestAggregatorConfig) MetricReceivers() []string {
	res := []string{}
	if t.Metrics.Enabled {
		res = append(res, "prometheus/self")
		if len(t.Metrics.Spec.AdditionalScrapeConfigs) > 0 {
			res = append(res, "prometheus/additional")
		}
		if len(t.Metrics.DiscoveredScrapeCfg) > 0 {
			res = append(res, "prometheus/discovered")
		}
	}
	return res
}

func (e *Environment) StartOTELCollectorContext(ctx waitctx.PermissiveContext, opniAgentId string, spec *otel.OTELSpec) error {
	otelDir := path.Join(e.tempDir, "otel", opniAgentId)
	os.MkdirAll(otelDir, 0755)

	receiverFile, err := os.Create(path.Join(otelDir, "receiver.yaml"))
	if err != nil {
		return err
	}

	configFile, err := os.Create(path.Join(otelDir, "config.yaml"))
	if err != nil {
		return err
	}
	aggregatorFile, err := os.Create(path.Join(otelDir, "aggregator.yaml"))
	if err != nil {
		return err
	}

	agent := e.GetAgent(opniAgentId)
	if agent.Agent == nil {
		panic("test bug: agent not found")
	}

	ports := freeport.GetFreePorts(4)
	aggregatorOTLP := fmt.Sprintf("127.0.0.1:%d", ports[0])

	e.nodeConfigOverridesMu.Lock()
	agentOverrides, ok := e.nodeConfigOverrides[opniAgentId]
	e.nodeConfigOverridesMu.Unlock()

	if ok {
		spec = spec.DeepCopy()
		for _, job := range agentOverrides.jobs {
			spec.AdditionalScrapeConfigs = append(spec.AdditionalScrapeConfigs, &otel.ScrapeConfig{
				JobName:        job.JobName,
				Targets:        []string{fmt.Sprintf("127.0.0.1:%d", job.ScrapePort)},
				ScrapeInterval: "15s",
			})
		}
	}

	aggregatorCfg := TestAggregatorConfig{
		AggregatorAddress:  aggregatorOTLP,
		HealthCheckAddress: fmt.Sprintf("127.0.0.1:%d", ports[1]),
		AgentEndpoint:      agent.Agent.ListenAddress(),
		Metrics: otel.MetricsConfig{
			Enabled:             true,
			LogLevel:            "error",
			ListenPort:          ports[2],
			RemoteWriteEndpoint: fmt.Sprintf("http://%s/api/agent/push", agent.Agent.ListenAddress()),
			DiscoveredScrapeCfg: "",
			Spec:                spec,
		},
		Containerized: false,
	}
	t := util.Must(otel.OTELTemplates.ParseFS(testdata.TestDataFS, "testdata/otel/base.tmpl"))
	aggregatorTmpl := util.Must(t.Parse(`{{template "aggregator-config" .}}`))
	if err := aggregatorTmpl.Execute(aggregatorFile, aggregatorCfg); err != nil {
		return err
	}
	// the pattern we use in production is node -> aggregator -> agent
	nodeCfg := TestNodeConfig{
		AggregatorAddress: aggregatorOTLP,
		Instance:          "opni",
		ReceiverFile:      fmt.Sprintf("%s/receiver.yaml", otelDir),
		Metrics: otel.MetricsConfig{
			Enabled:             true,
			LogLevel:            "error",
			ListenPort:          ports[3],
			RemoteWriteEndpoint: fmt.Sprintf("%s/api/agent/push", agent.Agent.ListenAddress()),
			DiscoveredScrapeCfg: "",
			Spec:                spec,
		},
		Containerized: false,
	}
	receiverTmpl := util.Must(t.Parse(`{{template "node-receivers" .}}`))
	if err := receiverTmpl.Execute(receiverFile, nodeCfg); err != nil {
		return err
	}
	receiverFile.Close()
	mainTmpl := util.Must(t.Parse(`{{template "node-config" .}}`))
	if err := mainTmpl.Execute(configFile, nodeCfg); err != nil {
		return err
	}
	configFile.Close()
	otelColBin := path.Join(e.TestBin, "otelcol-custom")
	nodeArgs := []string{
		fmt.Sprintf("--config=%s", path.Join(otelDir, "config.yaml")),
	}
	aggregatorArgs := []string{
		fmt.Sprintf("--config=%s", path.Join(otelDir, "aggregator.yaml")),
	}
	e.Logger.Infof("launching process: `%s %s`", otelColBin, strings.Join(nodeArgs, " "))
	nodeCmd := exec.CommandContext(ctx, otelColBin, nodeArgs...)
	plugins.ConfigureSysProcAttr(nodeCmd)
	nodeCmd.Cancel = func() error {
		return nodeCmd.Process.Signal(syscall.SIGINT)
	}
	nodeSession, err := testutil.StartCmd(nodeCmd)
	if err != nil {
		return err
	}
	e.Logger.Infof("launching process: `%s %s`", otelColBin, strings.Join(aggregatorArgs, " "))
	aggregatorCmd := exec.CommandContext(ctx, otelColBin, aggregatorArgs...)
	plugins.ConfigureSysProcAttr(aggregatorCmd)
	aggregatorCmd.Cancel = func() error {
		return aggregatorCmd.Process.Signal(syscall.SIGINT)
	}
	aggregatorSession, err := testutil.StartCmd(aggregatorCmd)
	if err != nil {
		return err
	}

	// wait for aggregator health to be ok

	e.Logger.Info("Waiting for collector to start...")
	var ready bool
	for i := 0; i < 40; i++ {
		if ctx.Err() != nil {
			break
		}
		resp, err := http.Get(fmt.Sprintf("http://%s/healthz", aggregatorCfg.HealthCheckAddress))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		e.Logger.Error("timed out waiting for collector to start")
		return fmt.Errorf("timed out waiting for collector to start")
	}
	e.Logger.Info("collector started")

	go func() {
		<-ctx.Done()
		aggregatorSession.Wait()
		nodeSession.Wait()
	}()

	return nil
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
	testlog.Log.Infof(chalk.Green.Color("Agent Disconnect server listening on %d"), e.ports.DisconnectPort)
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

func (e *Environment) SimulateKubeObject(kPort int) {
	// sample a random phase
	namespaces := []string{"kube-system", "default", "opni"}
	namespace := namespaces[rand.Intn(len(namespaces))]
	sampleObjects := []string{"pod", "deployment", "statefulset", "daemonset", "job", "cronjob", "service", "ingress"}
	sampleObject := sampleObjects[rand.Intn(len(sampleObjects))]
	sampleState := shared.KubeStates[rand.Intn(len(shared.KubeStates))]

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

func (e *Environment) NewGatewayConfig() *v1beta1.GatewayConfig {
	caCertData := testdata.TestData("root_ca.crt")
	servingCertData := testdata.TestData("localhost.crt")
	servingKeyData := testdata.TestData("localhost.key")
	e.certDir = path.Join(e.tempDir, "gateway/certs")
	err := os.MkdirAll(e.certDir, 0755)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(path.Join(e.certDir, "root_ca.crt"), caCertData, 0644)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(path.Join(e.certDir, "localhost.crt"), servingCertData, 0644)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(path.Join(e.certDir, "localhost.key"), servingKeyData, 0644)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(path.Join(e.certDir, "client.crt"), servingCertData, 0644)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(path.Join(e.certDir, "client.key"), servingKeyData, 0644)
	if err != nil {
		panic(err)
	}

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
				GRPCListenAddress: fmt.Sprintf("tcp://localhost:%d", e.ports.ManagementGRPC),
				HTTPListenAddress: fmt.Sprintf("localhost:%d", e.ports.ManagementHTTP),
				WebListenAddress:  fmt.Sprintf("localhost:%d", e.ports.ManagementWeb),
			},
			AuthProvider: "test",
			Certs: v1beta1.CertsSpec{
				CACertData:      caCertData,
				ServingCertData: servingCertData,
				ServingKeyData:  servingKeyData,
			},
			Plugins: v1beta1.PluginsSpec{
				Binary: v1beta1.BinaryPluginsSpec{
					Cache: v1beta1.CacheSpec{
						Backend:     v1beta1.CacheBackendFilesystem,
						PatchEngine: v1beta1.PatchEngineBsdiff,
						Filesystem: v1beta1.FilesystemCacheSpec{
							Dir: e.tempDir + "/cache",
						},
					},
				},
			},
			Cortex: v1beta1.CortexSpec{
				Management: v1beta1.ClusterManagementSpec{
					ClusterDriver: "test-environment",
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
					GRPCAddress: fmt.Sprintf("localhost:%d", e.ports.CortexGRPC),
				},
				QueryFrontend: v1beta1.QueryFrontendSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
					GRPCAddress: fmt.Sprintf("localhost:%d", e.ports.CortexGRPC),
				},
				Purger: v1beta1.PurgerSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				Querier: v1beta1.QuerierSpec{
					HTTPAddress: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				Certs: v1beta1.MTLSSpec{
					ServerCA:   path.Join(e.tempDir, "cortex/root.crt"),
					ClientCA:   path.Join(e.tempDir, "cortex/root.crt"),
					ClientCert: path.Join(e.tempDir, "cortex/client.crt"),
					ClientKey:  path.Join(e.tempDir, "cortex/client.key"),
				},
			},
			Storage: lo.Switch[v1beta1.StorageType, v1beta1.StorageSpec](e.storageBackend).
				Case(v1beta1.StorageTypeEtcd, v1beta1.StorageSpec{
					Type: v1beta1.StorageTypeEtcd,
					Etcd: &v1beta1.EtcdStorageSpec{
						Endpoints: []string{fmt.Sprintf("http://localhost:%d", e.ports.Etcd)},
					},
				}).
				Case(v1beta1.StorageTypeJetStream, v1beta1.StorageSpec{
					Type: v1beta1.StorageTypeJetStream,
					JetStream: &v1beta1.JetStreamStorageSpec{
						Endpoint:     fmt.Sprintf("nats://localhost:%d", e.ports.Jetstream),
						NkeySeedPath: path.Join(e.tempDir, "jetstream", "seed", "nats-auth.conf"),
					},
				}).
				DefaultF(func() v1beta1.StorageSpec {
					panic("unknown storage backend")
				}),
			Alerting: v1beta1.AlertingSpec{
				ConfigMap:             "alertmanager-config",
				Namespace:             "default",
				WorkerNodeService:     "opni-alerting",
				WorkerStatefulSet:     "opni-alerting-internal",
				WorkerPort:            9093,
				ControllerNodeService: "opni-alerting-controller",
				ControllerStatefulSet: "opni-alerting-controller-internal",
				ControllerNodePort:    9093,
				ControllerClusterPort: 9094,
				ManagementHookHandler: shared.AlertingDefaultHookName,
			},
			Keyring: v1beta1.KeyringSpec{
				EphemeralKeyDirs: []string{
					path.Join(e.tempDir, "keyring"),
				},
			},
		},
	}
}

type EnvClientOptions struct {
	dialOptions []grpc.DialOption
}

type EnvClientOption func(o *EnvClientOptions)

func (e *EnvClientOptions) apply(opts ...EnvClientOption) {
	for _, opt := range opts {
		opt(e)
	}
}

func WithClientCaching(memoryLimitBytes int64, _ time.Duration) EnvClientOption {
	return func(o *EnvClientOptions) {
		entityCacher := caching.NewInMemoryGrpcTtlCache(memoryLimitBytes, time.Minute)
		interceptor := caching.NewClientGrpcTtlCacher()
		interceptor.SetCache(entityCacher)
		o.dialOptions = append(o.dialOptions, grpc.WithChainUnaryInterceptor(
			interceptor.UnaryClientInterceptor(),
		))
	}
}

func (e *Environment) NewStreamConnection(pins []string) (grpc.ClientConnInterface, <-chan error) {
	if !e.enableGateway {
		panic("gateway is not enabled")
	}
	outC := make(chan error)

	publicKeyPins := make([]*pkp.PublicKeyPin, len(pins))
	for i, pin := range pins {
		d, err := pkp.DecodePin(pin)
		if err != nil {
			outC <- err
			return nil, outC
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
		outC <- err
		return nil, outC
	}

	tlsConfig, err := strategy.TLSConfig()
	if err != nil {
		outC <- err
		return nil, outC
	}

	conn, err := grpc.DialContext(e.ctx,
		fmt.Sprintf("localhost:%d", e.ports.GatewayGRPC),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithDefaultCallOptions(
			grpc.WaitForReady(true),
			grpc.MaxCallSendMsgSize(math.MaxInt32),
			grpc.MaxCallRecvMsgSize(math.MaxInt32),
		),
	)
	if err != nil {
		outC <- err
		return nil, outC
	}

	streamClient := streamv1.NewStreamClient(conn)
	stream, err := streamClient.Connect(e.ctx)
	if err != nil {
		outC <- err
		return nil, outC
	}

	ts, err := totem.NewServer(
		stream,
		totem.WithName("testenv"),
		totem.WithTracerOptions(
			resource.WithAttributes(
				semconv.ServiceNameKey.String("testenv"),
			),
		),
	)
	if err != nil {
		outC <- err
		return nil, outC
	}

	return ts.Serve()
}

func (e *Environment) NewManagementClient(opts ...EnvClientOption) managementv1.ManagementClient {
	options := EnvClientOptions{
		dialOptions: []grpc.DialOption{},
	}
	options.apply(opts...)
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}

	dialOpts := append([]grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	}, options.dialOptions...)

	c, err := clients.NewManagementClient(e.ctx,
		clients.WithAddress(fmt.Sprintf("localhost:%d", e.ports.ManagementGRPC)),
		clients.WithDialOptions(
			dialOpts...,
		),
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
	cc, err := grpc.DialContext(e.ctx, fmt.Sprintf("localhost:%d", e.ports.ManagementGRPC),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}
	return cc
}

func (e *Environment) NewAlertEndpointsClient() alertingv1.AlertEndpointsClient {
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}
	c, err := alertingv1.NewEndpointsClient(e.ctx,
		alertingv1.WithListenAddress(fmt.Sprintf("localhost:%d", e.ports.ManagementGRPC)),
		alertingv1.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))))
	if err != nil {
		panic(err)
	}
	return c
}

func (e *Environment) NewAlertConditionsClient() alertingv1.AlertConditionsClient {
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}
	c, err := alertingv1.NewConditionsClient(e.ctx,
		alertingv1.WithListenAddress(fmt.Sprintf("localhost:%d", e.ports.ManagementGRPC)),
		alertingv1.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))))
	if err != nil {
		panic(err)
	}
	return c
}

func (e *Environment) NewAlertNotificationsClient() alertingv1.AlertNotificationsClient {
	if !e.enableGateway {
		e.Logger.Panic("gateway disabled")
	}
	c, err := alertingv1.NewNotificationsClient(e.ctx,
		alertingv1.WithListenAddress(fmt.Sprintf("localhost:%d", e.ports.ManagementGRPC)),
		alertingv1.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))))
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
	e.gatewayConfig = e.NewGatewayConfig()
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
		gateway.WithExtraUpdateHandlers(noop.NewSyncServer()),
	)
	m := management.NewServer(e.ctx, &e.gatewayConfig.Spec.Management, g, pluginLoader,
		management.WithCapabilitiesDataSource(g),
		management.WithHealthStatusDataSource(g),
		management.WithLifecycler(lifecycler),
	)

	g.MustRegisterCollector(m)

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

	lg.Info("Loading gateway plugins...")
	globalTestPlugins.LoadPlugins(e.ctx, pluginLoader, pluginmeta.ModeGateway)

	lg.Info("Waiting for gateway to start...")
	started := false
	for i := 0; i < 100; i++ {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/healthz",
			e.gatewayConfig.Spec.MetricsListenAddress), nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				started = true
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !started {
		lg.Panic("gateway failed to start")
	}

	lg.Info("Gateway started")
}

type StartAgentOptions struct {
	ctx                  context.Context
	remoteGatewayAddress string
	listenPort           int
	version              string
	local                bool
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

func WithAgentVersion(version string) StartAgentOption {
	return func(o *StartAgentOptions) {
		o.version = version
	}
}

func WithLocalAgent() StartAgentOption {
	return func(o *StartAgentOptions) {
		o.local = true
	}
}

func WithListenPort(port int) StartAgentOption {
	return func(o *StartAgentOptions) {
		o.listenPort = port
	}
}

func (e *Environment) BootstrapNewAgent(id string, opts ...StartAgentOption) error {
	cc := e.ManagementClientConn()

	client := managementv1.NewManagementClient(cc)
	token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
		Ttl: durationpb.New(time.Hour),
	})
	if err != nil {
		return err
	}

	certs, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
	if err != nil {
		return err
	}
	fp := certs.Chain[len(certs.Chain)-1].Fingerprint

	_, errC := e.StartAgent(id, token, []string{fp}, opts...)
	select {
	case err := <-errC:
		return err
	case <-e.ctx.Done():
		return e.ctx.Err()
	}
}

func (e *Environment) DeleteAgent(id string) error {
	cc := e.ManagementClientConn()

	client := managementv1.NewManagementClient(cc)
	_, err := client.DeleteCluster(context.Background(), &corev1.Reference{
		Id: id,
	})

	return err
}

func (e *Environment) StartAgent(id string, token *corev1.BootstrapToken, pins []string, opts ...StartAgentOption) (int, <-chan error) {
	options := &StartAgentOptions{
		version:    e.defaultAgentVersion,
		ctx:        e.ctx,
		listenPort: freeport.GetFreePort(),
	}
	options.apply(e.defaultAgentOpts...)
	options.apply(opts...)
	if !e.enableGateway && options.remoteGatewayAddress == "" {
		e.Logger.Panic("gateway disabled")
	}

	errC := make(chan error, 2)
	if err := ident.RegisterProvider(id, func(_ ...any) ident.Provider {
		return mock_ident.NewTestIdentProvider(e.mockCtrl, id)
	}); err != nil {
		if !errors.Is(err, ident.ErrProviderAlreadyExists) {
			panic(err)
		}
	}

	gatewayAddress := fmt.Sprintf("127.0.0.1:%d", e.ports.GatewayGRPC)
	if options.remoteGatewayAddress != "" {
		gatewayAddress = options.remoteGatewayAddress
	}

	agentConfig := &v1beta1.AgentConfig{
		Spec: v1beta1.AgentConfigSpec{
			Upgrade: v1beta1.AgentUpgradeSpec{
				Type: v1beta1.AgentUpgradeNoop,
			},
			PluginUpgrade: v1beta1.PluginUpgradeSpec{
				Type: v1beta1.PluginUpgradeNoop,
			},
			TrustStrategy:    v1beta1.TrustStrategyPKP,
			ListenAddress:    fmt.Sprintf("127.0.0.1:%d", options.listenPort),
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
			// Storage: v1beta1.StorageSpec{
			// 	Type: v1beta1.StorageTypeEtcd,
			// 	Etcd: &v1beta1.EtcdStorageSpec{
			// 		Endpoints: []string{fmt.Sprintf("http://localhost:%d", e.ports.Etcd)},
			// 	},
			// },
			Storage: lo.Switch[v1beta1.StorageType, v1beta1.StorageSpec](e.storageBackend).
				Case(v1beta1.StorageTypeEtcd, v1beta1.StorageSpec{
					Type: v1beta1.StorageTypeEtcd,
					Etcd: &v1beta1.EtcdStorageSpec{
						Endpoints: []string{fmt.Sprintf("http://127.0.0.1:%d", e.ports.Etcd)},
					},
				}).
				Case(v1beta1.StorageTypeJetStream, v1beta1.StorageSpec{
					Type: v1beta1.StorageTypeJetStream,
					JetStream: &v1beta1.JetStreamStorageSpec{
						Endpoint:     fmt.Sprintf("nats://127.0.0.1:%d", e.ports.Jetstream),
						NkeySeedPath: path.Join(e.tempDir, "jetstream", "seed", "nats-auth.conf"),
					},
				}).
				DefaultF(func() v1beta1.StorageSpec {
					panic("unknown storage backend")
				}),
		},
	}
	if options.local {
		agentConfig.Spec.Keyring.EphemeralKeyDirs = append(agentConfig.Spec.Keyring.EphemeralKeyDirs,
			path.Join(e.tempDir, "keyring"),
		)
	}

	testlog.Log.With(
		"id", id,
		"address", agentConfig.Spec.ListenAddress,
		"version", options.version,
	).Info("starting agent")

	var bootstrapper bootstrap.Bootstrapper
	if token != nil && len(pins) > 0 {
		bt, err := tokens.FromBootstrapToken(token)
		if err != nil {
			errC <- err
			return 0, errC
		}
		publicKeyPins := make([]*pkp.PublicKeyPin, len(pins))
		for i, pin := range pins {
			d, err := pkp.DecodePin(pin)
			if err != nil {
				errC <- err
				return 0, errC
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
			return 0, errC
		}
		bootstrapper = &bootstrap.ClientConfigV2{
			Token:         bt,
			Endpoint:      gatewayAddress,
			TrustStrategy: strategy,
		}
	}
	var a agent.AgentInterface
	mu := &sync.Mutex{}
	waitctx.Permissive.Go(options.ctx, func() {
		mu.Lock()
		switch options.version {
		case "v2":
			var err error
			ctx, cancel := context.WithCancel(options.ctx)
			pl := plugins.NewPluginLoader()
			a, err = agentv2.New(ctx, agentConfig,
				agentv2.WithBootstrapper(bootstrapper),
				agentv2.WithUnmanagedPluginLoader(pl),
			)
			if err != nil {
				testlog.Log.With(zap.Error(err)).Error("failed to start agent")
				errC <- err
				cancel()
				mu.Unlock()
				return
			}
			globalTestPlugins.LoadPlugins(e.ctx, pl, pluginmeta.ModeAgent)
			agentListMu.Lock()
			agentList[id] = cancel
			agentListMu.Unlock()
		default:
			errC <- fmt.Errorf("unknown agent version %q (expected \"v2\")", options.version)
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
			testlog.Log.Errorf("agent %q exited: %v", id, err)
		}
		e.runningAgentsMu.Lock()
		delete(e.runningAgents, id)
		e.runningAgentsMu.Unlock()
	})
	return options.listenPort, errC
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

func (e *Environment) GatewayClientTLSConfig() *tls.Config {
	pool := x509.NewCertPool()

	// Load the root CA certificate
	caCertFile := path.Join(e.certDir, "root_ca.crt")
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		panic(err)
	}
	pool.AppendCertsFromPEM(caCert)

	// Load the client certificate and key
	clientCertFile := path.Join(e.certDir, "client.crt")
	clientKeyFile := path.Join(e.certDir, "client.key")
	clientCert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		panic(err)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      pool,
		Certificates: []tls.Certificate{clientCert},
	}
}

func (e *Environment) CortexTLSConfig() *tls.Config {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(testdata.TestData("cortex/root.crt")) {
		e.Logger.Panic("failed to load Cortex CA cert")
	}
	clientCert := testdata.TestData("cortex/client.crt")
	clientKey := testdata.TestData("cortex/client.key")
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

func (e *Environment) JetStreamConfig() *v1beta1.JetStreamStorageSpec {
	if !e.enableJetstream {
		e.Logger.Panic("JetStream disabled")
	}
	return &v1beta1.JetStreamStorageSpec{
		Endpoint:     fmt.Sprintf("http://localhost:%d", e.ports.Jetstream),
		NkeySeedPath: path.Join(e.tempDir, "jetstream", "seed", "nats-auth.conf"),
	}
}

type grafanaConfigTemplateData struct {
	DatasourceUrl string
}

func (e *Environment) WriteGrafanaConfig() {
	os.MkdirAll(path.Join(e.tempDir, "grafana", "provisioning", "datasources"), 0777)
	os.MkdirAll(path.Join(e.tempDir, "grafana", "provisioning", "dashboards"), 0777)
	os.MkdirAll(path.Join(e.tempDir, "grafana", "dashboards"), 0777)

	tmplData := grafanaConfigTemplateData{
		DatasourceUrl: fmt.Sprintf("https://localhost:%d", e.ports.GatewayHTTP),
	}

	datasourceTemplate := testdata.TestData("grafana/datasource.yaml")
	t := util.Must(template.New("datasource").Parse(string(datasourceTemplate)))
	datasourceFile, err := os.Create(path.Join(e.tempDir, "grafana", "provisioning", "datasources", "datasource.yaml"))
	if err != nil {
		panic(err)
	}
	defer datasourceFile.Close()
	err = t.Execute(datasourceFile, tmplData)
	if err != nil {
		panic(err)
	}

	dashboardTemplate := testdata.TestData("grafana/dashboards.yaml")
	t = util.Must(template.New("dashboard").Parse(string(dashboardTemplate)))
	dashboardFile, err := os.Create(path.Join(e.tempDir, "grafana", "provisioning", "dashboards", "dashboards.yaml"))
	if err != nil {
		panic(err)
	}
	defer dashboardFile.Close()
	err = t.Execute(dashboardFile, tmplData)
	if err != nil {
		panic(err)
	}
}

func (e *Environment) StartGrafana(extraDockerArgs ...string) {
	baseDir := path.Join(e.tempDir, "grafana")

	args := append(
		append(
			[]string{
				"run",
				"-q",
				"--rm",
				"--name=opni-testenv-grafana",
				"-v", fmt.Sprintf("%s/provisioning:/etc/grafana/provisioning", baseDir),
				"-v", fmt.Sprintf("%s/dashboards:/dashboards", baseDir),
				"-p", "3000:3000",
				"--net=host",
				"-e", "GF_INSTALL_PLUGINS=grafana-polystat-panel,marcusolsson-treemap-panel,michaeldmoore-multistat-panel",
				"-e", "GF_ALERTING_ENABLED=false",
				"-e", "GF_AUTH_DISABLE_LOGIN_FORM=true",
				"-e", "GF_AUTH_DISABLE_SIGNOUT_MENU=true",
				"-e", "GF_AUTH_ANONYMOUS_ENABLED=true",
				"-e", "GF_AUTH_ANONYMOUS_ORG_ROLE=Admin",
				"-e", "GF_AUTH_ANONYMOUS_ORG_NAME=Main Org.",
				"-e", "GF_FEATURE_TOGGLES_ENABLE=accessTokenExpirationCheck panelTitleSearch increaseInMemDatabaseQueryCache newPanelChromeUI",
				"-e", "GF_SERVER_DOMAIN=localhost",
				"-e", "GF_SERVER_ROOT_URL=http://localhost",
			},
			extraDockerArgs...,
		),
		"grafana/grafana:latest",
	)

	cmd := exec.CommandContext(e.ctx, "docker", args...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGINT)
	}
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		} else {
			return
		}
	}

	waitctx.Go(e.ctx, func() {
		<-e.ctx.Done()
		session.Wait()
	})
}
