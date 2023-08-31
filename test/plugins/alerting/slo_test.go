package alerting_test

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/slo/backend"
	"github.com/rancher/opni/pkg/test"
	mockslo "github.com/rancher/opni/pkg/test/mock/slo"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/slo/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	mockSource  = "slo"
	errorSource = "error"
)

var _ = Describe("SLO Alerting", Ordered, Label("integration"), func() {
	var sloClient slov1.SLOClient
	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env).NotTo(BeNil())
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		ctrl := gomock.NewController(GinkgoT())

		errStore := mockslo.NewMockSLOStore(ctrl)

		errDiscovery := mockslo.NewMockServiceBackend(ctrl)

		errStore.EXPECT().Create(gomock.Any(), gomock.Any()).Return(
			nil,
			fmt.Errorf("some error"),
		).AnyTimes()

		errStore.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(
			fmt.Errorf("some error"),
		).AnyTimes()

		errStore.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil,
			fmt.Errorf("some error"),
		).AnyTimes()

		errStore.EXPECT().Clone(gomock.Any(), gomock.Any()).Return(
			nil,
			nil,
			fmt.Errorf("some error"),
		).AnyTimes()

		errStore.EXPECT().MultiClusterClone(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil,
			nil,
			[]error{fmt.Errorf("some error")},
		).AnyTimes()

		errStore.EXPECT().Status(gomock.Any(), gomock.Any()).Return(
			nil,
			fmt.Errorf("some error"),
		).AnyTimes()

		errStore.EXPECT().Preview(gomock.Any(), gomock.Any()).Return(
			nil,
			fmt.Errorf("some error"),
		).AnyTimes()

		errDiscovery.EXPECT().ListEvents(gomock.Any(), gomock.Any()).Return(
			nil,
			fmt.Errorf("some error"),
		).AnyTimes()

		errDiscovery.EXPECT().ListMetrics(gomock.Any(), gomock.Any()).Return(
			nil,
			fmt.Errorf("some error"),
		).AnyTimes()

		errDiscovery.EXPECT().ListServices(gomock.Any(), gomock.Any()).Return(
			nil,
			fmt.Errorf("some error"),
		).AnyTimes()

		sloStore := mockslo.NewMockSLOStore(ctrl)
		sloDiscovery := mockslo.NewMockServiceBackend(ctrl)

		sloDiscovery.EXPECT().ListServices(gomock.Any(), gomock.Any()).Return(
			&slov1.ServiceList{
				Items: []*slov1.Service{
					{
						ServiceId: "test-service",
						ClusterId: "some-cluster",
					},
				},
			},
			nil,
		).AnyTimes()

		sloDiscovery.EXPECT().ListMetrics(gomock.Any(), gomock.Any()).Return(
			&slov1.MetricGroupList{
				GroupNameToMetrics: map[string]*slov1.MetricList{
					"testgroup": {
						Items: []*slov1.Metric{
							{
								Id: "testmetric",
							},
						},
					},
				},
			},
			nil,
		).AnyTimes()

		sloDiscovery.EXPECT().ListEvents(gomock.Any(), gomock.Any()).Return(
			&slov1.EventList{
				Items: []*slov1.Event{
					{
						Key:  "test-event",
						Vals: []string{"a", "b", "c"},
					},
				},
			},
			nil,
		).AnyTimes()

		sloStore.EXPECT().Create(gomock.Any(), gomock.Any()).Return(
			&corev1.Reference{
				Id: "test-slo",
			},
			nil,
		).AnyTimes()

		sloStore.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			&slov1.SLOData{
				Id:  "test-slo",
				SLO: util.ProtoClone(updatedMockSLO),
			},
			nil,
		).AnyTimes()

		sloStore.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		sloStore.EXPECT().Clone(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, toClone *slov1.SLOData) (*corev1.Reference, *slov1.SLOData, error) {
				id := uuid.New().String()
				clonedData := util.ProtoClone(toClone)
				clonedData.Id = id
				return &corev1.Reference{
						Id: id,
					},
					clonedData,
					nil
			}).AnyTimes()

		sloStore.EXPECT().MultiClusterClone(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(
				_ context.Context,
				slo *slov1.SLOData,
				toClusters []*corev1.Reference,
			) ([]*corev1.Reference, []*slov1.SLOData, []error) {
				ids := []*corev1.Reference{}
				datas := []*slov1.SLOData{}
				errors := []error{}
				for _, cluster := range toClusters {
					uuid := uuid.New().String()
					newSLO := util.ProtoClone(
						slo.GetSLO(),
					)
					newSLO.ClusterId = cluster.Id
					newSLO.Name += "-clone"
					ids = append(ids, &corev1.Reference{Id: uuid})
					datas = append(datas, &slov1.SLOData{
						Id:  uuid,
						SLO: newSLO,
					})
					errors = append(errors, nil)
				}
				return ids, datas, errors
			}).AnyTimes()

		sloStore.EXPECT().Status(gomock.Any(), gomock.Any()).Return(
			&slov1.SLOStatus{
				State: slov1.SLOStatusState_Creating,
			},
			nil,
		).AnyTimes()

		sloStore.EXPECT().Preview(gomock.Any(), gomock.Any()).Return(
			&slov1.SLOPreviewResponse{
				PlotVector: &slov1.PlotVector{
					Objective: 99.9,
					Items:     []*slov1.DataPoint{},
					Windows:   []*slov1.AlertFiringWindows{},
				},
			},
			nil,
		).AnyTimes()

		alertClusterClient := alertops.NewAlertingAdminClient(env.ManagementClientConn())
		mgmtClient := env.NewManagementClient()

		mockDatasource := backend.NewSLODatasource(sloStore, sloDiscovery, func(_ context.Context, _ *corev1.Reference) error {
			return nil
		})

		errDatasource := backend.NewSLODatasource(errStore, errDiscovery, func(_ context.Context, _ *corev1.Reference) error {
			return nil
		})

		slo.RegisterDatasource(mockSource, mockDatasource)
		slo.RegisterDatasource(errorSource, errDatasource)

		_, err := alertClusterClient.InstallCluster(env.Context(), &emptypb.Empty{})
		Expect(err).To(BeNil())

		certsInfo, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint := certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())
		token, err := mgmtClient.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		_, errC := env.StartAgent("agent", token, []string{fingerprint})
		Eventually(errC, time.Second*10, time.Millisecond*200).Should(Receive(BeNil()))

		_, errC2 := env.StartAgent("agent2", token, []string{fingerprint})
		Eventually(errC2, time.Second*10, time.Millisecond*200).Should(Receive(BeNil()))

		Eventually(func() error {
			status, err := alertClusterClient.GetClusterStatus(env.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			if status.State != alertops.InstallState_Installed {
				return fmt.Errorf("alerting cluster install state is %s", status.State.String())
			}
			return nil
		}, time.Second*5, time.Millisecond*200).Should(Succeed())

		sloClient = slov1.NewSLOClient(env.ManagementClientConn())
	})

	When("interacting with the SLO server", func() {
		It("should list the available datasources", func() {
			datasources, err := sloClient.ListDatasources(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(datasources.Items).To(ConsistOf([]string{errorSource, mockSource, "monitoring"}))
		})
	})

	When("Managing SLOs with the mocked datasource", func() {
		It("should return a list of available services", func() {
			svcList, err := sloClient.ListServices(env.Context(), &slov1.ListServicesRequest{
				Datasource: mockSource,
				ClusterId:  "some-cluster",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(svcList.Items).To(HaveLen(1))
			Expect(svcList.Items[0]).To(
				Equal(&slov1.Service{
					ServiceId: "test-service",
					ClusterId: "some-cluster",
				}),
			)
		})

		It("should return a list of available metrics", func() {
			metricList, err := sloClient.ListMetrics(env.Context(), &slov1.ListMetricsRequest{
				Datasource: mockSource,
				ClusterId:  "some-cluster",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(metricList.GroupNameToMetrics).To(HaveLen(1))
			Expect(metricList.GroupNameToMetrics["testgroup"].Items).To(
				Equal(
					[]*slov1.Metric{
						{
							Id: "testmetric",
						},
					},
				),
			)
		})

		It("should return a list of available events", func() {
			eventList, err := sloClient.ListEvents(env.Context(), &slov1.ListEventsRequest{
				Datasource: mockSource,
				ClusterId:  "some-cluster",
				ServiceId:  "test-service",
				MetricId:   "testmetric",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(eventList.Items).To(HaveLen(1))
			Expect(eventList.Items[0]).To(
				Equal(&slov1.Event{
					Key:  "test-event",
					Vals: []string{"a", "b", "c"},
				}),
			)
		})

		It("should create an SLO", func() {
			sloImpl := util.ProtoClone(mockSLO)
			sloRef, err := sloClient.CreateSLO(env.Context(), &slov1.CreateSLORequest{
				Slo: sloImpl,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(sloRef).NotTo(BeNil())

			By("verifying we can fetch the SLO")
			data, err := sloClient.GetSLO(env.Context(), sloRef)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.SLO).To(testutil.ProtoEqual((sloImpl)))

			By("verifying the SLO list only contains one element")
			sloList, err := sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(sloList.Items).To(HaveLen(1))
		})

		Specify("when the SLOStore fails to create an SLO, the management server should not have it", func() {
			sloList, err := sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			originalLen := len(sloList.Items)

			sloImpl := util.ProtoClone(mockSLO)
			sloImpl.Datasource = errorSource
			createSLOFailure := &slov1.CreateSLORequest{
				Slo: sloImpl,
			}

			sloRef, err := sloClient.CreateSLO(env.Context(), createSLOFailure)
			Expect(err).To(HaveOccurred())
			Expect(sloRef).To(BeNil())

			sloList, err = sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(sloList.Items).To(HaveLen(originalLen))
		})

		It("should update the SLO details", func() {
			sloImpl := util.ProtoClone(mockSLO)
			_, err := sloClient.UpdateSLO(env.Context(), &slov1.SLOData{
				Id:  "test-slo",
				SLO: sloImpl,
			})
			Expect(err).NotTo(HaveOccurred())

			data, err := sloClient.GetSLO(env.Context(), &corev1.Reference{
				Id: "test-slo",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SLO).To(testutil.ProtoEqual(updatedMockSLO))
		})

		It("should clone an SLO", func() {
			sloList, err := sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(sloList.Items).NotTo(HaveLen(0))

			toClone := sloList.GetItems()[0]

			clone, err := sloClient.CloneSLO(env.Context(), &corev1.Reference{Id: toClone.Id})
			Expect(err).NotTo(HaveOccurred())
			Expect(clone.SLO).To(testutil.ProtoEqual(toClone.GetSLO()))
		})

		It("should clone an SLO across clusters", func() {
			sloList, err := sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(sloList.Items).NotTo(HaveLen(0))

			originalLen := len(sloList.Items)

			toClone := sloList.GetItems()[0]

			failures, err := sloClient.CloneToClusters(env.Context(), &slov1.MultiClusterSLO{
				CloneId: &corev1.Reference{Id: toClone.GetId()},
				Clusters: []*corev1.Reference{
					{Id: "agent"},
					{Id: "agent2"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(failures.GetFailures()).To(HaveLen(0))

			sloList, err = sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())

			Expect(sloList.Items).To(HaveLen(originalLen + 2))

		})

		It("should error when the fetching the status of non-existent SLO", func() {
			status, err := sloClient.GetSLOStatus(env.Context(), &corev1.Reference{
				Id: "definitely does not exists",
			})

			Expect(err).To(HaveOccurred())
			Expect(status).To(BeNil())
		})

		It("should report the status of running SLOs", func() {
			slos, err := sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(slos.GetItems()).NotTo(HaveLen(0))

			for _, slo := range slos.GetItems() {
				status, err := sloClient.GetSLOStatus(env.Context(), &corev1.Reference{
					Id: slo.GetId(),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())
				Expect(status.State).To(Equal(slov1.SLOStatusState_Creating))
			}
		})

		It("should return a preview of an SLO definition", func() {
			payload := util.ProtoClone(mockSLO)
			vector, err := sloClient.Preview(env.Context(), &slov1.CreateSLORequest{
				Slo: payload,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(vector.GetPlotVector()).NotTo(BeNil())
		})

		It("should delete SLOs", func() {
			sloList, err := sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(sloList.Items)).To(BeNumerically(">", 0))

			for _, item := range sloList.GetItems() {
				_, err = sloClient.DeleteSLO(env.Context(), &corev1.Reference{
					Id: item.GetId(),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			sloList, err = sloClient.ListSLOs(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(sloList.Items).To(HaveLen(0))
		})
	})

	When("Managing SLOs with the metrics datasource", func() {
		It("Should return failed precondition when metrics is not installed", func() {
			mgmtClient := env.NewManagementClient()
			cluster, err := mgmtClient.GetCluster(env.Context(), &corev1.Reference{Id: "agent"})
			Expect(err).NotTo(HaveOccurred())
			Expect(capabilities.Has(cluster, capabilities.Cluster(wellknown.CapabilityMetrics))).To(BeFalse())

			_, err = sloClient.CreateSLO(env.Context(), &slov1.CreateSLORequest{
				Slo: metricsSLO,
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(testutil.MatchStatusCode(codes.FailedPrecondition))

			_, err = sloClient.CreateSLO(env.Context(), &slov1.CreateSLORequest{
				Slo: util.ProtoClone(metricsSLO),
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(testutil.MatchStatusCode(codes.FailedPrecondition))

			_, err = sloClient.ListServices(env.Context(), &slov1.ListServicesRequest{
				Datasource: "monitoring",
				ClusterId:  "agent",
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(testutil.MatchStatusCode(codes.FailedPrecondition))

			_, err = sloClient.ListEvents(env.Context(), &slov1.ListEventsRequest{
				Datasource: "monitoring",
				ClusterId:  "agent",
				ServiceId:  "test-service",
				MetricId:   "testmetric",
			})
			Expect(err).To(HaveOccurred())

			_, err = sloClient.ListMetrics(env.Context(), &slov1.ListMetricsRequest{
				Datasource: "monitoring",
				ClusterId:  "agent",
				ServiceId:  "test-service",
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(testutil.MatchStatusCode(codes.FailedPrecondition))
		})
	})
})

var (
	mockSLO = &slov1.ServiceLevelObjective{
		Name:           "test-slo-success",
		ClusterId:      "agent",
		ServiceId:      "test-service",
		GoodMetricName: "testmetric",
		GoodEvents: []*slov1.Event{
			{
				Key:  "a",
				Vals: []string{"a", "b", "c"},
			},
		},
		TotalMetricName:   "testmetric",
		TotalEvents:       []*slov1.Event{},
		Target:            &slov1.Target{Value: 99.9},
		SloPeriod:         "30d",
		BudgetingInterval: durationpb.New(time.Minute * 5),
		Datasource:        mockSource,
	}

	metricsSLO = &slov1.ServiceLevelObjective{
		Name:           "test-slo-success",
		ClusterId:      "agent",
		ServiceId:      "test-service",
		GoodMetricName: "testmetric",
		GoodEvents: []*slov1.Event{
			{
				Key:  "a",
				Vals: []string{"a", "b", "c"},
			},
		},
		TotalMetricName:   "testmetric",
		TotalEvents:       []*slov1.Event{},
		Target:            &slov1.Target{Value: 99.9},
		SloPeriod:         "30d",
		BudgetingInterval: durationpb.New(time.Minute * 5),
		Datasource:        "monitoring",
	}

	updatedMockSLO = &slov1.ServiceLevelObjective{
		Name:           "test-slo-success2",
		ClusterId:      "agent",
		ServiceId:      "test-service2",
		GoodMetricName: "testmetric2",
		GoodEvents: []*slov1.Event{
			{
				Key:  "a2",
				Vals: []string{"a2", "b2", "c2"},
			},
		},
		TotalMetricName:   "testmetric2",
		TotalEvents:       []*slov1.Event{},
		Target:            &slov1.Target{Value: 99.9},
		SloPeriod:         "30d",
		BudgetingInterval: durationpb.New(time.Minute * 5),
		Datasource:        mockSource,
	}
)
