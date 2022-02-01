package test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/kralicky/opni-monitoring/pkg/agent"
	"github.com/kralicky/opni-monitoring/pkg/auth"
	"github.com/kralicky/opni-monitoring/pkg/bootstrap"
	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/gateway"
	"github.com/kralicky/opni-monitoring/pkg/ident"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	mock_ident "github.com/kralicky/opni-monitoring/pkg/test/mock/ident"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
	"github.com/onsi/ginkgo/v2"
	"github.com/phayes/freeport"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type servicePorts struct {
	Etcd           int
	Gateway        int
	ManagementGRPC int
	ManagementHTTP int
	CortexGRPC     int
	CortexHTTP     int
}

type Environment struct {
	TestBin string
	Logger  *zap.SugaredLogger

	waitGroup *sync.WaitGroup
	mockCtrl  *gomock.Controller
	ctx       context.Context
	cancel    context.CancelFunc
	tempDir   string
	ports     servicePorts

	gatewayConfig *v1beta1.GatewayConfig
}

func (e *Environment) Start() error {
	e.ctx, e.cancel = context.WithCancel(context.Background())
	var t gomock.TestReporter
	if strings.HasSuffix(os.Args[0], ".test") {
		t = ginkgo.GinkgoT()
	}
	e.mockCtrl = gomock.NewController(t)
	e.waitGroup = &sync.WaitGroup{}

	if _, err := auth.GetMiddleware("test"); err != nil {
		if err := auth.RegisterMiddleware("test", &TestAuthMiddleware{
			Strategy: AuthStrategyUserIDInAuthHeader,
		}); err != nil {
			return fmt.Errorf("failed to install test auth middleware: %w", err)
		}
	}
	ports, err := freeport.GetFreePorts(6)
	if err != nil {
		panic(err)
	}
	e.ports = servicePorts{
		Etcd:           ports[0],
		Gateway:        ports[1],
		ManagementGRPC: ports[2],
		ManagementHTTP: ports[3],
		CortexGRPC:     ports[4],
		CortexHTTP:     ports[5],
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
	if portNum, ok := os.LookupEnv("OPNI_GATEWAY_PORT"); ok {
		e.ports.Gateway, err = strconv.Atoi(portNum)
		if err != nil {
			return fmt.Errorf("failed to parse gateway port: %w", err)
		}
	}

	e.tempDir, err = os.MkdirTemp("", "opni-monitoring-test-*")
	if err != nil {
		return err
	}
	if err := os.Mkdir(path.Join(e.tempDir, "etcd"), 0700); err != nil {
		return err
	}
	cortexTempDir := path.Join(e.tempDir, "cortex")
	if err := os.MkdirAll(path.Join(cortexTempDir, "rules"), 0700); err != nil {
		return err
	}
	entries, _ := fs.ReadDir(TestDataFS, "testdata/cortex")
	fmt.Printf("Copying %d files from embedded testdata/cortex to %s\n", len(entries), cortexTempDir)
	for _, entry := range entries {
		if err := os.WriteFile(path.Join(cortexTempDir, entry.Name()), TestData("cortex/"+entry.Name()), 0644); err != nil {
			return err
		}
	}
	if err := os.Mkdir(path.Join(e.tempDir, "prometheus"), 0700); err != nil {
		return err
	}

	e.startEtcd()
	e.startGateway()
	e.startCortex()
	return nil
}

func (e *Environment) Stop() error {
	e.cancel()
	e.mockCtrl.Finish()
	e.waitGroup.Wait()
	os.RemoveAll(e.tempDir)
	return nil
}

func (e *Environment) startEtcd() {
	e.waitGroup.Add(1)
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		} else {
			return
		}
	}
	fmt.Println("Waiting for etcd to start...")
	for e.ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", e.ports.Etcd))
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	fmt.Println("Etcd started")
	go func() {
		defer e.waitGroup.Done()
		<-e.ctx.Done()
		cmd.Wait()
	}()
}

type cortexTemplateOptions struct {
	HttpListenPort int
	GrpcListenPort int
	StorageDir     string
}

func (e *Environment) startCortex() {
	e.waitGroup.Add(1)
	configTemplate := TestData("cortex/config.yaml")
	t := template.Must(template.New("config").Parse(string(configTemplate)))
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
	cortexBin := path.Join(e.TestBin, "cortex")
	defaultArgs := []string{
		fmt.Sprintf("-config.file=%s", path.Join(e.tempDir, "cortex/config.yaml")),
	}
	cmd := exec.CommandContext(e.ctx, cortexBin, defaultArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		}
	}
	fmt.Println("Waiting for cortex to start...")
	for e.ctx.Err() == nil {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/ready", e.ports.Gateway), nil)
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: e.GatewayTLSConfig(),
			},
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		time.Sleep(time.Second)
	}
	fmt.Println("Cortex started")
	go func() {
		defer e.waitGroup.Done()
		<-e.ctx.Done()
		cmd.Wait()
	}()
}

type prometheusTemplateOptions struct {
	ListenPort    int
	OpniAgentPort int
}

func (e *Environment) StartPrometheus(opniAgentPort int) int {
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	e.waitGroup.Add(1)
	configTemplate := TestData("prometheus/config.yaml")
	t := template.Must(template.New("config").Parse(string(configTemplate)))
	configFile, err := os.Create(path.Join(e.tempDir, "prometheus", "config.yaml"))
	if err != nil {
		panic(err)
	}
	if err := t.Execute(configFile, prometheusTemplateOptions{
		ListenPort:    port,
		OpniAgentPort: opniAgentPort,
	}); err != nil {
		panic(err)
	}
	configFile.Close()
	prometheusBin := path.Join(e.TestBin, "prometheus")
	defaultArgs := []string{
		fmt.Sprintf("--config.file=%s", path.Join(e.tempDir, "prometheus/config.yaml")),
		fmt.Sprintf("--storage.agent.path=%s", path.Join(e.tempDir, "prometheus")),
		fmt.Sprintf("--web.listen-address=127.0.0.1:%d", port),
		"--log.level=error",
		"--web.enable-lifecycle",
		"--enable-feature=agent",
	}
	cmd := exec.CommandContext(e.ctx, prometheusBin, defaultArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		if !errors.Is(e.ctx.Err(), context.Canceled) {
			panic(err)
		}
	}
	fmt.Println("Waiting for prometheus to start...")
	for e.ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", port))
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		time.Sleep(time.Second)
	}
	fmt.Println("Prometheus started")
	go func() {
		defer e.waitGroup.Done()
		<-e.ctx.Done()
		cmd.Wait()
	}()
	return port
}

func (e *Environment) newGatewayConfig() *v1beta1.GatewayConfig {
	caCertData := string(TestData("root_ca.crt"))
	servingCertData := string(TestData("localhost.crt"))
	servingKeyData := string(TestData("localhost.key"))
	return &v1beta1.GatewayConfig{
		Spec: v1beta1.GatewayConfigSpec{
			ListenAddress: fmt.Sprintf("localhost:%d", e.ports.Gateway),
			Management: v1beta1.ManagementSpec{
				GRPCListenAddress: fmt.Sprintf("tcp://127.0.0.1:%d", e.ports.ManagementGRPC),
				HTTPListenAddress: fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementHTTP),
			},
			AuthProvider: "test",
			Certs: v1beta1.CertsSpec{
				CACertData:      &caCertData,
				ServingCertData: &servingCertData,
				ServingKeyData:  &servingKeyData,
			},
			Cortex: v1beta1.CortexSpec{
				Distributor: v1beta1.DistributorSpec{
					Address: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				Ingester: v1beta1.IngesterSpec{
					Address: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				Alertmanager: v1beta1.AlertmanagerSpec{
					Address: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				Ruler: v1beta1.RulerSpec{
					Address: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
				},
				QueryFrontend: v1beta1.QueryFrontendSpec{
					Address: fmt.Sprintf("localhost:%d", e.ports.CortexHTTP),
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

func (e *Environment) NewManagementClient() management.ManagementClient {
	c, err := management.NewClient(e.ctx,
		management.WithListenAddress(fmt.Sprintf("127.0.0.1:%d", e.ports.ManagementGRPC)),
		management.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
	)
	if err != nil {
		panic(err)
	}
	return c
}

func (e *Environment) startGateway() {
	e.waitGroup.Add(1)
	e.gatewayConfig = e.newGatewayConfig()
	g := gateway.NewGateway(e.gatewayConfig,
		gateway.WithAuthMiddleware(e.gatewayConfig.Spec.AuthProvider),
	)
	go func() {
		if err := g.Listen(); err != nil {
			fmt.Println("gateway error:", err)
		}
	}()
	fmt.Println("Waiting for gateway to start...")
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/healthz",
			e.gatewayConfig.Spec.ListenAddress), nil)
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: e.GatewayTLSConfig(),
			},
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
	}
	fmt.Println("Gateway started")
	go func() {
		defer e.waitGroup.Done()
		<-e.ctx.Done()
		if err := g.Shutdown(); err != nil {
			fmt.Println("gateway error:", err)
		}
	}()
}

func (e *Environment) StartAgent(id string, token *core.BootstrapToken, pins []string) (int, <-chan error) {
	errC := make(chan error, 1)
	e.waitGroup.Add(1)
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}

	if err := ident.RegisterProvider("test", func() ident.Provider {
		mockIdent := mock_ident.NewMockProvider(e.mockCtrl)
		mockIdent.EXPECT().
			UniqueIdentifier(gomock.Any()).
			Return(id, nil).
			AnyTimes()
		return mockIdent
	}); err != nil {
		if !errors.Is(err, ident.ErrProviderAlreadyExists) {
			panic(err)
		}
	}

	agentConfig := &v1beta1.AgentConfig{
		Spec: v1beta1.AgentConfigSpec{
			ListenAddress:    fmt.Sprintf("localhost:%d", port),
			GatewayAddress:   fmt.Sprintf("localhost:%d", e.ports.Gateway),
			IdentityProvider: "test",
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
	go func() {
		a, err = agent.New(agentConfig,
			agent.WithBootstrapper(&bootstrap.ClientConfig{
				Token:    bt,
				Pins:     publicKeyPins,
				Endpoint: fmt.Sprintf("http://localhost:%d", e.ports.Gateway),
			}))
		if err != nil {
			errC <- err
			return
		}
		if err := a.ListenAndServe(); err != nil {
			errC <- err
		}
	}()
	go func() {
		defer e.waitGroup.Done()
		<-e.ctx.Done()
		if a == nil {
			return
		}
		if err := a.Shutdown(); err != nil {
			errC <- err
		}
	}()
	return port, errC
}

func (e *Environment) GatewayTLSConfig() *tls.Config {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(*e.gatewayConfig.Spec.Certs.CACertData))
	return &tls.Config{
		RootCAs: pool,
	}
}

func (e *Environment) GatewayConfig() *v1beta1.GatewayConfig {
	return e.gatewayConfig
}
