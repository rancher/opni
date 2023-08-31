package metrics_test

import (
	"context"
	"errors"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/slo/backend/metrics"
	cortexadmin_mock "github.com/rancher/opni/pkg/test/mock/cortexadmin"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TODO : maybe move this to a conformance test
var _ = Describe("SLO Metrics store", Ordered, Label("unit"), func() {
	var metricsStore *metrics.MetricsSLOStore
	BeforeAll(func() {
		ctrl := gomock.NewController(GinkgoT())

		mockClient := cortexadmin_mock.NewMockCortexAdminClient(ctrl)

		mockClient.EXPECT().LoadRules(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil).AnyTimes()

		mockClient.EXPECT().DeleteRule(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil).AnyTimes()

		mockClient.EXPECT().GetRule(gomock.Any(), gomock.Any()).Return(nil, status.Error(codes.NotFound, "rule not found")).AnyTimes()

		mockClient.EXPECT().Query(gomock.Any(), gomock.Any()).Return(&cortexadmin.QueryResponse{
			Data: []byte{},
		}, nil).AnyTimes()

		lg := logger.NewPluginLogger().Named("slo")
		provider := metrics.NewMetricsSLOStoreProvider(lg)
		provider.Initialize(mockClient)
		metricsStore = metrics.NewMetricsSLOStore(
			provider,
		)
	})

	When("we use the mocked metrics store", func() {
		It("should be able to create metrics SLOs", func() {
			ref, err := metricsStore.Create(context.TODO(), &slov1.CreateSLORequest{
				Slo: util.ProtoClone(mockSLO),
			})
			Expect(err).To(Succeed())
			Expect(ref).NotTo(BeNil())
		})

		It("should be able to update metrics SLOs", func() {
			ref, err := metricsStore.Update(
				context.TODO(),
				&slov1.SLOData{
					Id:  uuid.New().String(),
					SLO: util.ProtoClone(mockSLO),
				},
				&slov1.SLOData{
					Id:  uuid.New().String(),
					SLO: util.ProtoClone(mockSLO),
				},
			)
			Expect(err).To(Succeed())
			Expect(ref).NotTo(BeNil())
		})

		It("should be able to clone metrics SLOs", func() {
			ref, _, err := metricsStore.Clone(
				context.TODO(),
				&slov1.SLOData{
					Id:  uuid.New().String(),
					SLO: util.ProtoClone(mockSLO),
				},
			)
			Expect(err).To(Succeed())
			Expect(ref).NotTo(BeNil())
		})

		It("should be able to clone metrics SLOs across clusters", func() {
			refs, _, errs := metricsStore.MultiClusterClone(
				context.TODO(),
				&slov1.SLOData{
					Id:  uuid.New().String(),
					SLO: util.ProtoClone(mockSLO),
				},
				[]*corev1.Reference{
					{
						Id: "test",
					},
				},
			)
			Expect(errors.Join(errs...)).To(Succeed())
			for _, ref := range refs {
				Expect(ref).NotTo(BeNil())
			}
		})

		It("should be able to delete metrics SLOs", func() {
			err := metricsStore.Delete(
				context.TODO(),
				&slov1.SLOData{
					Id:  uuid.New().String(),
					SLO: util.ProtoClone(mockSLO),
				},
			)
			Expect(err).To(Succeed())
		})

		XIt("should be able to fetch the status of metrics SLOs", func() {
			// TODO
			_, err := metricsStore.Status(
				context.TODO(),
				&slov1.SLOData{
					Id:  uuid.New().String(),
					SLO: util.ProtoClone(mockSLO),
				},
			)
			Expect(err).To(Succeed())
		})

		XIt("should fetch previews of the metrics SLOS", func() {
			// TODO
			_, err := metricsStore.Preview(
				context.TODO(),
				&slov1.CreateSLORequest{
					Slo: util.ProtoClone(mockSLO),
				},
			)
			Expect(err).To(Succeed())
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
		Datasource:        "monitoring",
	}
)
