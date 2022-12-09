package integration_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/gmeasure"
	"github.com/phayes/freeport"
	"github.com/prometheus/procfs"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/tokens"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"sigs.k8s.io/yaml"
)

func buildExamplePlugin(destDir string) error {
	test.Log.Debug("building new example plugin...")
	cmd := exec.Command("go", "build", "-o", path.Join(destDir, "/plugin_example"),
		"-ldflags", fmt.Sprintf("-w -s -X github.com/rancher/opni/pkg/util.BuildTime=\"%d\"", time.Now().UnixNano()),
		"./plugins/example",
	)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	cmd.Dir = "../../.."
	return cmd.Run()
}

func buildMinimalAgent() (string, error) {
	test.Log.Info("building minimal agent...")
	return gexec.BuildWithEnvironment("github.com/rancher/opni/cmd/opni", []string{"CGO_ENABLED=0"},
		"-tags=noagentv1,noplugins,nohooks,norealtime,nocortex,nodebug,noevents,nogateway,noscheme_thirdparty,nomsgpack",
	)
}

// this test takes approx. 2 minutes
var _ = Describe("Agent Memory Tests", Ordered, Label("aberrant", "temporal"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	var fingerprint string
	var gatewayConfig *v1beta1.GatewayConfig
	var agentSession *gexec.Session
	var gatewaySession *gexec.Session
	var startGateway func()
	agentListenPort := freeport.GetPort()

	waitForGatewayReady := func(timeout time.Duration) {
		// ping /healthz until it returns 200
		test.Log.Debug("waiting for gateway to be ready")
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		for ctx.Err() == nil {
			resp, err := http.Get(fmt.Sprintf("http://%s/healthz", gatewayConfig.Spec.MetricsListenAddress))
			if err == nil && resp.StatusCode == 200 {
				return
			} else if err == nil {
				test.Log.Debug(fmt.Sprintf("gateway not ready yet: %s", resp.Status))
			} else {
				test.Log.Debug(fmt.Sprintf("gateway not ready yet: %s", err.Error()))
			}
			time.Sleep(50 * time.Millisecond)
		}
		Fail("timed out waiting for gateway to be ready")
	}

	waitForAgentReady := func(timeout time.Duration) {
		// ping /healthz until it returns 200
		test.Log.Debug("waiting for agent to be ready")
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		for ctx.Err() == nil {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", agentListenPort))
			if err == nil && resp.StatusCode == 200 {
				return
			} else if err == nil {
				status, _ := io.ReadAll(resp.Body)
				test.Log.Debug(fmt.Sprintf("agent not ready yet: %s", string(status)))
			}
			time.Sleep(200 * time.Millisecond)
		}
		Fail("timed out waiting for agent to be ready")
	}

	waitForAgentUnready := func(timeout time.Duration) {
		test.Log.Debug("waiting for agent to become unready")
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		for ctx.Err() == nil {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", agentListenPort))
			if err != nil || resp.StatusCode != 200 {
				return
			}
			test.Log.Debug("agent still ready...")
			time.Sleep(200 * time.Millisecond)
		}
		Fail("timed out waiting for agent to become unready")
	}

	BeforeAll(func() {
		minimalAgentBin, err := buildMinimalAgent()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			gexec.CleanupBuildArtifacts()
		})

		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(environment.Start(test.WithEnableGateway(false))).To(Succeed())

		DeferCleanup(environment.Stop)
		tempDir, err := os.MkdirTemp("", "opni-test")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			os.RemoveAll(tempDir)
		})
		os.Mkdir(path.Join(tempDir, "plugins"), 0755)

		gatewayConfig = environment.NewGatewayConfig()
		gatewayConfig.Spec.Plugins = v1beta1.PluginsSpec{
			Dir: path.Join(tempDir, "plugins"),
			Cache: v1beta1.CacheSpec{
				PatchEngine: v1beta1.PatchEngineBsdiff,
				Backend:     v1beta1.CacheBackendFilesystem,
				Filesystem: v1beta1.FilesystemCacheSpec{
					Dir: path.Join(tempDir, "cache"),
				},
			},
		}
		configData, err := yaml.Marshal(gatewayConfig)
		Expect(err).NotTo(HaveOccurred())
		configFile := path.Join(tempDir, "config.yaml")
		Expect(os.WriteFile(configFile, configData, 0644)).To(Succeed())

		startGateway = func() {
			buildExamplePlugin(path.Join(tempDir, "plugins"))

			cmd := exec.Command("bin/opni", "gateway", "--config", configFile)
			cmd.Dir = "../../../"
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid: true,
			}
			gatewaySession, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			waitForGatewayReady(10 * time.Second)
			DeferCleanup(func() {
				gatewaySession.Terminate().Wait()
			})
		}

		By("starting an external gateway process", startGateway)

		By("starting an external agent process", func() {
			client, err = clients.NewManagementClient(environment.Context(), clients.WithAddress(
				strings.TrimPrefix(gatewayConfig.Spec.Management.GRPCListenAddress, "tcp://"),
			), clients.WithDialOptions(grpc.WithBlock(), grpc.FailOnNonTempDialError(false)))
			Expect(err).NotTo(HaveOccurred())

			certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
			Expect(fingerprint).NotTo(BeEmpty())

			token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())
			t, err := tokens.FromBootstrapToken(token)
			Expect(err).NotTo(HaveOccurred())

			tempDir, err := os.MkdirTemp("", "opni-test")
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() {
				os.RemoveAll(tempDir)
			})
			agentConfig := &v1beta1.AgentConfig{
				TypeMeta: meta.TypeMeta{
					APIVersion: "v1beta1",
					Kind:       "AgentConfig",
				},
				Spec: v1beta1.AgentConfigSpec{
					TrustStrategy:    v1beta1.TrustStrategyPKP,
					ListenAddress:    fmt.Sprintf("localhost:%d", agentListenPort),
					GatewayAddress:   gatewayConfig.Spec.GRPCListenAddress,
					IdentityProvider: "env",
					Storage: v1beta1.StorageSpec{
						Type: v1beta1.StorageTypeEtcd,
						Etcd: environment.EtcdConfig(),
					},
					Bootstrap: &v1beta1.BootstrapSpec{
						Token: t.EncodeHex(),
						Pins:  []string{fingerprint},
					},
					Plugins: v1beta1.PluginsSpec{
						Dir: path.Join(tempDir, "plugins"),
						Cache: v1beta1.CacheSpec{
							Backend: v1beta1.CacheBackendFilesystem,
							Filesystem: v1beta1.FilesystemCacheSpec{
								Dir: path.Join(tempDir, "cache"),
							},
						},
					},
				},
			}
			configFile := path.Join(tempDir, "config.yaml")
			configData, err := yaml.Marshal(agentConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(configFile, configData, 0644)).To(Succeed())

			cmd := exec.Command(minimalAgentBin, "agentv2", "--config", configFile)
			cmd.Dir = "../../../"
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid: true,
			}
			cmd.Env = append(os.Environ(), "OPNI_UNIQUE_IDENTIFIER=agent1")
			agentSession, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				agentSession.Terminate().Wait()
			})
		})
	})
	Specify("watching agent memory usage", func() {
		var rssValues []int
		exp := gmeasure.NewExperiment("agent rss")
		for i := 0; i < 10; i++ {
			waitForAgentReady(2 * time.Minute)
			time.Sleep(1 * time.Second) // wait for the agent to settle

			pid := agentSession.Command.Process.Pid
			proc, err := procfs.NewProc(pid)
			Expect(err).NotTo(HaveOccurred())

			status, err := proc.Stat()
			Expect(err).NotTo(HaveOccurred())

			rssKB := status.RSS * os.Getpagesize() / 1024
			test.Log.Info("agent rss", " rssKB ", rssKB)
			exp.RecordValue("rss", float64(rssKB), gmeasure.Units("KB"))
			rssValues = append(rssValues, rssKB)

			gatewaySession.Terminate().Wait()
			waitForAgentUnready(10 * time.Second)
			By("restarting the gateway", startGateway)
		}

		AddReportEntry(exp.Name, exp)

		// check that the memory usage is not monotonically increasing
		var changeOverTime []int
		test.Log.Debugf("rss 0: %d", rssValues[0])
		for i := 1; i < len(rssValues); i++ {
			diff := rssValues[i] - rssValues[i-1]
			changeOverTime = append(changeOverTime, diff)
			if diff >= 0 {
				test.Log.Debugf("rss %d: %d (+%d)", i, rssValues[i], diff)
			} else {
				test.Log.Debugf("rss %d: %d (%d)", i, rssValues[i], diff)
			}
		}
		Expect(changeOverTime).To(ContainElement(BeNumerically("<", 0)), "memory usage should not be monotonically increasing")
	})
})
