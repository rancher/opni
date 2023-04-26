package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mattn/go-tty"
	"github.com/pkg/browser"
	v1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/dashboard"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/spf13/pflag"
	"github.com/ttacon/chalk"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"

	_ "github.com/rancher/opni/plugins/alerting/test"
	_ "github.com/rancher/opni/plugins/logging/test"
	_ "github.com/rancher/opni/plugins/metrics/test"
)

func main() {
	gin.SetMode(gin.TestMode)
	var (
		enableGateway,
		enableEtcd,
		enableJetstream,
		enableNodeExporter bool
	)
	var remoteGatewayAddress string
	var agentIdSeed int64

	pflag.BoolVar(&enableGateway, "enable-gateway", true, "enable gateway")
	pflag.BoolVar(&enableEtcd, "enable-etcd", true, "enable etcd")
	pflag.BoolVar(&enableJetstream, "enable-jetstream", true, "enable jetstream")
	pflag.BoolVar(&enableNodeExporter, "enable-node-exporter", true, "enable node exporter")
	pflag.StringVar(&remoteGatewayAddress, "remote-gateway-address", "", "remote gateway address")
	pflag.Int64Var(&agentIdSeed, "agent-id-seed", 0, "random seed used for generating agent ids. if unset, uses a random seed.")

	pflag.Parse()

	if !pflag.Lookup("agent-id-seed").Changed {
		agentIdSeed = time.Now().UnixNano()
	}

	defaultAgentOpts := []test.StartAgentOption{}
	if remoteGatewayAddress != "" {
		fmt.Fprintln(os.Stdout, chalk.Blue.Color("disabling gateway and cortex since remote gateway address is set"))
		enableGateway = false
		defaultAgentOpts = append(defaultAgentOpts, test.WithRemoteGatewayAddress(remoteGatewayAddress))
	}

	tracing.Configure("testenv")

	options := []test.EnvironmentOption{
		test.WithEnableGateway(enableGateway),
		test.WithEnableEtcd(enableEtcd),
		test.WithEnableNodeExporter(enableNodeExporter),
		test.WithEnableJetstream(enableJetstream),
		test.WithDefaultAgentOpts(defaultAgentOpts...),
	}

	randSrc := rand.New(rand.NewSource(agentIdSeed))
	environment := &test.Environment{}
	var iPort int
	var kPort int
	var localAgentOnce sync.Once
	addAgent := func(rw http.ResponseWriter, r *http.Request) {
		testlog.Log.Infof("%s %s", r.Method, r.URL.Path)
		switch r.Method {
		case http.MethodPost:
			body := struct {
				Token string   `json:"token"`
				Pins  []string `json:"pins"`
				ID    string   `json:"id"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				rw.Write([]byte(err.Error()))
				return
			}
			if body.ID == "" {
				body.ID = util.Must(uuid.NewRandomFromReader(randSrc)).String()
			}
			token, err := tokens.ParseHex(body.Token)
			if err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				rw.Write([]byte(err.Error()))
				return
			}
			startOpts := slices.Clone(defaultAgentOpts)
			var isLocalAgent bool
			localAgentOnce.Do(func() {
				isLocalAgent = true
			})
			if isLocalAgent {
				startOpts = append(startOpts, test.WithLocalAgent())
			}
			port, errC := environment.StartAgent(body.ID, token.ToBootstrapToken(), body.Pins,
				append(startOpts, test.WithAgentVersion("v2"))...)
			if err := <-errC; err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				rw.Write([]byte(err.Error()))
				return
			}
			environment.SetPrometheusNodeConfigOverride(body.ID, func() *test.OverridePrometheusConfig {
				optional := []test.PrometheusJob{}
				if enableNodeExporter {
					optional = append(optional, test.PrometheusJob{
						JobName:    "node_exporter",
						ScrapePort: environment.GetPorts().NodeExporterPort,
					})
				}
				if enableGateway && isLocalAgent {
					optional = append(optional, test.PrometheusJob{
						JobName:     "opni-gateway",
						ScrapePort:  environment.GetPorts().GatewayMetrics,
						MetricsPath: "/metrics",
					})
				}
				return test.NewOverridePrometheusConfig("prometheus/config.yaml",
					append([]test.PrometheusJob{
						{
							JobName:    "payment_service",
							ScrapePort: iPort,
						},
						{
							JobName:    "kubernetes",
							ScrapePort: kPort,
						},
					}, optional...))
			}())

			rw.WriteHeader(http.StatusOK)
			rw.Write([]byte(fmt.Sprintf("%d", port)))
		default:
			rw.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
	http.HandleFunc("/agents", addAgent)
	if err := environment.Start(options...); err != nil {
		panic(err)
	}
	go func() {
		addr := fmt.Sprintf("127.0.0.1:%d", environment.GetPorts().TestEnvironment)
		testlog.Log.Infof(chalk.Green.Color("Test environment API listening on %s"), addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			panic(err)
		}
	}()
	c := make(chan os.Signal, 2)
	var closeOnce sync.Once
	signal.Notify(c, os.Interrupt)
	iPort, _ = environment.StartInstrumentationServer()
	kPort = environment.StartMockKubernetesMetricServer()
	for i := 0; i < 100; i++ {
		environment.SimulateKubeObject(kPort)
	}
	testlog.Log.Infof(chalk.Green.Color("Instrumentation server listening on %d"), iPort)
	var client managementv1.ManagementClient
	if enableGateway {
		client = environment.NewManagementClient()
	}

	showHelp := func() {
		testlog.Log.Infof(chalk.Green.Color("Kubernetes metric server listening on %d"), kPort)
		testlog.Log.Info(chalk.Blue.Color("Press (ctrl+c) or (q) to stop test environment"))
		if enableGateway {
			testlog.Log.Info(chalk.Blue.Color("Press (space) to open the web dashboard"))
		}
		if enableGateway {
			testlog.Log.Info(chalk.Blue.Color("Press (a) to launch a new agent"))
			testlog.Log.Info(chalk.Blue.Color("Press (M) to configure the metrics backend"))
			testlog.Log.Info(chalk.Blue.Color("Press (U) to uninstall the metrics backend"))
			testlog.Log.Info(chalk.Blue.Color("Press (m) to install the metrics capability on all agents"))
			testlog.Log.Info(chalk.Blue.Color("Press (u) to uninstall the metrics capability on all agents"))
			testlog.Log.Info(chalk.Blue.Color("Press (g) to run a Grafana container"))
			testlog.Log.Info(chalk.Blue.Color("Press (r) to configure sample rbac rules"))
			testlog.Log.Info(chalk.Blue.Color("Press (p)(i) to open the pprof index page"))
			testlog.Log.Info(chalk.Blue.Color("Press (p)(h) to open a pprof heap profile"))
			testlog.Log.Info(chalk.Blue.Color("Press (p)(a) to open a pprof allocs profile"))
			testlog.Log.Info(chalk.Blue.Color("Press (p)(p) to run and open a pprof profile"))
			testlog.Log.Info(chalk.Blue.Color("Press (i) to show runtime information"))
			testlog.Log.Info(chalk.Blue.Color("Press (h) to show this help message"))
		}
	}

	startDashboardOnce := sync.Once{}
	startDashboard := func() {
		startDashboardOnce.Do(func() {
			if !enableGateway {
				return
			}
			dashboardSrv, err := dashboard.NewServer(&environment.GatewayConfig().Spec.Management)
			if err != nil {
				testlog.Log.Error(err)
				return
			}
			go func() {
				dashboardSrv.ListenAndServe(environment.Context())
			}()
		})
	}

	var capabilityMu sync.Mutex
	var startedGrafana bool

	var pPressed bool
	handleKey := func(rn rune) {
		if pPressed {
			pPressed = false
			var path string
			switch rn {
			case 'i':
				go browser.OpenURL(fmt.Sprintf("http://localhost:%d/debug/pprof/", environment.GetPorts().TestEnvironment))
				return
			case 'h':
				path = "heap"
			case 'a':
				path = "allocs"
			case 'p':
				path = "profile"
			default:
				testlog.Log.Error("Invalid pprof command: %c", rn)
				return
			}
			url := fmt.Sprintf("http://localhost:%d/debug/pprof/%s", environment.GetPorts().TestEnvironment, path)
			port := freeport.GetFreePort()
			cmd := exec.CommandContext(environment.Context(), "go", "tool", "pprof", "-http", fmt.Sprintf("localhost:%d", port), url)
			session, err := testutil.StartCmd(cmd)
			if err != nil {
				testlog.Log.Error(err)
			} else {
				waitctx.Go(environment.Context(), func() {
					<-environment.Context().Done()
					session.Wait()
				})
			}
			testlog.Log.Infof("Starting pprof server on %s", url)
			return
		}

		switch rn {
		case ' ':
			startDashboard()
			go browser.OpenURL(fmt.Sprintf("http://localhost:%d", environment.GetPorts().ManagementWeb))
		case 'q':
			closeOnce.Do(func() {
				signal.Stop(c)
				close(c)
			})
		case 'a', 'A':
			go func() {
				bt, err := client.CreateBootstrapToken(environment.Context(), &managementv1.CreateBootstrapTokenRequest{
					Ttl: durationpb.New(1 * time.Minute),
				})
				if err != nil {
					testlog.Log.Error(err)
					return
				}
				token, err := tokens.FromBootstrapToken(bt)
				if err != nil {
					testlog.Log.Error(err)
					return
				}
				certInfo, err := client.CertsInfo(environment.Context(), &emptypb.Empty{})
				if err != nil {
					testlog.Log.Error(err)
					return
				}

				resp, err := http.Post(fmt.Sprintf("http://localhost:%d/agents", environment.GetPorts().TestEnvironment), "application/json",
					strings.NewReader(fmt.Sprintf(`{"token": "%s", "pins": ["%s"]}`,
						token.EncodeHex(), certInfo.Chain[len(certInfo.Chain)-1].Fingerprint)))
				if err != nil {
					testlog.Log.Error(err)
					return
				}
				if resp.StatusCode != http.StatusOK {
					testlog.Log.Errorf("%s", resp.Status)
					return
				}
			}()
		case 'M':
			capabilityMu.Lock()
			go func() {
				defer capabilityMu.Unlock()
				defer func() {
					testlog.Log.Info(chalk.Green.Color("Metrics backend configured"))
				}()
				opsClient := cortexops.NewCortexOpsClient(environment.ManagementClientConn())
				_, err := opsClient.ConfigureCluster(environment.Context(), &cortexops.ClusterConfiguration{
					Mode: cortexops.DeploymentMode_AllInOne,
					Storage: &storagev1.StorageSpec{
						Backend: "filesystem",
						Filesystem: &storagev1.FilesystemStorageSpec{
							Directory: path.Join(environment.GetTempDirectory(), "cortex", "data"),
						},
					},
				})
				if err != nil {
					testlog.Log.Error(err)
				}
			}()
		case 'U':
			capabilityMu.Lock()
			go func() {
				defer capabilityMu.Unlock()
				defer func() {
					testlog.Log.Info(chalk.Green.Color("Metrics backend uninstalled"))
				}()
				opsClient := cortexops.NewCortexOpsClient(environment.ManagementClientConn())
				_, err := opsClient.UninstallCluster(environment.Context(), &emptypb.Empty{})
				if err != nil {
					testlog.Log.Error(err)
				}
			}()
		case 'm':
			capabilityMu.Lock()
			go func() {
				defer capabilityMu.Unlock()
				clusters, err := client.ListClusters(environment.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					testlog.Log.Error(err)
					return
				}
				for _, cluster := range clusters.Items {
					_, err := client.InstallCapability(environment.Context(), &managementv1.CapabilityInstallRequest{
						Name: "metrics",
						Target: &v1.InstallRequest{
							Cluster:        cluster.Reference(),
							IgnoreWarnings: true,
						},
					})
					if err != nil {
						testlog.Log.Error(err)
					}
				}
			}()
		case 'u':
			capabilityMu.Lock()
			go func() {
				defer capabilityMu.Unlock()
				clusters, err := client.ListClusters(environment.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					testlog.Log.Error(err)
					return
				}
				for _, cluster := range clusters.Items {
					_, err := client.UninstallCapability(environment.Context(), &managementv1.CapabilityUninstallRequest{
						Name: "metrics",
						Target: &v1.UninstallRequest{
							Cluster: cluster.Reference(),
						},
					})
					if err != nil {
						testlog.Log.Error(err)
					}
				}
			}()
		case 'i':
			testlog.Log.Infof("Temp directory: %s", environment.GetTempDirectory())
			testlog.Log.Infof("Ports: %s", environment.GetTempDirectory())
			ports := environment.GetPorts()
			// print the field name and int value for each field (all ints)
			v := reflect.ValueOf(ports)
			for i := 0; i < v.NumField(); i++ {
				name := v.Type().Field(i).Name
				value := v.Field(i).Interface().(int)
				envVarName := v.Type().Field(i).Tag.Get("env")
				testlog.Log.Infof("  %s: %d (env: %s)", name, value, envVarName)
			}
		case 'p':
			pPressed = true
			testlog.Log.Info("'p' pressed, waiting for next key...")
		case 'g':
			if !startedGrafana {
				environment.WriteGrafanaConfig()
				environment.StartGrafana()
				startedGrafana = true
				testlog.Log.Info(chalk.Green.Color("Grafana started"))
			} else {
				testlog.Log.Error("Grafana already started")
			}
		case 'r':
			clusters, err := client.ListClusters(environment.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				testlog.Log.Error(err)
				return
			}
			if _, err := client.CreateRole(environment.Context(), &corev1.Role{
				Id: "testenv-role",
				MatchLabels: &corev1.LabelSelector{
					MatchLabels: map[string]string{
						"visible": "true",
					},
				},
			}); err != nil {
				testlog.Log.Error(err)
			}
			if _, err := client.CreateRoleBinding(environment.Context(), &corev1.RoleBinding{
				Id:       "testenv-rb",
				RoleId:   "testenv-role",
				Subjects: []string{"testenv"},
			}); err != nil {
				testlog.Log.Error(err)
			}
			for _, cluster := range clusters.Items {
				cluster.Metadata.Labels["visible"] = "true"
				if _, err := client.EditCluster(environment.Context(), &managementv1.EditClusterRequest{
					Cluster: cluster.Reference(),
					Labels:  cluster.GetLabels(),
				}); err != nil {
					testlog.Log.Error(err)
				}
			}
		case 'h':
			showHelp()
		}
	}

	// listen for spacebar on stdin
	t, err := tty.Open()
	if err == nil {
		showHelp()
		go func() {
			for {
				rn, err := t.ReadRune()
				if err != nil {
					testlog.Log.Panic(err)
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
						testlog.Log.Panic(err)
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
