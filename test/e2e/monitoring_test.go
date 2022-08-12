package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
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
	v1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Monitoring Test", Ordered, Label("e2e", "slow"), func() {
	var agentId string
	var s3Client *s3.S3
	BeforeAll(func() {
		agentId = uuid.NewString()
		s := util.Must(session.NewSession())
		s3Client = s3.New(s, &aws.Config{
			Region:      aws.String(outputs.S3Region),
			Credentials: credentials.NewStaticCredentials(outputs.S3AccessKeyId, outputs.S3SecretAccessKey, ""),
		})
	})
	It("should start a new agent", func() {
		By("starting a new agent")
		token, err := mgmtClient.CreateBootstrapToken(context.Background(), &v1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		certs, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		fp := certs.Chain[len(certs.Chain)-1].Fingerprint

		port, errC := testEnv.StartAgent(agentId, token, []string{fp})
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
		benchRunner.StartWorker(context.Background())
	})
	It("should become healthy", func() {
		Eventually(func() string {
			hs, err := mgmtClient.GetClusterHealthStatus(context.Background(), &corev1.Reference{
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
	It("should query metrics", func() {
		By("sleeping for 10 seconds")
		time.Sleep(10 * time.Second)

		By("querying metrics")
		resp, err := adminClient.Query(context.Background(), &cortexadmin.QueryRequest{
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
		_, err = adminClient.LoadRules(context.Background(), &cortexadmin.PostRuleRequest{
			YamlContent: string(sampleRuleYamlBytes),
			ClusterId:   agentId,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should write metrics to long-term storage", func() {
		By("flushing ingesters")
		_, err := adminClient.FlushBlocks(context.Background(), &emptypb.Empty{})
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
	It("should uninstall the metrics capability", func() {
		getTaskState := func() (corev1.TaskState, error) {
			stat, err := mgmtClient.CapabilityUninstallStatus(context.Background(), &v1.CapabilityStatusRequest{
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
		_, err := mgmtClient.UninstallCapability(context.Background(), &v1.CapabilityUninstallRequest{
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
		_, err = mgmtClient.CancelCapabilityUninstall(context.Background(), &v1.CapabilityUninstallCancelRequest{
			Name: wellknown.CapabilityMetrics,
			Cluster: &corev1.Reference{
				Id: agentId,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(getTaskState, 1*time.Minute, 1*time.Second).Should(Equal(task.StateCanceled))

		By("restarting the uninstall")
		_, err = mgmtClient.UninstallCapability(context.Background(), &v1.CapabilityUninstallRequest{
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
		_, err := mgmtClient.DeleteCluster(context.Background(), &corev1.Reference{
			Id: agentId,
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
