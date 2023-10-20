package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"slices"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mattn/go-tty"
	"github.com/pkg/browser"
	v1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/dashboard"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/spf13/pflag"
	"github.com/ttacon/chalk"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	_ "github.com/rancher/opni/plugins/alerting/test"
	_ "github.com/rancher/opni/plugins/example/test"
	_ "github.com/rancher/opni/plugins/logging/test"
	_ "github.com/rancher/opni/plugins/metrics/test"
	_ "github.com/rancher/opni/plugins/slo/test"
)

func main() {
	gin.SetMode(gin.TestMode)
	var (
		enableGateway,
		enableEtcd,
		enableJetstream,
		enableNodeExporter,
		noEmbeddedWebAssets,
		headlessDashboard bool
	)
	var remoteGatewayAddress string
	var agentIdSeed int64

	pflag.BoolVar(&enableGateway, "enable-gateway", true, "enable gateway")
	pflag.BoolVar(&enableEtcd, "enable-etcd", true, "enable etcd")
	pflag.BoolVar(&enableJetstream, "enable-jetstream", true, "enable jetstream")
	pflag.BoolVar(&enableNodeExporter, "enable-node-exporter", true, "enable node exporter")
	pflag.StringVar(&remoteGatewayAddress, "remote-gateway-address", "", "remote gateway address")
	pflag.Int64Var(&agentIdSeed, "agent-id-seed", 0, "random seed used for generating agent ids. if unset, uses a random seed.")
	pflag.BoolVar(&noEmbeddedWebAssets, "no-embedded-web-assets", false, "serve web assets from web/dist instead of using embedded assets")
	pflag.BoolVar(&headlessDashboard, "headless-dashboard", false, "run the dashboard without opening a browser window")

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

	var agentCancelMu sync.Mutex
	var agentCancelFuncs []context.CancelFunc

	randSrc := rand.New(rand.NewSource(agentIdSeed))
	environment := &test.Environment{}
	var iPort int
	var kPort int
	var localAgentOnce sync.Once
	addAgent := func(rw http.ResponseWriter, r *http.Request) {
		testlog.Log.Info(fmt.Sprintf("%s %s", r.Method, r.URL.Path))
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
			agentCancelMu.Lock()
			ctx, ca := context.WithCancel(context.Background())
			agentCancelFuncs = append(agentCancelFuncs, ca)
			agentCancelMu.Unlock()
			port := freeport.GetFreePort()
			_, errC := environment.StartAgent(body.ID, token.ToBootstrapToken(), body.Pins,
				append(startOpts, test.WithAgentVersion("v2"), test.WithContext(ctx), test.WithListenPort(port))...)
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
		testlog.Log.Info(fmt.Sprintf(chalk.Green.Color("Test environment API listening on %s"), addr))
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
	testlog.Log.Info(fmt.Sprintf(chalk.Green.Color("Instrumentation server listening on %d"), iPort))
	var client managementv1.ManagementClient
	if enableGateway {
		client = environment.NewManagementClient()
	}

	showHelp := func() {
		testlog.Log.Info(fmt.Sprintf(chalk.Green.Color("Kubernetes metric server listening on %d"), kPort))
		testlog.Log.Info(chalk.Blue.Color("Press (ctrl+c) or (q) to stop test environment"))
		if enableGateway {
			testlog.Log.Info(chalk.Blue.Color("Press (space) to open the web dashboard"))
		}
		if enableGateway {
			testlog.Log.Info(chalk.Blue.Color("Press (a) to launch a new agent"))
			testlog.Log.Info(chalk.Blue.Color("Press (s) to stop an agent"))
			testlog.Log.Info(chalk.Blue.Color("Press (M) to configure the metrics backend"))
			testlog.Log.Info(chalk.Blue.Color("Press (U) to uninstall the metrics backend"))
			testlog.Log.Info(chalk.Blue.Color("Press (L) to configure the alerting backend"))
			testlog.Log.Info(chalk.Blue.Color("Press (O) to uninstall the alerting backend"))
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
			opts := []dashboard.ServerOption{}
			if noEmbeddedWebAssets {
				absPath, err := filepath.Abs("web/")
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
					return
				}
				fs := os.DirFS(absPath)
				opts = append(opts, dashboard.WithAssetsFS(fs))
			}
			dashboardSrv, err := dashboard.NewServer(&environment.GatewayConfig().Spec.Management, opts...)
			if err != nil {
				testlog.Log.Error("error", logger.Err(err))
				return
			}
			go func() {
				dashboardSrv.ListenAndServe(environment.Context())
			}()
		})
	}

	var capabilityMu sync.Mutex

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
				testlog.Log.Error(fmt.Sprintf("Invalid pprof command: %c", rn))
				return
			}
			url := fmt.Sprintf("http://localhost:%d/debug/pprof/%s", environment.GetPorts().TestEnvironment, path)
			port := freeport.GetFreePort()
			cmd := exec.CommandContext(environment.Context(), "go", "tool", "pprof", "-http", fmt.Sprintf("localhost:%d", port), url)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				testlog.Log.Error("error", logger.Err(err))
				return
			}
			go cmd.Wait()
			testlog.Log.Info(fmt.Sprintf("Starting pprof server on %s", url))
			return
		}

		switch rn {
		case ' ':
			startDashboard()
			if !headlessDashboard {
				go browser.OpenURL(fmt.Sprintf("http://localhost:%d", environment.GetPorts().ManagementWeb))
			}
		case 'q':
			closeOnce.Do(func() {
				signal.Stop(c)
				close(c)
			})
		case 'a':
			go func() {
				bt, err := client.CreateBootstrapToken(environment.Context(), &managementv1.CreateBootstrapTokenRequest{
					Ttl: durationpb.New(1 * time.Minute),
				})
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
					return
				}
				token, err := tokens.FromBootstrapToken(bt)
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
					return
				}
				certInfo, err := client.CertsInfo(environment.Context(), &emptypb.Empty{})
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
					return
				}

				resp, err := http.Post(fmt.Sprintf("http://localhost:%d/agents", environment.GetPorts().TestEnvironment), "application/json",
					strings.NewReader(fmt.Sprintf(`{"token": "%s", "pins": ["%s"]}`,
						token.EncodeHex(), certInfo.Chain[len(certInfo.Chain)-1].Fingerprint)))
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
					return
				}
				if resp.StatusCode != http.StatusOK {
					testlog.Log.Error(fmt.Sprintf("%s", resp.Status))
					return
				}
			}()
		case 's':
			go func() {
				agentCancelMu.Lock()
				defer agentCancelMu.Unlock()
				if len(agentCancelFuncs) == 0 {
					testlog.Log.Error("No agents to stop")
					return
				}
				testlog.Log.Info(fmt.Sprintf("Stopping agent %v", agentCancelFuncs[0]))
				agentCancelFuncs[0]()
				agentCancelFuncs = agentCancelFuncs[1:]
			}()
		case 'M':
			capabilityMu.Lock()
			go func() {
				defer capabilityMu.Unlock()
				defer func() {
					testlog.Log.Info(chalk.Green.Color("Metrics backend configured"))
				}()
				opsClient := cortexops.NewCortexOpsClient(environment.ManagementClientConn())
				presets, err := opsClient.ListPresets(context.Background(), &emptypb.Empty{})
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
					return
				}
				if len(presets.Items) == 0 {
					testlog.Log.Error("No presets available")
					return
				}
				// take the first preset
				preset := presets.Items[0]
				_, err = opsClient.SetConfiguration(context.Background(), preset.GetSpec())
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
					return
				}

				_, err = opsClient.Install(context.Background(), &emptypb.Empty{})
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
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
				_, err := opsClient.Uninstall(environment.Context(), &emptypb.Empty{})
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
				}
			}()
		case 'm':
			capabilityMu.Lock()
			go func() {
				defer capabilityMu.Unlock()
				clusters, err := client.ListClusters(environment.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
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
						testlog.Log.Error("error", logger.Err(err))
					}

					conditionsClient := environment.NewAlertConditionsClient()
					_, err = conditionsClient.CreateAlertCondition(environment.Context(), &alertingv1.AlertCondition{
						Name:        "sanity metrics",
						Description: "Metrics watchdog : fires when metrics agent is receiving metrics",
						Severity:    alertingv1.OpniSeverity_Info,
						AlertType: &alertingv1.AlertTypeDetails{
							Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
								PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
									ClusterId: cluster.Reference(),
									Query:     "sum(up > 0) > 0",
									For:       durationpb.New(time.Second * 1),
								},
							},
						},
						GoldenSignal: alertingv1.GoldenSignal_Errors,
					})
					if err != nil {
						testlog.Log.Error("error", logger.Err(err))
					}
				}
			}()
		case 'u':
			capabilityMu.Lock()
			go func() {
				defer capabilityMu.Unlock()
				clusters, err := client.ListClusters(environment.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
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
						testlog.Log.Error("error", logger.Err(err))
					}
				}
			}()
		case 'i':
			testlog.Log.Info(fmt.Sprintf("Temp directory: %s", environment.GetTempDirectory()))
			testlog.Log.Info(fmt.Sprintf("Ports: %s", environment.GetTempDirectory()))
			ports := environment.GetPorts()
			// print the field name and int value for each field (all ints)
			v := reflect.ValueOf(ports)
			for i := 0; i < v.NumField(); i++ {
				name := v.Type().Field(i).Name
				value := v.Field(i).Interface().(int)
				envVarName := v.Type().Field(i).Tag.Get("env")
				testlog.Log.Info(fmt.Sprintf("  %s: %d (env: %s)", name, value, envVarName))
			}
		case 'p':
			pPressed = true
			testlog.Log.Info("'p' pressed, waiting for next key...")
		case 'g':
			environment.WriteGrafanaConfig()
			environment.StartGrafana()
			testlog.Log.Info(chalk.Green.Color("Grafana started"))
		case 'r':
			clusters, err := client.ListClusters(environment.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				testlog.Log.Error("error", logger.Err(err))
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
				testlog.Log.Error("error", logger.Err(err))
			}
			if _, err := client.CreateRoleBinding(environment.Context(), &corev1.RoleBinding{
				Id:       "testenv-rb",
				RoleId:   "testenv-role",
				Subjects: []string{"testenv"},
			}); err != nil {
				testlog.Log.Error("error", logger.Err(err))
			}
			for _, cluster := range clusters.Items {
				cluster.Metadata.Labels["visible"] = "true"
				if _, err := client.EditCluster(environment.Context(), &managementv1.EditClusterRequest{
					Cluster: cluster.Reference(),
					Labels:  cluster.GetLabels(),
				}); err != nil {
					testlog.Log.Error("error", logger.Err(err))
				}
			}
		case 'h':
			showHelp()
		case 'L':
			opsClient := alertops.NewAlertingAdminClient(environment.ManagementClientConn())
			_, err := opsClient.InstallCluster(environment.Context(), &emptypb.Empty{})
			if err != nil {
				testlog.Log.Error("error", logger.Err(err))
			} else {
				_, err = opsClient.ConfigureCluster(environment.Context(), &alertops.ClusterConfiguration{
					NumReplicas:             3,
					ClusterSettleTimeout:    "5s",
					ClusterPushPullInterval: "200ms",
					ClusterGossipInterval:   "1s",
					ResourceLimits: &alertops.ResourceLimitSpec{
						Storage: "500Mi",
						Memory:  "500m",
						Cpu:     "500m",
					},
				})
				if err != nil {
					testlog.Log.Error("error", logger.Err(err))
				}
			}

		case 'O':
			opsClient := alertops.NewAlertingAdminClient(environment.ManagementClientConn())
			_, err := opsClient.UninstallCluster(environment.Context(), &alertops.UninstallRequest{
				DeleteData: true,
			})
			if err != nil {
				testlog.Log.Error("error", logger.Err(err))
			}
		}
	}

	// listen for spacebar on stdin
	t, err := tty.Open()
	defer t.Close()
	if err == nil {
		showHelp()
		go func() {
			for {
				rn, err := t.ReadRune()
				if err != nil {
					panic(err)
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
						panic(err)
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
