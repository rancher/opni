package collector_test

import (
	"errors"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
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

		collectorServer1 := collector.NewCollectorServer()
		collectorServer2 := collector.NewCollectorServer()

		gauge1 := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:      "test1",
			Namespace: "remote",
			Help:      "remote test 1",
		}, []string{"label1", "label2"})
		collectorServer1.MustRegister(gauge1)

		gauge2 := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:      "test2",
			Namespace: "remote",
			Help:      "remote test 2",
		}, []string{"label1", "label2"})
		collectorServer2.MustRegister(gauge2)

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

		remoteCollector1 := collector.NewRemoteCollector(collector.NewRemoteCollectorClient(cc1))
		reg.MustRegister(remoteCollector1)
		remoteCollector2 := collector.NewRemoteCollector(collector.NewRemoteCollectorClient(cc2))
		reg.MustRegister(remoteCollector2)

		errConn, err := grpc.Dial("bufconn", grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDialer(func(s string, d time.Duration) (net.Conn, error) {
				return nil, errors.New("error")
			}))
		Expect(err).NotTo(HaveOccurred())
		errCollector := collector.NewRemoteCollector(collector.NewRemoteCollectorClient(errConn))
		Expect(reg.Register(errCollector)).To(HaveOccurred())

		// Create a local collector
		localCollector := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "local_test",
			Help: "local test",
		}, []string{"label"})
		reg.MustRegister(localCollector)

		localCollector.WithLabelValues("test1").Set(1)
		gauge1.WithLabelValues("a", "b").Set(1)
		gauge2.WithLabelValues("c", "d").Set(2)

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
		gauge1.WithLabelValues("a", "b").Set(10)
		gauge2.WithLabelValues("c", "d").Set(20)

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

		cc1.Close()

		Eventually(func() error {
			return promtestutil.GatherAndCompare(reg,
				strings.NewReader(expectedExpfmt),
				"local_test",
				"remote_test1",
				"remote_test2",
			)
		}).Should(MatchError("gathering metrics failed: rpc error: code = Canceled desc = grpc: the client connection is closing"))
	})
})
