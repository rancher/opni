package shipper_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/supportagent/shipper"
	"github.com/rancher/opni/pkg/test/testlog"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	otlpcommonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("otlp shipper", Ordered, Label("unit"), func() {
	var (
		mockServer *mockLogsServiceServer
		grpcSrv    *grpc.Server
		conn       *grpc.ClientConn
		lis        *bufconn.Listener
	)
	BeforeAll(func() {
		mockServer = newMockLogsServiceServer()
		lis = bufconn.Listen(1024 * 1024)
		grpcSrv = grpc.NewServer(
			grpc.Creds(insecure.NewCredentials()),
		)
		collogspb.RegisterLogsServiceServer(grpcSrv, mockServer)
		go grpcSrv.Serve(lis)
		var err error
		conn, err = grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		Expect(err).NotTo(HaveOccurred())
	})
	AfterAll(func() {
		conn.Close()
		lis.Close()
	})

	When("otlp shipper has a log line is sent", func() {
		var (
			otlpShipper shipper.Shipper
			scanner     *bufio.Scanner
			timestamp   time.Time
		)
		BeforeEach(func() {
			timestamp = time.Now()
			otlpShipper = shipper.NewOTLPShipper(
				conn,
				&mockDateParser{timestamp: timestamp},
				testlog.Log,
			)
			logLine := "this is a log line"
			scanner = bufio.NewScanner(strings.NewReader(logLine))
		})
		It("should publish logs with the log type", func() {
			By("publishing logs")
			err := otlpShipper.Publish(context.Background(), scanner)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockServer.logs).To(HaveLen(1))
			resourceLog := mockServer.logs[0]

			By("checking the log")
			Expect(resourceLog.GetScopeLogs()).To(HaveLen(1))
			scopeLog := resourceLog.GetScopeLogs()[0]
			Expect(scopeLog).NotTo(BeNil())
			Expect(scopeLog.GetLogRecords()).To(HaveLen(1))
			logRecord := scopeLog.GetLogRecords()[0]
			Expect(logRecord).NotTo(BeNil())
			Expect(logRecord.GetTimeUnixNano()).To(Equal(uint64(timestamp.UnixNano())))
			Expect(logRecord.GetBody().GetStringValue()).To(Equal("this is a log line"))
		})
		When("the shipper has a log type", func() {
			var otlpShipper shipper.Shipper
			BeforeEach(func() {
				otlpShipper = shipper.NewOTLPShipper(
					conn,
					&mockDateParser{timestamp: timestamp},
					testlog.Log,
					shipper.WithLogType("foo"),
				)
			})
			It("should attach the log type to the resource", func() {
				err := otlpShipper.Publish(context.Background(), scanner)
				Expect(err).NotTo(HaveOccurred())

				Expect(mockServer.logs).To(HaveLen(1))
				resourceLog := mockServer.logs[0]
				Expect(resourceLog.GetResource().GetAttributes()).To(HaveLen(1))
				Expect(resourceLog.GetResource().GetAttributes()).To(ContainElement(
					&otlpcommonv1.KeyValue{
						Key:   "log_type",
						Value: &otlpcommonv1.AnyValue{Value: &otlpcommonv1.AnyValue_StringValue{StringValue: "foo"}},
					},
				))
			})
		})
		When("the shipper has a component type", func() {
			var otlpShipper shipper.Shipper
			BeforeEach(func() {
				otlpShipper = shipper.NewOTLPShipper(
					conn,
					&mockDateParser{timestamp: timestamp},
					testlog.Log,
					shipper.WithComponent("bar"),
				)
			})
			It("should attach the log type to the resource", func() {
				err := otlpShipper.Publish(context.Background(), scanner)
				Expect(err).NotTo(HaveOccurred())

				Expect(mockServer.logs).To(HaveLen(1))
				resourceLog := mockServer.logs[0]
				Expect(resourceLog.GetResource().GetAttributes()).To(HaveLen(2))
				Expect(resourceLog.GetResource().GetAttributes()).To(ContainElement(
					&otlpcommonv1.KeyValue{
						Key:   "kubernetes_component",
						Value: &otlpcommonv1.AnyValue{Value: &otlpcommonv1.AnyValue_StringValue{StringValue: "bar"}},
					},
				))
			})
		})
	})
	When("otlp shipper has multiple log lines sent", func() {
		var (
			otlpShipper shipper.Shipper
			scanner     *bufio.Scanner
			timestamp   time.Time
		)
		BeforeEach(func() {
			timestamp = time.Now()
			otlpShipper = shipper.NewOTLPShipper(
				conn,
				&mockDateParser{timestamp: timestamp},
				testlog.Log,
			)
			logs := `this is the first log line
this is the second log line
this is the third log line
this is the fourth log line
this is the fifth log line
`
			scanner = bufio.NewScanner(strings.NewReader(logs))
		})
		It("should batch the logs together", func() {
			By("publishing logs")
			err := otlpShipper.Publish(context.Background(), scanner)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockServer.logs).To(HaveLen(1))
			resourceLog := mockServer.logs[0]

			By("checking the log")
			Expect(resourceLog.GetScopeLogs()).To(HaveLen(1))
			scopeLog := resourceLog.GetScopeLogs()[0]
			Expect(scopeLog).NotTo(BeNil())
			Expect(scopeLog.GetLogRecords()).To(HaveLen(5))
		})
	})
})
