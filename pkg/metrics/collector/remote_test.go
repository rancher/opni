package collector_test

import (
	"context"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/rancher/opni/pkg/metrics/collector"
)

var _ = Describe("Remote Collector", Label("unit"), func() {
	It("should collect metrics from remote collectors", func() {
		listener1 := bufconn.Listen(1024 * 1024)
		listener2 := bufconn.Listen(1024 * 1024)
		defer listener1.Close()
		defer listener2.Close()

		grpcServer1 := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
		grpcServer2 := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))

		reader1 := sdkmetric.NewManualReader()
		reader2 := sdkmetric.NewManualReader()

		collectorServer1 := collector.NewCollectorServer(reader1)
		collectorServer2 := collector.NewCollectorServer(reader2)

		meterProvider1 := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader1))
		meterProvider2 := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader2))

		meter1 := meterProvider1.Meter("meter1")
		meter2 := meterProvider2.Meter("meter2")

		gauge1, err := meter1.Int64ObservableGauge("test1",
			metric.WithDescription("remote test 1"))
		Expect(err).NotTo(HaveOccurred())

		gauge2, err := meter2.Int64ObservableGauge("test2",
			metric.WithDescription("remote test 2"))
		Expect(err).NotTo(HaveOccurred())

		gauge1Value := int64(1)
		gauge2Value := int64(2)

		meter1.RegisterCallback(
			func(ctx context.Context, obs metric.Observer) error {
				obs.ObserveInt64(gauge1, gauge1Value,
					metric.WithAttributes(attribute.String("label1", "a")),
					metric.WithAttributes(attribute.String("label2", "b")),
				)
				return nil
			},
			gauge1,
		)

		meter2.RegisterCallback(
			func(ctx context.Context, obs metric.Observer) error {
				obs.ObserveInt64(gauge2, gauge2Value,
					metric.WithAttributes(attribute.String("label1", "c")),
					metric.WithAttributes(attribute.String("label2", "d")),
				)
				return nil
			},
			gauge2,
		)

		collector.RegisterRemoteCollectorServer(grpcServer1, collectorServer1)
		collector.RegisterRemoteCollectorServer(grpcServer2, collectorServer2)

		go grpcServer1.Serve(listener1)
		go grpcServer2.Serve(listener2)

		cc1, err := grpc.Dial("bufconn", grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDialer(func(s string, d time.Duration) (net.Conn, error) {
				return listener1.Dial()
			}), grpc.WithBlock())
		Expect(err).NotTo(HaveOccurred())

		cc2, err := grpc.Dial("bufconn", grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDialer(func(s string, d time.Duration) (net.Conn, error) {
				return listener2.Dial()
			}), grpc.WithBlock())
		Expect(err).NotTo(HaveOccurred())

		// Create a remote collector
		reg := prometheus.NewRegistry()

		remotePrometheus1, err := otelprom.New(
			otelprom.WithNamespace("remote"),
			otelprom.WithRegisterer(reg),
			otelprom.WithoutScopeInfo(),
			otelprom.WithoutTargetInfo(),
		)
		Expect(err).NotTo(HaveOccurred())

		remotePrometheus2, err := otelprom.New(
			otelprom.WithNamespace("remote"),
			otelprom.WithRegisterer(reg),
			otelprom.WithoutScopeInfo(),
			otelprom.WithoutTargetInfo(),
		)
		Expect(err).NotTo(HaveOccurred())
		_ = sdkmetric.NewMeterProvider(sdkmetric.WithReader(remotePrometheus1))
		_ = sdkmetric.NewMeterProvider(sdkmetric.WithReader(remotePrometheus2))

		remoteCollector1 := collector.NewRemoteProducer(collector.NewRemoteCollectorClient(cc1))
		remotePrometheus1.RegisterProducer(remoteCollector1)
		remoteCollector2 := collector.NewRemoteProducer(collector.NewRemoteCollectorClient(cc2))
		remotePrometheus2.RegisterProducer(remoteCollector2)

		// Create a local collector
		localCollector := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "local_test",
			Help: "local test",
		}, []string{"label"})
		reg.MustRegister(localCollector)

		localCollector.WithLabelValues("test1").Set(1)

		expectedExpfmt := `
		# HELP local_test local test
		# TYPE local_test gauge
		local_test{label="test1"} 1

		# HELP remote_test1 remote test 1
		# TYPE remote_test1 gauge
		remote_test1{label1="a",label2="b"} 1

		# HELP remote_test2 remote test 2
		# TYPE remote_test2 gauge
		remote_test2{label1="c",label2="d"} 2
		`

		// Collect metrics from the remote collector
		Eventually(func() error {
			return promtestutil.GatherAndCompare(reg,
				strings.NewReader(expectedExpfmt),
				"local_test",
				"remote_test1",
				"remote_test2",
			)
		}).Should(Succeed())

		localCollector.WithLabelValues("test1").Set(5)
		gauge1Value = 10
		gauge2Value = 20

		expectedExpfmt = `
		# HELP local_test local test
		# TYPE local_test gauge
		local_test{label="test1"} 5

		# HELP remote_test1 remote test 1
		# TYPE remote_test1 gauge
		remote_test1{label1="a",label2="b"} 10

		# HELP remote_test2 remote test 2
		# TYPE remote_test2 gauge
		remote_test2{label1="c",label2="d"} 20
		`

		Eventually(func() error {
			return promtestutil.GatherAndCompare(reg,
				strings.NewReader(expectedExpfmt),
				"local_test",
				"remote_test1",
				"remote_test2",
			)
		}).Should(Succeed())
	})
})
