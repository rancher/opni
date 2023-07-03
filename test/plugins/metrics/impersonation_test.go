package metrics_test

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/golang/snappy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/metrics"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Tenant Impersonation", Ordered, Label("integration"), func() {
	var (
		agent1RwClient    remote.WriteClient
		agent2RwClient    remote.WriteClient
		cortexAdminClient cortexadmin.CortexAdminClient
	)
	var store = func(client remote.WriteClient, req *prompb.WriteRequest) error {
		wrData, err := req.Marshal()
		Expect(err).NotTo(HaveOccurred())

		compressed := snappy.Encode(nil, wrData)
		return client.Store(context.Background(), compressed)
	}
	var queryVec = func(tenants []string, promql string) (model.Vector, error) {
		resp, err := cortexAdminClient.Query(context.Background(), &cortexadmin.QueryRequest{
			Tenants: tenants,
			Query:   promql,
		})
		if err != nil {
			return nil, err
		}
		response, err := compat.UnmarshalPrometheusResponse(resp.Data)
		if err != nil {
			return nil, err
		}
		v, err := response.GetVector()
		if err != nil {
			return nil, err
		}
		return *v, nil
	}
	var env *test.Environment
	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env.Start()).To(Succeed())

		mgmtClient := env.NewManagementClient()

		cortexAdminClient = cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
		cortexOpsClient := cortexops.NewCortexOpsClient(env.ManagementClientConn())

		token, err := mgmtClient.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		certs, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		fp := certs.Chain[len(certs.Chain)-1].Fingerprint

		agent1Port, errC := env.StartAgent("agent1", token, []string{fp}, test.WithAgentVersion("v2"), test.WithLocalAgent())
		Eventually(errC).Should(Receive(BeNil()))

		agent2Port, errC := env.StartAgent("agent2", token, []string{fp}, test.WithAgentVersion("v2"))
		Eventually(errC).Should(Receive(BeNil()))

		_, err = cortexOpsClient.ConfigureCluster(context.Background(), &cortexops.ClusterConfiguration{
			Mode: cortexops.DeploymentMode_AllInOne,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = mgmtClient.InstallCapability(context.Background(), &managementv1.CapabilityInstallRequest{
			Name: "metrics",
			Target: &capabilityv1.InstallRequest{
				Cluster: &corev1.Reference{Id: "agent1"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = mgmtClient.InstallCapability(context.Background(), &managementv1.CapabilityInstallRequest{
			Name: "metrics",
			Target: &capabilityv1.InstallRequest{
				Cluster: &corev1.Reference{Id: "agent2"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		agent1RwClient, err = remote.NewWriteClient("agent1", &remote.ClientConfig{
			URL:     &config.URL{util.Must(url.Parse(fmt.Sprintf("http://127.0.0.1:%d/api/agent/push", agent1Port)))},
			Timeout: model.Duration(time.Second * 10),
		})
		Expect(err).NotTo(HaveOccurred())

		agent2RwClient, err = remote.NewWriteClient("agent2", &remote.ClientConfig{
			URL:     &config.URL{util.Must(url.Parse(fmt.Sprintf("http://127.0.0.1:%d/api/agent/push", agent2Port)))},
			Timeout: model.Duration(time.Second * 10),
		})
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(env.Stop)
	})
	When("timeseries are pushed to the remote write service", func() {
		When("the client has the required session attribute", func() {
			When("a timeseries in the request contains the tenant impersonation label", func() {
				It("should relabel and federate metrics to the impersonated tenant", func() {
					write := func() error {
						now := time.Now().UnixMilli()
						return store(agent1RwClient, &prompb.WriteRequest{
							Timeseries: []prompb.TimeSeries{
								{
									Labels: []prompb.Label{
										{Name: "__name__", Value: "opni_test_metric_1"},
										{Name: "example", Value: "a"},
										{Name: metrics.LabelImpersonateAs, Value: "agent1"},
									},
									Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
								},
								{
									Labels: []prompb.Label{
										{Name: "__name__", Value: "opni_test_metric_2"},
										{Name: "example", Value: "b"},
										// {Name: metrics.LabelImpersonateAs, Value: "agent1"}, // no impersonation label
									},
									Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
								},

								{
									Labels: []prompb.Label{
										{Name: "__name__", Value: "opni_test_metric_1"},
										{Name: "example", Value: "b"},
										{Name: metrics.LabelImpersonateAs, Value: "agent2"},
									},
									Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
								},
								{
									Labels: []prompb.Label{
										{Name: "__name__", Value: "opni_test_metric_2"},
										{Name: "example", Value: "b"},
										{Name: metrics.LabelImpersonateAs, Value: "agent2"},
									},
									Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
								},

								{
									Labels: []prompb.Label{
										{Name: "__name__", Value: "opni_test_metric_1"},
										{Name: "example", Value: "c"},
										{Name: metrics.LabelImpersonateAs, Value: "agent3"}, // note: agent3 does not exist (this is ok)
									},
									Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
								},
							},
						})
					}
					Eventually(write).Should(Succeed())
					_ = agent2RwClient

					Eventually(func() error {
						vec, err := queryVec([]string{"agent1"}, "opni_test_metric_1")
						if err != nil {
							return err
						}
						if len(vec) == 0 {
							return fmt.Errorf("no metrics found")
						}
						Expect(vec).To(HaveLen(1))
						Expect(vec[0].Metric).To(BeEquivalentTo(model.Metric{
							"__name__":      "opni_test_metric_1",
							"__tenant_id__": "agent1",
							"example":       "a",
						}))
						Expect(vec[0].Value).To(BeEquivalentTo(1))

						return nil
					}).Should(Succeed())

					Eventually(func() error {
						vec, err := queryVec([]string{"agent1"}, "opni_test_metric_2")
						if err != nil {
							return err
						}
						if len(vec) == 0 {
							return fmt.Errorf("no metrics found")
						}
						Expect(vec).To(HaveLen(1))
						Expect(vec[0].Metric).To(BeEquivalentTo(model.Metric{
							"__name__":      "opni_test_metric_2",
							"__tenant_id__": "agent1",
							"example":       "b",
						}))
						Expect(vec[0].Value).To(BeEquivalentTo(1))

						return nil
					}).Should(Succeed())

					Eventually(func() error {
						vec, err := queryVec([]string{"agent2"}, "opni_test_metric_1")
						if err != nil {
							return err
						}
						if len(vec) == 0 {
							return fmt.Errorf("no metrics found")
						}
						Expect(vec).To(HaveLen(1))
						Expect(vec[0].Metric).To(BeEquivalentTo(model.Metric{
							"__name__":      "opni_test_metric_1",
							"__tenant_id__": "agent2",
							"example":       "b",
						}))
						Expect(vec[0].Value).To(BeEquivalentTo(1))

						return nil
					}).Should(Succeed())

					Eventually(func() error {
						vec, err := queryVec([]string{"agent2"}, "opni_test_metric_2")
						if err != nil {
							return err
						}
						if len(vec) == 0 {
							return fmt.Errorf("no metrics found")
						}
						Expect(vec).To(HaveLen(1))
						Expect(vec[0].Metric).To(BeEquivalentTo(model.Metric{
							"__name__":      "opni_test_metric_2",
							"__tenant_id__": "agent2",
							"example":       "b",
						}))
						Expect(vec[0].Value).To(BeEquivalentTo(1))

						return nil
					}).Should(Succeed())

					Eventually(func() error {
						vec, err := queryVec([]string{"agent3"}, "opni_test_metric_1")
						if err != nil {
							return err
						}
						if len(vec) == 0 {
							return fmt.Errorf("no metrics found")
						}
						Expect(vec).To(HaveLen(1))
						Expect(vec[0].Metric).To(BeEquivalentTo(model.Metric{
							"__name__":      "opni_test_metric_1",
							"__tenant_id__": "agent3",
							"example":       "c",
						}))
						Expect(vec[0].Value).To(BeEquivalentTo(1))

						return nil
					}).Should(Succeed())

				})
			})
			When("a timeseries in the request does not contain the tenant impersonation label", func() {
				It("should push the timeseries to the client's own tenant", func() {
					now := time.Now().UnixMilli()
					err := store(agent1RwClient, &prompb.WriteRequest{
						Timeseries: []prompb.TimeSeries{
							{
								Labels: []prompb.Label{
									{Name: "__name__", Value: "opni_test_metric_3"},
									{Name: "example", Value: "a"},
								},
								Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
							},
						},
					})
					Expect(err).NotTo(HaveOccurred())

					Eventually(func() error {
						vec, err := queryVec([]string{"agent1"}, "opni_test_metric_3")
						if err != nil {
							return err
						}
						if len(vec) == 0 {
							return fmt.Errorf("no metrics found")
						}
						Expect(vec).To(HaveLen(1))
						Expect(vec[0].Metric).To(BeEquivalentTo(model.Metric{
							"__name__":      "opni_test_metric_3",
							"__tenant_id__": "agent1",
							"example":       "a",
						}))
						Expect(vec[0].Value).To(BeEquivalentTo(1))

						return nil
					}).Should(Succeed())
				})
			})
		})
		When("the client does not have the required session attribute", func() {
			When("a timeseries in the request contains the tenant impersonation label", func() {
				It("should push the timeseries as-is to the client's own tenant", func() {
					now := time.Now().UnixMilli()
					err := store(agent2RwClient, &prompb.WriteRequest{
						Timeseries: []prompb.TimeSeries{
							{
								Labels: []prompb.Label{
									{Name: "__name__", Value: "opni_test_metric_4"},
									{Name: "example", Value: "a"},
									{Name: metrics.LabelImpersonateAs, Value: "agent1"},
								},
								Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
							},
							{
								Labels: []prompb.Label{
									{Name: "__name__", Value: "opni_test_metric_4"},
									{Name: "example", Value: "b"},
								},
								Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
							},
						},
					})
					Expect(err).NotTo(HaveOccurred())
				})
				It("should drop the metric due to opni default relabeling rules", func() {
					Eventually(func() error {
						vec, err := queryVec([]string{"agent2"}, "opni_test_metric_4")
						if err != nil {
							return err
						}
						if len(vec) == 0 {
							return fmt.Errorf("no metrics found")
						}
						Expect(vec).To(HaveLen(1))
						Expect(vec[0].Metric).To(BeEquivalentTo(model.Metric{
							"__name__":      "opni_test_metric_4",
							"__tenant_id__": "agent2",
							"example":       "b",
						}))
						Expect(vec[0].Value).To(BeEquivalentTo(1))
						return nil
					}).Should(Succeed())
				})
			})
		})
	})
	When("metadata is pushed to the remote write service", func() {
		It("should replicate all metadata for previously federated metrics to affected tenants", func() {
			// push metadata for all test metrics as agent1 only
			err := store(agent1RwClient, &prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{},
				Metadata: []prompb.MetricMetadata{
					{
						MetricFamilyName: "opni_test_metric_1",
						Type:             prompb.MetricMetadata_GAUGE,
						Help:             "test metric 1",
						Unit:             "units",
					},
					{
						MetricFamilyName: "opni_test_metric_2",
						Type:             prompb.MetricMetadata_GAUGE,
						Help:             "test metric 2",
						Unit:             "units",
					},
					{
						MetricFamilyName: "opni_test_metric_3",
						Type:             prompb.MetricMetadata_GAUGE,
						Help:             "test metric 3",
						Unit:             "units",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				md, err := cortexAdminClient.GetMetricMetadata(context.Background(), &cortexadmin.MetricMetadataRequest{
					Tenants:    []string{"agent1"},
					MetricName: "opni_test_metric_1",
				})
				if err != nil {
					return err
				}
				Expect(md.MetricFamilyName).To(Equal("opni_test_metric_1"))
				Expect(md.Help).To(Equal("test metric 1"))
				Expect(md.Type).To(Equal(cortexadmin.MetricMetadata_GAUGE))
				Expect(md.Unit).To(Equal("units"))

				md, err = cortexAdminClient.GetMetricMetadata(context.Background(), &cortexadmin.MetricMetadataRequest{
					Tenants:    []string{"agent2"},
					MetricName: "opni_test_metric_2",
				})
				if err != nil {
					return err
				}
				Expect(md.MetricFamilyName).To(Equal("opni_test_metric_2"))
				Expect(md.Help).To(Equal("test metric 2"))
				Expect(md.Type).To(Equal(cortexadmin.MetricMetadata_GAUGE))
				Expect(md.Unit).To(Equal("units"))

				md, err = cortexAdminClient.GetMetricMetadata(context.Background(), &cortexadmin.MetricMetadataRequest{
					Tenants:    []string{"agent3"},
					MetricName: "opni_test_metric_1",
				})
				if err != nil {
					return err
				}
				Expect(md.MetricFamilyName).To(Equal("opni_test_metric_1"))
				Expect(md.Help).To(Equal("test metric 1"))
				Expect(md.Type).To(Equal(cortexadmin.MetricMetadata_GAUGE))
				Expect(md.Unit).To(Equal("units"))

				return nil
			}).Should(Succeed())
		})
	})
})
