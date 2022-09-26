package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/grafana/cortex-tools/pkg/bench"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/ini.v1"
	k8scorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Monitoring Test", Ordered, Label("e2e", "slow"), func() {
	var ctx context.Context
	var agentId string
	var s3Client *s3.S3
	BeforeAll(func() {
		ctx = context.Background()
		agentId = uuid.NewString()
		s := util.Must(session.NewSession())
		s3Client = s3.New(s, &aws.Config{
			Region:      aws.String(outputs.S3Region),
			Credentials: credentials.NewStaticCredentials(outputs.S3AccessKeyId, outputs.S3SecretAccessKey, ""),
		})
		cortexOpsClient.ConfigureCluster(ctx, &cortexops.ClusterConfiguration{
			Mode: cortexops.DeploymentMode_HighlyAvailable,
			Storage: &storagev1.StorageSpec{
				// Backend:    "filesystem",
				// Filesystem: &storagev1.FilesystemStorageSpec{},
				Backend: "s3",
				S3: &storagev1.S3StorageSpec{
					Endpoint:        outputs.S3Endpoint,
					Region:          outputs.S3Region,
					BucketName:      outputs.S3Bucket,
					SecretAccessKey: outputs.S3SecretAccessKey,
					AccessKeyID:     outputs.S3AccessKeyId,
				},
			},
			Grafana: &cortexops.GrafanaConfig{
				Enabled:  true,
				Hostname: outputs.GrafanaURL,
			},
		})
		Eventually(func() error {
			installStatus, err := cortexOpsClient.GetClusterStatus(ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}
			if installStatus.State != cortexops.InstallState_Installed {
				return fmt.Errorf("cortex has not fisnished installing yet")
			}
			return nil
		}, 20*time.Minute, 10*time.Second).Should(Succeed())
	})

	Specify("grafana should be configured correctly when cortex is deployed", func() {
		var grafanaConfig k8scorev1.ConfigMap

		err := k8sClient.Get(context.Background(), types.NamespacedName{
			Name:      "grafana-config",
			Namespace: "opni",
		}, &grafanaConfig)

		Expect(err).NotTo(HaveOccurred())
		data := grafanaConfig.Data["grafana.ini"]

		f, err := ini.Load([]byte(data))
		Expect(err).NotTo(HaveOccurred())

		genericOauth, err := f.GetSection("auth.generic_oauth")
		Expect(err).NotTo(HaveOccurred())

		wkc, err := (&openid.OpenidConfig{
			Discovery: &openid.DiscoverySpec{
				Issuer: outputs.OAuthIssuerURL,
			},
		}).GetWellKnownConfiguration()
		Expect(err).NotTo(HaveOccurred())

		kh := genericOauth.KeysHash()
		Expect(kh).To(HaveKeyWithValue("auth_url", wkc.AuthEndpoint))
		Expect(kh).To(HaveKeyWithValue("token_url", wkc.TokenEndpoint))
		Expect(kh).To(HaveKeyWithValue("api_url", wkc.UserinfoEndpoint))
		Expect(kh).To(HaveKeyWithValue("client_id", outputs.OAuthClientID))
		Expect(kh).To(HaveKeyWithValue("client_secret", outputs.OAuthClientSecret))
		Expect(kh).To(HaveKeyWithValue("enabled", "true"))
		Expect(strings.Fields(kh["scopes"])).To(ContainElements("openid", "profile", "email"))

		// This is a very important test: role_attribute_path is a JMESPath expression
		// used to extract a user's grafana role (Viewer, Editor, or Admin) from their
		// ID token. JMESPath has several characters that need to be escaped, one
		// of which is ':' (colon). Many identity providers require custom user
		// attributes to be prefixed with some string such as 'custom:' (cognito)
		// or a URL with a scheme (auth0), both of which contain ':' characters.
		// Unfortunately the only way to do escape sequences in JMESPath is to
		// surround the expression with *double* quotes. This presents a small
		// challenge since the user must configure a string such that when converted
		// from yaml to ini and parsed by the go ini library, the resulting string
		// contains proper double quote characters. If we can parse the grafana
		// ini config using the same library and end up with the correct expression,
		// then we know that the string set in the initial gateway config is correct.
		Expect(kh).To(HaveKeyWithValue("role_attribute_path", `"custom:grafana_role"`))

		server, err := f.GetSection("server")
		Expect(err).NotTo(HaveOccurred())

		kh = server.KeysHash()
		grafanaHostname, err := url.Parse(outputs.GrafanaURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(kh).To(HaveKeyWithValue("domain", grafanaHostname.Host))
		Expect(kh).To(HaveKeyWithValue("root_url", outputs.GrafanaURL))
	})

	It("should start a new agent", func() {
		By("starting a new agent")
		token, err := mgmtClient.CreateBootstrapToken(ctx, &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		certs, err := mgmtClient.CertsInfo(ctx, &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		fp := certs.Chain[len(certs.Chain)-1].Fingerprint

		port, errC := testEnv.StartAgent(agentId, token, []string{fp}, test.WithAgentVersion("v2"))
		Eventually(errC).Should(Receive(BeNil()))

		By("starting a new prometheus")
		testEnv.StartPrometheus(port)

		By("starting a new metrics writer")
		benchRunner, err := testEnv.NewBenchRunner(agentId, bench.WorkloadDesc{
			Replicas: 1,
			Series: []bench.SeriesDesc{
				{
					Name: "bench_test1",
					Type: bench.GaugeRandom,
					StaticLabels: map[string]string{
						"test": "monitoring_test",
					},
					Labels: []bench.LabelDesc{
						{
							Name:         "label1",
							ValuePrefix:  "value1",
							UniqueValues: 50,
						},
						{
							Name:         "label2",
							ValuePrefix:  "value2",
							UniqueValues: 50,
						},
					},
				},
			},
			QueryDesc: []bench.QueryDesc{},
			Write: bench.WriteDesc{
				Interval:  1 * time.Second,
				Timeout:   1 * time.Minute,
				BatchSize: 1000,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		benchRunner.StartWorker(ctx)
	})
	It("should become healthy", func() {
		Eventually(func() string {
			hs, err := mgmtClient.GetClusterHealthStatus(ctx, &corev1.Reference{
				Id: agentId,
			})
			if err != nil {
				return err.Error()
			}
			if !hs.Status.Connected {
				return "not connected"
			}
			if len(hs.Health.Conditions) > 0 {
				return strings.Join(hs.Health.Conditions, ", ")
			}
			if !hs.Health.Ready {
				return "not ready"
			}
			return "ok"
		}, 30*time.Second, 1*time.Second).Should(Equal("ok"))
	})
	It("should enable the metrics capability", func() {
		By("sending a capability install request to the agent")
		_, err := mgmtClient.InstallCapability(ctx, &managementv1.CapabilityInstallRequest{
			Name: "metrics",
			Target: &capabilityv1.InstallRequest{
				Cluster: &corev1.Reference{Id: agentId},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			status, err := mgmtClient.GetCluster(ctx, &corev1.Reference{Id: agentId})
			if err != nil {
				return err
			}
			for _, c := range status.Metadata.Capabilities {
				if c.Name == "metrics" && c.DeletionTimestamp == nil {
					return nil
				}
			}
			return fmt.Errorf("metrics capability not installed within time limit")
		}, time.Minute*20, time.Second*10).Should(Succeed())
	})

	It("should query metrics", func() {
		By("sleeping for 10 seconds")
		time.Sleep(10 * time.Second)

		By("querying metrics")
		resp, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
			Tenants: []string{agentId},
			Query:   "count(bench_test1)",
		})
		Expect(err).NotTo(HaveOccurred())
		result, err := unmarshal.UnmarshalPrometheusResponse(resp.Data)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Type).To(Equal(model.ValVector))
		Expect(int(result.V.(model.Vector)[0].Value)).To(Equal(2500))
	})

	It("Should be able to create recording rules", func() {
		ruleTestDataDir := "../../pkg/test/testdata/slo/cortexrule"
		sampleRule := fmt.Sprintf("%s/sampleRule.yaml", ruleTestDataDir)
		sampleRuleYamlBytes, err := ioutil.ReadFile(sampleRule)
		Expect(err).To(Succeed())
		_, err = adminClient.LoadRules(ctx, &cortexadmin.PostRuleRequest{
			YamlContent: string(sampleRuleYamlBytes),
			ClusterId:   agentId,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should write metrics to long-term storage", func() {
		By("flushing ingesters")
		_, err := adminClient.FlushBlocks(ctx, &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		By("ensuring blocks have been written to long-term storage")

		Eventually(func() ([]string, error) {
			resp, err := s3Client.ListObjectsV2(&s3.ListObjectsV2Input{
				Bucket: aws.String(outputs.S3Bucket),
			})
			if err != nil {
				return nil, err
			}
			keys := []string{}
			for _, obj := range resp.Contents {
				keys = append(keys, *obj.Key)
			}
			return keys, nil
		}, 1*time.Minute, 1*time.Second).Should(ContainElement(HavePrefix(agentId)))
	})

	It("should uninstall the metrics capability from the downstream agent", func() {
		By("listing what capabilities need to be uninstalled")
		status, err := mgmtClient.GetCluster(ctx, &corev1.Reference{Id: agentId})
		Expect(err).NotTo(HaveOccurred())

		By("sending a capability uninstall request to the agent")
		for _, c := range status.Metadata.Capabilities {
			if c.DeletionTimestamp == nil {
				mgmtClient.UninstallCapability(ctx, &managementv1.CapabilityUninstallRequest{
					Name: c.Name,
					Target: &capabilityv1.UninstallRequest{
						Cluster: &corev1.Reference{Id: agentId},
						Options: `{"initialDelay":"1s","deleteStoredData":"true"}`,
					},
				})
			}
		}
		By("checking that the capabilities are in fact deleted")
		Eventually(func() error {
			status, err := mgmtClient.GetCluster(ctx, &corev1.Reference{Id: agentId})
			if err != nil {
				return err
			}
			if len(status.Metadata.Capabilities) > 0 {
				return fmt.Errorf("metrics capability not uninstalled within time limit")
			}
			return nil
		}, time.Second*120, time.Second*2).Should(Succeed())

		By("ensuring blocks have been deleted from long-term storage")
		Eventually(func() error {
			resp, err := s3Client.ListObjectsV2(&s3.ListObjectsV2Input{
				Bucket: aws.String(outputs.S3Bucket),
			})
			if err != nil {
				return err
			}

			for _, obj := range resp.Contents {
				k := *obj.Key
				if strings.HasPrefix(k, agentId) {
					// It should only contain debug/metas/* and markers/tenant-deletion-mark.json
					if strings.HasPrefix(k, agentId+"/debug/metas/") ||
						strings.HasPrefix(k, agentId+"/markers/tenant-deletion-mark.json") {
						continue
					}
					return fmt.Errorf("expected key to have been deleted: %s", k)
				}
			}
			return nil
		}, 1*time.Minute, 1*time.Second).Should(Succeed())
	})

	// Retained for reference for v1 of the agent, but should not be used in v2
	XIt("should uninstall the metrics capability", func() {
		getTaskState := func() (corev1.TaskState, error) {
			stat, err := mgmtClient.CapabilityUninstallStatus(ctx, &managementv1.CapabilityStatusRequest{
				Name: wellknown.CapabilityMetrics,
				Cluster: &corev1.Reference{
					Id: agentId,
				},
			})
			if err != nil {
				return 0, err
			}
			return stat.State, nil
		}

		By("starting the uninstall")
		_, err := mgmtClient.UninstallCapability(ctx, &managementv1.CapabilityUninstallRequest{
			Name: wellknown.CapabilityMetrics,
			Target: &capabilityv1.UninstallRequest{
				Cluster: &corev1.Reference{
					Id: agentId,
				},
				Options: `{"initialDelay":"10m","deleteStoredData":"true"}`,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(getTaskState, 1*time.Minute, 1*time.Second).Should(Equal(task.StatePending))
		Consistently(getTaskState, 2*time.Second, 100*time.Millisecond).Should(Equal(task.StatePending))

		By("canceling the uninstall")
		_, err = mgmtClient.CancelCapabilityUninstall(ctx, &managementv1.CapabilityUninstallCancelRequest{
			Name: wellknown.CapabilityMetrics,
			Cluster: &corev1.Reference{
				Id: agentId,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(getTaskState, 1*time.Minute, 1*time.Second).Should(Equal(task.StateCanceled))

		By("restarting the uninstall")
		_, err = mgmtClient.UninstallCapability(ctx, &managementv1.CapabilityUninstallRequest{
			Name: wellknown.CapabilityMetrics,
			Target: &capabilityv1.UninstallRequest{
				Cluster: &corev1.Reference{
					Id: agentId,
				},
				Options: `{"initialDelay":"1s","deleteStoredData":"true"}`,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(getTaskState, 1*time.Minute, 1*time.Second).Should(Equal(task.StateRunning))
		Eventually(getTaskState, 20*time.Minute, 10*time.Second).Should(Equal(task.StateCompleted))

		By("ensuring blocks have been deleted from long-term storage")
		Eventually(func() error {
			resp, err := s3Client.ListObjectsV2(&s3.ListObjectsV2Input{
				Bucket: aws.String(outputs.S3Bucket),
			})
			if err != nil {
				return err
			}

			for _, obj := range resp.Contents {
				k := *obj.Key
				if strings.HasPrefix(k, agentId) {
					// It should only contain debug/metas/* and markers/tenant-deletion-mark.json
					if strings.HasPrefix(k, agentId+"/debug/metas/") ||
						strings.HasPrefix(k, agentId+"/markers/tenant-deletion-mark.json") {
						continue
					}
					return fmt.Errorf("expected key to have been deleted: %s", k)
				}
			}
			return nil
		}, 1*time.Minute, 1*time.Second).Should(Succeed())
	})
	It("should delete the cluster", func() {
		_, err := mgmtClient.DeleteCluster(ctx, &corev1.Reference{
			Id: agentId,
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
