package otel_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"text/template"
	"time"

	"github.com/golang/snappy"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/plugins/metrics/pkg/otel"
	"github.com/samber/lo"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"

	"github.com/rancher/opni/pkg/test/testdata"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

type agentSinkConfig struct {
	SinkPort           int
	TelemetryPort      int
	RemoteWriteAddress string
}

type agentForwarderConfig struct {
	TelemetryPort            int
	OTLPExporterEndpoint     string
	OTLPHttpExporterEndpoint string
}

var _ = Describe("OTEL forwarder", Ordered, Label("integration"), func() {
	var mut *test.RemoteWriteMutator
	var ctx context.Context
	var ctxCa context.Context
	var configDir string
	var forwarderAddress string
	BeforeAll(func() {
		ctx = context.Background()
		configDir = fmt.Sprintf("/tmp/otelcol-%s", uuid.New().String())
		ctxca, cancel := context.WithCancel(ctx)
		ctxCa = ctxca
		DeferCleanup(func() {
			cancel()
			os.RemoveAll(configDir)
		})
		By("setting up a dummy remote write api")
		mut = test.NewRemoteWriteMutator(
			func(bytes.Buffer) { /*no-=op*/ },
		)
		remoteWriteAddress := test.StartRemoteWriteServer(ctxca, mut)

		By("setting up a sink destination for our forwarder")
		sinkAgentConfig := testdata.TestData("otel/otlp-sink.tmpl")
		sinkTmpl, err := template.New("sinkAgentConfig").Parse(string(sinkAgentConfig))
		Expect(err).ToNot(HaveOccurred())
		var sb bytes.Buffer
		sag := agentSinkConfig{
			SinkPort:           freeport.GetFreePort(),
			TelemetryPort:      freeport.GetFreePort(),
			RemoteWriteAddress: remoteWriteAddress,
		}
		err = sinkTmpl.Execute(&sb, sag)
		Expect(err).ToNot(HaveOccurred())
		sinkAgentConfig = sb.Bytes()
		env.StartEmbeddedOTELCollector(ctxca, sinkAgentConfig, path.Join(configDir, "sink.yaml"))

		By("setting up our custom metrics forwarder")
		otelForwarder := otel.NewOTELForwarder(
			ctxca,
			otel.WithRemoteAddress(fmt.Sprintf("localhost:%d", sag.SinkPort)),
			otel.WithDialOptions(
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			),
			otel.WithPrivileged(false),
		)
		forwarderAddress = fmt.Sprintf(":%d", freeport.GetFreePort())
		forwarderListener, err := net.Listen("tcp4", forwarderAddress)
		Expect(err).ToNot(HaveOccurred())

		server := grpc.NewServer(
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             15 * time.Second,
				PermitWithoutStream: true,
			}),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    15 * time.Second,
				Timeout: 5 * time.Second,
			}),
		)
		colmetricspb.RegisterMetricsServiceServer(server, &otelForwarder)
		_ = lo.Async(func() error {
			return server.Serve(forwarderListener)
		})

		DeferCleanup(func() {
			server.Stop()
		})

	})
	When("we run the forwarder", func() {
		It("should forward the metrics on otlp inputs", func() {
			By("setting up a collector for our forwarder")
			forwarderAgentConfig := testdata.TestData("otel/otlp-agent-forwarder.tmpl")
			forwarderTmpl, err := template.New("forwarderAgentConfig").Parse(string(forwarderAgentConfig))
			Expect(err).ToNot(HaveOccurred())
			var fb bytes.Buffer
			err = forwarderTmpl.Execute(&fb, agentForwarderConfig{
				TelemetryPort:        freeport.GetFreePort(),
				OTLPExporterEndpoint: forwarderAddress,
			})
			Expect(err).ToNot(HaveOccurred())
			forwarderAgentConfig = fb.Bytes()
			env.StartEmbeddedOTELCollector(ctxCa, forwarderAgentConfig, path.Join(configDir, "forwarder.yaml"))

			receiver := make(chan []prompb.TimeSeries)
			mut.SetFn(func(input bytes.Buffer) {
				// prometheus exporter sends it this way
				data := input.Bytes()
				decodeData := []byte{}
				outputData, err := snappy.Decode(decodeData, data)
				if err != nil {
					return
				}
				if len(outputData) > 0 {
					decodeData = outputData
				}
				writeReq := prompb.WriteRequest{}
				err = proto.Unmarshal(decodeData, protoadapt.MessageV2Of(&writeReq))
				if err != nil {
					return
				}

				receiver <- writeReq.Timeseries
			})
			By("verifying the metrics arrive at the pipeline destination")
			Eventually(receiver, 10*time.Second).Should(Receive(Not(HaveLen(0))))
		})

		It("should forward the metrics on otlphttp inputs", func() {
			//TODO : implement me
		})
	})
})
