package metrics_test

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/slo/backend/metrics"
	cortexadmin_mock "github.com/rancher/opni/pkg/test/mock/cortexadmin"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ = Describe("SLO Metrics store", Ordered, Label("unit"), func() {
	var metricsStore *metrics.MetricsSLOStore
	BeforeAll(func() {
		ctrl := gomock.NewController(GinkgoT())

		mockClient := cortexadmin_mock.NewMockCortexAdminClient(ctrl)

		mockClient.EXPECT().LoadRules(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil).AnyTimes()

		mockClient.EXPECT().DeleteRule(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil).AnyTimes()

		mockClient.EXPECT().GetRule(gomock.Any(), gomock.Any()).Return(nil, status.Error(codes.NotFound, "rule not found")).AnyTimes()

		mockClient.EXPECT().Query(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req *cortexadmin.QueryRequest, _ ...grpc.CallOption) (*cortexadmin.QueryResponse, error) {
				v := model.Vector{
					{
						Metric: model.Metric{
							"job": "test-service",
						},
						Value:     1,
						Timestamp: model.Time(100000),
					},
				}

				wrapper := struct {
					Type   model.ValueType `json:"resultType"`
					Result json.RawMessage `json:"result"`
				}{}
				wrapper.Type = model.ValVector
				wrapperBytes, err := json.Marshal(v)
				if err != nil {
					return nil, nil
				}
				wrapper.Result = wrapperBytes

				vectorData, err := json.Marshal(wrapper)
				if err != nil {
					return nil, err
				}

				promResp := compat.ApiResponse{
					Status:    "success",
					Data:      vectorData,
					ErrorType: "",
					Error:     "",
					Warnings:  []string{},
				}
				data, err := json.Marshal(promResp)
				if err != nil {
					return nil, err
				}
				return &cortexadmin.QueryResponse{
					Data: data,
				}, nil
			},
		).AnyTimes()

		mockClient.EXPECT().QueryRange(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req *cortexadmin.QueryRangeRequest, _ ...grpc.CallOption) (*cortexadmin.QueryResponse, error) {
				v := model.Vector{
					{
						Metric: model.Metric{
							"job": "test-service",
						},
						Value:     1,
						Timestamp: model.Time(100000),
					},
				}

				wrapper := struct {
					Type   model.ValueType `json:"resultType"`
					Result json.RawMessage `json:"result"`
				}{}
				wrapper.Type = model.ValVector
				wrapperBytes, err := json.Marshal(v)
				if err != nil {
					return nil, nil
				}
				wrapper.Result = wrapperBytes

				vectorData, err := json.Marshal(wrapper)
				if err != nil {
					return nil, err
				}

				promResp := compat.ApiResponse{
					Status:    "success",
					Data:      vectorData,
					ErrorType: "",
					Error:     "",
					Warnings:  []string{},
				}
				data, err := json.Marshal(promResp)
				if err != nil {
					return nil, err
				}
				return &cortexadmin.QueryResponse{
					Data: data,
				}, nil
			},
		).AnyTimes()

		lg := logger.NewPluginLogger().Named("slo")
		provider := metrics.NewMetricsSLOStoreProvider(lg)
		provider.Initialize(mockClient)
		metricsStore = metrics.NewMetricsSLOStore(
			provider,
		)
	})

	It("should detect active SLO windows from samples", func() {
		samples := []compat.Sample{
			{
				Value:     0,
				Timestamp: 1,
			},
			{
				Value:     5,
				Timestamp: 2,
			},
			{
				Value:     10,
				Timestamp: 10,
			},
			{
				Value:     0,
				Timestamp: 11,
			},
			{
				Value:     0,
				Timestamp: 15,
			},
			{
				Value:     1,
				Timestamp: 33,
			},
			{
				Value:     0,
				Timestamp: 34,
			},
			{
				Value:     55,
				Timestamp: 4000,
			},
		}
		windows := metrics.DetectFiringIntervals("critical", samples)

		convert := func(a int64) time.Time {
			return time.Unix(a/1000, (a%1000)*int64(time.Millisecond))
		}

		// t := time.Unix(sample.Timestamp/1000, (sample.Timestamp%1000)*int64(time.Millisecond))
		first := &slov1.AlertFiringWindows{Start: timestamppb.New(convert(2)), End: timestamppb.New(convert(11)), Severity: "critical"}
		second := &slov1.AlertFiringWindows{Start: timestamppb.New(convert(33)), End: timestamppb.New(convert(34)), Severity: "critical"}
		third := &slov1.AlertFiringWindows{Start: timestamppb.New(convert(4000)), End: timestamppb.New(convert(4000)), Severity: "critical"}

		Expect(len(windows)).To(Equal(3))
		Expect(windows[0]).To(testutil.ProtoEqual(first))
		Expect(windows[1]).To(testutil.ProtoEqual(second))
		last := windows[2]
		Expect(last.Start).To(testutil.ProtoEqual(third.Start))
		Expect(last.Severity).To(Equal(third.Severity))
		Expect(last.End).NotTo(BeNil())
		Expect(last.End.AsTime().After(time.Now().Add(-time.Minute))).To(BeTrue())

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

		It("should be able to fetch the status of metrics SLOs", func() {
			_, err := metricsStore.Status(
				context.TODO(),
				&slov1.SLOData{
					Id:  uuid.New().String(),
					SLO: util.ProtoClone(mockSLO),
				},
			)
			Expect(err).To(Succeed())
		})

		It("should fetch previews of the metrics SLOS", func() {
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
