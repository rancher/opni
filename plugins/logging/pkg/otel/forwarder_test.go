package otel_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/logging/pkg/otel"
	"github.com/samber/lo"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	semconv "go.opentelemetry.io/collector/semconv/v1.18.0"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"net"
	"net/http"
	"os"
	"path"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testdata"
)

// aggregatorConfig is used to create the OTEL Collector that receives by OTLP
// and exports to the Forwarder
type aggregatorConfig struct {
	ForwarderAddress string
	AggregatorPort   int
}

// preprocessorConfig is used to create the OTEL Collector that receives by OTLP
// from the Forwarder and exports to the sink server
type preprocessorConfig struct {
	SinkAddress      string
	PreprocessorPort int
}

var _ = Describe("OTEL forwarder", Ordered, Label("integration"), func() {
	var configDir string
	var ctx context.Context
	var mut *test.SinkMutator
	var aggregatorCfg aggregatorConfig
	var preprocessorCfg preprocessorConfig

	BeforeAll(func() {
		ctx = context.Background()
		configDir = fmt.Sprintf("/tmp/otelcol-%s", uuid.New().String())
		ctxca, cancel := context.WithCancel(ctx)
		DeferCleanup(func() {
			cancel()
			os.RemoveAll(configDir)
		})

		By("setting up an HTTP sink for the exporter")
		mut = test.NewSinkMutator(
			func(bytes.Buffer) {},
		)
		sinkAddress := test.StartSinkHTTPServer(ctxca, "/v1/traces", mut)

		By("setting up the OTEL preprocessor")
		preprocessorTmpl := testdata.TestData("otel/otlp-preprocessor.tmpl")
		parsedPreprocessorTmpl, err := template.New("preprocessorTmpl").Parse(string(preprocessorTmpl))
		Expect(err).ToNot(HaveOccurred())

		var sb bytes.Buffer
		preprocessorCfg = preprocessorConfig{
			SinkAddress:      sinkAddress,
			PreprocessorPort: freeport.GetFreePort(),
		}
		err = parsedPreprocessorTmpl.Execute(&sb, preprocessorCfg)
		Expect(err).ToNot(HaveOccurred())

		preprocessorTmpl = sb.Bytes()
		env.StartEmbeddedOTELCollector(ctxca, preprocessorTmpl, path.Join(configDir, "preprocessor.yaml"))

		By("setting up the forwarder")
		forwarder := otel.NewTraceForwarder(
			otel.WithAddress(fmt.Sprintf("localhost:%d", preprocessorCfg.PreprocessorPort)),
			otel.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
			otel.WithPrivileged(false),
		)
		forwarder.Client.BackgroundInitClient(forwarder.InitializeTraceForwarder)

		forwarderAddress := fmt.Sprintf(":%d", freeport.GetFreePort())
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
		coltracepb.RegisterTraceServiceServer(server, forwarder)
		_ = lo.Async(func() error {
			return server.Serve(forwarderListener)
		})

		DeferCleanup(func() {
			server.Stop()
		})

		By("setting up the OTEL aggregator")
		aggregatorTmpl := testdata.TestData("otel/otlp-aggregator.tmpl")
		parsedAggregatorTmpl, err := template.New("aggregatorConfig").Parse(string(aggregatorTmpl))
		Expect(err).ToNot(HaveOccurred())

		var ab bytes.Buffer
		aggregatorCfg = aggregatorConfig{
			AggregatorPort:   freeport.GetFreePort(),
			ForwarderAddress: forwarderAddress,
		}
		err = parsedAggregatorTmpl.Execute(&ab, aggregatorCfg)
		Expect(err).ToNot(HaveOccurred())

		aggregatorTmpl = ab.Bytes()
		env.StartEmbeddedOTELCollector(ctxca, aggregatorTmpl, path.Join(configDir, "aggregator.yaml"))
	})
	When("we send a trace to the aggregator", func() {
		It("should forward the trace all the way to the sink", func() {
			By("setting up a channel that captures requests to the sink")
			receiver := make(chan ptrace.Traces)
			mut.SetFn(func(input bytes.Buffer) {
				data := input.Bytes()
				req := ptraceotlp.NewExportRequest()
				err := req.UnmarshalProto(data)
				if err != nil {
					return
				}

				receiver <- req.Traces()
			})

			By("sending a trace to the OTEL aggregator by OTLPHTTP")
			Eventually(func() error {
				trace := createTrace()
				req := ptraceotlp.NewExportRequestFromTraces(trace)
				buf, err := req.MarshalJSON()
				if err != nil {
					return err
				}
				_, err = http.Post(fmt.Sprintf("http://localhost:%d/v1/traces", aggregatorCfg.AggregatorPort), "application/json", bytes.NewReader(buf))
				return err
			}, 3*time.Second, 20*time.Millisecond).Should(Succeed())

			By("checking that trace is actually received by the sink channel")
			var trace ptrace.Traces
			Eventually(receiver, 15*time.Second).Should(Receive(&trace))

			spans := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
			Expect(spans.Len()).Should(Equal(2))
			Expect(spans.At(0).Name()).Should(Equal("testSpan1"))
			Expect(spans.At(1).Name()).Should(Equal("testSpan2"))
		})
	})
})

// createTrace was scraped from https://github.com/open-telemetry/opentelemetry-collector/blob/7c5ecef11dff4ce5501c9683b277a25a61ea0f1a/receiver/otlpreceiver/otlp_test.go#L105
func createTrace() ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(semconv.AttributeHostName, "testHost")
	spans := rs.ScopeSpans().AppendEmpty().Spans()
	span1 := spans.AppendEmpty()
	span1.SetTraceID([16]byte{0x5B, 0x8E, 0xFF, 0xF7, 0x98, 0x3, 0x81, 0x3, 0xD2, 0x69, 0xB6, 0x33, 0x81, 0x3F, 0xC6, 0xC})
	span1.SetSpanID([8]byte{0xEE, 0xE1, 0x9B, 0x7E, 0xC3, 0xC1, 0xB1, 0x74})
	span1.SetParentSpanID([8]byte{0xEE, 0xE1, 0x9B, 0x7E, 0xC3, 0xC1, 0xB1, 0x73})
	span1.SetName("testSpan1")
	span1.SetStartTimestamp(1544712660300000000)
	span1.SetEndTimestamp(1544712660600000000)
	span1.SetKind(ptrace.SpanKindServer)
	span1.Attributes().PutInt("attr1", 55)
	span2 := spans.AppendEmpty()
	span2.SetTraceID([16]byte{0x5B, 0x8E, 0xFF, 0xF7, 0x98, 0x3, 0x81, 0x3, 0xD2, 0x69, 0xB6, 0x33, 0x81, 0x3F, 0xC6, 0xC})
	span2.SetSpanID([8]byte{0xEE, 0xE1, 0x9B, 0x7E, 0xC3, 0xC1, 0xB1, 0x73})
	span2.SetName("testSpan2")
	span2.SetStartTimestamp(1544712660000000000)
	span2.SetEndTimestamp(1544712661000000000)
	span2.SetKind(ptrace.SpanKindClient)
	span2.Attributes().PutInt("attr1", 55)
	return td
}
